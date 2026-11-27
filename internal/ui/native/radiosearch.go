package native

import (
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"net/http"
	"net/url"
	"strings"
	"time"

	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/unit"
	"gioui.org/widget"
)

// Busca de rádios — mesma cadeia do VintageRadio (assets/js/radio.js):
// api.tunein.com/profiles → opml.radiotime.com/Tune.ashx →
// stream.core-prod.us-west-2.tunenet.io/listen.stream.

// radioResult é uma estação encontrada na busca (dados do TuneIn).
type radioResult struct {
	Title    string
	Subtitle string
	Desc     string
	Image    string
	GuideID  string
}

var tuneInHTTP = &http.Client{Timeout: 20 * time.Second}

func getJSON(u string, v any) error {
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 sfrega")
	resp, err := tuneInHTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return errStatus(resp.Status)
	}
	return json.NewDecoder(resp.Body).Decode(v)
}

type errStatus string

func (e errStatus) Error() string { return "http " + string(e) }

// tuneInProfiles espelha a resposta de api.tunein.com/profiles.
type tuneInProfiles struct {
	Items []struct {
		Title    string `json:"Title"`
		Children []struct {
			Title       string `json:"Title"`
			Type        string `json:"Type"`
			Image       string `json:"Image"`
			Subtitle    string `json:"Subtitle"`
			Description string `json:"Description"`
			Properties  struct {
				SEOInfo struct {
					Title string `json:"Title"`
				} `json:"SEOInfo"`
			} `json:"Properties"`
			Actions struct {
				Follow struct {
					GuideID string `json:"GuideId"`
				} `json:"Follow"`
			} `json:"Actions"`
		} `json:"Children"`
	} `json:"Items"`
}

// searchTuneIn busca estações no TuneIn (mesma query do VintageRadio).
func searchTuneIn(query string) ([]radioResult, error) {
	u := "https://api.tunein.com/profiles?fullTextSearch=true&query=" +
		url.QueryEscape(query) +
		"&formats=mp3,aac,ogg,flash,html,hls" +
		"&serial=369bf28b-5bc3-4ee1-bb43-14c29616cfd5" +
		"&partnerId=RadioTime&version=4.3507&itemUrlScheme=secure&reqAttempt=1"
	var resp tuneInProfiles
	if err := getJSON(u, &resp); err != nil {
		return nil, err
	}
	// o título do container varia com o idioma ("Rádios"/"Stations") —
	// coletar estações de todos os grupos
	var out []radioResult
	seen := map[string]bool{}
	for _, it := range resp.Items {
		for _, c := range it.Children {
			if c.Type != "Station" || c.Actions.Follow.GuideID == "" || seen[c.Actions.Follow.GuideID] {
				continue
			}
			seen[c.Actions.Follow.GuideID] = true
			title := c.Properties.SEOInfo.Title
			if title == "" {
				title = c.Title
			}
			out = append(out, radioResult{
				Title:    title,
				Subtitle: c.Subtitle,
				Desc:     c.Description,
				Image:    c.Image,
				GuideID:  c.Actions.Follow.GuideID,
			})
		}
	}
	return out, nil
}

// resolveRadioStream resolve o GuideId num URL de stream reproduzível
// (Tune.ashx → guide_id → listen.stream → Streams[0].Url).
func resolveRadioStream(guideID string) (streamURL string, err error) {
	u := "https://opml.radiotime.com/Tune.ashx?id=" + url.QueryEscape(guideID) +
		"&render=json&formats=mp3,aac,ogg,flash,html,hls&partnerId=RadioTime"
	var tune struct {
		Body []struct {
			GuideID string `json:"guide_id"`
		} `json:"body"`
	}
	if err := getJSON(u, &tune); err != nil {
		return "", err
	}
	if len(tune.Body) == 0 || tune.Body[0].GuideID == "" {
		return "", errStatus("sem guide_id")
	}
	// JS faz guide_id.replace('e','') — remove o 'e' inicial do id
	id := strings.TrimPrefix(tune.Body[0].GuideID, "e")

	u2 := "https://stream.core-prod.us-west-2.tunenet.io/listen.stream?streamId=" +
		url.QueryEscape(id) + "&render=json"
	var ls struct {
		Streams []struct {
			URL string `json:"Url"`
		} `json:"Streams"`
	}
	if err := getJSON(u2, &ls); err != nil {
		return "", err
	}
	if len(ls.Streams) == 0 || ls.Streams[0].URL == "" {
		return "", errStatus("sem stream")
	}
	return ls.Streams[0].URL, nil
}

// ---- UI: painel de busca na janela Radio (botão LIST) ----

const (
	radioSrchRowH  = 42 // linha de resultado (ícone + nome + info)
	radioSrchPageN = 10 // resultados por página (setas ◀ ▶)
)

