package native

import (
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/unit"
	"gioui.org/widget"
)

// ---- playlists de rádios (grupos Rock/Pop/NewAge + personalizadas) ----
// Persistidas em configs/radioPlaylists.json.

// radioPlEntry é uma estação dentro de uma playlist de rádio.
type radioPlEntry struct {
	Name     string `json:"name"`
	URL      string `json:"url,omitempty"`
	Icon     string `json:"icon,omitempty"`
	Guide    string `json:"guide,omitempty"` // GuideId TuneIn (resolve ao tocar)
	Desc     string `json:"desc,omitempty"`
	Subtitle string `json:"subtitle,omitempty"`
}

// radioPick é a estação neutra usada pelo balão (catálogo ou busca).
type radioPick struct {
	Name     string
	URL      string
	Icon     string
	Guide    string
	Desc     string
	Subtitle string
}

// radioPopState é o balão de contexto de uma estação.
type radioPopState struct {
	x, y  int
	pick  radioPick
	mode  int // 0 = menu, 1 = nome da nova playlist
	btns  []uibtn
	ed    widget.Editor
	edTag event.Tag
	menuR image.Rectangle // rect do menu (para clique-fora)
}

var (
	radioPlMu    sync.Mutex
	radioPlCache map[string][]radioPlEntry
	radioPlOnce  sync.Once
)

// radioPlBuiltin são os grupos criados por padrão.
var radioPlBuiltin = []string{"Rock", "Pop", "NewAge"}

func radioPlPath() string { return configPath("radioPlaylists.json") }

// loadRadioPls carrega (uma vez) as playlists de rádio; garante os grupos
// padrão mesmo num arquivo novo.
func loadRadioPls() map[string][]radioPlEntry {
	radioPlOnce.Do(func() {
		m := map[string][]radioPlEntry{}
		if b, err := os.ReadFile(radioPlPath()); err == nil {
			var f struct {
				Playlists map[string][]radioPlEntry `json:"playlists"`
			}
			if json.Unmarshal(b, &f) == nil && f.Playlists != nil {
				m = f.Playlists
			}
		}
		for _, g := range radioPlBuiltin {
			if m[g] == nil {
				m[g] = []radioPlEntry{}
			}
		}
		radioPlCache = m
	})
	return radioPlCache
}

func saveRadioPls() {
	radioPlMu.Lock()
	defer radioPlMu.Unlock()
	m := loadRadioPls()
	out := struct {
		Playlists map[string][]radioPlEntry `json:"playlists"`
	}{Playlists: m}
	b, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return
	}
	_ = os.MkdirAll(filepath.Dir(radioPlPath()), 0o755)
	_ = os.WriteFile(radioPlPath(), b, 0o644)
}

// radioPlGroups devolve os nomes dos grupos ordenados (padrão primeiro).
func radioPlGroups() []string {
	m := loadRadioPls()
	seen := map[string]bool{}
	var out []string
	for _, g := range radioPlBuiltin {
		if _, ok := m[g]; ok {
			out = append(out, g)
			seen[g] = true
		}
	}
	var rest []string
	for g := range m {
		if !seen[g] {
			rest = append(rest, g)
		}
	}
	sort.Strings(rest)
	return append(out, rest...)
}

// radioPlKey identifica a estação dentro de uma playlist (guide > url > nome).
func radioPlKey(e radioPlEntry) string {
	if e.Guide != "" {
		return "g:" + e.Guide
	}
	if e.URL != "" {
		return "u:" + e.URL
	}
	return "n:" + strings.ToLower(e.Name)
}

func radioPlEntryOf(p radioPick) radioPlEntry {
	return radioPlEntry{Name: p.Name, URL: p.URL, Icon: p.Icon, Guide: p.Guide, Desc: p.Desc, Subtitle: p.Subtitle}
}

func radioPlHas(group string, e radioPlEntry) bool {
	k := radioPlKey(e)
	for _, x := range loadRadioPls()[group] {
		if radioPlKey(x) == k {
			return true
		}
	}
	return false
}

