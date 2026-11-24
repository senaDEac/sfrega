// Package skins carrega skins do Winamp (.wsz extraído em PNGs ou diretório
// solto) e fornece acesso aos bitmaps e à paleta do analisador (VISCOLOR).
package skins

import (
	"bytes"
	"embed"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"

	"archive/zip"

	"golang.org/x/image/bmp"
)

//go:embed 00/*.png 00/VISCOLOR.TXT
var baseFS embed.FS

// Skin contém os bitmaps de uma skin do Winamp.
type Skin struct {
	imgs map[string]*image.RGBA
	vis  []color.NRGBA
	mu   sync.Mutex
	spr  map[string]*image.RGBA // cache de recortes
}

// OpenEmbedded carrega uma skin embutida no binário (ex.: "base-2.91").
func OpenEmbedded(name string) (*Skin, error) {
	return loadFS(baseFS, path.Join("00", name))
}

// OpenDir carrega uma skin de um diretório contendo PNGs + VISCOLOR.TXT.
func OpenDir(dir string) (*Skin, error) {
	return loadFS(os.DirFS(dir), ".")
}

// skinDirs retorna os diretórios de skins em runtime (sem rebuild):
// ./skins, ./internal/skins, $XDG_DATA_HOME/sfrega/skins e
// ~/.local/share/sfrega/skins.
func skinDirs() []string {
	home, _ := os.UserHomeDir()
	var dirs []string
	if x := os.Getenv("XDG_DATA_HOME"); x != "" {
		dirs = append(dirs, filepath.Join(x, "sfrega", "skins"))
	}
	dirs = append(dirs,
		filepath.Join(home, ".local", "share", "sfrega", "skins"),
		"skins",
		"internal",
		"internal/skins",
	)
	return dirs
}

// WszNames retorna os nomes (sem .wsz) das skins .wsz disponíveis
// (diretórios de runtime + embed), deduplicados e ordenados.
func WszNames() []string {
	seen := map[string]bool{}
	for _, d := range skinDirs() {
		ents, err := os.ReadDir(d)
		if err != nil {
			continue
		}
		for _, e := range ents {
			if strings.HasSuffix(e.Name(), ".wsz") {
				seen[strings.TrimSuffix(e.Name(), ".wsz")] = true
			}
		}
	}
	out := make([]string, 0, len(seen))
	for n := range seen {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// OpenWsz carrega uma skin de um arquivo .wsz nos diretórios de runtime.
// Aceita nome com ou sem extensão.
func OpenWsz(name string) (*Skin, error) {
	file := name
	if !strings.HasSuffix(strings.ToLower(file), ".wsz") {
		file += ".wsz"
	}
	for _, d := range skinDirs() {
		if data, err := os.ReadFile(filepath.Join(d, file)); err == nil {
			return openWszData(file, data)
		}
	}
	return nil, fmt.Errorf("wsz não encontrado: %s", file)
}

func openWszData(name string, data []byte) (*Skin, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, err
	}
	s := &Skin{imgs: map[string]*image.RGBA{}, spr: map[string]*image.RGBA{}}
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			continue
		}
		buf, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			continue
		}
		s.loadFile(f.Name, buf)
	}
	if len(s.imgs) == 0 {
		return nil, fmt.Errorf("skin sem PNGs em %s", name)
	}
	return s, nil
}

// Default retorna a skin padrão: SFREGA_SKIN (arquivo .wsz/.png dir) >
// ./internal/skins > ~/.local/share/sfrega/skins > base-2.91 embutida.
func Default() (*Skin, error) {
	if p := os.Getenv("SFREGA_SKIN"); p != "" {
		if s, err := OpenDir(p); err == nil {
			return s, nil
		}
	}
	for _, d := range []string{
		"internal/skins",
		filepath.Join(os.Getenv("HOME"), ".local/share/sfrega/skins"),
		filepath.Join(os.Getenv("XDG_DATA_HOME"), "sfrega/skins"),
	} {
		if d == "" || d == "sfrega/skins" {
			continue
		}
		ents, err := os.ReadDir(d)
		if err != nil {
			continue
		}
		for _, e := range ents {
			// só aceita subpastas que sejam skins de verdade (têm VISCOLOR.TXT);
			// pastas 0-9/a-z/_ do acervo contêm .wsz + thumbs, não skins.
			if e.IsDir() {
				if _, err := os.Stat(filepath.Join(d, e.Name(), "VISCOLOR.TXT")); err != nil {
					continue
				}
				if s, err := OpenDir(filepath.Join(d, e.Name())); err == nil {
					return s, nil
				}
			}
		}
	}
	return OpenEmbedded(".")
}

