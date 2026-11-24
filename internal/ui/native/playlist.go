package native

import (
	"fmt"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"gioui.org/app"
	"gioui.org/io/event"
	"gioui.org/io/pointer"
	"gioui.org/io/system"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/unit"
)

var plResTag event.Tag

// Cores do PLEDIT.TXT da skin base-2.91.
var (
	colPLBg      = color.NRGBA{R: 0x00, G: 0x00, B: 0x00, A: 0xFF}
	colPLText    = color.NRGBA{R: 0x00, G: 0xFF, B: 0x00, A: 0xFF} // Normal=#00FF00
	colPLCur     = color.NRGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF} // Current=#FFFFFF
	colPLSelBg   = color.NRGBA{R: 0x00, G: 0x00, B: 0xC6, A: 0xFF} // SelectedBG=#0000C6
	colPLSelText = color.NRGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF}
)

// Layout da janela playlist conforme WebAmp/PLEDIT:
//   topo 20px (tiles + shade/close), meio (lista + scrollbar),
//   rodapé 38px (4 menus suspensos à esquerda, tempo+transporte+list menu à direita).

const (
	plTopH  = 20
	plBotH  = 38
	plRowH  = 13
	plMenuX = 14 // primeiro menu button x (pitch 29)
)

// plMenuItem descreve um item de menu suspenso (sprite PLEDIT).
type plMenuItem struct {
	sx, sy int // sprite normal em PLEDIT
	fn     func()
}

// plWin/closePL acessam a janela PL com segurança (pode ser anulada pelo
// DestroyEvent enquanto um frame ainda renderiza).
func (ui *uiState) plWin() *app.Window {
	ui.mu.Lock()
	defer ui.mu.Unlock()
	return ui.winPL
}

func (ui *uiState) closePL() {
	if w := ui.plWin(); w != nil {
		w.Perform(system.ActionClose)
	}
}

// plLoop é o loop de eventos da janela playlist.
func (ui *uiState) plLoop() {
	var ops op.Ops
	w := ui.winPL
	for {
		switch e := w.Event().(type) {
		case app.DestroyEvent:
			wmSaveGeometry()
			ui.mu.Lock()
			ui.winPL = nil
			ui.plMenuOpen = -1
			delete(ui.ontopDone, "Playlist")
			saveOpenWindows(ui.eqOpen, false, ui.menuOpen)
			ui.mu.Unlock()
			return
		case app.FrameEvent:
			gtx := app.NewContext(&ops, e)
			logical := image.Pt(e.Size.X/ui.plScale, e.Size.Y/ui.plScale)
			if logical.X < 1 {
				logical.X = 1
			}
			if logical.Y < 1 {
				logical.Y = 1
			}
			gtx = beginScaledFrame(gtx, ui.plScale, logical)
			ui.plWidth = gtx.Metric.PxToDp(logical.X)
			ui.plHeight = gtx.Metric.PxToDp(logical.Y)
			ui.layoutPL(gtx)
			e.Frame(gtx.Ops)
		}
	}
}

// layoutPL desenha a janela da playlist com sprites PLEDIT.
func (ui *uiState) layoutPL(gtx layout.Context) {
	winW := gtx.Constraints.Max.X
	winH := gtx.Constraints.Max.Y
	tracks := ui.p.Tracks()

	R := func(x, y, w, h int) image.Rectangle { return image.Rect(x, y, x+w, y+h) }
	botY := winH - plBotH

	ui.shadePL.arm(shadePx(ui.plScale, 18))
	if ui.shadePL.on {
		ui.plShadeBar(gtx, winW, R)
		return
	}
	ui.plTopBar(gtx, winW, R)
	ui.plMiddle(gtx, winW, winH, botY, tracks)
	ui.plBottom(gtx, winW, winH, botY, tracks)
}

