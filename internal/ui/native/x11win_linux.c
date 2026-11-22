#include "x11win_linux.h"

#include <X11/Xlib.h>
#include <pthread.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>
#include <sys/stat.h>

/* MotifWmHints layout lido pelo muffin (src/x11/xprops.h):
 * flags, functions, decorations, input_mode, status — 5 cardinais. */
static const unsigned long motif_hints[5] = { 2 /*MWM_HINTS_DECORATIONS*/, 0, 0, 0, 0 };

static const char *window_names[] = { "sfrega", "Equalizador", "Playlist" };

static Display *g_dpy;
static pthread_mutex_t g_mu = PTHREAD_MUTEX_INITIALIZER;

static Display *get_dpy(void)
{
    pthread_mutex_lock(&g_mu);
    if (!g_dpy)
        g_dpy = XOpenDisplay(NULL);
    pthread_mutex_unlock(&g_mu);
    return g_dpy;
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

/* Itera _NET_CLIENT_LIST chamando fn(window, name) para cada cliente. */
static void for_each_client(Display *dpy, Atom ncl, Atom nwn, Atom u8,
                            void (*fn)(Display *, Window, const char *, void *), void *arg)
{
    Window root = DefaultRootWindow(dpy);
    Atom actual;
    int fmt;
    unsigned long n = 0, after = 0;
    unsigned char *data = NULL;

    XGetWindowProperty(dpy, root, ncl, 0, 1024, False, AnyPropertyType,
                       &actual, &fmt, &n, &after, &data);
    if (!data)
        return;
    Window *clients = (Window *)data;
    for (unsigned long i = 0; i < n; i++) {
        char *name = window_name(dpy, clients[i], nwn, u8);
        if (name) {
            fn(dpy, clients[i], name, arg);
            free(name);
        }
    }
    XFree(data);
}

static int name_is_ours(const char *name)
{
    for (size_t i = 0; i < sizeof(window_names) / sizeof(window_names[0]); i++)
        if (strcmp(name, window_names[i]) == 0)
            return 1;
    return 0;
}

/* ---------- undecorate ---------- */

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

struct undec_ctx { Atom ncl, nwn, u8, pid, mwm, card; };

static void undec_one(Display *dpy, Window w, const char *name, void *arg)
{
    struct undec_ctx *c = arg;
    if (!name_is_ours(name) || already_done(w))
        return;

    unsigned long pid = (unsigned long)getpid();
    XChangeProperty(dpy, w, c->pid, c->card, 32, PropModeReplace,
                    (unsigned char *)&pid, 1);

    /* muffin recarrega _MOTIF_WM_HINTS no PropertyNotify e destrói
     * a frame ao vivo. Ao remover a frame ele pode redimensionar
     * o client — restauramos a geometria depois. */
    XWindowAttributes attr;
    int ow = 0, oh = 0;
    if (XGetWindowAttributes(dpy, w, &attr)) {
        ow = attr.width; oh = attr.height;
    }
    XChangeProperty(dpy, w, c->mwm, c->mwm, 32,
                    PropModeReplace, (unsigned char *)motif_hints, 5);
    XFlush(dpy);
    usleep(200000);
    if (ow > 0)
        XResizeWindow(dpy, w, ow, oh);
    XFlush(dpy);
}

/* ---------- restore de posição ---------- */

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

struct restore_ctx { int x, y; char name[64]; Window win; };

static void find_named(Display *dpy, Window w, const char *name, void *arg)
{
    struct restore_ctx *c = arg;
    if (c->win == None && strcmp(name, c->name) == 0)
        c->win = w;
}

static void restore_one_line(Display *dpy, Atom ncl, Atom nwn, Atom u8,
                             const char *name, int x, int y)
{
    struct restore_ctx c = { x, y, {0}, None };
    snprintf(c.name, sizeof(c.name), "%s", name);
    for_each_client(dpy, ncl, nwn, u8, find_named, &c);
    if (c.win == None || was_restored(c.win))
        return;
    XMoveWindow(dpy, c.win, x, y);
}

/* ---------- tick: undecorate + restore ---------- */

static void wm_tick(Display *dpy)
{
    Atom ncl = XInternAtom(dpy, "_NET_CLIENT_LIST", False);
    Atom nwn = XInternAtom(dpy, "_NET_WM_NAME", False);
    Atom u8 = XInternAtom(dpy, "UTF8_STRING", False);

    struct undec_ctx uc = {
        ncl, nwn, u8,
        XInternAtom(dpy, "_NET_WM_PID", False),
        XInternAtom(dpy, "_MOTIF_WM_HINTS", False),
        XInternAtom(dpy, "CARDINAL", False),
    };
    for_each_client(dpy, ncl, nwn, u8, undec_one, &uc);
    XFlush(dpy);

    /* restore de posição (arquivo pode ter sido atualizado por drag/close) */
    char home[256];
    const char *h = getenv("HOME");
    snprintf(home, sizeof(home), "%s/.config/sfrega/geometry.txt",
             h ? h : "/tmp");
    FILE *f = fopen(home, "r");
    if (!f)
        return;
    char name[64];
    int x, y;
    while (fscanf(f, "%63s %d %d", name, &x, &y) == 3)
        restore_one_line(dpy, ncl, nwn, u8, name, x, y);
    fclose(f);
    XFlush(dpy);
}

static void *wm_thread(void *arg)
{
    (void)arg;
    Display *dpy = get_dpy();
    if (!dpy)
        return NULL;
    Window root = DefaultRootWindow(dpy);
    XSelectInput(dpy, root, SubstructureNotifyMask);
    pthread_mutex_lock(&g_mu);
    wm_tick(dpy); /* janelas que já existiam antes da thread */
    pthread_mutex_unlock(&g_mu);
    for (;;) {
        XEvent ev;
        XNextEvent(dpy, &ev);
        if (ev.type == MapNotify || ev.type == CreateNotify) {
            pthread_mutex_lock(&g_mu);
            wm_tick(dpy);
            pthread_mutex_unlock(&g_mu);
        }
    }
    return NULL;
}

static pthread_once_t start_once = PTHREAD_ONCE_INIT;

static void start_thread(void)
{
    pthread_t th;
    pthread_create(&th, NULL, wm_thread, NULL);
    pthread_detach(th);
}

void sfrega_wm_start(void)
{
    XInitThreads();
    pthread_once(&start_once, start_thread);
}

void sfrega_wm_tick(void)
{
    Display *dpy = get_dpy();
    if (!dpy)
        return;
    pthread_mutex_lock(&g_mu);
    wm_tick(dpy);
    pthread_mutex_unlock(&g_mu);
}

/* ---------- geometria: salvar posições ---------- */

static void geo_path(char *buf, size_t len)
{
    const char *home = getenv("HOME");
    if (!home)
        home = "/tmp";
    snprintf(buf, len, "%s/.config/sfrega/geometry.txt", home);
}

static int root_pos(Display *dpy, Window w, int *x, int *y)
{
    Window junk;
    return XTranslateCoordinates(dpy, w, DefaultRootWindow(dpy), 0, 0, x, y, &junk);
}

struct geo_ctx { FILE *f; };

static void geo_one(Display *dpy, Window w, const char *name, void *arg)
{
    struct geo_ctx *c = arg;
    if (!name_is_ours(name))
        return;
    int x, y;
    if (root_pos(dpy, w, &x, &y))
        fprintf(c->f, "%s %d %d\n", name, x, y);
}

void sfrega_geo_save(void)
{
    Display *dpy = get_dpy();
    if (!dpy)
        return;

    Atom ncl = XInternAtom(dpy, "_NET_CLIENT_LIST", False);
    Atom nwn = XInternAtom(dpy, "_NET_WM_NAME", False);
    Atom u8 = XInternAtom(dpy, "UTF8_STRING", False);

    char path[512], cfg[512];
    const char *h = getenv("HOME");
    geo_path(path, sizeof(path));
    snprintf(cfg, sizeof(cfg), "%s/.config/sfrega", h ? h : "/tmp");
    mkdir(cfg, 0755);

    FILE *f = fopen(path, "w");
    if (!f)
        return;
    struct geo_ctx c = { f };
    for_each_client(dpy, ncl, nwn, u8, geo_one, &c);
    fclose(f);
}
