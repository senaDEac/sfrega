package native

import (
	"image"
	"image/color"
	"strings"
	"time"

	"gioui.org/app"
	"gioui.org/f32"
	"gioui.org/io/event"
	"gioui.org/io/pointer"
	"gioui.org/io/system"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"

	"sfrega/internal/language"
	"sfrega/internal/skins"
	"sfrega/internal/spectrum"
)

// Janela de menu (File / Always on top / Config>Language / About),
// aberta pelo ícone de onda do winshade. Visual de menu clássico de
// janela: titlebar da skin + itens claros com hover azul + scrollbar.

const (
	menuRowH = 17
	menuSepH = 8
)

var (
	colMenuBg     = color.NRGBA{R: 0xEC, G: 0xE9, B: 0xD8, A: 0xFF}
	colMenuText   = color.NRGBA{R: 0x20, G: 0x20, B: 0x20, A: 0xFF}
	colMenuDim    = color.NRGBA{R: 0x7A, G: 0x7A, B: 0x72, A: 0xFF}
	colMenuHover  = color.NRGBA{R: 0x31, G: 0x6A, B: 0xC5, A: 0xFF}
	colMenuHiText = color.NRGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF}
	colMenuSep    = color.NRGBA{R: 0xAC, G: 0xA8, B: 0x99, A: 0xFF}
	colMenuSepHi  = color.NRGBA{R: 0xFF, G: 0xFF, B: 0xF0, A: 0xFF}
	colMenuFrame  = color.NRGBA{R: 0x40, G: 0x40, B: 0x40, A: 0xFF}
	colMenuTrack  = color.NRGBA{R: 0xD0, G: 0xCC, B: 0xBC, A: 0xFF}
	colMenuThumb  = color.NRGBA{R: 0x9A, G: 0x96, B: 0x86, A: 0xFF}
	colMenuThumbD = color.NRGBA{R: 0x5A, G: 0x7A, B: 0x9A, A: 0xFF}
)

// mrow é uma linha do menu: item, separador ou pai de submenu.
type mrow struct {
	label   string
	indent  int
	kind    int // 0=item, 1=check, 2=pai (seta)
	sep     bool
	checked bool
	act     func()
}

// toggleMenu abre/fecha a janela de menu.
func (ui *uiState) toggleMenu() {
	ui.mu.Lock()
	defer ui.mu.Unlock()
	if ui.winMenu == nil {
		ui.menuScale = ui.mainScale
		w := new(app.Window)
		w.Option(
			app.Title("Menu"),
			app.Decorated(false),
			app.Size(unit.Dp(275*ui.menuScale), unit.Dp(232*ui.menuScale)),
		)
		ui.winMenu = w
		ui.menuOpen = true
		saveOpenWindows(ui.eqOpen, ui.plMain.plOpen, true, ui.plRadio.plOpen, ui.plVideo.plOpen)
		go ui.menuPlaceLoop()
		go ui.menuLoop()
		return
	}
	ui.menuOpen = false
	saveOpenWindows(ui.eqOpen, ui.plMain.plOpen, false, ui.plRadio.plOpen, ui.plVideo.plOpen)
	ui.winMenu.Perform(system.ActionClose)
	ui.winMenu = nil
}

// closeMenu fecha a janela de menu com segurança (winMenu pode já ter sido
// anulado pelo DestroyEvent enquanto um frame ainda renderiza).
func (ui *uiState) closeMenu() {
	ui.mu.Lock()
	w := ui.winMenu
	ui.mu.Unlock()
	if w != nil {
		w.Perform(system.ActionClose)
	}
}

// menuPlaceLoop posiciona a janela 50px abaixo do player assim que ela
// aparece no WM.
func (ui *uiState) menuPlaceLoop() {
	for i := 0; i < 60; i++ {
		if wmBelow("Menu", "sfrega", 50) {
			return
		}
		select {
		case <-time.After(50 * time.Millisecond):
		}
	}
}

