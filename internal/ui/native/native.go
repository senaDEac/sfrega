// Package native implementa a interface gráfica do sfrega com Gio,
// clonando visualmente o Winamp 2.x (janela principal, equalizador,
// playlist) de forma independente de navegador.
package native

import (
	"image"
	"image/color"
	"math"
	"os"
	"sync"
	"time"

	"gioui.org/app"
	"gioui.org/f32"
	"gioui.org/font"
	"gioui.org/font/opentype"
	"gioui.org/io/event"
	"gioui.org/io/pointer"
	"gioui.org/io/system"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"

	"sfrega/internal/player"
	"sfrega/internal/skins"
)

// Paleta Winamp 2.x (skin clássica cinza).
var (
	colWindowTop   = color.NRGBA{R: 0xC0, G: 0xC0, B: 0xC0, A: 0xFF}
	colWindowBot   = color.NRGBA{R: 0x80, G: 0x80, B: 0x80, A: 0xFF}
	colBevelLight  = color.NRGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF}
	colBevelDark   = color.NRGBA{R: 0x40, G: 0x40, B: 0x40, A: 0xFF}
	colBevelDk2    = color.NRGBA{R: 0x80, G: 0x80, B: 0x80, A: 0xFF}
	colLCDTop      = color.NRGBA{R: 0x0A, G: 0x3D, B: 0x0A, A: 0xFF}
	colLCDBot      = color.NRGBA{R: 0x02, G: 0x1A, B: 0x02, A: 0xFF}
	colLCDText     = color.NRGBA{R: 0x7F, G: 0xFF, B: 0x7F, A: 0xFF}
	colLCDTextDim  = color.NRGBA{R: 0x2F, G: 0x8F, B: 0x2F, A: 0xFF}
	colBtnTop      = color.NRGBA{R: 0xE8, G: 0xE8, B: 0xE8, A: 0xFF}
	colBtnBot      = color.NRGBA{R: 0xA8, G: 0xA8, B: 0xA8, A: 0xFF}
	colBtnEdge     = color.NRGBA{R: 0x60, G: 0x60, B: 0x60, A: 0xFF}
	colSpecBg      = color.NRGBA{R: 0x05, G: 0x05, B: 0x05, A: 0xFF}
	colSpecBar     = color.NRGBA{R: 0x33, G: 0xFF, B: 0x66, A: 0xFF}
	colSpecBarDim  = color.NRGBA{R: 0x1A, G: 0x7A, B: 0x2A, A: 0xFF}
	colTitlebarTop = color.NRGBA{R: 0x30, G: 0x30, B: 0x30, A: 0xFF}
	colTitlebarBot = color.NRGBA{R: 0x10, G: 0x10, B: 0x10, A: 0xFF}
	colTitleText   = color.NRGBA{R: 0xC8, G: 0xC8, B: 0xC8, A: 0xFF}
	colListBg      = color.NRGBA{R: 0x10, G: 0x10, B: 0x10, A: 0xFF}
	colListSel     = color.NRGBA{R: 0x2A, G: 0x5A, B: 0x2A, A: 0xFF}
	colListText    = color.NRGBA{R: 0xBB, G: 0xBB, B: 0xBB, A: 0xFF}
	colListSelText = color.NRGBA{R: 0x7F, G: 0xFF, B: 0x7F, A: 0xFF}
	colEqBg        = color.NRGBA{R: 0x1A, G: 0x1A, B: 0x1A, A: 0xFF}
)

