#include "x11drag_linux.h"

#include <X11/Xlib.h>

/* Arrasto manual via XMoveWindow (ConfigureRequest). _NET_WM_MOVERESIZE
   não funciona porque o ButtonPress cria grab implícito na conexão do
   Gio, que não podemos liberar — o WM não consegue grabar o pointer. */

static Display *g_dpy;
static Window g_win;
static int g_px, g_py; /* pointer root no início */
static int g_ox, g_oy; /* posição root do client no início */
static int g_ex, g_ey; /* frame extents (left, top) a compensar */

static Window find_client(Display *dpy, Window w) {
	Atom wm_state = XInternAtom(dpy, "WM_STATE", False);
	for (int depth = 0; depth < 8; depth++) {
		Atom actual;
		int fmt;
		unsigned long n, after;
		unsigned char *prop = NULL;
		if (XGetWindowProperty(dpy, w, wm_state, 0, 1, False,
				       wm_state, &actual, &fmt, &n, &after,
				       &prop) == Success && prop != NULL) {
			XFree(prop);
			return w;
		}
		Window jr, jp;
		Window *children = NULL;
		unsigned int nc = 0;
		if (!XQueryTree(dpy, w, &jr, &jp, &children, &nc) || nc == 0) {
			return w;
		}
		Window next = children[0];
		XFree(children);
		w = next;
	}
	return w;
}

static int pointer_pos(int *rx, int *ry, unsigned int *mask) {
	Window root, child;
	int wx, wy;
	if (!g_dpy ||
	    !XQueryPointer(g_dpy, DefaultRootWindow(g_dpy), &root, &child,
			   rx, ry, &wx, &wy, mask) ||
	    child == None) {
		return 0;
	}
	return 1;
}

void sfrega_drag_begin(void) {
	if (g_dpy) {
		int rx, ry;
		unsigned int mask;
		if (pointer_pos(&rx, &ry, &mask) && (mask & Button1Mask)) {
			return; /* re-grab sintético: manter origem */
		}
		XCloseDisplay(g_dpy);
		g_dpy = NULL;
	}
	g_dpy = XOpenDisplay(NULL);
	if (!g_dpy) {
		return;
	}
	Window root = DefaultRootWindow(g_dpy);
	Window child = None;
	int rx, ry, wx, wy;
	unsigned int mask;
	if (!XQueryPointer(g_dpy, root, &root, &child, &rx, &ry, &wx, &wy,
			   &mask) ||
	    child == None) {
		XCloseDisplay(g_dpy);
		g_dpy = NULL;
		return;
	}
	g_win = find_client(g_dpy, child);
	Window junk;
	int cx = 0, cy = 0;
	if (!XTranslateCoordinates(g_dpy, g_win, root, 0, 0, &cx, &cy,
				   &junk)) {
		cx = rx - wx;
		cy = ry - wy;
	}
	g_px = rx;
	g_py = ry;
	g_ox = cx;
	g_oy = cy;

	/* muffin posiciona o ConfigureRequest como se x,y fossem do frame
	   (incluindo _NET_FRAME_EXTENTS); compensa left/top. */
	g_ex = 0;
	g_ey = 0;
	Atom fe = XInternAtom(g_dpy, "_NET_FRAME_EXTENTS", False);
	Atom actual;
	int fmt;
	unsigned long n = 0, after = 0;
	unsigned long *prop = NULL;
	Atom card = XInternAtom(g_dpy, "CARDINAL", False);
	if (XGetWindowProperty(g_dpy, g_win, fe, 0, 4, False, card,
			       &actual, &fmt, &n, &after, (unsigned char **)&prop) ==
	    Success && prop != NULL && n >= 4) {
		g_ex = (int)prop[0];
		g_ey = (int)prop[2];
		XFree(prop);
	}
}

void sfrega_drag_move(void) {
	if (!g_dpy) {
		return;
	}
	int rx, ry;
	unsigned int mask;
	if (!pointer_pos(&rx, &ry, &mask)) {
		return;
	}
	if (!(mask & Button1Mask)) {
		sfrega_drag_end();
		return;
	}
	int nx = g_ox + (rx - g_px);
	int ny = g_oy + (ry - g_py);
	XMoveWindow(g_dpy, g_win, nx-g_ex, ny-g_ey);
	XFlush(g_dpy);
}

void sfrega_drag_end(void) {
	if (g_dpy) {
		XCloseDisplay(g_dpy);
		g_dpy = NULL;
	}
}
