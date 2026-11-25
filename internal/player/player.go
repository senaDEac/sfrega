// Package player implementa o núcleo do player: playlist, transporte,
// pipeline de decode → análise de espectro → ring buffer → dispositivo.
package player

import (
	"bufio"
	"encoding/json"
	"errors"
	"image"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"sfrega/internal/audio"
	"sfrega/internal/spectrum"
)

// eqBandFreqs são as frequências centrais do equalizador de 10 bandas (ISO).
var eqBandFreqs = [10]float64{31, 62, 125, 250, 500, 1000, 2000, 4000, 8000, 16000}

// biquad é um filtro biquad (RBJ peaking EQ).
type biquad struct {
	b0, b1, b2, a1, a2 float64
	z1, z2             float64
}

func (b *biquad) process(x float64) float64 {
	y := b.b0*x + b.z1
	b.z1 = b.b1*x - b.a1*y + b.z2
	b.z2 = b.b2*x - b.a2*y
	return y
}

func (b *biquad) reset() { b.z1, b.z2 = 0, 0 }

// eqState contém os filtros EQ por banda e canal.
type eqState struct {
	filters [10][2]biquad
}

// eqFromGains calcula coeficientes para ganhos em dB (len deve ser 10).
func eqFromGains(gainsDb [10]float64, sampleRate uint32) *eqState {
	e := &eqState{}
	for b := 0; b < 10; b++ {
		e.filters[b][0] = biquad{b0: 1} // identity (bypass)
		e.filters[b][1] = biquad{b0: 1}
	}
	Q := 1.4
	for band := 0; band < 10; band++ {
		db := gainsDb[band]
		if math.Abs(db) < 0.01 {
			continue // bypass
		}
		A := math.Pow(10, db/40)
		w0 := 2 * math.Pi * eqBandFreqs[band] / float64(sampleRate)
		cosw := math.Cos(w0)
		sinw := math.Sin(w0)
		alpha := sinw / (2 * Q)
		b0 := 1 + alpha*A
		b1 := -2 * cosw
		b2 := 1 - alpha*A
		a0 := 1 + alpha/A
		a1 := -2 * cosw
		a2 := 1 - alpha/A
		for ch := 0; ch < 2; ch++ {
			e.filters[band][ch] = biquad{
				b0: b0 / a0, b1: b1 / a0, b2: b2 / a0,
				a1: a1 / a0, a2: a2 / a0,
			}
		}
	}
	return e
}

const (
	decodeChunkFrames = 4096
	ringSeconds       = 2 // ~2s de buffer
)

// Event é emitido para a UI.
type Event struct {
	Kind     string // "track-change", "state-change", "eof", "error", "spectrum"
	Index    int
	Title    string
	Playing  bool
	Volume   float32
	Duration time.Duration
	Bands    [spectrum.Bands]float32
	Err      error
}

// Player é o núcleo do player.
type Player struct {
	mu       sync.Mutex
	tracks   []string
	index    int
	listFile string        // playlist persistida em m3u ("" = desativado)
	tagsFile string        // cache de tags persistido em tags.json
	lib      *trackLibrary // metadados por faixa (plays/rating/tags)
	dec      *audio.Decoder
	rb       *audio.RingBuffer
	dev      *audio.Device
	analyzer *spectrum.Analyzer

	playing    atomic.Bool
	repeat     atomic.Bool
	volume     atomic.Uint32 // float32 bits
	eq         atomic.Value  // *eqState
	preamp     atomic.Uint32 // float32 bits (ganho pré-EQ em dB, clamp ±12)
	pan        atomic.Uint32 // float32 bits (-1 esquerda .. +1 direita)
	decoded    atomic.Uint64 // frames decodificados totais da faixa
	done       chan struct{}
	trackDone  chan struct{} // cancelado ao trocar de faixa
	events     chan Event
	lastBands  atomic.Value // [spectrum.Bands]float32
	tagMu      sync.Mutex
	tagCache   map[string]tagInfo // cache de metadados de áudio por path
	artDir     string             // diretório das capas extraídas (covers/)
	coverMu    sync.Mutex
	coverCache map[string]image.Image // thumbs por nome de capa
	coverPend  map[string]bool        // decodes de thumb em andamento
	genreMu    sync.Mutex
	genresList []string // gêneros distintos (preenchido pelo warmTags)
	artistMu   sync.Mutex
	artistsList []string          // artistas distintos (preenchido pelo warmTags)
	artistFirst map[string]string // artista → path da primeira ocorrência
	artistCount map[string]int    // artista → total de faixas
	foldersList []string          // diretórios distintos (preenchido pelo warmTags)
	folderFirst map[string]string // dir → path da primeira ocorrência
	folderCount map[string]int    // dir → total de faixas
	albumsList  []string          // álbuns distintos (preenchido pelo warmTags)
	albumFirst  map[string]string // álbum → path da primeira ocorrência
	albumCount  map[string]int    // álbum → total de faixas
}

