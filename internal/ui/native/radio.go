package native

import (
	"bytes"
	"crypto/sha1"
	"embed"
	"encoding/hex"
	"encoding/json"
	"image"
	"image/color"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/unit"
)

// Catálogo de rádios (extraído do projeto VintageRadio). O json embutido é
// a fonte padrão; se existir ~/.local/share/sfrega/configs/radios.json o
// usuário pode editar/adicionar estações sem recompilar.
//
//go:embed radios.json
var radiosJSON []byte

//go:embed radioicons/*.png
var radioIconFS embed.FS

// radioStation é uma estação de rádio do catálogo.
type radioStation struct {
	Name string `json:"name"`
	URL  string `json:"url"`
	Type string `json:"type"`
	Icon string `json:"icon"`
}

var (
	radiosOnce sync.Once
	radiosList []radioStation
)

// loadRadios lê o catálogo (configPath tem precedência sobre o embed).
func loadRadios() []radioStation {
	radiosOnce.Do(func() {
		data := radiosJSON
		p := configPath("radios.json")
		if b, err := os.ReadFile(p); err == nil && len(b) > 0 {
			data = b
		} else {
			// semeia o arquivo do usuário com o catálogo padrão
			_ = os.MkdirAll(filepath.Dir(p), 0o755)
			_ = os.WriteFile(p, radiosJSON, 0o644)
		}
		var cat struct {
			Stations []radioStation `json:"stations"`
		}
		if err := json.Unmarshal(data, &cat); err == nil {
			radiosList = cat.Stations
		}
	})
	return radiosList
}

// ---- ícones ----

var (
	radioIconMu    sync.Mutex
	radioIconCache = map[string]image.Image{} // chave do ícone → imagem pronta
	radioIconPend  = map[string]bool{}        // download em andamento
	radioIconBad   = map[string]bool{}        // falhou/indisponível
)

// radioIconDir é o diretório de cache dos ícones remotos.
func radioIconDir() string {
	return filepath.Join(filepath.Dir(configPath("x")), "radioicons")
}

// radioIconFile devolve o nome de cache em disco para uma URL de ícone.
func radioIconFile(icon string) string {
	sum := sha1.Sum([]byte(icon))
	ext := ".png"
	lower := strings.ToLower(icon)
	if strings.Contains(lower, ".jpg") || strings.Contains(lower, ".jpeg") {
		ext = ".jpg"
	}
	return hex.EncodeToString(sum[:8]) + ext
}

// radioIcon retorna a imagem do ícone da estação (nil = ainda não carregou
// ou indisponível). Ícones remotos são baixados em background; a UI
// redesenha sozinha no próximo tick (~8fps) quando a imagem chegar.
func radioIcon(st radioStation) image.Image {
	icon := st.Icon
	if icon == "" {
		return nil
	}
	radioIconMu.Lock()
	if img, ok := radioIconCache[icon]; ok {
		radioIconMu.Unlock()
		return img
	}
	if radioIconBad[icon] || radioIconPend[icon] {
		radioIconMu.Unlock()
		return nil
	}
	radioIconPend[icon] = true
	radioIconMu.Unlock()

	go func() {
		var img image.Image
		if strings.HasPrefix(icon, "http://") || strings.HasPrefix(icon, "https://") {
			img = fetchRadioIcon(icon)
		} else {
			img = loadLocalRadioIcon(icon)
		}
		radioIconMu.Lock()
		defer radioIconMu.Unlock()
		delete(radioIconPend, icon)
		if img != nil {
			radioIconCache[icon] = img
		} else {
			radioIconBad[icon] = true
		}
	}()
	return nil
}

// loadLocalRadioIcon busca o ícone nos assets embutidos (basename do path
// original, ex.: assets/images/logos/kissfm.png → radioicons/kissfm.png).
func loadLocalRadioIcon(icon string) image.Image {
	f, err := radioIconFS.Open("radioicons/" + path.Base(icon))
	if err != nil {
		return nil
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		return nil
	}
	return img
}

