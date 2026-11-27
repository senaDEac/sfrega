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

	"gioui.org/io/pointer"
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
	Name   string `json:"name"`
	URL    string `json:"url"`
	Type   string `json:"type"`
	Icon   string `json:"icon"`
	Vol    string `json:"vol,omitempty"`    // volume original do site
	Status string `json:"status,omitempty"` // onLine/offLine (informativo)
	Desc   string `json:"desc,omitempty"`   // descrição/tagline do site
	Estado string `json:"estado,omitempty"`
	Cidade string `json:"cidade,omitempty"`
	Guide  string `json:"guide,omitempty"` // GuideId TuneIn (se não houver url)
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

// ---- lista de estações na janela Radio ----

const radioRowH = 48 // altura de cada linha (ícone 40px + nome + info)

var (
	colRadioBg   = color.NRGBA{A: 0x00}                            // linhas transparentes
	colRadioLine = color.NRGBA{R: 0x80, G: 0x00, B: 0x20, A: 0xFF} // bordeaux ( playing )
	colRadioTxt  = color.NRGBA{R: 0xFF, G: 0xE8, B: 0x00, A: 0xFF} // amarelo
	colRadioDim  = color.NRGBA{R: 0xC0, G: 0xB0, B: 0x40, A: 0xFF} // amarelo apagado
	colRadioPh   = color.NRGBA{R: 0x30, G: 0x30, B: 0x38, A: 0xFF}
	colRadioOk   = color.NRGBA{R: 0x50, G: 0xB0, B: 0x50, A: 0xFF}
	colRadioOff  = color.NRGBA{R: 0xB0, G: 0x50, B: 0x50, A: 0xFF}
)

// radioInfo monta a linha de informações da estação (tudo que veio do site).
func radioInfo(st radioStation) string {
	var parts []string
	if st.Type != "" {
		parts = append(parts, st.Type)
	}
	if st.Status != "" {
		parts = append(parts, st.Status)
	}
	if st.Desc != "" {
		parts = append(parts, st.Desc)
	}
	loc := st.Cidade
	if st.Estado != "" {
		if loc != "" {
			loc += "/" + st.Estado
		} else {
			loc = st.Estado
		}
	}
	if loc != "" {
		parts = append(parts, loc)
	}
	if st.URL != "" {
		parts = append(parts, st.URL)
	}
	return strings.Join(parts, "  ·  ")
}

// plRadioGrid desenha a lista de estações no lugar da lista de faixas
// (janela Radio, botão RADIOS ativo): uma rádio por linha com ícone e
// todas as informações do catálogo. Duplo-clique toca o stream.
func (pl *plw) plRadioGrid(gtx layout.Context, listR image.Rectangle) {
	stations := allRadioStations() // catálogo + estações das playlists
	rows := len(stations)
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
		idx := row
		st := stations[idx]
		ry := listR.Min.Y + row*radioRowH - int(pl.radioScroll)
		line := image.Rect(listR.Min.X, ry, listR.Max.X, ry+radioRowH-1)
		if line.Max.Y < listR.Min.Y || line.Min.Y > listR.Max.Y {
			continue
		}
		playing := streamTitle != "" && streamTitle == st.Name

		fillRect(gtx.Ops, line, colRadioBg)
		if playing {
			fillRect(gtx.Ops, image.Rect(line.Min.X, line.Min.Y, line.Max.X, line.Min.Y+1), colRadioLine)
			fillRect(gtx.Ops, image.Rect(line.Min.X, line.Max.Y-1, line.Max.X, line.Max.Y), colRadioLine)
		}

		// ícone 40x40 à esquerda
		ir := image.Rect(line.Min.X+4, ry+4, line.Min.X+44, ry+44)
		if img := radioIcon(st); img != nil {
			blit(gtx, img, ir)
		} else {
			fillRect(gtx.Ops, ir, colRadioPh)
			if st.Name != "" {
				pl.ui.textAt(gtx, pl.ui.fontBold, unit.Sp(16), strings.ToUpper(st.Name[:1]),
					colRadioTxt, image.Pt(ir.Min.X+15, ir.Min.Y+12))
			}
		}

		// nome na 1ª linha, infos na 2ª — clipados à direita do ícone
		// (coordenadas absolutas: clip.Rect não translada a origem)
		nc := clip.Rect(image.Rect(line.Min.X+50, ry+2, line.Max.X-16, ry+radioRowH-2)).Push(gtx.Ops)
		pl.ui.textAt(gtx, pl.ui.fontUI, unit.Sp(10), st.Name, colRadioTxt,
			image.Pt(line.Min.X+50, ry+8))
		if info := radioInfo(st); info != "" {
			pl.ui.textAt(gtx, pl.ui.fontUI, unit.Sp(8), info, colRadioDim,
				image.Pt(line.Min.X+50, ry+28))
		}
		nc.Pop()

		// status colorido no canto direito
		if st.Status == "onLine" {
			pl.ui.textAt(gtx, pl.ui.fontUI, unit.Sp(9), "●", colRadioOk, image.Pt(line.Max.X-14, ry+4))
		} else if st.Status == "offLine" {
			pl.ui.textAt(gtx, pl.ui.fontUI, unit.Sp(9), "○", colRadioOff, image.Pt(line.Max.X-14, ry+4))
		}

		// botão invisível: direito abre o balão, duplo-clique toca
		b := &pl.radioBtns[idx]
		b.scrollFn = func(dy float32) { pl.radioScrollAcc += dy }
		if pl.ui.button(gtx, b, line, func(gtx layout.Context, down bool) {
			if down {
				fillRect(gtx.Ops, image.Rect(0, 0, line.Dx(), line.Dy()),
					color.NRGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0x14})
			}
		}) {
			if b.lastBtns.Contain(pointer.ButtonSecondary) {
				pl.openRadioPop(line.Min.X+10, line.Min.Y+10, radioPickOfStation(st))
			} else {
				now := time.Now()
				dbl := now.Sub(b.lastClick) < 500*time.Millisecond
				b.lastClick = now
				if dbl {
					b.lastClick = time.Time{}
					pl.playRadioPick(radioPickOfStation(st))
				}
			}
		}
	}
}