// tagPeek retorna o tag em cache sem ler o arquivo (seguro para a UI).
func (p *Player) tagPeek(path string) (tagInfo, bool) {
	p.tagMu.Lock()
	defer p.tagMu.Unlock()
	t, ok := p.tagCache[path]
	return t, ok
}

// tagForPath retorna os metadados de áudio da faixa (com cache).
func (p *Player) tagForPath(path string) tagInfo {
	p.tagMu.Lock()
	if t, ok := p.tagCache[path]; ok {
		p.tagMu.Unlock()
		return t
	}
	p.tagMu.Unlock()
	t := readAudioTag(path)
	p.tagMu.Lock()
	if p.tagCache == nil {
		p.tagCache = map[string]tagInfo{}
	}
	p.tagCache[path] = t
	p.tagMu.Unlock()
	return t
}

// warmTags pré-carrega tags das faixas em background e calcula os gêneros.
// Não pode ser chamado com p.mu segurado.
func (p *Player) warmTags(paths []string) {
	cp := append([]string(nil), paths...)
	p.computeGenres(cp) // cache pré-carregado do tags.json: gêneros instantâneos
	jobs := make(chan string)
	var wg sync.WaitGroup
	for w := 0; w < 8; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for path := range jobs {
				t := p.tagForPath(path)
				if t.Cover == "" {
					art := t.Art
					if len(art) == 0 {
						art = readAudioTag(path).Art // tag do cache: relê p/ arte
					}
					cov := p.saveCover(path, art)
					p.tagMu.Lock()
					if c, ok := p.tagCache[path]; ok {
						c.Cover = cov
						c.Art = nil
						p.tagCache[path] = c
					}
					p.tagMu.Unlock()
				}
			}
		}()
	}
	done := make(chan struct{})
	go func() { // gêneros progridem enquanto o warm roda
		tk := time.NewTicker(2 * time.Second)
		defer tk.Stop()
		for {
			select {
			case <-done:
				return
			case <-tk.C:
				p.computeGenres(cp)
				p.computeArtists(cp)
			}
		}
	}()
	go func() {
		for _, path := range cp {
			jobs <- path
		}
		close(jobs)
		wg.Wait()
		close(done)
		p.computeGenres(cp)
		p.computeArtists(cp)
		p.saveTags()
	}()
}

// computeGenres recalcula a lista de gêneros distintos na ordem das faixas.
func (p *Player) computeGenres(paths []string) {
	seen := map[string]bool{}
	var genres []string
	for _, path := range paths {
		t, _ := p.tagPeek(path)
		if t.Genre != "" && !seen[t.Genre] {
			seen[t.Genre] = true
			genres = append(genres, t.Genre)
		}
	}
	p.genreMu.Lock()
	p.genresList = genres
	p.genreMu.Unlock()
}

// computeArtists recalcula a lista de artistas distintos na ordem das faixas.
func (p *Player) computeArtists(paths []string) {
	seen := map[string]bool{}
	first := map[string]string{}
	count := map[string]int{}
	var artists []string
	seenF := map[string]bool{}
	fFirst := map[string]string{}
	fCount := map[string]int{}
	var folders []string
	seenA := map[string]bool{}
	aFirst := map[string]string{}
	aCount := map[string]int{}
	var albums []string
	for _, path := range paths {
		t, _ := p.tagPeek(path)
		if t.Artist != "" {
			count[t.Artist]++
			if !seen[t.Artist] {
				seen[t.Artist] = true
				first[t.Artist] = path
				artists = append(artists, t.Artist)
			}
		}
		dir := filepath.Dir(path)
		fCount[dir]++
		if !seenF[dir] {
			seenF[dir] = true
			fFirst[dir] = path
			folders = append(folders, dir)
		}
		if t.Album != "" {
			aCount[t.Album]++
			if !seenA[t.Album] {
				seenA[t.Album] = true
				aFirst[t.Album] = path
				albums = append(albums, t.Album)
			}
		}
	}
	sort.Slice(artists, func(a, b int) bool {
		return strings.ToLower(artists[a]) < strings.ToLower(artists[b])
	})
	sort.Slice(folders, func(a, b int) bool {
		return strings.ToLower(filepath.Base(folders[a])) < strings.ToLower(filepath.Base(folders[b]))
	})
	sort.Slice(albums, func(a, b int) bool {
		return strings.ToLower(albums[a]) < strings.ToLower(albums[b])
	})
	p.artistMu.Lock()
	p.artistsList = artists
	p.artistFirst = first
	p.artistCount = count
	p.foldersList = folders
	p.folderFirst = fFirst
	p.folderCount = fCount
	p.albumsList = albums
	p.albumFirst = aFirst
	p.albumCount = aCount
	p.artistMu.Unlock()
}

