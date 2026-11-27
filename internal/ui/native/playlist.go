package native

import (
	"fmt"
	"image"
	"image/color"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gioui.org/io/event"
	"gioui.org/io/pointer"
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
var plGenreWheelTag event.Tag
var plGenreScrollTag event.Tag

// plWheel rola a lista com a roda do mouse (mesmo padrão do menuWindow).
func (pl *plw) plWheel(gtx layout.Context, r image.Rectangle) {
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
		if pl.key == "rd" && pl.showRadios {
			pl.radioScrollAcc += e.Scroll.Y
		} else {
			pl.plList.ScrollBy(e.Scroll.Y / plRowH)
		}
	}
}

// plArtistWheel rola a fileira de artistas na área vazia do painel.
func (pl *plw) plArtistWheel(gtx layout.Context, r image.Rectangle) {
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
		pl.plArtistScrollAcc += e.Scroll.Y
	}
}

// plFolderWheel rola o painel de pastas na área vazia.
func (pl *plw) plFolderWheel(gtx layout.Context, r image.Rectangle) {
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
		pl.plFolderScrollAcc += e.Scroll.Y
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

// layoutPL desenha a janela da playlist com sprites PLEDIT.
func (pl *plw) layoutPL(gtx layout.Context) {
	if pl.dirty {
		pl.rebuildMeta()
	}
	winW := gtx.Constraints.Max.X
	winH := gtx.Constraints.Max.Y
	tracks := pl.tracks

	R := func(x, y, w, h int) image.Rectangle { return image.Rect(x, y, x+w, y+h) }
	botY := winH - plBotH

	pl.shadePL.arm(shadePx(pl.plScale, 18))
	if pl.shadePL.on {
		pl.plShadeBar(gtx, winW, R)
		return
	}
	pl.plTopBar(gtx, winW, R)
	pl.plMiddle(gtx, winW, winH, botY, tracks)
	pl.plBottom(gtx, winW, winH, botY, tracks)
}

// plShadeBar desenha a playlist recolhida: linha única na altura do shade
// do player (18px), começando com a decoração/título "Playlist".
func (pl *plw) plShadeBar(gtx layout.Context, winW int, R func(int, int, int, int) image.Rectangle) {
	bar := R(0, 0, winW, 18)
	// fundo: região PLEDIT (179,1)-(204,38) esticada até o fim da barra
	pl.ui.sprFit(gtx, "PLEDIT", image.Rect(179, 1, 204, 38), bar)

	// decoração PLEDIT (0,0-15,15) + nome da janela: sprite "Playlist" na
	// principal; texto para Radio/Video (a skin só tem o sprite da Playlist)
	pl.ui.sprFit(gtx, "PLEDIT", image.Rect(0, 0, 16, 16), R(2, 1, 16, 16))
	if pl.key == "pl" {
		pl.ui.sprFit(gtx, "PLEDIT", image.Rect(75, 0, 123, 16), R(20, 1, 48, 16))
	} else {
		pl.ui.textAt(gtx, pl.ui.fontUI, unit.Sp(9), pl.name,
			color.NRGBA{R: 0x00, G: 0xFF, B: 0x00, A: 0xFF}, image.Pt(22, 4))
	}

	// caixa do nome da música: capa esq. + meio elástico + capa dir.
	// sprites PLEDIT: (73,43-77,55), (72,58-96,71), (120,42-129,55)
	bx0, bx1, by, bh := 74, winW-40, 2, 14
	if bx1 > bx0+10 {
		pl.ui.sprFit(gtx, "PLEDIT", image.Rect(73, 43, 78, 55), R(bx0, by, 5, bh))
		pl.ui.sprFit(gtx, "PLEDIT", image.Rect(72, 58, 97, 71), R(bx0+5, by, bx1-bx0-15, bh))
		pl.ui.sprFit(gtx, "PLEDIT", image.Rect(120, 42, 130, 55), R(bx1-10, by, 10, bh))
		if idx := pl.curIdx(); idx >= 0 {
			if name, _ := pl.meta.TrackTitle(idx); name != "" {
				green := color.NRGBA{R: 0x00, G: 0xE2, B: 0x00, A: 0xFF}
				cs := clip.Rect(image.Rect(bx0+6, by, bx1-11, by+bh)).Push(gtx.Ops)
				pl.ui.textAt(gtx, pl.ui.fontMenu, unit.Sp(9), name, green, image.Pt(bx0+7, by+2))
				cs.Pop()
			}
		}
	}

	// área de move exclui a zona de resize (winW-60..winW-33)
	dragArea(gtx, R(0, 0, winW-61, 18))
	dragArea(gtx, R(winW-32, 0, 1, 18))
	dragAreaX11(gtx, R(0, 0, winW-61, 18), &pl.shadePL)
	dragAreaX11(gtx, R(winW-32, 0, 1, 18), &pl.shadePL)
	if pl.ui.spriteBtn(gtx, &pl.btnPLMin, "TITLEBAR", R(winW-31, 4, 9, 9), R(9, 0, 9, 9), R(9, 9, 9, 9)) {
		minimizeWindow(pl.window())
	}
	if pl.ui.spriteBtn(gtx, &pl.btnPLSize, "TITLEBAR", R(winW-21, 4, 9, 9), R(0, 18, 9, 9), R(9, 18, 9, 9)) {
		if w := pl.window(); w != nil {
			toggleWindowScale(&pl.plScale, w, pl.plWidth, pl.plHeight)
			if pl.key == "pl" {
				saveScale("plscale.txt", pl.plScale)
			} else {
				pl.saveJSON()
			}
		}
	}
	if pl.ui.spriteBtn(gtx, &pl.btnPLClose, "TITLEBAR", R(winW-11, 4, 9, 9), R(18, 0, 9, 9), R(18, 9, 9, 9)) {
		pl.close()
	}

	pl.plShadeResize(gtx, winW)
}

// plShadeResize registra a área do triângulo no fim do campo do nome
// (parte final do sprite da capa direita) para arrastar a largura da janela.
func (pl *plw) plShadeResize(gtx layout.Context, winW int) {
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
		scale := pl.plScale
		if scale < 1 {
			scale = 1
		}
		w0, h0 := winW*scale, pl.shadePL.px
		if h0 <= 0 {
			h0 = 18 * scale
		}
		go func() {
			wmResizeWDrag(pl.name, w0, h0, 300*scale)
			time.Sleep(150 * time.Millisecond)
			pl.savePLWidth()
		}()
	}
}

// plTopBar desenha a barra superior de 20px com tiles PLEDIT.
func (pl *plw) plTopBar(gtx layout.Context, winW int, R func(int, int, int, int) image.Rectangle) {
	// tiles: left corner 25 + title 100 + fill tiles + right corner 25
	pl.ui.spr(gtx, "PLEDIT", R(0, 21, 25, 20), R(0, 0, 25, 20))
	pl.ui.spr(gtx, "PLEDIT", R(26, 21, 100, 20), R(25, 0, 100, 20))
	for x := 125; x < winW-25; x += 25 {
		w := 25
		if x+w > winW-25 {
			w = winW - 25 - x
		}
		pl.ui.spr(gtx, "PLEDIT", R(127, 21, 25, 20), R(x, 0, w, 20))
	}
	pl.ui.spr(gtx, "PLEDIT", R(153, 21, 25, 20), R(winW-25, 0, 25, 20))

	dragArea(gtx, R(0, 0, winW-32, plTopH))
	dragAreaX11(gtx, R(0, 0, winW-32, plTopH), &pl.shadePL)
	if pl.ui.spriteBtn(gtx, &pl.btnPLMin, "TITLEBAR", R(winW-31, 3, 9, 9), R(9, 0, 9, 9), R(9, 9, 9, 9)) {
		minimizeWindow(pl.window())
	}
	if pl.ui.spriteBtn(gtx, &pl.btnPLSize, "TITLEBAR", R(winW-21, 3, 9, 9), R(0, 18, 9, 9), R(9, 18, 9, 9)) {
		if w := pl.window(); w != nil {
			toggleWindowScale(&pl.plScale, w, pl.plWidth, pl.plHeight)
			if pl.key == "pl" {
				saveScale("plscale.txt", pl.plScale)
			} else {
				pl.saveJSON()
			}
		}
	}
	if pl.ui.spriteBtn(gtx, &pl.btnPLClose, "TITLEBAR", R(winW-11, 3, 9, 9), R(18, 0, 9, 9), R(18, 9, 9, 9)) {
		pl.close()
	}
}