// uiState concentra widgets e fontes.
type uiState struct {
	mu       sync.Mutex // protege winEQ/winPL
	p        *player.Player
	skin     *skins.Skin
	shaper   *text.Shaper
	fontUI   font.Font
	fontBold font.Font
	fontMono font.Font

	// janelas
	winMain *app.Window
	winEQ   *app.Window
	winPL   *app.Window
	eqOpen  bool
	plOpen  bool

	mainScale, eqScale, plScale int
	plWidth, plHeight           unit.Dp

	// widgets main
	btnPrev, btnPlay, btnPause, btnStop, btnNext uibtn
	btnEject, btnMin, btnSize, btnClose          uibtn
	btnEQ, btnPL                                 uibtn
	btnShuf, btnRep                              uibtn
	volDrag, balDrag, posDrag                    fader

	// widgets EQ
	plRowClicks                     []uibtn
	eqSliders                       [10]fader
	preampSlide                     fader
	eqGains                         [10]float64 // dB
	btnEQClose, btnEQMin, btnEQSize uibtn
	btnEQOn, btnEQAuto, btnEQPres   uibtn
	eqOn                            bool
	eqAuto                          bool
	eqPresOpen                      bool
	eqPresetBtns                    [6]uibtn

	// widgets playlist
	plList                                        widget.List
	btnPLRemove, btnPLRand, btnPLSort, btnPLWrite uibtn
	btnPLClose, btnPLMin, btnPLSize               uibtn
	btnPLClear                                    uibtn
	btnPLShade                                    uibtn
	btnPLMAdd, btnPLMRem, btnPLMSel               uibtn
	btnPLMMisc, btnPLMList                        uibtn
	btnPLPrev, btnPLPlay, btnPLPause              uibtn
	btnPLStop, btnPLNext, btnPLEject              uibtn
	plMenuClicks                                  [5][]uibtn
	plMenuOpen                                    int
	plSel                                         int

	shapeMu sync.Mutex // text.Shaper não é thread-safe (3 janelas)

	shadeMain, shadeEQ, shadePL shadeClk

	shuffle, repeat bool
	marqueeX        int
	lastTick        time.Time
}

// uibtn é um botão simples via pointer events (sem macro do Clickable).
type uibtn struct {
	tag  event.Tag
	down bool
}

// beginScaledFrame configura o sistema de coordenadas lógico da janela e
// aplica a escala visual 1x/2x escolhida pelo botão de tamanho.
func beginScaledFrame(gtx layout.Context, scale int, logical image.Point) layout.Context {
	if scale < 1 {
		scale = 1
	}
	gtx.Constraints.Min = image.Point{}
	gtx.Constraints.Max = logical
	if scale != 1 {
		op.Affine(f32.Affine2D{}.Scale(f32.Pt(0, 0), f32.Pt(float32(scale), float32(scale)))).Add(gtx.Ops)
	}
	return gtx
}

// toggleWindowScale alterna a janela entre 1x e 2x. A chamada vai para uma
// goroutine porque Window.Option precisa retornar à thread de eventos.
func toggleWindowScale(scale *int, w *app.Window, width, height unit.Dp) {
	if *scale < 1 {
		*scale = 1
	}
	if *scale == 1 {
		*scale = 2
	} else {
		*scale = 1
	}
	if w == nil || width <= 0 || height <= 0 {
		return
	}
	win := w
	size := app.Size(width*unit.Dp(*scale), height*unit.Dp(*scale))
	go win.Option(size)
}

// minimizeWindow minimiza a janela pelo WM (funciona em X11/Wayland, onde
// Perform(ActionMinimize) não é implementado). Vai para uma goroutine porque
// Window.Option precisa retornar à thread de eventos.
func minimizeWindow(w *app.Window) {
	if w == nil {
		return
	}
	win := w
	go func() {
		win.Option(app.Windowed.Option())
		win.Option(app.Minimized.Option())
	}()
}

// dragArea registra uma área de título que pode ser arrastada pelo sistema.
func dragArea(gtx layout.Context, r image.Rectangle) {
	if r.Empty() {
		return
	}
	clipStack := clip.Rect(r).Push(gtx.Ops)
	system.ActionInputOp(system.ActionMove).Add(gtx.Ops)
	clipStack.Pop()
}

var (
	dragTag    event.Tag
	dragActive bool
)

