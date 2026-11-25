package player

import (
	"crypto/sha1"
	"encoding/hex"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"path/filepath"
	"sync"

	_ "golang.org/x/image/bmp"
	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/webp"
)

const coverSize = 40

var (
	noMusicImg  image.Image
	noMusicOnce sync.Once
)

// noMusicCover retorna o placeholder images/no-music.png (40x40), se existir.
func noMusicCover() image.Image {
	noMusicOnce.Do(func() {
		home, err := os.UserHomeDir()
		if err != nil {
			return
		}
		fp := filepath.Join(home, ".local", "share", "sfrega", "images", "no-music.png")
		f, err := os.Open(fp)
		if err != nil {
			return
		}
		defer f.Close()
		img, _, err := image.Decode(f)
		if err != nil {
			return
		}
		dst := image.NewNRGBA(image.Rect(0, 0, coverSize, coverSize))
		xdraw.CatmullRom.Scale(dst, dst.Bounds(), img, img.Bounds(), xdraw.Over, nil)
		noMusicImg = dst
	})
	return noMusicImg
}

// saveCover grava a imagem embutida em covers/<hash>.<ext> e retorna o
// nome do arquivo ("-" quando não há arte ou o diretório não existe).
func (p *Player) saveCover(path string, art []byte) string {
	if p.artDir == "" || len(art) == 0 {
		return "-"
	}
	ext := coverExt(art)
	if ext == "" {
		return "-"
	}
	// hash do conteúdo: faixas com a mesma arte compartilham o arquivo/thumb
	sum := sha1.Sum(art)
	name := hex.EncodeToString(sum[:8]) + "." + ext
	fp := filepath.Join(p.artDir, name)
	if _, err := os.Stat(fp); err != nil {
		if err := os.WriteFile(fp, art, 0o644); err != nil {
			return "-"
		}
	}
	return name
}

// coverExt detecta a extensão da imagem pelos magic bytes.
func coverExt(b []byte) string {
	switch {
	case len(b) > 3 && b[0] == 0xFF && b[1] == 0xD8:
		return "jpg"
	case len(b) > 8 && b[0] == 0x89 && b[1] == 'P' && b[2] == 'N' && b[3] == 'G':
		return "png"
	case len(b) > 4 && string(b[:4]) == "GIF8":
		return "gif"
	case len(b) > 2 && b[0] == 'B' && b[1] == 'M':
		return "bmp"
	case len(b) > 12 && string(b[:4]) == "RIFF" && string(b[8:12]) == "WEBP":
		return "webp"
	}
	return ""
}

// TrackCover retorna a capa da faixa i redimensionada (nil se não houver).
// Seguro para a thread de UI: só lê cache e arquivos pequenos sob demanda.
func (p *Player) TrackCover(i int) image.Image {
	p.mu.Lock()
	var path string
	if i >= 0 && i < len(p.tracks) {
		path = p.tracks[i]
	}
	p.mu.Unlock()
	return p.coverForPath(path)
}

// coverForPath retorna a capa do arquivo (nil se não houver).
func (p *Player) coverForPath(path string) image.Image {
	if path == "" {
		return noMusicCover()
	}
	t, _ := p.tagPeek(path)
	if t.Cover == "" || t.Cover == "-" || p.artDir == "" {
		return noMusicCover()
	}
	p.coverMu.Lock()
	if p.coverCache == nil {
		p.coverCache = map[string]image.Image{}
	}
	if img, ok := p.coverCache[t.Cover]; ok {
		p.coverMu.Unlock()
		return img
	}
	// decode em background: a capa aparece no próximo frame, sem travar a UI
	if p.coverPend == nil {
		p.coverPend = map[string]bool{}
	}
	if p.coverPend[t.Cover] {
		p.coverMu.Unlock()
		return nil // decode em andamento: sem imagem neste frame
	}
	p.coverPend[t.Cover] = true
	name := t.Cover
	p.coverMu.Unlock()
	go func() {
		img := loadCoverThumb(filepath.Join(p.artDir, name))
		p.coverMu.Lock()
		p.coverCache[name] = img
		if img == nil {
			p.coverCache[name] = noMusicCover()
		}
		delete(p.coverPend, name)
		p.coverMu.Unlock()
	}()
	return nil
}

// loadCoverThumb decodifica a imagem e reduz para coverSize×coverSize.
func loadCoverThumb(fp string) image.Image {
	f, err := os.Open(fp)
	if err != nil {
		return nil
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		if _, serr := f.Seek(0, 0); serr != nil {
			return nil
		}
		if img, err = webp.Decode(f); err != nil {
			return nil
		}
	}
	if img.Bounds().Dx() < 1 || img.Bounds().Dy() < 1 {
		return nil
	}
	dst := image.NewNRGBA(image.Rect(0, 0, coverSize, coverSize))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), img, img.Bounds(), xdraw.Over, nil)
	return dst
}