// ArtistTrackCount retorna o total de faixas do artista.
func (p *Player) ArtistTrackCount(artist string) int {
	p.artistMu.Lock()
	defer p.artistMu.Unlock()
	return p.artistCount[artist]
}

// ArtistCover retorna a capa da primeira faixa do artista (nil se não houver).
func (p *Player) ArtistCover(artist string) image.Image {
	p.artistMu.Lock()
	path := p.artistFirst[artist]
	p.artistMu.Unlock()
	return p.coverForPath(path)
}

// Artists retorna os artistas distintos das faixas (cache do warmTags).
func (p *Player) Artists() []string {
	p.artistMu.Lock()
	defer p.artistMu.Unlock()
	return append([]string(nil), p.artistsList...)
}

// Folders retorna os diretórios distintos das faixas, ordenados por nome.
func (p *Player) Folders() []string {
	p.artistMu.Lock()
	defer p.artistMu.Unlock()
	return append([]string(nil), p.foldersList...)
}

// FolderName retorna o nome de exibição do diretório (base).
func (p *Player) FolderName(dir string) string {
	return filepath.Base(dir)
}

// FolderTrackCount retorna o total de faixas do diretório.
func (p *Player) FolderTrackCount(dir string) int {
	p.artistMu.Lock()
	defer p.artistMu.Unlock()
	return p.folderCount[dir]
}

// FolderCover retorna a capa da primeira faixa do diretório (nil se não houver).
func (p *Player) FolderCover(dir string) image.Image {
	p.artistMu.Lock()
	path := p.folderFirst[dir]
	p.artistMu.Unlock()
	return p.coverForPath(path)
}

// FolderTracks retorna os índices das faixas do diretório.
func (p *Player) FolderTracks(dir string) []int {
	p.mu.Lock()
	tracks := append([]string(nil), p.tracks...)
	p.mu.Unlock()
	var out []int
	for i, path := range tracks {
		if filepath.Dir(path) == dir {
			out = append(out, i)
		}
	}
	return out
}

// Albums retorna os álbuns distintos das faixas, ordenados por nome.
func (p *Player) Albums() []string {
	p.artistMu.Lock()
	defer p.artistMu.Unlock()
	return append([]string(nil), p.albumsList...)
}

// AlbumTrackCount retorna o total de faixas do álbum.
func (p *Player) AlbumTrackCount(album string) int {
	p.artistMu.Lock()
	defer p.artistMu.Unlock()
	return p.albumCount[album]
}

// AlbumCover retorna a capa da primeira faixa do álbum (nil se não houver).
func (p *Player) AlbumCover(album string) image.Image {
	p.artistMu.Lock()
	path := p.albumFirst[album]
	p.artistMu.Unlock()
	return p.coverForPath(path)
}

// AlbumTracks retorna os índices das faixas do álbum.
func (p *Player) AlbumTracks(album string) []int {
	var out []int
	for i := 0; i < p.Count(); i++ {
		if p.trackAlbum(i) == album {
			out = append(out, i)
		}
	}
	return out
}

// trackAlbum retorna o álbum do metadado da faixa i ("" se não houver).
func (p *Player) trackAlbum(i int) string {
	p.mu.Lock()
	var path string
	if i >= 0 && i < len(p.tracks) {
		path = p.tracks[i]
	}
	p.mu.Unlock()
	if path == "" {
		return ""
	}
	t, _ := p.tagPeek(path)
	return t.Album
}

// TracksForArtist retorna os índices das faixas do artista ("" = todas).
func (p *Player) TracksForArtist(artist string) []int {
	p.mu.Lock()
	n := len(p.tracks)
	p.mu.Unlock()
	if artist == "" {
		out := make([]int, n)
		for i := range out {
			out[i] = i
		}
		return out
	}
	var out []int
	for i := 0; i < n; i++ {
		if p.trackArtist(i) == artist {
			out = append(out, i)
		}
	}
	return out
}

// trackArtist retorna o artista do metadado da faixa i ("" se não houver).
func (p *Player) trackArtist(i int) string {
	p.mu.Lock()
	var path string
	if i >= 0 && i < len(p.tracks) {
		path = p.tracks[i]
	}
	p.mu.Unlock()
	if path == "" {
		return ""
	}
	t, _ := p.tagPeek(path)
	return t.Artist
}

// TrackTitle retorna o nome de exibição da faixa i: título do metadado
// (ou "Artista - Título") quando existe; senão o nome do arquivo.
// hasMeta indica se o nome veio de metadados (para destaque na UI).
func (p *Player) TrackTitle(i int) (name string, hasMeta bool) {
	p.mu.Lock()
	var path string
	if i >= 0 && i < len(p.tracks) {
		path = p.tracks[i]
	}
	p.mu.Unlock()
	if path == "" {
		return "", false
	}
	t, _ := p.tagPeek(path)
	return displayNameCached(t, path), t.Title != "" || t.Artist != ""
}