// textCell retorna o retângulo do glifo de c no TEXT.png (fonte 5x6 do
// Winamp, case-folded; faixa verde = primeiro mapa).
func textCell(c byte) image.Rectangle {
	uc := c
	if uc >= 'a' && uc <= 'z' {
		uc -= 'a' - 'A'
	}
	switch {
	case uc >= 'A' && uc <= 'Z':
		i := int(uc - 'A')
		return image.Rect(i*5, 0, i*5+5, 6)
	case uc >= '0' && uc <= '9':
		i := int(uc - '0')
		return image.Rect(i*5, 6, i*5+5, 12)
	}
	punct := map[byte]int{
		'.': 10, ':': 11, '(': 12, ')': 13, '-': 14, '\'': 15, '!': 16,
		'_': 17, '+': 18, '\\': 19, '/': 20, '[': 21, ']': 22, '~': 23,
		'&': 24, '%': 25, ',': 26, '=': 27, '$': 28, '#': 29,
	}
	if i, ok := punct[uc]; ok {
		return image.Rect(i*5, 6, i*5+5, 12)
	}
	switch uc {
	case '"':
		return image.Rect(130, 0, 135, 6)
	case '@':
		return image.Rect(135, 0, 140, 6)
	case '?':
		return image.Rect(15, 12, 20, 18)
	case '*':
		return image.Rect(20, 12, 25, 18)
	}
	return image.Rect(150, 0, 155, 6) // espaço/em branco
}

var textGlyphCache sync.Map // image.Rectangle -> *image.RGBA

// textGlyph retorna o glifo do TEXT.png com fundo preto tornado transparente.
func (ui *uiState) textGlyph(src image.Rectangle) *image.RGBA {
	if v, ok := textGlyphCache.Load(src); ok {
		return v.(*image.RGBA)
	}
	img := ui.skin.Load().Spr("TEXT", src)
	if img == nil {
		return nil
	}
	out := image.NewRGBA(image.Rect(0, 0, img.Bounds().Dx(), img.Bounds().Dy()))
	for y := 0; y < img.Bounds().Dy(); y++ {
		for x := 0; x < img.Bounds().Dx(); x++ {
			c := img.RGBAAt(x, y)
			if c.R+c.G+c.B > 30 {
				out.SetRGBA(x, y, c)
			}
		}
	}
	textGlyphCache.Store(src, out)
	return out
}

// plTextBmp desenha txt com a fonte bitmap TEXT.png a partir de (x,y),
// avançando 5px por caractere até o limite maxX.
func (ui *uiState) plTextBmp(gtx layout.Context, txt string, x, y, maxX int) {
	pen := x
	for i := 0; i < len(txt) && pen+5 <= maxX; i++ {
		if g := ui.textGlyph(textCell(txt[i])); g != nil {
			blit(gtx, g, image.Rect(pen, y, pen+5, y+6))
		}
		pen += 5
	}
}

// plShadeBar desenha a playlist recolhida: linha única na altura do shade
// do player (18px), começando com a decoração/título "Playlist".
func (ui *uiState) plShadeBar(gtx layout.Context, winW int, R func(int, int, int, int) image.Rectangle) {
	bar := R(0, 0, winW, 18)
	fillRect(gtx.Ops, bar, color.NRGBA{R: 0x2D, G: 0x32, B: 0x42, A: 0xFF})
	fillRect(gtx.Ops, image.Rect(bar.Min.X, bar.Min.Y, bar.Max.X, bar.Min.Y+1),
		color.NRGBA{R: 0x5A, G: 0x62, B: 0x78, A: 0xFF})
	fillRect(gtx.Ops, image.Rect(bar.Min.X, bar.Max.Y-1, bar.Max.X, bar.Max.Y),
		color.NRGBA{R: 0x14, G: 0x17, B: 0x22, A: 0xFF})

	// decoração PLEDIT (0,0-15,15) + nome "Playlist" (75,0-122,15) em sequência
	ui.sprFit(gtx, "PLEDIT", image.Rect(0, 0, 16, 16), R(2, 1, 16, 16))
	ui.sprFit(gtx, "PLEDIT", image.Rect(75, 0, 123, 16), R(20, 1, 48, 16))

	// caixa do nome da música: capa esq. + meio elástico + capa dir.
	// sprites PLEDIT: (73,43-77,55), (72,58-96,71), (120,42-129,55)
	bx0, bx1, by, bh := 74, winW-40, 2, 14
	if bx1 > bx0+10 {
		ui.sprFit(gtx, "PLEDIT", image.Rect(73, 43, 78, 55), R(bx0, by, 5, bh))
		ui.sprFit(gtx, "PLEDIT", image.Rect(72, 58, 97, 71), R(bx0+5, by, bx1-bx0-15, bh))
		ui.sprFit(gtx, "PLEDIT", image.Rect(120, 42, 130, 55), R(bx1-10, by, 10, bh))
		if idx := ui.p.Index(); idx >= 0 {
			if t := ui.p.TrackAt(idx); t != "" {
				ui.plTextBmp(gtx, filepath.Base(t), bx0+7, by+4, bx1-12)
			}
		}
	}

	dragArea(gtx, R(0, 0, winW-32, 18))
	dragAreaX11(gtx, R(0, 0, winW-32, 18), &ui.shadePL)
	if ui.spriteBtn(gtx, &ui.btnPLMin, "TITLEBAR", R(winW-31, 4, 9, 9), R(9, 0, 9, 9), R(9, 9, 9, 9)) {
		minimizeWindow(ui.plWin())
	}
	if ui.spriteBtn(gtx, &ui.btnPLSize, "TITLEBAR", R(winW-21, 4, 9, 9), R(0, 18, 9, 9), R(9, 18, 9, 9)) {
		if w := ui.plWin(); w != nil {
			toggleWindowScale(&ui.plScale, w, ui.plWidth, ui.plHeight)
		}
	}
	if ui.spriteBtn(gtx, &ui.btnPLClose, "TITLEBAR", R(winW-11, 4, 9, 9), R(18, 0, 9, 9), R(18, 9, 9, 9)) {
		ui.closePL()
	}
}

