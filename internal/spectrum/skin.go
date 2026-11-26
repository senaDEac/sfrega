package spectrum

import (
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// Variantes do analisador de espectro: cada subpasta de
// ~/.local/share/sfrega/spectrum/<nome>/ é uma variante com
// config.json opcional + imagens PNG opcionais (ex.: "bg": "fundo.png").
// Pastas podem ser baixadas/compartilhadas como estão.

// SpecDir retorna o diretório raiz das variantes.
func SpecDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "/tmp"
	}
	return filepath.Join(home, ".local", "share", "sfrega", "spectrum")
}

// SpecNames retorna os nomes das variantes (subpastas) ordenados.
func SpecNames() []string {
	ents, err := os.ReadDir(SpecDir())
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range ents {
		if e.IsDir() {
			out = append(out, e.Name())
		}
	}
	sort.Strings(out)
	return out
}

// Spec é uma variante carregada do disco.
type Spec struct {
	Name      string
	Dir       string
	Bands     int // nº de barras (0 = todas as do analisador)
	Direction string
	Style     string // "bars" | "line"
	Gain      float64
	Peak      bool
	Fall      float64 // decaimento do pico por frame (0..1)
	Segments  int     // segmentos por barra estilo VU LED (0 = barra sólida)
	BarW      int     // espessura da barra em px, centrada no slot (0 = slot todo)
	Palette   string  // "" | "vu" (verde/amarelo/vermelho por altura)
	BgImg     image.Image
	FG        color.NRGBA
	HasFG     bool
	Grad      []color.NRGBA
}

type specJSON struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Bands       int      `json:"bands"`
	Direction   string   `json:"direction"` // up | down | center
	Style       string   `json:"style"`     // bars | line
	Gain        float64  `json:"gain"`
	Peak        bool     `json:"peak"`
	Fall        float64  `json:"fall"`
	Segments    int      `json:"segments"`
	BarW        int      `json:"width"`
	Palette     string   `json:"palette"`
	Bg          string   `json:"bg"`
	FG          string   `json:"fg"`
	Gradient    []string `json:"gradient"`
}

// LoadSpec carrega a variante name; nil se não existir.
func LoadSpec(name string) *Spec {
	if name == "" || name == "." || name == ".." || filepath.Base(name) != name {
		return nil
	}
	dir := filepath.Join(SpecDir(), name)
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		return nil
	}
	s := &Spec{Name: name, Dir: dir, Direction: "up", Style: "bars",
		Gain: 1, Fall: 0.9}
	var j specJSON
	if b, err := os.ReadFile(filepath.Join(dir, "config.json")); err == nil {
		_ = json.Unmarshal(b, &j)
	}
	if j.Name != "" {
		s.Name = j.Name
	}
	if j.Bands > 0 {
		s.Bands = j.Bands
	}
	if s.Bands > Bands {
		s.Bands = Bands
	}
	if j.Direction != "" {
		s.Direction = j.Direction
	}
	if j.Style != "" {
		s.Style = j.Style
	}
	if j.Gain > 0 {
		s.Gain = j.Gain
	}
	s.Peak = j.Peak
	if j.Fall > 0 && j.Fall < 1 {
		s.Fall = j.Fall
	}
	if j.Segments > 0 {
		s.Segments = j.Segments
	}
	if j.BarW > 0 {
		s.BarW = j.BarW
	}
	if j.Palette != "" {
		s.Palette = j.Palette
	}
	if c, ok := parseHex(j.FG); ok {
		s.FG, s.HasFG = c, true
	}
	for _, g := range j.Gradient {
		if c, ok := parseHex(g); ok {
			s.Grad = append(s.Grad, c)
		}
	}
	if j.Bg != "" {
		p := filepath.Join(dir, filepath.Base(j.Bg))
		if f, err := os.Open(p); err == nil {
			if img, err := png.Decode(f); err == nil {
				s.BgImg = img
			}
			_ = f.Close()
		}
	}
	return s
}

// parseHex aceita "#rrggbb" ou "rrggbb".
func parseHex(s string) (color.NRGBA, bool) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "#")
	if len(s) != 6 {
		return color.NRGBA{}, false
	}
	v, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		return color.NRGBA{}, false
	}
	return color.NRGBA{R: uint8(v >> 16), G: uint8(v >> 8), B: uint8(v), A: 0xFF}, true
}
