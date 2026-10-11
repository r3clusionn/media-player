/*
 * Phase 0 spike: CEF off-screen rendering into GPU buffers (dmabuf) on X11,
 * imported into our own EGL/GLES context and blended over an "underlay"
 * (a stand-in for the video), the way the finished appshell will work.
 *
 * Usage: SPIKE_URL=... [SPIKE_SHOT=file.ppm SPIKE_SHOT_FRAMES=n] [SPIKE_SECONDS=s]
 *        [SPIKE_SOFTWARE=1] [SPIKE_SWITCHES=a,b=c] osr_spike
 * (CEF treats positional arguments as start URLs, so options are in the environment.)
 *
 * Prints how frames arrive (accelerated or software), the buffer layout,
 * time to first frame, and the cost of importing and drawing each frame.
 * Reference counting is a no-op: every object here lives for the whole run.
 */

#define CEF_API_VERSION 15400

#include "include/capi/cef_app_capi.h"
#include "include/capi/cef_browser_capi.h"
#include "include/capi/cef_browser_process_handler_capi.h"
#include "include/capi/cef_client_capi.h"
#include "include/capi/cef_life_span_handler_capi.h"
#include "include/capi/cef_load_handler_capi.h"
#include "include/capi/cef_render_handler_capi.h"
#include "include/cef_api_hash.h"

#include <EGL/egl.h>
#include <EGL/eglext.h>
#include <GLES3/gl3.h>
#include <GLES2/gl2ext.h>
#include <X11/Xlib.h>
#include <X11/Xutil.h>
#include <drm_fourcc.h>

#include <errno.h>
#include <poll.h>
#include <stdatomic.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/eventfd.h>
#include <time.h>
#include <unistd.h>

#define SCALE 2.0f

static Display *xdpy;
static Window xwin;
static Atom wm_delete;
static EGLDisplay edpy;
static EGLSurface esurf;
static EGLContext ectx;
static int win_w = 1440, win_h = 900; /* pixels; dwm resizes the window */

static PFNEGLCREATEIMAGEKHRPROC create_image;
static PFNEGLDESTROYIMAGEKHRPROC destroy_image;
static PFNGLEGLIMAGETARGETTEXTURE2DOESPROC image_target_texture;

static GLuint prog, page_tex;
static int have_page;

static cef_browser_t *browser;
static atomic_int browser_closed;
static atomic_llong pump_due_ms = 0; /* 0 = run now */
static int wake_fd;

static double t_start;
static long frames_accel, frames_soft, n_view_rect, n_screen_info;
static double import_draw_total_ms, t_import, t_draw, t_swap;
static long shot_after_frames;
static const char *shot_path;

static double now_ms(void)
{
	struct timespec ts;
	clock_gettime(CLOCK_MONOTONIC, &ts);
	return ts.tv_sec * 1e3 + ts.tv_nsec / 1e6;
}

/* ---- no-op reference counting ------------------------------------------ */

static void CEF_CALLBACK ref_add(cef_base_ref_counted_t *s) { (void)s; }
static int CEF_CALLBACK ref_release(cef_base_ref_counted_t *s) { (void)s; return 0; }
static int CEF_CALLBACK ref_true(cef_base_ref_counted_t *s) { (void)s; return 1; }

static void init_base(cef_base_ref_counted_t *b, size_t size)
{
	b->size = size;
	b->add_ref = ref_add;
	b->release = ref_release;
	b->has_one_ref = ref_true;
	b->has_at_least_one_ref = ref_true;
}

static void release_browser(cef_browser_t *b)
{
	if (b && b != browser && !getenv("SPIKE_NORELEASE"))
		b->base.release(&b->base);
}

static cef_string_t str(const char *s)
{
	cef_string_t out = {0};
	cef_string_utf8_to_utf16(s, strlen(s), &out);
	return out;
}

/* ---- GL --------------------------------------------------------------- */

static const char *vs_src =
	"#version 300 es\n"
	"out vec2 uv;\n"
	"void main() {\n"
	"  vec2 p = vec2(float(gl_VertexID & 1), float(gl_VertexID >> 1));\n"
	"  uv = vec2(p.x, 1.0 - p.y);\n"
	"  gl_Position = vec4(p * 2.0 - 1.0, 0.0, 1.0);\n"
	"}\n";