// plTopBar desenha a barra superior de 20px com tiles PLEDIT.
func (ui *uiState) plTopBar(gtx layout.Context, winW int, R func(int, int, int, int) image.Rectangle) {
	// tiles: left corner 25 + title 100 + fill tiles + right corner 25
	ui.spr(gtx, "PLEDIT", R(0, 21, 25, 20), R(0, 0, 25, 20))
	ui.spr(gtx, "PLEDIT", R(26, 21, 100, 20), R(25, 0, 100, 20))
	for x := 125; x < winW-25; x += 25 {
		w := 25
		if x+w > winW-25 {
			w = winW - 25 - x
		}
		ui.spr(gtx, "PLEDIT", R(127, 21, 25, 20), R(x, 0, w, 20))
	}
	ui.spr(gtx, "PLEDIT", R(153, 21, 25, 20), R(winW-25, 0, 25, 20))

	dragArea(gtx, R(0, 0, winW-32, plTopH))
	dragAreaX11(gtx, R(0, 0, winW-32, plTopH), &ui.shadePL)
	if ui.spriteBtn(gtx, &ui.btnPLMin, "TITLEBAR", R(winW-31, 3, 9, 9), R(9, 0, 9, 9), R(9, 9, 9, 9)) {
		minimizeWindow(ui.plWin())
	}
	if ui.spriteBtn(gtx, &ui.btnPLSize, "TITLEBAR", R(winW-21, 3, 9, 9), R(0, 18, 9, 9), R(9, 18, 9, 9)) {
		if w := ui.plWin(); w != nil {
			toggleWindowScale(&ui.plScale, w, ui.plWidth, ui.plHeight)
		}
	}
	if ui.spriteBtn(gtx, &ui.btnPLClose, "TITLEBAR", R(winW-11, 3, 9, 9), R(18, 0, 9, 9), R(18, 9, 9, 9)) {
		ui.closePL()
	}
}