// startRadioSearch dispara a busca TuneIn em background.
func (pl *plw) startRadioSearch() {
	q := strings.TrimSpace(pl.radioEd.Text())
	if q == "" || pl.radioSearching {
		return
	}
	pl.radioSearching = true
	pl.radioSrchStat = "buscando…"
	pl.radioResults = nil
	pl.radioPage = 0
	pl.radioScroll = 0
	pl.radioScrollAcc = 0
	if pl.win != nil {
		pl.win.Invalidate()
	}
	go func() {
		res, err := searchTuneIn(q)
		pl.radioSearching = false
		if err != nil {
			pl.radioSrchStat = "erro: " + err.Error()
		} else {
			pl.radioResults = res
			pl.radioSrchStat = ""
		}
		if pl.win != nil {
			pl.win.Invalidate()
		}
	}()
}

// playRadioResult resolve o stream da estação e toca (background — a
// resolução são 2 chamadas HTTP).
func (pl *plw) playRadioResult(r radioResult) {
	pl.radioSrchStat = "resolvendo " + r.Title + "…"
	if pl.win != nil {
		pl.win.Invalidate()
	}
	go func() {
		u, err := resolveRadioStream(r.GuideID)
		pl.radioSrchStat = ""
		if err == nil && u != "" {
			_ = pl.ui.p.PlayStream(r.Title, u)
		}
		if pl.win != nil {
			pl.win.Invalidate()
		}
	}()
}

