package native

import (
	"image"
	"image/color"
	"os"
	"path/filepath"

	"gioui.org/app"
	"gioui.org/f32"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
)

// ovlFlagPath é o arquivo-sinal lido pelo OverlayService (Java): fica no
// filesDir (UserHomeDir no Android) — o CWD do processo é "/", então o
// caminho precisa ser absoluto.
func ovlFlagPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "/tmp"
	}
	return filepath.Join(home, "ovl.flag")
}

// toggleOverlay alterna a flag que controla o overlay: o ticker sincroniza
// ovlWin com a existência do arquivo, e o serviço Java cria/remove a view.
// Não há Perform(ActionClose) no Android.
func (ui *uiState) toggleOverlay() {
	if !androidUI() {
		return
	}
	ui.mu.Lock()
	open := ui.ovlWin != nil
	ui.mu.Unlock()
	if open {
		_ = os.Remove(ovlFlagPath())
		return
	}
	_ = os.WriteFile(ovlFlagPath(), []byte("1"), 0o644)
}

// syncOverlay alinha ovlWin com a flag ovl.flag (chamado com ui.mu preso).
func (ui *uiState) syncOverlay() {
	_, err := os.Stat(ovlFlagPath())
	want := err == nil
	if want && ui.ovlWin == nil {
		w := new(app.Window)
		w.Option(
			app.Title("sfrega"),
			app.Decorated(false),
			app.Size(unit.Dp(320), unit.Dp(96)),
		)
		ui.ovlWin = w
		go ui.ovlLoop(w)
	} else if !want && ui.ovlWin != nil {
		ui.ovlWin = nil
	}
}

// ovlLoop é o loop de eventos da janela overlay.
func (ui *uiState) ovlLoop(w *app.Window) {
	var ops op.Ops
	for {
		switch e := w.Event().(type) {
		case app.DestroyEvent:
			return
		case app.FrameEvent:
			gtx := app.NewContext(&ops, e)
			ui.layoutOvl(gtx, e)
			e.Frame(gtx.Ops)
		}
	}
}

// layoutOvl desenha o mini-player: faixa de arrasto no topo (30px físicos
// reservados — o Java move a janela quando o toque começa nela) e a barra
// de transporte 275x18 escalada para a largura toda.
func (ui *uiState) layoutOvl(gtx layout.Context, e app.FrameEvent) {
	W, H := e.Size.X, e.Size.Y
	s := float32(W) / 275
	if s < 0.5 {
		s = 0.5
	}
	lw := int(float32(W) / s)
	lh := int(float32(H) / s)

	// overlay é uma janela TRANSLUCENT real: dá para ver o app atrás —
	// fundo totalmente transparente, só os sprites desenham conteúdo
	paint.FillShape(gtx.Ops, color.NRGBA{R: 0x14, G: 0x14, B: 0x18, A: 0x00},
		clip.Rect(image.Rect(0, 0, lw, lh)).Op())

	op.Affine(f32.Affine2D{}.Scale(f32.Pt(0, 0), f32.Pt(s, s))).Add(gtx.Ops)

	// faixa de arrasto: 30px físicos => 30/s lógicos
	strip := int(float32(30) / s)
	off := op.Offset(image.Pt(0, strip)).Push(gtx.Ops)
	ui.ovlBar(gtx, image.Rect(0, 0, 275, 18))
	off.Pop()
}

// ovlBar é a versão compacta da shadeBar para o overlay: transporte,
// VU, volume e fechar — sem EQ/PL/menu (painéis ficam no app principal).
func (ui *uiState) ovlBar(gtx layout.Context, r image.Rectangle) {
	R := func(x, y, w, h int) image.Rectangle { return image.Rect(x, y, x+w, y+h) }

	ui.spr(gtx, "MAIN", R(0, 0, 275, 12), r)

	type tb struct {
		b   *uibtn
		x   int
		col int
		w   int
		fn  func()
	}
	btns := []tb{
		{&ui.btnPrev, 64, 0, 23, func() { ui.p.Prev(); ui.p.Play() }},
		{&ui.btnPlay, 77, 23, 23, func() { ui.p.Play() }},
		{&ui.btnPause, 90, 46, 23, func() { ui.p.Pause() }},
		{&ui.btnStop, 103, 69, 23, func() { ui.p.Pause(); _ = ui.p.SeekFraction(0) }},
		{&ui.btnNext, 116, 92, 22, func() { ui.p.Next(); ui.p.Play() }},
	}
	for _, b := range btns {
		if ui.spriteBtn(gtx, b.b, "CBUTTONS", R(b.x, 3, 12, 12),
			R(b.col, 0, b.w, 18), R(b.col, 18, b.w, 18)) {
			b.fn()
		}
	}
	if ui.spriteBtn(gtx, &ui.btnEject, "CBUTTONS", R(135, 3, 12, 12),
		R(114, 0, 22, 16), R(114, 16, 22, 16)) {
		ui.eject()
	}

	ui.spectrumDraw(gtx, R(149, 1, 36, 16))

	volR := R(187, 6, 28, 6)
	fillRect(gtx.Ops, volR, color.NRGBA{R: 0x18, G: 0x1C, B: 0x28, A: 0xFF})
	vw := int(ui.volDrag.val * 28)
	if vw > 0 {
		fillRect(gtx.Ops, image.Rect(volR.Min.X, volR.Min.Y, volR.Min.X+vw, volR.Max.Y),
			color.NRGBA{R: 0x5E, G: 0xC8, B: 0x50, A: 0xFF})
	}
	ui.scrub(gtx, &ui.volDrag, R(187, 4, 28, 10))

	// fechar: apaga a flag — o ticker derruba ovlWin e o serviço remove a view
	if ui.spriteBtn(gtx, &ui.btnClose, "TITLEBAR", R(261, 3, 12, 12),
		R(18, 0, 9, 9), R(18, 9, 9, 9)) {
		_ = os.Remove(ovlFlagPath())
	}
}
