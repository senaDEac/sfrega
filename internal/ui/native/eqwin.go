package native

import (
	"image"
	"image/color"
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
	"gioui.org/widget"
)

// Coordenadas canônicas da skin base-2.91 (EQMAIN.png, 275x315 = folha de sprites;
// a janela EQ usa só a região 0..115 como fundo, o resto são controles).
// Layout conforme webamp/css/equalizer-window.css.

var eqFreqLabels = [10]string{"31", "62", "125", "250", "500", "1k", "2k", "4k", "8k", "16k"}

var eqPresets = []struct {
	name  string
	gains [10]float64
}{
	{"Flat", [10]float64{}},
	{"Bass Boost", [10]float64{6, 5, 4, 2, 0, 0, 0, 0, 0, 0}},
	{"Rock", [10]float64{4, 3, 1, -1, -2, -1, 2, 4, 5, 4}},
	{"Pop", [10]float64{-2, -1, 0, 2, 4, 4, 2, 0, -1, -2}},
	{"Classical", [10]float64{3, 2, 0, -1, -2, -2, 0, 2, 3, 4}},
	{"Dance", [10]float64{5, 4, 1, -1, -2, -1, 2, 4, 5, 4}},
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
			app.Size(unit.Dp(275), unit.Dp(116)),
		)
		ui.winEQ = w
		ui.eqOpen = true
		go ui.eqLoop()
		go func() {
			time.Sleep(400 * time.Millisecond)
			wmNoDecorations()
		}()
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
			app.Size(unit.Dp(280), unit.Dp(240)),
		)
		ui.winPL = w
		ui.plOpen = true
		go ui.plLoop()
		go func() {
			time.Sleep(400 * time.Millisecond)
			wmNoDecorations()
		}()
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
			ui.eqPresOpen = false
			ui.mu.Unlock()
			return
		case app.FrameEvent:
			gtx := app.NewContext(&ops, e)
			gtx = beginScaledFrame(gtx, ui.eqScale, image.Pt(275, 116))
			ui.layoutEQ(gtx)
			e.Frame(gtx.Ops)
		}
	}
}

// applyEQ envia ganhos (dB) ao player; OFF zera tudo.
func (ui *uiState) applyEQ() {
	var gains [10]float64
	pre := 0.0
	if ui.eqOn {
		for i := range ui.eqSliders {
			gains[i] = float64((ui.eqSliders[i].val - 0.5) * 24)
		}
		pre = float64((ui.preampSlide.val - 0.5) * 24)
	}
	ui.p.SetEQ(gains, 0)
	ui.p.SetPreamp(pre)
}

// eqBtn4 desenha botão EQ de 4 estados (normal/pressed/selected/sel+pressed).
// srcs em EQMAIN: n, s, p, sp — todos w×h.
func (ui *uiState) eqBtn4(gtx layout.Context, b *uibtn, dst image.Rectangle, n, s, p, sp image.Rectangle, sel bool, fn func()) bool {
	clicked := ui.button(gtx, b, dst, func(gtx layout.Context, down bool) {
		src := n
		if sel {
			src = s
		}
		if down {
			if sel {
				src = sp
			} else {
				src = p
			}
		}
		ui.spr(gtx, "EQMAIN", src, image.Rectangle{Max: dst.Size()})
	})
	if clicked {
		fn()
	}
	return clicked
}