// TrackParts retorna artista e título separados do metadado da faixa i.
func (p *Player) TrackParts(i int) (artist, title string, hasMeta bool) {
	p.mu.Lock()
	var path string
	if i >= 0 && i < len(p.tracks) {
		path = p.tracks[i]
	}
	p.mu.Unlock()
	if path == "" {
		return "", "", false
	}
	t, _ := p.tagPeek(path)
	return t.Artist, t.Title, t.Title != "" || t.Artist != ""
}

// TrackGenre retorna o gênero da faixa i (metadado), "" se não houver.
func (p *Player) TrackGenre(i int) string {
	p.mu.Lock()
	var path string
	if i >= 0 && i < len(p.tracks) {
		path = p.tracks[i]
	}
	p.mu.Unlock()
	if path == "" {
		return ""
	}
	t, _ := p.tagPeek(path)
	return t.Genre
}

// Genres retorna os gêneros distintos das faixas (cache do warmTags).
func (p *Player) Genres() []string {
	p.genreMu.Lock()
	defer p.genreMu.Unlock()
	return append([]string(nil), p.genresList...)
}

// TracksForGenre retorna os índices das faixas do gênero ("" = todas).
func (p *Player) TracksForGenre(genre string) []int {
	p.mu.Lock()
	n := len(p.tracks)
	p.mu.Unlock()
	if genre == "" {
		out := make([]int, n)
		for i := range out {
			out[i] = i
		}
		return out
	}
	var out []int
	for i := 0; i < n; i++ {
		if p.TrackGenre(i) == genre {
			out = append(out, i)
		}
	}
	return out
}

func displayNameCached(t tagInfo, path string) string {
	if t.Title != "" && t.Artist != "" {
		return t.Artist + " - " + t.Title
	}
	if t.Title != "" {
		return t.Title
	}
	if t.Artist != "" {
		return t.Artist
	}
	return filepath.Base(path)
}

// New cria um Player. rate/channels são do dispositivo de saída.
func New(rate uint32, channels uint32) (*Player, error) {
	capSamples := uint64(rate) * uint64(channels) * ringSeconds
	rb := audio.NewRingBuffer(capSamples)
	if rb == nil {
		return nil, errors.New("player: falha ao criar ring buffer")
	}
	p := &Player{
		rb:       rb,
		done:     make(chan struct{}),
		events:   make(chan Event, 64),
		analyzer: spectrum.New(),
	}
	p.volume.Store(math.Float32bits(1.0))
	p.preamp.Store(math.Float32bits(1))
	p.pan.Store(math.Float32bits(0))
	p.eq.Store(eqFromGains([10]float64{}, 44100))
	go p.resumeLoop()
	return p, nil
}

func (p *Player) Events() <-chan Event { return p.events }

func (p *Player) emit(e Event) {
	// persiste a última faixa tocada (Title carrega o path) para retomada
	if e.Kind == "track-change" && e.Title != "" && p.listFile != "" {
		_ = os.WriteFile(p.resumePath(), []byte(e.Title), 0o644)
	}
	select {
	case p.events <- e:
	default:
	}
}

// resumePath retorna o arquivo que guarda a última faixa + posição.
func (p *Player) resumePath() string {
	return filepath.Join(filepath.Dir(p.listFile), "resume.txt")
}

// saveResume grava path + fração de posição da faixa atual.
func (p *Player) saveResume() {
	if p.listFile == "" {
		return
	}
	p.mu.Lock()
	if p.index < 0 || p.index >= len(p.tracks) {
		p.mu.Unlock()
		return
	}
	data := p.tracks[p.index]
	if p.dec != nil {
		if dur := p.dec.LengthFrames(); dur > 0 {
			if frac := float64(p.playedFrames(p.dec.SampleRate(), p.dec.Channels())) / float64(dur); frac > 0.001 {
				data += "\n" + strconv.FormatFloat(frac, 'g', -1, 64)
			}
		}
	}
	p.mu.Unlock()
	_ = os.WriteFile(p.resumePath(), []byte(data), 0o644)
}

// resumeLoop grava a posição periodicamente (sobrevive a kill -9/SIGTERM).
func (p *Player) resumeLoop() {
	tk := time.NewTicker(5 * time.Second)
	defer tk.Stop()
	for {
		select {
		case <-p.done:
			return
		case <-tk.C:
			p.saveResume()
		}
	}
}

