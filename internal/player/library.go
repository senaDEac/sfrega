package player

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// TrackMeta são os metadados persistidos de uma faixa.
type TrackMeta struct {
	Plays    int      `json:"plays"`
	Rating   int      `json:"rating"` // 0..5
	Hashtags []string `json:"hashtags"`
	Shuffle  int      `json:"shuffle"` // 0/1: já tocada >=15s no ciclo de shuffle atual
}

// trackLibrary persiste metadados por caminho de faixa em JSON.
type trackLibrary struct {
	mu    sync.Mutex
	path  string
	meta  map[string]*TrackMeta
	dirty bool
}

type libraryFile struct {
	Version int                   `json:"version"`
	Tracks  map[string]*TrackMeta `json:"tracks"`
}

// newTrackLibrary carrega (ou cria) o arquivo de biblioteca.
func newTrackLibrary(path string) *trackLibrary {
	l := &trackLibrary{path: path, meta: map[string]*TrackMeta{}}
	if b, err := os.ReadFile(path); err == nil {
		var f libraryFile
		if json.Unmarshal(b, &f) == nil && f.Tracks != nil {
			l.meta = f.Tracks
			// shuffle é binário (0/1): normaliza valores antigos >1
			for _, m := range l.meta {
				if m.Shuffle > 1 {
					m.Shuffle = 1
				}
			}
		}
	}
	return l
}

// ensure retorna o meta da faixa, criando com hashtags aleatórias se novo.
func (l *trackLibrary) ensure(path string) *TrackMeta {
	l.mu.Lock()
	defer l.mu.Unlock()
	m, ok := l.meta[path]
	if !ok {
		m = &TrackMeta{}
		l.meta[path] = m
		l.dirty = true
	}
	return m
}

// get retorna cópia do meta (nil se não existe).
func (l *trackLibrary) get(path string) *TrackMeta {
	l.mu.Lock()
	defer l.mu.Unlock()
	if m, ok := l.meta[path]; ok {
		cp := *m
		cp.Hashtags = append([]string(nil), m.Hashtags...)
		return &cp
	}
	return nil
}

// setRating atualiza a nota (0..5) da faixa.
func (l *trackLibrary) setRating(path string, r int) {
	if r < 0 {
		r = 0
	}
	if r > 5 {
		r = 5
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	m, ok := l.meta[path]
	if !ok {
		m = &TrackMeta{}
		l.meta[path] = m
	}
	m.Rating = r
	l.dirty = true
}

// addPlay incrementa o contador de reproduções.
func (l *trackLibrary) addPlay(path string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	m, ok := l.meta[path]
	if !ok {
		m = &TrackMeta{}
		l.meta[path] = m
	}
	m.Plays++
	l.dirty = true
}

// shuffleCount retorna a marca de shuffle da faixa (0/1; 0 se sem meta).
func (l *trackLibrary) shuffleCount(path string) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	if m, ok := l.meta[path]; ok {
		return m.Shuffle
	}
	return 0
}

// addShuffle marca a faixa como tocada no ciclo de shuffle atual (0→1).
func (l *trackLibrary) addShuffle(path string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	m, ok := l.meta[path]
	if !ok {
		m = &TrackMeta{}
		l.meta[path] = m
	}
	m.Shuffle = 1
	l.dirty = true
}

// resetShuffle zera o contador de shuffle da faixa.
func (l *trackLibrary) resetShuffle(path string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	m, ok := l.meta[path]
	if !ok {
		return
	}
	m.Shuffle = 0
	l.dirty = true
}

// resetAllShuffle zera o contador de shuffle de todas as faixas.
func (l *trackLibrary) resetAllShuffle() {
	l.mu.Lock()
	for _, m := range l.meta {
		m.Shuffle = 0
	}
	l.dirty = true
	l.mu.Unlock()
	l.flush()
}

// resetAllPlays zera o contador de plays de todas as faixas.
func (l *trackLibrary) resetAllPlays() {
	l.mu.Lock()
	for _, m := range l.meta {
		m.Plays = 0
	}
	l.dirty = true
	l.mu.Unlock()
	l.flush()
}

// resetAllTags remove as hashtags de todas as faixas.
func (l *trackLibrary) resetAllTags() {
	l.mu.Lock()
	for _, m := range l.meta {
		m.Hashtags = nil
	}
	l.dirty = true
	l.mu.Unlock()
	l.flush()
}

// flush grava no disco se houver mudanças. Seguro chamar sempre.
func (l *trackLibrary) flush() {
	l.mu.Lock()
	if !l.dirty {
		l.mu.Unlock()
		return
	}
	l.dirty = false
	f := libraryFile{Version: 1, Tracks: l.meta}
	l.mu.Unlock()

	b, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return
	}
	_ = os.MkdirAll(filepath.Dir(l.path), 0o755)
	tmp := l.path + ".tmp"
	if os.WriteFile(tmp, b, 0o644) == nil {
		_ = os.Rename(tmp, l.path)
	}
}