// layoutEQ desenha a janela do equalizador (275x116) com sprites EQMAIN.
func (ui *uiState) layoutEQ(gtx layout.Context) {
	R := func(x, y, w, h int) image.Rectangle { return image.Rect(x, y, x+w, y+h) }

	// fundo: EQ_WINDOW_BACKGROUND = EQMAIN(0,0,275,116)
	ui.spr(gtx, "EQMAIN", R(0, 0, 275, 116), R(0, 0, 275, 116))

	// título na barra superior
	ui.textAt(gtx, ui.fontUI, unit.Sp(8), "Equalizer",
		color.NRGBA{R: 0xCC, G: 0xCC, B: 0xCC, A: 0xFF}, image.Pt(20, 4))

	dragArea(gtx, R(16, 0, 44, 14))
	dragArea(gtx, R(208, 0, 36, 14))
	dragAreaX11(gtx, R(16, 0, 44, 14))
	dragAreaX11(gtx, R(208, 0, 36, 14))
	if ui.spriteBtn(gtx, &ui.btnEQMin, "TITLEBAR", R(244, 3, 9, 9), R(9, 0, 9, 9), R(9, 9, 9, 9)) {
		minimizeWindow(ui.winEQ)
	}
	if ui.spriteBtn(gtx, &ui.btnEQSize, "TITLEBAR", R(254, 3, 9, 9), R(0, 18, 9, 9), R(9, 18, 9, 9)) {
		toggleWindowScale(&ui.eqScale, ui.winEQ, unit.Dp(275), unit.Dp(116))
	}

	// close (EQ_CLOSE_BUTTON)
	if ui.spriteBtn(gtx, &ui.btnEQClose, "EQMAIN", R(264, 3, 9, 9), R(0, 116, 9, 9), R(0, 125, 9, 9)) {
		ui.winEQ.Perform(system.ActionClose)
	}

	// volume/balance compactos na barra de título (61,4,97x6) e (164,4,43x6)
	ui.eqMiniSlider(gtx, &ui.volDrag, 61, 4, 97)
	ui.eqMiniSlider(gtx, &ui.balDrag, 164, 4, 43)

	// ON / AUTO / PRESETS
	ui.eqBtn4(gtx, &ui.btnEQOn, R(14, 18, 26, 12),
		R(10, 119, 26, 12), R(69, 119, 26, 12), R(128, 119, 26, 12), R(187, 119, 26, 12),
		ui.eqOn, func() { ui.eqOn = !ui.eqOn; ui.applyEQ() })
	ui.eqBtn4(gtx, &ui.btnEQAuto, R(40, 18, 32, 12),
		R(36, 119, 32, 12), R(95, 119, 32, 12), R(154, 119, 32, 12), R(213, 119, 32, 12),
		ui.eqAuto, func() { ui.eqAuto = !ui.eqAuto })
	ui.eqBtn4(gtx, &ui.btnEQPres, R(217, 18, 44, 12),
		R(224, 164, 44, 12), R(224, 164, 44, 12), R(224, 176, 44, 12), R(224, 176, 44, 12),
		ui.eqPresOpen, func() { ui.eqPresOpen = !ui.eqPresOpen })

	// gráfico da curva EQ (86,17,113x19)
	ui.spr(gtx, "EQMAIN", R(0, 294, 113, 19), R(86, 17, 113, 19))
	ui.eqCurve(gtx)

	// rótulos de ganho (+12 / 0 / -12) à esquerda dos faders
	lbl := color.NRGBA{R: 0x99, G: 0x99, B: 0x99, A: 0xFF}
	for _, d := range []struct {
		t string
		y int
	}{{"+12", 36}, {"0", 65}, {"-12", 96}} {
		ui.textAt(gtx, ui.fontMono, unit.Sp(8), d.t, lbl, image.Pt(45, d.y))
	}

	// preamp (x=21) e 10 bandas (x=78+18i), faders y38..101
	ui.eqFader(gtx, &ui.preampSlide, 21)
	for i := 0; i < 10; i++ {
		ui.eqFader(gtx, &ui.eqSliders[i], 78+i*18)
		ui.textAt(gtx, ui.fontMono, unit.Sp(7), eqFreqLabels[i], lbl, image.Pt(78+i*18-4, 103))
	}

	// painel de presets (sobreposto quando aberto)
	if ui.eqPresOpen {
		ui.eqPresetPanel(gtx)
	}
}

// eqMiniSlider desenha slider horizontal fino (volume/balance do EQ).
func (ui *uiState) eqMiniSlider(gtx layout.Context, f *fader, x, y, w int) {
	r := image.Rect(x, y, x+w, y+6)
	fillRect(gtx.Ops, r, color.NRGBA{R: 0x10, G: 0x10, B: 0x18, A: 0xFF})
	cx := x + int(f.val*float32(w-3))
	fillRect(gtx.Ops, image.Rect(cx, y-1, cx+3, y+7), color.NRGBA{R: 0xCC, G: 0xCC, B: 0xDD, A: 0xFF})
	ui.scrub(gtx, f, r)
}

