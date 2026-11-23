#ifndef SFREGA_X11WIN_H
#define SFREGA_X11WIN_H

void sfrega_no_decorations(void);
void sfrega_geo_save(void);
void sfrega_geo_restore(void);
void sfrega_shade_toggle(int title_h);
void sfrega_shade_full(const char *name, int h);
int sfrega_shade_set(const char *name, int title_h, int full_h);

#endif
