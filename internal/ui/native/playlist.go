package native

import (
	"fmt"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"sort"

	"gioui.org/app"
	"gioui.org/io/system"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
)

const plRowH = 18

// Cores da playlist clássica (fundo preto, texto claro).
var (
	colPLBg      = color.NRGBA{R: 0x08, G: 0x08, B: 0x10, A: 0xFF}
	colPLText    = color.NRGBA{R: 0xCC, G: 0xCC, B: 0xCC, A: 0xFF}
	colPLCur     = color.NRGBA{R: 0x33, G: 0xFF, B: 0x66, A: 0xFF}
	colPLSelBg   = color.NRGBA{R: 0x00, G: 0x00, B: 0x80, A: 0xFF}
	colPLSelText = color.NRGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF}
)

// plLoop é o loop de eventos da janela playlist.
func (ui *uiState) plLoop() {
	var ops op.Ops
	w := ui.winPL
	for {
		switch e := w.Event().(type) {
		case app.DestroyEvent:
			ui.mu.Lock()
			ui.winPL = nil
			ui.mu.Unlock()
			return
		case app.FrameEvent:
			gtx := app.NewContext(&ops, e)
			ui.layoutPL(gtx)
			e.Frame(gtx.Ops)
		}
	}
}

// layoutPL desenha a janela da playlist (280px) com sprites PLEDIT.
// O painel inferior (PLEDIT y111-186) contém a grade de botões de ícone.
func (ui *uiState) layoutPL(gtx layout.Context) {
	winW := gtx.Constraints.Max.X
	winH := gtx.Constraints.Max.Y
	tracks := ui.p.Tracks()

	R := func(x, y, w, h int) image.Rectangle { return image.Rect(x, y, x+w, y+h) }

	// ---- titlebar (mesmos sprites da janela principal) ----
	ui.spr(gtx, "TITLEBAR", R(47, 0, winW-53, 11), R(20, 0, winW-53, 11))
	ui.spr(gtx, "TITLEBAR", R(0, 0, 9, 9), R(6, 3, 9, 9))
	ui.spr(gtx, "TITLEBAR", R(9, 0, 9, 9), R(winW-31, 3, 9, 9))
	ui.spr(gtx, "TITLEBAR", R(0, 18, 9, 9), R(winW-21, 3, 9, 9))
	if ui.spriteBtn(gtx, &ui.btnPLClose, "TITLEBAR", R(winW-11, 3, 9, 9), R(18, 0, 9, 9), R(18, 9, 9, 9)) {
		ui.winPL.Perform(system.ActionClose)
	}
	ui.textAt(gtx, ui.fontMono, unit.Sp(8), "Playlist - sfrega",
		color.NRGBA{R: 0, G: 0xFF, B: 0, A: 0xFF}, image.Pt(20, 4))

	// ---- painel inferior: grade de botões PLEDIT (src y111-186 → dst winH-75) ----
	panelTop := winH - 75
	ui.spr(gtx, "PLEDIT", R(0, 111, 280, 75), R(0, panelTop, 280, 75))

	// botões funcionais: (col x, row y) no sistema de coordenadas do PLEDIT
	type plBtnDef struct {
		b    *uibtn
		sx   int // x do sprite normal em PLEDIT
		sy   int
		fn   func()
	}
	plBtns := []plBtnDef{
		{&ui.btnPLClear, 54, 111, func() { // REMOVE_ALL
			ui.p.SetTracks([]string{})
			ui.plSel = -1
		}},
		{&ui.btnPLSort, 154, 111, func() { // SORT_LIST
			cp := append([]string(nil), tracks...)
			sort.Strings(cp)
			ui.p.SetTracks(cp)
		}},
		{&ui.btnPLWrite, 204, 130, func() { // SAVE_LIST
			ui.writeM3U(tracks)
		}},
		{&ui.btnPLRemove, 54, 149, func() { // REMOVE_SELECTED
			if ui.plSel >= 0 && ui.plSel < len(tracks) {
				ui.p.RemoveAt(ui.plSel)
				ui.plSel = -1
			}
		}},
	}
	for _, b := range plBtns {
		sx, sy := b.sx, b.sy
		dst := image.Rect(sx, panelTop+(sy-111), sx+22, panelTop+(sy-111)+18)
		if ui.spriteBtn(gtx, b.b, "PLEDIT", dst,
			image.Rect(sx, sy, sx+22, sy+18), image.Rect(sx+23, sy, sx+45, sy+18)) {
			b.fn()
		}
	}

	// ---- área da lista ----
	listTop := 22
	listH := panelTop - 2 - listTop
	listR := image.Rect(4, listTop, winW-8, listTop+listH)
	fillRect(gtx.Ops, listR, colPLBg)
	bevel(gtx, listR, colBevelDark, colBevelDark)

	loff := op.Offset(listR.Min).Push(gtx.Ops)
	lgtx := gtx
	lgtx.Constraints.Min = image.Point{}
	lgtx.Constraints.Max = image.Pt(listR.Dx(), listH)
	ui.plList.Axis = layout.Vertical
	ui.plList.Layout(lgtx, len(tracks), func(gtx layout.Context, i int) layout.Dimensions {
		return ui.plRow(gtx, i, tracks, listR.Dx())
	})
	loff.Pop()

	// ---- status bar ----
	stY := panelTop + 4
	ui.textAt(gtx, ui.fontUI, unit.Sp(9), fmt.Sprintf("%d faixas", len(tracks)),
		colPLText, image.Pt(12, stY+2))
}

// plRow desenha uma linha da playlist.
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
	c := colPLText
	if cur {
		c = colPLCur
	}
	if sel {
		c = colPLSelText
	}
	ui.textAt(gtx, ui.fontUI, unit.Sp(10), name, c, image.Pt(6, 2))
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