// plBgColor retorna a cor de fundo da área da lista: o pixel (185,15) do
// bitmap PLEDIT da skin; fallback para o preto original.
func (pl *plw) plBgColor() color.NRGBA {
	if img := pl.ui.skin.Load().Get("PLEDIT"); img != nil {
		b := img.Bounds()
		if b.Dx() > 185 && b.Dy() > 15 {
			p := img.RGBAAt(b.Min.X+185, b.Min.Y+15)
			return color.NRGBA{R: p.R, G: p.G, B: p.B, A: 0xFF}
		}
	}
	return colPLBg
}

// plMiddle desenha a área da lista com tiles laterais.
func (pl *plw) plMiddle(gtx layout.Context, winW, winH, botY int, tracks []string) {
	R := func(x, y, w, h int) image.Rectangle { return image.Rect(x, y, x+w, y+h) }
	midH := botY - plTopH
	plBg := pl.plBgColor()

	// tiles laterais esticados
	pl.ui.spr(gtx, "PLEDIT", R(0, 42, 12, 29), R(0, plTopH, 12, midH))
	pl.ui.spr(gtx, "PLEDIT", R(31, 42, 20, 29), R(winW-20, plTopH, 20, midH))

	// fundo preto do miolo inteiro (inclui a fileira dos botões GENEX)
	fillRect(gtx.Ops, image.Rect(12, plTopH, winW-20, botY), plBg)

	// 6 botões GENEX abaixo da linha de título, com rótulos da skin GEN
	gbtns := []*uibtn{&pl.btnPLG1, &pl.btnPLG2, &pl.btnPLG3,
		&pl.btnPLG4, &pl.btnPLG5, &pl.btnPLG6}
	songsLbl := language.T("pl.btn.songs")
	if pl.key == "rd" {
		songsLbl = language.T("pl.btn.radios")
	}
	labels := []string{
		language.T("pl.btn.genre"), songsLbl,
		language.T("pl.btn.artist"), language.T("pl.btn.folders"),
		language.T("pl.btn.albums"), language.T("pl.btn.list"),
	}
	dark := color.NRGBA{R: 0x10, G: 0x10, B: 0x18, A: 0xFF}
	genGreen := color.NRGBA{R: 0x80, G: 0x00, B: 0x20, A: 0xFF} // bordeaux
	// janela Radio não tem pastas/artistas/álbuns: esconde esses botões
	hidden := map[int]bool{}
	if pl.key == "rd" {
		hidden[2], hidden[3], hidden[4] = true, true, true
	}
	vi := 0
	for i, gb := range gbtns {
		if hidden[i] {
			continue
		}
		bx := 14 + vi*47
		vi++
		dst := R(bx, plTopH+3, 45, 11)
		// fundo branco; o sprite GENEX da skin fica por cima
		fillRect(gtx.Ops, dst, color.NRGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF})
		if pl.ui.spriteBtn(gtx, gb, "GENEX", dst,
			R(0, 0, 48, 16), R(0, 16, 48, 15)) {
			if i == 0 { // GENRE: abre/fecha a lista de gêneros
				pl.showRadios = false
				pl.plGenreOpen = !pl.plGenreOpen
				// só um painel por vez; as seleções são mantidas
				pl.plArtistOpen = false
				pl.plFolderOpen = false
				pl.plAlbumOpen = false
			}
			if i == 2 { // ARTISTA: abre/fecha a lista de artistas
				pl.plArtistOpen = !pl.plArtistOpen
				pl.plGenreOpen = false
				pl.plFolderOpen = false
				pl.plAlbumOpen = false
			}
			if i == 3 { // PASTAS: abre/fecha a lista de diretórios
				pl.plFolderOpen = !pl.plFolderOpen
				pl.plGenreOpen = false
				pl.plArtistOpen = false
				pl.plAlbumOpen = false
			}
			if i == 4 { // ÁLBUNS: abre/fecha a lista de álbuns
				pl.plAlbumOpen = !pl.plAlbumOpen
				pl.plGenreOpen = false
				pl.plArtistOpen = false
				pl.plFolderOpen = false
			}
			if i == 1 { // MÚSICAS/RÁDIOS: na janela Radio alterna a grade de estações
				if pl.key == "rd" {
					pl.showRadios = !pl.showRadios
					pl.radioScroll = 0
					pl.radioScrollAcc = 0
				} else {
					pl.showRadios = false
				}
				pl.plGenreOpen = false
				pl.plArtistOpen = false
				pl.plArtistExpand = ""
				pl.plArtistExpandIdx = nil
				pl.plFolderOpen = false
				pl.plFolderExpand = ""
				pl.plFolderExpandIdx = nil
				pl.plAlbumOpen = false
				pl.plAlbumExpand = ""
				pl.plAlbumExpandIdx = nil
			}
			if i == 5 { // LIST: fecha painéis e desliga o filtro, mantendo as seleções
				pl.showRadios = false
				pl.plGenreOpen = false
				pl.plArtistOpen = false
				pl.plFolderOpen = false
				pl.plAlbumOpen = false
			}
			if pl.plGenSel == i {
				pl.plGenSel = -1
			} else {
				pl.plGenSel = i
			}
			pl.savePanel()
		}
		// RADIOS ativo na janela Radio: botão fica vermelho
		radiosOn := pl.key == "rd" && i == 1 && pl.showRadios
		if radiosOn {
			fillRect(gtx.Ops, dst, color.NRGBA{R: 0xC0, G: 0x10, B: 0x10, A: 0x70})
		}
		lc := dark
		sel := pl.plGenSel == i || radiosOn || (i == 0 && pl.plGenreOpen) || (i == 2 && pl.plArtistOpen) || (i == 3 && pl.plFolderOpen) || (i == 4 && pl.plAlbumOpen)
		if sel {
			pl.ui.btnTextB(gtx, labels[i], dst, genGreen)
			// borda no botão selecionado (vermelha no RADIOS ativo)
			bc := genGreen
			if radiosOn {
				bc = color.NRGBA{R: 0xFF, G: 0x30, B: 0x30, A: 0xFF}
			}
			bx0, by0 := dst.Min.X, dst.Min.Y
			bx1, by1 := dst.Max.X, dst.Max.Y
			fillRect(gtx.Ops, image.Rect(bx0, by0, bx1, by0+2), bc)
			fillRect(gtx.Ops, image.Rect(bx0, by1-2, bx1, by1), bc)
			fillRect(gtx.Ops, image.Rect(bx0, by0, bx0+2, by1), bc)
			fillRect(gtx.Ops, image.Rect(bx1-2, by0, bx1, by1), bc)
		} else {
			pl.ui.btnText(gtx, labels[i], dst, lc)
		}
	}

	// painel de gêneros: mesmo formato do de artistas, um gênero por linha;
	// duplo-clique expande e mostra todas as músicas do gênero
	genreH := 0
	if pl.plGenreOpen {
		genres := pl.meta.Genres()
		for len(pl.plGenreBtns) < len(genres) {
			pl.plGenreBtns = append(pl.plGenreBtns, uibtn{})
		}
		// restaura a expansão persistida quando as faixas já estão carregadas
		if pl.plGenreExpand != "" && pl.plGenreExpandIdx == nil {
			for _, g := range genres {
				if g == pl.plGenreExpand {
					pl.plGenreExpandIdx = pl.genreTracks(g, tracks)
					break
				}
			}
		}
		// faixas do gênero expandido (só se ainda existir na lista)
		expN := 0
		if pl.plGenreExpand != "" {
			for _, g := range genres {
				if g == pl.plGenreExpand {
					expN = len(pl.plGenreExpandIdx)
					break
				}
			}
		}
		areaTop := plTopH + 16
		avail := botY - 3 - areaTop
		contentH := (len(genres) + expN) * plRowH
		gH := contentH
		if gH > avail {
			gH = avail
		}
		if gH < 0 {
			gH = 0
		}
		pl.plCount = len(genres)
		pl.plCount2 = expN
		pl.plGenreMax = float32(contentH - gH)
		if pl.plGenreMax < 0 {
			pl.plGenreMax = 0
		}
		if pl.plGenreScroll > pl.plGenreMax {
			pl.plGenreScroll = pl.plGenreMax
		}
		if pl.plGenreScroll < 0 {
			pl.plGenreScroll = 0
		}
		panel := image.Rect(12, areaTop, winW-23, areaTop+gH)
		fillRect(gtx.Ops, panel, plBg)
		cs := clip.Rect(panel).Push(gtx.Ops)
		// scroll via deslocamento de ry (sem op.Offset: quebrava a área de eventos)
		ry := areaTop - int(pl.plGenreScroll)
		// roda primeiro (embaixo): se fosse por cima, engoliria os Press dos botões
		pl.plGenreWheel(gtx, panel)
		pl.plScrollSink = &pl.plGenreScrollAcc
		// só renderiza linhas visíveis (evita decode de capa / ops de 300+ linhas)
		visMin := areaTop - plRowH
		visMax := areaTop + gH
		// gênero da faixa tocando (borda verde na linha dele)
		curGen := ""
		if idx := pl.curIdx(); idx >= 0 {
			curGen = pl.meta.TrackGenre(idx)
		}
		for gi, g := range genres {
			rowR := image.Rect(14, ry, winW-27, ry+plRowH)
			exp := pl.plGenreExpand == g
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
				if curGen != "" && g == curGen {
					// borda verde no gênero da faixa tocando
					fillRect(gtx.Ops, image.Rect(rowR.Min.X, rowR.Min.Y, rowR.Max.X, rowR.Min.Y+1), colPLText)
					fillRect(gtx.Ops, image.Rect(rowR.Min.X, rowR.Max.Y-1, rowR.Max.X, rowR.Max.Y), colPLText)
					fillRect(gtx.Ops, image.Rect(rowR.Min.X, rowR.Min.Y, rowR.Min.X+1, rowR.Max.Y), colPLText)
					fillRect(gtx.Ops, image.Rect(rowR.Max.X-1, rowR.Min.Y, rowR.Max.X, rowR.Max.Y), colPLText)
				}
				ac := colPLText
				if exp {
					ac = colPLSelText
				}
				if img := pl.meta.GenreCover(g); img != nil {
					blit(gtx, img, image.Rect(15, ry+1, 55, ry+41))
				}
				pl.ui.textAt(gtx, pl.ui.fontUI, unit.Sp(9), g, ac, image.Pt(59, ry+9))
				if n := pl.meta.GenreTrackCount(g); n > 0 {
					cnt := fmt.Sprintf(language.T("pl.artist.tracks"), n)
					pl.ui.textAt(gtx, pl.ui.fontUI, unit.Sp(9), cnt, colPLFile, image.Pt(59, ry+25))
				}
				pl.plGenreBtns[gi].scrollFn = func(dy float32) { pl.plGenreScrollAcc += dy }
				b := &pl.plGenreBtns[gi]
				if pl.ui.button(gtx, b, rowR, func(gtx layout.Context, down bool) {}) {
					now := time.Now()
					dbl := now.Sub(b.lastClick) < 500*time.Millisecond
					b.lastClick = now
					if dbl {
						b.lastClick = time.Time{}
						if exp {
							pl.plGenreExpand = ""
							pl.plGenreExpandIdx = nil
						} else {
							pl.plGenreExpand = g
							pl.plGenreExpandIdx = pl.genreTracks(g, tracks)
						}
						pl.savePanel()
					}
				}
			}
			ry += plRowH
			// músicas do gênero logo abaixo, no formato das linhas de música
			if exp {
				for _, ti := range pl.plGenreExpandIdx {
					if ry > visMin && ry < visMax {
						ro := op.Offset(image.Pt(54, ry)).Push(gtx.Ops)
						fillRect(gtx.Ops, image.Rect(0, 0, winW-81, plRowH), colPLExpBg)
						pl.plRow(gtx, ti, tracks, winW-81, pl.plGenreExpandIdx)
						ro.Pop()
					}
					ry += plRowH
				}
			}
		}
		cs.Pop()
		// aplica o scroll acumulado pela roda (botões e área vazia)
		if pl.plGenreScrollAcc != 0 {
			pl.plGenreScroll -= pl.plGenreScrollAcc
			if pl.plGenreScroll > pl.plGenreMax {
				pl.plGenreScroll = pl.plGenreMax
			}
			if pl.plGenreScroll < 0 {
				pl.plGenreScroll = 0
			}
			pl.plGenreScrollAcc = 0
		}
		// scrollbar quando o conteúdo excede a área visível
		if contentH > gH && gH > 0 {
			track := image.Rect(winW-20, areaTop+1, winW-16, areaTop+gH-1)
			fillRect(gtx.Ops, track, plBg)
			thumbH := gH * gH / contentH
			if thumbH < 12 {
				thumbH = 12
			}
			if thumbH > gH-2 {
				thumbH = gH - 2
			}
			frac := float32(0)
			if pl.plGenreMax > 0 {
				frac = pl.plGenreScroll / pl.plGenreMax
			}
			thumbY := areaTop + 1 + int(frac*float32(gH-2-thumbH))
			fillRect(gtx.Ops, image.Rect(winW-20, thumbY, winW-16, thumbY+thumbH),
				color.NRGBA{R: 0x80, G: 0x80, B: 0x80, A: 0xFF})
			pl.plGenreScrollDrag(gtx, track, thumbY, thumbH, gH, pl.plGenreMax)
		}
	}

	// fileira de artistas abaixo dos botões GENEX (ARTISTA abre/fecha)
	// um artista por linha, ordenado, com scroll próprio
	artistH := 0
	if pl.plArtistOpen {
		artists := pl.meta.Artists() // já vem ordenada alfabeticamente
		for len(pl.plArtistBtns) < len(artists) {
			pl.plArtistBtns = append(pl.plArtistBtns, uibtn{})
		}
		// restaura a expansão persistida quando as faixas já estão carregadas
		if pl.plArtistExpand != "" && pl.plArtistExpandIdx == nil {
			for _, a2 := range artists {
				if a2 == pl.plArtistExpand {
					pl.plArtistExpandIdx = pl.artistTracks(a2, tracks)
					break
				}
			}
		}
		// faixas do artista expandido (só se ainda existir na lista)
		expN := 0
		if pl.plArtistExpand != "" {
			for _, a2 := range artists {
				if a2 == pl.plArtistExpand {
					expN = len(pl.plArtistExpandIdx)
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
		pl.plCount = len(artists)
		pl.plCount2 = expN
		pl.plArtistMax = float32(contentH - artistH)
		if pl.plArtistMax < 0 {
			pl.plArtistMax = 0
		}
		if pl.plArtistScroll > pl.plArtistMax {
			pl.plArtistScroll = pl.plArtistMax
		}
		if pl.plArtistScroll < 0 {
			pl.plArtistScroll = 0
		}
		panel := image.Rect(12, areaTop, winW-23, areaTop+artistH)
		fillRect(gtx.Ops, panel, plBg)
		cs := clip.Rect(panel).Push(gtx.Ops)
		// scroll via deslocamento de ry (sem op.Offset: quebrava a área de eventos)
		ry := areaTop - int(pl.plArtistScroll)
		// roda primeiro (embaixo): se fosse por cima, engoliria os Press dos botões
		pl.plArtistWheel(gtx, panel)
		pl.plScrollSink = &pl.plArtistScrollAcc
		// só renderiza linhas visíveis (evita decode de capa / ops de 300+ linhas)
		visMin := areaTop - plRowH
		visMax := areaTop + artistH
		// artista da faixa tocando (borda verde na linha dele)
		curArt := ""
		if idx := pl.curIdx(); idx >= 0 {
			curArt, _, _ = pl.meta.TrackParts(idx)
		}
		for ai, a := range artists {
			rowR := image.Rect(14, ry, winW-27, ry+plRowH)
			exp := pl.plArtistExpand == a
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
				if curArt != "" && a == curArt {
					// borda verde no artista da faixa tocando
					fillRect(gtx.Ops, image.Rect(rowR.Min.X, rowR.Min.Y, rowR.Max.X, rowR.Min.Y+1), colPLText)
					fillRect(gtx.Ops, image.Rect(rowR.Min.X, rowR.Max.Y-1, rowR.Max.X, rowR.Max.Y), colPLText)
					fillRect(gtx.Ops, image.Rect(rowR.Min.X, rowR.Min.Y, rowR.Min.X+1, rowR.Max.Y), colPLText)
					fillRect(gtx.Ops, image.Rect(rowR.Max.X-1, rowR.Min.Y, rowR.Max.X, rowR.Max.Y), colPLText)
				}
				ac := colPLText
				if exp {
					ac = colPLSelText
				}
				if img := pl.meta.ArtistCover(a); img != nil {
					blit(gtx, img, image.Rect(15, ry+1, 55, ry+41))
				}
				pl.ui.textAt(gtx, pl.ui.fontUI, unit.Sp(9), a, ac, image.Pt(59, ry+9))
				if n := pl.meta.ArtistTrackCount(a); n > 0 {
					cnt := fmt.Sprintf(language.T("pl.artist.tracks"), n)
					pl.ui.textAt(gtx, pl.ui.fontUI, unit.Sp(9), cnt, colPLFile, image.Pt(59, ry+25))
				}
				pl.plArtistBtns[ai].scrollFn = func(dy float32) { pl.plArtistScrollAcc += dy }
				b := &pl.plArtistBtns[ai]
				if pl.ui.button(gtx, b, rowR, func(gtx layout.Context, down bool) {}) {
					now := time.Now()
					dbl := now.Sub(b.lastClick) < 500*time.Millisecond
					b.lastClick = now
					if dbl {
						b.lastClick = time.Time{}
						if exp {
							pl.plArtistExpand = ""
							pl.plArtistExpandIdx = nil
						} else {
							pl.plArtistExpand = a
							pl.plArtistExpandIdx = pl.artistTracks(a, tracks)
						}
						pl.savePanel()
					}
				}
			}
			ry += plRowH
			// músicas do artista logo abaixo, no formato das linhas de música
			if exp {
				for _, ti := range pl.plArtistExpandIdx {
					if ry > visMin && ry < visMax {
						ro := op.Offset(image.Pt(54, ry)).Push(gtx.Ops)
						fillRect(gtx.Ops, image.Rect(0, 0, winW-81, plRowH), colPLExpBg)
						pl.plRow(gtx, ti, tracks, winW-81, pl.plArtistExpandIdx)
						ro.Pop()
					}
					ry += plRowH
				}
			}
		}
		cs.Pop()
		// aplica o scroll acumulado pela roda (botões e área vazia)
		if pl.plArtistScrollAcc != 0 {
			pl.plArtistScroll -= pl.plArtistScrollAcc
			if pl.plArtistScroll > pl.plArtistMax {
				pl.plArtistScroll = pl.plArtistMax
			}
			if pl.plArtistScroll < 0 {
				pl.plArtistScroll = 0
			}
			pl.plArtistScrollAcc = 0
		}
		// scrollbar quando o conteúdo excede a área visível
		if contentH > artistH && artistH > 0 {
			track := image.Rect(winW-20, areaTop+1, winW-16, areaTop+artistH-1)
			fillRect(gtx.Ops, track, plBg)
			thumbH := artistH * artistH / contentH
			if thumbH < 12 {
				thumbH = 12
			}
			if thumbH > artistH-2 {
				thumbH = artistH - 2
			}
			frac := float32(0)
			if pl.plArtistMax > 0 {
				frac = pl.plArtistScroll / pl.plArtistMax
			}
			thumbY := areaTop + 1 + int(frac*float32(artistH-2-thumbH))
			fillRect(gtx.Ops, image.Rect(winW-20, thumbY, winW-16, thumbY+thumbH),
				color.NRGBA{R: 0x80, G: 0x80, B: 0x80, A: 0xFF})
			pl.plArtistScrollDrag(gtx, track, thumbY, thumbH, artistH, pl.plArtistMax)
		}
	}

	// painel de pastas: mesmo formato do de artistas, agrupado por diretório
	if pl.plFolderOpen {
		folders := pl.meta.Folders()
		for len(pl.plFolderBtns) < len(folders) {
			pl.plFolderBtns = append(pl.plFolderBtns, uibtn{})
		}
		if pl.plFolderExpand != "" && pl.plFolderExpandIdx == nil {
			for _, d := range folders {
				if d == pl.plFolderExpand {
					pl.plFolderExpandIdx = pl.folderTracks(d, tracks)
					break
				}
			}
		}
		expN := 0
		if pl.plFolderExpand != "" {
			for _, d := range folders {
				if d == pl.plFolderExpand {
					expN = len(pl.plFolderExpandIdx)
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
		pl.plCount = len(folders)
		pl.plCount2 = expN
		pl.plFolderMax = float32(contentH - folderH)
		if pl.plFolderMax < 0 {
			pl.plFolderMax = 0
		}
		if pl.plFolderScroll > pl.plFolderMax {
			pl.plFolderScroll = pl.plFolderMax
		}
		if pl.plFolderScroll < 0 {
			pl.plFolderScroll = 0
		}
		panel := image.Rect(12, areaTop, winW-23, areaTop+folderH)
		fillRect(gtx.Ops, panel, plBg)
		cs := clip.Rect(panel).Push(gtx.Ops)
		// scroll via deslocamento de ry (sem op.Offset: quebrava a área de eventos)
		ry := areaTop - int(pl.plFolderScroll)
		// roda primeiro (embaixo): se fosse por cima, engoliria os Press dos botões
		pl.plFolderWheel(gtx, panel)
		pl.plScrollSink = &pl.plFolderScrollAcc
		visMin := areaTop - plRowH
		visMax := areaTop + folderH
		// pasta da faixa tocando (borda verde na linha dela)
		curDir := ""
		if idx := pl.curIdx(); idx >= 0 {
			if path := pl.tracksAt(idx); path != "" {
				curDir = filepath.Dir(path)
			}
		}
		for di, dir := range folders {
			rowR := image.Rect(14, ry, winW-27, ry+plRowH)
			exp := pl.plFolderExpand == dir
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
				if curDir != "" && dir == curDir {
					fillRect(gtx.Ops, image.Rect(rowR.Min.X, rowR.Min.Y, rowR.Max.X, rowR.Min.Y+1), colPLText)
					fillRect(gtx.Ops, image.Rect(rowR.Min.X, rowR.Max.Y-1, rowR.Max.X, rowR.Max.Y), colPLText)
					fillRect(gtx.Ops, image.Rect(rowR.Min.X, rowR.Min.Y, rowR.Min.X+1, rowR.Max.Y), colPLText)
					fillRect(gtx.Ops, image.Rect(rowR.Max.X-1, rowR.Min.Y, rowR.Max.X, rowR.Max.Y), colPLText)
				}
				ac := colPLText
				if exp {
					ac = colPLSelText
				}
				if img := pl.meta.FolderCover(dir); img != nil {
					blit(gtx, img, image.Rect(15, ry+1, 55, ry+41))
				}
				pl.ui.textAt(gtx, pl.ui.fontUI, unit.Sp(9), pl.meta.FolderName(dir), ac, image.Pt(59, ry+9))
				if n := pl.meta.FolderTrackCount(dir); n > 0 {
					cnt := fmt.Sprintf(language.T("pl.artist.tracks"), n)
					pl.ui.textAt(gtx, pl.ui.fontUI, unit.Sp(9), cnt, colPLFile, image.Pt(59, ry+25))
				}
				pl.plFolderBtns[di].scrollFn = func(dy float32) { pl.plFolderScrollAcc += dy }
				b := &pl.plFolderBtns[di]
				if pl.ui.button(gtx, b, rowR, func(gtx layout.Context, down bool) {}) {
					now := time.Now()
					dbl := now.Sub(b.lastClick) < 500*time.Millisecond
					b.lastClick = now
					if dbl {
						b.lastClick = time.Time{}
						if exp {
							pl.plFolderExpand = ""
							pl.plFolderExpandIdx = nil
						} else {
							pl.plFolderExpand = dir
							pl.plFolderExpandIdx = pl.folderTracks(dir, tracks)
						}
						pl.savePanel()
					}
				}
			}
			ry += plRowH
			if exp {
				for _, ti := range pl.plFolderExpandIdx {
					if ry > visMin && ry < visMax {
						ro := op.Offset(image.Pt(54, ry)).Push(gtx.Ops)
						fillRect(gtx.Ops, image.Rect(0, 0, winW-81, plRowH), colPLExpBg)
						pl.plRow(gtx, ti, tracks, winW-81, pl.plFolderExpandIdx)
						ro.Pop()
					}
					ry += plRowH
				}
			}
		}
		cs.Pop()
		if pl.plFolderScrollAcc != 0 {
			pl.plFolderScroll -= pl.plFolderScrollAcc
			if pl.plFolderScroll > pl.plFolderMax {
				pl.plFolderScroll = pl.plFolderMax
			}
			if pl.plFolderScroll < 0 {
				pl.plFolderScroll = 0
			}
			pl.plFolderScrollAcc = 0
		}
		if contentH > folderH && folderH > 0 {
			track := image.Rect(winW-20, areaTop+1, winW-16, areaTop+folderH-1)
			fillRect(gtx.Ops, track, plBg)
			thumbH := folderH * folderH / contentH
			if thumbH < 12 {
				thumbH = 12
			}
			if thumbH > folderH-2 {
				thumbH = folderH - 2
			}
			frac := float32(0)
			if pl.plFolderMax > 0 {
				frac = pl.plFolderScroll / pl.plFolderMax
			}
			thumbY := areaTop + 1 + int(frac*float32(folderH-2-thumbH))
			fillRect(gtx.Ops, image.Rect(winW-20, thumbY, winW-16, thumbY+thumbH),
				color.NRGBA{R: 0x80, G: 0x80, B: 0x80, A: 0xFF})
			pl.plFolderScrollDrag(gtx, track, thumbY, thumbH, folderH, pl.plFolderMax)
		}
	}

	// painel de álbuns: mesmo formato, agrupado pela tag ALBUM/TALB
	if pl.plAlbumOpen {
		albums := pl.meta.Albums()
		for len(pl.plAlbumBtns) < len(albums) {
			pl.plAlbumBtns = append(pl.plAlbumBtns, uibtn{})
		}
		if pl.plAlbumExpand != "" && pl.plAlbumExpandIdx == nil {
			for _, al := range albums {
				if al == pl.plAlbumExpand {
					pl.plAlbumExpandIdx = pl.albumTracks(al, tracks)
					break
				}
			}
		}
		expN := 0
		if pl.plAlbumExpand != "" {
			for _, al := range albums {
				if al == pl.plAlbumExpand {
					expN = len(pl.plAlbumExpandIdx)
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
		pl.plCount = len(albums)
		pl.plCount2 = expN
		pl.plAlbumMax = float32(contentH - albumH)
		if pl.plAlbumMax < 0 {
			pl.plAlbumMax = 0
		}
		if pl.plAlbumScroll > pl.plAlbumMax {
			pl.plAlbumScroll = pl.plAlbumMax
		}
		if pl.plAlbumScroll < 0 {
			pl.plAlbumScroll = 0
		}
		panel := image.Rect(12, areaTop, winW-23, areaTop+albumH)
		fillRect(gtx.Ops, panel, plBg)
		cs := clip.Rect(panel).Push(gtx.Ops)
		ry := areaTop - int(pl.plAlbumScroll)
		// roda primeiro (embaixo): se fosse por cima, engoliria os Press dos botões
		pl.plAlbumWheel(gtx, panel)
		pl.plScrollSink = &pl.plAlbumScrollAcc
		visMin := areaTop - plRowH
		visMax := areaTop + albumH
		// álbum da faixa tocando (borda verde na linha dele)
		curAlb := ""
		if idx := pl.curIdx(); idx >= 0 {
			curAlb = pl.meta.TrackAlbum(idx)
		}
		for ai2, al := range albums {
			rowR := image.Rect(14, ry, winW-27, ry+plRowH)
			exp := pl.plAlbumExpand == al
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
				if curAlb != "" && al == curAlb {
					fillRect(gtx.Ops, image.Rect(rowR.Min.X, rowR.Min.Y, rowR.Max.X, rowR.Min.Y+1), colPLText)
					fillRect(gtx.Ops, image.Rect(rowR.Min.X, rowR.Max.Y-1, rowR.Max.X, rowR.Max.Y), colPLText)
					fillRect(gtx.Ops, image.Rect(rowR.Min.X, rowR.Min.Y, rowR.Min.X+1, rowR.Max.Y), colPLText)
					fillRect(gtx.Ops, image.Rect(rowR.Max.X-1, rowR.Min.Y, rowR.Max.X, rowR.Max.Y), colPLText)
				}
				ac := colPLText
				if exp {
					ac = colPLSelText
				}
				if img := pl.meta.AlbumCover(al); img != nil {
					blit(gtx, img, image.Rect(15, ry+1, 55, ry+41))
				}
				pl.ui.textAt(gtx, pl.ui.fontUI, unit.Sp(9), al, ac, image.Pt(59, ry+9))
				if n := pl.meta.AlbumTrackCount(al); n > 0 {
					cnt := fmt.Sprintf(language.T("pl.artist.tracks"), n)
					pl.ui.textAt(gtx, pl.ui.fontUI, unit.Sp(9), cnt, colPLFile, image.Pt(59, ry+25))
				}
				pl.plAlbumBtns[ai2].scrollFn = func(dy float32) { pl.plAlbumScrollAcc += dy }
				b := &pl.plAlbumBtns[ai2]
				if pl.ui.button(gtx, b, rowR, func(gtx layout.Context, down bool) {}) {
					now := time.Now()
					dbl := now.Sub(b.lastClick) < 500*time.Millisecond
					b.lastClick = now
					if dbl {
						b.lastClick = time.Time{}
						if exp {
							pl.plAlbumExpand = ""
							pl.plAlbumExpandIdx = nil
						} else {
							pl.plAlbumExpand = al
							pl.plAlbumExpandIdx = pl.albumTracks(al, tracks)
						}
						pl.savePanel()
					}
				}
			}
			ry += plRowH
			if exp {
				for _, ti := range pl.plAlbumExpandIdx {
					if ry > visMin && ry < visMax {
						ro := op.Offset(image.Pt(54, ry)).Push(gtx.Ops)
						fillRect(gtx.Ops, image.Rect(0, 0, winW-81, plRowH), colPLExpBg)
						pl.plRow(gtx, ti, tracks, winW-81, pl.plAlbumExpandIdx)
						ro.Pop()
					}
					ry += plRowH
				}
			}
		}
		cs.Pop()
		if pl.plAlbumScrollAcc != 0 {
			pl.plAlbumScroll -= pl.plAlbumScrollAcc
			if pl.plAlbumScroll > pl.plAlbumMax {
				pl.plAlbumScroll = pl.plAlbumMax
			}
			if pl.plAlbumScroll < 0 {
				pl.plAlbumScroll = 0
			}
			pl.plAlbumScrollAcc = 0
		}
		if contentH > albumH && albumH > 0 {
			track := image.Rect(winW-20, areaTop+1, winW-16, areaTop+albumH-1)
			fillRect(gtx.Ops, track, plBg)
			thumbH := albumH * albumH / contentH
			if thumbH < 12 {
				thumbH = 12
			}
			if thumbH > albumH-2 {
				thumbH = albumH - 2
			}
			frac := float32(0)
			if pl.plAlbumMax > 0 {
				frac = pl.plAlbumScroll / pl.plAlbumMax
			}
			thumbY := areaTop + 1 + int(frac*float32(albumH-2-thumbH))
			fillRect(gtx.Ops, image.Rect(winW-20, thumbY, winW-16, thumbY+thumbH),
				color.NRGBA{R: 0x80, G: 0x80, B: 0x80, A: 0xFF})
			pl.plAlbumScrollDrag(gtx, track, thumbY, thumbH, albumH, pl.plAlbumMax)
		}
	}

	// com painel de gêneros/artistas/pastas/álbuns aberto ele ocupa toda a área da lista
	if pl.plGenreOpen || pl.plArtistOpen || pl.plFolderOpen || pl.plAlbumOpen {
		return
	}

	// fundo da lista
	listR := image.Rect(12, plTopH+16+genreH+artistH, winW-23, botY-3)
	fillRect(gtx.Ops, listR, plBg)

	pl.plWheel(gtx, listR)

	// janela Radio com RADIOS ativo: grade de estações no lugar das faixas
	if pl.key == "rd" && pl.showRadios {
		pl.plRadioGrid(gtx, listR)
		return
	}

	if pl.plGenSel == 5 {
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
				pl.ui.shaper, pl.ui.fontBold, unit.Sp(27), language.T("pl.list.placeholder"), mat)
		})
		loff.Pop()
		return
	}

	pl.plScrollSink = nil
	loff := op.Offset(listR.Min).Push(gtx.Ops)
	lgtx := gtx
	lgtx.Constraints.Min = image.Point{}
	lgtx.Constraints.Max = image.Pt(listR.Dx(), listR.Dy())
	order := pl.meta.TracksForGenre("")
	// ordem alfabética pelo nome de exibição
	names := make(map[int]string, len(order))
	for _, i := range order {
		n, _ := pl.meta.TrackTitle(i)
		if n == "" {
			n = filepath.Base(tracks[i])
		}
		names[i] = strings.ToLower(n)
	}
	sort.Slice(order, func(a, b int) bool { return names[order[a]] < names[order[b]] })
	pl.plCount = len(order)
	pl.plCount2 = 0
	pl.plList.Axis = layout.Vertical
	pl.plList.Layout(lgtx, len(order), func(gtx layout.Context, i int) layout.Dimensions {
		return pl.plRow(gtx, order[i], tracks, listR.Dx(), nil)
	})
	loff.Pop()
	if pl.plScrollAcc != 0 {
		pl.plList.ScrollBy(pl.plScrollAcc / plRowH)
		pl.plScrollAcc = 0
	}

	// indicador da scrollbar acompanha a posição de scroll da lista
	pos := pl.plList.Position
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
	pl.ui.spr(gtx, "PLEDIT", R(52, 53, 8, 18), R(winW-17, thumbY, 8, 18))
	pl.plScrollDrag(gtx, winW, botY, maxS, thumbY)
}

// plScrollDrag permite arrastar o indicador da scrollbar da lista de músicas.
func (pl *plw) plScrollDrag(gtx layout.Context, winW, botY, maxS, thumbY int) {
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
			pl.plScrollDragging = true
			pos := pl.plList.Position
			pl.plScrollDragLast = float32(pos.First)*plRowH + float32(pos.Offset)
		case pointer.Drag:
			if !pl.plScrollDragging {
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
			pl.plList.ScrollBy((target - pl.plScrollDragLast) / plRowH)
			pl.plScrollDragLast = target
		case pointer.Release, pointer.Cancel:
			pl.plScrollDragging = false
		}
	}
}

// artistTracks retorna os índices das faixas do artista em ordem alfabética.
func (pl *plw) artistTracks(a string, tracks []string) []int {
	idx := pl.meta.TracksForArtist(a)
	names := make(map[int]string, len(idx))
	for _, i := range idx {
		n, _ := pl.meta.TrackTitle(i)
		if n == "" {
			n = filepath.Base(tracks[i])
		}
		names[i] = strings.ToLower(n)
	}
	sort.Slice(idx, func(x, y int) bool { return names[idx[x]] < names[idx[y]] })
	return idx
}

// plArtistScrollDrag permite arrastar o thumb da scrollbar de artistas.
func (pl *plw) plArtistScrollDrag(gtx layout.Context, track image.Rectangle, thumbY, thumbH, viewH int, maxScroll float32) {
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
			pl.plScrollDragging = true
		case pointer.Drag:
			if !pl.plScrollDragging {
				continue
			}
			frac := (e.Position.Y - trackTop) / float32(travel)
			if frac < 0 {
				frac = 0
			}
			if frac > 1 {
				frac = 1
			}
			pl.plArtistScroll = frac * maxScroll
		case pointer.Release, pointer.Cancel:
			pl.plScrollDragging = false
		}
	}
}

