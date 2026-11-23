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

/* O handler de erro padrão do Xlib chama exit(1) — fatal para nós,
 * pois consultamos janelas que podem estar sendo destruídas
 * (BadWindow em _NET_CLIENT_LIST). Ignoramos erros não-fatais. */
static int x_error_handler(Display *d, XErrorEvent *e)
{
    (void)d;
    (void)e;
    return 0;
}

static Display *open_display(void)
{
    Display *d = XOpenDisplay(NULL);
    if (d)
        XSetErrorHandler(x_error_handler);
    return d;
}

/* MotifWmHints layout lido pelo muffin (src/x11/xprops.h):
 * flags, functions, decorations, input_mode, status — 5 cardinais. */
static const unsigned long motif_hints[5] = { 2 /*MWM_HINTS_DECORATIONS*/, 0, 0, 0, 0 };

static const char *window_names[] = { "sfrega", "Equalizador", "Playlist" };

/* janelas que recebem undecorate (inclui Menu, que não tem geometria salva) */
static const char *ui_names[] = { "sfrega", "Equalizador", "Playlist", "Menu" };

static Window find_by_name(Display *dpy, const char *want, Atom ncl, Atom nwn, Atom u8);

/* Janela "nossa" já está sem frame quando o parent é o root
 * (muffin reparenta em frame só quando decorada). Stateless —
 * imune a reuso de XID. */