// radioPlToggle adiciona/remove a estação do grupo e persiste.
func radioPlToggle(group string, e radioPlEntry) {
	m := loadRadioPls()
	k := radioPlKey(e)
	list := m[group]
	out := list[:0]
	found := false
	for _, x := range list {
		if radioPlKey(x) == k && !found {
			found = true
			continue
		}
		out = append(out, x)
	}
	if !found {
		out = append(out, e)
	}
	m[group] = out
	saveRadioPls()
}

// ---- catálogo (botão RADIOS) ----

// radioCatalogHas diz se a estação já existe no catálogo (por URL ou nome).
func radioCatalogHas(p radioPick) bool {
	for _, st := range loadRadios() {
		if p.URL != "" && st.URL == p.URL {
			return true
		}
		if strings.EqualFold(st.Name, p.Name) {
			return true
		}
	}
	return false
}

// saveRadiosCatalog regrava o radios.json do usuário com a lista atual.
func saveRadiosCatalog() {
	out := struct {
		Stations []radioStation `json:"stations"`
	}{Stations: radiosList}
	b, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return
	}
	_ = os.MkdirAll(filepath.Dir(configPath("radios.json")), 0o755)
	_ = os.WriteFile(configPath("radios.json"), b, 0o644)
}

// radioCatalogAdd adiciona a estação ao catálogo e persiste.
func radioCatalogAdd(st radioStation) {
	for _, x := range radiosList {
		if st.URL != "" && x.URL == st.URL || strings.EqualFold(x.Name, st.Name) {
			return
		}
	}
	radiosList = append(radiosList, st)
	saveRadiosCatalog()
}

// ---- tocar uma entrada de playlist ----

// playRadioEntry toca a estação; sem URL usa o GuideId (resolve em background).
func (ui *uiState) playRadioEntry(e radioPlEntry) {
	if e.URL != "" {
		_ = ui.p.PlayStream(e.Name, e.URL)
		return
	}
	if e.Guide == "" {
		return
	}
	go func() {
		u, err := resolveRadioStream(e.Guide)
		if err == nil && u != "" {
			_ = ui.p.PlayStream(e.Name, u)
		}
	}()
}

func radioPickOfStation(st radioStation) radioPick {
	return radioPick{Name: st.Name, URL: st.URL, Icon: st.Icon, Desc: st.Desc}
}

func radioPickOfResult(r radioResult) radioPick {
	return radioPick{Name: r.Title, Icon: r.Image, Guide: r.GuideID, Desc: r.Desc, Subtitle: r.Subtitle}
}

// ---- balão de contexto ----

const (
	radioPopW   = 190
	radioPopRow = 20
)

// openRadioPop abre o balão sobre a estação na posição (x,y) da janela.
func (pl *plw) openRadioPop(x, y int, p radioPick) {
	pl.radioPop = &radioPopState{pick: p, x: x, y: y}
	pl.radioPop.ed.SingleLine = true
	pl.radioPop.ed.Submit = true
	if pl.win != nil {
		pl.win.Invalidate()
	}
}

func (pl *plw) closeRadioPop() {
	pl.radioPop = nil
	if pl.win != nil {
		pl.win.Invalidate()
	}
}

