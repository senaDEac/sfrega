package native

import (
	"image"
	"image/color"
	"time"

	"gioui.org/app"
	"gioui.org/io/system"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"sfrega/internal/language"
)

// Janela de confirmação para ações destrutivas do menu Config>Contadores.
// confirmKind: 1 = zerar shuffle, 2 = zerar plays, 3 = zerar hashtags.

// openConfirm abre (ou troca o tipo da) janela de confirmação.
func (ui *uiState) openConfirm(kind int) {
	ui.mu.Lock()
	defer ui.mu.Unlock()
	ui.confirmKind = kind
	if ui.winConfirm == nil {
		ui.confirmScale = ui.mainScale
		w := new(app.Window)
		w.Option(
			app.Title("Confirmar"),
			app.Decorated(false),
			app.Size(unit.Dp(250*ui.confirmScale), unit.Dp(80*ui.confirmScale)),
		)
		ui.winConfirm = w
		ui.confirmOpen = true
		go ui.confirmPlaceLoop()
		go ui.confirmLoop()
		return
	}
	ui.winConfirm.Invalidate()
}

// closeConfirm fecha a janela de confirmação com segurança.
func (ui *uiState) closeConfirm() {
	ui.mu.Lock()
	w := ui.winConfirm
	ui.mu.Unlock()
	if w != nil {
		w.Perform(system.ActionClose)
	}
}

// confirmPlaceLoop posiciona a janela abaixo do Menu (fallback: centro).
func (ui *uiState) confirmPlaceLoop() {
	for i := 0; i < 60; i++ {
		if wmBelow("Confirmar", "Menu", 0) {
			return
		}
		select {
		case <-time.After(50 * time.Millisecond):
		}
	}
	wmCenter("Confirmar")
}

// confirmLoop é o loop de eventos da janela de confirmação.
func (ui *uiState) confirmLoop() {
	var ops op.Ops
	w := ui.winConfirm
	for {
		switch e := w.Event().(type) {
		case app.DestroyEvent:
			wmSaveGeometry()
			ui.mu.Lock()
			ui.winConfirm = nil
			ui.confirmOpen = false
			ui.confirmKind = 0
			delete(ui.ontopDone, "Confirmar")
			ui.mu.Unlock()
			return
		case app.FrameEvent:
			gtx := app.NewContext(&ops, e)
			gtx = beginScaledFrame(gtx, ui.confirmScale, image.Pt(250, 80))
			ui.layoutConfirm(gtx)
			e.Frame(gtx.Ops)
		}
	}
}

// layoutConfirm desenha a pergunta e os botões Sim/Não.
func (ui *uiState) layoutConfirm(gtx layout.Context) {
	R := func(x, y, w, h int) image.Rectangle { return image.Rect(x, y, x+w, y+h) }

	// fundo branco cobrindo a janela inteira (mesmo que o WM a deixe maior)
	fillRect(gtx.Ops, image.Rect(-100, -100, 2000, 2000),
		color.NRGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF})
	fillRect(gtx.Ops, image.Rect(0, 0, 250, 1), colMenuSepHi)
	fillRect(gtx.Ops, image.Rect(0, 79, 250, 1), colMenuSep)

	var msg string
	switch ui.confirmKind {
	case 1:
		msg = language.T("confirm.reset_shuffle")
	case 2:
		msg = language.T("confirm.reset_plays")
	case 3:
		msg = language.T("confirm.reset_tags")
	default:
		msg = "?"
	}
	colBordo := color.NRGBA{R: 0x80, G: 0x00, B: 0x1E, A: 0xFF}
	colRed := color.NRGBA{R: 0xC0, G: 0x20, B: 0x20, A: 0xFF}
	colRedD := color.NRGBA{R: 0x90, G: 0x10, B: 0x10, A: 0xFF}
	colGreen := color.NRGBA{R: 0x2E, G: 0x8B, B: 0x57, A: 0xFF}
	colGreenD := color.NRGBA{R: 0x1E, G: 0x6B, B: 0x40, A: 0xFF}
	colBlk := color.NRGBA{R: 0x10, G: 0x10, B: 0x10, A: 0xFF}

	// mensagem em bordô, quebra de linha automática dentro da margem
	layout.Inset{Top: unit.Dp(8), Left: unit.Dp(14), Right: unit.Dp(14)}.Layout(gtx,
		func(gtx layout.Context) layout.Dimensions {
			return ui.lbl(gtx, ui.fontMenuBold, unit.Sp(12), msg, colBordo)
		})

	dragArea(gtx, R(0, 0, 250, 22))
	dragAreaX11(gtx, R(0, 0, 250, 22), nil)

	// Não à esquerda (verde), Sim à direita (vermelho)
	no := ui.button(gtx, &ui.confirmBtns[0], R(60, 50, 60, 20),
		func(gtx layout.Context, down bool) {
			b := &ui.confirmBtns[0]
			bg := colGreen
			if down || b.over {
				bg = colGreenD
			}
			fillRect(gtx.Ops, R(0, 0, 60, 20), bg)
			ui.textAt(gtx, ui.fontMenuBold, unit.Sp(12), language.T("confirm.no"), colBlk, image.Pt(23, 4))
		})
	yes := ui.button(gtx, &ui.confirmBtns[1], R(130, 50, 60, 20),
		func(gtx layout.Context, down bool) {
			b := &ui.confirmBtns[1]
			bg := colRed
			if down || b.over {
				bg = colRedD
			}
			fillRect(gtx.Ops, R(0, 0, 60, 20), bg)
			ui.textAt(gtx, ui.fontMenuBold, unit.Sp(12), language.T("confirm.yes"), colBlk, image.Pt(23, 4))
		})
	if yes {
		switch ui.confirmKind {
		case 1:
			ui.p.ResetShuffleCounts()
		case 2:
			ui.p.ResetPlayCounts()
		case 3:
			ui.p.ResetTags()
		}
		ui.closeConfirm()
	}
	if no {
		ui.closeConfirm()
	}
}
