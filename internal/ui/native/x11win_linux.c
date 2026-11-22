#include "x11win_linux.h"

#include <X11/Xlib.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>
#include <sys/stat.h>

/* Xlib não é thread-safe por padrão; janelas/EQ/PL chamam estas
 * funções de goroutines Go distintas. */
__attribute__((constructor)) static void x11_threads_init(void)
{
    XInitThreads();
}

/* MotifWmHints layout lido pelo muffin (src/x11/xprops.h):
 * flags, functions, decorations, input_mode, status — 5 cardinais. */
static const unsigned long motif_hints[5] = { 2 /*MWM_HINTS_DECORATIONS*/, 0, 0, 0, 0 };

static const char *window_names[] = { "sfrega", "Equalizador", "Playlist" };

static Window done[16];
static int done_n;

static int already_done(Window w)
{
    for (int i = 0; i < done_n; i++)
        if (done[i] == w)
            return 1;
    if (done_n < 16)
        done[done_n++] = w;
    return 0;
}

static char *window_name(Display *dpy, Window w, Atom name_atom, Atom utf8_string)
{
    Atom actual;
    int fmt;
    unsigned long n, after;
    unsigned char *data = NULL;
    if (XGetWindowProperty(dpy, w, name_atom, 0, 1024, False, utf8_string,
                           &actual, &fmt, &n, &after, &data) != Success || !data) {
        data = NULL;
        if (XGetWindowProperty(dpy, w, name_atom, 0, 1024, False,
                               XInternAtom(dpy, "STRING", False),
                               &actual, &fmt, &n, &after, &data) != Success || !data)
            return NULL;
    }
    char *s = strndup((char *)data, n);
    XFree(data);
    return s;
}

static Display *g_dpy;

void sfrega_no_decorations(void)
{
    if (!g_dpy) {
        g_dpy = XOpenDisplay(NULL);
        if (!g_dpy)
            return;
    }
    Display *dpy = g_dpy;

    Atom net_client_list = XInternAtom(dpy, "_NET_CLIENT_LIST", False);
    Atom net_wm_name = XInternAtom(dpy, "_NET_WM_NAME", False);
    Atom utf8_string = XInternAtom(dpy, "UTF8_STRING", False);
    Atom net_wm_pid = XInternAtom(dpy, "_NET_WM_PID", False);
    Atom motif_hints_atom = XInternAtom(dpy, "_MOTIF_WM_HINTS", False);
    Atom cardinal = XInternAtom(dpy, "CARDINAL", False);

    Window root = DefaultRootWindow(dpy);
    Window *clients = NULL;
    int nclients = 0;

    Atom actual;
    int fmt;
    unsigned long n, after;
    XGetWindowProperty(dpy, root, net_client_list, 0, 1024, False, AnyPropertyType,
                       &actual, &fmt, &n, &after, (unsigned char **)&clients);
    if (!clients)
        goto out;
    nclients = (int)n;

    for (int i = 0; i < nclients; i++) {
        Window w = clients[i];
        char *name = window_name(dpy, w, net_wm_name, utf8_string);
        /* fallback WM_NAME */
        if (!name || !*name) {
            free(name);
            Atom wm_name = XInternAtom(dpy, "WM_NAME", False);
            name = window_name(dpy, w, wm_name, utf8_string);
        }
        if (getenv("SFREGA_X11WIN_DEBUG"))
            fprintf(stderr, "x11win: win 0x%lx name='%s'\n", (unsigned long)w, name ? name : "(null)");
        if (!name)
            continue;

        int match = 0;
        for (size_t j = 0; j < sizeof(window_names) / sizeof(window_names[0]); j++)
            if (strcmp(name, window_names[j]) == 0)
                match = 1;
        free(name);
        if (!match || already_done(w))
            continue;

        /* Garante _NET_WM_PID para identificação futura. */
        unsigned long pid = (unsigned long)getpid();
        XChangeProperty(dpy, w, net_wm_pid, cardinal, 32, PropModeReplace,
                        (unsigned char *)&pid, 1);

        /* muffin recarrega _MOTIF_WM_HINTS no PropertyNotify e destrói
         * a frame ao vivo (sem withdraw). Ao remover a frame ele pode
         * redimensionar o client — restauramos a geometria depois. */
        XWindowAttributes attr;
        int ow = 0, oh = 0;
        if (XGetWindowAttributes(dpy, w, &attr)) {
            ow = attr.width; oh = attr.height;
        }
        XChangeProperty(dpy, w, motif_hints_atom, motif_hints_atom, 32,
                        PropModeReplace, (unsigned char *)motif_hints, 5);
        XFlush(dpy);
        usleep(200000);
        if (ow > 0)
            XResizeWindow(dpy, w, ow, oh);
        XFlush(dpy);
    }

out:
    if (clients)
        XFree(clients);
}