// dragAreaX11 move a janela via XMoveWindow durante o drag. Complementa
// dragArea: no X11 o Gio não implementa ActionMove (ver x11drag_linux.go).
func dragAreaX11(gtx layout.Context, r image.Rectangle) {
	if r.Empty() {
		return
	}
	clipStack := clip.Rect(r).Push(gtx.Ops)
	event.Op(gtx.Ops, &dragTag)
	clipStack.Pop()
	for {
		ev, ok := gtx.Event(pointer.Filter{
			Target: &dragTag,
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
		case pointer.Press:
			if e.Buttons == pointer.ButtonPrimary {
				dragActive = true
				wmDragBegin()
			}
		case pointer.Drag:
			if dragActive {
				wmDragMove()
			}
		case pointer.Release:
			if dragActive {
				dragActive = false
				wmDragEnd()
				wmSaveGeometry()
			}
			/* Cancel: mantém ativo — o C encerra ao soltar o botão. */
		}
	}
}

// shadeClk detecta duplo-clique numa área de título e alterna o "winshade"
// (janela reduzida à altura da barra). Um por janela.
type shadeClk struct {
	tag  event.Tag
	last time.Time
	x, y float32
	on   bool
}

// area registra a hit-area de shade. titlePx é a altura da titlebar em pixels.
func (sc *shadeClk) area(gtx layout.Context, r image.Rectangle, titlePx int) {
	if r.Empty() || titlePx <= 0 {
		return
	}
	clipStack := clip.Rect(r).Push(gtx.Ops)
	event.Op(gtx.Ops, &sc.tag)
	clipStack.Pop()
	for {
		ev, ok := gtx.Event(pointer.Filter{
			Target: &sc.tag,
			Kinds:  pointer.Press,
		})
		if !ok {
			break
		}
		e, ok := ev.(pointer.Event)
		if !ok || e.Buttons != pointer.ButtonPrimary {
			continue
		}
		now := time.Now()
		dbl := now.Sub(sc.last) < 400*time.Millisecond &&
			math.Abs(float64(e.Position.X-sc.x)) < 8 &&
			math.Abs(float64(e.Position.Y-sc.y)) < 8
		sc.last, sc.x, sc.y = now, e.Position.X, e.Position.Y
		if dbl {
			sc.on = !sc.on
			wmShadeToggle(titlePx)
		}
	}
}

// shadePx converte a altura lógica da titlebar (~14px) para pixels na escala atual.
func shadePx(scale int) int {
	if scale < 1 {
		scale = 1
	}
	return 14 * scale
}

// button registra hit-area, processa press/release e desenha via face.
// Retorna true no frame do clique (release dentro).
func (ui *uiState) button(gtx layout.Context, b *uibtn, r image.Rectangle, face func(gtx layout.Context, down bool)) bool {
	clipStack := clip.Rect(r).Push(gtx.Ops)
	event.Op(gtx.Ops, &b.tag)
	clipStack.Pop()
	clicked := false
	for {
		ev, ok := gtx.Event(pointer.Filter{
			Target: &b.tag,
			Kinds:  pointer.Press | pointer.Release | pointer.Drag | pointer.Move | pointer.Cancel | pointer.Enter | pointer.Leave,
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
			b.down = true
		case pointer.Release:
			if b.down {
				clicked = true
			}
			b.down = false
		case pointer.Cancel, pointer.Leave:
			b.down = false
		}
	}
	off := op.Offset(r.Min).Push(gtx.Ops)
	face(gtx, b.down)
	off.Pop()
	return clicked
}

// fader é um slider vertical/horizontal arrastável.
type fader struct {
	tag    event.Tag
	val    float32 // 0..1
	active bool
	commit func(float32)
}

func loadFonts() ([]text.FontFace, error) {
	var faces []text.FontFace
	paths := []string{
		"/usr/share/fonts/TTF/DejaVuSans.ttf",
		"/usr/share/fonts/TTF/DejaVuSans-Bold.ttf",
		"/usr/share/fonts/TTF/DejaVuSansMono.ttf",
		"/usr/share/fonts/liberation/LiberationSans-Regular.ttf",
		"/usr/share/fonts/liberation/LiberationSans-Bold.ttf",
		"/usr/share/fonts/liberation/LiberationMono-Regular.ttf",
	}
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		fc, err := opentype.ParseCollection(data)
		if err == nil {
			faces = append(faces, fc...)
		}
	}
	return faces, nil
}

// Run inicia a aplicação nativa.
func Run(p *player.Player) error {
	skin, err := skins.Default()
	if err != nil {
		return err
	}
	faces, _ := loadFonts()
	shaper := text.NewShaper(text.WithCollection(faces))
	ui := &uiState{
		p:          p,
		skin:       skin,
		shaper:     shaper,
		fontUI:     font.Font{Typeface: "DejaVu Sans"},
		fontBold:   font.Font{Typeface: "DejaVu Sans", Weight: font.Bold},
		fontMono:   font.Font{Typeface: "DejaVu Sans Mono"},
		plSel:      -1,
		plMenuOpen: -1,
	}
	ui.volDrag.val = 1.0
	ui.volDrag.commit = func(v float32) { p.SetVolume(v) }
	ui.eqOn = true
	ui.mainScale, ui.eqScale, ui.plScale = 1, 1, 1
	ui.plWidth, ui.plHeight = unit.Dp(280), unit.Dp(240)
	ui.balDrag.val = 0.5
	ui.balDrag.commit = func(v float32) { p.SetBalance(v*2 - 1) }
	ui.posDrag.commit = func(v float32) { p.SeekFraction(float64(v)) }
	ui.preampSlide.val = 0.5
	ui.preampSlide.commit = func(v float32) {
		// -12..+12 dB
		db := (v - 0.5) * 24
		p.SetPreamp(float64(db))
	}
	for i := range ui.eqSliders {
		ui.eqSliders[i].val = 0.5
		ui.eqSliders[i].commit = func(v float32) { ui.applyEQ() }
	}

	ui.winMain = new(app.Window)
	ui.winMain.Option(
		app.Title("sfrega"),
		app.Decorated(false),
		app.Size(unit.Dp(275), unit.Dp(116)),
	)

	// X11/muffin: aplica _MOTIF_WM_HINTS assim que a janela aparece
	// em _NET_CLIENT_LIST — polling curto evita a piscada de titlebar.
	go func() {
		t := time.NewTicker(30 * time.Millisecond)
		for range t.C {
			wmNoDecorations()
			wmRestoreGeometry()
		}
	}()

	go func() {
		t := time.NewTicker(33 * time.Millisecond)
		for range t.C {
			ui.winMain.Invalidate()
			ui.mu.Lock()
			if ui.winEQ != nil {
				ui.winEQ.Invalidate()
			}
			if ui.winPL != nil {
				ui.winPL.Invalidate()
			}
			ui.mu.Unlock()
		}
	}()

	var ops op.Ops
	for {
		switch e := ui.winMain.Event().(type) {
		case app.DestroyEvent:
			wmSaveGeometry()
			return e.Err
		case app.FrameEvent:
			gtx := app.NewContext(&ops, e)
			gtx = beginScaledFrame(gtx, ui.mainScale, image.Pt(275, 116))
			ui.layoutMain(gtx)
			e.Frame(gtx.Ops)
		}
	}
}

// ---------- helpers de desenho ----------

func fillRect(ops *op.Ops, r image.Rectangle, c color.NRGBA) {
	paint.FillShape(ops, c, clip.Rect(r).Op())
}

// vgradient preenche um retângulo com gradiente vertical.
func vgradient(gtx layout.Context, r image.Rectangle, top, bot color.NRGBA) {
	paint.LinearGradientOp{
		Stop1:  f32.Pt(float32(r.Min.X), float32(r.Min.Y)),
		Color1: top,
		Stop2:  f32.Pt(float32(r.Min.X), float32(r.Max.Y)),
		Color2: bot,
	}.Add(gtx.Ops)
	paint.PaintOp{}.Add(gtx.Ops)
}

// bevel desenha borda 3D estilo Winamp (claro sup/esq, escuro inf/dir).
func bevel(gtx layout.Context, r image.Rectangle, light, dark color.NRGBA) {
	fillRect(gtx.Ops, image.Rect(r.Min.X, r.Min.Y, r.Max.X, r.Min.Y+1), light)
	fillRect(gtx.Ops, image.Rect(r.Min.X, r.Min.Y, r.Min.X+1, r.Max.Y), light)
	fillRect(gtx.Ops, image.Rect(r.Min.X, r.Max.Y-1, r.Max.X, r.Max.Y), dark)
	fillRect(gtx.Ops, image.Rect(r.Max.X-1, r.Min.Y, r.Max.X, r.Max.Y), dark)
}

// box desenha retângulo com gradiente + bevel.
func box(gtx layout.Context, r image.Rectangle, top, bot, light, dark color.NRGBA) {
	vgradient(gtx, r, top, bot)
	bevel(gtx, r, light, dark)
}

// lbl desenha texto simples.
func (ui *uiState) lbl(gtx layout.Context, fnt font.Font, size unit.Sp, txt string, c color.NRGBA) layout.Dimensions {
	ui.shapeMu.Lock()
	defer ui.shapeMu.Unlock()
	m := op.Record(gtx.Ops)
	paint.ColorOp{Color: c}.Add(gtx.Ops)
	mat := m.Stop()
	return widget.Label{Alignment: text.Start}.Layout(gtx, ui.shaper, fnt, size, txt, mat)
}

// textAt desenha uma linha de texto em pos (thread-safe).
func (ui *uiState) textAt(gtx layout.Context, fnt font.Font, size unit.Sp, txt string, c color.NRGBA, pos image.Point) {
	ui.shapeMu.Lock()
	defer ui.shapeMu.Unlock()
	m := op.Record(gtx.Ops)
	paint.ColorOp{Color: c}.Add(gtx.Ops)
	mat := m.Stop()
	off := op.Offset(pos).Push(gtx.Ops)
	widget.Label{MaxLines: 1}.Layout(gtx, ui.shaper, fnt, size, txt, mat)
	off.Pop()
}

// winampButton desenha um botão quadrado estilo Winamp com glifo.
func (ui *uiState) winampButton(gtx layout.Context, c *widget.Clickable, glyph string, w, h int) layout.Dimensions {
	sz := image.Pt(w, h)
	return c.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min = sz
		gtx.Constraints.Max = sz
		r := image.Rectangle{Max: sz}
		pressed := c.Pressed()
		top, bot := colBtnTop, colBtnBot
		if pressed {
			top, bot = colBtnBot, colBtnTop
		}
		box(gtx, r, top, bot, colBevelLight, colBevelDark)
		// glifo centrado
		m := op.Record(gtx.Ops)
		paint.ColorOp{Color: color.NRGBA{R: 0x20, G: 0x20, B: 0x20, A: 0xFF}}.Add(gtx.Ops)
		mat := m.Stop()
		offX := (w - 8) / 2
		offY := (h - 12) / 2
		offset := op.Offset(image.Pt(offX, offY)).Push(gtx.Ops)
		ui.shapeMu.Lock()
		widget.Label{}.Layout(gtx, ui.shaper, ui.fontUI, unit.Sp(10), glyph, mat)
		ui.shapeMu.Unlock()
		offset.Pop()
		return layout.Dimensions{Size: sz}
	})
}