// plMiddle desenha a área da lista com tiles laterais.
func (ui *uiState) plMiddle(gtx layout.Context, winW, winH, botY int, tracks []string) {
	R := func(x, y, w, h int) image.Rectangle { return image.Rect(x, y, x+w, y+h) }
	midH := botY - plTopH

	// tiles laterais esticados
	ui.spr(gtx, "PLEDIT", R(0, 42, 12, 29), R(0, plTopH, 12, midH))
	ui.spr(gtx, "PLEDIT", R(31, 42, 20, 29), R(winW-20, plTopH, 20, midH))

	// fundo preto do miolo inteiro (inclui a fileira dos botões GENEX)
	fillRect(gtx.Ops, image.Rect(12, plTopH+1, winW-23, botY-3), colPLBg)

	// 6 botões GENEX abaixo da linha de título, com rótulos da skin GEN
	gbtns := []*uibtn{&ui.btnPLG1, &ui.btnPLG2, &ui.btnPLG3,
		&ui.btnPLG4, &ui.btnPLG5, &ui.btnPLG6}
	labels := []string{"GENERO", "MUSICAS", "ARTISTA", "PASTAS", "ALBUNS", "LISTA"}
	dark := color.NRGBA{R: 0x10, G: 0x10, B: 0x18, A: 0xFF}
	for i, gb := range gbtns {
		bx := 14 + i*47
		ui.spriteBtn(gtx, gb, "GENEX", R(bx, plTopH+3, 45, 11),
			R(0, 0, 48, 16), R(0, 16, 48, 15))
		ui.genText(gtx, labels[i], R(bx, plTopH+3, 45, 11), dark)
	}

	// fundo da lista
	listR := image.Rect(12, plTopH+16, winW-23, botY-3)
	fillRect(gtx.Ops, listR, colPLBg)

	loff := op.Offset(listR.Min).Push(gtx.Ops)
	lgtx := gtx
	lgtx.Constraints.Min = image.Point{}
	lgtx.Constraints.Max = image.Pt(listR.Dx(), listR.Dy())
	ui.plList.Axis = layout.Vertical
	ui.plList.Layout(lgtx, len(tracks), func(gtx layout.Context, i int) layout.Dimensions {
		return ui.plRow(gtx, i, tracks, listR.Dx())
	})
	loff.Pop()

	// handle da scrollbar (decorativo)
	ui.spr(gtx, "PLEDIT", R(52, 53, 8, 18), R(winW-17, plTopH+6, 8, 18))
}