// plRadioSearch desenha o painel de busca no lugar da lista de faixas.
func (pl *plw) plRadioSearch(gtx layout.Context, listR image.Rectangle) {
	ui := pl.ui

	cs := clip.Rect(listR).Push(gtx.Ops)
	defer cs.Pop()

	// campo de busca no topo
	inR := image.Rect(listR.Min.X, listR.Min.Y, listR.Max.X, listR.Min.Y+24)
	fillRect(gtx.Ops, inR, color.NRGBA{R: 0x10, G: 0x10, B: 0x18, A: 0xFF})
	bevel(gtx, inR, colBevelLight, colBevelDark)

	// editor dentro do clip+offset da caixa: Update e Layout registram as
	// áreas de evento (clique/foco) na posição certa
	// clique no campo dá foco ao editor (o clicker interno não recebe
	// os eventos dentro do op.Offset desta lista)
	{
		dc := clip.Rect(inR).Push(gtx.Ops)
		event.Op(gtx.Ops, &pl.radioEdDbg)
		dc.Pop()
		for {
			_, ok := gtx.Event(pointer.Filter{Target: &pl.radioEdDbg, Kinds: pointer.Release})
			if !ok {
				break
			}
			gtx.Execute(key.FocusCmd{Tag: &pl.radioEd})
		}
	}
	ic := clip.Rect(inR).Push(gtx.Ops)
	off := op.Offset(image.Pt(inR.Min.X+6, inR.Min.Y+5)).Push(gtx.Ops)
	gtxE := gtx
	gtxE.Constraints = layout.Constraints{Min: image.Point{}, Max: image.Pt(inR.Dx()-12, 14)}
	textMat := ui.txtMat(gtx, color.NRGBA{R: 0xEE, G: 0xEE, B: 0xEE, A: 0xFF})
	selMat := ui.txtMat(gtx, color.NRGBA{R: 0x40, G: 0x80, B: 0xFF, A: 0xFF})
	ui.shapeMu.Lock()
	for {
		ev, ok := pl.radioEd.Update(gtxE)
		if !ok {
			break
		}
		if _, isSubmit := ev.(widget.SubmitEvent); isSubmit {
			pl.startRadioSearch()
		}
	}
	pl.radioEd.Layout(gtxE, ui.shaper, ui.fontUI, unit.Sp(10), textMat, selMat)
	ui.shapeMu.Unlock()
	off.Pop()
	ic.Pop()
	if pl.radioEd.Len() == 0 {
		ui.textAt(gtx, ui.fontUI, unit.Sp(9), "digite o nome da rádio e pressione Enter",
			color.NRGBA{R: 0x60, G: 0x60, B: 0x68, A: 0xFF}, image.Pt(inR.Min.X+7, inR.Min.Y+7))
	}

	// status / resultados
	resTop := inR.Max.Y + 4
	navR := image.Rect(listR.Max.X-92, resTop, listR.Max.X-6, resTop+18)
	pageTop := resTop + 22
	areaH := listR.Max.Y - pageTop
	if pl.radioSearching {
		ui.textAt(gtx, ui.fontUI, unit.Sp(9), "buscando estações…",
			colRadioDim, image.Pt(listR.Min.X+4, pageTop+2))
	}
	if pl.radioSrchStat != "" {
		ui.textAt(gtx, ui.fontUI, unit.Sp(9), pl.radioSrchStat,
			colRadioDim, image.Pt(listR.Min.X+4, pageTop+2))
	}

	all := pl.radioResults
	pages := (len(all) + radioSrchPageN - 1) / radioSrchPageN
	if pages < 1 {
		pages = 1
	}
	if pl.radioPage >= pages {
		pl.radioPage = pages - 1
	}
	if pl.radioPage < 0 {
		pl.radioPage = 0
	}
	pStart := pl.radioPage * radioSrchPageN
	pEnd := pStart + radioSrchPageN
	if pEnd > len(all) {
		pEnd = len(all)
	}
	results := all[pStart:pEnd]

	// navegação ◀ ▶ + "página/total"
	if len(all) > 0 {
		pgBtn := func(idx int, r image.Rectangle, label string, enabled bool, fn func()) {
			fillRect(gtx.Ops, r, color.NRGBA{R: 0x30, G: 0x30, B: 0x3A, A: 0xFF})
			fg := colRadioDim
			if enabled {
				fg = colRadioTxt
			}
			ui.textAt(gtx, ui.fontUI, unit.Sp(10), label, fg,
				image.Pt(r.Min.X+r.Dx()/2-4, r.Min.Y+4))
			if !enabled || pl.radioPop != nil {
				return
			}
			if ui.button(gtx, &pl.radioPgBtns[idx], r, func(gtx layout.Context, down bool) {
				if down {
					fillRect(gtx.Ops, image.Rect(0, 0, r.Dx(), r.Dy()),
						color.NRGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0x14})
				}
			}) {
				fn()
			}
		}
		prevR := image.Rect(navR.Min.X, navR.Min.Y, navR.Min.X+22, navR.Max.Y)
		nextR := image.Rect(navR.Max.X-22, navR.Min.Y, navR.Max.X, navR.Max.Y)
		pgBtn(0, prevR, "◀", pl.radioPage > 0, func() {
			pl.radioPage--
			pl.radioScroll = 0
		})
		pgBtn(1, nextR, "▶", pl.radioPage < pages-1, func() {
			pl.radioPage++
			pl.radioScroll = 0
		})
		ui.textAt(gtx, ui.fontUI, unit.Sp(9),
			fmt.Sprintf("%d/%d", pl.radioPage+1, pages), colRadioDim,
			image.Pt(prevR.Max.X+4, navR.Min.Y+4))
	}

	contentH := len(results) * radioSrchRowH
	maxScroll := contentH - areaH
	if maxScroll < 0 {
		maxScroll = 0
	}
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
	if len(all) == 0 && !pl.radioSearching && pl.radioSrchStat == "" {
		ui.textAt(gtx, ui.fontUI, unit.Sp(9), "nenhum resultado — digite e pressione Enter para buscar",
			colRadioDim, image.Pt(listR.Min.X+4, pageTop+2))
	}

	visTop := int(pl.radioScroll) / radioSrchRowH
	visBot := (int(pl.radioScroll) + areaH + radioSrchRowH - 1) / radioSrchRowH
	for row := visTop; row <= visBot && row < len(results); row++ {
		if row < 0 {
			continue
		}
		r := results[row]
		ry := pageTop + row*radioSrchRowH - int(pl.radioScroll)
		line := image.Rect(listR.Min.X, ry, listR.Max.X, ry+radioSrchRowH-1)
		if line.Max.Y < pageTop || line.Min.Y > listR.Max.Y {
			continue
		}

		// ícone (reusa o cache de ícones remotos)
		st := radioStation{Icon: r.Image, Name: r.Title}
		ir := image.Rect(line.Min.X+4, ry+4, line.Min.X+36, ry+36)
		if img := radioIcon(st); img != nil {
			blit(gtx, img, ir)
		} else {
			fillRect(gtx.Ops, ir, colRadioPh)
		}

		// nome + info clipados à direita do ícone
		nc := clip.Rect(image.Rect(line.Min.X+42, ry+2, line.Max.X-10, ry+radioSrchRowH-2)).Push(gtx.Ops)
		ui.textAt(gtx, ui.fontUI, unit.Sp(10), r.Title, colRadioTxt,
			image.Pt(line.Min.X+42, ry+6))
		info := r.Subtitle
		if r.Desc != "" {
			if info != "" {
				info += "  ·  " + r.Desc
			} else {
				info = r.Desc
			}
		}
		if info != "" {
			ui.textAt(gtx, ui.fontUI, unit.Sp(8), info, colRadioDim,
				image.Pt(line.Min.X+42, ry+24))
		}
		nc.Pop()

		for len(pl.radioSrchBtns) < len(results) {
			pl.radioSrchBtns = append(pl.radioSrchBtns, uibtn{})
		}
		b := &pl.radioSrchBtns[row]
		b.scrollFn = func(dy float32) { pl.radioScrollAcc += dy }
		if pl.radioPop == nil && ui.button(gtx, b, line, func(gtx layout.Context, down bool) {
			if down {
				fillRect(gtx.Ops, image.Rect(0, 0, line.Dx(), line.Dy()),
					color.NRGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0x14})
			}
		}) {
			pl.openRadioPop(line.Min.X+10, line.Min.Y+10, radioPickOfResult(r))
		}
	}
}