// menuLoop é o loop de eventos da janela de menu.
func (ui *uiState) menuLoop() {
	var ops op.Ops
	w := ui.winMenu
	for {
		switch e := w.Event().(type) {
		case app.DestroyEvent:
			wmSaveGeometry()
			ui.mu.Lock()
			ui.winMenu = nil
			ui.menuOpen = false
			delete(ui.ontopDone, "Menu")
			saveOpenWindows(ui.eqOpen, ui.plMain.plOpen, false, ui.plRadio.plOpen, ui.plVideo.plOpen)
			ui.mu.Unlock()
			ui.menuAbout = false
			ui.menuFile = false
			ui.menuCfg = false
			ui.menuLang = false
			ui.menuSkins = false
			ui.menuCnt = false
			ui.menuSpec = false
			ui.menuScroll = 0
			return
		case app.FrameEvent:
			gtx := app.NewContext(&ops, e)
			gtx = beginScaledFrame(gtx, ui.menuScale, image.Pt(275, 232))
			ui.layoutMenu(gtx)
			e.Frame(gtx.Ops)
		}
	}
}

// layoutMenu desenha a janela de menu: cópia fiel da janela do player
// (MAIN.png + titlebar + botões), exceto o miolo — fundo azul escuro.
func (ui *uiState) layoutMenu(gtx layout.Context) {
	R := func(x, y, w, h int) image.Rectangle { return image.Rect(x, y, x+w, y+h) }

	ui.spr(gtx, "MAIN", R(0, 0, 275, 107), R(0, 0, 275, 107))
	// altura dobrada: estende laterais e base com as bordas do MAIN
	ui.spr(gtx, "MAIN", R(0, 0, 2, 107), R(0, 107, 2, 125))
	ui.spr(gtx, "MAIN", R(273, 0, 2, 107), R(273, 107, 2, 125))
	ui.spr(gtx, "MAIN", R(0, 97, 275, 10), R(0, 222, 275, 10))
	ui.spr(gtx, "TITLEBAR", R(47, 0, 222, 11), R(20, 0, 222, 11))
	ui.spr(gtx, "TITLEBAR", R(0, 0, 9, 9), R(6, 3, 9, 9))

	dragArea(gtx, R(16, 0, 228, 11))
	dragAreaX11(gtx, R(16, 0, 228, 11), nil)
	if ui.spriteBtn(gtx, &ui.btnMenuMin, "TITLEBAR", R(244, 3, 9, 9),
		R(9, 0, 9, 9), R(9, 9, 9, 9)) {
		minimizeWindow(ui.winMenu)
	}
	if ui.spriteBtn(gtx, &ui.btnMenuSize, "TITLEBAR", R(254, 3, 9, 9),
		R(0, 18, 9, 9), R(9, 18, 9, 9)) {
		ui.mu.Lock()
		w := ui.winMenu
		ui.mu.Unlock()
		if w != nil {
			toggleWindowScale(&ui.menuScale, w, unit.Dp(275), unit.Dp(116))
		}
	}
	if ui.spriteBtn(gtx, &ui.btnMenuClose, "TITLEBAR", R(264, 3, 9, 9),
		R(18, 0, 9, 9), R(18, 9, 9, 9)) {
		ui.closeMenu()
	}

	// miolo: fundo azul escuro + itens de menu
	contentTop := 12
	contentBot := 230
	availH := contentBot - contentTop
	fillRect(gtx.Ops, R(2, contentTop, 271, availH),
		color.NRGBA{R: 0x2D, G: 0x32, B: 0x42, A: 0xFF})

	if ui.menuAbout {
		cs := clip.Rect(R(2, contentTop, 271, availH)).Push(gtx.Ops)
		ui.menuAboutPanel(gtx, 275, contentTop, availH, R)
		cs.Pop()
		return
	}

	rows := ui.menuRows()
	totalH := 0
	for _, r := range rows {
		if r.sep {
			totalH += menuSepH
		} else {
			totalH += menuRowH
		}
	}
	maxScroll := totalH - availH
	if maxScroll < 0 {
		maxScroll = 0
	}
	if ui.menuScroll > maxScroll {
		ui.menuScroll = maxScroll
	}
	if ui.menuScroll < 0 {
		ui.menuScroll = 0
	}
	needBar := totalH > availH

	// linhas repassam a roda do mouse para o scroll do menu
	for i := range ui.menuBtns {
		ui.menuBtns[i].scrollFn = func(dy float32) {
			ui.menuScroll -= int(dy)
			if ui.menuScroll < 0 {
				ui.menuScroll = 0
			}
			if ui.menuScroll > maxScroll {
				ui.menuScroll = maxScroll
			}
		}
	}

	ui.menuWheel(gtx, R(2, contentTop, 271, availH), maxScroll)

	cs := clip.Rect(R(2, contentTop, 271, availH)).Push(gtx.Ops)
	y := contentTop - ui.menuScroll
	bi := 0
	rowRight := 271
	if needBar {
		rowRight = 258
	}
	for _, row := range rows {
		h := menuRowH
		if row.sep {
			h = menuSepH
		}
		if y+h > contentTop && y < contentBot {
			ui.menuDrawRow(gtx, row, &bi, y, rowRight, R)
		}
		y += h
	}
	cs.Pop()

	if needBar {
		ui.menuScrollbar(gtx, 275, contentTop, availH, totalH, maxScroll, R)
	}
}

