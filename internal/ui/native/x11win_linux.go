package native

/*
#cgo linux CFLAGS: -I${SRCDIR}
#cgo linux LDFLAGS: -lX11
#include "x11win_linux.h"
#include <stdlib.h>
*/
import "C"

import (
	"sync"
	"unsafe"
)

// Serializa chamadas X11 vindas das goroutines das 3 janelas.
var wmMu sync.Mutex

func wmNoDecorations() {
	wmMu.Lock()
	defer wmMu.Unlock()
	C.sfrega_no_decorations()
}

func wmSaveGeometry() {
	wmMu.Lock()
	defer wmMu.Unlock()
	C.sfrega_geo_save()
}

func wmRestoreGeometry() {
	wmMu.Lock()
	defer wmMu.Unlock()
	C.sfrega_geo_restore()
}

func wmShadeToggle(titleH int) {
	wmMu.Lock()
	defer wmMu.Unlock()
	C.sfrega_shade_toggle(C.int(titleH))
}

func wmShadeFull(name string, h int) {
	wmMu.Lock()
	defer wmMu.Unlock()
	cs := C.CString(name)
	defer C.free(unsafe.Pointer(cs))
	C.sfrega_shade_full(cs, C.int(h))
}