// fetchRadioIcon baixa o ícone do site uma única vez e cacheia em disco
// (~/.local/share/sfrega/radioicons/<hash>.<ext>).
func fetchRadioIcon(icon string) image.Image {
	dir := radioIconDir()
	_ = os.MkdirAll(dir, 0o755)
	fpath := filepath.Join(dir, radioIconFile(icon))

	if f, err := os.Open(fpath); err == nil {
		defer f.Close()
		if img, _, err := image.Decode(f); err == nil {
			return img
		}
	}

	req, err := http.NewRequest("GET", icon, nil)
	if err != nil {
		return nil
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 sfrega")
	cli := &http.Client{Timeout: 15 * time.Second}
	resp, err := cli.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil || len(data) == 0 {
		return nil
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil // webp etc. sem decoder registrado
	}
	_ = os.WriteFile(fpath, data, 0o644)
	return img
}

// ---- grade de estações na janela Radio ----

const (
	radioCellW = 62 // largura de cada célula da grade
	radioIconS = 40 // lado do ícone
	radioRowH  = 58 // altura da linha (ícone + nome)
)

var (
	colRadioBg   = color.NRGBA{R: 0x14, G: 0x14, B: 0x1C, A: 0xFF}
	colRadioLine = color.NRGBA{R: 0x80, G: 0x00, B: 0x20, A: 0xFF} // bordeaux ( playing )
	colRadioTxt  = color.NRGBA{R: 0xC8, G: 0xC8, B: 0xC8, A: 0xFF}
	colRadioPh   = color.NRGBA{R: 0x30, G: 0x30, B: 0x38, A: 0xFF}
)

// plRadioGrid desenha a grade de estações no lugar da lista de faixas
// (janela Radio, botão RADIOS ativo). Duplo-clique toca o stream.
func (pl *plw) plRadioGrid(gtx layout.Context, listR image.Rectangle) {
	stations := loadRadios()
	cols := listR.Dx() / radioCellW
	if cols < 1 {
		cols = 1
	}
	rows := (len(stations) + cols - 1) / cols
	contentH := rows * radioRowH
	maxScroll := contentH - listR.Dy()
	if maxScroll < 0 {
		maxScroll = 0
	}

	// aplica scroll da roda
	if pl.radioScrollAcc != 0 {
		pl.radioScroll -= pl.radioScrollAcc
		pl.radioScrollAcc = 0
	}
	if pl.radioScroll > float32(maxScroll) {
		pl.radioScroll = float32(maxScroll)
	}
	if pl.radioScroll < 0 {
		pl.radioScroll = 0
	}

	cs := clip.Rect(listR).Push(gtx.Ops)
	defer cs.Pop()

	streamTitle := pl.ui.p.StreamTitle()
	for len(pl.radioBtns) < len(stations) {
		pl.radioBtns = append(pl.radioBtns, uibtn{})
	}

	visTop := int(pl.radioScroll) / radioRowH
	visBot := (int(pl.radioScroll) + listR.Dy() + radioRowH - 1) / radioRowH
	for row := visTop; row <= visBot && row < rows; row++ {
		if row < 0 {
			continue
		}
		for col := 0; col < cols; col++ {
			idx := row*cols + col
			if idx >= len(stations) {
				continue
			}
			st := stations[idx]
			cx := listR.Min.X + col*radioCellW
			cy := listR.Min.Y + row*radioRowH - int(pl.radioScroll)
			cell := image.Rect(cx, cy, cx+radioCellW-4, cy+radioRowH-2)
			if cell.Max.Y < listR.Min.Y || cell.Min.Y > listR.Max.Y {
				continue
			}
			playing := streamTitle != "" && streamTitle == st.Name

			// fundo da célula + borda se estiver tocando
			fillRect(gtx.Ops, cell, colRadioBg)
			if playing {
				fillRect(gtx.Ops, image.Rect(cell.Min.X, cell.Min.Y, cell.Max.X, cell.Min.Y+2), colRadioLine)
				fillRect(gtx.Ops, image.Rect(cell.Min.X, cell.Max.Y-2, cell.Max.X, cell.Max.Y), colRadioLine)
				fillRect(gtx.Ops, image.Rect(cell.Min.X, cell.Min.Y, cell.Min.X+2, cell.Max.Y), colRadioLine)
				fillRect(gtx.Ops, image.Rect(cell.Max.X-2, cell.Min.Y, cell.Max.X, cell.Max.Y), colRadioLine)
			}

			// ícone (40x40) centrado na célula
			ix := cx + (radioCellW-4-radioIconS)/2
			iy := cy + 2
			ir := image.Rect(ix, iy, ix+radioIconS, iy+radioIconS)
			if img := radioIcon(st); img != nil {
				blit(gtx, img, ir)
			} else {
				// placeholder: caixa escura com a inicial da estação
				fillRect(gtx.Ops, ir, colRadioPh)
				if st.Name != "" {
					initial := strings.ToUpper(st.Name[:1])
					pl.ui.textAt(gtx, pl.ui.fontBold, unit.Sp(14), initial,
						colRadioTxt, image.Pt(ix+radioIconS/2-4, iy+radioIconS/2-8))
				}
			}

			// nome embaixo do ícone (clipado na célula)
			nameR := image.Rect(cx+2, iy+radioIconS+2, cx+radioCellW-6, cy+radioRowH-2)
			nc := clip.Rect(nameR).Push(gtx.Ops)
			pl.ui.textAt(gtx, pl.ui.fontUI, unit.Sp(8), st.Name, colRadioTxt, image.Pt(0, 0))
			nc.Pop()

			// botão invisível por cima: duplo-clique toca a rádio
			b := &pl.radioBtns[idx]
			b.scrollFn = func(dy float32) { pl.radioScrollAcc += dy }
			if pl.ui.button(gtx, b, cell, func(gtx layout.Context, down bool) {
				if down {
					fillRect(gtx.Ops, image.Rect(0, 0, cell.Dx(), cell.Dy()),
						color.NRGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0x18})
				}
			}) {
				now := time.Now()
				dbl := now.Sub(b.lastClick) < 500*time.Millisecond
				b.lastClick = now
				if dbl {
					b.lastClick = time.Time{}
					_ = pl.ui.p.PlayStream(st.Name, st.URL)
				}
			}
		}
	}
}