// faderLayout desenha e processa um fader (slider).
// dir: true = vertical, false = horizontal. rect é a área total do controle.
func (ui *uiState) faderLayout(gtx layout.Context, f *fader, rect image.Rectangle, vertical bool, label string) {
	// registra input
	event.Op(gtx.Ops, &f.tag)
	for {
		ev, ok := gtx.Event(pointer.Filter{
			Target: &f.tag,
			Kinds:  pointer.Press | pointer.Drag | pointer.Move | pointer.Release,
		})
		if !ok {
			break
		}
		if e, ok := ev.(pointer.Event); ok {
			var t float32
			if vertical {
				t = 1 - (float32(e.Position.Y)-float32(rect.Min.Y))/float32(rect.Dy())
			} else {
				t = (float32(e.Position.X) - float32(rect.Min.X)) / float32(rect.Dx())
			}
			if t < 0 {
				t = 0
			}
			if t > 1 {
				t = 1
			}
			f.val = t
			if f.commit != nil {
				f.commit(t)
			}
		}
	}
	// trilho
	track := rect
	if vertical {
		track = image.Rect(rect.Min.X+rect.Dx()/2-1, rect.Min.Y, rect.Min.X+rect.Dx()/2+1, rect.Max.Y)
	} else {
		track = image.Rect(rect.Min.X, rect.Min.Y+rect.Dy()/2-1, rect.Max.X, rect.Min.Y+rect.Dy()/2+1)
	}
	fillRect(gtx.Ops, track, colBevelDark)
	// preenchimento
	fill := track
	if vertical {
		y := track.Max.Y - int(float32(track.Dy())*f.val)
		fill = image.Rect(track.Min.X, y, track.Max.X, track.Max.Y)
	} else {
		w := int(float32(track.Dx()) * f.val)
		fill = image.Rect(track.Min.X, track.Min.Y, track.Min.X+w, track.Max.Y)
	}
	fillRect(gtx.Ops, fill, colSpecBarDim)
	// thumb
	thumb := image.Rect(0, 0, 7, 7)
	if vertical {
		cy := track.Max.Y - int(float32(track.Dy())*f.val)
		thumb = image.Rect(rect.Min.X-3, cy-3, rect.Min.X+4, cy+4)
	} else {
		cx := track.Min.X + int(float32(track.Dx())*f.val)
		thumb = image.Rect(cx-3, rect.Min.Y-3, cx+4, rect.Min.Y+4)
	}
	box(gtx, thumb, colBtnTop, colBtnBot, colBevelLight, colBevelDark)
	_ = label
}

