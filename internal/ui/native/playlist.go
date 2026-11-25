package native

import (
	"fmt"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gioui.org/app"
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

	"sfrega/internal/language"
)

var plResTag event.Tag
var plShadeResTag event.Tag
var plWheelTag event.Tag
var plArtistWheelTag event.Tag
var plScrollTag event.Tag
var plArtistScrollTag event.Tag
var plFolderWheelTag event.Tag
var plFolderScrollTag event.Tag
var plAlbumWheelTag event.Tag
var plAlbumScrollTag event.Tag

// plWheel rola a lista com a roda do mouse (mesmo padrão do menuWindow).
func (ui *uiState) plWheel(gtx layout.Context, r image.Rectangle) {
	cs := clip.Rect(r).Push(gtx.Ops)
	event.Op(gtx.Ops, &plWheelTag)
	cs.Pop()
	for {
		ev, ok := gtx.Event(pointer.Filter{
			Target:  &plWheelTag,
			Kinds:   pointer.Scroll,
			ScrollX: pointer.ScrollRange{Min: -1 << 30, Max: 1 << 30},
			ScrollY: pointer.ScrollRange{Min: -1 << 30, Max: 1 << 30},
		})
		if !ok {
			break
		}
		e, ok := ev.(pointer.Event)
		if !ok {
			continue
		}
		ui.plList.ScrollBy(e.Scroll.Y / plRowH)
	}
}

// plArtistWheel rola a fileira de artistas na área vazia do painel.
func (ui *uiState) plArtistWheel(gtx layout.Context, r image.Rectangle) {
	cs := clip.Rect(r).Push(gtx.Ops)
	event.Op(gtx.Ops, &plArtistWheelTag)
	cs.Pop()
	for {
		ev, ok := gtx.Event(pointer.Filter{
			Target:  &plArtistWheelTag,
			Kinds:   pointer.Scroll,
			ScrollX: pointer.ScrollRange{Min: -1 << 30, Max: 1 << 30},
			ScrollY: pointer.ScrollRange{Min: -1 << 30, Max: 1 << 30},
		})
		if !ok {
			break
		}
		e, ok := ev.(pointer.Event)
		if !ok {
			continue
		}
		ui.plArtistScrollAcc += e.Scroll.Y
	}
}

// plFolderWheel rola o painel de pastas na área vazia.
func (ui *uiState) plFolderWheel(gtx layout.Context, r image.Rectangle) {
	cs := clip.Rect(r).Push(gtx.Ops)
	event.Op(gtx.Ops, &plFolderWheelTag)
	cs.Pop()
	for {
		ev, ok := gtx.Event(pointer.Filter{
			Target:  &plFolderWheelTag,
			Kinds:   pointer.Scroll,
			ScrollX: pointer.ScrollRange{Min: -1 << 30, Max: 1 << 30},
			ScrollY: pointer.ScrollRange{Min: -1 << 30, Max: 1 << 30},
		})
		if !ok {
			break
		}
		e, ok := ev.(pointer.Event)
		if !ok {
			continue
		}
		ui.plFolderScrollAcc += e.Scroll.Y
	}
}

// Cores do PLEDIT.TXT da skin base-2.91.
var (
	colPLBg      = color.NRGBA{R: 0x00, G: 0x00, B: 0x00, A: 0xFF}
	colPLText    = color.NRGBA{R: 0x00, G: 0xFF, B: 0x00, A: 0xFF} // Normal=#00FF00
	colPLCur     = color.NRGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF} // Current=#FFFFFF
	colPLSelBg   = color.NRGBA{R: 0x00, G: 0x00, B: 0xC6, A: 0xFF} // SelectedBG=#0000C6
	colPLSelText = color.NRGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF}
	colPLFile    = color.NRGBA{R: 0x40, G: 0xD0, B: 0x40, A: 0xFF} // fallback sem metadados
	colPLCurBg   = color.NRGBA{R: 0x00, G: 0x00, B: 0x8C, A: 0xFF} // fundo azul da faixa tocando
	colPLExpBg   = color.NRGBA{R: 0x28, G: 0x28, B: 0x28, A: 0xFF} // fundo cinza das faixas expandidas
)

// Layout da janela playlist conforme WebAmp/PLEDIT:
//   topo 20px (tiles + shade/close), meio (lista + scrollbar),
//   rodapé 38px (4 menus suspensos à esquerda, tempo+transporte+list menu à direita).

const (
	plTopH  = 20
	plBotH  = 38
	plRowH  = 42
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
			if name, _ := ui.p.TrackTitle(idx); name != "" {
				green := color.NRGBA{R: 0x00, G: 0xE2, B: 0x00, A: 0xFF}
				cs := clip.Rect(image.Rect(bx0+6, by, bx1-11, by+bh)).Push(gtx.Ops)
				ui.textAt(gtx, ui.fontMenu, unit.Sp(9), name, green, image.Pt(bx0+7, by+2))
				cs.Pop()
			}
		}
	}

	// área de move exclui a zona de resize (winW-60..winW-33)
	dragArea(gtx, R(0, 0, winW-61, 18))
	dragArea(gtx, R(winW-32, 0, 1, 18))
	dragAreaX11(gtx, R(0, 0, winW-61, 18), &ui.shadePL)
	dragAreaX11(gtx, R(winW-32, 0, 1, 18), &ui.shadePL)
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

	ui.plShadeResize(gtx, winW)
}

