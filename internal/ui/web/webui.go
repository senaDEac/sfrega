// Package web serve a interface visual do sfrega no navegador
// (estilo Winamp) e expõe a API REST do player.
package web

import (
	"embed"
	"encoding/json"
	"net/http"
	"path/filepath"
	"strconv"

	"sfrega/internal/player"
)

//go:embed index.html
var assets embed.FS

type Server struct {
	p *player.Player
}

func New(p *player.Player) *Server { return &Server{p: p} }

type stateJSON struct {
	Title   string    `json:"title"`
	Index   int       `json:"index"`
	Tracks  []string  `json:"tracks"`
	Playing bool      `json:"playing"`
	Volume  float32   `json:"vol"`
	Pos     float64   `json:"pos"`
	Dur     float64   `json:"dur"`
	Bands   []float32 `json:"bands"`
}

func (s *Server) state() stateJSON {
	p := s.p
	bands := p.Spectrum()
	bl := make([]float32, len(bands))
	for i, v := range bands {
		bl[i] = v
	}
	tracks := make([]string, 0)
	// nomes de exibição (basename)
	for i := 0; i < p.Count(); i++ {
		tracks = append(tracks, filepath.Base(p.TrackAt(i)))
	}
	return stateJSON{
		Title:   filepath.Base(p.TrackAt(p.Index())),
		Index:   p.Index(),
		Tracks:  tracks,
		Playing: p.Playing(),
		Volume:  p.Volume(),
		Pos:     p.Position().Seconds(),
		Dur:     p.Duration().Seconds(),
		Bands:   bl,
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		data, _ := assets.ReadFile("index.html")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(data)
	})
	mux.HandleFunc("/api/state", func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewEncoder(w).Encode(s.state()); err != nil {
			http.Error(w, err.Error(), 500)
		}
	})
	mux.HandleFunc("/api/toggle", func(w http.ResponseWriter, r *http.Request) {
		s.p.Toggle()
		w.Write([]byte(`{"ok":true}`))
	})
	mux.HandleFunc("/api/stop", func(w http.ResponseWriter, r *http.Request) {
		s.p.Pause()
		s.p.SeekFrame(0)
		w.Write([]byte(`{"ok":true}`))
	})
	mux.HandleFunc("/api/next", func(w http.ResponseWriter, r *http.Request) {
		s.p.Next()
		s.p.Play()
		w.Write([]byte(`{"ok":true}`))
	})
	mux.HandleFunc("/api/prev", func(w http.ResponseWriter, r *http.Request) {
		s.p.Prev()
		s.p.Play()
		w.Write([]byte(`{"ok":true}`))
	})
	mux.HandleFunc("/api/play", func(w http.ResponseWriter, r *http.Request) {
		if i, err := strconv.Atoi(r.URL.Query().Get("i")); err == nil {
			s.p.PlayIndex(i)
		}
		w.Write([]byte(`{"ok":true}`))
	})
	mux.HandleFunc("/api/volume", func(w http.ResponseWriter, r *http.Request) {
		if v, err := strconv.ParseFloat(r.URL.Query().Get("v"), 32); err == nil {
			s.p.SetVolume(float32(v))
		}
		w.Write([]byte(`{"ok":true}`))
	})
	return mux
}