// plBottom desenha o rodapé de 38px: menus à esquerda, transporte à direita.
func (ui *uiState) plBottom(gtx layout.Context, winW, winH, botY int, tracks []string) {
	R := func(x, y, w, h int) image.Rectangle { return image.Rect(x, y, x+w, y+h) }

	// tiles do rodapé
	ui.spr(gtx, "PLEDIT", R(0, 72, 125, 38), R(0, botY, 125, 38))
	ui.spr(gtx, "PLEDIT", R(126, 72, 150, 38), R(winW-150, botY, 150, 38))
	if winW > 280 {
		ui.spr(gtx, "PLEDIT", R(179, 0, 25, 38), R(125, botY, winW-275, 38))
	}

	// ---- menus suspensos (bottom:12 → y=botY+8) ----
	type menuDef struct {
		btn   *uibtn
		faceX int // x do sprite da face (linha y149)
		sy    int
		items []plMenuItem
	}
	clearAll := func() { ui.p.SetTracks([]string{}); ui.plSel = -1 }
	sortList := func() {
		cp := append([]string(nil), tracks...)
		sort.Strings(cp)
		ui.p.SetTracks(cp)
	}
	writeList := func() { ui.writeM3U(tracks) }
	removeSel := func() {
		if ui.plSel >= 0 && ui.plSel < len(tracks) {
			ui.p.RemoveAt(ui.plSel)
			ui.plSel = -1
		}
	}

	menus := []menuDef{
		{&ui.btnPLMAdd, 0, 149, []plMenuItem{
			{0, 111, nil},
			{0, 130, func() { ui.addPicked(true) }},
			{0, 149, func() { ui.addPicked(false) }},
		}},
		{&ui.btnPLMRem, 54, 149, []plMenuItem{
			{54, 111, clearAll}, {54, 130, nil}, {54, 149, removeSel}, {54, 168, nil},
		}},
		{&ui.btnPLMSel, 104, 149, []plMenuItem{
			{104, 111, nil}, {104, 130, func() { ui.plSel = -1 }}, {104, 149, nil},
		}},
		{&ui.btnPLMMisc, 154, 149, []plMenuItem{
			{154, 111, sortList}, {154, 130, nil}, {154, 149, nil},
		}},
	}
	for mi, mn := range menus {
		bx := plMenuX + mi*29
		by := botY + 8
		// face do menu (botão colapsado) = item da linha y149
		faceSrc := R(mn.faceX, 149, 22, 18)
		faceSel := R(mn.faceX+23, 149, 22, 18)
		dst := R(bx, by, 22, 18)
		if ui.spriteBtn(gtx, mn.btn, "PLEDIT", dst, faceSrc, faceSel) {
			if ui.plMenuOpen == mi {
				ui.plMenuOpen = -1
			} else {
				ui.plMenuOpen = mi
			}
		}
		// popup de itens (acima do botão)
		if ui.plMenuOpen == mi {
			n := len(mn.items)
			for ii, it := range mn.items {
				iy := by - (n-ii)*18
				idst := R(bx, iy, 22, 18)
				nrm := R(it.sx, it.sy, 22, 18)
				sel := R(it.sx+23, it.sy, 22, 18)
				for len(ui.plMenuClicks[mi]) <= ii {
					ui.plMenuClicks[mi] = append(ui.plMenuClicks[mi], uibtn{})
				}
				if ui.spriteBtn(gtx, &ui.plMenuClicks[mi][ii], "PLEDIT", idst, nrm, sel) {
					ui.plMenuOpen = -1
					if it.fn != nil {
						it.fn()
					}
				}
			}
		}
	}

	// ---- lado direito do rodapé (150px) ----
	rx := winW - 150
	// tempo total da playlist (verde)
	ui.textAt(gtx, ui.fontMono, unit.Sp(8), ui.plTimeText(), colPLText, image.Pt(rx+7, botY+10))

	// botões de transporte 10x10 (CBUTTONS escalado)
	type tbtn struct {
		b   *uibtn
		col int
		w   int
		fn  func()
	}
	tbtns := []tbtn{
		{&ui.btnPLPrev, 0, 23, func() { ui.p.Prev(); ui.p.Play() }},
		{&ui.btnPLPlay, 23, 23, func() { ui.p.Play() }},
		{&ui.btnPLPause, 46, 23, func() { ui.p.Pause() }},
		{&ui.btnPLStop, 69, 23, func() { ui.p.Pause(); _ = ui.p.SeekFraction(0) }},
		{&ui.btnPLNext, 92, 22, func() { ui.p.Next(); ui.p.Play() }},
	}
	for i, b := range tbtns {
		dst := R(rx+3+i*10, botY+22, 10, 10)
		if ui.spriteBtn(gtx, b.b, "CBUTTONS", dst,
			R(b.col, 0, b.w, 18), R(b.col, 18, b.w, 18)) {
			b.fn()
		}
	}
	// eject
	if ui.spriteBtn(gtx, &ui.btnPLEject, "CBUTTONS", R(rx+53, botY+22, 10, 10),
		R(114, 0, 22, 16), R(114, 16, 22, 16)) {
		ui.p.Pause()
		_ = ui.p.SeekFraction(0)
	}
	// mini-time (tempo atual)
	ui.textAt(gtx, ui.fontMono, unit.Sp(7), ui.fmtTime(ui.p.Position().Seconds()),
		colPLText, image.Pt(rx+66, botY+23))

	// menu List (new/save/load) à direita
	lx := winW - 44
	ly := botY + 8
	faceSrc := R(204, 149, 22, 18)
	faceSel := R(227, 149, 22, 18)
	if ui.spriteBtn(gtx, &ui.btnPLMList, "PLEDIT", R(lx, ly, 22, 18), faceSrc, faceSel) {
		if ui.plMenuOpen == 4 {
			ui.plMenuOpen = -1
		} else {
			ui.plMenuOpen = 4
		}
	}
	if ui.plMenuOpen == 4 {
		litems := []plMenuItem{
			{204, 111, clearAll},
			{204, 130, writeList},
			{204, 149, nil},
		}
		n := len(litems)
		for ii, it := range litems {
			iy := ly - (n-ii)*18
			nrm := R(it.sx, it.sy, 22, 18)
			sel := R(it.sx+23, it.sy, 22, 18)
			for len(ui.plMenuClicks[4]) <= ii {
				ui.plMenuClicks[4] = append(ui.plMenuClicks[4], uibtn{})
			}
			if ui.spriteBtn(gtx, &ui.plMenuClicks[4][ii], "PLEDIT", R(lx, iy, 22, 18), nrm, sel) {
				ui.plMenuOpen = -1
				if it.fn != nil {
					it.fn()
				}
			}
		}
	}

	// setas de scroll (decorativas)
	ui.plScrollArrow(gtx, winW-15, botY+2, true)
	ui.plScrollArrow(gtx, winW-15, botY+8, false)

	// orelha de redimensionamento (canto inferior direito)
	ui.plResize(gtx, image.Rect(winW-16, botY+20, winW-2, botY+36), winW, winH)
}

