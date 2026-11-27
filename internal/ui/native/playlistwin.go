package native

import (
	"encoding/json"
	"image"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gioui.org/app"
	"gioui.org/io/system"
	"gioui.org/op"
	"gioui.org/unit"
	"gioui.org/widget"
)

// plw é uma janela de playlist (Playlist, Radio ou Video): concentra todo o
// estado da janela — geometria, widgets, abas e a lista de faixas própria —
// para que as três instâncias não se misturem.
type plw struct {
	ui   *uiState
	name string // título da janela
	key  string // "pl" | "rd" | "vd"
	// jsonPath é a persistência da própria janela ("" = playlist principal,
	// que continua usando os arquivos m3u/txt legados).
	jsonPath string

	tracks []string // lista de faixas desta janela
	dirs   []string // diretórios importados (re-varridos no Update lists)
	meta   *plMeta  // índices (gêneros/artistas/álbuns/pastas) desta lista
	dirty  bool     // tracks mudou; meta é recalculado no próximo frame

	win         *app.Window
	plOpen      bool
	plWidth     unit.Dp
	plHeight    unit.Dp
	plScale     int
	plSizeW     int
	plSizeH     int
	shadePL     shadeClk
	plPosDrag   fader
	plList      widget.List
	plRowClicks []uibtn

	plMenuClicks [5][]uibtn
	plMenuOpen   int
	plSel        int
	plScrollAcc  float32

	// grade de estações (só janela Radio): substitui a lista de faixas
	showRadios     bool
	radioScroll    float32
	radioScrollAcc float32
	radioBtns      []uibtn

	plScrollDragging bool
	plScrollDragLast float32
	plScrollSink     *float32

	plGenreOpen      bool
	plGenreSel       string
	plGenreOn        bool
	plGenreExpand    string
	plGenreExpandIdx []int
	plGenreBtns      []uibtn
	plGenreScroll    float32
	plGenreScrollAcc float32
	plGenreMax       float32

	plArtistOpen      bool
	plArtistExpand    string
	plArtistExpandIdx []int
	plArtistBtns      []uibtn
	plArtistScroll    float32
	plArtistScrollAcc float32
	plArtistMax       float32

	plFolderOpen      bool
	plFolderExpand    string
	plFolderExpandIdx []int
	plFolderBtns      []uibtn
	plFolderScroll    float32
	plFolderScrollAcc float32
	plFolderMax       float32

	plAlbumOpen      bool
	plAlbumExpand    string
	plAlbumExpandIdx []int
	plAlbumBtns      []uibtn
	plAlbumScroll    float32
	plAlbumScrollAcc float32
	plAlbumMax       float32

	plGenSel int
	plCount  int
	plCount2 int

	btnPLRemove, btnPLRand, btnPLSort, btnPLWrite uibtn
	btnPLClose, btnPLMin, btnPLSize               uibtn
	btnPLClear                                    uibtn
	btnPLShade                                    uibtn
	btnPLMAdd, btnPLMRem, btnPLMSel               uibtn
	btnPLMMisc, btnPLMList                        uibtn
	btnPLPrev, btnPLPlay, btnPLPause              uibtn
	btnPLStop, btnPLNext, btnPLEject              uibtn
	btnPLG1, btnPLG2, btnPLG3                     uibtn
	btnPLG4, btnPLG5, btnPLG6                     uibtn
}

// plState é o que a janela persiste no seu json (Radio e Video).
type plState struct {
	Open         bool     `json:"open"`
	Name         string   `json:"name"`
	Width        int      `json:"width"`
	Height       int      `json:"height"`
	Scale        int      `json:"scale"`
	Shade        bool     `json:"shade"`
	Tracks       []string `json:"tracks"`
	Dirs         []string `json:"dirs"`
	GenreOpen    bool     `json:"genreOpen"`
	GenreSel     string   `json:"genreSel"`
	GenreOn      bool     `json:"genreOn"`
	GenreExpand  string   `json:"genreExpand"`
	ArtistOpen   bool     `json:"artistOpen"`
	ArtistExpand string   `json:"artistExpand"`
	FolderOpen   bool     `json:"folderOpen"`
	FolderExpand string   `json:"folderExpand"`
	AlbumOpen    bool     `json:"albumOpen"`
	AlbumExpand  string   `json:"albumExpand"`
	GenSel       int      `json:"genSel"`
	Radios       bool     `json:"radios"`
}

