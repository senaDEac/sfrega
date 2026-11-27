// sfrega — player de áudio com visual Winamp (spectrum analyzer, volume, etc).
package main

import (
	"bufio"
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"sfrega/internal/player"
	"sfrega/internal/spectrum"
	"sfrega/internal/ui/native"
	"sfrega/internal/ui/web"
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
	webMode := flag.Bool("web", false, "serve interface web em http://localhost:8080")
	addr := flag.String("addr", ":8080", "endereço do servidor web")
	flag.Parse()
	args := flag.Args()
	var tracks []string
	var musicDirs []string // dirs importados (scan inicial ou args) p/ Update lists
	if len(args) == 0 {
		// sem argumentos: restaura a playlist persistida; se o arquivo não
		// existe (primeira execução), cai no scan dos diretórios de música.
		// Um m3u existente porém vazio significa playlist propositalmente
		// limpa — não reescaneia.
		plPath := player.PlaylistFilePath()
		if _, err := os.Stat(plPath); err == nil {
			tracks = player.LoadPlaylist(plPath)
		} else {
			for _, d := range defaultMusicDirs() {
				musicDirs = append(musicDirs, d)
				tracks = append(tracks, collect(d)...)
			}
		}
	} else {
		for _, a := range args {
			a = fileURIToPath(a)
			if st, err := os.Stat(a); err == nil && st.IsDir() {
				musicDirs = append(musicDirs, a)
				tracks = append(tracks, collect(a)...)
			} else if err == nil {
				tracks = append(tracks, a)
			}
		}
	}
	if len(tracks) == 0 && !*webMode {
		// GUI sem faixas ainda é útil; segue com playlist vazia
	} else if len(tracks) == 0 {
		fmt.Println("nenhuma faixa encontrada")
		os.Exit(1)
	}

	p, err := player.New(44100, 2)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer p.Close()
	p.SetPlaylistFile(player.PlaylistFilePath())
	// stream.txt: a última coisa tocada foi uma rádio — retoma a estação
	// e ignora o resume de faixa (que é mais antigo).
	streamResumed := false
	if b, err := os.ReadFile(filepath.Join(filepath.Dir(player.PlaylistFilePath()), "stream.txt")); err == nil {
		if parts := strings.SplitN(strings.TrimSpace(string(b)), "\t", 2); len(parts) == 2 && parts[1] != "" {
			if err := p.PlayStream(parts[0], parts[1]); err == nil {
				streamResumed = true
			}
		}
	}
	if !streamResumed && len(tracks) > 0 {
		// lê resume.txt ANTES do SetTracks: o emit de track-change
		// sobrescreveria o arquivo com a faixa 0
		var last, fracStr string
		if b, err := os.ReadFile(filepath.Join(filepath.Dir(player.PlaylistFilePath()), "resume.txt")); err == nil {
			lines := strings.Split(strings.TrimSpace(string(b)), "\n")
			last = strings.TrimSpace(lines[0])
			if len(lines) > 1 {
				fracStr = strings.TrimSpace(lines[1])
			}
		}
		p.SetTracks(tracks)
		// retoma a última faixa tocada (+ posição), se ainda estiver na playlist
		found := false
		frac, _ := strconv.ParseFloat(fracStr, 64)
		for i, t := range tracks {
			if t == last {
				if err := p.PlayIndex(i); err != nil {
					fmt.Fprintln(os.Stderr, err)
					os.Exit(1)
				}
				if frac > 0 {
					// a duração é calculada em background; tenta o seek
					// até o cache ficar pronto (senão a posição se perde)
					go func() {
						for try := 0; try < 150; try++ {
							if p.Index() != i {
								return // usuário trocou de faixa
							}
							if p.SeekFraction(frac) == nil {
								return
							}
							time.Sleep(100 * time.Millisecond)
						}
					}()
				}
				found = true
				break
			}
		}
		if !found {
			if err := p.Play(); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
		}
	}

	if *webMode {
		srv := web.New(p)
		fmt.Printf("sfrega web → http://localhost%s (%d faixas)\n", *addr, len(tracks))
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

// defaultMusicDirs retorna diretórios de música comuns quando nenhum
// argumento é passado (execução pelo menu de apps).
func defaultMusicDirs() []string {
	home, _ := os.UserHomeDir()
	var out []string
	for _, d := range []string{"Music", "Música", "Downloads", "Baixados"} {
		p := filepath.Join(home, d)
		if st, err := os.Stat(p); err == nil && st.IsDir() {
			out = append(out, p)
		}
	}
	return out
}

// fileURIToPath converte "file:///caminho" (ou file://hostname) em path.
func fileURIToPath(a string) string {
	if !strings.HasPrefix(a, "file://") {
		return a
	}
	u := strings.TrimPrefix(a, "file://")
	if i := strings.Index(u, "/"); i > 0 {
		u = u[i:] // remove host
	}
	if dec, err := url.PathUnescape(u); err == nil {
		return dec
	}
	return u
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
