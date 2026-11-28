// Package native implementa a interface gráfica do sfrega com Gio,
// clonando visualmente o Winamp 2.x (janela principal, equalizador,
// playlist) de forma independente de navegador.
package native

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
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

	xdraw "golang.org/x/image/draw"

	"sfrega/internal/player"
	"sfrega/internal/skins"
	"sfrega/internal/spectrum"
)

// Paleta Winamp 2.x (skin clássica cinza).

// ontopWinNames são as janelas que seguem o flag always-on-top.
var ontopWinNames = []string{"sfrega", "Equalizador", "Playlist", "Radio", "Video", "Menu", "Confirmar"}

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
	mu           sync.Mutex // protege winEQ e as janelas pl*
	p            *player.Player
	skin         atomic.Pointer[skins.Skin]
	shaper       *text.Shaper
	fontUI       font.Font
	fontBold     font.Font
	fontMono     font.Font
	fontMenu     font.Font
	fontMenuBold font.Font

	// janelas
	winMain    *app.Window
	winEQ      *app.Window
	winMenu    *app.Window
	winConfirm *app.Window
	eqOpen     bool
	menuOpen   bool

	// janelas de playlist: uma lista ativa por vez (activePL), cada uma com
	// sua própria lista de faixas persistida no seu json.
	plMain, plRadio, plVideo *plw
	activePL                 *plw

	confirmOpen  bool
	confirmKind  int // 1 = shuffle, 2 = plays
	confirmScale int

	mainScale, eqScale int

	// widgets main
	btnPrev, btnPlay, btnPause, btnStop, btnNext uibtn
	btnEject, btnMin, btnSize, btnClose          uibtn
	btnOpt                                       uibtn
	btnEQ, btnPL                                 uibtn
	btnShuf, btnRep                              uibtn
	btnGenex, btnGenex2                          uibtn
	volDrag, balDrag, posDrag                    fader

	// widgets EQ
	eqSliders                       [10]fader
	preampSlide                     fader
	eqGains                         [10]float64 // dB
	btnEQClose, btnEQMin, btnEQSize uibtn
	btnEQOn, btnEQAuto, btnEQPres   uibtn
	eqOn                            bool
	eqAuto                          bool
	eqPresOpen                      bool
	eqPresetBtns                    [6]uibtn

	shapeMu sync.Mutex // text.Shaper não é thread-safe (3 janelas)

	shadeMain, shadeEQ shadeClk

	// menu suspenso do winshade (ícone de onda → janela Menu)
	btnWave      uibtn
	btnMenuMin   uibtn
	btnMenuSize  uibtn
	btnMenuClose uibtn
	menuBtns     [16]uibtn
	menuFile     bool
	menuCfg      bool
	menuLang     bool
	menuSkins    bool
	menuCnt      bool
	menuSpec     bool
	menuAbout    bool
	alwaysOnTop  bool
	menuScale    int
	menuScroll   int
	menuBarDown  bool
	menuBarGrab  float32
	menuWheelTag event.Tag
	menuBarTag   event.Tag
	updPhase     int // 0=idle, 1=update lists ativo, 3=fade-out
	updT0        time.Time
	confirmBtns  [2]uibtn
	ontopDone    map[string]bool

	shuffle, repeat bool
	marqueeX        int
	marqueeTitle    string
	marqueeW        int // largura real do título (medida via shaper)
	lastTick        time.Time

	// variante do analisador de espectro (~/.local/share/sfrega/spectrum)
	spec      *spectrum.Spec
	specName  string
	specPeaks [spectrum.Bands]float32
	specCap   [spectrum.Bands]float32 // capa branca: persegue o pico com atraso
}

// wheelMul amplifica o delta da roda (o X11 entrega ~10px por notch).
const wheelMul = 5

// uibtn é um botão simples via pointer events (sem macro do Clickable).
type uibtn struct {
	tag       event.Tag
	down      bool
	over      bool
	scrollFn  func(dy float32)   // opcional: roda do mouse sobre o botão
	dragFn    func(dx, dy float32) // opcional: chamado a cada Drag (dx,dy = delta)
	dragDX    float32            // delta acumulado desde o Press
	dragDY    float32
	dragPos   f32.Point          // última posição do Drag
	moved     bool               // houve Drag (suprime o clique)
	released  bool               // Release neste frame (mesmo após drag)
	lastClick time.Time          // último clique (detecção de duplo-clique)
	lastBtns  pointer.Buttons    // botões do último Press (secundário = direito)
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

// configPath retorna ~/.local/share/sfrega/configs/<name>.
func configPath(name string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "/tmp"
	}
	return filepath.Join(home, ".local", "share", "sfrega", "configs", name)
}

// loadScale lê a escala persistida de uma janela (1 ou 2).
func loadScale(file string) int {
	b, err := os.ReadFile(configPath(file))
	if err != nil {
		return 1
	}
	s, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil || s < 1 {
		return 1
	}
	return s
}

// saveScale grava a escala de uma janela para a próxima sessão.
func saveScale(file string, s int) {
	p := configPath(file)
	_ = os.MkdirAll(filepath.Dir(p), 0o755)
	_ = os.WriteFile(p, []byte(strconv.Itoa(s)+"\n"), 0o644)
}

func loadMainScale() int  { return loadScale("scale.txt") }
func saveMainScale(s int) { saveScale("scale.txt", s) }

// loadShade lê se a janela principal deve abrir em modo shade.
func loadShade() bool {
	b, err := os.ReadFile(configPath("shade.txt"))
	return err == nil && strings.TrimSpace(string(b)) == "1"
}

// saveShade grava o estado shade da janela principal.
func saveShade(on bool) {
	p := configPath("shade.txt")
	_ = os.MkdirAll(filepath.Dir(p), 0o755)
	v := "0\n"
	if on {
		v = "1\n"
	}
	_ = os.WriteFile(p, []byte(v), 0o644)
}

// loadPLShade lê se a janela Playlist deve abrir em modo shade.
func loadPLShade() bool {
	b, err := os.ReadFile(configPath("plshade.txt"))
	return err == nil && strings.TrimSpace(string(b)) == "1"
}

// savePLShade grava o estado shade da janela Playlist.
func savePLShade(on bool) {
	p := configPath("plshade.txt")
	_ = os.MkdirAll(filepath.Dir(p), 0o755)
	v := "0\n"
	if on {
		v = "1\n"
	}
	_ = os.WriteFile(p, []byte(v), 0o644)
}