// plShadeResize registra a área do triângulo no fim do campo do nome
// (parte final do sprite da capa direita) para arrastar a largura da janela.
func (ui *uiState) plShadeResize(gtx layout.Context, winW int) {
	// cobre a última decoração do campo do nome (capa+triângulo) e ~10px
	// à esquerda dela, até perto dos botões
	r := image.Rect(winW-60, 3, winW-33, 15)
	clipStack := clip.Rect(r).Push(gtx.Ops)
	event.Op(gtx.Ops, &plShadeResTag)
	clipStack.Pop()
	for {
		ev, ok := gtx.Event(pointer.Filter{
			Target: &plShadeResTag,
			Kinds:  pointer.Press,
		})
		if !ok {
			break
		}
		e, ok := ev.(pointer.Event)
		if !ok || e.Kind != pointer.Press || e.Buttons != pointer.ButtonPrimary {
			continue
		}
		scale := ui.plScale
		if scale < 1 {
			scale = 1
		}
		w0, h0 := winW*scale, ui.shadePL.px
		if h0 <= 0 {
			h0 = 18 * scale
		}
		go func() {
			wmResizeWDrag("Playlist", w0, h0, 300*scale)
			time.Sleep(150 * time.Millisecond)
			ui.savePLWidth()
		}()
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
	labels := []string{
		language.T("pl.btn.genre"), language.T("pl.btn.songs"),
		language.T("pl.btn.artist"), language.T("pl.btn.folders"),
		language.T("pl.btn.albums"), language.T("pl.btn.list"),
	}
	dark := color.NRGBA{R: 0x10, G: 0x10, B: 0x18, A: 0xFF}
	genGreen := color.NRGBA{R: 0x80, G: 0x00, B: 0x20, A: 0xFF} // bordeaux
	for i, gb := range gbtns {
		bx := 14 + i*47
		dst := R(bx, plTopH+3, 45, 11)
		if ui.spriteBtn(gtx, gb, "GENEX", dst,
			R(0, 0, 48, 16), R(0, 16, 48, 15)) {
			if i == 0 {
				ui.plGenreOpen = !ui.plGenreOpen
				saveGenreOpen(ui.plGenreOpen)
				if ui.plGenreOpen && ui.plGenreSel != "" {
					ui.plGenreOn = true
					saveGenreOn(true)
				}
				// só um painel por vez; as seleções são mantidas
				ui.plArtistOpen = false
				saveArtistOpen(false)
				ui.plFolderOpen = false
				saveFolderOpen(false)
				ui.plAlbumOpen = false
				saveAlbumOpen(false)
			}
			if i == 2 { // ARTISTA: abre/fecha a lista de artistas
				ui.plArtistOpen = !ui.plArtistOpen
				saveArtistOpen(ui.plArtistOpen)
				ui.plGenreOpen = false
				saveGenreOpen(false)
				ui.plFolderOpen = false
				saveFolderOpen(false)
				ui.plAlbumOpen = false
				saveAlbumOpen(false)
			}
			if i == 3 { // PASTAS: abre/fecha a lista de diretórios
				ui.plFolderOpen = !ui.plFolderOpen
				saveFolderOpen(ui.plFolderOpen)
				ui.plGenreOpen = false
				saveGenreOpen(false)
				ui.plArtistOpen = false
				saveArtistOpen(false)
				ui.plAlbumOpen = false
				saveAlbumOpen(false)
			}
			if i == 4 { // ÁLBUNS: abre/fecha a lista de álbuns
				ui.plAlbumOpen = !ui.plAlbumOpen
				saveAlbumOpen(ui.plAlbumOpen)
				ui.plGenreOpen = false
				saveGenreOpen(false)
				ui.plArtistOpen = false
				saveArtistOpen(false)
				ui.plFolderOpen = false
				saveFolderOpen(false)
			}
			if i == 1 { // MÚSICAS: exibe todas as faixas e fecha os painéis
				ui.plGenreOpen = false
				ui.plGenreOn = false
				saveGenreOn(false)
				saveGenreOpen(false)
				ui.plArtistOpen = false
				ui.plArtistExpand = ""
				ui.plArtistExpandIdx = nil
				saveArtistOpen(false)
				ui.plFolderOpen = false
				ui.plFolderExpand = ""
				ui.plFolderExpandIdx = nil
				saveFolderOpen(false)
				ui.plAlbumOpen = false
				ui.plAlbumExpand = ""
				ui.plAlbumExpandIdx = nil
				saveAlbumOpen(false)
			}
			if i == 5 { // LIST: fecha painéis e desliga o filtro, mantendo as seleções
				ui.plGenreOpen = false
				ui.plGenreOn = false
				saveGenreOn(false)
				saveGenreOpen(false)
				ui.plArtistOpen = false
				saveArtistOpen(false)
				ui.plFolderOpen = false
				saveFolderOpen(false)
				ui.plAlbumOpen = false
				saveAlbumOpen(false)
			}
			if ui.plGenSel == i {
				ui.plGenSel = -1
			} else {
				ui.plGenSel = i
			}
			saveGenSel(ui.plGenSel)
			saveExpand("artistexp.txt", ui.plArtistExpand)
			saveExpand("folderexp.txt", ui.plFolderExpand)
			saveExpand("albumexp.txt", ui.plAlbumExpand)
		}
		lc := dark
		sel := ui.plGenSel == i || (i == 0 && ui.plGenreOn && ui.plGenreSel != "" && !ui.plArtistOpen && !ui.plFolderOpen && !ui.plAlbumOpen) || (i == 2 && ui.plArtistOpen) || (i == 3 && ui.plFolderOpen) || (i == 4 && ui.plAlbumOpen)
		if sel {
			ui.btnTextB(gtx, labels[i], dst, genGreen)
			// borda bordeaux no botão selecionado
			bx0, by0 := dst.Min.X, dst.Min.Y
			bx1, by1 := dst.Max.X, dst.Max.Y
			fillRect(gtx.Ops, image.Rect(bx0, by0, bx1, by0+2), genGreen)
			fillRect(gtx.Ops, image.Rect(bx0, by1-2, bx1, by1), genGreen)
			fillRect(gtx.Ops, image.Rect(bx0, by0, bx0+2, by1), genGreen)
			fillRect(gtx.Ops, image.Rect(bx1-2, by0, bx1, by1), genGreen)
		} else {
			ui.btnText(gtx, labels[i], dst, lc)
		}
	}

	// fileira de gêneros abaixo dos botões GENEX (GENRE abre/fecha)
	genreH := 0
	if ui.plGenreOpen {
		genres := ui.p.Genres()
		for len(ui.plGenreBtns) < len(genres) {
			ui.plGenreBtns = append(ui.plGenreBtns, uibtn{})
		}
		x := 14
		for gi, g := range genres {
			gw := 12 + len([]rune(g))*6
			if gw > 160 {
				gw = 160
			}
			if x+gw > winW-23 {
				break
			}
			dst := R(x, plTopH+16, gw, 11)
			if ui.spriteBtn(gtx, &ui.plGenreBtns[gi], "GENEX", dst,
				R(0, 0, 48, 16), R(0, 16, 48, 15)) {
				if ui.plGenreSel == g {
					if ui.plGenreOn {
						ui.plGenreSel = ""
						ui.plGenreOn = false
					} else {
						ui.plGenreOn = true
					}
				} else {
					ui.plGenreSel = g
					ui.plGenreOn = true
				}
				saveGenreSel(ui.plGenreSel)
				saveGenreOn(ui.plGenreOn)
			}
			gc := dark
			if ui.plGenreSel == g {
				gc = genGreen
			}
			ui.btnTextB(gtx, g, dst, gc)
			if ui.plGenreSel == g {
				bx0, by0 := dst.Min.X, dst.Min.Y
				bx1, by1 := dst.Max.X, dst.Max.Y
				fillRect(gtx.Ops, image.Rect(bx0, by0, bx1, by0+2), genGreen)
				fillRect(gtx.Ops, image.Rect(bx0, by1-2, bx1, by1), genGreen)
				fillRect(gtx.Ops, image.Rect(bx0, by0, bx0+2, by1), genGreen)
				fillRect(gtx.Ops, image.Rect(bx1-2, by0, bx1, by1), genGreen)
			}
			x += gw + 2
		}
		genreH = 13
	}

	// fileira de artistas abaixo dos botões GENEX (ARTISTA abre/fecha)
	// um artista por linha, ordenado, com scroll próprio
	artistH := 0
	if ui.plArtistOpen {
		artists := ui.p.Artists() // já vem ordenada alfabeticamente
		for len(ui.plArtistBtns) < len(artists) {
			ui.plArtistBtns = append(ui.plArtistBtns, uibtn{})
		}
		// restaura a expansão persistida quando as faixas já estão carregadas
		if ui.plArtistExpand != "" && ui.plArtistExpandIdx == nil {
			for _, a2 := range artists {
				if a2 == ui.plArtistExpand {
					ui.plArtistExpandIdx = ui.artistTracks(a2, tracks)
					break
				}
			}
		}
		// faixas do artista expandido (só se ainda existir na lista)
		expN := 0
		if ui.plArtistExpand != "" {
			for _, a2 := range artists {
				if a2 == ui.plArtistExpand {
					expN = len(ui.plArtistExpandIdx)
					break
				}
			}
		}
		areaTop := plTopH + 16 + genreH
		avail := botY - 3 - areaTop
		contentH := (len(artists) + expN) * plRowH
		artistH = contentH
		if artistH > avail {
			artistH = avail
		}
		if artistH < 0 {
			artistH = 0
		}
		ui.plCount = len(artists)
		ui.plArtistMax = float32(contentH - artistH)
		if ui.plArtistMax < 0 {
			ui.plArtistMax = 0
		}
		if ui.plArtistScroll > ui.plArtistMax {
			ui.plArtistScroll = ui.plArtistMax
		}
		if ui.plArtistScroll < 0 {
			ui.plArtistScroll = 0
		}
		panel := image.Rect(12, areaTop, winW-23, areaTop+artistH)
		fillRect(gtx.Ops, panel, colPLBg)
		cs := clip.Rect(panel).Push(gtx.Ops)
		// scroll via deslocamento de ry (sem op.Offset: quebrava a área de eventos)
		ry := areaTop - int(ui.plArtistScroll)
		// roda primeiro (embaixo): se fosse por cima, engoliria os Press dos botões
		ui.plArtistWheel(gtx, panel)
		ui.plScrollSink = &ui.plArtistScrollAcc
		// só renderiza linhas visíveis (evita decode de capa / ops de 300+ linhas)
		visMin := areaTop - plRowH
		visMax := areaTop + artistH
		for ai, a := range artists {
			rowR := image.Rect(14, ry, winW-27, ry+plRowH)
			exp := ui.plArtistExpand == a
			if ry > visMin && ry < visMax {
				if exp {
					fillRect(gtx.Ops, rowR, colPLCurBg)
					bx0, by0 := rowR.Min.X, rowR.Min.Y
					bx1, by1 := rowR.Max.X, rowR.Max.Y
					fillRect(gtx.Ops, image.Rect(bx0, by0, bx1, by0+2), genGreen)
					fillRect(gtx.Ops, image.Rect(bx0, by1-2, bx1, by1), genGreen)
					fillRect(gtx.Ops, image.Rect(bx0, by0, bx0+2, by1), genGreen)
					fillRect(gtx.Ops, image.Rect(bx1-2, by0, bx1, by1), genGreen)
				}
				ac := colPLText
				if exp {
					ac = colPLSelText
				}
				if img := ui.p.ArtistCover(a); img != nil {
					blit(gtx, img, image.Rect(15, ry+1, 55, ry+41))
				}
				ui.textAt(gtx, ui.fontUI, unit.Sp(9), a, ac, image.Pt(59, ry+9))
				if n := ui.p.ArtistTrackCount(a); n > 0 {
					cnt := fmt.Sprintf(language.T("pl.artist.tracks"), n)
					ui.textAt(gtx, ui.fontUI, unit.Sp(9), cnt, colPLFile, image.Pt(59, ry+25))
				}
				ui.plArtistBtns[ai].scrollFn = func(dy float32) { ui.plArtistScrollAcc += dy }
				b := &ui.plArtistBtns[ai]
				if ui.button(gtx, b, rowR, func(gtx layout.Context, down bool) {}) {
					now := time.Now()
					dbl := now.Sub(b.lastClick) < 500*time.Millisecond
					b.lastClick = now
					if dbl {
						b.lastClick = time.Time{}
						if exp {
							ui.plArtistExpand = ""
							ui.plArtistExpandIdx = nil
						} else {
							ui.plArtistExpand = a
							ui.plArtistExpandIdx = ui.artistTracks(a, tracks)
						}
						saveExpand("artistexp.txt", ui.plArtistExpand)
					}
				}
			}
			ry += plRowH
			// músicas do artista logo abaixo, no formato das linhas de música
			if exp {
				for _, ti := range ui.plArtistExpandIdx {
					if ry > visMin && ry < visMax {
						ro := op.Offset(image.Pt(54, ry)).Push(gtx.Ops)
						fillRect(gtx.Ops, image.Rect(0, 0, winW-81, plRowH), colPLExpBg)
						ui.plRow(gtx, ti, tracks, winW-81)
						er := image.Rect(0, 0, winW-81, plRowH)
						bc := color.NRGBA{A: 0xFF}
						if ti == ui.p.Index() {
							bc = colPLText
						}
						fillRect(gtx.Ops, image.Rect(er.Min.X, er.Min.Y, er.Max.X, er.Min.Y+1), bc)
						fillRect(gtx.Ops, image.Rect(er.Min.X, er.Max.Y-1, er.Max.X, er.Max.Y), bc)
						fillRect(gtx.Ops, image.Rect(er.Min.X, er.Min.Y, er.Min.X+1, er.Max.Y), bc)
						fillRect(gtx.Ops, image.Rect(er.Max.X-1, er.Min.Y, er.Max.X, er.Max.Y), bc)
						ro.Pop()
					}
					ry += plRowH
				}
			}
		}
		cs.Pop()
		// aplica o scroll acumulado pela roda (botões e área vazia)
		if ui.plArtistScrollAcc != 0 {
			ui.plArtistScroll -= ui.plArtistScrollAcc
			if ui.plArtistScroll > ui.plArtistMax {
				ui.plArtistScroll = ui.plArtistMax
			}
			if ui.plArtistScroll < 0 {
				ui.plArtistScroll = 0
			}
			ui.plArtistScrollAcc = 0
		}
		// scrollbar quando o conteúdo excede a área visível
		if contentH > artistH && artistH > 0 {
			track := image.Rect(winW-20, areaTop+1, winW-16, areaTop+artistH-1)
			fillRect(gtx.Ops, track, colPLBg)
			thumbH := artistH * artistH / contentH
			if thumbH < 12 {
				thumbH = 12
			}
			if thumbH > artistH-2 {
				thumbH = artistH - 2
			}
			frac := float32(0)
			if ui.plArtistMax > 0 {
				frac = ui.plArtistScroll / ui.plArtistMax
			}
			thumbY := areaTop + 1 + int(frac*float32(artistH-2-thumbH))
			fillRect(gtx.Ops, image.Rect(winW-20, thumbY, winW-16, thumbY+thumbH),
				color.NRGBA{R: 0x80, G: 0x80, B: 0x80, A: 0xFF})
			ui.plArtistScrollDrag(gtx, track, thumbY, thumbH, artistH, ui.plArtistMax)
		}
	}

	// painel de pastas: mesmo formato do de artistas, agrupado por diretório
	if ui.plFolderOpen {
		folders := ui.p.Folders()
		for len(ui.plFolderBtns) < len(folders) {
			ui.plFolderBtns = append(ui.plFolderBtns, uibtn{})
		}
		if ui.plFolderExpand != "" && ui.plFolderExpandIdx == nil {
			for _, d := range folders {
				if d == ui.plFolderExpand {
					ui.plFolderExpandIdx = ui.folderTracks(d, tracks)
					break
				}
			}
		}
		expN := 0
		if ui.plFolderExpand != "" {
			for _, d := range folders {
				if d == ui.plFolderExpand {
					expN = len(ui.plFolderExpandIdx)
					break
				}
			}
		}
		areaTop := plTopH + 16 + genreH
		avail := botY - 3 - areaTop
		contentH := (len(folders) + expN) * plRowH
		folderH := contentH
		if folderH > avail {
			folderH = avail
		}
		if folderH < 0 {
			folderH = 0
		}
		ui.plCount = len(folders)
		ui.plFolderMax = float32(contentH - folderH)
		if ui.plFolderMax < 0 {
			ui.plFolderMax = 0
		}
		if ui.plFolderScroll > ui.plFolderMax {
			ui.plFolderScroll = ui.plFolderMax
		}
		if ui.plFolderScroll < 0 {
			ui.plFolderScroll = 0
		}
		panel := image.Rect(12, areaTop, winW-23, areaTop+folderH)
		fillRect(gtx.Ops, panel, colPLBg)
		cs := clip.Rect(panel).Push(gtx.Ops)
		// scroll via deslocamento de ry (sem op.Offset: quebrava a área de eventos)
		ry := areaTop - int(ui.plFolderScroll)
		// roda primeiro (embaixo): se fosse por cima, engoliria os Press dos botões
		ui.plFolderWheel(gtx, panel)
		ui.plScrollSink = &ui.plFolderScrollAcc
		visMin := areaTop - plRowH
		visMax := areaTop + folderH
		for di, dir := range folders {
			rowR := image.Rect(14, ry, winW-27, ry+plRowH)
			exp := ui.plFolderExpand == dir
			if ry > visMin && ry < visMax {
				if exp {
					fillRect(gtx.Ops, rowR, colPLCurBg)
					bx0, by0 := rowR.Min.X, rowR.Min.Y
					bx1, by1 := rowR.Max.X, rowR.Max.Y
					fillRect(gtx.Ops, image.Rect(bx0, by0, bx1, by0+2), genGreen)
					fillRect(gtx.Ops, image.Rect(bx0, by1-2, bx1, by1), genGreen)
					fillRect(gtx.Ops, image.Rect(bx0, by0, bx0+2, by1), genGreen)
					fillRect(gtx.Ops, image.Rect(bx1-2, by0, bx1, by1), genGreen)
				}
				ac := colPLText
				if exp {
					ac = colPLSelText
				}
				if img := ui.p.FolderCover(dir); img != nil {
					blit(gtx, img, image.Rect(15, ry+1, 55, ry+41))
				}
				ui.textAt(gtx, ui.fontUI, unit.Sp(9), ui.p.FolderName(dir), ac, image.Pt(59, ry+9))
				if n := ui.p.FolderTrackCount(dir); n > 0 {
					cnt := fmt.Sprintf(language.T("pl.artist.tracks"), n)
					ui.textAt(gtx, ui.fontUI, unit.Sp(9), cnt, colPLFile, image.Pt(59, ry+25))
				}
				ui.plFolderBtns[di].scrollFn = func(dy float32) { ui.plFolderScrollAcc += dy }
				b := &ui.plFolderBtns[di]
				if ui.button(gtx, b, rowR, func(gtx layout.Context, down bool) {}) {
					now := time.Now()
					dbl := now.Sub(b.lastClick) < 500*time.Millisecond
					b.lastClick = now
					if dbl {
						b.lastClick = time.Time{}
						if exp {
							ui.plFolderExpand = ""
							ui.plFolderExpandIdx = nil
						} else {
							ui.plFolderExpand = dir
							ui.plFolderExpandIdx = ui.folderTracks(dir, tracks)
						}
						saveExpand("folderexp.txt", ui.plFolderExpand)
					}
				}
			}
			ry += plRowH
			if exp {
				for _, ti := range ui.plFolderExpandIdx {
					if ry > visMin && ry < visMax {
						ro := op.Offset(image.Pt(54, ry)).Push(gtx.Ops)
						fillRect(gtx.Ops, image.Rect(0, 0, winW-81, plRowH), colPLExpBg)
						ui.plRow(gtx, ti, tracks, winW-81)
						er := image.Rect(0, 0, winW-81, plRowH)
						bc := color.NRGBA{A: 0xFF}
						if ti == ui.p.Index() {
							bc = colPLText
						}
						fillRect(gtx.Ops, image.Rect(er.Min.X, er.Min.Y, er.Max.X, er.Min.Y+1), bc)
						fillRect(gtx.Ops, image.Rect(er.Min.X, er.Max.Y-1, er.Max.X, er.Max.Y), bc)
						fillRect(gtx.Ops, image.Rect(er.Min.X, er.Min.Y, er.Min.X+1, er.Max.Y), bc)
						fillRect(gtx.Ops, image.Rect(er.Max.X-1, er.Min.Y, er.Max.X, er.Max.Y), bc)
						ro.Pop()
					}
					ry += plRowH
				}
			}
		}
		cs.Pop()
		if ui.plFolderScrollAcc != 0 {
			ui.plFolderScroll -= ui.plFolderScrollAcc
			if ui.plFolderScroll > ui.plFolderMax {
				ui.plFolderScroll = ui.plFolderMax
			}
			if ui.plFolderScroll < 0 {
				ui.plFolderScroll = 0
			}
			ui.plFolderScrollAcc = 0
		}
		if contentH > folderH && folderH > 0 {
			track := image.Rect(winW-20, areaTop+1, winW-16, areaTop+folderH-1)
			fillRect(gtx.Ops, track, colPLBg)
			thumbH := folderH * folderH / contentH
			if thumbH < 12 {
				thumbH = 12
			}
			if thumbH > folderH-2 {
				thumbH = folderH - 2
			}
			frac := float32(0)
			if ui.plFolderMax > 0 {
				frac = ui.plFolderScroll / ui.plFolderMax
			}
			thumbY := areaTop + 1 + int(frac*float32(folderH-2-thumbH))
			fillRect(gtx.Ops, image.Rect(winW-20, thumbY, winW-16, thumbY+thumbH),
				color.NRGBA{R: 0x80, G: 0x80, B: 0x80, A: 0xFF})
			ui.plFolderScrollDrag(gtx, track, thumbY, thumbH, folderH, ui.plFolderMax)
		}
	}

	// painel de álbuns: mesmo formato, agrupado pela tag ALBUM/TALB
	if ui.plAlbumOpen {
		albums := ui.p.Albums()
		for len(ui.plAlbumBtns) < len(albums) {
			ui.plAlbumBtns = append(ui.plAlbumBtns, uibtn{})
		}
		if ui.plAlbumExpand != "" && ui.plAlbumExpandIdx == nil {
			for _, al := range albums {
				if al == ui.plAlbumExpand {
					ui.plAlbumExpandIdx = ui.albumTracks(al, tracks)
					break
				}
			}
		}
		expN := 0
		if ui.plAlbumExpand != "" {
			for _, al := range albums {
				if al == ui.plAlbumExpand {
					expN = len(ui.plAlbumExpandIdx)
					break
				}
			}
		}
		areaTop := plTopH + 16 + genreH
		avail := botY - 3 - areaTop
		contentH := (len(albums) + expN) * plRowH
		albumH := contentH
		if albumH > avail {
			albumH = avail
		}
		if albumH < 0 {
			albumH = 0
		}
		ui.plCount = len(albums)
		ui.plAlbumMax = float32(contentH - albumH)
		if ui.plAlbumMax < 0 {
			ui.plAlbumMax = 0
		}
		if ui.plAlbumScroll > ui.plAlbumMax {
			ui.plAlbumScroll = ui.plAlbumMax
		}
		if ui.plAlbumScroll < 0 {
			ui.plAlbumScroll = 0
		}
		panel := image.Rect(12, areaTop, winW-23, areaTop+albumH)
		fillRect(gtx.Ops, panel, colPLBg)
		cs := clip.Rect(panel).Push(gtx.Ops)
		ry := areaTop - int(ui.plAlbumScroll)
		// roda primeiro (embaixo): se fosse por cima, engoliria os Press dos botões
		ui.plAlbumWheel(gtx, panel)
		ui.plScrollSink = &ui.plAlbumScrollAcc
		visMin := areaTop - plRowH
		visMax := areaTop + albumH
		for ai2, al := range albums {
			rowR := image.Rect(14, ry, winW-27, ry+plRowH)
			exp := ui.plAlbumExpand == al
			if ry > visMin && ry < visMax {
				if exp {
					fillRect(gtx.Ops, rowR, colPLCurBg)
					bx0, by0 := rowR.Min.X, rowR.Min.Y
					bx1, by1 := rowR.Max.X, rowR.Max.Y
					fillRect(gtx.Ops, image.Rect(bx0, by0, bx1, by0+2), genGreen)
					fillRect(gtx.Ops, image.Rect(bx0, by1-2, bx1, by1), genGreen)
					fillRect(gtx.Ops, image.Rect(bx0, by0, bx0+2, by1), genGreen)
					fillRect(gtx.Ops, image.Rect(bx1-2, by0, bx1, by1), genGreen)
				}
				ac := colPLText
				if exp {
					ac = colPLSelText
				}
				if img := ui.p.AlbumCover(al); img != nil {
					blit(gtx, img, image.Rect(15, ry+1, 55, ry+41))
				}
				ui.textAt(gtx, ui.fontUI, unit.Sp(9), al, ac, image.Pt(59, ry+9))
				if n := ui.p.AlbumTrackCount(al); n > 0 {
					cnt := fmt.Sprintf(language.T("pl.artist.tracks"), n)
					ui.textAt(gtx, ui.fontUI, unit.Sp(9), cnt, colPLFile, image.Pt(59, ry+25))
				}
				ui.plAlbumBtns[ai2].scrollFn = func(dy float32) { ui.plAlbumScrollAcc += dy }
				b := &ui.plAlbumBtns[ai2]
				if ui.button(gtx, b, rowR, func(gtx layout.Context, down bool) {}) {
					now := time.Now()
					dbl := now.Sub(b.lastClick) < 500*time.Millisecond
					b.lastClick = now
					if dbl {
						b.lastClick = time.Time{}
						if exp {
							ui.plAlbumExpand = ""
							ui.plAlbumExpandIdx = nil
						} else {
							ui.plAlbumExpand = al
							ui.plAlbumExpandIdx = ui.albumTracks(al, tracks)
						}
						saveExpand("albumexp.txt", ui.plAlbumExpand)
					}
				}
			}
			ry += plRowH
			if exp {
				for _, ti := range ui.plAlbumExpandIdx {
					if ry > visMin && ry < visMax {
						ro := op.Offset(image.Pt(54, ry)).Push(gtx.Ops)
						fillRect(gtx.Ops, image.Rect(0, 0, winW-81, plRowH), colPLExpBg)
						ui.plRow(gtx, ti, tracks, winW-81)
						er := image.Rect(0, 0, winW-81, plRowH)
						bc := color.NRGBA{A: 0xFF}
						if ti == ui.p.Index() {
							bc = colPLText
						}
						fillRect(gtx.Ops, image.Rect(er.Min.X, er.Min.Y, er.Max.X, er.Min.Y+1), bc)
						fillRect(gtx.Ops, image.Rect(er.Min.X, er.Max.Y-1, er.Max.X, er.Max.Y), bc)
						fillRect(gtx.Ops, image.Rect(er.Min.X, er.Min.Y, er.Min.X+1, er.Max.Y), bc)
						fillRect(gtx.Ops, image.Rect(er.Max.X-1, er.Min.Y, er.Max.X, er.Max.Y), bc)
						ro.Pop()
					}
					ry += plRowH
				}
			}
		}
		cs.Pop()
		if ui.plAlbumScrollAcc != 0 {
			ui.plAlbumScroll -= ui.plAlbumScrollAcc
			if ui.plAlbumScroll > ui.plAlbumMax {
				ui.plAlbumScroll = ui.plAlbumMax
			}
			if ui.plAlbumScroll < 0 {
				ui.plAlbumScroll = 0
			}
			ui.plAlbumScrollAcc = 0
		}
		if contentH > albumH && albumH > 0 {
			track := image.Rect(winW-20, areaTop+1, winW-16, areaTop+albumH-1)
			fillRect(gtx.Ops, track, colPLBg)
			thumbH := albumH * albumH / contentH
			if thumbH < 12 {
				thumbH = 12
			}
			if thumbH > albumH-2 {
				thumbH = albumH - 2
			}
			frac := float32(0)
			if ui.plAlbumMax > 0 {
				frac = ui.plAlbumScroll / ui.plAlbumMax
			}
			thumbY := areaTop + 1 + int(frac*float32(albumH-2-thumbH))
			fillRect(gtx.Ops, image.Rect(winW-20, thumbY, winW-16, thumbY+thumbH),
				color.NRGBA{R: 0x80, G: 0x80, B: 0x80, A: 0xFF})
			ui.plAlbumScrollDrag(gtx, track, thumbY, thumbH, albumH, ui.plAlbumMax)
		}
	}

	// com painel de artistas/pastas/álbuns aberto ele ocupa toda a área da lista
	if ui.plArtistOpen || ui.plFolderOpen || ui.plAlbumOpen {
		return
	}

	// fundo da lista
	listR := image.Rect(12, plTopH+16+genreH+artistH, winW-23, botY-3)
	fillRect(gtx.Ops, listR, colPLBg)

	ui.plWheel(gtx, listR)

	if ui.plGenSel == 5 {
		// LIST: placeholder em destaque no lugar da lista
		m := op.Record(gtx.Ops)
		paint.ColorOp{Color: colPLText}.Add(gtx.Ops)
		mat := m.Stop()
		loff := op.Offset(listR.Min).Push(gtx.Ops)
		lgtx := gtx
		lgtx.Constraints.Min = image.Point{}
		lgtx.Constraints.Max = image.Pt(listR.Dx(), listR.Dy())
		layout.Center.Layout(lgtx, func(gtx layout.Context) layout.Dimensions {
			return widget.Label{Alignment: text.Middle, MaxLines: 1}.Layout(gtx,
				ui.shaper, ui.fontBold, unit.Sp(27), language.T("pl.list.placeholder"), mat)
		})
		loff.Pop()
		return
	}

	ui.plScrollSink = nil
	loff := op.Offset(listR.Min).Push(gtx.Ops)
	lgtx := gtx
	lgtx.Constraints.Min = image.Point{}
	lgtx.Constraints.Max = image.Pt(listR.Dx(), listR.Dy())
	gsel := ""
	if ui.plGenreOn {
		gsel = ui.plGenreSel
	}
	order := ui.p.TracksForGenre(gsel)
	// ordem alfabética pelo nome de exibição
	names := make(map[int]string, len(order))
	for _, i := range order {
		n, _ := ui.p.TrackTitle(i)
		if n == "" {
			n = filepath.Base(tracks[i])
		}
		names[i] = strings.ToLower(n)
	}
	sort.Slice(order, func(a, b int) bool { return names[order[a]] < names[order[b]] })
	ui.plCount = len(order)
	ui.plList.Axis = layout.Vertical
	ui.plList.Layout(lgtx, len(order), func(gtx layout.Context, i int) layout.Dimensions {
		return ui.plRow(gtx, order[i], tracks, listR.Dx())
	})
	loff.Pop()
	if ui.plScrollAcc != 0 {
		ui.plList.ScrollBy(ui.plScrollAcc / plRowH)
		ui.plScrollAcc = 0
	}

	// indicador da scrollbar acompanha a posição de scroll da lista
	pos := ui.plList.Position
	availH := listR.Dy()
	totalH := int(pos.Length)
	if totalH < availH {
		totalH = availH
	}
	maxS := totalH - availH
	thumbY := plTopH + 6
	if maxS > 0 {
		sc := float32(pos.First)*plRowH + float32(pos.Offset)
		f := sc / float32(maxS)
		if f < 0 {
			f = 0
		} else if f > 1 {
			f = 1
		}
		trackH := botY - 3 - thumbY
		thumbY += int(f * float32(trackH-18))
		if thumbY+18 > botY-3 {
			thumbY = botY - 3 - 18
		}
	}
	ui.spr(gtx, "PLEDIT", R(52, 53, 8, 18), R(winW-17, thumbY, 8, 18))
	ui.plScrollDrag(gtx, winW, botY, maxS, thumbY)
}

// plScrollDrag permite arrastar o indicador da scrollbar da lista de músicas.
func (ui *uiState) plScrollDrag(gtx layout.Context, winW, botY, maxS, thumbY int) {
	if maxS <= 0 {
		return
	}
	trackTop := plTopH + 6
	trackH := botY - 3 - trackTop
	thumbH := 18
	travel := trackH - thumbH
	if travel <= 0 {
		return
	}
	sbR := image.Rect(winW-19, trackTop, winW-7, botY-3)
	cs := clip.Rect(sbR).Push(gtx.Ops)
	event.Op(gtx.Ops, &plScrollTag)
	cs.Pop()
	for {
		ev, ok := gtx.Event(pointer.Filter{
			Target: &plScrollTag,
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
			// só inicia o arraste se o ponteiro estiver sobre o thumb
			if e.Position.Y < float32(thumbY) || e.Position.Y > float32(thumbY+18) {
				continue
			}
			ui.plScrollDragging = true
			pos := ui.plList.Position
			ui.plScrollDragLast = float32(pos.First)*plRowH + float32(pos.Offset)
		case pointer.Drag:
			if !ui.plScrollDragging {
				continue
			}
			frac := (e.Position.Y - float32(trackTop)) / float32(travel)
			if frac < 0 {
				frac = 0
			}
			if frac > 1 {
				frac = 1
			}
			target := frac * float32(maxS)
			ui.plList.ScrollBy((target - ui.plScrollDragLast) / plRowH)
			ui.plScrollDragLast = target
		case pointer.Release, pointer.Cancel:
			ui.plScrollDragging = false
		}
	}
}

// artistTracks retorna os índices das faixas do artista em ordem alfabética.
func (ui *uiState) artistTracks(a string, tracks []string) []int {
	idx := ui.p.TracksForArtist(a)
	names := make(map[int]string, len(idx))
	for _, i := range idx {
		n, _ := ui.p.TrackTitle(i)
		if n == "" {
			n = filepath.Base(tracks[i])
		}
		names[i] = strings.ToLower(n)
	}
	sort.Slice(idx, func(x, y int) bool { return names[idx[x]] < names[idx[y]] })
	return idx
}

// plArtistScrollDrag permite arrastar o thumb da scrollbar de artistas.
func (ui *uiState) plArtistScrollDrag(gtx layout.Context, track image.Rectangle, thumbY, thumbH, viewH int, maxScroll float32) {
	if maxScroll <= 0 {
		return
	}
	travel := viewH - 2 - thumbH
	if travel <= 0 {
		return
	}
	trackTop := float32(track.Min.Y)
	cs := clip.Rect(track).Push(gtx.Ops)
	event.Op(gtx.Ops, &plArtistScrollTag)
	cs.Pop()
	for {
		ev, ok := gtx.Event(pointer.Filter{
			Target: &plArtistScrollTag,
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
			if e.Position.Y < float32(thumbY) || e.Position.Y > float32(thumbY+thumbH) {
				continue
			}
			ui.plScrollDragging = true
		case pointer.Drag:
			if !ui.plScrollDragging {
				continue
			}
			frac := (e.Position.Y - trackTop) / float32(travel)
			if frac < 0 {
				frac = 0
			}
			if frac > 1 {
				frac = 1
			}
			ui.plArtistScroll = frac * maxScroll
		case pointer.Release, pointer.Cancel:
			ui.plScrollDragging = false
		}
	}
}

// plFolderScrollDrag permite arrastar o thumb da scrollbar de pastas.
func (ui *uiState) plFolderScrollDrag(gtx layout.Context, track image.Rectangle, thumbY, thumbH, viewH int, maxScroll float32) {
	if maxScroll <= 0 {
		return
	}
	travel := viewH - 2 - thumbH
	if travel <= 0 {
		return
	}
	trackTop := float32(track.Min.Y)
	cs := clip.Rect(track).Push(gtx.Ops)
	event.Op(gtx.Ops, &plFolderScrollTag)
	cs.Pop()
	for {
		ev, ok := gtx.Event(pointer.Filter{
			Target: &plFolderScrollTag,
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
			if e.Position.Y < float32(thumbY) || e.Position.Y > float32(thumbY+thumbH) {
				continue
			}
			ui.plScrollDragging = true
		case pointer.Drag:
			if !ui.plScrollDragging {
				continue
			}
			frac := (e.Position.Y - trackTop) / float32(travel)
			if frac < 0 {
				frac = 0
			}
			if frac > 1 {
				frac = 1
			}
			ui.plFolderScroll = frac * maxScroll
		case pointer.Release, pointer.Cancel:
			ui.plScrollDragging = false
		}
	}
}

// plAlbumWheel rola o painel de álbuns na área vazia.
func (ui *uiState) plAlbumWheel(gtx layout.Context, r image.Rectangle) {
	cs := clip.Rect(r).Push(gtx.Ops)
	event.Op(gtx.Ops, &plAlbumWheelTag)
	cs.Pop()
	for {
		ev, ok := gtx.Event(pointer.Filter{
			Target:  &plAlbumWheelTag,
			Kinds:   pointer.Scroll,
			ScrollX: pointer.ScrollRange{Min: -1 << 30, Max: 1 << 30},
			ScrollY: pointer.ScrollRange{Min: -1 << 30, Max: 1 << 30},
		})
		if !ok {
			break
		}
		e, ok := ev.(pointer.Event)
		if !ok {
			continue
		}
		ui.plAlbumScrollAcc += e.Scroll.Y
	}
}

// plAlbumScrollDrag permite arrastar o thumb da scrollbar de álbuns.
func (ui *uiState) plAlbumScrollDrag(gtx layout.Context, track image.Rectangle, thumbY, thumbH, viewH int, maxScroll float32) {
	if maxScroll <= 0 {
		return
	}
	travel := viewH - 2 - thumbH
	if travel <= 0 {
		return
	}
	trackTop := float32(track.Min.Y)
	cs := clip.Rect(track).Push(gtx.Ops)
	event.Op(gtx.Ops, &plAlbumScrollTag)
	cs.Pop()
	for {
		ev, ok := gtx.Event(pointer.Filter{
			Target: &plAlbumScrollTag,
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
			if e.Position.Y < float32(thumbY) || e.Position.Y > float32(thumbY+thumbH) {
				continue
			}
			ui.plScrollDragging = true
		case pointer.Drag:
			if !ui.plScrollDragging {
				continue
			}
			frac := (e.Position.Y - trackTop) / float32(travel)
			if frac < 0 {
				frac = 0
			}
			if frac > 1 {
				frac = 1
			}
			ui.plAlbumScroll = frac * maxScroll
		case pointer.Release, pointer.Cancel:
			ui.plScrollDragging = false
		}
	}
}

// albumTracks retorna os índices das faixas do álbum em ordem alfabética.
func (ui *uiState) albumTracks(a string, tracks []string) []int {
	idx := ui.p.AlbumTracks(a)
	names := make(map[int]string, len(idx))
	for _, i := range idx {
		n, _ := ui.p.TrackTitle(i)
		if n == "" {
			n = filepath.Base(tracks[i])
		}
		names[i] = strings.ToLower(n)
	}
	sort.Slice(idx, func(x, y int) bool { return names[idx[x]] < names[idx[y]] })
	return idx
}

// folderTracks retorna os índices das faixas do diretório em ordem alfabética.
func (ui *uiState) folderTracks(dir string, tracks []string) []int {
	idx := ui.p.FolderTracks(dir)
	names := make(map[int]string, len(idx))
	for _, i := range idx {
		n, _ := ui.p.TrackTitle(i)
		if n == "" {
			n = filepath.Base(tracks[i])
		}
		names[i] = strings.ToLower(n)
	}
	sort.Slice(idx, func(x, y int) bool { return names[idx[x]] < names[idx[y]] })
	return idx
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
	// cobre a caixa preta do indicador de tempo (sprite PLEDIT) com a cor do fundo local
	fillRect(gtx.Ops, image.Rect(rx+4, botY+8, rx+96, botY+18), color.NRGBA{R: 40, G: 40, B: 63, A: 0xFF})

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

	// barra de progresso no espaço liberado à direita (antiga caixa do time)
	barR := image.Rect(winW-144, botY+8, winW-55, botY+18)
	if barR.Min.X > plMenuX+4*29+72 {
		ui.plPosBar(gtx, barR)
	}

	// caixa LCD com a quantidade de faixas visíveis
	if ui.plCount > 0 {
		bx, by := plMenuX+4*29+4, botY+9
		box := image.Rect(bx, by, bx+64, by+16)
		if box.Max.X+8 < barR.Min.X {
			fillRect(gtx.Ops, box, color.NRGBA{R: 0, G: 0, B: 0, A: 0xFF})
			fillRect(gtx.Ops, image.Rect(box.Min.X+1, box.Min.Y+1, box.Max.X-1, box.Min.Y+2), colLCDTop)
			fillRect(gtx.Ops, image.Rect(box.Min.X+1, box.Max.Y-2, box.Max.X-1, box.Max.Y-1), colLCDBot)
			s := fmt.Sprintf("%d", ui.plCount)
			x := box.Max.X - 5 - len(s)*10
			for i, ch := range s {
				d := int(ch - '0')
				ui.spr(gtx, "NUMBERS", image.Rect(d*9, 0, d*9+9, 13),
					image.Rect(x+i*10, by+2, x+i*10+9, by+15))
			}
		}
	}

	// setas de scroll (decorativas)
	ui.plScrollArrow(gtx, winW-15, botY+2, true)
	ui.plScrollArrow(gtx, winW-15, botY+8, false)

	// orelha de redimensionamento (canto inferior direito)
	ui.plResize(gtx, image.Rect(winW-16, botY+20, winW-2, botY+36), winW, winH)
}

// plPosBar desenha a barra de progresso da música no rodapé da Playlist,
// com o mesmo sprite POSBAR e indicador do player.
func (ui *uiState) plPosBar(gtx layout.Context, barR image.Rectangle) {
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
	if !ui.plPosDrag.active {
		ui.plPosDrag.val = float32(frac)
	}
	ui.sprFit(gtx, "POSBAR", image.Rect(0, 0, 248, 10), barR)
	// indicador (mesmo sprite do player, escalado)
	kw := 22
	if barR.Dx() < 80 {
		kw = 16
	}
	travel := barR.Dx() - kw
	if travel < 0 {
		travel = 0
	}
	kx := barR.Min.X + int(float32(travel)*ui.plPosDrag.val)
	ui.sprFit(gtx, "POSBAR", image.Rect(248, 0, 277, 10),
		image.Rect(kx, barR.Max.Y-10, kx+kw, barR.Max.Y))
	ui.scrub(gtx, &ui.plPosDrag, image.Rect(barR.Min.X, barR.Min.Y, barR.Max.X, barR.Max.Y+10))
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

// plRow desenha uma linha da playlist (plRowH px, fonte verde).
func (ui *uiState) plRow(gtx layout.Context, i int, tracks []string, w int) layout.Dimensions {
	h := plRowH
	r := image.Rect(0, 0, w, h)
	cur := i == ui.p.Index()
	sel := i == ui.plSel
	if cur {
		fillRect(gtx.Ops, r, colPLCurBg)
	}
	if sel {
		fillRect(gtx.Ops, r, colPLSelBg)
	}
	artist, title, hasMeta := ui.p.TrackParts(i)
	twoLine := hasMeta && artist != "" && title != ""
	name := ""
	if twoLine {
		name = title
	} else if hasMeta {
		name = artist
		if name == "" {
			name = title
		}
	}
	if name == "" {
		name = filepath.Base(tracks[i])
	}
	if sel && !twoLine {
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
	if !hasMeta {
		c = colPLFile // sem metadados: nome do arquivo em verde claro
	}
	if cur {
		c = colPLCur
	}
	if sel {
		c = colPLSelText
	}
	if img := ui.p.TrackCover(i); img != nil {
		blit(gtx, img, image.Rect(1, 1, 41, 41))
	}
	if twoLine {
		ui.textAt(gtx, ui.fontUI, unit.Sp(9), artist, c, image.Pt(45, 9))
		ui.textAt(gtx, ui.fontUI, unit.Sp(9), name, c, image.Pt(45, 23))
	} else {
		ui.textAt(gtx, ui.fontUI, unit.Sp(9), name, c, image.Pt(45, 16))
	}
	for len(ui.plRowClicks) <= i {
		ui.plRowClicks = append(ui.plRowClicks, uibtn{})
	}
	// a roda do mouse é consumida pelo botão da linha (ui.button filtra
	// Scroll); acumula o delta e aplica após o Layout para não corromper
	// a iteração do List
	sink := &ui.plScrollAcc
	if ui.plScrollSink != nil {
		sink = ui.plScrollSink
	}
	ui.plRowClicks[i].scrollFn = func(dy float32) { *sink += dy }
	b := &ui.plRowClicks[i]
	if ui.button(gtx, b, r, func(gtx layout.Context, down bool) {}) {
		ui.plSel = i
		now := time.Now()
		if now.Sub(b.lastClick) < 500*time.Millisecond {
			b.lastClick = time.Time{}
			ui.p.PlayIndex(i)
		} else {
			b.lastClick = now
		}
	}
	if cur {
		// borda verde na faixa tocando
		fillRect(gtx.Ops, image.Rect(r.Min.X, r.Min.Y, r.Max.X, r.Min.Y+1), colPLText)
		fillRect(gtx.Ops, image.Rect(r.Min.X, r.Max.Y-1, r.Max.X, r.Max.Y), colPLText)
		fillRect(gtx.Ops, image.Rect(r.Min.X, r.Min.Y, r.Min.X+1, r.Max.Y), colPLText)
		fillRect(gtx.Ops, image.Rect(r.Max.X-1, r.Min.Y, r.Max.X, r.Max.Y), colPLText)
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
