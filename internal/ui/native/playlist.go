package native

import (
	"fmt"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"sort"

	"gioui.org/app"
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

const plRowH = 18

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

// layoutPL desenha a janela da playlist.
func (ui *uiState) layoutPL(gtx layout.Context) {
	winW := gtx.Constraints.Max.X
	winH := gtx.Constraints.Max.Y
	tracks := ui.p.Tracks()

	// titlebar com sprites da skin
	R := func(x, y, w, h int) image.Rectangle { return image.Rect(x, y, x+w, y+h) }
	ui.spr(gtx, "TITLEBAR", R(47, 0, 222, 11), R(20, 0, winW-53, 11))
	ui.spr(gtx, "TITLEBAR", R(0, 0, 9, 9), R(6, 3, 9, 9))
	ui.spr(gtx, "TITLEBAR", R(9, 0, 9, 9), R(winW-31, 3, 9, 9))
	ui.spr(gtx, "TITLEBAR", R(0, 18, 9, 9), R(winW-21, 3, 9, 9))
	if ui.spriteBtn(gtx, &ui.btnPLClose, "TITLEBAR", R(winW-11, 3, 9, 9), R(18, 0, 9, 9), R(18, 9, 9, 9)) {
		ui.winPL.Perform(system.ActionClose)
	}
	// título verde
	m := op.Record(gtx.Ops)
	paint.ColorOp{Color: color.NRGBA{R: 0, G: 0xFF, B: 0, A: 0xFF}}.Add(gtx.Ops)
	tmat := m.Stop()
	off := op.Offset(image.Pt(20, 4)).Push(gtx.Ops)
	widget.Label{}.Layout(gtx, ui.shaper, ui.fontMono, unit.Sp(8), "Playlist - sfrega", tmat)
	off.Pop()

	// toolbar
	by := 22
	bh := 20
	bw := 62
	bx := 6
	plBtns := []struct {
		b   *uibtn
		txt string
		fn  func()
	}{
		{&ui.btnPLRemove, "Remove", func() {
			if ui.plSel >= 0 && ui.plSel < len(tracks) {
				ui.p.RemoveAt(ui.plSel)
				ui.plSel = -1
			}
		}},
		{&ui.btnPLRand, "Random", func() {
			cp := append([]string(nil), tracks...)
			for i := len(cp) - 1; i > 0; i-- {
				j := int(uint32(i*2654435761) % uint32(i+1))
				cp[i], cp[j] = cp[j], cp[i]
			}
			idx := ui.p.Index()
			cur := ""
			if idx >= 0 && idx < len(tracks) {
				cur = tracks[idx]
			}
			ui.p.SetTracks(cp)
			for i, t := range cp {
				if t == cur {
					ui.p.PlayIndex(i)
					break
				}
			}
		}},
		{&ui.btnPLSort, "Sort A-Z", func() {
			cp := append([]string(nil), tracks...)
			sort.Strings(cp)
			ui.p.SetTracks(cp)
		}},
		{&ui.btnPLWrite, "Write", func() {
			ui.writeM3U(tracks)
		}},
	}
	for _, b := range plBtns {
		if ui.plBtn(gtx, b.b, bx, by, bw, bh, b.txt) {
			b.fn()
		}
		bx += bw + 6
	}

	// área da lista
	listTop := 48
	listH := winH - listTop - 20
	listR := image.Rect(6, listTop, winW-14, listTop+listH)
	fillRect(gtx.Ops, listR, colListBg)
	bevel(gtx, listR, colBevelLight, colBevelDark)

	loff := op.Offset(listR.Min).Push(gtx.Ops)
	lgtx := gtx
	lgtx.Constraints.Min = image.Point{}
	lgtx.Constraints.Max = image.Pt(listR.Dx(), listH)
	ui.plList.Axis = layout.Vertical
	ui.plList.Layout(lgtx, len(tracks), func(gtx layout.Context, i int) layout.Dimensions {
		return ui.plRow(gtx, i, tracks)
	})
	loff.Pop()

	// status bar
	stY := winH - 18
	st := image.Rect(6, stY, winW-6, stY+16)
	fillRect(gtx.Ops, st, colWindowBot)
	bevel(gtx, st, colBevelLight, colBevelDark)
	m2 := op.Record(gtx.Ops)
	paint.ColorOp{Color: colTitleText}.Add(gtx.Ops)
	smat := m2.Stop()
	soff := op.Offset(image.Pt(10, stY+2)).Push(gtx.Ops)
	widget.Label{}.Layout(gtx, ui.shaper, ui.fontUI, unit.Sp(9),
		fmt.Sprintf("%d faixas", len(tracks)), smat)
	soff.Pop()
}

// plCloseBtn desenha o X da janela playlist.
func (ui *uiState) plCloseBtn(gtx layout.Context, r image.Rectangle) bool {
	return ui.button(gtx, &ui.btnPLClose, r, func(gtx layout.Context, down bool) {
		if down {
			fillRect(gtx.Ops, image.Rectangle{Max: r.Size()}, color.NRGBA{R: 0x80, G: 0, B: 0, A: 0xFF})
		}
		path := clip.Path{}
		path.Begin(gtx.Ops)
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

// plRow desenha uma linha da playlist.
func (ui *uiState) plRow(gtx layout.Context, i int, tracks []string) layout.Dimensions {
	w := gtx.Constraints.Max.X
	h := plRowH
	r := image.Rect(0, 0, w, h)
	sel := i == ui.p.Index()
	if sel {
		fillRect(gtx.Ops, r, colListSel)
	} else if i == ui.plSel {
		fillRect(gtx.Ops, r, color.NRGBA{R: 0x20, G: 0x20, B: 0x20, A: 0xFF})
	}
	name := filepath.Base(tracks[i])
	if i == ui.plSel {
		name = "> " + name
	}
	m := op.Record(gtx.Ops)
	c := colListText
	if sel {
		c = colListSelText
	}
	paint.ColorOp{Color: c}.Add(gtx.Ops)
	mat := m.Stop()
	off := op.Offset(image.Pt(4, 2)).Push(gtx.Ops)
	widget.Label{MaxLines: 1}.Layout(gtx, ui.shaper, ui.fontUI, unit.Sp(10), name, mat)
	off.Pop()
	// clique seleciona/toca
	for len(ui.plRowClicks) <= i {
		ui.plRowClicks = append(ui.plRowClicks, uibtn{})
	}
	if ui.button(gtx, &ui.plRowClicks[i], r, func(gtx layout.Context, down bool) {}) {
		ui.plSel = i
		ui.p.PlayIndex(i)
	}
	return layout.Dimensions{Size: image.Pt(w, h)}
}

// plBtn desenha botão da toolbar da playlist.
func (ui *uiState) plBtn(gtx layout.Context, b *uibtn, x, y, w, h int, txt string) bool {
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
		off2 := op.Offset(image.Pt(0, 3)).Push(gtx.Ops)
		widget.Label{Alignment: text.Middle}.Layout(gtx, ui.shaper, ui.fontUI, unit.Sp(9), txt, mat)
		off2.Pop()
	})
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