func loadFS(fsys fs.FS, dir string) (*Skin, error) {
	s := &Skin{imgs: map[string]*image.RGBA{}, spr: map[string]*image.RGBA{}}
	err := fs.WalkDir(fsys, dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := fs.ReadFile(fsys, p)
		if err != nil {
			return err
		}
		s.loadFile(p, data)
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(s.imgs) == 0 {
		return nil, fmt.Errorf("skin sem PNGs em %s", dir)
	}
	return s, nil
}

// loadFile ingere um único arquivo (PNG ou VISCOLOR.TXT) pelo nome.
func (s *Skin) loadFile(fileName string, data []byte) {
	name := strings.ToUpper(strings.TrimSuffix(filepath.Base(fileName), filepath.Ext(fileName)))
	switch strings.ToUpper(filepath.Ext(fileName)) {
	case ".PNG":
		im, err := png.Decode(bytes.NewReader(data))
		if err != nil {
			return // ignora PNG inválido
		}
		s.imgs[name] = toRGBA(im)
	case ".BMP":
		im, err := bmp.Decode(bytes.NewReader(data))
		if err != nil {
			return
		}
		s.imgs[name] = toRGBA(im)
	case ".TXT":
		if strings.EqualFold(filepath.Base(fileName), "VISCOLOR.TXT") {
			s.vis = parseVisColors(string(data))
		}
	}
}

// Get retorna o bitmap completo pelo nome ("MAIN", "VOLUME", ...).
func (s *Skin) Get(name string) *image.RGBA {
	return s.imgs[strings.ToUpper(name)]
}

// Spr retorna um recorte cacheado de um bitmap.
func (s *Skin) Spr(name string, r image.Rectangle) *image.RGBA {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := fmt.Sprintf("%s@%d,%d,%d,%d", name, r.Min.X, r.Min.Y, r.Max.X, r.Max.Y)
	if c, ok := s.spr[key]; ok {
		return c
	}
	base := s.Get(name)
	if base == nil {
		return nil
	}
	b := base.Bounds()
	r = r.Intersect(b)
	if r.Empty() {
		return nil
	}
	dst := image.NewRGBA(image.Rect(0, 0, r.Dx(), r.Dy()))
	draw.Draw(dst, dst.Bounds(), base, r.Min, draw.Src)
	s.spr[key] = dst
	return dst
}

// VisColor retorna a cor do gradiente do analisador (0=topo ... n-1=base).
func (s *Skin) VisColor(i int) color.NRGBA {
	if len(s.vis) == 0 {
		return color.NRGBA{R: 0x33, G: 0xFF, B: 0x66, A: 0xFF}
	}
	if i < 0 {
		i = 0
	}
	if i >= len(s.vis) {
		i = len(s.vis) - 1
	}
	return s.vis[i]
}

func toRGBA(im image.Image) *image.RGBA {
	b := im.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			dst.Set(x, y, im.At(b.Min.X+x, b.Min.Y+y))
		}
	}
	return dst
}

func parseVisColors(txt string) []color.NRGBA {
	var out []color.NRGBA
	for _, line := range strings.Split(txt, "\n") {
		line = strings.TrimSpace(strings.SplitN(line, "//", 2)[0])
		if line == "" {
			continue
		}
		parts := strings.Split(strings.TrimSuffix(line, ","), ",")
		if len(parts) < 3 {
			continue
		}
		var c color.NRGBA
		c.A = 0xFF
		ok := true
		for i, p := range parts[:3] {
			v, err := strconv.Atoi(strings.TrimSpace(p))
			if err != nil {
				ok = false
				break
			}
			switch i {
			case 0:
				c.R = uint8(v)
			case 1:
				c.G = uint8(v)
			case 2:
				c.B = uint8(v)
			}
		}
		if ok {
			out = append(out, c)
		}
	}
	return out
}
