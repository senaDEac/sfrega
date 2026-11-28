// Package launch prepara o player e a playlist inicial (compartilhado
// entre o binário desktop e o entrypoint Android/gomobile).
package launch

import (
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"sfrega/internal/player"
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

// Prepare monta a lista de faixas (args ou playlist/diretórios padrão),
// cria o player e aplica a retomada (stream.txt / resume.txt).
// Retorna o player, os diretórios de música importados e erro fatal.
func Prepare(args []string) (*player.Player, []string, error) {
	var tracks []string
	var musicDirs []string
	if len(args) == 0 {
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

	p, err := player.New(44100, 2)
	if err != nil {
		return nil, nil, err
	}
	p.SetPlaylistFile(player.PlaylistFilePath())
	streamResumed := false
	if b, err := os.ReadFile(filepath.Join(filepath.Dir(player.PlaylistFilePath()), "stream.txt")); err == nil {
		if parts := strings.SplitN(strings.TrimSpace(string(b)), "\t", 2); len(parts) == 2 && parts[1] != "" {
			if err := p.PlayStream(parts[0], parts[1]); err == nil {
				streamResumed = true
			}
		}
	}
	if !streamResumed && len(tracks) > 0 {
		var last, fracStr string
		if b, err := os.ReadFile(filepath.Join(filepath.Dir(player.PlaylistFilePath()), "resume.txt")); err == nil {
			lines := strings.Split(strings.TrimSpace(string(b)), "\n")
			last = strings.TrimSpace(lines[0])
			if len(lines) > 1 {
				fracStr = strings.TrimSpace(lines[1])
			}
		}
		p.SetTracks(tracks)
		found := false
		frac, _ := strconv.ParseFloat(fracStr, 64)
		for i, t := range tracks {
			if t == last {
				// falha de decode não é fatal: segue sem retomar
				_ = p.PlayIndex(i)
				if frac > 0 {
					go func() {
						for try := 0; try < 150; try++ {
							if p.Index() != i {
								return
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
			_ = p.Play()
		}
	}
	return p, musicDirs, nil
}

func defaultMusicDirs() []string {
	if runtime.GOOS == "android" {
		var out []string
		seen := map[string]bool{}
		add := func(d string) {
			if !seen[d] {
				if st, err := os.Stat(d); err == nil && st.IsDir() {
					seen[d] = true
					out = append(out, d)
				}
			}
		}
		for _, d := range []string{"/sdcard/Music", "/sdcard/Download", "/sdcard/Recordings"} {
			add(d)
		}
		if entries, err := os.ReadDir("/sdcard"); err == nil {
			for _, en := range entries {
				if !en.IsDir() || en.Name() == "Android" || en.Name() == "DCIM" {
					continue
				}
				d := "/sdcard/" + en.Name()
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

func fileURIToPath(a string) string {
	if !strings.HasPrefix(a, "file://") {
		return a
	}
	u := strings.TrimPrefix(a, "file://")
	if i := strings.Index(u, "/"); i > 0 {
		u = u[i:]
	}
	if dec, err := url.PathUnescape(u); err == nil {
		return dec
	}
	return u
}