// newPLW cria uma janela de playlist. A principal (key "pl") carrega o estado
// legado (txts) e usa a lista do player; as demais leem o próprio json.
func (ui *uiState) newPLW(name, key string) *plw {
	pl := &plw{
		ui:         ui,
		name:       name,
		key:        key,
		meta:       &plMeta{},
		plSel:      -1,
		plMenuOpen: -1,
	}
	if key == "pl" {
		pl.plGenreOpen = loadGenreOpen()
		pl.plGenreSel = loadGenreSel()
		pl.plGenreOn = loadGenreOn()
		pl.plGenreExpand = loadExpand("genreexp.txt")
		pl.plArtistOpen = loadArtistOpen()
		pl.plFolderOpen = loadFolderOpen()
		pl.plAlbumOpen = loadAlbumOpen()
		pl.plArtistExpand = loadExpand("artistexp.txt")
		pl.plFolderExpand = loadExpand("folderexp.txt")
		pl.plAlbumExpand = loadExpand("albumexp.txt")
		pl.plGenSel = loadGenSel()
		pl.plScale = loadScale("plscale.txt")
		pl.plWidth, pl.plHeight = unit.Dp(340), unit.Dp(240)
		pl.plSizeW, pl.plSizeH = loadPLSize()
		if pl.plSizeW > 0 && pl.plSizeH > 0 {
			pl.plWidth, pl.plHeight = unit.Dp(pl.plSizeW), unit.Dp(pl.plSizeH)
		}
		pl.shadePL.on = loadPLShade()
		pl.shadePL.onChange = func(on bool) { savePLShade(on) }
		pl.tracks = ui.p.Tracks()
		pl.dirs = loadDirList("pldirs.txt")
	} else {
		pl.jsonPath = configPath(key + ".json")
		pl.plWidth, pl.plHeight = unit.Dp(340), unit.Dp(240)
		pl.plScale = 1
		pl.loadJSON()
		// quem reabre a janela é o windows.txt; o flag Open do json é só
		// informativo — zerar evita botão marcado/janela fantasma no boot
		pl.plOpen = false
		pl.shadePL.onChange = func(on bool) { pl.saveJSON() }
	}
	pl.plPosDrag.commit = func(v float32) {
		pl.activate()
		ui.p.SeekFraction(float64(v))
	}
	pl.plPosDrag.releaseCommit = true
	pl.rebuildMeta()
	go ui.p.WarmTracks(pl.tracks)
	return pl
}

// toggle abre/fecha esta janela (mesmo esquema do EQ/Playlist).
func (pl *plw) toggle() {
	ui := pl.ui
	ui.mu.Lock()
	defer ui.mu.Unlock()
	if pl.win == nil {
		w := new(app.Window)
		w.Option(
			app.Title(pl.name),
			app.Decorated(false),
			app.Size(pl.plWidth*unit.Dp(max(1, pl.plScale)),
				pl.plHeight*unit.Dp(max(1, pl.plScale))),
		)
		pl.win = w
		pl.plOpen = true
		saveOpenWindows(ui.eqOpen, ui.plMain.plOpen, ui.menuOpen, ui.plRadio.plOpen, ui.plVideo.plOpen)
		go pl.loop()
		if pl.shadePL.on {
			titleH := shadePx(pl.plScale, 18)
			fullH := int(pl.plHeight) * pl.plScale
			go func() {
				for i := 0; i < 60; i++ {
					time.Sleep(50 * time.Millisecond)
					if wmShadeSet(pl.name, titleH, fullH) {
						return
					}
				}
			}()
		}
		return
	}
	pl.plOpen = false
	saveOpenWindows(ui.eqOpen, ui.plMain.plOpen, ui.menuOpen, ui.plRadio.plOpen, ui.plVideo.plOpen)
	// A referência fica até o DestroyEvent: zerar aqui criaria uma janela nova
	// no clique seguinte enquanto a atual ainda existe (duplicata órfã).
	pl.win.Perform(system.ActionClose)
}