// ---------- desenho via sprites da skin ----------

// spr desenha o recorte src do bitmap name em dst (escala se necessário).
func (ui *uiState) spr(gtx layout.Context, name string, src, dst image.Rectangle) {
	img := ui.skin.Spr(name, src)
	if img == nil {
		return
	}
	blit(gtx, img, dst)
}

// blit desenha src preenchendo dst.
func blit(gtx layout.Context, src image.Image, dst image.Rectangle) {
	if src == nil || dst.Empty() {
		return
	}
	cs := clip.Rect(dst).Push(gtx.Ops)
	off := op.Offset(dst.Min).Push(gtx.Ops)
	if sb := src.Bounds(); sb.Dx() != dst.Dx() || sb.Dy() != dst.Dy() {
		op.Affine(f32.Affine2D{}.Scale(f32.Pt(0, 0),
			f32.Pt(float32(dst.Dx())/float32(sb.Dx()), float32(dst.Dy())/float32(sb.Dy())))).Add(gtx.Ops)
	}
	paint.NewImageOp(src).Add(gtx.Ops)
	paint.PaintOp{}.Add(gtx.Ops)
	off.Pop()
	cs.Pop()
}

// scrub registra drag horizontal em r e atualiza f.val (0..1) + commit.
func (ui *uiState) scrub(gtx layout.Context, f *fader, r image.Rectangle) {
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
		if e.Kind == pointer.Press || e.Kind == pointer.Drag {
			t := (float32(e.Position.X) - float32(r.Min.X)) / float32(r.Dx())
			if t < 0 {
				t = 0
			}
			if t > 1 {
				t = 1
			}
			f.val = t
			if f.commit != nil {
				f.commit(t)
			}
		}
	}
}

