/*
 * Phase 0 spike: libmpv's render API drawing into our own X11/EGL (GLES 3)
 * window, the way appshell will draw video under the page. Measures which
 * renderer libmpv uses, whether VA-API decoding works through it, the GPU time
 * of each shader pass (vo-passes), dropped frames, GPU load and CPU time, with
 * and without Anime4K Mode A+A.
 *
 * Usage: SPIKE_FILE=video [SPIKE_A4K=aa|none] [SPIKE_SECONDS=20]
 *        [SPIKE_VSYNC=1] [SPIKE_SHADERS=dir] mpv_spike
 */

#include <mpv/client.h>
#include <mpv/render.h>
#include <mpv/render_gl.h>

#include <EGL/egl.h>
#include <EGL/eglext.h>
#include <GLES3/gl3.h>
#include <X11/Xatom.h>
#include <X11/Xlib.h>
#include <X11/Xutil.h>

#include <poll.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/eventfd.h>
#include <sys/resource.h>
#include <time.h>
#include <unistd.h>

static Display *xdpy;
static Window xwin;
static EGLDisplay edpy;
static EGLSurface esurf;
static int win_w = 1440, win_h = 900;
static int wake_fd;

static double now_ms(void)
{
	struct timespec ts;
	clock_gettime(CLOCK_MONOTONIC, &ts);
	return ts.tv_sec * 1e3 + ts.tv_nsec / 1e6;
}

static void *get_proc(void *ctx, const char *name)
{
	(void)ctx;
	return (void *)eglGetProcAddress(name);
}

static void wake(void *ctx)
{
	(void)ctx;
	uint64_t one = 1;
	ssize_t r = write(wake_fd, &one, sizeof one);
	(void)r;
}

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
	EGLint ncfg, vid;
	eglChooseConfig(edpy, cfg_attr, &cfg, 1, &ncfg);
	eglGetConfigAttrib(edpy, cfg, EGL_NATIVE_VISUAL_ID, &vid);
	XVisualInfo tmpl = {.visualid = (VisualID)vid}, *vi;
	int nvi;
	vi = XGetVisualInfo(xdpy, VisualIDMask, &tmpl, &nvi);
	XSetWindowAttributes swa = {
		.colormap = XCreateColormap(xdpy, DefaultRootWindow(xdpy), vi->visual, AllocNone),
		.event_mask = StructureNotifyMask,
	};
	xwin = XCreateWindow(xdpy, DefaultRootWindow(xdpy), 0, 0, win_w, win_h, 0,
			     vi->depth, InputOutput, vi->visual, CWColormap | CWEventMask, &swa);
	XClassHint ch = {.res_name = "mpv-spike", .res_class = "mpv-spike"};
	XSetClassHint(xdpy, xwin, &ch);
	XStoreName(xdpy, xwin, "appshell mpv spike");
	/* Ask dwm for fullscreen so Anime4K renders at the panel's 2880x1800. */
	Atom state = XInternAtom(xdpy, "_NET_WM_STATE", False);
	Atom fs = XInternAtom(xdpy, "_NET_WM_STATE_FULLSCREEN", False);
	XChangeProperty(xdpy, xwin, state, XA_ATOM, 32, PropModeReplace, (unsigned char *)&fs, 1);
	XMapWindow(xdpy, xwin);
	XFree(vi);

	EGLint ctx_attr[] = {EGL_CONTEXT_MAJOR_VERSION, 3, EGL_NONE};
	EGLContext ectx = eglCreateContext(edpy, cfg, EGL_NO_CONTEXT, ctx_attr);
	esurf = eglCreatePlatformWindowSurface(edpy, cfg, &xwin, NULL);
	eglMakeCurrent(edpy, esurf, esurf, ectx);
	eglSwapInterval(edpy, getenv("SPIKE_VSYNC") ? 1 : 0);
	fprintf(stderr, "spike: GL %s / %s\n", glGetString(GL_VERSION), glGetString(GL_RENDERER));
}

static int gpu_busy(void)
{
	FILE *f = fopen("/sys/class/drm/card1/device/gpu_busy_percent", "r");
	int v = -1;
	if (f) {
		if (fscanf(f, "%d", &v) != 1)
			v = -1;
		fclose(f);
	}
	return v;
}