// eqFader desenha um fader vertical da skin (fundo em células + thumb).
// bx = borda esquerda do fader (14px), área y38..101.
func (ui *uiState) eqFader(gtx layout.Context, f *fader, bx int) {
	R := func(x, y, w, h int) image.Rectangle { return image.Rect(x, y, x+w, y+h) }

	// fundo: EQ_SLIDER_BACKGROUND = EQMAIN(13,164,209,129), grade 14x2 de células 15x65
	n := int(f.val*27 + 0.5)
	if n > 27 {
		n = 27
	}
	if n < 0 {
		n = 0
	}
	ui.spr(gtx, "EQMAIN",
		R(13+(n%14)*15, 164+(n/14)*65, 15, 65),
		R(bx, 38, 14, 63))

	// thumb 11x11: normal (0,164) / selecionado (0,176)
	ty := 38 + int((1-f.val)*52)
	ts := R(0, 164, 11, 11)
	if f.active {
		ts = R(0, 176, 11, 11)
	}
	ui.spr(gtx, "EQMAIN", ts, R(bx+1, ty, 11, 11))

	// input vertical
	r := image.Rect(bx-3, 36, bx+17, 104)
	cs := clip.Rect(r).Push(gtx.Ops)
	event.Op(gtx.Ops, &f.tag)
	cs.Pop()
	for {
		ev, ok := gtx.Event(pointer.Filter{
			Target: &f.tag,
			Kinds:  pointer.Press | pointer.Drag | pointer.Release | pointer.Cancel,
		})
		if !ok {
			break
		}
		e, ok := ev.(pointer.Event)
		if !ok {
			continue
		}
		switch e.Kind {
		case pointer.Press, pointer.Drag:
			f.active = true
			t := 1 - (float32(e.Position.Y)-38)/63
			if t < 0 {
				t = 0
			}
			if t > 1 {
				t = 1
			}
			f.val = t
			ui.applyEQ()
		case pointer.Release, pointer.Cancel:
			f.active = false
		}
	}
}

// eqCurve desenha a curva de resposta do EQ no retângulo do gráfico.
func (ui *uiState) eqCurve(gtx layout.Context) {
	if !ui.eqOn {
		return
	}
	r := image.Rect(88, 19, 197, 34)
	path := clip.Path{}
	path.Begin(gtx.Ops)
	path.MoveTo(f32.Pt(88, 26))
	for i := 0; i < 10; i++ {
		g := (ui.eqSliders[i].val - 0.5) * 24 // -12..+12
		y := 26 - int(g*0.9)
		if y < 20 {
			y = 20
		}
		if y > 33 {
			y = 33
		}
		path.LineTo(f32.Pt(float32(88+i*11), float32(y)))
	}
	path.LineTo(f32.Pt(197, 26))
	paint.FillShape(gtx.Ops, color.NRGBA{R: 0x33, G: 0xFF, B: 0x66, A: 0xFF},
		clip.Outline{Path: path.End()}.Op())
	_ = r
}

// eqPresetPanel desenha o menu de presets sobre a área dos faders.
func (ui *uiState) eqPresetPanel(gtx layout.Context) {
	panel := image.Rect(104, 34, 262, 108)
	fillRect(gtx.Ops, panel, color.NRGBA{R: 0x14, G: 0x14, B: 0x1C, A: 0xF0})
	bevel(gtx, panel, colBevelLight, colBevelDark)
	for i, pr := range eqPresets {
		by := 38 + i*11
		if by+10 > 106 {
			break
		}
		br := image.Rect(108, by, 258, by+10)
		if ui.button(gtx, &ui.eqPresetBtns[i], br, func(gtx layout.Context, down bool) {
			c := color.NRGBA{R: 0xCC, G: 0xCC, B: 0xCC, A: 0xFF}
			if down {
				c = color.NRGBA{R: 0x33, G: 0xFF, B: 0x66, A: 0xFF}
			}
			m := op.Record(gtx.Ops)
			paint.ColorOp{Color: c}.Add(gtx.Ops)
			mat := m.Stop()
			off := op.Offset(image.Pt(4, 1)).Push(gtx.Ops)
			ui.shapeMu.Lock()
			widget.Label{}.Layout(gtx, ui.shaper, ui.fontUI, unit.Sp(8), pr.name, mat)
			ui.shapeMu.Unlock()
			off.Pop()
		}) {
			for j := range ui.eqSliders {
				v := float32(pr.gains[j]/24 + 0.5)
				if v < 0 {
					v = 0
				}
				if v > 1 {
					v = 1
				}
				ui.eqSliders[j].val = v
			}
			ui.applyEQ()
		}
	}
}
