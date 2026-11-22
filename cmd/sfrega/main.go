// sfrega — player de áudio com visual Winamp (spectrum analyzer, volume, etc).
package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"sfrega/internal/player"
	"sfrega/internal/spectrum"
)

var audioExts = map[string]bool{
	".mp3": true, ".wav": true, ".flac": true, ".ogg": true, ".m4a": true,
	".aac": true, ".wma": true, ".opus": true, ".aiff": true, ".ape": true,
}

func collect(dir string) []string {
	var out []string
	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if audioExts[strings.ToLower(filepath.Ext(path))] {
			out = append(out, path)
		}
		return nil
	})
	return out
}

func main() {
	args := os.Args[1:]
	if len(args) == 0 {
		fmt.Println("uso: sfrega <arquivo|diretorio> [...]")
		fmt.Println("controles: [espaço]=play/pause [n]=prox [p]=ant [q]=sair [+/-]=vol [m]=mute")
		os.Exit(1)
	}
	var tracks []string
	for _, a := range args {
		if st, err := os.Stat(a); err == nil && st.IsDir() {
			tracks = append(tracks, collect(a)...)
		} else {
			tracks = append(tracks, a)
		}
	}
	if len(tracks) == 0 {
		fmt.Println("nenhuma faixa encontrada")
		os.Exit(1)
	}

	p, err := player.New(44100, 2)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer p.Close()
	p.SetTracks(tracks)
	if err := p.Play(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	fmt.Printf("sfrega — %d faixas | %s\n", len(tracks), tracks[0])
	fmt.Println("controles: [espaço]=play/pause [n]=prox [p]=ant [q]=sair [+/-]=vol [m]=mute")

	go handleKeys(p)
	renderSpectrum(p)
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
