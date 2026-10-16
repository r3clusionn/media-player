/*
 * appshell: a standalone window for a web UI, without a browser or Electron.
 *
 * CEF (Chromium as a library) renders the page off-screen into GPU buffers;
 * an X11 window with an EGL/OpenGL context imports them and blends them over
 * video that libmpv draws underneath. The page reaches native code through
 * the shim in appshell/helper (window.electron, window.mpvPrism).
 *
 * Everything runs on the process's main thread (CEF's UI thread), except
 * mpv, which renders on its own threads into an off-screen child window that
 * the main thread draws under the page (player.c). The wakeup callbacks (CEF
 * pump scheduling, mpv events) just write to an eventfd.
 */
#ifndef APPSHELL_H
#define APPSHELL_H

#define CEF_API_VERSION 15400

#include "include/capi/cef_app_capi.h"
#include "include/capi/cef_browser_capi.h"
#include "include/capi/cef_client_capi.h"

#include <EGL/egl.h>
#include <GLES3/gl3.h>
#include <X11/Xlib.h>
#include <stddef.h>
#include <stdint.h>

typedef struct appshell_opts {
	const char *url;           /* page to open */
	const char *title;         /* initial window title */
	const char *wm_class;      /* X11 class, for dwm rules */
	const char *cache_dir;     /* CEF profile and cache */
	const char *data_dir;      /* settings, screenshots, logs */
	const char *helper_path;   /* appshell-helper */
	const char *resources_dir; /* CEF .pak files, icudtl.dat */
	const char *locales_dir;
	const char *anime4k_dir;   /* shaders offered to the player */
	int server_port;           /* reported to the page as the local server */
	int width, height;         /* initial size in pixels */
	double scale;              /* device scale factor (2 on this laptop) */
} appshell_opts;

/* Runs until the window is closed. Returns the process exit status. */
int appshell_run(const appshell_opts *o);
/* Thread-safe: close the window and return from appshell_run. */
void appshell_request_quit(void);

/* ---- shared state (shell.c) ------------------------------------------- */

extern const appshell_opts *as_opts;
extern cef_browser_t *as_browser; /* NULL until created */
extern int as_wake_fd;
void as_wake(void);
void as_request_redraw(void);
void as_quit(void);
double as_now_ms(void);
/* Copied at startup (see shell.c: Chromium reuses the environment's memory). */
extern char as_home[4096], as_runtime_dir[4096];
void as_page_send_raw(const char *json); /* window.__appshellRecv(json) */
void as_page_run_js(const char *code);   /* in the main frame */
extern int as_debug; /* APPSHELL_DEBUG */
#define AS_LOG(...) do { if (as_debug) fprintf(stderr, "appshell: " __VA_ARGS__); } while (0)

/* ---- window.c --------------------------------------------------------- */

extern Display *as_dpy;
extern Window as_win;
extern EGLDisplay as_egl_dpy;
extern int as_win_w, as_win_h; /* pixels */
extern int as_visible;         /* mapped, not fully obscured, display on */
extern int as_fullscreen;
extern int as_depth;    /* bits per channel of the window: 8, or 10 at X depth 30 */
extern double as_scale; /* device scale factor: opts->scale, else Xft.dpi / 96 */
/* HDR: `colormode watch` puts the screen under the window into HDR (PQ) mode
 * while the window shows HDR video fullscreen, and says so on the window. */
extern int as_hdr;          /* the screen is in HDR mode: draw PQ */
extern double as_hdr_white; /* its SDR white in nits (the page is drawn at it) */
extern double as_hdr_peak;  /* the panel's peak in nits */
extern int as_hdr_p3;       /* Display P3 primaries rather than BT.2020 */
extern unsigned as_hdr_gen; /* bumped whenever the above change */

int window_init(void);
void window_handle_events(void);
void window_set_title(const char *utf8);
void window_set_fullscreen(int on);
void window_set_cursor(int cef_cursor_type);
void window_minimize(void);
void window_set_hdr_video(int on); /* the page shows HDR video */
void window_inhibit_screensaver(int on);
int window_display_on(void);
int window_swap(void); /* returns 0 if skipped (not visible) */
void window_sync_surface_size(void);
void window_destroy(void);

/* ---- compositor.c ----------------------------------------------------- */

int compositor_init(void);
/* Copies a CEF dmabuf (valid only during the paint callback). */
void compositor_page_from_dmabuf(int popup, const void *paint_info);
void compositor_page_from_pixels(int popup, const void *bgra, int w, int h);
void compositor_set_popup(int show, int x, int y, int w, int h); /* DIP */
/* Draws video rects (via player_draw_all), then the page, then the popup. */
void compositor_draw(void);
/* An offscreen RGBA8 copy of a page layer, reused between frames. */
typedef struct as_target {
	GLuint fbo, tex;
	int w, h;
} as_target;
void compositor_target_size(as_target *t, int w, int h);
void compositor_blit_texture(GLuint tex, int x, int y, int w, int h); /* pixels, top-left origin */
void compositor_destroy(void);

/* ---- player.c --------------------------------------------------------- */

int player_handle_request(const char *ch, void *args /* yyjson_val* */, void *reply /* reply_t* */);
void player_process_events(void); /* mpv events -> page */
void player_tick(void);           /* video windows follow their presenters */
int player_handle_xevent(void *xevent /* XEvent* */); /* 1: a video frame (Damage) */
void player_draw_all(void);       /* the videos, into the current framebuffer */
int player_any_playing(void);
void player_destroy_all(void);

/* ---- stats.c (timing counters for tests) ---------------------------- */

void stats_loop_tick(void);  /* the loop runs */
void stats_loop_idle(void);  /* the loop is about to sleep */
void stats_page_painted(void);
void stats_swapped(void);
void stats_video(int shown, int skipped);
void *stats_take(void *doc /* yyjson_mut_doc* */); /* yyjson_mut_val*, resets */

/* ---- bridge.c --------------------------------------------------------- */

void bridge_handle_message(const char *json); /* from __appshellPost */
void bridge_send_event(const char *channel, const char *args_json /* JSON array or NULL */);
void bridge_paste_text(const char *text, size_t len); /* as a paste event + insertText */
int bridge_logging_enabled(void);
void bridge_log_path(char *out, size_t n);

#endif