// plFolderScrollDrag permite arrastar o thumb da scrollbar de pastas.
func (pl *plw) plFolderScrollDrag(gtx layout.Context, track image.Rectangle, thumbY, thumbH, viewH int, maxScroll float32) {
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
			pl.plScrollDragging = true
		case pointer.Drag:
			if !pl.plScrollDragging {
				continue
			}
			frac := (e.Position.Y - trackTop) / float32(travel)
			if frac < 0 {
				frac = 0
			}
			if frac > 1 {
				frac = 1
			}
			pl.plFolderScroll = frac * maxScroll
		case pointer.Release, pointer.Cancel:
			pl.plScrollDragging = false
		}
	}
}

// plAlbumWheel rola o painel de álbuns na área vazia.
func (pl *plw) plAlbumWheel(gtx layout.Context, r image.Rectangle) {
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
		pl.plAlbumScrollAcc += e.Scroll.Y
	}
}

// plGenreWheel rola o painel de gêneros na área vazia.
func (pl *plw) plGenreWheel(gtx layout.Context, r image.Rectangle) {
	cs := clip.Rect(r).Push(gtx.Ops)
	event.Op(gtx.Ops, &plGenreWheelTag)
	cs.Pop()
	for {
		ev, ok := gtx.Event(pointer.Filter{
			Target:  &plGenreWheelTag,
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
		pl.plGenreScrollAcc += e.Scroll.Y
	}
}

// plAlbumScrollDrag permite arrastar o thumb da scrollbar de álbuns.
func (pl *plw) plAlbumScrollDrag(gtx layout.Context, track image.Rectangle, thumbY, thumbH, viewH int, maxScroll float32) {
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
			pl.plScrollDragging = true
		case pointer.Drag:
			if !pl.plScrollDragging {
				continue
			}
			frac := (e.Position.Y - trackTop) / float32(travel)
			if frac < 0 {
				frac = 0
			}
			if frac > 1 {
				frac = 1
			}
			pl.plAlbumScroll = frac * maxScroll
		case pointer.Release, pointer.Cancel:
			pl.plScrollDragging = false
		}
	}
}

