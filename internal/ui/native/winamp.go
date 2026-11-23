package native

import (
	"fmt"
	"image"
	"image/color"

	"gioui.org/io/system"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
)

// Coordenadas extraídas da skin base-2.91 (MAIN 275x116).

// layoutMain desenha a janela principal do Winamp usando sprites da skin.
func (ui *uiState) layoutMain(gtx layout.Context) {
	R := func(x, y, w, h int) image.Rectangle { return image.Rect(x, y, x+w, y+h) }

	ui.shadeMain.arm(shadePx(ui.mainScale, 22))
	if ui.shadeMain.on {
		ui.shadeBar(gtx, R(0, 0, 275, 22))
		return
	}

	ui.spr(gtx, "MAIN", R(0, 0, 275, 116), R(0, 0, 275, 116))

	// ---- titlebar ----
	ui.spr(gtx, "TITLEBAR", R(47, 0, 222, 11), R(20, 0, 222, 11)) // contend
	ui.spr(gtx, "TITLEBAR", R(0, 0, 9, 9), R(6, 3, 9, 9))         // option
	dragArea(gtx, R(16, 0, 228, 11))
	dragAreaX11(gtx, R(16, 0, 228, 11), &ui.shadeMain)
	if ui.spriteBtn(gtx, &ui.btnMin, "TITLEBAR", R(244, 3, 9, 9), R(9, 0, 9, 9), R(9, 9, 9, 9)) {
		minimizeWindow(ui.winMain)
	}
	if ui.spriteBtn(gtx, &ui.btnSize, "TITLEBAR", R(254, 3, 9, 9), R(0, 18, 9, 9), R(9, 18, 9, 9)) {
		toggleWindowScale(&ui.mainScale, ui.winMain, unit.Dp(275), unit.Dp(116))
	}
	if ui.spriteBtn(gtx, &ui.btnClose, "TITLEBAR", R(264, 3, 9, 9), R(18, 0, 9, 9), R(18, 9, 9, 9)) {
		ui.winMain.Perform(system.ActionClose)
	}

	// ---- clutter bar (decorativo) ----
	ui.spr(gtx, "TITLEBAR", R(304, 0, 8, 8), R(10, 22, 8, 8))
	ui.spr(gtx, "TITLEBAR", R(304, 8, 8, 7), R(10, 30, 8, 7))
	ui.spr(gtx, "TITLEBAR", R(304, 15, 8, 7), R(10, 37, 8, 7))
	ui.spr(gtx, "TITLEBAR", R(304, 22, 8, 8), R(10, 44, 8, 8))

	ui.lcdContent(gtx)
	ui.spectrumDraw(gtx, R(12, 40, 92, 21))
	ui.infoRight(gtx)
	ui.volBal(gtx, R)
	ui.eqplBtns(gtx, R)
	ui.posBar(gtx, R)
	ui.transport(gtx, R)
	ui.shuffleRepeat(gtx, R)
}