// plResize registra drag na orelha do canto inferior direito e
// redimensiona a janela via X11 (wmResize).
func (ui *uiState) plResize(gtx layout.Context, r image.Rectangle, winW, winH int) {
	if r.Empty() {
		return
	}
	clipStack := clip.Rect(r).Push(gtx.Ops)
	event.Op(gtx.Ops, &plResTag)
	clipStack.Pop()
	for {
		ev, ok := gtx.Event(pointer.Filter{
			Target: &plResTag,
			Kinds:  pointer.Press,
		})
		if !ok {
			break
		}
		e, ok := ev.(pointer.Event)
		if !ok {
			continue
		}
		if e.Kind == pointer.Press && e.Buttons == pointer.ButtonPrimary {
			scale := ui.plScale
			if scale < 1 {
				scale = 1
			}
			go func() {
				wmResizeDrag("Playlist", winW*scale, winH*scale,
					300*scale, 140*scale)
				time.Sleep(150 * time.Millisecond)
				ui.savePLSize()
			}()
		}
	}
}

// plScrollArrow desenha seta up/down 8x5.
func (ui *uiState) plScrollArrow(gtx layout.Context, x, y int, up bool) {
	col := color.NRGBA{R: 0xAA, G: 0xAA, B: 0xAA, A: 0xFF}
	if up {
		for i := 0; i < 4; i++ {
			fillRect(gtx.Ops, image.Rect(x+i, y+4-i, x+i+1, y+5-i), col)
		}
		fillRect(gtx.Ops, image.Rect(x, y+4, x+4, y+5), col)
	} else {
		for i := 0; i < 4; i++ {
			fillRect(gtx.Ops, image.Rect(x+i, y+i, x+i+1, y+i+1), col)
		}
		fillRect(gtx.Ops, image.Rect(x, y, x+4, y+1), col)
	}
}

// plTimeText retorna tempo total estimado da playlist (mm:ss).
func (ui *uiState) plTimeText() string {
	return ui.fmtTime(ui.p.Position().Seconds())
}

func (ui *uiState) fmtTime(sec float64) string {
	s := int(sec)
	if s < 0 {
		s = 0
	}
	return fmt.Sprintf("%d:%02d", (s/60)%100, s%60)
}

// plRow desenha uma linha da playlist (13px, fonte verde).
func (ui *uiState) plRow(gtx layout.Context, i int, tracks []string, w int) layout.Dimensions {
	h := plRowH
	r := image.Rect(0, 0, w, h)
	cur := i == ui.p.Index()
	sel := i == ui.plSel
	if sel {
		fillRect(gtx.Ops, r, colPLSelBg)
	}
	name := filepath.Base(tracks[i])
	if sel {
		name = "> " + name
	}
	if m := ui.p.TrackMeta(tracks[i]); m != nil {
		var sb strings.Builder
		if m.Rating > 0 {
			fmt.Fprintf(&sb, "%d/5 ", m.Rating)
		}
		if m.Plays > 0 {
			fmt.Fprintf(&sb, "x%d ", m.Plays)
		}
		for j, t := range m.Tags {
			if j == 2 {
				break
			}
			fmt.Fprintf(&sb, "#%s ", t)
		}
		name += "  " + strings.TrimSpace(sb.String())
	}
	c := colPLText
	if cur {
		c = colPLCur
	}
	if sel {
		c = colPLSelText
	}
	ui.textAt(gtx, ui.fontUI, unit.Sp(9), name, c, image.Pt(4, 1))
	for len(ui.plRowClicks) <= i {
		ui.plRowClicks = append(ui.plRowClicks, uibtn{})
	}
	if ui.button(gtx, &ui.plRowClicks[i], r, func(gtx layout.Context, down bool) {}) {
		ui.plSel = i
		ui.p.PlayIndex(i)
	}
	return layout.Dimensions{Size: image.Pt(w, h)}
}

// writeM3U salva a playlist em .m3u.
func (ui *uiState) writeM3U(tracks []string) {
	if len(tracks) == 0 {
		return
	}
	f, err := os.Create("playlist.m3u")
	if err != nil {
		return
	}
	defer f.Close()
	f.WriteString("#EXTM3U\n")
	for _, t := range tracks {
		f.WriteString(t + "\n")
	}
}
