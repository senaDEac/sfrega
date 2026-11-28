package native

import (
	"image"
	"image/color"
	"os"
	"path/filepath"
	"strings"

	"gioui.org/app"
	"gioui.org/f32"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
)

// androidMediaDirs retorna os diretórios de mídia do aparelho: nomes
// conhecidos + qualquer pasta de 1º nível do /sdcard com áudio direto.
func androidMediaDirs() []string {
	var out []string
	seen := map[string]bool{}
	add := func(d string) {
		if seen[d] {
			return
		}
		if st, err := os.Stat(d); err == nil && st.IsDir() {
			seen[d] = true
			out = append(out, d)
		}
	}
	for _, d := range []string{
		"/sdcard/Music", "/sdcard/Music2", "/sdcard/Download",
		"/sdcard/Music/Downloaded", "/sdcard/Recordings",
		"/sdcard/Música", "/sdcard/Baixados", "/sdcard/Audiobooks",
		"/sdcard/Podcasts", "/sdcard/Ringtones", "/sdcard/Alarms",
		"/sdcard/Notifications", "/sdcard/Midia", "/sdcard/Media",
	} {
		add(d)
	}
	// pastas próprias do usuário: inclui as que têm áudio logo dentro
	if entries, err := os.ReadDir("/sdcard"); err == nil {
		for _, en := range entries {
			if !en.IsDir() {
				continue
			}
			name := en.Name()
			if name == "Android" || name == "DCIM" {
				continue
			}
			d := "/sdcard/" + name
			if sub, err := os.ReadDir(d); err == nil {
				for _, f := range sub {
					if !f.IsDir() && audioExts[strings.ToLower(filepath.Ext(f.Name()))] {
						add(d)
						break
					}
				}
			}
		}
	}
	return out
}

// androidImportMedia importa as pastas de mídia padrão para a janela pl.
func androidImportMedia(pl *plw) {
	for _, d := range androidMediaDirs() {
		pl.addDir(d)
		pl.addTracks(audioFilesInDir(d))
	}
}

// layoutAndroid desenha tudo na janela única do Android:
// player no topo (centralizado) e UM painel na área livre abaixo.
// Coordenadas internas são "lógicas" (px/s) — a affine escala tudo.
func (ui *uiState) layoutAndroid(gtx layout.Context, e app.FrameEvent) {
	W, H := e.Size.X, e.Size.Y

	// o player ocupa toda a extensão horizontal: escala = largura/275
	// (fracionária — sprites escalam junto, sem sobra nas laterais)
	s := float32(W) / 275
	if s < 1 {
		s = 1
	}
	op.Affine(f32.Affine2D{}.Scale(f32.Pt(0, 0), f32.Pt(s, s))).Add(gtx.Ops)

	lw := int(float32(W) / s)
	lh := int(float32(H) / s)

	// fundo escuro opaco: no Android a janela/surface não consegue
	// mostrar o wallpaper de forma confiável (MIUI impõe backdrop
	// claro) — preenchimento sólido evita o "branco puro"
	paint.FillShape(gtx.Ops, color.NRGBA{R: 0x14, G: 0x14, B: 0x18, A: 0xff},
		clip.Rect(image.Rect(0, 0, lw, lh)).Op())

	pw, ph := 275, 107
	if ui.shadeMain.on {
		ph = 18
	}
	ox := (lw - pw) / 2
	if ox < 0 {
		ox = 0
	}

	mk := func(w, h int) layout.Context {
		c := gtx
		c.Constraints = layout.Constraints{Max: image.Pt(w, h)}
		// desktop assume 1px por dp/sp; no Android a densidade (~3x)
		// inflaria todos os textos/insets — forçamos 1:1 lógico
		c.Metric = unit.Metric{PxPerDp: 1, PxPerSp: 1}
		return c
	}

	// ---- player no topo ----
	poff := op.Offset(image.Pt(ox, 0)).Push(gtx.Ops)
	ui.layoutMain(mk(pw, ph))
	poff.Pop()

	// ---- área do painel: resto da tela abaixo do player ----
	top := ph
	avail := lh - top
	if avail <= 0 {
		return
	}
	hasPanel := ui.panelPL != nil || ui.panelKind != panelNone
	if !hasPanel && !ui.confirmOpen {
		return
	}

	// clip impede o painel de invadir a área do player
	cc := clip.Rect(image.Rect(0, top, lw, lh)).Push(gtx.Ops)
	poff2 := op.Offset(image.Pt(0, top)).Push(gtx.Ops)
	pc := mk(lw, avail)

	switch {
	case ui.panelPL != nil:
		ui.panelPL.layoutPL(pc)
	case ui.panelKind == panelEQ:
		x := (lw - 275) / 2
		if x < 0 {
			x = 0
		}
		eoff := op.Offset(image.Pt(x, 0)).Push(gtx.Ops)
		ui.layoutEQ(mk(275, 116))
		eoff.Pop()
	case ui.panelKind == panelMenu:
		x := (lw - 275) / 2
		if x < 0 {
			x = 0
		}
		moff := op.Offset(image.Pt(x, 0)).Push(gtx.Ops)
		ui.layoutMenu(mk(275, 232))
		moff.Pop()
	}

	// confirmação: overlay centralizado sobre o painel
	if ui.confirmOpen {
		cx := (lw - 250) / 2
		cy := (avail - 80) / 2
		if cx < 0 {
			cx = 0
		}
		if cy < 0 {
			cy = 0
		}
		coff := op.Offset(image.Pt(cx, cy)).Push(gtx.Ops)
		ui.layoutConfirm(mk(250, 80))
		coff.Pop()
	}

	poff2.Pop()
	cc.Pop()
}