// spriteBtn é um uibtn que desenha estados normal/pressed de um sprite.
// states: [normal, pressed] retângulos de origem no bitmap name.
func (ui *uiState) spriteBtn(gtx layout.Context, b *uibtn, name string, dst image.Rectangle, normal, pressed image.Rectangle) bool {
	return ui.button(gtx, b, dst, func(gtx layout.Context, down bool) {
		src := normal
		if down {
			src = pressed
		}
		ui.spr(gtx, name, src, image.Rectangle{Max: dst.Size()})
	})
}

// spectrumDraw desenha as barras do analisador com a paleta VISCOLOR da skin.
func (ui *uiState) spectrumDraw(gtx layout.Context, r image.Rectangle) {
	fillRect(gtx.Ops, r, color.NRGBA{A: 0xFF})
	bands := ui.p.Spectrum()
	n := len(bands)
	bw := r.Dx() / n
	if bw < 1 {
		bw = 1
	}
	for i := 0; i < n; i++ {
		v := float32(bands[i])
		if v < 0 {
			v = 0
		}
		if v > 1 {
			v = 1
		}
		h := int(v * float32(r.Dy()) * 1.2)
		if h < 1 {
			continue
		}
		bar := image.Rect(r.Min.X+i*bw, r.Max.Y-h, r.Min.X+(i+1)*bw-1, r.Max.Y)
		// cor por altura: topo = cores claras (índice baixo)
		ci := int((1 - v) * 17)
		fillRect(gtx.Ops, bar, ui.skin.VisColor(ci))
	}
}
