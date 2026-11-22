package native

import (
	"image"
	"image/color"

	"gioui.org/f32"
	"gioui.org/io/system"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
)

// layoutMain desenha a janela principal do Winamp.
func (ui *uiState) layoutMain(gtx layout.Context) {
	winW := gtx.Constraints.Max.X

	// ---- titlebar (18px) ----
	tb := image.Rect(0, 0, winW, 18)
	vgradient(gtx, tb, colTitlebarTop, colTitlebarBot)
	fillRect(gtx.Ops, image.Rect(0, 17, winW, 18), colBevelDk2)
	m := op.Record(gtx.Ops)
	paint.ColorOp{Color: colTitleText}.Add(gtx.Ops)
	tmat := m.Stop()
	off := op.Offset(image.Pt(8, 1)).Push(gtx.Ops)
	widget.Label{}.Layout(gtx, ui.shaper, ui.fontBold, unit.Sp(11), "Winamp", tmat)
	off.Pop()
	// botão fechar da titlebar
	if ui.titleClose(gtx, image.Rect(winW-20, 0, winW-2, 18)) {
		ui.winMain.Perform(system.ActionClose)
	}

	// ---- LCD (y=20..52) ----
	lcd := image.Rect(6, 20, winW-6, 52)
	lcdBox(gtx, lcd)
	ui.lcdContent(gtx, lcd)

	// ---- spectrum (y=54..78) ----
	spec := image.Rect(6, 54, winW-6, 78)
	box(gtx, spec, colSpecBg, colSpecBg, color.NRGBA{A: 0}, color.NRGBA{A: 0})
	ui.spectrumDraw(gtx, spec)

	// ---- controles (y=80..112) ----
	ui.controlsRow(gtx, winW)
}