// genreTracks retorna os índices das faixas do gênero em ordem alfabética.
func (pl *plw) genreTracks(g string, tracks []string) []int {
	idx := pl.meta.TracksForGenre(g)
	names := make(map[int]string, len(idx))
	for _, i := range idx {
		n, _ := pl.meta.TrackTitle(i)
		if n == "" {
			n = filepath.Base(tracks[i])
		}
		names[i] = strings.ToLower(n)
	}
	sort.Slice(idx, func(x, y int) bool { return names[idx[x]] < names[idx[y]] })
	return idx
}

// plGenreScrollDrag permite arrastar o thumb da scrollbar de gêneros.
func (pl *plw) plGenreScrollDrag(gtx layout.Context, track image.Rectangle, thumbY, thumbH, viewH int, maxScroll float32) {
	if maxScroll <= 0 {
		return
	}
	travel := viewH - 2 - thumbH
	if travel <= 0 {
		return
	}
	trackTop := float32(track.Min.Y)
	cs := clip.Rect(track).Push(gtx.Ops)
	event.Op(gtx.Ops, &plGenreScrollTag)
	cs.Pop()
	for {
		ev, ok := gtx.Event(pointer.Filter{
			Target: &plGenreScrollTag,
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
			pl.plScrollDragging = true
		case pointer.Drag:
			if !pl.plScrollDragging {
				continue
			}
			frac := (e.Position.Y - trackTop) / float32(travel)
			if frac < 0 {
				frac = 0
			}
			if frac > 1 {
				frac = 1
			}
			pl.plGenreScroll = frac * maxScroll
		case pointer.Release, pointer.Cancel:
			pl.plScrollDragging = false
		}
	}
}

// albumTracks retorna os índices das faixas do álbum em ordem alfabética.
func (pl *plw) albumTracks(a string, tracks []string) []int {
	idx := pl.meta.AlbumTracks(a)
	names := make(map[int]string, len(idx))
	for _, i := range idx {
		n, _ := pl.meta.TrackTitle(i)
		if n == "" {
			n = filepath.Base(tracks[i])
		}
		names[i] = strings.ToLower(n)
	}
	sort.Slice(idx, func(x, y int) bool { return names[idx[x]] < names[idx[y]] })
	return idx
}

// folderTracks retorna os índices das faixas do diretório em ordem alfabética.
func (pl *plw) folderTracks(dir string, tracks []string) []int {
	idx := pl.meta.FolderTracks(dir)
	names := make(map[int]string, len(idx))
	for _, i := range idx {
		n, _ := pl.meta.TrackTitle(i)
		if n == "" {
			n = filepath.Base(tracks[i])
		}
		names[i] = strings.ToLower(n)
	}
	sort.Slice(idx, func(x, y int) bool { return names[idx[x]] < names[idx[y]] })
	return idx
}

// plBottom desenha o rodapé de 38px: menus à esquerda, transporte à direita.
func (pl *plw) plBottom(gtx layout.Context, winW, winH, botY int, tracks []string) {
	R := func(x, y, w, h int) image.Rectangle { return image.Rect(x, y, x+w, y+h) }

	// tiles do rodapé
	pl.ui.spr(gtx, "PLEDIT", R(0, 72, 125, 38), R(0, botY, 125, 38))
	pl.ui.spr(gtx, "PLEDIT", R(126, 72, 150, 38), R(winW-150, botY, 150, 38))
	if winW > 280 {
		pl.ui.spr(gtx, "PLEDIT", R(179, 0, 25, 38), R(125, botY, winW-275, 38))
	}

	// ---- menus suspensos (bottom:12 → y=botY+8) ----
	type menuDef struct {
		btn   *uibtn
		faceX int // x do sprite da face (linha y149)
		sy    int
		items []plMenuItem
	}
	clearAll := func() { pl.applyTracks([]string{}); pl.plSel = -1 }
	sortList := func() {
		cp := append([]string(nil), tracks...)
		sort.Strings(cp)
		pl.applyTracks(cp)
	}
	writeList := func() { pl.writeM3U(tracks) }
	removeSel := func() {
		if pl.plSel >= 0 && pl.plSel < len(tracks) {
			pl.removeAt(pl.plSel)
			pl.plSel = -1
		}
	}

	menus := []menuDef{
		{&pl.btnPLMAdd, 0, 149, []plMenuItem{
			{0, 111, nil},
			{0, 130, func() { pl.addPicked(true) }},
			{0, 149, func() { pl.addPicked(false) }},
		}},
		{&pl.btnPLMRem, 54, 149, []plMenuItem{
			{54, 111, clearAll}, {54, 130, nil}, {54, 149, removeSel}, {54, 168, nil},
		}},
		{&pl.btnPLMSel, 104, 149, []plMenuItem{
			{104, 111, nil}, {104, 130, func() { pl.plSel = -1 }}, {104, 149, nil},
		}},
		{&pl.btnPLMMisc, 154, 149, []plMenuItem{
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
		if pl.ui.spriteBtn(gtx, mn.btn, "PLEDIT", dst, faceSrc, faceSel) {
			if pl.plMenuOpen == mi {
				pl.plMenuOpen = -1
			} else {
				pl.plMenuOpen = mi
			}
		}
		// popup de itens (acima do botão)
		if pl.plMenuOpen == mi {
			n := len(mn.items)
			for ii, it := range mn.items {
				iy := by - (n-ii)*18
				idst := R(bx, iy, 22, 18)
				nrm := R(it.sx, it.sy, 22, 18)
				sel := R(it.sx+23, it.sy, 22, 18)
				for len(pl.plMenuClicks[mi]) <= ii {
					pl.plMenuClicks[mi] = append(pl.plMenuClicks[mi], uibtn{})
				}
				if pl.ui.spriteBtn(gtx, &pl.plMenuClicks[mi][ii], "PLEDIT", idst, nrm, sel) {
					pl.plMenuOpen = -1
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
		{&pl.btnPLPrev, 0, 23, func() { pl.prev(); pl.play() }},
		{&pl.btnPLPlay, 23, 23, func() { pl.play() }},
		{&pl.btnPLPause, 46, 23, func() { pl.pause() }},
		{&pl.btnPLStop, 69, 23, func() { pl.pause(); _ = pl.seek(0) }},
		{&pl.btnPLNext, 92, 22, func() { pl.next(); pl.play() }},
	}
	for i, b := range tbtns {
		dst := R(rx+3+i*10, botY+22, 10, 10)
		if pl.ui.spriteBtn(gtx, b.b, "CBUTTONS", dst,
			R(b.col, 0, b.w, 18), R(b.col, 18, b.w, 18)) {
			b.fn()
		}
	}
	// eject
	if pl.ui.spriteBtn(gtx, &pl.btnPLEject, "CBUTTONS", R(rx+53, botY+22, 10, 10),
		R(114, 0, 22, 16), R(114, 16, 22, 16)) {
		pl.pause()
		_ = pl.seek(0)
	}
	// menu List (new/save/load) à direita
	lx := winW - 44
	ly := botY + 8
	faceSrc := R(204, 149, 22, 18)
	faceSel := R(227, 149, 22, 18)
	if pl.ui.spriteBtn(gtx, &pl.btnPLMList, "PLEDIT", R(lx, ly, 22, 18), faceSrc, faceSel) {
		if pl.plMenuOpen == 4 {
			pl.plMenuOpen = -1
		} else {
			pl.plMenuOpen = 4
		}
	}
	if pl.plMenuOpen == 4 {
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
			for len(pl.plMenuClicks[4]) <= ii {
				pl.plMenuClicks[4] = append(pl.plMenuClicks[4], uibtn{})
			}
			if pl.ui.spriteBtn(gtx, &pl.plMenuClicks[4][ii], "PLEDIT", R(lx, iy, 22, 18), nrm, sel) {
				pl.plMenuOpen = -1
				if it.fn != nil {
					it.fn()
				}
			}
		}
	}

	// barra de progresso no espaço liberado à direita (antiga caixa do time)
	barR := image.Rect(winW-144, botY+8, winW-55, botY+18)
	if barR.Min.X > plMenuX+4*29+72 {
		pl.plPosBar(gtx, barR)
	}

	// contagem: painel aberto = categorias/faixas da categoria expandida;
	// lista normal = total de faixas
	if pl.plCount > 0 {
		bx, by := plMenuX+4*29+4, botY+9
		box := image.Rect(bx, by, bx+64, by+16)
		if box.Max.X+8 < barR.Min.X {
			panelOpen := pl.plGenreOpen || pl.plArtistOpen || pl.plFolderOpen || pl.plAlbumOpen
			s := fmt.Sprintf("%d", pl.plCount)
			if panelOpen {
				s = fmt.Sprintf("%d/%d", pl.plCount, pl.plCount2)
			}
			x := box.Max.X - 5 - len(s)*10
			lcdGreen := color.NRGBA{R: 0, G: 0xF8, B: 0, A: 0xFF}
			for i, ch := range s {
				cx := x + i*10
				if ch == '/' {
					// '/' em blocos (NUMBERS só tem dígitos)
					for j := 0; j < 5; j++ {
						fillRect(gtx.Ops,
							image.Rect(cx+5-j, by+2+j*2, cx+7-j, by+4+j*2), lcdGreen)
					}
					continue
				}
				d := int(ch - '0')
				pl.ui.spr(gtx, "NUMBERS", image.Rect(d*9, 0, d*9+9, 13),
					image.Rect(cx, by+2, cx+9, by+15))
			}
		}
	}

	// setas de scroll (decorativas)
	pl.plScrollArrow(gtx, winW-15, botY+2, true)
	pl.plScrollArrow(gtx, winW-15, botY+8, false)

	// orelha de redimensionamento (canto inferior direito)
	pl.plResize(gtx, image.Rect(winW-16, botY+20, winW-2, botY+36), winW, winH)
}

// plPosBar desenha a barra de progresso da música no rodapé da Playlist,
// com o mesmo sprite POSBAR e indicador do player.
func (pl *plw) plPosBar(gtx layout.Context, barR image.Rectangle) {
	pos, dur := pl.position(), pl.duration()
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
	if !pl.plPosDrag.active {
		pl.plPosDrag.val = float32(frac)
	}
	pl.ui.sprFit(gtx, "POSBAR", image.Rect(0, 0, 248, 10), barR)
	// indicador (mesmo sprite do player, escalado)
	kw := 22
	if barR.Dx() < 80 {
		kw = 16
	}
	travel := barR.Dx() - kw
	if travel < 0 {
		travel = 0
	}
	kx := barR.Min.X + int(float32(travel)*pl.plPosDrag.val)
	pl.ui.sprFit(gtx, "POSBAR", image.Rect(248, 0, 277, 10),
		image.Rect(kx, barR.Max.Y-10, kx+kw, barR.Max.Y))
	pl.ui.scrub(gtx, &pl.plPosDrag, image.Rect(barR.Min.X, barR.Min.Y, barR.Max.X, barR.Max.Y+10))
}

// plResize registra drag na orelha do canto inferior direito e
// redimensiona a janela via X11 (wmResize).
func (pl *plw) plResize(gtx layout.Context, r image.Rectangle, winW, winH int) {
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
			scale := pl.plScale
			if scale < 1 {
				scale = 1
			}
			go func() {
				wmResizeDrag(pl.name, winW*scale, winH*scale,
					300*scale, 140*scale)
				time.Sleep(150 * time.Millisecond)
				pl.savePLSize()
			}()
		}
	}
}

// plScrollArrow desenha seta up/down 8x5.
func (pl *plw) plScrollArrow(gtx layout.Context, x, y int, up bool) {
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
func (pl *plw) plTimeText() string {
	return pl.fmtTime(pl.position().Seconds())
}

func (pl *plw) fmtTime(sec float64) string {
	s := int(sec)
	if s < 0 {
		s = 0
	}
	return fmt.Sprintf("%d:%02d", (s/60)%100, s%60)
}

// plRow desenha uma linha da playlist (plRowH px, fonte verde).
// order != nil define a sequência de reprodução (grupo expandido);
// nil = playlist inteira.
func (pl *plw) plRow(gtx layout.Context, i int, tracks []string, w int, order []int) layout.Dimensions {
	h := plRowH
	r := image.Rect(0, 0, w, h)
	cur := i == pl.curIdx()
	sel := i == pl.plSel
	if cur {
		fillRect(gtx.Ops, r, colPLCurBg)
	}
	if sel {
		fillRect(gtx.Ops, r, colPLSelBg)
	}
	artist, title, hasMeta := pl.meta.TrackParts(i)
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
	if m := pl.meta.TrackMeta(tracks[i]); m != nil {
		var sb strings.Builder
		if m.Rating > 0 {
			fmt.Fprintf(&sb, "%d/5 ", m.Rating)
		}
		if m.Plays > 0 {
			fmt.Fprintf(&sb, "x%d ", m.Plays)
		}
		for j, t := range m.Hashtags {
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
	if img := pl.meta.TrackCover(i); img != nil {
		blit(gtx, img, image.Rect(1, 1, 41, 41))
	}
	if twoLine {
		pl.ui.textAt(gtx, pl.ui.fontUI, unit.Sp(9), artist, c, image.Pt(45, 9))
		pl.ui.textAt(gtx, pl.ui.fontUI, unit.Sp(9), name, c, image.Pt(45, 23))
	} else {
		pl.ui.textAt(gtx, pl.ui.fontUI, unit.Sp(9), name, c, image.Pt(45, 16))
	}
	for len(pl.plRowClicks) <= i {
		pl.plRowClicks = append(pl.plRowClicks, uibtn{})
	}
	// a roda do mouse é consumida pelo botão da linha (pl.ui.button filtra
	// Scroll); acumula o delta e aplica após o Layout para não corromper
	// a iteração do List
	sink := &pl.plScrollAcc
	if pl.plScrollSink != nil {
		sink = pl.plScrollSink
	}
	pl.plRowClicks[i].scrollFn = func(dy float32) { *sink += dy }
	b := &pl.plRowClicks[i]
	if pl.ui.button(gtx, b, r, func(gtx layout.Context, down bool) {}) {
		pl.plSel = i
		now := time.Now()
		if now.Sub(b.lastClick) < 500*time.Millisecond {
			b.lastClick = time.Time{}
			if order != nil {
				_ = pl.playInOrder(i, order)
			} else {
				pl.playIndex(i)
			}
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
