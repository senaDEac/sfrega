// Package language implementa i18n simples baseado em arquivos .txt
// (formato chave=valor) embutidos no binário.
package language

import (
	"embed"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

//go:embed *.txt
var fs embed.FS

var (
	mu      sync.RWMutex
	current = "en"
	table   = map[string]string{}
)

// init carrega o idioma persistido (ou en) na inicialização do pacote.
func init() {
	Set(loadSaved())
}

func configPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "/tmp"
	}
	return filepath.Join(home, ".config", "sfrega", "lang.txt")
}

// langDirs retorna os diretórios de idiomas em runtime (sem rebuild).
func langDirs() []string {
	home, _ := os.UserHomeDir()
	var dirs []string
	if x := os.Getenv("XDG_DATA_HOME"); x != "" {
		dirs = append(dirs,
			filepath.Join(x, "sfrega", "lang"),
			filepath.Join(x, "sfrega", "language"))
	}
	dirs = append(dirs,
		filepath.Join(home, ".local", "share", "sfrega", "lang"),
		filepath.Join(home, ".local", "share", "sfrega", "language"),
		"internal/language",
		"language",
		"lang",
	)
	return dirs
}

// readLang lê o arquivo de idioma: primeiro nos diretórios de runtime,
// depois no embed.
func readLang(l string) ([]byte, bool) {
	name := l + ".txt"
	for _, d := range langDirs() {
		if b, err := os.ReadFile(filepath.Join(d, name)); err == nil {
			return b, true
		}
	}
	if b, err := fs.ReadFile(name); err == nil {
		return b, true
	}
	return nil, false
}

func loadSaved() string {
	b, err := os.ReadFile(configPath())
	if err != nil {
		return "en"
	}
	l := strings.TrimSpace(string(b))
	if _, ok := readLang(l); !ok {
		return "en"
	}
	return l
}

// Set troca o idioma ativo e persiste a escolha.
func Set(l string) {
	mu.Lock()
	defer mu.Unlock()
	data, ok := readLang(l)
	if !ok {
		return
	}
	m := map[string]string{}
	last := ""
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if k, v, ok := strings.Cut(line, "="); ok {
			k = strings.TrimSpace(k)
			m[k] = strings.TrimSpace(v)
			last = k
		} else if last != "" {
			m[last] += "\n" + line
		}
	}
	current, table = l, m
	_ = os.MkdirAll(filepath.Dir(configPath()), 0o755)
	_ = os.WriteFile(configPath(), []byte(l+"\n"), 0o644)
}

// Current retorna o código do idioma ativo (ex.: "en", "pt-br").
func Current() string {
	mu.RLock()
	defer mu.RUnlock()
	return current
}

// T traduz a chave; retorna a própria chave se não houver tradução.
func T(key string) string {
	mu.RLock()
	defer mu.RUnlock()
	if v, ok := table[key]; ok {
		return v
	}
	return key
}

// Name retorna o nome de exibição do idioma (chave lang.name do arquivo);
// cai para o código se não houver.
func Name(l string) string {
	data, ok := readLang(l)
	if !ok {
		return l
	}
	for _, line := range strings.Split(string(data), "\n") {
		if k, v, ok := strings.Cut(line, "="); ok && strings.TrimSpace(k) == "lang.name" {
			return strings.TrimSpace(v)
		}
	}
	return l
}

// Available retorna os idiomas disponíveis (diretórios de runtime + embed).
func Available() []string {
	seen := map[string]bool{}
	var out []string
	for _, d := range langDirs() {
		ents, err := os.ReadDir(d)
		if err != nil {
			continue
		}
		for _, e := range ents {
			if strings.HasSuffix(e.Name(), ".txt") {
				seen[strings.TrimSuffix(e.Name(), ".txt")] = true
			}
		}
	}
	if ents, err := fs.ReadDir("."); err == nil {
		for _, e := range ents {
			if strings.HasSuffix(e.Name(), ".txt") {
				seen[strings.TrimSuffix(e.Name(), ".txt")] = true
			}
		}
	}
	for l := range seen {
		out = append(out, l)
	}
	sort.Strings(out)
	return out
}