/* mode 0: underlay gradient (stands in for video); mode 1: the page */
static const char *fs_src =
	"#version 300 es\n"
	"precision mediump float;\n"
	"in vec2 uv; out vec4 color;\n"
	"uniform sampler2D tex; uniform int mode;\n"
	"void main() {\n"
	"  if (mode == 0) color = vec4(uv.x * 0.6, 0.2, uv.y * 0.8, 1.0);\n"
	"  else color = texture(tex, uv);\n"
	"}\n";

static GLuint compile(GLenum type, const char *src)
{
	GLuint s = glCreateShader(type);
	glShaderSource(s, 1, &src, NULL);
	glCompileShader(s);
	GLint ok;
	glGetShaderiv(s, GL_COMPILE_STATUS, &ok);
	if (!ok) {
		char log[1024];
		glGetShaderInfoLog(s, sizeof log, NULL, log);
		fprintf(stderr, "shader: %s\n", log);
		exit(1);
	}
	return s;
}

static void gl_init(void)
{
	prog = glCreateProgram();
	glAttachShader(prog, compile(GL_VERTEX_SHADER, vs_src));
	glAttachShader(prog, compile(GL_FRAGMENT_SHADER, fs_src));
	glLinkProgram(prog);
	glUseProgram(prog);
	GLuint vao;
	glGenVertexArrays(1, &vao);
	glBindVertexArray(vao);
	glGenTextures(1, &page_tex);
	glBindTexture(GL_TEXTURE_2D, page_tex);
	glTexParameteri(GL_TEXTURE_2D, GL_TEXTURE_MIN_FILTER, GL_NEAREST);
	glTexParameteri(GL_TEXTURE_2D, GL_TEXTURE_MAG_FILTER, GL_NEAREST);
}

static void set_swizzle(GLint r, GLint g, GLint b, GLint a)
{
	glTexParameteri(GL_TEXTURE_2D, GL_TEXTURE_SWIZZLE_R, r);
	glTexParameteri(GL_TEXTURE_2D, GL_TEXTURE_SWIZZLE_G, g);
	glTexParameteri(GL_TEXTURE_2D, GL_TEXTURE_SWIZZLE_B, b);
	glTexParameteri(GL_TEXTURE_2D, GL_TEXTURE_SWIZZLE_A, a);
}

static void write_shot(void)
{
	unsigned char *px = malloc((size_t)win_w * win_h * 4);
	glReadPixels(0, 0, win_w, win_h, GL_RGBA, GL_UNSIGNED_BYTE, px);
	FILE *f = fopen(shot_path, "wb");
	fprintf(f, "P6\n%d %d\n255\n", win_w, win_h);
	for (int y = win_h - 1; y >= 0; y--)
		for (int x = 0; x < win_w; x++)
			fwrite(px + ((size_t)y * win_w + x) * 4, 1, 3, f);
	fclose(f);
	free(px);
	fprintf(stderr, "spike: screenshot written to %s\n", shot_path);
}

/* Draw the underlay, then the page blended on top (premultiplied alpha). */
static void present(void)
{
	glViewport(0, 0, win_w, win_h);
	glDisable(GL_BLEND);
	glUniform1i(glGetUniformLocation(prog, "mode"), 0);
	glDrawArrays(GL_TRIANGLE_STRIP, 0, 4);
	if (have_page) {
		glEnable(GL_BLEND);
		glBlendFunc(GL_ONE, GL_ONE_MINUS_SRC_ALPHA);
		glUniform1i(glGetUniformLocation(prog, "mode"), 1);
		glDrawArrays(GL_TRIANGLE_STRIP, 0, 4);
	}
	double td = now_ms();
	glFinish();
	t_draw += now_ms() - td;
	long total = frames_accel + frames_soft;
	if (shot_path && shot_after_frames && total == shot_after_frames)
		write_shot();
	double ts = now_ms();
	eglSwapBuffers(edpy, esurf);
	t_swap += now_ms() - ts;
}

/* ---- render handler ----------------------------------------------------- */

static void CEF_CALLBACK get_view_rect(cef_render_handler_t *self,
				       cef_browser_t *b, cef_rect_t *rect)
{
	(void)self;
	n_view_rect++;
	rect->x = rect->y = 0;
	rect->width = (int)(win_w / SCALE);
	rect->height = (int)(win_h / SCALE);
	release_browser(b);
}