// SetPlaylistFile define o arquivo m3u onde a playlist é persistida
// a cada mutação ("" desativa).
func (p *Player) SetPlaylistFile(path string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.listFile = path
	if path != "" {
		dir := filepath.Dir(path)
		p.lib = newTrackLibrary(filepath.Join(dir, "library.json"))
		p.tagsFile = filepath.Join(dir, "tags.json")
		p.artDir = filepath.Join(dir, "covers")
		_ = os.MkdirAll(p.artDir, 0o755)
		p.loadTags()
	} else {
		p.lib = nil
		p.tagsFile = ""
		p.artDir = ""
	}
}

// loadTags carrega o cache de tags persistido (título/artista/gênero).
func (p *Player) loadTags() {
	if p.tagsFile == "" {
		return
	}
	b, err := os.ReadFile(p.tagsFile)
	if err != nil {
		return
	}
	var m map[string]tagInfo
	if json.Unmarshal(b, &m) != nil {
		return
	}
	p.tagMu.Lock()
	if p.tagCache == nil {
		p.tagCache = map[string]tagInfo{}
	}
	for k, v := range m {
		p.tagCache[k] = v
	}
	p.tagMu.Unlock()
}

// saveTags grava o cache de tags de forma atômica.
func (p *Player) saveTags() {
	if p.tagsFile == "" {
		return
	}
	p.tagMu.Lock()
	m := make(map[string]tagInfo, len(p.tagCache))
	for k, v := range p.tagCache {
		m[k] = v
	}
	p.tagMu.Unlock()
	b, err := json.Marshal(m)
	if err != nil {
		return
	}
	tmp := p.tagsFile + ".tmp"
	if os.WriteFile(tmp, b, 0o644) == nil {
		_ = os.Rename(tmp, p.tagsFile)
	}
}

// saveTracksLocked grava a playlist em p.listFile (formato m3u).
func (p *Player) saveTracksLocked() {
	if p.listFile == "" {
		return
	}
	_ = os.MkdirAll(filepath.Dir(p.listFile), 0o755)
	var b strings.Builder
	b.WriteString("#EXTM3U\n")
	for _, t := range p.tracks {
		b.WriteString(t + "\n")
	}
	_ = os.WriteFile(p.listFile, []byte(b.String()), 0o644)
}

// LoadPlaylist lê um arquivo m3u e retorna as faixas (ignora comentários).
func LoadPlaylist(path string) []string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	var out []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		l := strings.TrimSpace(sc.Text())
		if l == "" || strings.HasPrefix(l, "#") {
			continue
		}
		out = append(out, l)
	}
	return out
}

// PlaylistFilePath retorna o caminho padrão de persistência da playlist.
func PlaylistFilePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "/tmp"
	}
	return filepath.Join(home, ".local", "share", "sfrega", "configs", "playlist.m3u")
}

// SetTracks define a playlist.
func (p *Player) SetTracks(tracks []string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.tracks = append([]string(nil), tracks...)
	p.index = -1
	if len(tracks) > 0 {
		p.index = 0
	}
	p.ensureMetaLocked()
	p.saveTracksLocked()
	p.emit(Event{Kind: "track-change", Index: p.index, Title: p.currentTitle()})
	p.warmTags(tracks)
}

// AddTracks anexa faixas ao fim da playlist sem interromper a reprodução.
func (p *Player) AddTracks(tracks []string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(tracks) == 0 {
		return
	}
	p.tracks = append(p.tracks, tracks...)
	if p.index < 0 {
		p.index = 0
	}
	p.ensureMetaLocked()
	p.saveTracksLocked()
	p.emit(Event{Kind: "track-change", Index: p.index, Title: p.currentTitle()})
	p.warmTags(tracks)
}

// ensureMetaLocked cria entradas de metadados (com hashtags) para faixas novas.
func (p *Player) ensureMetaLocked() {
	if p.lib == nil {
		return
	}
	for _, t := range p.tracks {
		p.lib.ensure(t)
	}
	p.lib.flush()
}

// TrackMeta retorna os metadados persistidos da faixa (nil se sem biblioteca).
func (p *Player) TrackMeta(path string) *TrackMeta {
	p.mu.Lock()
	lib := p.lib
	p.mu.Unlock()
	if lib == nil {
		return nil
	}
	return lib.get(path)
}

// SetRating define a nota (0..5) da faixa.
func (p *Player) SetRating(path string, r int) {
	p.mu.Lock()
	lib := p.lib
	p.mu.Unlock()
	if lib == nil {
		return
	}
	lib.setRating(path, r)
	lib.flush()
}

func (p *Player) currentTitle() string {
	if p.index >= 0 && p.index < len(p.tracks) {
		return p.tracks[p.index]
	}
	return ""
}

// TrackAt retorna o caminho da faixa i (ou "").
func (p *Player) TrackAt(i int) string {
	if i >= 0 && i < len(p.tracks) {
		return p.tracks[i]
	}
	return ""
}

