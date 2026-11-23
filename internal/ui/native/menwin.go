package native

import (
	"image"
	"image/color"
	"strings"
	"time"

	"gioui.org/app"
	"gioui.org/io/system"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"sfrega/internal/language"
)

// Janela de menu (File / Always on top / Config>Language / About),
// aberta pelo ícone de onda do winshade. Centralizada na tela.

const (
	menuW = 180
	menuH = 230
)

// toggleMenu abre/fecha a janela de menu.
func (ui *uiState) toggleMenu() {
	ui.mu.Lock()
	defer ui.mu.Unlock()
	if ui.winMenu == nil {
		w := new(app.Window)
		w.Option(
			app.Title("Menu"),
			app.Decorated(false),
			app.Size(unit.Dp(menuW), unit.Dp(menuH)),
		)
		ui.winMenu = w
		go ui.menuCenterLoop()
		go ui.menuLoop()
		return
	}
	ui.winMenu.Perform(system.ActionClose)
	ui.winMenu = nil
}

// menuCenterLoop posiciona a janela 50px abaixo do player assim que ela
// aparece no WM.
func (ui *uiState) menuCenterLoop() {
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
			ui.mu.Lock()
			ui.winMenu = nil
			ui.mu.Unlock()
			return
		case app.FrameEvent:
			gtx := app.NewContext(&ops, e)
			ui.layoutMenu(gtx)
			e.Frame(gtx.Ops)
		}
	}
}

// layoutMenu desenha a janela de menu (180x230).
func (ui *uiState) layoutMenu(gtx layout.Context) {
	R := func(x, y, w, h int) image.Rectangle { return image.Rect(x, y, x+w, y+h) }

	bg := color.NRGBA{R: 0x2D, G: 0x32, B: 0x42, A: 0xFF}
	fillRect(gtx.Ops, R(0, 0, menuW, menuH), bg)
	border := color.NRGBA{R: 0x5A, G: 0x62, B: 0x78, A: 0xFF}
	fillRect(gtx.Ops, image.Rect(0, 0, menuW, 1), border)
	fillRect(gtx.Ops, image.Rect(0, menuH-1, menuW, menuH), border)
	fillRect(gtx.Ops, image.Rect(0, 0, 1, menuH), border)
	fillRect(gtx.Ops, image.Rect(menuW-1, 0, menuW, menuH), border)

	// ---- titlebar ----
	fillRect(gtx.Ops, R(0, 0, menuW, 16), color.NRGBA{R: 0x22, G: 0x26, B: 0x36, A: 0xFF})
	ui.textAt(gtx, ui.fontUI, unit.Sp(9), "Menu",
		color.NRGBA{R: 0xD8, G: 0xDE, B: 0xEC, A: 0xFF}, image.Pt(8, 4))
	dragArea(gtx, R(0, 0, menuW-20, 16))
	dragAreaX11(gtx, R(0, 0, menuW-20, 16), nil)
	if ui.spriteBtn(gtx, &ui.btnMenuClose, "TITLEBAR", R(menuW-18, 2, 14, 12),
		R(18, 0, 9, 9), R(18, 9, 9, 9)) {
		ui.winMenu.Perform(system.ActionClose)
	}

	txtCol := color.NRGBA{R: 0xD8, G: 0xDE, B: 0xEC, A: 0xFF}
	dimCol := color.NRGBA{R: 0x90, G: 0x98, B: 0xAC, A: 0xFF}
	y := 24
	bi := 0

	drawRow := func(label string, indent int, check bool, arrow bool, fn func()) {
		if bi >= len(ui.menuBtns) {
			return
		}
		b := &ui.menuBtns[bi]
		bi++
		ry := y
		y += 18
		px := 14 + indent*16
		if ui.button(gtx, b, R(6, ry-2, menuW-12, 18), func(gtx layout.Context, down bool) {
			if down {
				fillRect(gtx.Ops, R(6, ry-2, menuW-12, 18),
					color.NRGBA{R: 0x3A, G: 0x42, B: 0x58, A: 0xFF})
			}
			pre := ""
			if check {
				if ui.alwaysOnTop {
					pre = "x "
				} else {
					pre = "  "
				}
			}
			suf := ""
			if arrow {
				suf = "  >"
			}
			ui.textAt(gtx, ui.fontUI, unit.Sp(9), pre+label+suf, txtCol, image.Pt(px, ry+2))
		}) {
			fn()
		}
	}

	if ui.menuAbout {
		ui.textAt(gtx, ui.fontBold, unit.Sp(11), language.T("about.title"), txtCol, image.Pt(14, y))
		y += 20
		for _, ln := range strings.Split(language.T("about.body"), "\n") {
			ui.textAt(gtx, ui.fontUI, unit.Sp(9), ln, dimCol, image.Pt(14, y))
			y += 15
		}
		y += 10
		drawRow(language.T("about.close"), 0, false, false, func() {
			ui.menuAbout = false
		})
		return
	}

	drawRow(language.T("menu.file"), 0, false, true, func() {
		ui.menuFile = !ui.menuFile
	})
	if ui.menuFile {
		drawRow(language.T("menu.file.exit"), 1, false, false, func() {
			ui.winMenu.Perform(system.ActionClose)
			ui.winMain.Perform(system.ActionClose)
		})
	}
	drawRow(language.T("menu.ontop"), 0, true, false, func() {
		ui.alwaysOnTop = !ui.alwaysOnTop
		saveOntop(ui.alwaysOnTop)
		wmOntop("sfrega", ui.alwaysOnTop)
	})
	drawRow(language.T("menu.config"), 0, false, true, func() {
		ui.menuCfg = !ui.menuCfg
	})
	if ui.menuCfg {
		drawRow(language.T("menu.config.lang"), 1, false, true, func() {
			ui.menuLang = !ui.menuLang
		})
		if ui.menuLang {
			for _, l := range language.Available() {
				lbl := language.T("menu.lang." + l)
				if lbl == "menu.lang."+l {
					lbl = l
				}
				code := l
				drawRow(lbl, 2, false, false, func() {
					language.Set(code)
				})
			}
		}
	}
	drawRow(language.T("menu.about"), 0, false, false, func() {
		ui.menuAbout = true
	})
}
