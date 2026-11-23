// Package language implementa i18n simples baseado em arquivos .txt
// (formato chave=valor) embutidos no binário.
package language

import (
	"embed"
	"os"
	"path/filepath"
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

func loadSaved() string {
	b, err := os.ReadFile(configPath())
	if err != nil {
		return "en"
	}
	l := strings.TrimSpace(string(b))
	if _, err := fs.ReadFile(l + ".txt"); err != nil {
		return "en"
	}
	return l
}

// Set troca o idioma ativo e persiste a escolha.
func Set(l string) {
	mu.Lock()
	defer mu.Unlock()
	data, err := fs.ReadFile(l + ".txt")
	if err != nil {
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
	data, err := fs.ReadFile(l + ".txt")
	if err != nil {
		return l
	}
	for _, line := range strings.Split(string(data), "\n") {
		if k, v, ok := strings.Cut(line, "="); ok && strings.TrimSpace(k) == "lang.name" {
			return strings.TrimSpace(v)
		}
	}
	return l
}

// Available retorna os idiomas embutidos (sem a extensão).
func Available() []string {
	entries, _ := fs.ReadDir(".")
	var out []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".txt") {
			out = append(out, strings.TrimSuffix(e.Name(), ".txt"))
		}
	}
	return out
}