// PlayIndex toca a faixa i da playlist.
func (p *Player) PlayIndex(i int) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if i < 0 || i >= len(p.tracks) {
		return errors.New("player: índice inválido")
	}
	p.index = i
	if p.dec != nil {
		p.closeTrackLocked()
		p.rb.Clear()
		p.decoded.Store(0)
	}
	if err := p.openCurrentLocked(); err != nil {
		return err
	}
	p.playing.Store(true)
	p.emit(Event{Kind: "track-change", Index: i, Title: p.currentTitle()})
	return nil
}

func (p *Player) Index() int    { return p.index }
func (p *Player) Count() int    { return len(p.tracks) }
func (p *Player) Playing() bool { return p.playing.Load() }

// SetRepeat liga/desliga o repeat da playlist.
func (p *Player) SetRepeat(on bool) { p.repeat.Store(on) }

// Repeat retorna o estado do repeat.
func (p *Player) Repeat() bool { return p.repeat.Load() }

// Volume retorna 0..1.
func (p *Player) Volume() float32 { return math.Float32frombits(p.volume.Load()) }

// SetVolume define volume 0..1.
func (p *Player) SetVolume(v float32) {
	if v < 0 {
		v = 0
	}
	if v > 1 {
		v = 1
	}
	p.volume.Store(math.Float32bits(v))
	p.mu.Lock()
	if p.dev != nil {
		p.dev.SetVolume(v)
	}
	p.mu.Unlock()
	p.emit(Event{Kind: "state-change", Volume: v, Playing: p.playing.Load()})
}

// SetEQ define os ganhos do equalizador em dB (10 bandas ISO).
// Requer sampleRate para calcular coeficientes; use EQSampleRate ou 0 para
// usar a taxa da faixa atual quando souber.
func (p *Player) SetEQ(gainsDb [10]float64, sampleRate uint32) {
	if sampleRate == 0 {
		p.mu.Lock()
		if p.dec != nil {
			sampleRate = p.dec.SampleRate()
		}
		p.mu.Unlock()
		if sampleRate == 0 {
			sampleRate = 44100
		}
	}
	p.eq.Store(eqFromGains(gainsDb, sampleRate))
}

// EQGains retorna ganhos atuais (se trackeados externamente).
// SetPreamp define ganho pré-EQ em dB (±12).
func (p *Player) SetPreamp(db float64) {
	if db > 12 {
		db = 12
	}
	if db < -12 {
		db = -12
	}
	// converte dB para multiplicador e guarda em volume-like? guardamos o
	// multiplicador linear em preamp (float32 bits)
	p.preamp.Store(math.Float32bits(float32(math.Pow(10, db/20))))
}

func (p *Player) Preamp() float32 { return math.Float32frombits(p.preamp.Load()) }

// SetBalance define o balanço -1 (esquerda) .. +1 (direita).
func (p *Player) SetBalance(b float32) {
	if b < -1 {
		b = -1
	}
	if b > 1 {
		b = 1
	}
	p.pan.Store(math.Float32bits(b))
	p.mu.Lock()
	if p.dev != nil {
		p.dev.SetPan(b)
	}
	p.mu.Unlock()
}

func (p *Player) Balance() float32 { return math.Float32frombits(p.pan.Load()) }

// Channels retorna o número de canais da faixa atual (0 se nenhuma).
func (p *Player) Channels() uint32 {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.dec == nil {
		return 0
	}
	return p.dec.Channels()
}

// SeekFraction posiciona em fração 0..1 da duração.
func (p *Player) SeekFraction(f float64) error {
	if f < 0 {
		f = 0
	}
	if f > 1 {
		f = 1
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.dec == nil {
		return errors.New("player: nada tocando")
	}
	total := p.dec.LengthFrames()
	if !p.dec.SeekFrame(int64(float64(total) * f)) {
		return errors.New("player: seek falhou")
	}
	p.rb.Clear()
	p.decoded.Store(uint64(int64(float64(total) * f)))
	return nil
}

// Tracks retorna uma cópia da playlist.
func (p *Player) Tracks() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.tracks...)
}

// RemoveAt remove a faixa i da playlist.
func (p *Player) RemoveAt(i int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if i < 0 || i >= len(p.tracks) {
		return
	}
	p.tracks = append(p.tracks[:i], p.tracks[i+1:]...)
	if len(p.tracks) == 0 {
		p.index = -1
		p.resetLocked()
		p.saveTracksLocked()
		return
	}
	if i < p.index {
		p.index--
	} else if i == p.index {
		p.index = -1
		p.index = 0
		if p.dec != nil {
			p.closeTrackLocked()
			p.decoded.Store(0)
		}
	}
	p.saveTracksLocked()
	p.emit(Event{Kind: "track-change", Index: p.index, Title: p.currentTitle()})
}