// shadeBar desenha o "window shade" com todos os controles essenciais:
// transporte, título da faixa, tempo, volume, EQ/PL — estilo Winamp 5.
func (ui *uiState) shadeBar(gtx layout.Context, r image.Rectangle) {
	R := func(x, y, w, h int) image.Rectangle { return image.Rect(x, y, x+w, y+h) }

	bg := color.NRGBA{R: 0x2D, G: 0x32, B: 0x42, A: 0xFF}
	fillRect(gtx.Ops, r, bg)
	fillRect(gtx.Ops, image.Rect(r.Min.X, r.Min.Y, r.Max.X, r.Min.Y+1),
		color.NRGBA{R: 0x5A, G: 0x62, B: 0x78, A: 0xFF})
	fillRect(gtx.Ops, image.Rect(r.Min.X, r.Max.Y-1, r.Max.X, r.Max.Y),
		color.NRGBA{R: 0x14, G: 0x17, B: 0x22, A: 0xFF})

	dragArea(gtx, R(0, 0, 200, 22))
	dragAreaX11(gtx, R(0, 0, 200, 22), &ui.shadeMain)

	// ---- transporte compacto (CBUTTONS escalado p/ 14px) ----
	type tb struct {
		b   *uibtn
		x   int
		col int
		fn  func()
	}
	btns := []tb{
		{&ui.btnPrev, 4, 0, func() { ui.p.Prev(); ui.p.Play() }},
		{&ui.btnStop, 48, 69, func() { ui.p.Pause(); _ = ui.p.SeekFraction(0) }},
		{&ui.btnNext, 64, 92, func() { ui.p.Next(); ui.p.Play() }},
	}
	for _, b := range btns {
		if ui.spriteBtn(gtx, b.b, "CBUTTONS", R(b.x, 4, 14, 14),
			R(b.col, 0, 23, 18), R(b.col, 18, 23, 18)) {
			b.fn()
		}
	}
	// play/pause combinado em x=32
	playCol := 23
	if ui.p.Playing() {
		playCol = 46
	}
	if ui.spriteBtn(gtx, &ui.btnPlay, "CBUTTONS", R(32, 4, 14, 14),
		R(playCol, 0, 23, 18), R(playCol, 18, 23, 18)) {
		if ui.p.Playing() {
			ui.p.Pause()
		} else {
			ui.p.Play()
		}
	}

	// ---- título da faixa ----
	title := ui.p.TrackAt(ui.p.Index())
	if title == "" {
		title = "Winamp"
	}
	for i := len(title) - 1; i > 0; i-- {
		if title[i] == '/' || title[i] == '\\' {
			title = title[i+1:]
			break
		}
	}
	cs := clip.Rect(R(82, 2, 92, 18)).Push(gtx.Ops)
	ui.textAt(gtx, ui.fontMono, unit.Sp(8), title,
		color.NRGBA{R: 0xC8, G: 0xCE, B: 0xDC, A: 0xFF}, image.Pt(82, 7))
	cs.Pop()

	// ---- tempo decorrido ----
	s := int(ui.p.Position().Seconds())
	if s < 0 {
		s = 0
	}
	ui.textAt(gtx, ui.fontMono, unit.Sp(8),
		fmt.Sprintf("%d:%02d", s/60, s%60),
		color.NRGBA{R: 0x86, G: 0x8E, B: 0xA0, A: 0xFF}, image.Pt(176, 7))

	// ---- volume (mini slider) ----
	volR := R(196, 8, 34, 6)
	fillRect(gtx.Ops, volR, color.NRGBA{R: 0x18, G: 0x1C, B: 0x28, A: 0xFF})
	vw := int(ui.volDrag.val * 34)
	if vw > 0 {
		fillRect(gtx.Ops, image.Rect(volR.Min.X, volR.Min.Y, volR.Min.X+vw, volR.Max.Y),
			color.NRGBA{R: 0x5E, G: 0xC8, B: 0x50, A: 0xFF})
	}
	ui.scrub(gtx, &ui.volDrag, R(196, 6, 34, 10))

	// ---- EQ / PL ----
	ui.eqplBtn(gtx, &ui.btnEQ, "SHUFREP", R(234, 5, 16, 11), ui.eqOpen,
		R(0, 62, 23, 11), R(46, 61, 23, 11), R(0, 73, 23, 11), R(46, 72, 23, 11),
		func() { ui.toggleEQ() })
	ui.eqplBtn(gtx, &ui.btnPL, "SHUFREP", R(252, 5, 16, 11), ui.plOpen,
		R(23, 62, 23, 11), R(69, 61, 23, 11), R(23, 73, 23, 11), R(69, 72, 23, 11),
		func() { ui.togglePL() })
}