/* Print vo-passes while the reply event's data is still valid. */
static void print_passes(const mpv_node *np)
{
	mpv_node n = *np;
	for (int i = 0; n.format == MPV_FORMAT_NODE_MAP && i < n.u.list->num; i++) {
		if (strcmp(n.u.list->keys[i], "fresh"))
			continue;
		mpv_node *arr = &n.u.list->values[i];
		double total_avg = 0;
		for (int j = 0; arr->format == MPV_FORMAT_NODE_ARRAY && j < arr->u.list->num; j++) {
			mpv_node *p = &arr->u.list->values[j];
			const char *desc = "?";
			double avg = 0, peak = 0;
			for (int k = 0; k < p->u.list->num; k++) {
				const char *key = p->u.list->keys[k];
				mpv_node *v = &p->u.list->values[k];
				if (!strcmp(key, "desc") && v->format == MPV_FORMAT_STRING)
					desc = v->u.string;
				else if (!strcmp(key, "avg") && v->format == MPV_FORMAT_INT64)
					avg = v->u.int64 / 1e6;
				else if (!strcmp(key, "peak") && v->format == MPV_FORMAT_INT64)
					peak = v->u.int64 / 1e6;
			}
			total_avg += avg;
			fprintf(stderr, "spike:   pass %-40.40s avg %6.2f ms  peak %6.2f ms\n", desc, avg, peak);
		}
		fprintf(stderr, "spike: GPU time per fresh frame (sum of pass averages): %.2f ms\n", total_avg);
	}
}

static void print_prop(mpv_handle *mpv, const char *name)
{
	char *s = mpv_get_property_string(mpv, name);
	fprintf(stderr, "spike: %s = %s\n", name, s ? s : "(unavailable)");
	mpv_free(s);
}