static int CEF_CALLBACK get_screen_info(cef_render_handler_t *self,
					cef_browser_t *b, cef_screen_info_t *si)
{
	(void)self;
	n_screen_info++;
	si->device_scale_factor = SCALE;
	si->depth = 24;
	si->depth_per_component = 8;
	si->rect = (cef_rect_t){0, 0, (int)(2880 / SCALE), (int)(1800 / SCALE)};
	si->available_rect = si->rect;
	release_browser(b);
	return 1;
}

static void CEF_CALLBACK on_paint(cef_render_handler_t *self, cef_browser_t *b,
				  cef_paint_element_type_t type, size_t ndirty,
				  cef_rect_t const *dirty, const void *buffer,
				  int width, int height)
{
	(void)self; (void)ndirty; (void)dirty;
	release_browser(b);
	if (type != PET_VIEW)
		return;
	if (frames_soft++ == 0)
		fprintf(stderr, "spike: SOFTWARE frame %dx%d at %.0f ms (no GPU buffer)\n",
			width, height, now_ms() - t_start);
	double t0 = now_ms();
	glBindTexture(GL_TEXTURE_2D, page_tex);
	/* CEF gives BGRA; GLES has no BGRA upload in core, swizzle on upload. */
	glTexImage2D(GL_TEXTURE_2D, 0, GL_RGBA, width, height, 0, GL_RGBA,
		     GL_UNSIGNED_BYTE, buffer);
	set_swizzle(GL_BLUE, GL_GREEN, GL_RED, GL_ALPHA);
	have_page = 1;
	present();
	import_draw_total_ms += now_ms() - t0;
}

static void CEF_CALLBACK on_accelerated_paint(cef_render_handler_t *self,
					      cef_browser_t *b,
					      cef_paint_element_type_t type,
					      size_t ndirty, cef_rect_t const *dirty,
					      const cef_accelerated_paint_info_t *info)
{
	(void)self; (void)ndirty; (void)dirty;
	release_browser(b);
	if (type != PET_VIEW)
		return;
	int w = info->extra.coded_size.width, h = info->extra.coded_size.height;
	uint32_t fourcc = info->format == CEF_COLOR_TYPE_BGRA_8888 ?
		DRM_FORMAT_ARGB8888 : DRM_FORMAT_ABGR8888;
	if (frames_accel++ == 0)
		fprintf(stderr,
			"spike: GPU frame %dx%d at %.0f ms: planes=%d format=%s "
			"modifier=0x%llx stride=%u\n",
			w, h, now_ms() - t_start, info->plane_count,
			info->format == CEF_COLOR_TYPE_BGRA_8888 ? "BGRA" : "RGBA",
			(unsigned long long)info->modifier, info->planes[0].stride);

	double t0 = now_ms();
	EGLint a[64];
	int n = 0;
	a[n++] = EGL_WIDTH; a[n++] = w;
	a[n++] = EGL_HEIGHT; a[n++] = h;
	a[n++] = EGL_LINUX_DRM_FOURCC_EXT; a[n++] = (EGLint)fourcc;
	static const EGLint fd_k[] = {EGL_DMA_BUF_PLANE0_FD_EXT, EGL_DMA_BUF_PLANE1_FD_EXT,
				      EGL_DMA_BUF_PLANE2_FD_EXT, EGL_DMA_BUF_PLANE3_FD_EXT};
	static const EGLint off_k[] = {EGL_DMA_BUF_PLANE0_OFFSET_EXT, EGL_DMA_BUF_PLANE1_OFFSET_EXT,
				       EGL_DMA_BUF_PLANE2_OFFSET_EXT, EGL_DMA_BUF_PLANE3_OFFSET_EXT};
	static const EGLint pitch_k[] = {EGL_DMA_BUF_PLANE0_PITCH_EXT, EGL_DMA_BUF_PLANE1_PITCH_EXT,
					 EGL_DMA_BUF_PLANE2_PITCH_EXT, EGL_DMA_BUF_PLANE3_PITCH_EXT};
	static const EGLint modlo_k[] = {EGL_DMA_BUF_PLANE0_MODIFIER_LO_EXT, EGL_DMA_BUF_PLANE1_MODIFIER_LO_EXT,
					 EGL_DMA_BUF_PLANE2_MODIFIER_LO_EXT, EGL_DMA_BUF_PLANE3_MODIFIER_LO_EXT};
	static const EGLint modhi_k[] = {EGL_DMA_BUF_PLANE0_MODIFIER_HI_EXT, EGL_DMA_BUF_PLANE1_MODIFIER_HI_EXT,
					 EGL_DMA_BUF_PLANE2_MODIFIER_HI_EXT, EGL_DMA_BUF_PLANE3_MODIFIER_HI_EXT};
	for (int i = 0; i < info->plane_count && i < 4; i++) {
		a[n++] = fd_k[i]; a[n++] = info->planes[i].fd;
		a[n++] = off_k[i]; a[n++] = (EGLint)info->planes[i].offset;
		a[n++] = pitch_k[i]; a[n++] = (EGLint)info->planes[i].stride;
		if (info->modifier != DRM_FORMAT_MOD_INVALID) {
			a[n++] = modlo_k[i]; a[n++] = (EGLint)(info->modifier & 0xffffffff);
			a[n++] = modhi_k[i]; a[n++] = (EGLint)(info->modifier >> 32);
		}
	}
	a[n++] = EGL_NONE;

	EGLImageKHR img = create_image(edpy, EGL_NO_CONTEXT, EGL_LINUX_DMA_BUF_EXT, NULL, a);
	if (img == EGL_NO_IMAGE_KHR) {
		fprintf(stderr, "spike: eglCreateImage failed: 0x%x\n", eglGetError());
		return;
	}
	glBindTexture(GL_TEXTURE_2D, page_tex);
	image_target_texture(GL_TEXTURE_2D, img);
	t_import += now_ms() - t0;
	set_swizzle(GL_RED, GL_GREEN, GL_BLUE, GL_ALPHA);
	have_page = 1;
	present(); /* the buffer is only valid during this callback */
	destroy_image(edpy, img);
	import_draw_total_ms += now_ms() - t0;
}

