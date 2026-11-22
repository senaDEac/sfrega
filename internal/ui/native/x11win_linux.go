package native

/*
#cgo linux CFLAGS: -I${SRCDIR}
#cgo linux LDFLAGS: -lX11 -lpthread
#include "x11win_linux.h"
*/
import "C"

func wmStart()        { C.sfrega_wm_start() }
func wmTick()         { C.sfrega_wm_tick() }
func wmSaveGeometry() { C.sfrega_geo_save() }