// lcdContent desenha dígitos do tempo, título rolante e indicador play.
func (ui *uiState) lcdContent(gtx layout.Context) {
	R := func(x, y, w, h int) image.Rectangle { return image.Rect(x, y, x+w, y+h) }

	s := int(ui.p.Position().Seconds())
	if s < 0 {
		s = 0
	}
	mm, ss := (s/60)%100, s%60
	digits := []int{mm / 10, mm % 10, ss / 10, ss % 10}
	xs := []int{39, 51, 69, 81}
	for i, d := range digits {
		ui.spr(gtx, "NUMBERS", R(d*9, 0, 9, 13), R(xs[i], 26, 9, 13))
	}
	green := color.NRGBA{R: 0, G: 0xFF, B: 0, A: 0xFF}
	fillRect(gtx.Ops, R(63, 29, 1, 2), green)
	fillRect(gtx.Ops, R(63, 32, 1, 2), green)
	fillRect(gtx.Ops, R(64, 29, 1, 2), green)
	fillRect(gtx.Ops, R(64, 32, 1, 2), green)

	// título com marquee em (111,24,154,12)
	title := ui.p.TrackAt(ui.p.Index())
	if title == "" {
		title = "sfrega"
	}
	for i := len(title) - 1; i > 0; i-- {
		if title[i] == '/' || title[i] == '\\' {
			title = title[i+1:]
			break
		}
	}
	viewW := 154
	textW := len(title) * 5
	if textW > viewW {
		ui.marqueeX--
		if -ui.marqueeX > textW-viewW+30 {
			ui.marqueeX = 0
		}
	} else {
		ui.marqueeX = 0
	}
	cs := clip.Rect(R(111, 24, viewW, 12)).Push(gtx.Ops)
	off := op.Offset(image.Pt(111+ui.marqueeX, 26)).Push(gtx.Ops)
	ui.shapeMu.Lock()
	widget.Label{MaxLines: 1}.Layout(gtx, ui.shaper, ui.fontMono, unit.Sp(8), title, ui.txtMat(gtx, green))
	ui.shapeMu.Unlock()
	off.Pop()
	cs.Pop()

	// indicador play/pause/stop em (26,28)
	state := 2
	if ui.p.Playing() {
		state = 0
	} else if ui.p.Position() > 0 {
		state = 1
	}
	ui.spr(gtx, "PLAYPAUS", R(state*9, 0, 9, 9), R(26, 28, 9, 9))
	if ui.p.Playing() {
		ui.spr(gtx, "PLAYPAUS", R(39, 0, 3, 9), R(24, 28, 3, 9))
	}
}

// txtMat retorna a matriz de material com cor para texto.
func (ui *uiState) txtMat(gtx layout.Context, c color.NRGBA) op.CallOp {
	m := op.Record(gtx.Ops)
	paint.ColorOp{Color: c}.Add(gtx.Ops)
	return m.Stop()
}

// infoRight desenha indicador mono/stereo.
func (ui *uiState) infoRight(gtx layout.Context) {
	R := func(x, y, w, h int) image.Rectangle { return image.Rect(x, y, x+w, y+h) }
	channels := ui.p.Channels()
	monoSrc, stSrc := R(29, 12, 27, 12), R(0, 12, 29, 12)
	if channels == 1 {
		monoSrc = R(29, 0, 27, 12)
	} else if channels > 1 {
		stSrc = R(0, 0, 29, 12)
	}
	ui.spr(gtx, "MONOSTER", monoSrc, R(212, 37, 27, 12))
	ui.spr(gtx, "MONOSTER", stSrc, R(239, 37, 29, 12))
}

// volBal desenha sliders de volume e balance com thumbs da skin.
func (ui *uiState) volBal(gtx layout.Context, R func(int, int, int, int) image.Rectangle) {
	ui.spr(gtx, "VOLUME", R(0, 0, 68, 13), R(107, 52, 68, 13))
	ui.spr(gtx, "VOLUME", R(15, 422, 14, 11), R(107+int(ui.volDrag.val*54), 53, 14, 11))
	ui.scrub(gtx, &ui.volDrag, R(107, 52, 68, 13))

	ui.spr(gtx, "BALANCE", R(9, 0, 38, 13), R(177, 52, 38, 13))
	ui.spr(gtx, "BALANCE", R(15, 422, 14, 11), R(177+int(ui.balDrag.val*24), 53, 14, 11))
	ui.scrub(gtx, &ui.balDrag, R(177, 52, 38, 13))
}

// eqplBtns desenha botões EQ e PL (SHUFREP.png, 4 estados).
func (ui *uiState) eqplBtns(gtx layout.Context, R func(int, int, int, int) image.Rectangle) {
	ui.eqplBtn(gtx, &ui.btnEQ, "SHUFREP", R(219, 49, 23, 11), ui.eqOpen,
		R(0, 62, 23, 11), R(46, 61, 23, 11), R(0, 73, 23, 11), R(46, 72, 23, 11),
		func() { ui.toggleEQ() })
	ui.eqplBtn(gtx, &ui.btnPL, "SHUFREP", R(242, 49, 23, 11), ui.plOpen,
		R(23, 62, 23, 11), R(69, 61, 23, 11), R(23, 73, 23, 11), R(69, 72, 23, 11),
		func() { ui.togglePL() })
}