// menuRows monta a lista visível de linhas conforme os submenus abertos.
func (ui *uiState) menuRows() []mrow {
	var rows []mrow

	rows = append(rows, mrow{label: language.T("menu.file"), kind: 2,
		act: func() { ui.menuFile = !ui.menuFile }})
	if ui.menuFile {
		rows = append(rows, mrow{label: language.T("menu.file.exit"), indent: 1,
			act: func() {
				ui.closeMenu()
				ui.winMain.Perform(system.ActionClose)
			}})
	}
	rows = append(rows, mrow{label: language.T("menu.ontop"), kind: 1,
		act: func() {
			ui.mu.Lock()
			ui.alwaysOnTop = !ui.alwaysOnTop
			on := ui.alwaysOnTop
			ui.ontopDone = map[string]bool{}
			ui.mu.Unlock()
			saveOntop(on)
			for _, n := range ontopWinNames {
				wmOntop(n, on)
			}
		}})
	rows = append(rows, mrow{label: language.T("menu.update_lists"),
		act: func() { go ui.updateAllLists() }})
	rows = append(rows, mrow{label: language.T("menu.config"), kind: 2,
		act: func() { ui.menuCfg = !ui.menuCfg }})
	if ui.menuCfg {
		rows = append(rows, mrow{label: "Language", indent: 1, kind: 2,
			act: func() { ui.menuLang = !ui.menuLang }})
		if ui.menuLang {
			cur := language.Current()
			for _, l := range language.Available() {
				lbl := language.Name(l)
				code := l
				rows = append(rows, mrow{label: lbl, indent: 2,
					checked: l == cur,
					act:     func() { language.Set(code) }})
			}
		}
		rows = append(rows, mrow{label: language.T("menu.skins"), indent: 1, kind: 2,
			act: func() { ui.menuSkins = !ui.menuSkins }})
		if ui.menuSkins {
			curSkin := loadSkinSel()
			if curSkin == "" {
				curSkin = "padrao"
			}
			rows = append(rows, mrow{label: language.T("menu.default"), indent: 2,
				checked: curSkin == "padrao",
				act:     func() { ui.applySkin("padrao"); saveSkinSel("padrao") }})
			for _, n := range skins.WszNames() {
				code := n
				rows = append(rows, mrow{label: trMenuName("skin.", n), indent: 2,
					checked: code == curSkin,
					act:     func() { ui.applySkin(code); saveSkinSel(code) }})
			}
		}
		rows = append(rows, mrow{label: language.T("menu.counters"), indent: 1, kind: 2,
			act: func() { ui.menuCnt = !ui.menuCnt }})
		if ui.menuCnt {
			rows = append(rows, mrow{label: language.T("menu.counters.reset_shuffle"), indent: 2,
				act: func() { ui.openConfirm(1) }})
			rows = append(rows, mrow{label: language.T("menu.counters.reset_plays"), indent: 2,
				act: func() { ui.openConfirm(2) }})
			rows = append(rows, mrow{label: language.T("menu.counters.reset_tags"), indent: 2,
				act: func() { ui.openConfirm(3) }})
		}
		rows = append(rows, mrow{label: language.T("menu.spectrum"), indent: 1, kind: 2,
			act: func() { ui.menuSpec = !ui.menuSpec }})
		if ui.menuSpec {
			cur := ui.specName
			if cur == "" {
				cur = "padrao"
			}
			rows = append(rows, mrow{label: language.T("menu.default"), indent: 2,
				checked: cur == "padrao",
				act:     func() { ui.applySpec("padrao"); saveSpecSel("padrao") }})
			for _, n := range spectrum.SpecNames() {
				code := n
				rows = append(rows, mrow{label: trMenuName("spec.", n), indent: 2,
					checked: code == cur,
					act:     func() { ui.applySpec(code); saveSpecSel(code) }})
			}
		}
	}
	rows = append(rows, mrow{label: language.T("menu.about"),
		act: func() { ui.menuAbout = true }})
	rows = append(rows, mrow{label: language.T("about.close"),
		act: func() { ui.closeMenu() }})
	return rows
}

