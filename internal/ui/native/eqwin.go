package native

import (
	"image"
	"image/color"

	"gioui.org/app"
	"gioui.org/io/system"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
)

var eqFreqLabels = [10]string{"31", "62", "125", "250", "500", "1k", "2k", "4k", "8k", "16k"}

var eqPresets = map[string][10]float64{
	"Flat":       {},
	"Bass Boost": {6, 5, 4, 2, 0, 0, 0, 0, 0, 0},
	"Rock":       {4, 3, 1, -1, -2, -1, 2, 4, 5, 4},
	"Pop":        {-2, -1, 0, 2, 4, 4, 2, 0, -1, -2},
	"Classical":  {3, 2, 0, -1, -2, -2, 0, 2, 3, 4},
	"Dance":      {5, 4, 1, -1, -2, -1, 2, 4, 5, 4},
}

// toggleEQ abre/fecha a janela do equalizador.
func (ui *uiState) toggleEQ() {
	ui.mu.Lock()
	defer ui.mu.Unlock()
	if ui.winEQ == nil {
		w := new(app.Window)
		w.Option(
			app.Title("Equalizador"),
			app.Decorated(false),
			app.Size(unit.Dp(275), unit.Dp(315)),
		)
		ui.winEQ = w
		ui.eqOpen = true
		go ui.eqLoop()
		return
	}
	ui.eqOpen = false
	ui.winEQ.Perform(system.ActionClose)
}

// togglePL abre/fecha a janela da playlist.
func (ui *uiState) togglePL() {
	ui.mu.Lock()
	defer ui.mu.Unlock()
	if ui.winPL == nil {
		w := new(app.Window)
		w.Option(
			app.Title("Playlist"),
			app.Decorated(false),
			app.Size(unit.Dp(275), unit.Dp(240)),
		)
		ui.winPL = w
		ui.plOpen = true
		go ui.plLoop()
		return
	}
	ui.plOpen = false
	ui.winPL.Perform(system.ActionClose)
	ui.winPL = nil
}

// eqLoop é o loop de eventos da janela EQ.
func (ui *uiState) eqLoop() {
	var ops op.Ops
	w := ui.winEQ
	for {
		switch e := w.Event().(type) {
		case app.DestroyEvent:
			ui.mu.Lock()
			ui.winEQ = nil
			ui.eqOpen = false
			ui.mu.Unlock()
			return
		case app.FrameEvent:
			gtx := app.NewContext(&ops, e)
			ui.layoutEQ(gtx)
			e.Frame(gtx.Ops)
		}
	}
}

// applyEQ envia ganhos (dB) ao player.
func (ui *uiState) applyEQ() {
	var gains [10]float64
	for i := range ui.eqSliders {
		gains[i] = float64((ui.eqSliders[i].val - 0.5) * 24)
	}
	ui.p.SetEQ(gains, 0)
}

// layoutEQ desenha a janela do equalizador com a skin EQMAIN (275x315).
func (ui *uiState) layoutEQ(gtx layout.Context) {
	R := func(x, y, w, h int) image.Rectangle { return image.Rect(x, y, x+w, y+h) }

	ui.spr(gtx, "EQMAIN", R(0, 0, 275, 315), R(0, 0, 275, 315))

	// close (TITLEBAR sprite)
	if ui.spriteBtn(gtx, &ui.btnEQClose, "TITLEBAR", R(264, 3, 9, 9), R(18, 0, 9, 9), R(18, 9, 9, 9)) {
		ui.winEQ.Perform(system.ActionClose)
	}

	// ON / AUTO / PRESETS (botões de texto sobre a área y~16-30)
	ui.eqTxtBtn(gtx, &ui.btnEQOn, "ON", 10, 16, func() { ui.eqOn = !ui.eqOn })
	ui.eqTxtBtn(gtx, &ui.btnEQAuto, "AUTO", 40, 16, func() {})
	ui.eqTxtBtn(gtx, &ui.btnEQPres, "PRESETS", 226, 16, func() {})

	// preamp: coluna x=16, faders y170-285
	ui.eqFader(gtx, &ui.preampSlide, 16, true)

	// 10 bandas: colunas x=31..211 step 15 → reespaça para 10 em x=30..240
	for i := 0; i < 10; i++ {
		x := 30 + i*23
		ui.eqFader(gtx, &ui.eqSliders[i], x, false)
		// label freq (EQMAIN já tem escalas; desenhamos valor dB pequeno)
		m := op.Record(gtx.Ops)
		paint.ColorOp{Color: color.NRGBA{R: 0xAA, G: 0xAA, B: 0xAA, A: 0xFF}}.Add(gtx.Ops)
		mat := m.Stop()
		off := op.Offset(image.Pt(x-8, 292)).Push(gtx.Ops)
		widget.Label{}.Layout(gtx, ui.shaper, ui.fontMono, unit.Sp(7), eqFreqLabels[i], mat)
		off.Pop()
	}
}

// eqFader desenha um fader vertical com trilho escuro + thumb claro.
// x = centro do fader; track y170-285.
func (ui *uiState) eqFader(gtx layout.Context, f *fader, x int, isPreamp bool) {
	R := func(a, b, c, d int) image.Rectangle { return image.Rect(a, b, c, d) }
	track := R(x-3, 170, x+3, 285)
	fillRect(gtx.Ops, track, color.NRGBA{R: 0x10, G: 0x10, B: 0x18, A: 0xFF})
	// centro (0dB) marca
	fillRect(gtx.Ops, R(x-3, 226, x+3, 228), color.NRGBA{R: 0x40, G: 0x40, B: 0x50, A: 0xFF})
	// thumb 12x6 na posição do valor
	ty := 285 - int(f.val*float32(115)) - 3
	thumb := R(x-6, ty, x+6, ty+6)
	fillRect(gtx.Ops, thumb, color.NRGBA{R: 0xCC, G: 0xCC, B: 0xDD, A: 0xFF})
	bevel(gtx, thumb, color.NRGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF}, color.NRGBA{R: 0x40, G: 0x40, B: 0x40, A: 0xFF})
	_ = isPreamp
	// input: área track inteira
	r := R(x-8, 170, x+8, 285)
	ui.faderLayout(gtx, f, r, true, "")
}

// eqTxtBtn desenha botão de texto pequeno.
func (ui *uiState) eqTxtBtn(gtx layout.Context, b *uibtn, txt string, x, y int, fn func()) {
	r := image.Rect(x, y, x+24, y+10)
	if ui.button(gtx, b, r, func(gtx layout.Context, down bool) {
		c := color.NRGBA{R: 0x88, G: 0x88, B: 0x99, A: 0xFF}
		if down {
			c = color.NRGBA{R: 0x33, G: 0xFF, B: 0x66, A: 0xFF}
		}
		if ui.eqOpen { // usa como "ativo" quando EQ ligado? simplificação
		}
		fillRect(gtx.Ops, image.Rectangle{Max: r.Size()}, color.NRGBA{R: 0x18, G: 0x18, B: 0x28, A: 0xFF})
		m := op.Record(gtx.Ops)
		paint.ColorOp{Color: c}.Add(gtx.Ops)
		mat := m.Stop()
		off := op.Offset(image.Pt(2, 1)).Push(gtx.Ops)
		widget.Label{}.Layout(gtx, ui.shaper, ui.fontUI, unit.Sp(7), txt, mat)
		off.Pop()
	}) {
		fn()
	}
}