// Move move a faixa de from para to.
func (p *Player) Move(from, to int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if from < 0 || from >= len(p.tracks) || to < 0 || to >= len(p.tracks) || from == to {
		return
	}
	t := p.tracks[from]
	p.tracks = append(p.tracks[:from], p.tracks[from+1:]...)
	p.tracks = append(p.tracks[:to], append([]string{t}, p.tracks[to:]...)...)
	if p.index == from {
		p.index = to
	} else if from < p.index && to >= p.index {
		p.index--
	} else if from > p.index && to <= p.index {
		p.index++
	}
	p.saveTracksLocked()
}

// Play inicia a faixa atual.
func (p *Player) Play() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.index < 0 || p.index >= len(p.tracks) {
		return errors.New("player: playlist vazia")
	}
	if p.dec == nil {
		if err := p.openCurrentLocked(); err != nil {
			return err
		}
	}
	if !p.playing.Load() {
		p.playing.Store(true)
		p.emit(Event{Kind: "state-change", Playing: true, Volume: p.Volume()})
	}
	return nil
}

func (p *Player) Pause() {
	if p.playing.CompareAndSwap(true, false) {
		p.rb.Clear()
		p.emit(Event{Kind: "state-change", Playing: false, Volume: p.Volume()})
	}
}

func (p *Player) Toggle() {
	if p.playing.Load() {
		p.Pause()
	} else {
		_ = p.Play()
	}
}

// Next avança; no fim da playlist para.
func (p *Player) Next() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.index+1 < len(p.tracks) {
		p.index++
		p.playNextLocked()
	} else {
		p.index = -1
		p.resetLocked()
	}
}

// Prev volta.
func (p *Player) Prev() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.index-1 >= 0 {
		p.index--
		p.playNextLocked()
	} else {
		p.resetLocked()
	}
	p.emit(Event{Kind: "track-change", Index: p.index, Title: p.currentTitle()})
}

// playNextLocked assume lock; troca de faixa mantendo estado de play.
func (p *Player) playNextLocked() {
	wasPlaying := p.playing.Load()
	p.closeTrackLocked()
	p.rb.Clear()
	p.decoded.Store(0)
	if wasPlaying {
		p.playing.Store(false)
		_ = p.openCurrentLocked()
		p.playing.Store(true)
	} else {
		p.playing.Store(false)
	}
	p.emit(Event{Kind: "track-change", Index: p.index, Title: p.currentTitle()})
}

func (p *Player) resetLocked() {
	p.closeTrackLocked()
	p.rb.Clear()
	p.playing.Store(false)
	p.decoded.Store(0)
	p.emit(Event{Kind: "track-change", Index: p.index, Title: p.currentTitle()})
}

func (p *Player) closeTrackLocked() {
	if p.trackDone != nil {
		close(p.trackDone)
		p.trackDone = nil
	}
	if p.dec != nil {
		p.dec.Close()
		p.dec = nil
	}
	if p.dev != nil {
		p.dev.Close()
		p.dev = nil
	}
}

func (p *Player) openCurrentLocked() error {
	path := p.tracks[p.index]
	dec, err := audio.OpenDecoder(path)
	if err != nil {
		p.emit(Event{Kind: "error", Err: err, Index: p.index})
		return err
	}
	rate := dec.SampleRate()
	ch := dec.Channels()
	dev, err := audio.OpenDevice(rate, ch, p.rb)
	if err != nil {
		dec.Close()
		return err
	}
	dev.SetVolume(p.Volume())
	dev.SetPan(p.Balance())
	p.dec = dec
	p.dev = dev
	if !dev.Start() {
		dec.Close()
		dev.Close()
		p.dec, p.dev = nil, nil
		return errors.New("player: falha ao iniciar dispositivo")
	}
	p.decoded.Store(0)
	p.decoded.Store(0)
	if p.lib != nil {
		p.lib.addPlay(path)
		p.lib.flush()
	}
	p.trackDone = make(chan struct{})
	p.emit(Event{Kind: "track-change", Index: p.index, Title: path,
		Duration: time.Duration(dec.LengthFrames()) * time.Second / time.Duration(rate)})
	go p.decodeLoop(dec, rate, ch, p.trackDone)
	return nil
}