// loop é o loop de eventos da janela. Recebe a janela criada para não
// capturar uma referência já substituída.
func (pl *plw) loop() {
	var ops op.Ops
	w := pl.win
	for {
		switch e := w.Event().(type) {
		case app.DestroyEvent:
			wmSaveGeometry()
			ui := pl.ui
			ui.mu.Lock()
			// Limpa só se esta janela ainda é a atual: um DestroyEvent
			// atrasado não pode anular a referência de uma mais nova.
			if pl.win == w {
				pl.win = nil
				pl.plOpen = false
				pl.plMenuOpen = -1
				delete(ui.ontopDone, pl.name)
				if pl.jsonPath != "" {
					pl.saveJSON()
				}
				saveOpenWindows(ui.eqOpen, ui.plMain.plOpen, ui.menuOpen, ui.plRadio.plOpen, ui.plVideo.plOpen)
			}
			ui.mu.Unlock()
			return
		case app.FrameEvent:
			gtx := app.NewContext(&ops, e)
			logical := image.Pt(e.Size.X/pl.plScale, e.Size.Y/pl.plScale)
			if logical.X < 1 {
				logical.X = 1
			}
			if logical.Y < 1 {
				logical.Y = 1
			}
			gtx = beginScaledFrame(gtx, pl.plScale, logical)
			// tamanho só vale em modo janela: em shade o frame reporta a
			// altura da barra (~18px) e frames transitórios de abertura
			// podem vir minúsculos — persistir isso quebraria o tamanho
			// de reabertura ("volta sempre numa linha").
			if logical.X >= 200 {
				pl.plWidth = gtx.Metric.PxToDp(logical.X)
			}
			if !pl.shadePL.on && logical.Y >= 60 {
				pl.plHeight = gtx.Metric.PxToDp(logical.Y)
			}
			pl.layoutPL(gtx)
			e.Frame(gtx.Ops)
		}
	}
}

// activate carrega a lista desta janela no player (uma lista ativa por vez),
// preservando a lista da janela anterior. Toda ação da janela passa por aqui.
func (pl *plw) activate() {
	ui := pl.ui
	if ui.activePL == pl {
		return
	}
	if ui.activePL != nil {
		ui.activePL.tracks = ui.p.Tracks()
		ui.activePL.dirty = true
		// Interrompe a lista anterior: a troca não pode deixar o áudio dela
		// tocando enquanto a nova janela assume o player.
		ui.p.Pause()
	}
	// Tags/rating/capas ficam amarrados ao arquivo do player (mesmo para as
	// outras janelas); noPersist impede só a gravação do m3u/resume, que é
	// exclusiva da playlist principal.
	ui.p.SetNoPersist(pl.key != "pl")
	ui.p.SetTracks(pl.tracks)
	ui.activePL = pl
}

// curIdx devolve o índice tocando (-1 quando esta janela não é a ativa).
func (pl *plw) curIdx() int {
	if pl.ui.activePL != pl {
		return -1
	}
	return pl.ui.p.Index()
}

// tracksAt devolve o caminho da faixa i ("" se fora dos limites).
func (pl *plw) tracksAt(i int) string {
	if i < 0 || i >= len(pl.tracks) {
		return ""
	}
	return pl.tracks[i]
}

// savePanel persiste o estado dos painéis (gêneros/artistas/pastas/álbuns,
// expansões e botão GENEX selecionado). A playlist principal usa os arquivos
// legados; Radio/Video gravam tudo no próprio json — nada é compartilhado.
func (pl *plw) savePanel() {
	if pl.key == "pl" {
		saveGenreOpen(pl.plGenreOpen)
		saveArtistOpen(pl.plArtistOpen)
		saveFolderOpen(pl.plFolderOpen)
		saveAlbumOpen(pl.plAlbumOpen)
		saveGenreSel(pl.plGenreSel)
		saveGenreOn(pl.plGenreOn)
		saveExpand("genreexp.txt", pl.plGenreExpand)
		saveExpand("artistexp.txt", pl.plArtistExpand)
		saveExpand("folderexp.txt", pl.plFolderExpand)
		saveExpand("albumexp.txt", pl.plAlbumExpand)
		saveGenSel(pl.plGenSel)
		return
	}
	pl.saveJSON()
}