// titleClose desenha o X da barra de título.
func (ui *uiState) titleClose(gtx layout.Context, r image.Rectangle) bool {
	return ui.button(gtx, &ui.btnClose, r, func(gtx layout.Context, down bool) {
		if down {
			fillRect(gtx.Ops, image.Rectangle{Max: r.Size()}, color.NRGBA{R: 0x80, G: 0, B: 0, A: 0xFF})
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
	})
}

// lcdContent desenha tempo + título com marquee dentro do LCD.
func (ui *uiState) lcdContent(gtx layout.Context, lcd image.Rectangle) {
	inner := image.Rect(lcd.Min.X+4, lcd.Min.Y+3, lcd.Max.X-4, lcd.Max.Y-3)
	clipStack := clip.Rect(inner).Push(gtx.Ops)
	defer clipStack.Pop()

	pos := ui.p.Position()
	dur := ui.p.Duration()
	m := op.Record(gtx.Ops)
	paint.ColorOp{Color: colLCDText}.Add(gtx.Ops)
	tmat := m.Stop()
	left := op.Offset(image.Pt(inner.Min.X, inner.Min.Y+2)).Push(gtx.Ops)
	widget.Label{}.Layout(gtx, ui.shaper, ui.fontMono, unit.Sp(12), formatDur(pos.Seconds()), tmat)
	left.Pop()

	right := op.Offset(image.Pt(inner.Max.X-40, inner.Min.Y+2)).Push(gtx.Ops)
	widget.Label{}.Layout(gtx, ui.shaper, ui.fontMono, unit.Sp(12), formatDur(dur.Seconds()), tmat)
	right.Pop()

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
	textW := len(title) * 7
	viewW := inner.Dx() - 90
	if textW > viewW {
		ui.marqueeX -= 2
		span := textW - viewW + 40
		if -ui.marqueeX > span {
			ui.marqueeX = 0
		}
	} else {
		ui.marqueeX = 0
	}
	cx := inner.Min.X + (inner.Dx()-min(textW, viewW))/2 + ui.marqueeX
	m2 := op.Record(gtx.Ops)
	paint.ColorOp{Color: colLCDTextDim}.Add(gtx.Ops)
	tmat2 := m2.Stop()
	toff := op.Offset(image.Pt(cx, inner.Min.Y+2)).Push(gtx.Ops)
	widget.Label{MaxLines: 1}.Layout(gtx, ui.shaper, ui.fontMono, unit.Sp(12), title, tmat2)
	toff.Pop()
}

// controlsRow desenha a fileira de transporte + volume/balance + EQ/PL.
func (ui *uiState) controlsRow(gtx layout.Context, winW int) {
	y := 80
	btnY := y + 3
	btnH := 24
	btnW := 26
	gap := 4
	x := 10

	if ui.transportBtn(gtx, &ui.btnPrev, x, btnY, btnW, btnH, glyphPrev) {
		ui.p.Prev()
		ui.p.Play()
	}
	x += btnW + gap
	if ui.transportBtn(gtx, &ui.btnPlay, x, btnY, btnW, btnH, glyphPlay) {
		ui.p.Play()
	}
	x += btnW + gap
	if ui.transportBtn(gtx, &ui.btnPause, x, btnY, btnW, btnH, glyphPause) {
		ui.p.Pause()
	}
	x += btnW + gap
	if ui.transportBtn(gtx, &ui.btnStop, x, btnY, btnW, btnH, glyphStop) {
		ui.p.Pause()
		ui.p.SeekFrame(0)
	}
	x += btnW + gap
	if ui.transportBtn(gtx, &ui.btnNext, x, btnY, btnW, btnH, glyphNext) {
		ui.p.Next()
		ui.p.Play()
	}
	x += btnW + gap
	if ui.transportBtn(gtx, &ui.btnEject, x, btnY, btnW, btnH, glyphEject) {
		ui.p.Pause()
		ui.p.SeekFrame(0)
	}
	x += btnW + gap
	if ui.transportBtn(gtx, &ui.btnOpen, x, btnY, btnW, btnH, glyphOpen) {
		// TODO: seletor de arquivos/pasta
	}

	// EQ / PL à direita
	eqX := winW - 96
	plX := winW - 48
	if ui.textBtn(gtx, &ui.btnEQ, eqX, btnY, 42, btnH, "EQ") {
		ui.toggleEQ()
	}
	if ui.textBtn(gtx, &ui.btnPL, plX, btnY, 42, btnH, "PL") {
		ui.togglePL()
	}

	// volume + balance
	volY := y + 14
	volX := 240
	volW := 200
	m := op.Record(gtx.Ops)
	paint.ColorOp{Color: colTitleText}.Add(gtx.Ops)
	vmat := m.Stop()
	voff := op.Offset(image.Pt(volX, volY+2)).Push(gtx.Ops)
	widget.Label{}.Layout(gtx, ui.shaper, ui.fontUI, unit.Sp(9), "VOL", vmat)
	voff.Pop()
	vr := image.Rect(volX+34, volY+4, volX+volW, volY+12)
	box(gtx, vr, colBtnBot, colBtnBot, colBevelDark, colBevelDk2)
	ui.faderLayout(gtx, &ui.volSlider, vr, false, "")
	// mute
	muteR := image.Rect(volX+volW+6, volY, volX+volW+30, volY+16)
	if ui.miniBtn(gtx, &ui.btnMute, muteR, "M") {
		if ui.p.Volume() > 0 {
			ui.volSlider.val = 0
			ui.p.SetVolume(0)
		} else {
			ui.volSlider.val = 1
			ui.p.SetVolume(1)
		}
	}
	// balance
	balX := volX + volW + 40
	m2 := op.Record(gtx.Ops)
	paint.ColorOp{Color: colTitleText}.Add(gtx.Ops)
	bmat := m2.Stop()
	boff := op.Offset(image.Pt(balX, volY+2)).Push(gtx.Ops)
	widget.Label{}.Layout(gtx, ui.shaper, ui.fontUI, unit.Sp(9), "BAL", bmat)
	boff.Pop()
	br := image.Rect(balX+34, volY+4, balX+110, volY+12)
	box(gtx, br, colBtnBot, colBtnBot, colBevelDark, colBevelDk2)
	ui.faderLayout(gtx, &ui.balSlider, br, false, "")
}

// transportBtn desenha botão de transporte com glifo vetorial.
func (ui *uiState) transportBtn(gtx layout.Context, b *uibtn, x, y, w, h int, g glyph) bool {
	r := image.Rect(x, y, x+w, y+h)
	return ui.button(gtx, b, r, func(gtx layout.Context, down bool) {
		top, bot := colBtnTop, colBtnBot
		if down {
			top, bot = colBtnBot, colBtnTop
		}
		lr := image.Rectangle{Max: r.Size()}
		box(gtx, lr, top, bot, colBevelLight, colBevelDark)
		drawGlyph(gtx, g, lr)
	})
}

// textBtn desenha botão com texto.
func (ui *uiState) textBtn(gtx layout.Context, b *uibtn, x, y, w, h int, txt string) bool {
	r := image.Rect(x, y, x+w, y+h)
	return ui.button(gtx, b, r, func(gtx layout.Context, down bool) {
		top, bot := colBtnTop, colBtnBot
		if down {
			top, bot = colBtnBot, colBtnTop
		}
		lr := image.Rectangle{Max: r.Size()}
		box(gtx, lr, top, bot, colBevelLight, colBevelDark)
		m := op.Record(gtx.Ops)
		paint.ColorOp{Color: color.NRGBA{R: 0x20, G: 0x20, B: 0x20, A: 0xFF}}.Add(gtx.Ops)
		mat := m.Stop()
		// centraliza texto
		off2 := op.Offset(image.Pt(0, 4)).Push(gtx.Ops)
		widget.Label{Alignment: text.Middle}.Layout(gtx, ui.shaper, ui.fontBold, unit.Sp(9), txt, mat)
		off2.Pop()
	})
}

// miniBtn botão pequeno quadrado.
func (ui *uiState) miniBtn(gtx layout.Context, b *uibtn, r image.Rectangle, txt string) bool {
	return ui.button(gtx, b, r, func(gtx layout.Context, down bool) {
		top, bot := colBtnTop, colBtnBot
		if down {
			top, bot = colBtnBot, colBtnTop
		}
		lr := image.Rectangle{Max: r.Size()}
		box(gtx, lr, top, bot, colBevelLight, colBevelDark)
		m := op.Record(gtx.Ops)
		paint.ColorOp{Color: color.NRGBA{R: 0x20, G: 0x20, B: 0x20, A: 0xFF}}.Add(gtx.Ops)
		mat := m.Stop()
		off2 := op.Offset(image.Pt(0, 2)).Push(gtx.Ops)
		widget.Label{Alignment: text.Middle}.Layout(gtx, ui.shaper, ui.fontUI, unit.Sp(9), txt, mat)
		off2.Pop()
	})
}

// lcdBox desenha o display LCD.
func lcdBox(gtx layout.Context, r image.Rectangle) {
	inner := image.Rect(r.Min.X+1, r.Min.Y+1, r.Max.X-1, r.Max.Y-1)
	vgradient(gtx, inner, colLCDTop, colLCDBot)
	bevel(gtx, r, color.NRGBA{R: 0x55, G: 0x55, B: 0x55, A: 0xFF}, color.NRGBA{A: 0})
}

// glyph identifica o símbolo do botão.
type glyph int

const (
	glyphPrev glyph = iota
	glyphPlay
	glyphPause
	glyphStop
	glyphNext
	glyphEject
	glyphOpen
)

// drawGlyph desenha o símbolo vetorial do botão.
func drawGlyph(gtx layout.Context, g glyph, r image.Rectangle) {
	ink := color.NRGBA{R: 0x18, G: 0x18, B: 0x18, A: 0xFF}
	cx, cy := r.Dx()/2, r.Dy()/2
	switch g {
	case glyphPlay:
		path := clip.Path{}
		path.MoveTo(f32.Pt(float32(cx-4), float32(cy-6)))
		path.LineTo(f32.Pt(float32(cx+6), float32(cy)))
		path.LineTo(f32.Pt(float32(cx-4), float32(cy+6)))
		path.Close()
		paint.FillShape(gtx.Ops, ink, clip.Outline{Path: path.End()}.Op())
	case glyphPause:
		fillRect(gtx.Ops, image.Rect(cx-5, cy-6, cx-2, cy+6), ink)
		fillRect(gtx.Ops, image.Rect(cx+1, cy-6, cx+4, cy+6), ink)
	case glyphStop:
		fillRect(gtx.Ops, image.Rect(cx-5, cy-5, cx+5, cy+5), ink)
	case glyphPrev:
		fillRect(gtx.Ops, image.Rect(cx-6, cy-5, cx-4, cy+5), ink)
		path := clip.Path{}
		path.MoveTo(f32.Pt(float32(cx+5), float32(cy-6)))
		path.LineTo(f32.Pt(float32(cx-4), float32(cy)))
		path.LineTo(f32.Pt(float32(cx+5), float32(cy+6)))
		path.Close()
		paint.FillShape(gtx.Ops, ink, clip.Outline{Path: path.End()}.Op())
	case glyphNext:
		path := clip.Path{}
		path.MoveTo(f32.Pt(float32(cx-5), float32(cy-6)))
		path.LineTo(f32.Pt(float32(cx+4), float32(cy)))
		path.LineTo(f32.Pt(float32(cx-5), float32(cy+6)))
		path.Close()
		paint.FillShape(gtx.Ops, ink, clip.Outline{Path: path.End()}.Op())
		fillRect(gtx.Ops, image.Rect(cx+4, cy-5, cx+6, cy+5), ink)
	case glyphEject:
		fillRect(gtx.Ops, image.Rect(cx-8, cy+5, cx+8, cy+6), ink)
		path := clip.Path{}
		path.MoveTo(f32.Pt(float32(cx-5), float32(cy-4)))
		path.LineTo(f32.Pt(float32(cx+5), float32(cy-4)))
		path.LineTo(f32.Pt(float32(cx), float32(cy+4)))
		path.Close()
		paint.FillShape(gtx.Ops, ink, clip.Outline{Path: path.End()}.Op())
	case glyphOpen:
		fillRect(gtx.Ops, image.Rect(cx-7, cy-3, cx+7, cy+5), ink)
		fillRect(gtx.Ops, image.Rect(cx-7, cy-5, cx-2, cy-3), ink)
	}
}

func formatDur(sec float64) string {
	s := int(sec)
	if s < 0 {
		s = 0
	}
	h := s / 3600
	m := (s % 3600) / 60
	s = s % 60
	if h > 0 {
		return itoa(h) + ":" + pad2(m) + ":" + pad2(s)
	}
	return itoa(m) + ":" + pad2(s)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [8]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

func pad2(n int) string {
	if n < 10 {
		return "0" + itoa(n)
	}
	return itoa(n)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
