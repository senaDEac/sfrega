package native

/*
#cgo linux CFLAGS: -I${SRCDIR}
#cgo linux LDFLAGS: -lX11
#include "x11win_linux.h"
*/
import "C"

func wmNoDecorations() { C.sfrega_no_decorations() }