// eqplBtn desenha botão de 4 estados: normal/pressed/selected/sel+pressed.
func (ui *uiState) eqplBtn(gtx layout.Context, b *uibtn, name string, dst image.Rectangle, open bool, n, p, s, sp image.Rectangle, fn func()) bool {
	clicked := ui.button(gtx, b, dst, func(gtx layout.Context, down bool) {
		src := n
		if open {
			src = s
		}
		if down {
			if open {
				src = sp
			} else {
				src = p
			}
		}
		ui.spr(gtx, name, src, image.Rectangle{Max: dst.Size()})
	})
	if clicked {
		fn()
	}
	return clicked
}

// posBar desenha a barra de posição/seek (POSBAR.png).
func (ui *uiState) posBar(gtx layout.Context, R func(int, int, int, int) image.Rectangle) {
	ui.spr(gtx, "POSBAR", R(0, 0, 248, 10), R(16, 64, 248, 10))
	pos, dur := ui.p.Position(), ui.p.Duration()
	frac := 0.0
	if dur > 0 {
		frac = pos.Seconds() / dur.Seconds()
	}
	if frac < 0 {
		frac = 0
	}
	if frac > 1 {
		frac = 1
	}
	tx := 16 + int(frac*219)
	ui.spr(gtx, "POSBAR", R(248, 0, 29, 10), R(tx, 63, 29, 10))
	ui.posDrag.val = float32(frac)
	ui.scrub(gtx, &ui.posDrag, R(16, 64, 248, 10))
}

// transport desenha prev/play/pause/stop/next/eject (CBUTTONS.png).
func (ui *uiState) transport(gtx layout.Context, R func(int, int, int, int) image.Rectangle) {
	type tb struct {
		b   *uibtn
		x   int
		col int
		w   int
		fn  func()
	}
	btns := []tb{
		{&ui.btnPrev, 16, 0, 23, func() { ui.p.Prev(); ui.p.Play() }},
		{&ui.btnPlay, 39, 23, 23, func() { ui.p.Play() }},
		{&ui.btnPause, 62, 46, 23, func() { ui.p.Pause() }},
		{&ui.btnStop, 85, 69, 23, func() { ui.p.Pause(); _ = ui.p.SeekFraction(0) }},
		{&ui.btnNext, 108, 92, 22, func() { ui.p.Next(); ui.p.Play() }},
	}
	for _, b := range btns {
		if ui.spriteBtn(gtx, b.b, "CBUTTONS", R(b.x, 88, b.w, 18),
			R(b.col, 0, b.w, 18), R(b.col, 18, b.w, 18)) {
			b.fn()
		}
	}
	if ui.spriteBtn(gtx, &ui.btnEject, "CBUTTONS", R(136, 89, 22, 16),
		R(114, 0, 22, 16), R(114, 16, 22, 16)) {
		ui.p.Pause()
		_ = ui.p.SeekFraction(0)
	}
}

// shuffleRepeat desenha botões repeat/shuffle (SHUFREP.png).
func (ui *uiState) shuffleRepeat(gtx layout.Context, R func(int, int, int, int) image.Rectangle) {
	// repeat em x=0 (28px), shuffle em x=28 (47px); y: 0 norm,15 pressed,30 sel,45 sel+pressed
	if ui.eqplBtn(gtx, &ui.btnShuf, "SHUFREP", R(164, 88, 28, 15), ui.repeat,
		R(0, 0, 28, 15), R(0, 15, 28, 15), R(0, 30, 28, 15), R(0, 45, 28, 15),
		func() { ui.repeat = !ui.repeat }) {
	}
	if ui.eqplBtn(gtx, &ui.btnRep, "SHUFREP", R(210, 88, 47, 15), ui.shuffle,
		R(28, 0, 47, 15), R(28, 15, 47, 15), R(28, 30, 47, 15), R(28, 45, 47, 15),
		func() { ui.shuffle = !ui.shuffle }) {
	}
}