// plRadioPop desenha o balão por cima de tudo (chamar por último no layout).
func (pl *plw) plRadioPop(gtx layout.Context, winW, winH int) {
	pop := pl.radioPop
	if pop == nil || pl.key != "rd" {
		return
	}
	ui := pl.ui
	groups := radioPlGroups()

	var mh int
	if pop.mode == 0 {
		// header + Tocar + Adicionar + sep + "playlists" + grupos + "+ adicionar"
		mh = 120 + len(groups)*radioPopRow
	} else {
		mh = 8 + radioPopRow*3
	}
	mw := radioPopW
	mx := pop.x
	my := pop.y
	if mx+mw > winW-4 {
		mx = winW - 4 - mw
	}
	if my+mh > winH-plBotH-4 {
		my = winH - plBotH - 4 - mh
	}
	if mx < 4 {
		mx = 4
	}
	if my < plTopH+20 {
		my = plTopH + 20
	}
	menuR := image.Rect(mx, my, mx+mw, my+mh)
	pop.menuR = menuR

	// bloqueador: clique fora fecha o balão
	{
		bc := clip.Rect(image.Rect(0, 0, winW, winH)).Push(gtx.Ops)
		event.Op(gtx.Ops, &pl.radioPopBlock)
		bc.Pop()
		for {
			ev, ok := gtx.Event(pointer.Filter{Target: &pl.radioPopBlock, Kinds: pointer.Press})
			if !ok {
				break
			}
			if pe, ok := ev.(pointer.Event); ok {
				p := pe.Position
				if p.X < float32(menuR.Min.X) || p.X >= float32(menuR.Max.X) ||
					p.Y < float32(menuR.Min.Y) || p.Y >= float32(menuR.Max.Y) {
					pl.closeRadioPop()
					return
				}
			}
		}
	}

	// caixa do balão
	fillRect(gtx.Ops, menuR, color.NRGBA{R: 0x18, G: 0x18, B: 0x22, A: 0xFF})
	bevel(gtx, menuR, color.NRGBA{R: 0x70, G: 0x70, B: 0x80, A: 0xFF},
		color.NRGBA{R: 0x08, G: 0x08, B: 0x0C, A: 0xFF})

	for len(pop.btns) < 16 {
		pop.btns = append(pop.btns, uibtn{})
	}

	rowY := menuR.Min.Y + 4
	btnIdx := 0
	drawRow := func(r image.Rectangle, label string, enabled bool, fn func()) {
		fg := colRadioTxt
		if !enabled {
			fg = colRadioDim
		}
		ui.textAt(gtx, ui.fontUI, unit.Sp(9), label, fg, image.Pt(r.Min.X+6, r.Min.Y+5))
		if !enabled {
			return
		}
		b := &pop.btns[btnIdx]
		btnIdx++
		if ui.button(gtx, b, r, func(gtx layout.Context, down bool) {
			if down {
				fillRect(gtx.Ops, image.Rect(0, 0, r.Dx(), r.Dy()),
					color.NRGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0x14})
			}
		}) {
			fn()
		}
	}
	line := func(label string, enabled bool, fn func()) {
		r := image.Rect(menuR.Min.X+2, rowY, menuR.Max.X-2, rowY+radioPopRow-2)
		drawRow(r, label, enabled, fn)
		rowY += radioPopRow
	}

	if pop.mode == 0 {
		// cabeçalho: nome da estação (clipado)
		nc := clip.Rect(image.Rect(menuR.Min.X+6, rowY, menuR.Max.X-6, rowY+radioPopRow)).Push(gtx.Ops)
		ui.textAt(gtx, ui.fontBold, unit.Sp(9), pop.pick.Name, colRadioTxt,
			image.Pt(menuR.Min.X+6, rowY+5))
		nc.Pop()
		rowY += radioPopRow

		full := radioPlEntryOf(pop.pick)
		inCatalog := radioCatalogHas(pop.pick)

		line("Tocar", true, func() { pl.playRadioPick(pop.pick) })
		addLbl := "Adicionar"
		if inCatalog {
			addLbl = "já adicionada"
		}
		line(addLbl, !inCatalog, func() { pl.radioAddToCatalog() })

		// separador + rótulo "playlists"
		fillRect(gtx.Ops, image.Rect(menuR.Min.X+4, rowY+8, menuR.Max.X-4, rowY+10),
			color.NRGBA{R: 0x50, G: 0x50, B: 0x60, A: 0xFF})
		rowY += 12
		ui.textAt(gtx, ui.fontUI, unit.Sp(8), "playlists", colRadioDim,
			image.Pt(menuR.Min.X+6, rowY+6))
		rowY += radioPopRow

		for _, g := range groups {
			lbl := g
			if radioPlHas(g, full) {
				lbl = g + "  ✓"
			}
			grp := g
			line(lbl, true, func() { pl.radioTogglePlaylist(grp) })
		}
		line("+ adicionar playlist", true, func() {
			pop.mode = 1
			pop.ed.SetText("")
			if pl.win != nil {
				pl.win.Invalidate()
			}
		})
	} else {
		// modo nome da nova playlist
		ui.textAt(gtx, ui.fontUI, unit.Sp(9), "nome da playlist:", colRadioDim,
			image.Pt(menuR.Min.X+6, rowY+5))
		rowY += radioPopRow

		erR := image.Rect(menuR.Min.X+4, rowY+2, menuR.Max.X-4, rowY+radioPopRow-2)
		fillRect(gtx.Ops, erR, color.NRGBA{R: 0x0A, G: 0x0A, B: 0x10, A: 0xFF})
		// foco ao clicar (mesmo truque do campo de busca)
		{
			dc := clip.Rect(erR).Push(gtx.Ops)
			event.Op(gtx.Ops, &pop.edTag)
			dc.Pop()
			for {
				_, ok := gtx.Event(pointer.Filter{Target: &pop.edTag, Kinds: pointer.Release})
				if !ok {
					break
				}
				gtx.Execute(key.FocusCmd{Tag: &pop.ed})
			}
		}
		ic := clip.Rect(erR).Push(gtx.Ops)
		off := op.Offset(image.Pt(erR.Min.X+4, erR.Min.Y+3)).Push(gtx.Ops)
		gtxE := gtx
		gtxE.Constraints = layout.Constraints{Min: image.Point{}, Max: image.Pt(erR.Dx()-8, 14)}
		ui.shapeMu.Lock()
		for {
			ev, ok := pop.ed.Update(gtxE)
			if !ok {
				break
			}
			if _, isSubmit := ev.(widget.SubmitEvent); isSubmit {
				pl.radioCreatePlaylist()
			}
		}
		pop.ed.Layout(gtxE, ui.shaper, ui.fontUI, unit.Sp(10),
			ui.txtMat(gtx, color.NRGBA{R: 0xEE, G: 0xEE, B: 0xEE, A: 0xFF}),
			ui.txtMat(gtx, color.NRGBA{R: 0x40, G: 0x80, B: 0xFF, A: 0xFF}))
		ui.shapeMu.Unlock()
		off.Pop()
		ic.Pop()
		rowY += radioPopRow

		okR := image.Rect(menuR.Min.X+4, rowY, menuR.Min.X+44, rowY+radioPopRow-4)
		ccR := image.Rect(menuR.Max.X-44, rowY, menuR.Max.X-4, rowY+radioPopRow-4)
		drawRow(okR, "OK", true, func() { pl.radioCreatePlaylist() })
		drawRow(ccR, "sair", true, func() {
			pop.mode = 0
			if pl.win != nil {
				pl.win.Invalidate()
			}
		})
	}
}