// decodeLoop decodifica a faixa, aplica EQ/preamp, alimenta analisador e ring buffer.
func (p *Player) decodeLoop(dec *audio.Decoder, rate, channels uint32, stop <-chan struct{}) {
	chBuf := make([]float32, decodeChunkFrames*channels)
	eq := p.eq.Load().(*eqState)
	preamp := p.Preamp()
	for {
		select {
		case <-p.done:
			return
		case <-stop:
			return
		default:
		}
		if !p.playing.Load() {
			time.Sleep(20 * time.Millisecond)
			continue
		}
		frames := dec.ReadPCM(chBuf, decodeChunkFrames)
		if frames == 0 {
			p.emit(Event{Kind: "eof", Index: p.index})
			p.autoAdvance()
			return
		}
		samples := int(frames) * int(channels)
		chunk := chBuf[:samples]
		// EQ + preamp (recarrega estado a cada chunk p/ sliders ao vivo)
		if eq2, ok := p.eq.Load().(*eqState); ok && eq2 != eq {
			eq = eq2
		}
		preamp = p.Preamp()
		if channels == 2 {
			for i := 0; i < samples; i += 2 {
				l := float64(chunk[i])
				r := float64(chunk[i+1])
				for b := 0; b < 10; b++ {
					l = eq.filters[b][0].process(l)
					r = eq.filters[b][1].process(r)
				}
				chunk[i] = float32(l * float64(preamp))
				chunk[i+1] = float32(r * float64(preamp))
			}
		} else {
			for i := 0; i < samples; i++ {
				l := float64(chunk[i])
				for b := 0; b < 10; b++ {
					l = eq.filters[b][0].process(l)
				}
				chunk[i] = float32(l * float64(preamp))
			}
		}
		bands := p.analyzer.Feed(chunk, rate)
		p.lastBands.Store(bands)
		// escreve no ring buffer (parcial + espera)
		written := 0
		for written < samples {
			n := p.rb.Write(chunk[written:])
			if n == 0 {
				time.Sleep(2 * time.Millisecond)
				continue
			}
			written += n
		}
		p.decoded.Add(uint64(frames))
	}
}

// autoAdvance decide o que fazer ao fim da faixa: próxima faixa, wrap
// com repeat, ou volta ao início pausado.
func (p *Player) autoAdvance() {
	p.mu.Lock()
	n := len(p.tracks)
	hasNext := p.index+1 < n
	rep := p.repeat.Load()
	p.mu.Unlock()
	switch {
	case hasNext:
		p.Next()
	case rep && n > 0:
		_ = p.PlayIndex(0)
	default:
		_ = p.SeekFrame(0)
		p.Pause()
	}
}

// playedFrames estima frames já tocados = decodificados - buffer restante.
func (p *Player) playedFrames(rate, channels uint32) uint64 {
	dec := p.decoded.Load()
	avail := p.rb.Available()
	ch := uint64(channels)
	if ch == 0 {
		ch = 2
	}
	buffered := avail / ch
	if dec > buffered {
		return dec - buffered
	}
	return 0
}

// PositionFrames retorna posição aproximada em frames.
func (p *Player) PositionFrames() uint64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.dec == nil {
		return 0
	}
	return p.playedFrames(p.dec.SampleRate(), p.dec.Channels())
}

// SeekFrame posiciona em frame de PCM.
func (p *Player) SeekFrame(frame int64) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.dec == nil {
		return errors.New("player: nada tocando")
	}
	if !p.dec.SeekFrame(frame) {
		return errors.New("player: seek falhou")
	}
	p.rb.Clear()
	p.decoded.Store(uint64(frame))
	return nil
}

// Spectrum retorna o último espectro calculado.
func (p *Player) Spectrum() [spectrum.Bands]float32 {
	if v, ok := p.lastBands.Load().([spectrum.Bands]float32); ok {
		return v
	}
	return [spectrum.Bands]float32{}
}

// Duration retorna duração da faixa atual.
func (p *Player) Duration() time.Duration {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.dec == nil {
		return 0
	}
	return time.Duration(p.dec.LengthFrames()) * time.Second / time.Duration(p.dec.SampleRate())
}

// Position retorna posição atual (aprox., baseada em frames escritos no buffer
// menos o buffer restante — estimativa simples por pos de decode).
func (p *Player) Position() time.Duration {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.dec == nil {
		return 0
	}
	return time.Duration(p.playedFrames(p.dec.SampleRate(), p.dec.Channels())) * time.Second / time.Duration(p.dec.SampleRate())
}

// Stats retorna (frames decodificados, frames no buffer, frames tocados).
func (p *Player) Stats() (decoded, buffered, played uint64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.dec == nil {
		return 0, 0, 0
	}
	ch := p.dec.Channels()
	if ch == 0 {
		ch = 2
	}
	return p.decoded.Load(), p.rb.Available() / uint64(ch), p.playedFrames(p.dec.SampleRate(), ch)
}

// Close libera recursos.
func (p *Player) Close() {
	close(p.done)
	p.saveResume() // antes do lock; decoder ainda vivo
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closeTrackLocked()
	p.rb.Destroy()
	if p.lib != nil {
		p.lib.flush()
	}
	p.saveTags()
}
