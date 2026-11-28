// sfrega — player de áudio com visual Winamp (spectrum analyzer, volume, etc).
package main

import (
	"bufio"
	"flag"
	"fmt"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"

	"sfrega/internal/launch"
	"sfrega/internal/player"
	"sfrega/internal/spectrum"
	"sfrega/internal/ui/native"
	"sfrega/internal/ui/web"
)

func main() {
	// Android: stderr não vai p/ logcat — redireciona p/ arquivo p/ debug
	if runtime.GOOS == "android" {
		dupStderrToFile(os.Getenv("HOME") + "/stderr.txt")
	}
	webMode := flag.Bool("web", false, "serve interface web em http://localhost:8080")
	addr := flag.String("addr", ":8080", "endereço do servidor web")
	flag.Parse()
	args := flag.Args()

	p, musicDirs, err := launch.Prepare(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer p.Close()
	if len(p.Tracks()) == 0 && *webMode {
		fmt.Println("nenhuma faixa encontrada")
		os.Exit(1)
	}

	if *webMode {
		srv := web.New(p)
		fmt.Printf("sfrega web → http://localhost%s (%d faixas)\n", *addr, len(p.Tracks()))
		if err := http.ListenAndServe(*addr, srv.Handler()); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}

	// modo nativo (padrão): janela Winamp via Gio
	fmt.Println("sfrega — interface nativa (feche a janela para sair)")
	if err := native.Run(p, musicDirs); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	return
}

func handleKeys(p *player.Player) {
	sc := bufio.NewScanner(os.Stdin)
	sc.Split(bufio.ScanRunes)
	for sc.Scan() {
		for _, r := range sc.Text() {
			switch r {
			case ' ':
				p.Toggle()
			case 'n', 'N':
				p.Next()
			case 'p', 'P':
				p.Prev()
			case 'q', 'Q':
				p.Close()
				os.Exit(0)
			case '+', '=':
				p.SetVolume(p.Volume() + 0.05)
			case '-', '_':
				p.SetVolume(p.Volume() - 0.05)
			case 'm', 'M':
				if p.Volume() > 0 {
					p.SetVolume(0)
				} else {
					p.SetVolume(1)
				}
			}
		}
	}
}

func renderSpectrum(p *player.Player) {
	const cols = 60
	bars := make([]string, cols)
	for i := range bars {
		bars[i] = " "
	}
	for {
		bands := p.Spectrum()
		// mapeia 48 bandas -> cols barras (pico)
		for c := 0; c < cols; c++ {
			b0 := c * spectrum.Bands / cols
			b1 := (c + 1) * spectrum.Bands / cols
			if b1 <= b0 {
				b1 = b0 + 1
			}
			var peak float32
			for b := b0; b < b1 && b < spectrum.Bands; b++ {
				if bands[b] > peak {
					peak = bands[b]
				}
			}
			lvl := int(peak * 10)
			if lvl > 9 {
				lvl = 9
			}
			bars[c] = strings.Repeat("#", lvl+1)
		}
		pos := p.Position()
		dur := p.Duration()
		dec, buf, played := p.Stats()
		fmt.Printf("\r\x1b[K%s | %s / %s | vol %.0f%% | d:%d b:%d p:%d",
			strings.Join(bars, ""),
			pos.Truncate(time.Second), dur.Truncate(time.Second),
			p.Volume()*100, dec, buf, played)
		time.Sleep(50 * time.Millisecond)
	}
}
