package native

import (
	"image"
	"image/color"
	"gioui.org/f32"
	
	"gioui.org/app"
	"gioui.org/io/system"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
)

var eqFreqLabels = [10]string{"31", "62", "125", "250", "500", "1k", "2k", "4k", "8k", "16k"}

var eqPresets = map[string][10]float64{
	"Flat":      {},
	"Bass Boost": {6, 5, 4, 2, 0, 0, 0, 0, 0, 0},
	"Rock":      {4, 3, 1, -1, -2, -1, 2, 4, 5, 4},
	"Pop":       {-2, -1, 0, 2, 4, 4, 2, 0, -1, -2},
	"Classical": {3, 2, 0, -1, -2, -2, 0, 2, 3, 4},
	"Dance":     {5, 4, 1, -1, -2, -1, 2, 4, 5, 4},
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
			app.Size(unit.Dp(320), unit.Dp(150)),
		)
		ui.winEQ = w
		go ui.eqLoop()
		return
	}
	ui.winEQ.Perform(system.ActionClose)
	// winEQ será nil quando DestroyEvent chegar
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
			app.Size(unit.Dp(380), unit.Dp(300)),
		)
		ui.winPL = w
		go ui.plLoop()
		return
	}
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

// layoutEQ desenha a janela do equalizador.
func (ui *uiState) layoutEQ(gtx layout.Context) {
	if ui.btnEQClose.Clicked(gtx) {
		ui.winEQ.Perform(system.ActionClose)
	}
	winW := gtx.Constraints.Max.X
	winH := gtx.Constraints.Max.Y

	// titlebar
	tb := image.Rect(0, 0, winW, 18)
	vgradient(gtx, tb, colTitlebarTop, colTitlebarBot)
	m := op.Record(gtx.Ops)
	paint.ColorOp{Color: colTitleText}.Add(gtx.Ops)
	tmat := m.Stop()
	off := op.Offset(image.Pt(8, 1)).Push(gtx.Ops)
	widget.Label{}.Layout(gtx, ui.shaper, ui.fontBold, unit.Sp(11), "Equalizador", tmat)
	off.Pop()
	// close X
	closeR := image.Rect(winW-20, 0, winW-2, 18)
	uioff := op.Offset(closeR.Min).Push(gtx.Ops)
	sz := closeR.Size()
	_ = ui.btnEQClose.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min = sz
		gtx.Constraints.Max = sz
		lr := image.Rectangle{Max: sz}
		if ui.btnEQClose.Hovered() {
			fillRect(gtx.Ops, lr, color.NRGBA{R: 0x80, G: 0, B: 0, A: 0xFF})
		}
		path := clip.Path{}
		path.MoveTo(f32.Pt(5, 5))
		path.LineTo(f32.Pt(11, 11))
		path.LineTo(f32.Pt(11, 12))
		path.LineTo(f32.Pt(5, 6))
		path.Close()
		path.MoveTo(f32.Pt(11, 5))
		path.LineTo(f32.Pt(5, 11))
		path.LineTo(f32.Pt(4, 11))
		path.LineTo(f32.Pt(10, 5))
		path.Close()
		paint.FillShape(gtx.Ops, colTitleText, clip.Outline{Path: path.End()}.Op())
		return layout.Dimensions{Size: sz}
	})
	uioff.Pop()

	// fundo
	body := image.Rect(0, 18, winW, winH)
	fillRect(gtx.Ops, body, colWindowBot)

	// fileira de presets (topo)
	presetNames := []string{"Flat", "Bass+", "Rock", "Pop", "Classic", "Dance"}
	px := 10
	for i, name := range presetNames {
		_ = i
		ui.presetBtn(gtx, name, px, 24, 44, 18, func() {
			preset, ok := eqPresets[presetKey(name)]
			if !ok {
				return
			}
			for b := 0; b < 10; b++ {
				db := preset[b]
				ui.eqSliders[b].val = float32(db/24 + 0.5)
			}
			ui.applyEQ()
			ui.mu.Lock()
			if ui.winEQ != nil {
				ui.winEQ.Invalidate()
			}
			ui.mu.Unlock()
		})
		px += 48
	}

	// preamp (fader vertical esquerdo)
	preR := image.Rect(14, 52, 30, 118)
	box(gtx, preR, colBtnBot, colBtnBot, colBevelDark, colBevelDk2)
	ui.faderLayout(gtx, &ui.preampSlide, preR, true, "")
	// label preamp
	m2 := op.Record(gtx.Ops)
	paint.ColorOp{Color: colTitleText}.Add(gtx.Ops)
	pmat := m2.Stop()
	poff := op.Offset(image.Pt(2, 124)).Push(gtx.Ops)
	widget.Label{}.Layout(gtx, ui.shaper, ui.fontUI, unit.Sp(8), "Preamp", pmat)
	poff.Pop()

	// 10 faders
	sliderArea := image.Rect(44, 50, winW-10, 118)
	bandW := sliderArea.Dx() / 10
	for i := 0; i < 10; i++ {
		x := sliderArea.Min.X + i*bandW + bandW/2 - 5
		fr := image.Rect(x, sliderArea.Min.Y, x+10, sliderArea.Max.Y)
		box(gtx, fr, colBtnBot, colBtnBot, colBevelDark, colBevelDk2)
		ui.faderLayout(gtx, &ui.eqSliders[i], fr, true, "")
		// label freq
		m3 := op.Record(gtx.Ops)
		paint.ColorOp{Color: colTitleText}.Add(gtx.Ops)
		fmat := m3.Stop()
		foff := op.Offset(image.Pt(x-6, sliderArea.Max.Y+4)).Push(gtx.Ops)
		widget.Label{}.Layout(gtx, ui.shaper, ui.fontUI, unit.Sp(8), eqFreqLabels[i], fmat)
		foff.Pop()
		// valor dB
		db := int((ui.eqSliders[i].val-0.5)*24 + (dbSign(ui.eqSliders[i].val-0.5) * 0))
		_ = db
		m4 := op.Record(gtx.Ops)
		paint.ColorOp{Color: colLCDTextDim}.Add(gtx.Ops)
		dmat := m4.Stop()
		dboff := op.Offset(image.Pt(x-4, sliderArea.Min.Y-14)).Push(gtx.Ops)
		widget.Label{}.Layout(gtx, ui.shaper, ui.fontMono, unit.Sp(8), eqDbText(ui.eqSliders[i].val), dmat)
		dboff.Pop()
	}
}