// trMenuName traduz um nome de variante (skin/espectro) pela chave
// prefix+nome; sem tradução, mostra o nome original.
func trMenuName(prefix, n string) string {
	k := prefix + n
	if t := language.T(k); t != k {
		return t
	}
	return n
}

// menuDrawRow desenha uma linha do menu (item, separador ou pai).
func (ui *uiState) menuDrawRow(gtx layout.Context, row mrow, bi *int, y, rowRight int,
	R func(int, int, int, int) image.Rectangle) {
	if row.sep {
		mid := y + menuSepH/2
		fillRect(gtx.Ops, R(8, mid-1, rowRight-16, 1), colMenuSep)
		fillRect(gtx.Ops, R(8, mid, rowRight-16, 1), colMenuSepHi)
		return
	}
	if *bi >= len(ui.menuBtns) {
		return
	}
	b := &ui.menuBtns[*bi]
	(*bi)++
	rx, rw, rh := 4, rowRight-4, menuRowH
	clicked := ui.button(gtx, b, R(rx, y, rw, rh), func(gtx layout.Context, down bool) {
		tx := colMenuHiText
		if down || b.over {
			fillRect(gtx.Ops, R(0, 0, rw, rh), colMenuHover)
		}
		if row.kind == 1 {
			ui.mu.Lock()
			on := ui.alwaysOnTop
			ui.mu.Unlock()
			if on {
				menuCheck(gtx, 10, rh/2, tx)
			}
		} else if row.checked {
			menuCheck(gtx, 10, rh/2, tx)
		}
		ui.textAt(gtx, ui.fontMenu, unit.Sp(10), row.label, tx,
			image.Pt(20+row.indent*16, 2))
		if row.kind == 2 {
			menuArrow(gtx, rw-14, rh/2, tx)
		}
	})
	if clicked {
		row.act()
	}
}

// menuWheel rola a lista com a roda do mouse.
func (ui *uiState) menuWheel(gtx layout.Context, r image.Rectangle, maxScroll int) {
	cs := clip.Rect(r).Push(gtx.Ops)
	event.Op(gtx.Ops, &ui.menuWheelTag)
	cs.Pop()
	for {
		ev, ok := gtx.Event(pointer.Filter{
			Target:  &ui.menuWheelTag,
			Kinds:   pointer.Scroll,
			ScrollX: pointer.ScrollRange{Min: -1 << 30, Max: 1 << 30},
			ScrollY: pointer.ScrollRange{Min: -1 << 30, Max: 1 << 30},
		})
		if !ok {
			break
		}
		e, ok := ev.(pointer.Event)
		if !ok {
			continue
		}
		ui.menuScroll -= int(e.Scroll.Y)
		if ui.menuScroll < 0 {
			ui.menuScroll = 0
		}
		if ui.menuScroll > maxScroll {
			ui.menuScroll = maxScroll
		}
	}
}

// menuScrollTo converte posição do thumb na barra para o offset de scroll.
func (ui *uiState) menuScrollTo(thumbY float32, top float32, availH, thumbH, maxScroll int) {
	maxT := float32(availH - thumbH)
	if maxT <= 0 || maxScroll <= 0 {
		return
	}
	ny := thumbY - top
	if ny < 0 {
		ny = 0
	}
	if ny > maxT {
		ny = maxT
	}
	ui.menuScroll = int(ny * float32(maxScroll) / maxT)
}

