// Package native implementa a interface gráfica do sfrega com Gio,
// clonando visualmente o Winamp 2.x (janela principal, equalizador,
// playlist) de forma independente de navegador.
package native

import (
	"image"
	"image/color"
	"os"
	"sync"
	"time"

	"gioui.org/app"
	"gioui.org/f32"
	"gioui.org/font"
	"gioui.org/font/opentype"
	"gioui.org/io/event"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"

	"sfrega/internal/player"
)

// Paleta Winamp 2.x (skin clássica cinza).
var (
	colWindowTop    = color.NRGBA{R: 0xC0, G: 0xC0, B: 0xC0, A: 0xFF}
	colWindowBot    = color.NRGBA{R: 0x80, G: 0x80, B: 0x80, A: 0xFF}
	colBevelLight   = color.NRGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF}
	colBevelDark    = color.NRGBA{R: 0x40, G: 0x40, B: 0x40, A: 0xFF}
	colBevelDk2     = color.NRGBA{R: 0x80, G: 0x80, B: 0x80, A: 0xFF}
	colLCDTop       = color.NRGBA{R: 0x0A, G: 0x3D, B: 0x0A, A: 0xFF}
	colLCDBot       = color.NRGBA{R: 0x02, G: 0x1A, B: 0x02, A: 0xFF}
	colLCDText      = color.NRGBA{R: 0x7F, G: 0xFF, B: 0x7F, A: 0xFF}
	colLCDTextDim   = color.NRGBA{R: 0x2F, G: 0x8F, B: 0x2F, A: 0xFF}
	colBtnTop       = color.NRGBA{R: 0xE8, G: 0xE8, B: 0xE8, A: 0xFF}
	colBtnBot       = color.NRGBA{R: 0xA8, G: 0xA8, B: 0xA8, A: 0xFF}
	colBtnEdge      = color.NRGBA{R: 0x60, G: 0x60, B: 0x60, A: 0xFF}
	colSpecBg       = color.NRGBA{R: 0x05, G: 0x05, B: 0x05, A: 0xFF}
	colSpecBar      = color.NRGBA{R: 0x33, G: 0xFF, B: 0x66, A: 0xFF}
	colSpecBarDim   = color.NRGBA{R: 0x1A, G: 0x7A, B: 0x2A, A: 0xFF}
	colTitlebarTop  = color.NRGBA{R: 0x30, G: 0x30, B: 0x30, A: 0xFF}
	colTitlebarBot  = color.NRGBA{R: 0x10, G: 0x10, B: 0x10, A: 0xFF}
	colTitleText    = color.NRGBA{R: 0xC8, G: 0xC8, B: 0xC8, A: 0xFF}
	colListBg       = color.NRGBA{R: 0x10, G: 0x10, B: 0x10, A: 0xFF}
	colListSel      = color.NRGBA{R: 0x2A, G: 0x5A, B: 0x2A, A: 0xFF}
	colListText     = color.NRGBA{R: 0xBB, G: 0xBB, B: 0xBB, A: 0xFF}
	colListSelText  = color.NRGBA{R: 0x7F, G: 0xFF, B: 0x7F, A: 0xFF}
	colEqBg         = color.NRGBA{R: 0x1A, G: 0x1A, B: 0x1A, A: 0xFF}
)

// uiState concentra widgets e fontes.
type uiState struct {
	mu       sync.Mutex // protege winEQ/winPL
	p        *player.Player
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

	// widgets main
	btnPrev, btnPlay, btnPause, btnStop, btnNext uibtn
	btnEject, btnOpen, btnClose, btnEQ, btnPL    uibtn
	btnMute                                      uibtn
	volSlider, balSlider                         fader

	// widgets EQ
	plRowClicks []uibtn
	eqSliders   [10]fader
	preampSlide fader
	eqGains     [10]float64 // dB
	btnEQClose  uibtn

	// widgets playlist
	plList    widget.List
	btnPLRemove, btnPLRand, btnPLSort, btnPLWrite, btnPLClose uibtn
	plSel     int

	marqueeX int
	lastTick time.Time
}

// uibtn é um botão simples via pointer events (sem macro do Clickable).
type uibtn struct {
	tag event.Tag
	down bool
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
	faces, _ := loadFonts()
	shaper := text.NewShaper(text.WithCollection(faces))
	ui := &uiState{
		p:        p,
		shaper:   shaper,
		fontUI:   font.Font{Typeface: "DejaVu Sans"},
		fontBold: font.Font{Typeface: "DejaVu Sans", Weight: font.Bold},
		fontMono: font.Font{Typeface: "DejaVu Sans Mono"},
		plSel:    -1,
	}
	ui.volSlider.val = 1.0
	ui.volSlider.commit = func(v float32) { p.SetVolume(v) }
	ui.balSlider.val = 0.5
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
		app.Size(unit.Dp(540), unit.Dp(120)),
	)

	go func() {
		t := time.NewTicker(33 * time.Millisecond)
		for range t.C {
			ui.winMain.Invalidate()
			ui.mu.Lock()
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
			return e.Err
		case app.FrameEvent:
			gtx := app.NewContext(&ops, e)
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
	m := op.Record(gtx.Ops)
	paint.ColorOp{Color: c}.Add(gtx.Ops)
	mat := m.Stop()
	return widget.Label{Alignment: text.Start}.Layout(gtx, ui.shaper, fnt, size, txt, mat)
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
		widget.Label{}.Layout(gtx, ui.shaper, ui.fontUI, unit.Sp(10), glyph, mat)
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
				t = 1 - float32(e.Position.Y)/float32(rect.Dy())
			} else {
				t = float32(e.Position.X) / float32(rect.Dx())
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

// spectrumDraw desenha as barras do analisador.
func (ui *uiState) spectrumDraw(gtx layout.Context, r image.Rectangle) {
	fillRect(gtx.Ops, r, colSpecBg)
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
		fillRect(gtx.Ops, bar, colSpecBar)
	}
}
