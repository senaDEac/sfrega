//go:build linux

package native

/*
#cgo LDFLAGS: -lX11
#include "x11drag_linux.h"
*/
import "C"

import "os"

// wmDrag* implementam o arrasto da janela no X11 via XMoveWindow.
// Necessário porque o Gio não implementa ActionMove no backend X11 e o
// grab implícito do ButtonPress impede _NET_WM_MOVERESIZE. No-op em
// sessão Wayland.
func wmDragBegin() {
	if os.Getenv("WAYLAND_DISPLAY") != "" {
		return
	}
	C.sfrega_drag_begin()
}

func wmDragMove() {
	if os.Getenv("WAYLAND_DISPLAY") != "" {
		return
	}
	C.sfrega_drag_move()
}

func wmDragEnd() {
	if os.Getenv("WAYLAND_DISPLAY") != "" {
		return
	}
	C.sfrega_drag_end()
}