// saveJSON grava o estado da janela no seu json.
func (pl *plw) saveJSON() {
	if pl.jsonPath == "" {
		return
	}
	st := plState{
		Open:         pl.plOpen,
		Name:         pl.name,
		Width:        int(pl.plWidth),
		Height:       int(pl.plHeight),
		Scale:        pl.plScale,
		Shade:        pl.shadePL.on,
		Tracks:       pl.tracks,
		Dirs:         pl.dirs,
		GenreOpen:    pl.plGenreOpen,
		GenreSel:     pl.plGenreSel,
		GenreOn:      pl.plGenreOn,
		GenreExpand:  pl.plGenreExpand,
		ArtistOpen:   pl.plArtistOpen,
		ArtistExpand: pl.plArtistExpand,
		FolderOpen:   pl.plFolderOpen,
		FolderExpand: pl.plFolderExpand,
		AlbumOpen:    pl.plAlbumOpen,
		AlbumExpand:  pl.plAlbumExpand,
		GenSel:       pl.plGenSel,
		Radios:       pl.showRadios,
	}
	b, err := json.MarshalIndent(&st, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(pl.jsonPath, b, 0o644)
}

// loadJSON lê o estado da janela do seu json.
func (pl *plw) loadJSON() {
	b, err := os.ReadFile(pl.jsonPath)
	if err != nil {
		return
	}
	var st plState
	if json.Unmarshal(b, &st) != nil {
		return
	}
	if st.Width > 0 && st.Height > 0 {
		pl.plWidth, pl.plHeight = unit.Dp(st.Width), unit.Dp(st.Height)
	}
	if st.Scale > 0 {
		pl.plScale = st.Scale
	}
	pl.shadePL.on = st.Shade
	pl.tracks = st.Tracks
	pl.dirs = st.Dirs
	pl.plGenreOpen = st.GenreOpen
	pl.plGenreSel = st.GenreSel
	pl.plGenreOn = st.GenreOn
	pl.plGenreExpand = st.GenreExpand
	pl.plArtistOpen = st.ArtistOpen
	pl.plArtistExpand = st.ArtistExpand
	pl.plFolderOpen = st.FolderOpen
	pl.plFolderExpand = st.FolderExpand
	pl.plAlbumOpen = st.AlbumOpen
	pl.plAlbumExpand = st.AlbumExpand
	pl.plGenSel = st.GenSel
	pl.showRadios = st.Radios && pl.key == "rd"
	pl.plOpen = st.Open
}

// ---- ações da janela ----
//
// Toda ação parte da lista desta janela: as de transporte carregam a lista no
// player (activate) e as de edição aplicam direto na lista, refletindo no
// player apenas quando esta janela é a ativa.

// window devolve a janela com segurança (pode ser anulada pelo DestroyEvent).
func (pl *plw) window() *app.Window {
	pl.ui.mu.Lock()
	defer pl.ui.mu.Unlock()
	return pl.win
}

// close fecha a janela (botão X dela).
func (pl *plw) close() {
	pl.ui.mu.Lock()
	defer pl.ui.mu.Unlock()
	pl.closeLocked()
}

// closeLocked pressupõe pl.ui.mu segurado.
func (pl *plw) closeLocked() {
	if pl.win == nil {
		return
	}
	pl.plOpen = false
	saveOpenWindows(pl.ui.eqOpen, pl.ui.plMain.plOpen, pl.ui.menuOpen,
		pl.ui.plRadio.plOpen, pl.ui.plVideo.plOpen)
	pl.win.Perform(system.ActionClose)
}

func (pl *plw) play() {
	pl.activate()
	pl.ui.p.Play()
}

func (pl *plw) pause() {
	pl.activate()
	pl.ui.p.Pause()
}

func (pl *plw) prev() {
	pl.activate()
	pl.ui.p.Prev()
}

func (pl *plw) next() {
	pl.activate()
	pl.ui.p.Next()
}

func (pl *plw) seek(f float64) error {
	pl.activate()
	return pl.ui.p.SeekFraction(f)
}

func (pl *plw) playIndex(i int) {
	pl.activate()
	_ = pl.ui.p.PlayIndex(i)
	pl.syncFromPlayer()
}

func (pl *plw) playInOrder(i int, order []int) error {
	pl.activate()
	err := pl.ui.p.PlayInOrder(i, order)
	pl.syncFromPlayer()
	return err
}

func (pl *plw) duration() time.Duration {
	if pl.ui.activePL != pl {
		return 0
	}
	return pl.ui.p.Duration()
}

func (pl *plw) position() time.Duration {
	if pl.ui.activePL != pl {
		return 0
	}
	return pl.ui.p.Position()
}

// applyTracks substitui a lista da janela (clear/ordenação).
func (pl *plw) applyTracks(t []string) {
	if pl.ui.activePL == pl {
		pl.ui.p.SetTracks(t)
		pl.tracks = pl.ui.p.Tracks()
	} else {
		pl.tracks = t
	}
	pl.dirty = true
	pl.persist()
}

// removeAt remove a faixa i da lista da janela.
func (pl *plw) removeAt(i int) {
	if pl.ui.activePL == pl {
		pl.ui.p.RemoveAt(i)
		pl.tracks = pl.ui.p.Tracks()
	} else if i >= 0 && i < len(pl.tracks) {
		pl.tracks = append(pl.tracks[:i], pl.tracks[i+1:]...)
	}
	pl.dirty = true
	pl.persist()
}

// addTracks anexa faixas à lista da janela.
func (pl *plw) addTracks(files []string) {
	if len(files) == 0 {
		return
	}
	if pl.ui.activePL == pl {
		pl.ui.p.AddTracks(files)
		pl.tracks = pl.ui.p.Tracks()
	} else {
		// janela inativa: dedup por caminho contra a lista dela
		have := make(map[string]bool, len(pl.tracks)+len(files))
		for _, t := range pl.tracks {
			have[filepath.Clean(t)] = true
		}
		for _, t := range files {
			c := filepath.Clean(t)
			if have[c] {
				continue
			}
			have[c] = true
			pl.tracks = append(pl.tracks, t)
		}
	}
	pl.dirty = true
	pl.persist()
	// preenche tags/capas em background sem tocar nos índices do player
	go pl.ui.p.WarmTracks(pl.tracks)
}

// persist grava a lista da janela no seu arquivo de estado: Radio/Video no
// json próprio; a Playlist principal no playlist.m3u mesmo quando outra
// janela está ativa (edição não pode se perder ao fechar o app).
func (pl *plw) persist() {
	pl.saveJSON()
	if pl.key == "pl" {
		pl.ui.p.SavePlaylistFile(pl.tracks)
		pl.saveDirsFile()
	}
}

// ---- diretórios importados (Update lists) ----

// loadDirList lê um arquivo texto com um diretório por linha.
func loadDirList(file string) []string {
	b, err := os.ReadFile(configPath(file))
	if err != nil {
		return nil
	}
	var out []string
	for _, l := range strings.Split(string(b), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
		}
	}
	return out
}

