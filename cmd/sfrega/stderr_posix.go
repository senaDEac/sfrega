//go:build !windows

package main

import (
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

// dupStderrToFile redireciona fd 2 p/ o arquivo (panics do Go runtime
// escrevem direto no fd — necessário p/ depurar no Android).
func dupStderrToFile(path string) {
	_ = os.MkdirAll(filepath.Dir(path), 0755)
	if f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644); err == nil {
		_ = unix.Dup2(int(f.Fd()), 2)
	}
}