static cef_render_handler_t render_handler;

/* ---- life span, client, app -------------------------------------------- */

static void CEF_CALLBACK on_after_created(cef_life_span_handler_t *self, cef_browser_t *b)
{
	(void)self;
	browser = b; /* keep this reference */
	fprintf(stderr, "spike: browser created at %.0f ms\n", now_ms() - t_start);
}

static int CEF_CALLBACK do_close(cef_life_span_handler_t *self, cef_browser_t *b)
{
	(void)self;
	release_browser(b);
	return 0;
}

static void CEF_CALLBACK on_before_close(cef_life_span_handler_t *self, cef_browser_t *b)
{
	(void)self; (void)b;
	atomic_store(&browser_closed, 1);
}

static cef_life_span_handler_t life_span;

static cef_render_handler_t *CEF_CALLBACK get_render_handler(cef_client_t *self)
{
	(void)self;
	return &render_handler;
}

static cef_life_span_handler_t *CEF_CALLBACK get_life_span_handler(cef_client_t *self)
{
	(void)self;
	return &life_span;
}

static void CEF_CALLBACK on_loading_state_change(cef_load_handler_t *self, cef_browser_t *b,
						   int loading, int back, int fwd)
{
	(void)self; (void)back; (void)fwd;
	release_browser(b);
	fprintf(stderr, "spike: loading=%d at %.0f ms\n", loading, now_ms() - t_start);
}

static void CEF_CALLBACK on_load_error(cef_load_handler_t *self, cef_browser_t *b,
				       cef_frame_t *frame, cef_errorcode_t code,
				       const cef_string_t *text, const cef_string_t *url)
{
	(void)self; (void)text; (void)url;
	release_browser(b);
	if (frame)
		frame->base.release(&frame->base);
	fprintf(stderr, "spike: load error %d\n", code);
}

static cef_load_handler_t load_handler;

static cef_load_handler_t *CEF_CALLBACK get_load_handler(cef_client_t *self)
{
	(void)self;
	return &load_handler;
}

static cef_client_t client;

static void append_switch(cef_command_line_t *cl, const char *name, const char *value)
{
	cef_string_t n = str(name);
	if (value) {
		cef_string_t v = str(value);
		cl->append_switch_with_value(cl, &n, &v);
		cef_string_clear(&v);
	} else {
		cl->append_switch(cl, &n);
	}
	cef_string_clear(&n);
}