// saveDirList grava dirs em file (um por linha).
func saveDirList(file string, dirs []string) {
	p := configPath(file)
	_ = os.MkdirAll(filepath.Dir(p), 0o755)
	_ = os.WriteFile(p, []byte(strings.Join(dirs, "\n")+"\n"), 0o644)
}

// addDir registra um diretório importado (dedup por caminho).
func (pl *plw) addDir(dir string) {
	dir = filepath.Clean(dir)
	for _, d := range pl.dirs {
		if filepath.Clean(d) == dir {
			return
		}
	}
	pl.dirs = append(pl.dirs, dir)
	if pl.jsonPath != "" {
		pl.saveJSON()
	} else {
		pl.saveDirsFile()
	}
}

// saveDirsFile persiste os dirs da playlist principal (pldirs.txt).
func (pl *plw) saveDirsFile() {
	saveDirList("pldirs.txt", pl.dirs)
}

// scanRoots retorna os diretórios efetivos de varredura: os registrados
// mais raízes derivadas das faixas atuais. Pais que compartilham o mesmo
// avô (ex.: Music/<gênero>/…) colapsam para o avô — assim pastas novas
// criadas dentro dele também passam a ser encontradas.
func (pl *plw) scanRoots() []string {
	roots := append([]string(nil), pl.dirs...)
	parents := map[string]bool{}
	for _, t := range pl.tracks {
		if d := filepath.Dir(t); d != "." && d != string(filepath.Separator) {
			parents[d] = true
		}
	}
	byGrand := map[string][]string{}
	for d := range parents {
		g := filepath.Dir(d)
		byGrand[g] = append(byGrand[g], d)
	}
	for g, kids := range byGrand {
		if len(kids) >= 2 {
			roots = append(roots, g)
		} else {
			roots = append(roots, kids[0])
		}
	}
	// dedup e remove dirs contidos em outro dir da lista
	seen := map[string]bool{}
	var out []string
	for _, r := range roots {
		r = filepath.Clean(r)
		if !seen[r] {
			seen[r] = true
			out = append(out, r)
		}
	}
	var res []string
	for _, r := range out {
		nested := false
		for _, o := range out {
			if o != r && strings.HasPrefix(r+string(filepath.Separator), o+string(filepath.Separator)) {
				nested = true
				break
			}
		}
		if !nested {
			res = append(res, r)
		}
	}
	return res
}