// ---- ações do balão ----

// playRadioPick toca a estação do balão (catálogo: URL direta; busca:
// resolve o GuideId ou usa URL se já conhecida).
func (pl *plw) playRadioPick(p radioPick) {
	pl.closeRadioPop()
	if p.URL != "" {
		_ = pl.ui.p.PlayStream(p.Name, p.URL)
		return
	}
	if p.Guide != "" {
		go func() {
			u, err := resolveRadioStream(p.Guide)
			if err == nil && u != "" {
				_ = pl.ui.p.PlayStream(p.Name, u)
			}
		}()
	}
}

// radioAddToCatalog adiciona a estação à lista do botão RADIOS
// (resolve a URL antes quando ela ainda não é conhecida).
func (pl *plw) radioAddToCatalog() {
	p := pl.radioPop.pick
	pl.closeRadioPop()
	if p.URL != "" {
		radioCatalogAdd(radioStation{Name: p.Name, URL: p.URL, Icon: p.Icon,
			Type: "search", Desc: p.Desc})
		return
	}
	if p.Guide == "" {
		return
	}
	go func() {
		u, err := resolveRadioStream(p.Guide)
		if err != nil {
			return
		}
		radioCatalogAdd(radioStation{Name: p.Name, URL: u, Icon: p.Icon,
			Type: "search", Desc: p.Desc})
		if pl.win != nil {
			pl.win.Invalidate()
		}
	}()
}