// loadEQShade lê se a janela EQ deve abrir em modo shade.
func loadEQShade() bool {
	b, err := os.ReadFile(configPath("eqshade.txt"))
	return err == nil && strings.TrimSpace(string(b)) == "1"
}

// saveEQShade grava o estado shade da janela EQ.
func saveEQShade(on bool) {
	p := configPath("eqshade.txt")
	_ = os.MkdirAll(filepath.Dir(p), 0o755)
	v := "0\n"
	if on {
		v = "1\n"
	}
	_ = os.WriteFile(p, []byte(v), 0o644)
}

// loadGenreOpen lê se a fileira de gêneros da Playlist abre expandida.
func loadGenreOpen() bool {
	b, err := os.ReadFile(configPath("genreopen.txt"))
	return err == nil && strings.TrimSpace(string(b)) == "1"
}

// saveGenreOpen grava o estado da fileira de gêneros.
func saveGenreOpen(on bool) {
	p := configPath("genreopen.txt")
	_ = os.MkdirAll(filepath.Dir(p), 0o755)
	v := "0\n"
	if on {
		v = "1\n"
	}
	_ = os.WriteFile(p, []byte(v), 0o644)
}

// loadGenreSel lê o gênero selecionado persistido ("" = todos).
func loadGenreSel() string {
	b, err := os.ReadFile(configPath("genre.txt"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// saveGenreSel grava o gênero selecionado na Playlist.
func saveGenreSel(g string) {
	p := configPath("genre.txt")
	_ = os.MkdirAll(filepath.Dir(p), 0o755)
	_ = os.WriteFile(p, []byte(g), 0o644)
}

// loadGenreOn lê se o filtro de gênero está ativo.
func loadGenreOn() bool {
	b, err := os.ReadFile(configPath("genreon.txt"))
	return err == nil && strings.TrimSpace(string(b)) == "1"
}

// saveGenreOn grava se o filtro de gênero está ativo.
func saveGenreOn(on bool) {
	p := configPath("genreon.txt")
	_ = os.MkdirAll(filepath.Dir(p), 0o755)
	v := "0\n"
	if on {
		v = "1\n"
	}
	_ = os.WriteFile(p, []byte(v), 0o644)
}

// loadArtistOpen lê se a fileira de artistas da Playlist abre expandida.
func loadArtistOpen() bool {
	b, err := os.ReadFile(configPath("artistopen.txt"))
	return err == nil && strings.TrimSpace(string(b)) == "1"
}

// saveArtistOpen grava o estado da fileira de artistas.
func saveArtistOpen(on bool) {
	p := configPath("artistopen.txt")
	_ = os.MkdirAll(filepath.Dir(p), 0o755)
	v := "0\n"
	if on {
		v = "1\n"
	}
	_ = os.WriteFile(p, []byte(v), 0o644)
}

// loadFolderOpen lê se a fileira de pastas da Playlist abre expandida.
func loadFolderOpen() bool {
	b, err := os.ReadFile(configPath("folderopen.txt"))
	return err == nil && strings.TrimSpace(string(b)) == "1"
}

// saveFolderOpen grava o estado da fileira de pastas.
func saveFolderOpen(on bool) {
	p := configPath("folderopen.txt")
	_ = os.MkdirAll(filepath.Dir(p), 0o755)
	v := "0\n"
	if on {
		v = "1\n"
	}
	_ = os.WriteFile(p, []byte(v), 0o644)
}

// loadAlbumOpen lê se o painel de álbuns da Playlist abre expandido.
func loadAlbumOpen() bool {
	b, err := os.ReadFile(configPath("albumopen.txt"))
	return err == nil && strings.TrimSpace(string(b)) == "1"
}

// saveAlbumOpen grava o estado do painel de álbuns.
func saveAlbumOpen(on bool) {
	p := configPath("albumopen.txt")
	_ = os.MkdirAll(filepath.Dir(p), 0o755)
	v := "0\n"
	if on {
		v = "1\n"
	}
	_ = os.WriteFile(p, []byte(v), 0o644)
}

// loadExpand lê a seleção expandida persistida de um painel ("" = nenhuma).
func loadExpand(file string) string {
	b, err := os.ReadFile(configPath(file))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// saveExpand grava a seleção expandida de um painel.
func saveExpand(file, v string) {
	p := configPath(file)
	_ = os.MkdirAll(filepath.Dir(p), 0o755)
	_ = os.WriteFile(p, []byte(v+"\n"), 0o644)
}

// loadGenSel lê o botão GENEX selecionado persistido (-1 = nenhum).
func loadGenSel() int {
	b, err := os.ReadFile(configPath("gensex.txt"))
	if err != nil {
		return -1
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil {
		return -1
	}
	return n
}

// saveGenSel grava o botão GENEX selecionado (-1 = nenhum).
func saveGenSel(i int) {
	p := configPath("gensex.txt")
	_ = os.MkdirAll(filepath.Dir(p), 0o755)
	_ = os.WriteFile(p, []byte(strconv.Itoa(i)), 0o644)
}

// loadFlag lê uma flag booleana persistida ("1" = true).
func loadFlag(file string) bool {
	b, err := os.ReadFile(configPath(file))
	return err == nil && strings.TrimSpace(string(b)) == "1"
}

// saveFlag grava uma flag booleana para a próxima sessão.
func saveFlag(file string, on bool) {
	p := configPath(file)
	_ = os.MkdirAll(filepath.Dir(p), 0o755)
	v := "0\n"
	if on {
		v = "1\n"
	}
	_ = os.WriteFile(p, []byte(v), 0o644)
}

// loadOntop lê se a janela principal deve abrir sempre no topo.
func loadOntop() bool {
	b, err := os.ReadFile(configPath("ontop.txt"))
	return err == nil && strings.TrimSpace(string(b)) == "1"
}

// saveOntop grava o estado always-on-top da janela principal.
func saveOntop(on bool) {
	p := configPath("ontop.txt")
	_ = os.MkdirAll(filepath.Dir(p), 0o755)
	v := "0\n"
	if on {
		v = "1\n"
	}
	_ = os.WriteFile(p, []byte(v), 0o644)
}

// loadPLSize lê o tamanho lógico persistido da janela Playlist (0,0 = padrão).
func loadPLSize() (int, int) {
	b, err := os.ReadFile(configPath("plsize.txt"))
	if err != nil {
		return 0, 0
	}
	var w, h int
	if n, _ := fmt.Sscanf(string(b), "%d %d", &w, &h); n != 2 || w <= 0 || h <= 0 {
		return 0, 0
	}
	return w, h
}

// savePLSize grava o tamanho lógico da janela Playlist (principal).
func (pl *plw) savePLSize() {
	if pl.key != "pl" {
		pl.saveJSON()
		return
	}
	w, h := int(pl.plWidth), int(pl.plHeight)
	if w <= 0 || h <= 0 {
		return
	}
	p := configPath("plsize.txt")
	_ = os.MkdirAll(filepath.Dir(p), 0o755)
	_ = os.WriteFile(p, []byte(fmt.Sprintf("%d %d\n", w, h)), 0o644)
}

// savePLWidth grava só a largura da Playlist, preservando a altura salva
// (usado no resize pela barra de shade, onde a altura lógica é a da barra).
func (pl *plw) savePLWidth() {
	if pl.key != "pl" {
		pl.saveJSON()
		return
	}
	w := int(pl.plWidth)
	if w <= 0 {
		return
	}
	_, h := loadPLSize()
	if h <= 0 {
		h = 240
	}
	p := configPath("plsize.txt")
	_ = os.MkdirAll(filepath.Dir(p), 0o755)
	_ = os.WriteFile(p, []byte(fmt.Sprintf("%d %d\n", w, h)), 0o644)
}

// loadOpenWindows lê quais janelas secundárias estavam abertas.
func loadOpenWindows() (eq, pl, menu, rd, vd bool) {
	b, err := os.ReadFile(configPath("windows.txt"))
	if err != nil {
		return
	}
	for _, f := range strings.Fields(string(b)) {
		switch f {
		case "eq":
			eq = true
		case "pl":
			pl = true
		case "menu":
			menu = true
		case "rd":
			rd = true
		case "vd":
			vd = true
		}
	}
	return
}

// saveOpenWindows grava quais janelas secundárias estão abertas.
func saveOpenWindows(eq, pl, menu, rd, vd bool) {
	var names []string
	if eq {
		names = append(names, "eq")
	}
	if pl {
		names = append(names, "pl")
	}
	if rd {
		names = append(names, "rd")
	}
	if vd {
		names = append(names, "vd")
	}
	if menu {
		names = append(names, "menu")
	}
	p := configPath("windows.txt")
	_ = os.MkdirAll(filepath.Dir(p), 0o755)
	_ = os.WriteFile(p, []byte(strings.Join(names, " ")+"\n"), 0o644)
}

// loadSpecSel lê a variante de espectro escolhida ("" = padrão).
func loadSpecSel() string {
	b, err := os.ReadFile(configPath("spectrum.txt"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// saveSpecSel grava a variante de espectro para a próxima sessão.
func saveSpecSel(code string) {
	p := configPath("spectrum.txt")
	_ = os.MkdirAll(filepath.Dir(p), 0o755)
	_ = os.WriteFile(p, []byte(code+"\n"), 0o644)
}

// applySpec troca a variante do analisador ("" ou "padrao" = visual original).
func (ui *uiState) applySpec(code string) {
	ui.specName = code
	if code == "" || code == "padrao" {
		ui.spec = nil
	} else if s := spectrum.LoadSpec(code); s != nil {
		ui.spec = s
	} else {
		ui.spec = nil
		ui.specName = "padrao"
	}
	ui.specPeaks = [spectrum.Bands]float32{}
	ui.specCap = [spectrum.Bands]float32{}
	ui.mu.Lock()
	if ui.winMain != nil {
		ui.winMain.Invalidate()
	}
	ui.mu.Unlock()
}

// loadSkinSel lê a skin escolhida ("" = nenhuma/padrão automático).
func loadSkinSel() string {
	b, err := os.ReadFile(configPath("skin.txt"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// saveSkinSel grava a skin escolhida para a próxima sessão.
func saveSkinSel(code string) {
	p := configPath("skin.txt")
	_ = os.MkdirAll(filepath.Dir(p), 0o755)
	_ = os.WriteFile(p, []byte(code+"\n"), 0o644)
}

// applySkin troca a skin ativa: "padrao" usa a pasta 00 embutida; demais
// códigos são nomes de .wsz embutidos. Persiste a escolha.
func (ui *uiState) applySkin(code string) {
	var s *skins.Skin
	var err error
	if code == "" || code == "padrao" {
		s, err = skins.OpenEmbedded(".")
	} else {
		s, err = skins.OpenWsz(code)
	}
	if err != nil {
		return
	}
	ui.skin.Store(s)
	fitCache.Range(func(k, _ any) bool {
		fitCache.Delete(k)
		return true
	})
	glyphCache.Range(func(k, _ any) bool {
		glyphCache.Delete(k)
		return true
	})
	ui.mu.Lock()
	var plWins []*app.Window
	for _, pl := range []*plw{ui.plMain, ui.plRadio, ui.plVideo} {
		if pl != nil && pl.win != nil {
			plWins = append(plWins, pl.win)
		}
	}
	for _, w := range append([]*app.Window{ui.winMain, ui.winEQ, ui.winMenu, ui.winConfirm}, plWins...) {
		if w != nil {
			w.Invalidate()
		}
	}
	ui.mu.Unlock()
}

// updateAllLists re-varre os diretórios importados por cada janela de
// playlist (Playlist/Radio/Video) em background, anexando só faixas novas.
func (ui *uiState) updateAllLists() {
	for _, pl := range []*plw{ui.plMain, ui.plRadio, ui.plVideo} {
		if pl != nil {
			pl.updateFromDirs()
		}
	}
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
// sc, quando não-nil, habilita winshade: duplo-clique na área alterna a barra.
func dragAreaX11(gtx layout.Context, r image.Rectangle, sc *shadeClk) {
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
				if sc != nil && sc.dblClick(e.Position) {
					sc.on = !sc.on
					if sc.onChange != nil {
						sc.onChange(sc.on)
					}
					wmShadeToggle(sc.px)
				} else {
					dragActive = true
					wmDragBegin()
				}
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

// shadeClk detecta duplo-clique numa área de título (dentro de dragAreaX11)
// e alterna o "winshade" (janela reduzida à altura da barra). Um por janela.
type shadeClk struct {
	last     time.Time
	x, y     float32
	on       bool
	px       int // altura da barra de shade em pixels
	onChange func(on bool)
}

// arm define a altura de shade em pixels (chamado a cada frame).
func (sc *shadeClk) arm(px int) { sc.px = px }

// dblClick registra o press e retorna true se for o segundo clique rápido
// perto do anterior (duplo-clique).
func (sc *shadeClk) dblClick(p f32.Point) bool {
	now := time.Now()
	dbl := now.Sub(sc.last) < 400*time.Millisecond &&
		math.Abs(float64(p.X-sc.x)) < 8 &&
		math.Abs(float64(p.Y-sc.y)) < 8
	sc.last, sc.x, sc.y = now, p.X, p.Y
	return dbl
}

// shadePx converte altura lógica da barra de shade para pixels na escala atual.
func shadePx(scale, logical int) int {
	if scale < 1 {
		scale = 1
	}
	return logical * scale
}

// button registra hit-area, processa press/release e desenha via face.
// Retorna true no frame do clique (release dentro).
func (ui *uiState) button(gtx layout.Context, b *uibtn, r image.Rectangle, face func(gtx layout.Context, down bool)) bool {
	clipStack := clip.Rect(r).Push(gtx.Ops)
	event.Op(gtx.Ops, &b.tag)
	clipStack.Pop()
	clicked := false
	b.released = false
	for {
		ev, ok := gtx.Event(pointer.Filter{
			Target:  &b.tag,
			Kinds:   pointer.Press | pointer.Release | pointer.Drag | pointer.Move | pointer.Cancel | pointer.Enter | pointer.Leave | pointer.Scroll,
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
		switch e.Kind {
		case pointer.Press:
			b.down = true
			b.lastBtns = e.Buttons
			b.moved = false
			b.dragDX, b.dragDY = 0, 0
			b.dragPos = e.Position
		case pointer.Drag:
			dx, dy := e.Position.X-b.dragPos.X, e.Position.Y-b.dragPos.Y
			b.dragPos = e.Position
			if dx != 0 || dy != 0 {
				b.moved = true
			}
			b.dragDX += dx
			b.dragDY += dy
			if b.dragFn != nil {
				b.dragFn(dx, dy)
			}
		case pointer.Release:
			b.released = true
			if b.down && !b.moved {
				clicked = true
			}
			b.down = false
		case pointer.Enter:
			b.over = true
		case pointer.Leave:
			b.over = false
			b.down = false
		case pointer.Cancel:
			b.down = false
		case pointer.Scroll:
			if b.scrollFn != nil {
				b.scrollFn(e.Scroll.Y * wheelMul)
			}
		}
	}
	off := op.Offset(r.Min).Push(gtx.Ops)
	face(gtx, b.down)
	off.Pop()
	return clicked
}

// fader é um slider vertical/horizontal arrastável.
type fader struct {
	tag           event.Tag
	val           float32 // 0..1
	active        bool
	releaseCommit bool // commita só no release (ex.: seek)
	commit        func(float32)
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
		// fallbacks p/ caracteres especiais: CJK/largura total e símbolos
		"/usr/share/fonts/noto-cjk/NotoSansCJK-Regular.ttc",
		"/usr/share/fonts/noto/NotoSansSymbols-Regular.ttf",
		"/usr/share/fonts/noto/NotoSans-Regular.ttf",
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
func Run(p *player.Player, musicDirs []string) error {
	skin, err := skins.Default()
	if err != nil {
		return err
	}
	faces, _ := loadFonts()
	shaper := text.NewShaper(text.WithCollection(faces))
	ui := &uiState{
		p:            p,
		shaper:       shaper,
		fontUI:       font.Font{Typeface: "DejaVu Sans"},
		fontBold:     font.Font{Typeface: "DejaVu Sans", Weight: font.Bold},
		fontMono:     font.Font{Typeface: "DejaVu Sans Mono"},
		fontMenu:     font.Font{Typeface: "Liberation Sans"},
		fontMenuBold: font.Font{Typeface: "Liberation Sans", Weight: font.Bold},
		shuffle:      loadFlag("shuffle.txt"),
		repeat:       loadFlag("repeat.txt"),
	}
	p.SetShuffle(ui.shuffle)
	p.SetRepeat(ui.repeat)
	ui.volDrag.val = 1.0
	ui.skin.Store(skin)
	if sel := loadSkinSel(); sel != "" {
		ui.applySkin(sel)
	}
	ui.applySpec(loadSpecSel())
	ui.menuScale = 1
	ui.ontopDone = map[string]bool{}
	ui.volDrag.commit = func(v float32) { p.SetVolume(v) }
	ui.eqOn = true
	ui.mainScale = loadMainScale()
	ui.eqScale = loadScale("eqscale.txt")
	ui.shadeEQ.onChange = func(on bool) { saveEQShade(on) }
	// três janelas de playlist independentes (Playlist, Radio, Video)
	ui.plMain = ui.newPLW("Playlist", "pl")
	ui.plRadio = ui.newPLW("Radio", "rd")
	ui.plVideo = ui.newPLW("Video", "vd")
	ui.activePL = ui.plMain
	// diretórios do scan inicial entram nos dirs da Playlist p/ Update lists
	for _, d := range musicDirs {
		ui.plMain.addDir(d)
	}
	// Update lists automático na abertura: busca músicas novas nos dirs
	// já importados (dedup por caminho — só entra o inédito).
	go ui.updateAllLists()
	ui.balDrag.val = 0.5
	ui.balDrag.commit = func(v float32) { p.SetBalance(v*2 - 1) }
	ui.posDrag.commit = func(v float32) { p.SeekFraction(float64(v)) }
	ui.posDrag.releaseCommit = true

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
	// restaura estado persistido do EQ (ON/AUTO/presets + ganhos) e aplica
	ui.loadEQState()
	ui.applyEQ()

	ui.winMain = new(app.Window)
	ui.winMain.Option(
		app.Title("sfrega"),
		app.Decorated(false),
		app.Size(unit.Dp(275*ui.mainScale), unit.Dp(107*ui.mainScale)),
	)

	// restaura estado shade persistido
	ui.shadeMain.on = loadShade()
	ui.shadeMain.onChange = func(on bool) { saveShade(on) }
	shadePending := ui.shadeMain.on
	ui.alwaysOnTop = loadOntop()
	ontopPending := ui.alwaysOnTop

	// X11/muffin: aplica _MOTIF_WM_HINTS assim que a janela aparece
	// em _NET_CLIENT_LIST — polling curto evita a piscada de titlebar.
	go func() {
		t := time.NewTicker(30 * time.Millisecond)
		ontopTicks := 0
		ontopPoll := 0
		for range t.C {
			wmNoDecorations()
			wmRestoreGeometry()
			if shadePending && wmShadeSet("sfrega",
				shadePx(ui.mainScale, 18), shadePx(ui.mainScale, 107)) {
				shadePending = false
			}
			if ontopPending && ontopTicks < 40 {
				wmOntop("sfrega", true)
				ontopTicks++
				if ontopTicks >= 40 {
					ontopPending = false
				}
			}
			ui.mu.Lock()
			ontopPoll++
			if ui.alwaysOnTop && ontopPoll%10 == 0 {
				for _, n := range ontopWinNames {
					if !ui.ontopDone[n] && wmOntop(n, true) {
						ui.ontopDone[n] = true
					}
				}
			}
			ui.mu.Unlock()
		}
	}()

	// reabre as janelas secundárias que estavam abertas na sessão anterior
	go func() {
		time.Sleep(700 * time.Millisecond)
		eq, pl, menu, rd, vd := loadOpenWindows()
		if eq {
			ui.toggleEQ()
		}
		if pl {
			ui.plMain.toggle()
		}
		if menu {
			ui.toggleMenu()
		}
		if rd {
			ui.plRadio.toggle()
		}
		if vd {
			ui.plVideo.toggle()
		}
	}()

	// A janela principal precisa de ~30fps (espectro, tempo, marquee).
	// As secundárias são majoritariamente estáticas: ~8fps basta para a
	// barra de posição e mudanças de estado, e corta o custo de render
	// (principalmente da Playlist, que é grande).
	go func() {
		fast := time.NewTicker(33 * time.Millisecond)
		slow := time.NewTicker(120 * time.Millisecond)
		defer fast.Stop()
		defer slow.Stop()
		for {
			select {
			case <-fast.C:
				ui.winMain.Invalidate()
				ui.mu.Lock()
				if ui.updPhase != 0 && ui.winMenu != nil {
					ui.winMenu.Invalidate()
				}
				ui.mu.Unlock()
			case <-slow.C:
				ui.mu.Lock()
				if ui.winEQ != nil {
					ui.winEQ.Invalidate()
				}
				for _, pl := range []*plw{ui.plMain, ui.plRadio, ui.plVideo} {
					if pl != nil && pl.win != nil {
						pl.win.Invalidate()
					}
				}
				if ui.winMenu != nil {
					ui.winMenu.Invalidate()
				}
				if ui.winConfirm != nil {
					ui.winConfirm.Invalidate()
				}
				ui.mu.Unlock()
			}
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
			gtx = beginScaledFrame(gtx, ui.mainScale, image.Pt(275, 107))
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
	// remove variation selectors de emoji (U+FE0F etc.): forçam fallback para
	// fonte emoji/cor, que o shaper resolve como glifo nulo invisível
	txt = strings.Map(func(r rune) rune {
		if r >= 0xFE00 && r <= 0xFE0F {
			return -1
		}
		return r
	}, txt)
	ui.shapeMu.Lock()
	defer ui.shapeMu.Unlock()
	m := op.Record(gtx.Ops)
	paint.ColorOp{Color: c}.Add(gtx.Ops)
	mat := m.Stop()
	off := op.Offset(pos).Push(gtx.Ops)
	widget.Label{MaxLines: 1}.Layout(gtx, ui.shaper, fnt, size, txt, mat)
	off.Pop()
}

// btnText desenha s centralizado em dst com fonte vetorial —
// substitui genText, que só cobre o charset bitmap da skin.
func (ui *uiState) btnText(gtx layout.Context, s string, dst image.Rectangle, c color.NRGBA) {
	ui.btnTextF(gtx, ui.fontMenu, s, dst, c)
}

// btnTextB é btnText em negrito.
func (ui *uiState) btnTextB(gtx layout.Context, s string, dst image.Rectangle, c color.NRGBA) {
	ui.btnTextF(gtx, ui.fontMenuBold, s, dst, c)
}

func (ui *uiState) btnTextF(gtx layout.Context, fnt font.Font, s string, dst image.Rectangle, c color.NRGBA) {
	x := dst.Min.X + 4
	y := dst.Min.Y + (dst.Dy()-10)/2
	// glifos CJK são mais altos que latinos; compensa o centro óptico
	for _, r := range s {
		if r > 0x2E80 {
			y -= 2
			break
		}
	}
	inner := image.Rect(dst.Min.X+1, dst.Min.Y, dst.Max.X-1, dst.Max.Y)
	cs := clip.Rect(inner).Push(gtx.Ops)
	ui.textAt(gtx, fnt, unit.Sp(8), s, c, image.Pt(x, y))
	cs.Pop()
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
	img := ui.skin.Load().Spr(name, src)
	if img == nil {
		return
	}
	blit(gtx, img, dst)
}

// glyphCache guarda glifos já recoloridos (a varredura de pixels por frame
// era um custo desnecessário a 30fps).
var glyphCache sync.Map // glyphKey -> *image.RGBA

type glyphKey struct {
	name string
	src  image.Rectangle
	c    color.NRGBA
}

// sprGlyph desenha um glifo do bitmap name recolorido: pixels claros
// viram a cor c, o restante fica transparente.
func (ui *uiState) sprGlyph(gtx layout.Context, name string, src, dst image.Rectangle, c color.NRGBA) {
	img := ui.skin.Load().Spr(name, src)
	if img == nil {
		return
	}
	key := glyphKey{name, src, c}
	var out *image.RGBA
	if v, ok := glyphCache.Load(key); ok {
		out = v.(*image.RGBA)
	} else {
		b := img.Bounds()
		out = image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
		for y := 0; y < b.Dy(); y++ {
			for x := 0; x < b.Dx(); x++ {
				p := img.RGBAAt(b.Min.X+x, b.Min.Y+y)
				if p.R > 180 && p.G > 180 && p.B > 180 {
					out.Set(x, y, c)
				}
			}
		}
		glyphCache.Store(key, out)
	}
	blit(gtx, out, dst)
}

// genGlyphSrc devolve o retângulo do glifo A-Z na faixa inferior do GEN.png.
func genGlyphSrc(r rune) image.Rectangle {
	if r >= 'a' && r <= 'z' {
		r -= 'a' - 'A'
	}
	i := int(r - 'A')
	if i < 0 || i > 25 {
		return image.Rectangle{}
	}
	x0 := [...]int{1, 9, 17, 24, 32, 39, 46, 54, 61, 66, 73, 80, 87, 95, 102, 110, 117, 125, 132, 139, 146, 153, 160, 169, 176, 183}[i]
	x1 := [...]int{7, 14, 22, 30, 37, 43, 51, 59, 63, 69, 78, 84, 94, 100, 107, 115, 123, 130, 137, 144, 150, 158, 167, 174, 181, 188}[i]
	return image.Rect(x0, 89, x1, 95)
}

// genText desenha s com os glifos A-Z do GEN.png recoloridos em c,
// em tamanho fixo; se exceder dst, as letras excedentes são cortadas.
func (ui *uiState) genText(gtx layout.Context, s string, dst image.Rectangle, c color.NRGBA) {
	const gh = 6
	var glyphs []*image.RGBA
	w := 0
	for _, r := range s {
		src := genGlyphSrc(r)
		if src.Empty() {
			continue
		}
		g := ui.skin.Load().Spr("GEN", src)
		if g == nil {
			continue
		}
		b := g.Bounds()
		out := image.NewRGBA(image.Rect(0, 0, b.Dx(), gh))
		for y := 0; y < b.Dy() && y < gh; y++ {
			for x := 0; x < b.Dx(); x++ {
				p := g.RGBAAt(b.Min.X+x, b.Min.Y+y)
				if p.R > 180 && p.G > 180 && p.B > 180 {
					out.Set(x, y, c)
				}
			}
		}
		glyphs = append(glyphs, out)
		w += b.Dx() + 1
	}
	if len(glyphs) == 0 {
		return
	}
	w--
	inner := image.Rect(dst.Min.X+1, dst.Min.Y+1, dst.Max.X-1, dst.Max.Y-1)
	cs := clip.Rect(inner).Push(gtx.Ops)
	defer cs.Pop()
	x := dst.Min.X + (dst.Dx()-w)/2
	if x < inner.Min.X {
		x = inner.Min.X
	}
	y := dst.Min.Y + (dst.Dy()-gh)/2
	for _, g := range glyphs {
		gw := g.Bounds().Dx()
		blit(gtx, g, image.Rect(x, y, x+gw, y+gh))
		x += gw + 1
	}
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

type fitKey struct {
	name   string
	src    image.Rectangle
	dx, dy int
}

var fitCache sync.Map // fitKey -> *image.RGBA escalada

// sprFit desenha o recorte src do bitmap name em dst, escalando a imagem
// em Go quando necessário (o affine do Gio não escala de forma confiável
// em todos os contextos de desenho).
func (ui *uiState) sprFit(gtx layout.Context, name string, src, dst image.Rectangle) {
	img := ui.skin.Load().Spr(name, src)
	if img == nil || dst.Empty() {
		return
	}
	dx, dy := dst.Dx(), dst.Dy()
	if img.Bounds().Dx() == dx && img.Bounds().Dy() == dy {
		blit(gtx, img, dst)
		return
	}
	key := fitKey{name, src, dx, dy}
	if v, ok := fitCache.Load(key); ok {
		blit(gtx, v.(*image.RGBA), dst)
		return
	}
	out := image.NewRGBA(image.Rect(0, 0, dx, dy))
	xdraw.CatmullRom.Scale(out, out.Bounds(), img, img.Bounds(), xdraw.Over, nil)
	fitCache.Store(key, out)
	blit(gtx, out, dst)
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
		switch e.Kind {
		case pointer.Press:
			f.active = true
			fallthrough
		case pointer.Drag:
			t := (float32(e.Position.X) - float32(r.Min.X)) / float32(r.Dx())
			if t < 0 {
				t = 0
			}
			if t > 1 {
				t = 1
			}
			f.val = t
			if f.commit != nil && !f.releaseCommit {
				f.commit(t)
			}
		case pointer.Release:
			f.active = false
			if f.releaseCommit && f.commit != nil {
				f.commit(f.val)
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
		ui.sprFit(gtx, name, src, image.Rectangle{Max: dst.Size()})
	})
}

// spectrumDraw desenha o analisador de espectro. Sem variante usa o visual
// original (barras pretas + paleta VISCOLOR por altura); com variante
// (~/.local/share/sfrega/spectrum/<nome>/) aplica config.json + PNGs.
func (ui *uiState) spectrumDraw(gtx layout.Context, r image.Rectangle) {
	spec := ui.spec
	if spec != nil && spec.BgImg != nil {
		blit(gtx, spec.BgImg, r)
	}
	// sem fundo: a área fica transparente (mostra o que estiver atrás)
	if spec != nil && spec.Style == "scope" {
		ui.scopeDraw(gtx, r, spec)
		return
	}
	bands := ui.p.Spectrum()

	// agrega as 48 bandas do analisador em n barras (max por grupo)
	n := len(bands)
	if spec != nil && spec.Bands > 0 && spec.Bands < n {
		n = spec.Bands
	}
	var vals [spectrum.Bands]float32
	if n == len(bands) {
		copy(vals[:], bands[:])
	} else {
		for i := 0; i < n; i++ {
			lo, hi := i*len(bands)/n, (i+1)*len(bands)/n
			for k := lo; k < hi; k++ {
				if bands[k] > vals[i] {
					vals[i] = bands[k]
				}
			}
		}
	}

	gain := 1.0
	fall := float32(0.9)
	dir, style := "up", "bars"
	if spec != nil {
		gain = spec.Gain
		fall = float32(spec.Fall)
		dir, style = spec.Direction, spec.Style
	}

	h := float32(r.Dy())
	mid := float32(r.Min.Y) + h/2
	peakOn := spec != nil && spec.Peak

	type pt struct{ x, y float32 }
	linePts := make([]pt, 0, n)
	effFall := 1 - (1-fall)*0.5 // decaimento do pico: metade da taxa

	for i := 0; i < n; i++ {
		v := vals[i] * float32(gain)
		if v < 0 {
			v = 0
		}
		if v > 1 {
			v = 1
		}
		// pico com decaimento; a capa branca persegue com atraso
		if peakOn {
			if v > ui.specPeaks[i] {
				ui.specPeaks[i] = v
			} else {
				ui.specPeaks[i] *= effFall
			}
			// capa: sobe rápido, desce devagar (atraso visual)
			d := ui.specPeaks[i] - ui.specCap[i]
			if d > 0 {
				ui.specCap[i] += d * 0.3
			} else {
				ui.specCap[i] += d * 0.04
			}
		} else {
			ui.specPeaks[i] = 0
			ui.specCap[i] = 0
		}
		bh := v * h * 1.2
		if bh > h {
			bh = h
		}
		col := ui.specColor(spec, i, n, v)
		// distribui as barras pela largura toda (posições fracionárias)
		x0 := r.Min.X + i*r.Dx()/n
		x1 := r.Min.X + (i+1)*r.Dx()/n
		if x1 <= x0 {
			x1 = x0 + 1
		}
		// barra extrafina: largura fixa centrada no slot ("width" no config)
		if spec != nil && spec.BarW > 0 {
			cx := (x0 + x1) / 2
			x0 = cx - spec.BarW/2
			x1 = x0 + spec.BarW
		}
		// linha de intervalo transparente entre colunas de LEDs
		if spec != nil && spec.Segments > 0 && i < n-1 && x1 > x0+1 {
			x1--
		}
		if style == "line" {
			linePts = append(linePts, pt{float32(x0+x1) / 2, specLevelY(dir, mid, h, v)})
			continue
		}
		var bar image.Rectangle
		switch dir {
		case "down":
			bar = image.Rect(x0, r.Min.Y, x1, r.Min.Y+int(bh))
		case "center":
			y0 := int(mid - bh/2)
			bar = image.Rect(x0, y0, x1, y0+int(bh))
		default:
			bar = image.Rect(x0, r.Max.Y-int(bh), x1, r.Max.Y)
		}
		if spec != nil && spec.Segments > 0 {
			ui.specSegments(gtx, x0, x1, spec, r, dir, mid, h, v, col)
		} else if bar.Dy() > 0 {
			if spec != nil && spec.Palette == "fire" {
				ui.fillFireBar(gtx, bar, r, dir, mid, h)
			} else {
				fillRect(gtx.Ops, bar, col)
			}
		}
		// capa de pico branca, exatamente 1px, na posição atrasada
		if peakOn && ui.specCap[i] > v+0.01 {
			py := int(specLevelY(dir, mid, h, ui.specCap[i]))
			if py < r.Min.Y {
				py = r.Min.Y
			}
			if py > r.Max.Y-1 {
				py = r.Max.Y - 1
			}
			fillRect(gtx.Ops, image.Rect(x0, py, x1, py+1),
				color.NRGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF})
		}
	}

	// divisórias entre colunas de LEDs removidas: transparentes p/ não
	// cobrir o tema da skin

	if style == "line" && len(linePts) > 1 {
		path := clip.Path{}
		path.Begin(gtx.Ops)
		path.MoveTo(f32.Pt(linePts[0].x, linePts[0].y))
		for _, p := range linePts[1:] {
			path.LineTo(f32.Pt(p.x, p.y))
		}
		lc := ui.specColor(spec, n/2, n, 0.5)
		if spec != nil && spec.HasFG {
			lc = spec.FG
		} else if spec != nil && len(spec.Grad) > 0 {
			lc = spec.Grad[0]
		}
		paint.FillShape(gtx.Ops, lc, clip.Stroke{Path: path.End(), Width: 1}.Op())
	}
}

// scopeDraw desenha a forma de onda do áudio reproduzido (estilo osciloscópio).
// Renderiza min/max por coluna de pixels — cada faixa vertical mostra o
// envelope da onda, como num osciloscópio real (sem empilhar amostras).
func (ui *uiState) scopeDraw(gtx layout.Context, r image.Rectangle, spec *spectrum.Spec) {
	wave := ui.p.Wave()
	if len(wave) < 2 {
		return
	}
	mid := float32(r.Min.Y) + float32(r.Dy())/2
	amp := float32(r.Dy()) * 0.45
	lc := color.NRGBA{R: 0x30, G: 0xFF, B: 0x50, A: 0xFF}
	if spec.HasFG {
		lc = spec.FG
	} else if len(spec.Grad) > 0 {
		lc = spec.Grad[0]
	}
	n := len(wave)
	for cx := 0; cx < r.Dx(); cx++ {
		lo := cx * n / r.Dx()
		hi := (cx + 1) * n / r.Dx()
		if hi <= lo {
			hi = lo + 1
		}
		mn, mx := float32(1), float32(-1)
		for i := lo; i < hi && i < n; i++ {
			s := wave[i]
			if s > 1 {
				s = 1
			}
			if s < -1 {
				s = -1
			}
			if s < mn {
				mn = s
			}
			if s > mx {
				mx = s
			}
		}
		y0 := int(mid - mx*amp)
		y1 := int(mid - mn*amp)
		if y1 <= y0 {
			y1 = y0 + 1
		}
		fillRect(gtx.Ops, image.Rect(r.Min.X+cx, y0, r.Min.X+cx+1, y1), lc)
	}
}

// specSegments desenha LEDs em posições fixas no painel: cada um acende
// quando o nível o atinge (estilo VU). Apagados ficam transparentes.
// Com palette "vu"/"fire", a cor vem da posição fixa do LED no painel.
func (ui *uiState) specSegments(gtx layout.Context, x0, x1 int,
	spec *spectrum.Spec, r image.Rectangle, dir string, mid, h, v float32, col color.NRGBA) {
	n := spec.Segments
	// LEDs de altura igual: ledH px + vão de 1px; reduz n se não couber
	for n > 1 && (r.Dy()-(n-1))/n < 1 {
		n--
	}
	if n <= 0 {
		return
	}
	ledH := (r.Dy() - (n - 1)) / n
	if ledH < 1 {
		ledH = 1
	}
	totH := n*ledH + (n - 1)
	pad := (r.Dy() - totH) / 2 // sobra distribuída nas pontas
	disp := v * 1.2            // mesmo fator do bh das barras
	if disp > 1 {
		disp = 1
	}
	// gate: abaixo de ~60% da 1ª faixa zera — o LED da base não acende
	// com o piso de ruído do analisador (só com sinal de verdade)
	if disp < 0.6/float32(n) {
		disp = 0
	}
	for k := 0; k < n; k++ {
		lo := float32(k) / float32(n)
		hi := float32(k+1) / float32(n)
		// k=0 na base; "down" inverte (cresce do topo)
		var yTop, yBot int
		if dir == "down" {
			yTop = r.Min.Y + pad + k*(ledH+1)
			yBot = yTop + ledH
		} else {
			yBot = r.Max.Y - pad - k*(ledH+1)
			yTop = yBot - ledH
		}
		// LED apagado: sombra cinza clara se a variante pedir, senão transparente
		if disp <= lo {
			if spec.HasOffLED {
				fillRect(gtx.Ops, image.Rect(x0, yTop, x1, yBot), spec.OffLED)
			}
			continue
		}
		c := col
		t := (lo + hi) / 2
		if spec.Palette == "vu" {
			c = vuColor(t)
		} else if spec.Palette == "fire" {
			c = fireColor(t)
		}
		fillRect(gtx.Ops, image.Rect(x0, yTop, x1, yBot), c)
	}
}

// vuColor mapeia nível 0..1 para verde -> amarelo -> vermelho (LEDs de VU).
func vuColor(t float32) color.NRGBA {
	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}
	green := color.NRGBA{R: 0x18, G: 0xC8, B: 0x18, A: 0xFF}
	yellow := color.NRGBA{R: 0xE8, G: 0xC8, B: 0x10, A: 0xFF}
	red := color.NRGBA{R: 0xE0, G: 0x18, B: 0x18, A: 0xFF}
	var a, b color.NRGBA
	f := t
	if t < 0.5 {
		a, b, f = green, yellow, t*2
	} else {
		a, b, f = yellow, red, (t-0.5)*2
	}
	return color.NRGBA{
		R: uint8(float32(a.R) + (float32(b.R)-float32(a.R))*f),
		G: uint8(float32(a.G) + (float32(b.G)-float32(a.G))*f),
		B: uint8(float32(a.B) + (float32(b.B)-float32(a.B))*f),
		A: 0xFF,
	}
}

// fillFireBar desenha a barra em fatias de ~2px com cor de fogo pela
// posição no painel: base amarelo vivo -> meio laranja -> topo vermelho.
func (ui *uiState) fillFireBar(gtx layout.Context, bar image.Rectangle,
	r image.Rectangle, dir string, mid, h float32) {
	for y := bar.Min.Y; y < bar.Max.Y; y += 2 {
		y2 := y + 2
		if y2 > bar.Max.Y {
			y2 = bar.Max.Y
		}
		yc := (float32(y) + float32(y2)) / 2
		var t float32
		switch dir {
		case "down":
			t = (yc - float32(r.Min.Y)) / h
		case "center":
			t = (yc - mid) / (h / 2)
			if t < 0 {
				t = -t
			}
		default:
			t = (float32(r.Max.Y) - yc) / h
		}
		fillRect(gtx.Ops, image.Rect(bar.Min.X, y, bar.Max.X, y2), fireColor(t))
	}
}

// fireColor mapeia posição 0..1 no painel p/ a paleta de chama
// (base amarelo vivo -> meio laranja -> topo vermelho vivo).
func fireColor(t float32) color.NRGBA {
	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}
	stops := []struct {
		t float32
		c color.NRGBA
	}{
		{0.0, color.NRGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF}},
		{0.05, color.NRGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF}},
		{0.2, color.NRGBA{R: 0xFF, G: 0xDD, B: 0x00, A: 0xFF}},
		{0.4, color.NRGBA{R: 0xFF, G: 0x88, B: 0x00, A: 0xFF}},
		{1.0, color.NRGBA{R: 0xFF, G: 0x00, B: 0x00, A: 0xFF}},
	}
	for i := 1; i < len(stops); i++ {
		if t <= stops[i].t {
			f := (t - stops[i-1].t) / (stops[i].t - stops[i-1].t)
			a, b := stops[i-1].c, stops[i].c
			return color.NRGBA{
				R: uint8(float32(a.R) + (float32(b.R)-float32(a.R))*f),
				G: uint8(float32(a.G) + (float32(b.G)-float32(a.G))*f),
				B: uint8(float32(a.B) + (float32(b.B)-float32(a.B))*f),
				A: 0xFF,
			}
		}
	}
	return stops[len(stops)-1].c
}

// specLevelY devolve a posição vertical do nível v conforme a direção.
func specLevelY(dir string, mid, h, v float32) float32 {
	switch dir {
	case "down":
		return mid - h/2 + v*h*1.2
	case "center":
		return mid + (0.5-v)*h*0.9
	default:
		return mid + h/2 - v*h*1.2
	}
}

// specColor resolve a cor da barra i: fg sólido > gradiente > VISCOLOR.
func (ui *uiState) specColor(spec *spectrum.Spec, i, n int, v float32) color.NRGBA {
	if spec != nil {
		if spec.HasFG {
			return spec.FG
		}
		if spec.Palette == "vu" {
			return vuColor(v)
		}
		if spec.Palette == "fire" {
			return fireColor(v)
		}
		if len(spec.Grad) > 0 {
			t := float32(i) / float32(max(1, n-1))
			pos := t * float32(len(spec.Grad)-1)
			k := int(pos)
			if k >= len(spec.Grad)-1 {
				return spec.Grad[len(spec.Grad)-1]
			}
			f := pos - float32(k)
			a, b := spec.Grad[k], spec.Grad[k+1]
			return color.NRGBA{
				R: uint8(float32(a.R) + (float32(b.R)-float32(a.R))*f),
				G: uint8(float32(a.G) + (float32(b.G)-float32(a.G))*f),
				B: uint8(float32(a.B) + (float32(b.B)-float32(a.B))*f),
				A: 0xFF,
			}
		}
	}
	ci := int((1 - v) * 17)
	if ci < 0 {
		ci = 0
	}
	if ci > 17 {
		ci = 17
	}
	return ui.skin.Load().VisColor(ci)
}

// eject abre o diálogo nativo de arquivos; as faixas escolhidas são
// anexadas à playlist e a janela Playlist é exibida.
func (ui *uiState) eject() {
	go func() {
		files := ui.dialogWrap(pickAudioFiles)
		if len(files) == 0 {
			return
		}
		ui.plMain.addTracks(files)
		ui.mu.Lock()
		open := ui.plMain.plOpen
		ui.mu.Unlock()
		if !open {
			ui.plMain.toggle()
		}
	}()
}

// dialogWrap desativa o always-on-top das janelas sfrega durante um
// diálogo nativo (senão o zenity/kdialog abre atrás) e restaura depois.
func (ui *uiState) dialogWrap(fn func() []string) []string {
	if !ui.alwaysOnTop {
		return fn()
	}
	for _, n := range ontopWinNames {
		wmOntop(n, false)
	}
	time.Sleep(400 * time.Millisecond)
	defer func() {
		ui.mu.Lock()
		ui.ontopDone = map[string]bool{}
		ui.mu.Unlock()
		for _, n := range ontopWinNames {
			wmOntop(n, true)
		}
	}()
	return fn()
}