/* ---------- geometria (posição das janelas) ---------- */

static void geo_path(char *buf, size_t len)
{
    const char *home = getenv("HOME");
    if (!home)
        home = "/tmp";
    snprintf(buf, len, "%s/.config/sfrega/geometry.txt", home);
}

static Window find_by_name(Display *dpy, const char *want, Atom ncl, Atom nwn, Atom u8)
{
    Window root = DefaultRootWindow(dpy);
    Atom actual;
    int fmt;
    unsigned long n = 0, after = 0;
    unsigned char *data = NULL;
    Window *clients = NULL;
    Window found = None;

    XGetWindowProperty(dpy, root, ncl, 0, 1024, False, AnyPropertyType,
                       &actual, &fmt, &n, &after, &data);
    if (!data)
        return None;
    clients = (Window *)data;

    for (unsigned long i = 0; i < n && found == None; i++) {
        char *name = window_name(dpy, clients[i], nwn, u8);
        if (name && strcmp(name, want) == 0)
            found = clients[i];
        free(name);
    }
    XFree(data);
    return found;
}

static int root_pos(Display *dpy, Window w, int *x, int *y)
{
    Window junk;
    return XTranslateCoordinates(dpy, w, DefaultRootWindow(dpy), 0, 0, x, y, &junk);
}

void sfrega_geo_save(void)
{
    if (!g_dpy)
        g_dpy = XOpenDisplay(NULL);
    if (!g_dpy)
        return;
    Display *dpy = g_dpy;

    Atom ncl = XInternAtom(dpy, "_NET_CLIENT_LIST", False);
    Atom nwn = XInternAtom(dpy, "_NET_WM_NAME", False);
    Atom u8 = XInternAtom(dpy, "UTF8_STRING", False);

    char path[512], cfg[512];
    geo_path(path, sizeof(path));
    snprintf(cfg, sizeof(cfg), "%s/.config/sfrega", getenv("HOME") ? getenv("HOME") : "/tmp");
    mkdir(cfg, 0755);

    FILE *f = fopen(path, "w");
    if (!f)
        return;
    for (size_t i = 0; i < sizeof(window_names) / sizeof(window_names[0]); i++) {
        Window w = find_by_name(dpy, window_names[i], ncl, nwn, u8);
        if (w == None)
            continue;
        int x, y;
        if (root_pos(dpy, w, &x, &y))
            fprintf(f, "%s %d %d\n", window_names[i], x, y);
    }
    fclose(f);
}

static Window restored[16];
static int restored_n;

static int was_restored(Window w)
{
    for (int i = 0; i < restored_n; i++)
        if (restored[i] == w)
            return 1;
    if (restored_n < 16)
        restored[restored_n++] = w;
    return 0;
}

void sfrega_geo_restore(void)
{
    if (!g_dpy)
        g_dpy = XOpenDisplay(NULL);
    if (!g_dpy)
        return;
    Display *dpy = g_dpy;

    char path[512];
    geo_path(path, sizeof(path));
    FILE *f = fopen(path, "r");
    if (!f)
        return;

    Atom ncl = XInternAtom(dpy, "_NET_CLIENT_LIST", False);
    Atom nwn = XInternAtom(dpy, "_NET_WM_NAME", False);
    Atom u8 = XInternAtom(dpy, "UTF8_STRING", False);

    char name[64];
    int x, y;
    while (fscanf(f, "%63s %d %d", name, &x, &y) == 3) {
        Window w = find_by_name(dpy, name, ncl, nwn, u8);
        if (w == None || was_restored(w))
            continue;
        XMoveWindow(dpy, w, x, y);
    }
    fclose(f);
    XFlush(dpy);
}