int main(void)
{
	const char *file = getenv("SPIKE_FILE");
	if (!file) {
		fprintf(stderr, "usage: SPIKE_FILE=video mpv_spike\n");
		return 2;
	}
	const char *a4k = getenv("SPIKE_A4K") ? getenv("SPIKE_A4K") : "aa";
	double seconds = getenv("SPIKE_SECONDS") ? atof(getenv("SPIKE_SECONDS")) : 20;
	char shaders_dir[512];
	snprintf(shaders_dir, sizeof shaders_dir, "%s",
		 getenv("SPIKE_SHADERS") ? getenv("SPIKE_SHADERS") : "anime4k");

	wake_fd = eventfd(0, EFD_NONBLOCK);
	window_init();

	mpv_handle *mpv = mpv_create();
	mpv_set_option_string(mpv, "vo", "libmpv");
	mpv_set_option_string(mpv, "hwdec", "vaapi");
	mpv_set_option_string(mpv, "deband", "yes");
	mpv_set_option_string(mpv, "deband-iterations", "2");
	mpv_set_option_string(mpv, "deband-threshold", "35");
	mpv_set_option_string(mpv, "deband-range", "16");
	mpv_set_option_string(mpv, "deband-grain", "4");
	mpv_set_option_string(mpv, "loop-file", "inf");
	mpv_set_option_string(mpv, "terminal", "no");
	/* Anime4K Mode A+A, HQ ("aa") and Fast ("aafast", Seanime's default),
	 * in the order Anime4K's own mpv config uses. */
	static const char *chain_hq[] = {
		"Clamp_Highlights", "Restore_CNN_VL", "Upscale_CNN_x2_VL",
		"Restore_CNN_M", "AutoDownscalePre_x2", "AutoDownscalePre_x4",
		"Upscale_CNN_x2_M"};
	static const char *chain_fast[] = {
		"Clamp_Highlights", "Restore_CNN_M", "Upscale_CNN_x2_M",
		"Restore_CNN_S", "AutoDownscalePre_x2", "AutoDownscalePre_x4",
		"Upscale_CNN_x2_S"};
	if (!strcmp(a4k, "aa") || !strcmp(a4k, "aafast")) {
		const char **chain = !strcmp(a4k, "aa") ? chain_hq : chain_fast;
		char list[4096] = "";
		for (size_t i = 0; i < 7; i++) {
			char one[600];
			snprintf(one, sizeof one, "%s%s/Anime4K_%s.glsl", i ? ":" : "", shaders_dir, chain[i]);
			strncat(list, one, sizeof list - strlen(list) - 1);
		}
		mpv_set_option_string(mpv, "glsl-shaders", list);
	}
	mpv_request_log_messages(mpv, "v");
	if (mpv_initialize(mpv) < 0) {
		fprintf(stderr, "spike: mpv_initialize failed\n");
		return 1;
	}
	mpv_set_wakeup_callback(mpv, wake, NULL);

	mpv_opengl_init_params gl_init = {.get_proc_address = get_proc};
	int advanced = 1;
	mpv_render_param params[] = {
		{MPV_RENDER_PARAM_API_TYPE, (void *)MPV_RENDER_API_TYPE_OPENGL},
		{MPV_RENDER_PARAM_OPENGL_INIT_PARAMS, &gl_init},
		{MPV_RENDER_PARAM_X11_DISPLAY, xdpy},
		{MPV_RENDER_PARAM_ADVANCED_CONTROL, &advanced},
		{0, NULL},
	};
	mpv_render_context *rc;
	if (mpv_render_context_create(&rc, mpv, params) < 0) {
		fprintf(stderr, "spike: mpv_render_context_create failed\n");
		return 1;
	}
	mpv_render_context_set_update_callback(rc, wake, NULL);

	const char *cmd[] = {"loadfile", file, NULL};
	mpv_command(mpv, cmd);

	double t0 = now_ms(), t_first = 0;
	int asked_passes = 0;
	long renders = 0, busy_sum = 0, busy_n = 0;
	double next_sample = t0 + 2000; /* skip startup */
	struct rusage ru0;
	getrusage(RUSAGE_SELF, &ru0);
	int xfd = ConnectionNumber(xdpy);
	while (now_ms() - t0 < seconds * 1e3) {
		while (XPending(xdpy)) {
			XEvent ev;
			XNextEvent(xdpy, &ev);
			if (ev.type == ConfigureNotify) {
				win_w = ev.xconfigure.width;
				win_h = ev.xconfigure.height;
			}
		}
		for (;;) {
			mpv_event *e = mpv_wait_event(mpv, 0);
			if (e->event_id == MPV_EVENT_NONE)
				break;
			if (e->event_id == MPV_EVENT_GET_PROPERTY_REPLY && e->reply_userdata == 1) {
				mpv_event_property *pr = e->data;
				if (e->error >= 0 && pr->format == MPV_FORMAT_NODE)
					print_passes(pr->data);
				else
					fprintf(stderr, "spike: vo-passes unavailable\n");
				continue;
			}
			if (e->event_id == MPV_EVENT_LOG_MESSAGE) {
				mpv_event_log_message *m = e->data;
				if (strstr(m->text, "libplacebo") || strstr(m->prefix, "vd") ||
				    strstr(m->text, "hwdec") || strstr(m->text, "Using") ||
				    !strcmp(m->level, "error") || !strcmp(m->level, "warn"))
					fprintf(stderr, "mpv[%s] %s: %s", m->level, m->prefix, m->text);
			}
		}
		if (mpv_render_context_update(rc) & MPV_RENDER_UPDATE_FRAME) {
			mpv_opengl_fbo fbo = {.fbo = 0, .w = win_w, .h = win_h};
			int flip = 1;
			mpv_render_param rp[] = {
				{MPV_RENDER_PARAM_OPENGL_FBO, &fbo},
				{MPV_RENDER_PARAM_FLIP_Y, &flip},
				{0, NULL},
			};
			mpv_render_context_render(rc, rp);
			eglSwapBuffers(edpy, esurf);
			mpv_render_context_report_swap(rc);
			if (!renders++)
				t_first = now_ms() - t0;
		}
		double t = now_ms();
		/* vo-passes is answered by the render thread (this one), so ask while still rendering. */
		if (!asked_passes && t - t0 > seconds * 1e3 - 1500) {
			mpv_get_property_async(mpv, 1, "vo-passes", MPV_FORMAT_NODE);
			asked_passes = 1;
		}
		if (t >= next_sample) {
			int b = gpu_busy();
			if (b >= 0) {
				busy_sum += b;
				busy_n++;
			}
			next_sample = t + 250;
		}
		struct pollfd p[2] = {{xfd, POLLIN, 0}, {wake_fd, POLLIN, 0}};
		if (poll(p, 2, 50) > 0 && (p[1].revents & POLLIN)) {
			uint64_t v;
			ssize_t r = read(wake_fd, &v, sizeof v);
			(void)r;
		}
	}
	struct rusage ru1;
	getrusage(RUSAGE_SELF, &ru1);
	double cpu_ms = (ru1.ru_utime.tv_sec - ru0.ru_utime.tv_sec) * 1e3 +
			(ru1.ru_utime.tv_usec - ru0.ru_utime.tv_usec) / 1e3 +
			(ru1.ru_stime.tv_sec - ru0.ru_stime.tv_sec) * 1e3 +
			(ru1.ru_stime.tv_usec - ru0.ru_stime.tv_usec) / 1e3;
	double wall = now_ms() - t0;

	fprintf(stderr, "spike: mode=%s window=%dx%d first frame %.0f ms, %ld frames in %.1f s\n",
		a4k, win_w, win_h, t_first, renders, wall / 1e3);
	print_prop(mpv, "current-vo");
	print_prop(mpv, "hwdec-current");
	print_prop(mpv, "video-params/pixelformat");
	print_prop(mpv, "estimated-vf-fps");
	print_prop(mpv, "frame-drop-count");
	print_prop(mpv, "vo-delayed-frame-count");
	print_prop(mpv, "mistimed-frame-count");
	fprintf(stderr, "spike: GPU busy %.1f%% average, CPU %.1f%% of one core\n",
		busy_n ? (double)busy_sum / busy_n : -1.0, 100.0 * cpu_ms / wall);

	mpv_render_context_free(rc);
	mpv_terminate_destroy(mpv);
	return 0;
}