// radioTogglePlaylist alterna a estação no grupo e fecha o balão.
func (pl *plw) radioTogglePlaylist(group string) {
	if pl.radioPop == nil {
		return
	}
	e := radioPlEntryOf(pl.radioPop.pick)
	radioPlToggle(group, e)
	pl.closeRadioPop()
}

// radioCreatePlaylist cria o grupo digitado e já coloca a estação nele.
func (pl *plw) radioCreatePlaylist() {
	pop := pl.radioPop
	if pop == nil {
		return
	}
	name := strings.TrimSpace(pop.ed.Text())
	pop.mode = 0
	if name == "" {
		return
	}
	m := loadRadioPls()
	if m[name] == nil {
		m[name] = []radioPlEntry{}
	}
	e := radioPlEntryOf(pop.pick)
	k := radioPlKey(e)
	found := false
	for _, x := range m[name] {
		if radioPlKey(x) == k {
			found = true
		}
	}
	if !found {
		m[name] = append(m[name], e)
	}
	saveRadioPls()
}

// ---- painel GENRE da janela Radio: grupos de playlists de estações ----

const (
	radioGrpRowH = 30 // linha de grupo
	radioPlRowH  = 26 // linha de estação dentro do grupo
)

// plRadioGroupPanel desenha o painel de grupos (Rock/Pop/NewAge/...)
// com as estações de cada playlist; clique no grupo expande/fecha,
// clique na estação toca. Devolve a altura usada (genreH).
func (pl *plw) plRadioGroupPanel(gtx layout.Context, winW, botY int) int {
	ui := pl.ui
	groups := radioPlGroups()
	pls := loadRadioPls()

	type row struct {
		entry bool
		group string
		e     radioPlEntry
	}
	var rows []row
	for _, g := range groups {
		rows = append(rows, row{group: g})
		if pl.plGenreExpand == g {
			for _, e := range pls[g] {
				rows = append(rows, row{entry: true, group: g, e: e})
			}
		}
	}
	for len(pl.plGenreBtns) < len(rows) {
		pl.plGenreBtns = append(pl.plGenreBtns, uibtn{})
	}

	areaTop := plTopH + 16
	avail := botY - 3 - areaTop
	contentH := 0
	for _, rw := range rows {
		if rw.entry {
			contentH += radioPlRowH
		} else {
			contentH += radioGrpRowH
		}
	}
	gH := contentH
	if gH > avail {
		gH = avail
	}
	if gH < 0 {
		gH = 0
	}
	pl.plCount = len(groups)
	pl.plCount2 = 0
	if exp := pls[pl.plGenreExpand]; pl.plGenreExpand != "" {
		pl.plCount2 = len(exp)
	}
	pl.plGenreMax = float32(contentH - gH)
	if pl.plGenreMax < 0 {
		pl.plGenreMax = 0
	}
	if pl.plGenreScroll > pl.plGenreMax {
		pl.plGenreScroll = pl.plGenreMax
	}
	if pl.plGenreScroll < 0 {
		pl.plGenreScroll = 0
	}

	panel := image.Rect(12, areaTop, winW-23, areaTop+gH)
	fillRect(gtx.Ops, panel, pl.plBgColor())
	cs := clip.Rect(panel).Push(gtx.Ops)
	ry := areaTop - int(pl.plGenreScroll)
	pl.plGenreWheel(gtx, panel)
	pl.plScrollSink = &pl.plGenreScrollAcc

	visMin := areaTop - radioGrpRowH
	visMax := areaTop + gH
	ri := 0
	for _, rw := range rows {
		rh := radioGrpRowH
		if rw.entry {
			rh = radioPlRowH
		}
		rowR := image.Rect(14, ry, winW-27, ry+rh)
		if ry > visMin && ry < visMax && pl.radioPop == nil {
			b := &pl.plGenreBtns[ri]
			b.scrollFn = func(dy float32) { pl.plGenreScrollAcc += dy }
			if ui.button(gtx, b, rowR, func(gtx layout.Context, down bool) {
				if down {
					fillRect(gtx.Ops, image.Rect(0, 0, rowR.Dx(), rowR.Dy()),
						color.NRGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0x14})
				}
			}) {
				if rw.entry {
					ui.playRadioEntry(rw.e)
				} else if pl.plGenreExpand == rw.group {
					pl.plGenreExpand = ""
					pl.savePanel()
				} else {
					pl.plGenreExpand = rw.group
					pl.savePanel()
				}
			}
		}
		// desenho da linha (sempre, mesmo com balão aberto)
		if ry > visMin && ry < visMax {
			if rw.entry {
				st := radioStation{Icon: rw.e.Icon, Name: rw.e.Name}
				ir := image.Rect(rowR.Min.X+16, ry+3, rowR.Min.X+32, ry+19)
				if img := radioIcon(st); img != nil {
					blit(gtx, img, ir)
				} else {
					fillRect(gtx.Ops, ir, colRadioPh)
				}
				ui.textAt(gtx, ui.fontUI, unit.Sp(9), rw.e.Name, colRadioTxt,
					image.Pt(rowR.Min.X+38, ry+5))
			} else {
				exp := pl.plGenreExpand == rw.group
				marker := "▸"
				ac := colPLText
				if exp {
					marker = "▾"
					ac = colPLSelText
				}
				ui.textAt(gtx, ui.fontUI, unit.Sp(10), marker, ac, image.Pt(rowR.Min.X+4, ry+7))
				ui.textAt(gtx, ui.fontBold, unit.Sp(10), rw.group, ac, image.Pt(rowR.Min.X+20, ry+7))
				n := len(pls[rw.group])
				cnt := fmt.Sprintf("%d", n)
				ui.textAt(gtx, ui.fontUI, unit.Sp(9), cnt, colPLFile, image.Pt(rowR.Min.X+20+len(rw.group)*7+8, ry+8))
			}
		}
		ry += rh
		ri++
	}
	cs.Pop()

	// aplica scroll acumulado
	if pl.plGenreScrollAcc != 0 {
		pl.plGenreScroll -= pl.plGenreScrollAcc
		if pl.plGenreScroll > pl.plGenreMax {
			pl.plGenreScroll = pl.plGenreMax
		}
		if pl.plGenreScroll < 0 {
			pl.plGenreScroll = 0
		}
		pl.plGenreScrollAcc = 0
	}
	// scrollbar
	if contentH > gH && gH > 0 {
		track := image.Rect(winW-20, areaTop+1, winW-16, areaTop+gH-1)
		fillRect(gtx.Ops, track, pl.plBgColor())
		thumbH := gH * gH / contentH
		if thumbH < 12 {
			thumbH = 12
		}
		if thumbH > gH-2 {
			thumbH = gH - 2
		}
		frac := float32(0)
		if pl.plGenreMax > 0 {
			frac = pl.plGenreScroll / pl.plGenreMax
		}
		thumbY := areaTop + 1 + int(frac*float32(gH-2-thumbH))
		fillRect(gtx.Ops, image.Rect(winW-20, thumbY, winW-16, thumbY+thumbH),
			color.NRGBA{R: 0x80, G: 0x80, B: 0x80, A: 0xFF})
		pl.plGenreScrollDrag(gtx, track, thumbY, thumbH, gH, pl.plGenreMax)
	}
	return gH
}