// menuScrollbar desenha e processa a barra de rolagem vertical.
func (ui *uiState) menuScrollbar(gtx layout.Context, winW, top, availH, totalH, maxScroll int,
	R func(int, int, int, int) image.Rectangle) {
	sbX := winW - 13
	track := R(sbX, top, 9, availH)
	fillRect(gtx.Ops, track, colMenuTrack)
	fillRect(gtx.Ops, image.Rect(track.Min.X, track.Min.Y, track.Max.X, track.Min.Y+1), colMenuSepHi)
	fillRect(gtx.Ops, image.Rect(track.Min.X, track.Max.Y-1, track.Max.X, track.Max.Y), colMenuSep)

	thumbH := availH * availH / totalH
	if thumbH < 18 {
		thumbH = 18
	}
	if thumbH > availH {
		thumbH = availH
	}
	thumbY := top
	if maxScroll > 0 {
		thumbY = top + (availH-thumbH)*ui.menuScroll/maxScroll
	}
	thumbCol := colMenuThumb
	if ui.menuBarDown {
		thumbCol = colMenuThumbD
	}
	fillRect(gtx.Ops, R(sbX+1, thumbY, 7, thumbH), thumbCol)

	cs := clip.Rect(track).Push(gtx.Ops)
	event.Op(gtx.Ops, &ui.menuBarTag)
	cs.Pop()
	for {
		ev, ok := gtx.Event(pointer.Filter{
			Target:  &ui.menuBarTag,
			Kinds:   pointer.Press | pointer.Drag | pointer.Release | pointer.Cancel | pointer.Scroll,
			ScrollX: pointer.ScrollRange{Min: -1 << 30, Max: 1 << 30},
			ScrollY: pointer.ScrollRange{Min: -1 << 30, Max: 1 << 30},
		})
		if !ok {
			break
		}
		e, ok := ev.(pointer.Event)
		if !ok {
			continue
		}
		switch e.Kind {
		case pointer.Press:
			ui.menuBarDown = true
			ui.menuBarGrab = float32(e.Position.Y) - float32(thumbY)
			if e.Position.Y < float32(thumbY) || e.Position.Y > float32(thumbY+thumbH) {
				ui.menuBarGrab = float32(thumbH) / 2
				ui.menuScrollTo(e.Position.Y-ui.menuBarGrab,
					float32(top), availH, thumbH, maxScroll)
			}
		case pointer.Drag:
			ui.menuScrollTo(e.Position.Y-ui.menuBarGrab,
				float32(top), availH, thumbH, maxScroll)
		case pointer.Scroll:
			ui.menuScroll -= int(e.Scroll.Y)
			if ui.menuScroll < 0 {
				ui.menuScroll = 0
			}
			if ui.menuScroll > maxScroll {
				ui.menuScroll = maxScroll
			}
		case pointer.Release, pointer.Cancel:
			ui.menuBarDown = false
		}
	}
}

// menuAboutPanel mostra o painel About no estilo do menu.
func (ui *uiState) menuAboutPanel(gtx layout.Context, winW, top, availH int,
	R func(int, int, int, int) image.Rectangle) {
	y := top + 8
	ui.textAt(gtx, ui.fontMenuBold, unit.Sp(11), language.T("about.title"), colMenuHiText, image.Pt(14, y))
	y += 20
	for _, ln := range strings.Split(language.T("about.body"), "\n") {
		ui.textAt(gtx, ui.fontMenu, unit.Sp(9), ln, colMenuHiText, image.Pt(14, y))
		y += 14
	}
	y += 8
	b := &ui.menuBtns[0]
	rx, rw, rh := 8, winW-16, menuRowH
	clicked := ui.button(gtx, b, R(rx, y, rw, rh), func(gtx layout.Context, down bool) {
		tx := colMenuHiText
		if down || b.over {
			fillRect(gtx.Ops, R(0, 0, rw, rh), colMenuHover)
		}
		ui.textAt(gtx, ui.fontMenu, unit.Sp(10), language.T("about.close"), tx, image.Pt(12, 4))
	})
	if clicked {
		ui.menuAbout = false
	}
}

// menuCheck desenha um checkmark vectorial.
func menuCheck(gtx layout.Context, cx, cy int, c color.NRGBA) {
	p := clip.Path{}
	p.Begin(gtx.Ops)
	p.MoveTo(f32.Pt(float32(cx-3), float32(cy)))
	p.LineTo(f32.Pt(float32(cx-1), float32(cy+2)))
	p.LineTo(f32.Pt(float32(cx+4), float32(cy-4)))
	paint.FillShape(gtx.Ops, c, clip.Stroke{Path: p.End(), Width: 1.8}.Op())
}

// menuArrow desenha a seta de submenu (triângulo pequeno à direita).
func menuArrow(gtx layout.Context, x, cy int, c color.NRGBA) {
	p := clip.Path{}
	p.Begin(gtx.Ops)
	p.MoveTo(f32.Pt(float32(x), float32(cy-3)))
	p.LineTo(f32.Pt(float32(x), float32(cy+3)))
	p.LineTo(f32.Pt(float32(x+4), float32(cy)))
	p.Close()
	paint.FillShape(gtx.Ops, c, clip.Outline{Path: p.End()}.Op())
}