static void CEF_CALLBACK on_before_command_line_processing(cef_app_t *self,
							   const cef_string_t *process_type,
							   cef_command_line_t *cl)
{
	(void)self;
	if (process_type && process_type->length)
		return; /* browser process only; switches are passed on to children */
	if (getenv("SPIKE_NOSWITCHES"))
		return;
	append_switch(cl, "no-zygote", NULL);
	append_switch(cl, "renderer-process-limit", "1");
	append_switch(cl, "disable-site-isolation-trials", NULL);
	append_switch(cl, "disable-background-networking", NULL);
	append_switch(cl, "disable-component-update", NULL);
	append_switch(cl, "disable-extensions", NULL);
	append_switch(cl, "disable-spell-checking", NULL);
	append_switch(cl, "no-pings", NULL);
	append_switch(cl, "enable-gpu-rasterization", NULL);
	append_switch(cl, "enable-zero-copy", NULL);
	append_switch(cl, "ignore-gpu-blocklist", NULL);
	const char *extra = getenv("SPIKE_SWITCHES"); /* "a,b=c" for experiments */
	if (extra) {
		char *copy = strdup(extra), *save = NULL;
		for (char *t = strtok_r(copy, ",", &save); t; t = strtok_r(NULL, ",", &save)) {
			char *eq = strchr(t, '=');
			if (eq)
				*eq++ = 0;
			append_switch(cl, t, eq);
		}
		free(copy);
	}
}

static void CEF_CALLBACK on_schedule_message_pump_work(cef_browser_process_handler_t *self,
						       int64_t delay_ms)
{
	(void)self;
	atomic_store(&pump_due_ms, (long long)(now_ms() + (delay_ms > 0 ? delay_ms : 0)));
	uint64_t one = 1;
	ssize_t r = write(wake_fd, &one, sizeof one);
	(void)r;
}

static cef_browser_process_handler_t browser_process_handler;

static cef_browser_process_handler_t *CEF_CALLBACK get_browser_process_handler(cef_app_t *self)
{
	(void)self;
	return &browser_process_handler;
}

static cef_app_t app;

static void init_objects(void)
{
	init_base(&render_handler.base, sizeof render_handler);
	render_handler.get_view_rect = get_view_rect;
	render_handler.get_screen_info = get_screen_info;
	render_handler.on_paint = on_paint;
	render_handler.on_accelerated_paint = on_accelerated_paint;

	init_base(&life_span.base, sizeof life_span);
	life_span.on_after_created = on_after_created;
	life_span.do_close = do_close;
	life_span.on_before_close = on_before_close;

	init_base(&client.base, sizeof client);
	client.get_render_handler = get_render_handler;
	client.get_life_span_handler = get_life_span_handler;
	client.get_load_handler = get_load_handler;

	init_base(&load_handler.base, sizeof load_handler);
	load_handler.on_loading_state_change = on_loading_state_change;
	load_handler.on_load_error = on_load_error;

	init_base(&browser_process_handler.base, sizeof browser_process_handler);
	browser_process_handler.on_schedule_message_pump_work = on_schedule_message_pump_work;

	init_base(&app.base, sizeof app);
	app.on_before_command_line_processing = on_before_command_line_processing;
	app.get_browser_process_handler = get_browser_process_handler;
}

/* ---- X11 + EGL window --------------------------------------------------- */