static int has_frame(Display *dpy, Window w)
{
    Window root, parent;
    Window *children = NULL;
    unsigned int nc = 0;
    int framed = 1;
    if (XQueryTree(dpy, w, &root, &parent, &children, &nc)) {
        framed = (parent != root && parent != None);
    }
    if (children)
        XFree(children);
    return framed;
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
        g_dpy = open_display();
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
        for (size_t j = 0; j < sizeof(ui_names) / sizeof(ui_names[0]); j++)
            if (strcmp(name, ui_names[j]) == 0)
                match = 1;
        free(name);
        if (!match || !has_frame(dpy, w))
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

/* ---------- winshade (duplo-clique na titlebar) ---------- */

struct shade_ent { char name[32]; int full_h; int shaded; };
static struct shade_ent shades[8];
static int shades_n;

static Window find_client_win(Display *dpy)
{
    Window root, child;
    int rx, ry, wx, wy;
    unsigned int mask;
    if (!XQueryPointer(dpy, DefaultRootWindow(dpy), &root, &child,
                       &rx, &ry, &wx, &wy, &mask) || child == None)
        return None;
    Atom wm_state = XInternAtom(dpy, "WM_STATE", False);
    for (int depth = 0; depth < 8; depth++) {
        Atom actual;
        int fmt;
        unsigned long n, after;
        unsigned char *prop = NULL;
        if (XGetWindowProperty(dpy, child, wm_state, 0, 1, False,
                               wm_state, &actual, &fmt, &n, &after,
                               &prop) == Success && prop != NULL) {
            XFree(prop);
            return child;
        }
        Window jr, jp;
        Window *children = NULL;
        unsigned int nc = 0;
        if (!XQueryTree(dpy, child, &jr, &jp, &children, &nc) || nc == 0)
            return child;
        Window next = children[0];
        XFree(children);
        child = next;
    }
    return child;
}

void sfrega_shade_toggle(int title_h)
{
    if (!g_dpy)
        g_dpy = open_display();
    if (!g_dpy)
        return;
    Display *dpy = g_dpy;

    Window w = find_client_win(dpy);
    if (w == None)
        return;

    XWindowAttributes attr;
    if (!XGetWindowAttributes(dpy, w, &attr))
        return;

    /* chave por nome de janela: imune a reuso de XID (EQ/PL são recriadas) */
    Atom nwn = XInternAtom(dpy, "_NET_WM_NAME", False);
    Atom u8 = XInternAtom(dpy, "UTF8_STRING", False);
    char *name = window_name(dpy, w, nwn, u8);
    if (!name)
        name = window_name(dpy, w, XInternAtom(dpy, "WM_NAME", False), u8);
    if (!name)
        return;

    int idx = -1;
    for (int i = 0; i < shades_n; i++)
        if (strcmp(shades[i].name, name) == 0)
            idx = i;

    if (idx >= 0) {
        if (shades[idx].shaded) {
            XResizeWindow(dpy, w, attr.width, shades[idx].full_h);
            shades[idx].shaded = 0;
        } else {
            shades[idx].full_h = attr.height;
            XResizeWindow(dpy, w, attr.width, title_h);
            shades[idx].shaded = 1;
        }
    } else if (attr.height > title_h * 2 && shades_n < 8) {
        snprintf(shades[shades_n].name, sizeof(shades[shades_n].name), "%s", name);
        shades[shades_n].full_h = attr.height;
        shades[shades_n].shaded = 1;
        shades_n++;
        XResizeWindow(dpy, w, attr.width, title_h);
    }
    free(name);
    XFlush(dpy);
}

/* Aplica shade numa janela pelo nome (usado no startup p/ restaurar o
   estado shade persistido — não depende da posição do ponteiro).
   Retorna 1 quando a janela foi encontrada/encontrada já na altura. */
int sfrega_shade_set(const char *name, int title_h, int full_h)
{
    if (!g_dpy)
        g_dpy = open_display();
    if (!g_dpy)
        return 0;
    Display *dpy = g_dpy;

    Atom ncl = XInternAtom(dpy, "_NET_CLIENT_LIST", False);
    Atom nwn = XInternAtom(dpy, "_NET_WM_NAME", False);
    Atom u8 = XInternAtom(dpy, "UTF8_STRING", False);
    Window w = find_by_name(dpy, name, ncl, nwn, u8);
    if (w == None)
        return 0;

    XWindowAttributes attr;
    if (!XGetWindowAttributes(dpy, w, &attr))
        return 0;
    if (attr.height <= title_h * 2) {
        /* já está com altura de shade — marca estado p/ unshade futuro */
        for (int i = 0; i < shades_n; i++)
            if (strcmp(shades[i].name, name) == 0)
                shades[i].shaded = 1;
        return 1;
    }

    int idx = -1;
    for (int i = 0; i < shades_n; i++)
        if (strcmp(shades[i].name, name) == 0)
            idx = i;
    if (idx < 0) {
        if (shades_n >= 8)
            return 0;
        idx = shades_n++;
        snprintf(shades[idx].name, sizeof(shades[idx].name), "%s", name);
    }
    shades[idx].full_h = full_h;
    shades[idx].shaded = 1;
    /* Gio pode reverter o resize inicial — o caller repete até confirmar. */
    XResizeWindow(dpy, w, attr.width, title_h);
    XFlush(dpy);
    return 0;
}

/* Centraliza uma janela nossa pelo nome na tela. Retorna 1 se encontrou. */
int sfrega_center(const char *name)
{
    if (!g_dpy)
        g_dpy = open_display();
    if (!g_dpy)
        return 0;
    Display *dpy = g_dpy;
    Atom ncl = XInternAtom(dpy, "_NET_CLIENT_LIST", False);
    Atom nwn = XInternAtom(dpy, "_NET_WM_NAME", False);
    Atom u8 = XInternAtom(dpy, "UTF8_STRING", False);
    Window win = find_by_name(dpy, name, ncl, nwn, u8);
    if (win == None)
        return 0;
    XWindowAttributes attr;
    if (!XGetWindowAttributes(dpy, win, &attr))
        return 0;
    int x = (DisplayWidth(dpy, DefaultScreen(dpy)) - attr.width) / 2;
    int y = (DisplayHeight(dpy, DefaultScreen(dpy)) - attr.height) / 2;
    XMoveWindow(dpy, win, x, y);
    XFlush(dpy);
    return 1;
}

/* Redimensiona uma janela nossa pelo nome (usado p/ esticar a janela
   shade quando o menu suspenso está aberto). */
void sfrega_resize(const char *name, int w, int h)
{
    if (!g_dpy)
        g_dpy = open_display();
    if (!g_dpy)
        return;
    Display *dpy = g_dpy;
    Atom ncl = XInternAtom(dpy, "_NET_CLIENT_LIST", False);
    Atom nwn = XInternAtom(dpy, "_NET_WM_NAME", False);
    Atom u8 = XInternAtom(dpy, "UTF8_STRING", False);
    Window win = find_by_name(dpy, name, ncl, nwn, u8);
    if (win == None)
        return;
    XResizeWindow(dpy, win, w, h);
    XFlush(dpy);
}

/* Alterna _NET_WM_STATE_ABOVE (always on top) via client message EWMH. */
void sfrega_ontop(const char *name, int on)
{
    if (!g_dpy)
        g_dpy = open_display();
    if (!g_dpy)
        return;
    Display *dpy = g_dpy;
    Atom ncl = XInternAtom(dpy, "_NET_CLIENT_LIST", False);
    Atom nwn = XInternAtom(dpy, "_NET_WM_NAME", False);
    Atom u8 = XInternAtom(dpy, "UTF8_STRING", False);
    Window win = find_by_name(dpy, name, ncl, nwn, u8);
    if (win == None)
        return;

    Atom state = XInternAtom(dpy, "_NET_WM_STATE", False);
    Atom above = XInternAtom(dpy, "_NET_WM_STATE_ABOVE", False);
    XEvent ev;
    memset(&ev, 0, sizeof(ev));
    ev.xclient.type = ClientMessage;
    ev.xclient.window = win;
    ev.xclient.message_type = state;
    ev.xclient.format = 32;
    ev.xclient.data.l[0] = on ? 1 : 0; /* _NET_WM_STATE_ADD : REMOVE */
    ev.xclient.data.l[1] = above;
    ev.xclient.data.l[2] = 0;
    XSendEvent(dpy, DefaultRootWindow(dpy), False,
               SubstructureRedirectMask | SubstructureNotifyMask, &ev);
    XFlush(dpy);
}

/* Atualiza a altura "cheia" guardada de uma janela com shade (ex.: após
   toggle de escala 1x/2x, para o unshade restaurar a altura correta). */
void sfrega_shade_full(const char *name, int h)
{
    for (int i = 0; i < shades_n; i++)
        if (strcmp(shades[i].name, name) == 0) {
            shades[i].full_h = h;
            return;
        }
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
        g_dpy = open_display();
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

    /* preserva posições de janelas hoje fechadas */
    char old_name[3][64];
    int old_x[3], old_y[3], nold = 0;
    FILE *of = fopen(path, "r");
    if (of) {
        char on[64];
        int ox, oy;
        while (nold < 3 && fscanf(of, "%63s %d %d", on, &ox, &oy) == 3) {
            snprintf(old_name[nold], 64, "%s", on);
            old_x[nold] = ox;
            old_y[nold] = oy;
            nold++;
        }
        fclose(of);
    }

    FILE *f = fopen(path, "w");
    if (!f)
        return;
    for (size_t i = 0; i < sizeof(window_names) / sizeof(window_names[0]); i++) {
        Window w = find_by_name(dpy, window_names[i], ncl, nwn, u8);
        if (w == None)
            continue;
        int x, y;
        if (root_pos(dpy, w, &x, &y)) {
            fprintf(f, "%s %d %d\n", window_names[i], x, y);
            for (int j = 0; j < nold; j++)
                if (strcmp(old_name[j], window_names[i]) == 0)
                    old_name[j][0] = 0;
        }
    }
    for (int i = 0; i < nold; i++)
        if (old_name[i][0])
            fprintf(f, "%s %d %d\n", old_name[i], old_x[i], old_y[i]);
    fclose(f);
}

/* Marca de "posição já restaurada" gravada na própria janela —
 * imune a reuso de XID (instância nova não tem a propriedade). */
static int prop_exists(Display *dpy, Window w, Atom prop)
{
    Atom actual;
    int fmt;
    unsigned long n, after;
    unsigned char *data = NULL;
    int found = 0;
    if (XGetWindowProperty(dpy, w, prop, 0, 1, False, AnyPropertyType,
                           &actual, &fmt, &n, &after, &data) == Success && data) {
        found = 1;
        XFree(data);
    }
    return found;
}

void sfrega_geo_restore(void)
{
    if (!g_dpy)
        g_dpy = open_display();
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
    Atom done_prop = XInternAtom(dpy, "_SFREGA_POS_DONE", False);
    Atom cardinal = XInternAtom(dpy, "CARDINAL", False);

    char name[64];
    int x, y;
    while (fscanf(f, "%63s %d %d", name, &x, &y) == 3) {
        Window w = find_by_name(dpy, name, ncl, nwn, u8);
        if (w == None || prop_exists(dpy, w, done_prop))
            continue;
        XMoveWindow(dpy, w, x, y);
        unsigned long one = 1;
        XChangeProperty(dpy, w, done_prop, cardinal, 32, PropModeReplace,
                        (unsigned char *)&one, 1);
    }
    fclose(f);
    XFlush(dpy);
}