// updateFromDirs re-varre os diretórios importados desta janela e anexa
// as faixas novas (addTracks já deduplica por caminho — só entra o inédito).
func (pl *plw) updateFromDirs() {
	roots := pl.scanRoots()
	if len(roots) == 0 {
		return
	}
	for _, r := range roots {
		pl.addDir(r)
	}
	pl.pruneMissing(roots)
	var files []string
	for _, d := range roots {
		files = append(files, audioFilesInDir(d)...)
	}
	if len(files) > 0 {
		pl.addTracks(files)
	}
}

// pruneMissing remove da lista as faixas que estão sob uma das raízes
// varridas mas cujo arquivo não existe mais no disco (música apagada
// da pasta). Faixas fora das raízes nunca são podadas.
func (pl *plw) pruneMissing(roots []string) {
	under := func(t string) bool {
		c := filepath.Clean(t)
		for _, r := range roots {
			if strings.HasPrefix(c, r+string(filepath.Separator)) {
				return true
			}
		}
		return false
	}
	gone := func(t string) bool {
		_, err := os.Stat(t)
		return err != nil
	}
	if pl.ui.activePL == pl {
		cur := pl.ui.p.Tracks()
		removed := false
		for i := len(cur) - 1; i >= 0; i-- {
			if under(cur[i]) && gone(cur[i]) {
				pl.ui.p.RemoveAt(i)
				removed = true
			}
		}
		if removed {
			pl.tracks = pl.ui.p.Tracks()
			pl.dirty = true
			pl.persist()
		}
		return
	}
	kept := pl.tracks[:0]
	removed := false
	for _, t := range pl.tracks {
		if under(t) && gone(t) {
			removed = true
			continue
		}
		kept = append(kept, t)
	}
	if removed {
		pl.tracks = kept
		pl.dirty = true
		pl.persist()
	}
}

// syncFromPlayer copia do player a lista recém-editada.
func (pl *plw) syncFromPlayer() {
	if pl.ui.activePL != pl {
		return
	}
	pl.tracks = pl.ui.p.Tracks()
}

// addPicked abre o diálogo de arquivos e anexa à lista desta janela.
func (pl *plw) addPicked(dir bool) {
	go func() {
		ui := pl.ui
		var files []string
		var pickedDir string
		if dir {
			pick := func() []string {
				if d := pickAudioDir(); d != "" {
					pickedDir = d
					return audioFilesInDir(d)
				}
				return nil
			}
			files = ui.dialogWrap(pick)
		} else {
			files = ui.dialogWrap(pickAudioFiles)
		}
		if pickedDir != "" {
			pl.addDir(pickedDir)
		}
		pl.addTracks(files)
	}()
}

// writeM3U exporta a lista da janela em .m3u ao lado da persistência dela.
func (pl *plw) writeM3U(tracks []string) {
	if len(tracks) == 0 {
		return
	}
	name := "playlist.m3u"
	if pl.jsonPath != "" {
		name = pl.key + ".m3u"
	}
	f, err := os.Create(name)
	if err != nil {
		return
	}
	defer f.Close()
	f.WriteString("#EXTM3U\n")
	for _, t := range tracks {
		f.WriteString(t + "\n")
	}
}