static void window_init(void)
{
	xdpy = XOpenDisplay(NULL);
	if (!xdpy) {
		fprintf(stderr, "spike: cannot open X display\n");
		exit(1);
	}
	edpy = eglGetPlatformDisplay(EGL_PLATFORM_X11_KHR, xdpy, NULL);
	eglInitialize(edpy, NULL, NULL);
	eglBindAPI(EGL_OPENGL_ES_API);
	EGLint cfg_attr[] = {EGL_SURFACE_TYPE, EGL_WINDOW_BIT, EGL_RENDERABLE_TYPE,
			     EGL_OPENGL_ES3_BIT, EGL_RED_SIZE, 8, EGL_GREEN_SIZE, 8,
			     EGL_BLUE_SIZE, 8, EGL_NONE};
	EGLConfig cfg;
	EGLint ncfg;
	eglChooseConfig(edpy, cfg_attr, &cfg, 1, &ncfg);
	EGLint vid;
	eglGetConfigAttrib(edpy, cfg, EGL_NATIVE_VISUAL_ID, &vid);
	XVisualInfo tmpl = {.visualid = (VisualID)vid}, *vi;
	int nvi;
	vi = XGetVisualInfo(xdpy, VisualIDMask, &tmpl, &nvi);
	XSetWindowAttributes swa = {
		.colormap = XCreateColormap(xdpy, DefaultRootWindow(xdpy), vi->visual, AllocNone),
		.event_mask = StructureNotifyMask | ExposureMask | KeyPressMask,
	};
	xwin = XCreateWindow(xdpy, DefaultRootWindow(xdpy), 0, 0, win_w, win_h, 0,
			     vi->depth, InputOutput, vi->visual, CWColormap | CWEventMask, &swa);
	XClassHint ch = {.res_name = "osr-spike", .res_class = "osr-spike"};
	XSetClassHint(xdpy, xwin, &ch);
	XStoreName(xdpy, xwin, "appshell spike");
	wm_delete = XInternAtom(xdpy, "WM_DELETE_WINDOW", False);
	XSetWMProtocols(xdpy, xwin, &wm_delete, 1);
	XMapWindow(xdpy, xwin);
	XFree(vi);

	EGLint ctx_attr[] = {EGL_CONTEXT_MAJOR_VERSION, 3, EGL_NONE};
	ectx = eglCreateContext(edpy, cfg, EGL_NO_CONTEXT, ctx_attr);
	esurf = eglCreatePlatformWindowSurface(edpy, cfg, &xwin, NULL);
	eglMakeCurrent(edpy, esurf, esurf, ectx);
	eglSwapInterval(edpy, getenv("SPIKE_NOVSYNC") ? 0 : 1);

	const char *ext = eglQueryString(edpy, EGL_EXTENSIONS);
	if (!strstr(ext, "EGL_EXT_image_dma_buf_import"))
		fprintf(stderr, "spike: WARNING: no EGL_EXT_image_dma_buf_import\n");
	create_image = (PFNEGLCREATEIMAGEKHRPROC)eglGetProcAddress("eglCreateImageKHR");
	destroy_image = (PFNEGLDESTROYIMAGEKHRPROC)eglGetProcAddress("eglDestroyImageKHR");
	image_target_texture = (PFNGLEGLIMAGETARGETTEXTURE2DOESPROC)
		eglGetProcAddress("glEGLImageTargetTexture2DOES");
	fprintf(stderr, "spike: GL %s / %s\n", glGetString(GL_VERSION), glGetString(GL_RENDERER));
}

static void handle_x_events(void)
{
	while (XPending(xdpy)) {
		XEvent ev;
		XNextEvent(xdpy, &ev);
		switch (ev.type) {
		case ConfigureNotify:
			if (ev.xconfigure.width != win_w || ev.xconfigure.height != win_h) {
				win_w = ev.xconfigure.width;
				win_h = ev.xconfigure.height;
				if (browser) {
					cef_browser_host_t *h = browser->get_host(browser);
					h->was_resized(h);
				}
			}
			break;
		case Expose:
			present();
			break;
		case ClientMessage:
			if ((Atom)ev.xclient.data.l[0] == wm_delete && browser) {
				cef_browser_host_t *h = browser->get_host(browser);
				h->close_browser(h, 1);
			}
			break;
		}
	}
}