func dbSign(v float32) float32 {
	if v < 0 {
		return -1
	}
	return 1
}

func eqDbText(v float32) string {
	db := (v - 0.5) * 24
	s := ""
	if db < -0.5 {
		s = "-"
		db = -db
	} else if db > 0.5 {
		s = "+"
	}
	whole := int(db)
	frac := int(db*10+0.5) % 10
	return s + itoa(whole) + "." + itoa(frac)
}

func presetKey(name string) string {
	switch name {
	case "Flat":
		return "Flat"
	case "Bass+":
		return "Bass Boost"
	case "Rock":
		return "Rock"
	case "Pop":
		return "Pop"
	case "Classic":
		return "Classical"
	case "Dance":
		return "Dance"
	}
	return ""
}

var eqPresetBtns [6]widget.Clickable

// presetBtn desenha botão de preset.
func (ui *uiState) presetBtn(gtx layout.Context, name string, x, y, w, h int, fn func()) {
	// mapeia nome -> índice estável
	idx := 0
	for i, n := range []string{"Flat", "Bass+", "Rock", "Pop", "Classic", "Dance"} {
		if n == name {
			idx = i
		}
	}
	c := &eqPresetBtns[idx]
	off := op.Offset(image.Pt(x, y)).Push(gtx.Ops)
	defer off.Pop()
	sz := image.Point{X: w, Y: h}
	_ = c.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min = sz
		gtx.Constraints.Max = sz
		r := image.Rectangle{Max: sz}
		top, bot := colBtnTop, colBtnBot
		if c.Pressed() {
			top, bot = colBtnBot, colBtnTop
		}
		box(gtx, r, top, bot, colBevelLight, colBevelDark)
		m := op.Record(gtx.Ops)
		paint.ColorOp{Color: color.NRGBA{R: 0x20, G: 0x20, B: 0x20, A: 0xFF}}.Add(gtx.Ops)
		mat := m.Stop()
		widget.Label{Alignment: text.Middle}.Layout(gtx, ui.shaper, ui.fontUI, unit.Sp(8), name, mat)
		return layout.Dimensions{Size: sz}
	})
	if c.Clicked(gtx) {
		fn()
	}
}