int main(int argc, char **argv)
{
	cef_api_hash(CEF_API_VERSION, 0);
	init_objects();

	cef_main_args_t args = {argc, argv};
	int code = cef_execute_process(&args, &app, NULL);
	if (code >= 0)
		return code; /* a CEF child process finished */

	/* Copy now: Chromium reuses the environment's memory for the process title. */
	const char *url_s = getenv("SPIKE_URL") ? strdup(getenv("SPIKE_URL")) : NULL;
	if (!url_s) {
		fprintf(stderr, "usage: SPIKE_URL=... %s\n", argv[0]);
		return 2;
	}
	shot_path = getenv("SPIKE_SHOT") ? strdup(getenv("SPIKE_SHOT")) : NULL;
	shot_after_frames = getenv("SPIKE_SHOT_FRAMES") ? atol(getenv("SPIKE_SHOT_FRAMES")) : 0;
	args.argc = 1; /* nothing positional for CEF to open */
	t_start = now_ms();
	wake_fd = eventfd(0, EFD_NONBLOCK);
	window_init();
	gl_init();

	cef_settings_t st = {0};
	st.size = sizeof st;
	st.no_sandbox = 1;
	st.windowless_rendering_enabled = 1;
	st.external_message_pump = getenv("SPIKE_RUNLOOP") ? 0 : 1;
	st.log_severity = getenv("SPIKE_VERBOSE") ? LOGSEVERITY_INFO : LOGSEVERITY_WARNING;
	char cache[512];
	snprintf(cache, sizeof cache, "%s/.cache/appshell-spike", getenv("HOME"));
	st.root_cache_path = str(cache);
	st.cache_path = str(cache);
	if (!cef_initialize(&args, &st, &app, NULL)) {
		fprintf(stderr, "spike: cef_initialize failed\n");
		return 1;
	}
	fprintf(stderr, "spike: CEF initialized at %.0f ms\n", now_ms() - t_start);

	cef_window_info_t wi = {0};
	wi.size = sizeof wi;
	wi.windowless_rendering_enabled = 1;
	wi.shared_texture_enabled = getenv("SPIKE_SOFTWARE") ? 0 : 1;
	wi.parent_window = xwin;
	wi.runtime_style = CEF_RUNTIME_STYLE_ALLOY;
	cef_browser_settings_t bs = {0};
	bs.size = sizeof bs;
	bs.windowless_frame_rate = 120;
	bs.background_color = 0; /* transparent, so the underlay shows through */
	cef_string_t url = str(url_s);
	int created = cef_browser_host_create_browser(&wi, &client, &url, &bs, NULL, NULL);
	fprintf(stderr, "spike: create_browser returned %d (url length %zu)\n", created, url.length);

	if (getenv("SPIKE_RUNLOOP")) {
		cef_run_message_loop(); /* experiment: CEF's own loop, X events ignored */
		return 0;
	}
	int xfd = ConnectionNumber(xdpy);
	while (!atomic_load(&browser_closed)) {
		handle_x_events();
		long long due = atomic_load(&pump_due_ms);
		double t = now_ms();
		if (due <= t) {
			atomic_store(&pump_due_ms, (long long)(t + 1000 / 120)); /* safety tick */
			cef_do_message_loop_work();
		}
		due = atomic_load(&pump_due_ms);
		int timeout = due > now_ms() ? (int)(due - now_ms()) : 0;
		if (timeout > 33)
			timeout = 33; /* CEF asks external pumps to run at least ~30 times a second */
		struct pollfd p[2] = {{xfd, POLLIN, 0}, {wake_fd, POLLIN, 0}};
		if (poll(p, 2, timeout) > 0 && (p[1].revents & POLLIN)) {
			uint64_t v;
			ssize_t r = read(wake_fd, &v, sizeof v);
			(void)r;
		}
		if (browser && !getenv("SPIKE_PROBED") && now_ms() - t_start > 3000) {
			setenv("SPIKE_PROBED", "1", 1);
			cef_frame_t *f = browser->get_main_frame(browser);
			cef_string_userfree_t u = f ? f->get_url(f) : NULL;
			cef_string_utf8_t u8 = {0};
			if (u)
				cef_string_utf16_to_utf8(u->str, u->length, &u8);
			fprintf(stderr, "spike: probe: frame=%p url=%s loading=%d view_rect=%ld screen_info=%ld\n",
				(void *)f, u8.str ? u8.str : "(null)", browser->is_loading(browser),
				n_view_rect, n_screen_info);
		}
		if (getenv("SPIKE_SECONDS") && now_ms() - t_start > atof(getenv("SPIKE_SECONDS")) * 1e3 &&
		    browser) {
			cef_browser_host_t *h = browser->get_host(browser);
			h->close_browser(h, 1);
			setenv("SPIKE_SECONDS", "1e9", 1);
		}
	}

	long total = frames_accel + frames_soft;
	fprintf(stderr, "spike: %ld GPU frames, %ld software frames, %.2f ms average import+draw\n",
		frames_accel, frames_soft, total ? import_draw_total_ms / total : 0.0);
	if (total)
		fprintf(stderr, "spike: per frame: import %.2f ms, draw+finish %.2f ms, swap %.2f ms\n",
			t_import / total, t_draw / total, t_swap / total);
	cef_shutdown();
	return 0;
}
