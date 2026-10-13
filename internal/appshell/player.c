/*
 * player.c: libmpv players behind window.mpvPrism.
 *
 * Each player renders on its own thread, through libmpv's render API, into a
 * small queue of textures the size of its presenter's rectangle on the page
 * (so mpv's scalers and shaders work at the real display size). mpv hands
 * frames over ahead of time with the time each should appear; the main
 * thread draws the newest frame that is due under the page at every swap.
 * So a slow frame (Anime4K takes 20-35 ms of GPU time) never holds up the
 * window: the page, menus and input keep running at the panel's rate while
 * the video thread works ahead.
 *
 * Property changes are batched per wakeup and sent to the page as mpv-prism
 * events; the screen saver is held off while anything plays.
 */
#include "appshell.h"
#include "bridge.h"

#include <mpv/client.h>
#include <mpv/render.h>
#include <mpv/render_gl.h>

#include <math.h>
#include <pthread.h>
#include <sys/wait.h>
#include <unistd.h>
#include <stdatomic.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

#define OBSERVE_PAGE 1     /* reply_userdata of properties the page observes */
#define OBSERVE_INTERNAL 2 /* ours: pause, core-idle */
#define MAX_OBSERVED 96

/*
 * Video frames in flight. 6 covers mpv's render-ahead (video-timing-offset,
 * 0.1 s: 3 frames at 24 fps) plus the frame on screen and the one being
 * drawn; at higher frame rates the render thread waits for a free slot.
 */
#define NSLOTS 6
/* A frame is shown this long before its time: the swap reaches the panel at
 * the next vblank, on average half a refresh (4.2 ms at 120 Hz) later. */
#define DUE_LEAD_NS 4000000

enum { SLOT_FREE, SLOT_RENDERING, SLOT_QUEUED, SLOT_SHOWN };

typedef struct vslot {
	GLuint tex, fbo;
	int w, h;
	int state;
	GLsync done;       /* the render thread's drawing finished (on the GPU) */
	GLsync read;       /* the window's last draw from this texture finished */
	/* When to show it, in mpv_get_time_ns() time (render.h says
	 * microseconds, but mpv 0.41 fills target_time in nanoseconds); 0: at once. */
	int64_t due;
	uint64_t seq;      /* render order */
} vslot;

typedef struct player {
	char id[128];
	mpv_handle *mpv;
	mpv_render_context *rc; /* used only on the render thread */
	int presenter;        /* a presenter is attached */
	int has_rect, rect_visible, suspended;
	double rx, ry, rw, rh; /* presenter rectangle, DIP */
	/* Render thread. Everything below is under lock. */
	pthread_t thread;
	int thread_running;
	EGLContext gl;
	pthread_mutex_t lock;
	pthread_cond_t cond;
	int started;          /* 1 render context ready, -1 failed */
	int wake, quit;
	int want_w, want_h, want_visible; /* what the window shows (pixels) */
	vslot slots[NSLOTS];
	int shown;            /* slot on screen, -1 none */
	uint64_t seq;
	char observed[MAX_OBSERVED][64];
	int nobserved;
	int paused, idle, loaded;
	int paused_for_cache;
	int audio_lost, audio_tries; /* no audio output (PipeWire stopped under us) */
	double audio_retry_at;
	char aid[32];                /* last selected audio track */
	char *buffering_json; /* last cache-buffering-state value */
	/* Property changes waiting to go to the page: latest JSON value per name. */
	char *pend_name[MAX_OBSERVED];
	char *pend_json[MAX_OBSERVED];
	int npend;
	double last_flush;
	struct player *next;
} player;

static player *players;

#define REPLY_BASE 1000 /* reply_userdata of requests; observe ids are 1 and 2 */
static void answer(uint64_t token, int error, const char *value_json);

static player *find(const char *id)
{
	for (player *p = players; p; p = p->next)
		if (id && !strcmp(p->id, id))
			return p;
	return NULL;
}

/*
 * mpv's GL calls, with a flush after every draw and compute dispatch: each
 * shader pass then reaches the GPU as its own submission instead of one
 * 20-35 ms block per Anime4K frame. The render context has low priority, so
 * the GPU scheduler runs the window's frames between mpv's passes and the
 * page stays at the panel's rate while shaders run.
 */
static void (*real_draw_arrays)(GLenum, GLint, GLsizei);
static void (*real_dispatch_compute)(GLuint, GLuint, GLuint);
static void (*real_flush)(void);

static void flushing_draw_arrays(GLenum mode, GLint first, GLsizei count)
{
	real_draw_arrays(mode, first, count);
	real_flush();
}

static void flushing_dispatch_compute(GLuint x, GLuint y, GLuint z)
{
	real_dispatch_compute(x, y, z);
	real_flush();
}

static void *get_proc(void *ctx, const char *name)
{
	(void)ctx;
	void *f = (void *)eglGetProcAddress(name);
	if (!real_flush)
		real_flush = (void (*)(void))eglGetProcAddress("glFlush");
	if (f && !strcmp(name, "glDrawArrays")) {
		real_draw_arrays = f;
		return (void *)flushing_draw_arrays;
	}
	if (f && !strcmp(name, "glDispatchCompute")) {
		real_dispatch_compute = f;
		return (void *)flushing_dispatch_compute;
	}
	return f;
}

/* mpv has something to render: wake the player's render thread. */
static void on_update(void *ctx)
{
	player *p = ctx;
	pthread_mutex_lock(&p->lock);
	p->wake = 1;
	pthread_cond_broadcast(&p->cond);
	pthread_mutex_unlock(&p->lock);
}

static void on_wakeup(void *ctx)
{
	(void)ctx;
	as_wake();
}

/*
 * Audio on this laptop is on demand: PipeWire only runs after F2 or
 * `audio ensure`, and mpv here has no other audio output. Start it before
 * playing (the script waits for the default sink) and tell players that are
 * already open to reopen their audio output.
 */
static void reload_audio_outputs(void);

static void ensure_audio(void)
{
	char sock[4200];
	snprintf(sock, sizeof sock, "%s/pipewire-0", as_runtime_dir);
	if (access(sock, F_OK) == 0 || access("/usr/local/bin/audio", X_OK) != 0)
		return;
	AS_LOG("starting audio (audio ensure)\n");
	pid_t pid = fork();
	if (pid == 0) {
		/* PipeWire and WirePlumber are started from here and live on: they
		 * must not inherit our descriptors (they kept the DevTools socket). */
		closefrom(3);
		execl("/usr/local/bin/audio", "audio", "ensure", (char *)NULL);
		_exit(127);
	}
	if (pid > 0)
		waitpid(pid, NULL, 0);
	/* The status bar shows the audio state. */
	pid_t bar = fork();
	if (bar == 0) {
		closefrom(3);
		execlp("pkill", "pkill", "-USR1", "-x", "dwmstat", (char *)NULL);
		_exit(127);
	}
	if (bar > 0)
		waitpid(bar, NULL, 0);
	reload_audio_outputs();
}

/* ---- mpv node <-> JSON ------------------------------------------------- */

static yyjson_mut_val *node_to_json(yyjson_mut_doc *d, const mpv_node *n)
{
	switch (n->format) {
	case MPV_FORMAT_STRING:
	case MPV_FORMAT_OSD_STRING:
		return yyjson_mut_strcpy(d, n->u.string ? n->u.string : "");
	case MPV_FORMAT_FLAG:
		return yyjson_mut_bool(d, n->u.flag);
	case MPV_FORMAT_INT64:
		return yyjson_mut_sint(d, n->u.int64);
	case MPV_FORMAT_DOUBLE:
		return isfinite(n->u.double_) ? yyjson_mut_real(d, n->u.double_) : yyjson_mut_null(d);
	case MPV_FORMAT_NODE_ARRAY: {
		yyjson_mut_val *a = yyjson_mut_arr(d);
		for (int i = 0; i < n->u.list->num; i++)
			yyjson_mut_arr_append(a, node_to_json(d, &n->u.list->values[i]));
		return a;
	}
	case MPV_FORMAT_NODE_MAP: {
		yyjson_mut_val *o = yyjson_mut_obj(d);
		for (int i = 0; i < n->u.list->num; i++)
			yyjson_mut_obj_put(o, yyjson_mut_strcpy(d, n->u.list->keys[i]),
					   node_to_json(d, &n->u.list->values[i]));
		return o;
	}
	default:
		return yyjson_mut_null(d);
	}
}

/* A JSON scalar as the string mpv's option parser takes. */
static int json_to_option_string(yyjson_val *v, char *out, size_t n)
{
	if (yyjson_is_str(v))
		snprintf(out, n, "%s", yyjson_get_str(v));
	else if (yyjson_is_bool(v))
		snprintf(out, n, "%s", yyjson_get_bool(v) ? "yes" : "no");
	else if (yyjson_is_int(v))
		snprintf(out, n, "%lld", (long long)yyjson_get_sint(v));
	else if (yyjson_is_num(v))
		snprintf(out, n, "%.17g", yyjson_get_num(v));
	else
		return -1;
	return 0;
}

/* ---- events to the page ------------------------------------------------ */

/*
 * Property changes are batched: one message per player carrying the latest
 * value of every changed property, at most every 100 ms for values that change
 * every frame (time-pos, avsync, cache state...). Changes that matter at once
 * (pause, end of file, tracks) flush immediately, and state events (file
 * loaded, end of file...) flush pending properties first to keep the order.
 * Sending each change as its own call kept the page's renderer at 45% CPU.
 */
#define FLUSH_MS 100

static int urgent_property(const char *name)
{
	static const char *const list[] = {
		"pause", "eof-reached", "idle-active", "core-idle", "track-list", "aid", "sid",
		"vid", "duration", "path", "media-title", "video-params", "sub-text", "sub-start",
		"sub-end", "sub-visibility", "chapter-list", "speed", "volume", "mute", NULL};
	for (int i = 0; list[i]; i++)
		if (!strcmp(name, list[i]))
			return 1;
	return 0;
}

static void str_append(char **buf, size_t *len, size_t *cap, const char *s)
{
	size_t n = strlen(s);
	if (*len + n + 1 > *cap) {
		*cap = (*len + n + 1) * 2;
		*buf = realloc(*buf, *cap);
	}
	memcpy(*buf + *len, s, n + 1);
	*len += n;
}

/* JSON-quoted copy of a plain identifier (player ids and property names are
 * restricted by mpv-prism to [A-Za-z0-9_.:/-]). */
static void append_quoted(char **buf, size_t *len, size_t *cap, const char *s)
{
	str_append(buf, len, cap, "\"");
	char one[2] = {0};
	for (; *s; s++) {
		if (*s == '"' || *s == '\\' || (unsigned char)*s < 0x20)
			continue;
		one[0] = *s;
		str_append(buf, len, cap, one);
	}
	str_append(buf, len, cap, "\"");
}

/* Sends the pending properties, plus one state event (JSON object) if given. */
static void flush_events(player *p, const char *state_event_json)
{
	if (!p->npend && !state_event_json)
		return;
	char *buf = NULL;
	size_t len = 0, cap = 0;
	str_append(&buf, &len, &cap, "{\"event\":\"mpv-prism:events\",\"playerId\":");
	append_quoted(&buf, &len, &cap, p->id);
	str_append(&buf, &len, &cap, ",\"events\":[");
	for (int i = 0; i < p->npend; i++) {
		if (i)
			str_append(&buf, &len, &cap, ",");
		str_append(&buf, &len, &cap, "{\"type\":\"property-change\",\"name\":");
		append_quoted(&buf, &len, &cap, p->pend_name[i]);
		str_append(&buf, &len, &cap, ",\"value\":");
		str_append(&buf, &len, &cap, p->pend_json[i]);
		str_append(&buf, &len, &cap, "}");
		free(p->pend_name[i]);
		free(p->pend_json[i]);
	}
	if (state_event_json) {
		if (p->npend)
			str_append(&buf, &len, &cap, ",");
		str_append(&buf, &len, &cap, state_event_json);
	}
	str_append(&buf, &len, &cap, "]}");
	p->npend = 0;
	p->last_flush = as_now_ms();
	as_page_send_raw(buf);
	free(buf);
}

static void queue_property(player *p, const char *name, char *json /* owned */)
{
	for (int i = 0; i < p->npend; i++) {
		if (!strcmp(p->pend_name[i], name)) {
			free(p->pend_json[i]);
			p->pend_json[i] = json;
			return;
		}
	}
	if (p->npend >= MAX_OBSERVED) {
		free(json);
		return;
	}
	p->pend_name[p->npend] = strdup(name);
	p->pend_json[p->npend] = json;
	p->npend++;
}

static void update_playing(void)
{
	window_inhibit_screensaver(player_any_playing());
}

static const char *end_reason(mpv_end_file_reason r)
{
	switch (r) {
	case MPV_END_FILE_REASON_EOF: return "eof";
	case MPV_END_FILE_REASON_STOP: return "stop";
	case MPV_END_FILE_REASON_QUIT: return "quit";
	case MPV_END_FILE_REASON_ERROR: return "error";
	case MPV_END_FILE_REASON_REDIRECT: return "redirect";
	}
	return "unknown";
}

void player_process_events(void)
{
	double now = as_now_ms();
	for (player *p = players; p; p = p->next) {
		int urgent = 0;
		for (;;) {
			mpv_event *e = mpv_wait_event(p->mpv, 0);
			if (e->event_id == MPV_EVENT_NONE)
				break;
			char state[512];
			state[0] = 0;
			switch (e->event_id) {
			case MPV_EVENT_COMMAND_REPLY:
			case MPV_EVENT_SET_PROPERTY_REPLY:
				if (e->reply_userdata >= REPLY_BASE)
					answer(e->reply_userdata, e->error, NULL);
				break;
			case MPV_EVENT_GET_PROPERTY_REPLY:
				if (e->reply_userdata >= REPLY_BASE) {
					mpv_event_property *pr = e->data;
					char *json = NULL;
					if (e->error >= 0 && pr->format == MPV_FORMAT_NODE) {
						yyjson_mut_doc *d = yyjson_mut_doc_new(NULL);
						json = yyjson_mut_val_write(node_to_json(d, pr->data), 0, NULL);
						yyjson_mut_doc_free(d);
					}
					/* A missing property reads as null, as before. */
					answer(e->reply_userdata, 0, json ? json : "null");
					free(json);
				}
				break;
			case MPV_EVENT_PROPERTY_CHANGE: {
				mpv_event_property *pr = e->data;
				if (e->reply_userdata == OBSERVE_INTERNAL) {
					int flag = pr->format == MPV_FORMAT_NODE &&
						   ((mpv_node *)pr->data)->format == MPV_FORMAT_FLAG &&
						   ((mpv_node *)pr->data)->u.flag;
					if (!strcmp(pr->name, "current-ao")) {
						/* PipeWire can be stopped and started again during
						 * playback (F2); mpv then gives up on audio for the
						 * rest of the file. Bring it back. */
						int none = pr->format != MPV_FORMAT_NODE ||
							   ((mpv_node *)pr->data)->format != MPV_FORMAT_STRING;
						if (none && !p->audio_lost) {
							p->audio_lost = 1;
							p->audio_tries = 0;
							p->audio_retry_at = now + 1000;
						} else if (!none) {
							p->audio_lost = 0;
						}
						break;
					}
					if (!strcmp(pr->name, "pause")) p->paused = flag;
					else if (!strcmp(pr->name, "core-idle")) p->idle = flag;
					else if (!strcmp(pr->name, "paused-for-cache")) {
						p->paused_for_cache = flag;
						queue_property(p, "cache-buffering-state",
							       strdup(flag && p->buffering_json ? p->buffering_json : "0"));
						urgent = 1;
					}
					update_playing();
					break;
				}
				if (!strcmp(pr->name, "aid") && pr->format == MPV_FORMAT_NODE) {
					mpv_node *n = pr->data;
					if (n->format == MPV_FORMAT_INT64)
						snprintf(p->aid, sizeof p->aid, "%lld", (long long)n->u.int64);
				}
				yyjson_mut_doc *d = yyjson_mut_doc_new(NULL);
				yyjson_mut_val *v = pr->format == MPV_FORMAT_NODE
					? node_to_json(d, pr->data) : yyjson_mut_null(d);
				char *json = yyjson_mut_val_write(v, 0, NULL);
				yyjson_mut_doc_free(d);
				/* mpv reports 100 when the cache is full; Seanime's player takes any
				 * value above 0 as "buffering" and would show its spinner forever.
				 * Report it only while playback actually waits for the cache. */
				if (json && !strcmp(pr->name, "cache-buffering-state")) {
					free(p->buffering_json);
					p->buffering_json = json;
					json = strdup(p->paused_for_cache ? p->buffering_json : "0");
				}
				if (json)
					queue_property(p, pr->name, json);
				if (urgent_property(pr->name))
					urgent = 1;
				break;
			}
			case MPV_EVENT_FILE_LOADED:
				p->loaded = 1;
				snprintf(state, sizeof state, "{\"type\":\"file-loaded\"}");
				update_playing();
				break;
			case MPV_EVENT_PLAYBACK_RESTART:
				snprintf(state, sizeof state, "{\"type\":\"playback-restart\"}");
				break;
			case MPV_EVENT_END_FILE: {
				mpv_event_end_file *ef = e->data;
				p->loaded = 0;
				if (ef->reason == MPV_END_FILE_REASON_ERROR)
					snprintf(state, sizeof state,
						 "{\"type\":\"end-file\",\"reason\":\"error\",\"numericReason\":%d,\"error\":\"%s\"}",
						 ef->reason, mpv_error_string(ef->error));
				else
					snprintf(state, sizeof state, "{\"type\":\"end-file\",\"reason\":\"%s\",\"numericReason\":%d}",
						 end_reason(ef->reason), ef->reason);
				update_playing();
				break;
			}
			case MPV_EVENT_SHUTDOWN:
				snprintf(state, sizeof state, "{\"type\":\"shutdown\"}");
				break;
			case MPV_EVENT_LOG_MESSAGE: {
				mpv_event_log_message *m = e->data;
				AS_LOG("mpv[%s] %s: %s", m->prefix, m->level, m->text);
				break;
			}
			default:
				break;
			}
			if (state[0])
				flush_events(p, state);
		}
		/* Reopen audio once PipeWire is back (F2 turned it off on purpose:
		 * wait for it rather than starting it again). */
		if (p->audio_lost && !p->idle && now >= p->audio_retry_at && p->audio_tries < 10) {
			p->audio_retry_at = now + 2000;
			char sock[4200];
			snprintf(sock, sizeof sock, "%s/pipewire-0", as_runtime_dir);
			if (access(sock, F_OK) == 0) {
				p->audio_tries++;
				AS_LOG("%s: no audio output, reopening (%d)\n", p->id, p->audio_tries);
				/* After a failed open mpv drops the audio chain; selecting
				 * the track again rebuilds it (ao-reload alone doesn't). */
				const char *off[] = {"set", "aid", "no", NULL};
				const char *on[] = {"set", "aid", p->aid[0] ? p->aid : "auto", NULL};
				mpv_command_async(p->mpv, 0, off);
				mpv_command_async(p->mpv, 0, on);
			}
		}
		if (p->npend && (urgent || now - p->last_flush >= FLUSH_MS))
			flush_events(p, NULL);
	}
}

/* ---- render threads ---------------------------------------------------- */

static void slot_size(vslot *v, int w, int h)
{
	if (v->tex && v->w == w && v->h == h)
		return;
	if (!v->tex) {
		glGenTextures(1, &v->tex);
		glGenFramebuffers(1, &v->fbo);
	}
	glBindTexture(GL_TEXTURE_2D, v->tex);
	/* 10 bits per channel when the window has them (X at depth 30): mpv then
	 * dithers to 10 instead of 8 bits. */
	if (as_depth >= 10)
		glTexImage2D(GL_TEXTURE_2D, 0, GL_RGB10_A2, w, h, 0, GL_RGBA,
			     GL_UNSIGNED_INT_2_10_10_10_REV, NULL);
	else
		glTexImage2D(GL_TEXTURE_2D, 0, GL_RGBA8, w, h, 0, GL_RGBA, GL_UNSIGNED_BYTE, NULL);
	glTexParameteri(GL_TEXTURE_2D, GL_TEXTURE_MIN_FILTER, GL_LINEAR);
	glTexParameteri(GL_TEXTURE_2D, GL_TEXTURE_MAG_FILTER, GL_LINEAR);
	glTexParameteri(GL_TEXTURE_2D, GL_TEXTURE_WRAP_S, GL_CLAMP_TO_EDGE);
	glTexParameteri(GL_TEXTURE_2D, GL_TEXTURE_WRAP_T, GL_CLAMP_TO_EDGE);
	glBindFramebuffer(GL_FRAMEBUFFER, v->fbo);
	glFramebufferTexture2D(GL_FRAMEBUFFER, GL_COLOR_ATTACHMENT0, GL_TEXTURE_2D, v->tex, 0);
	glBindFramebuffer(GL_FRAMEBUFFER, 0);
	v->w = w;
	v->h = h;
}

static int free_slot(player *p)
{
	for (int i = 0; i < NSLOTS; i++)
		if (p->slots[i].state == SLOT_FREE)
			return i;
	return -1;
}

/*
 * The render thread: waits for mpv (a new frame, a redraw) or a new size,
 * renders into a free slot without waiting for the frame's time, and queues
 * it with that time. While the video isn't visible, frames are consumed
 * without drawing so playback and audio go on.
 */
static void *render_main(void *arg)
{
	player *p = arg;
	eglMakeCurrent(as_egl_dpy, EGL_NO_SURFACE, EGL_NO_SURFACE, p->gl);
	mpv_opengl_init_params glp = {.get_proc_address = get_proc};
	int advanced = 1;
	mpv_render_param params[] = {
		{MPV_RENDER_PARAM_API_TYPE, (void *)MPV_RENDER_API_TYPE_OPENGL},
		{MPV_RENDER_PARAM_OPENGL_INIT_PARAMS, &glp},
		{MPV_RENDER_PARAM_X11_DISPLAY, as_dpy},
		{MPV_RENDER_PARAM_ADVANCED_CONTROL, &advanced},
		{0, NULL},
	};
	int ok = mpv_render_context_create(&p->rc, p->mpv, params) >= 0;
	if (ok)
		mpv_render_context_set_update_callback(p->rc, on_update, p);
	pthread_mutex_lock(&p->lock);
	p->started = ok ? 1 : -1;
	pthread_cond_broadcast(&p->cond);
	pthread_mutex_unlock(&p->lock);
	if (!ok) {
		eglMakeCurrent(as_egl_dpy, EGL_NO_SURFACE, EGL_NO_SURFACE, EGL_NO_CONTEXT);
		return NULL;
	}

	int last_w = 0, last_h = 0;
	for (;;) {
		pthread_mutex_lock(&p->lock);
		while (!p->wake && !p->quit)
			pthread_cond_wait(&p->cond, &p->lock);
		if (p->quit) {
			pthread_mutex_unlock(&p->lock);
			break;
		}
		p->wake = 0;
		int w = p->want_w, h = p->want_h, vis = p->want_visible && w > 0 && h > 0;
		pthread_mutex_unlock(&p->lock);

		int frame = (mpv_render_context_update(p->rc) & MPV_RENDER_UPDATE_FRAME) != 0;
		int resized = vis && (w != last_w || h != last_h);
		if (!frame && !resized)
			continue;
		int block = 0; /* the main thread shows frames on time */
		if (!vis) {
			int skip = 1;
			mpv_render_param rp[] = {
				{MPV_RENDER_PARAM_SKIP_RENDERING, &skip},
				{MPV_RENDER_PARAM_BLOCK_FOR_TARGET_TIME, &block},
				{0, NULL},
			};
			mpv_render_context_render(p->rc, rp);
			last_w = last_h = 0; /* draw again once visible */
			continue;
		}
		mpv_render_frame_info info = {0};
		if (frame)
			mpv_render_context_get_info(p->rc, (mpv_render_param){MPV_RENDER_PARAM_NEXT_FRAME_INFO, &info});

		pthread_mutex_lock(&p->lock);
		int i;
		while ((i = free_slot(p)) < 0 && !p->quit)
			pthread_cond_wait(&p->cond, &p->lock);
		if (p->quit) {
			pthread_mutex_unlock(&p->lock);
			break;
		}
		vslot *v = &p->slots[i];
		GLsync read = v->read;
		v->read = NULL;
		v->state = SLOT_RENDERING;
		pthread_mutex_unlock(&p->lock);

		if (read) { /* the window may still be sampling this texture */
			glWaitSync(read, 0, GL_TIMEOUT_IGNORED);
			glDeleteSync(read);
		}
		slot_size(v, w, h);
		mpv_opengl_fbo fbo = {.fbo = (int)v->fbo, .w = w, .h = h,
				      .internal_format = as_depth >= 10 ? GL_RGB10_A2 : GL_RGBA8};
		int flip = 0, depth = as_depth;
		mpv_render_param rp[] = {
			{MPV_RENDER_PARAM_OPENGL_FBO, &fbo},
			{MPV_RENDER_PARAM_FLIP_Y, &flip},
			{MPV_RENDER_PARAM_DEPTH, &depth},
			{MPV_RENDER_PARAM_BLOCK_FOR_TARGET_TIME, &block},
			{0, NULL},
		};
		mpv_render_context_render(p->rc, rp);
		GLsync done = glFenceSync(GL_SYNC_GPU_COMMANDS_COMPLETE, 0);
		glFlush();
		int now = !(info.flags & MPV_RENDER_FRAME_INFO_PRESENT) ||
			  (info.flags & MPV_RENDER_FRAME_INFO_REDRAW) || info.target_time <= 0;

		pthread_mutex_lock(&p->lock);
		v->done = done;
		v->due = now ? 0 : info.target_time;
		v->seq = ++p->seq;
		v->state = SLOT_QUEUED;
		pthread_mutex_unlock(&p->lock);
		last_w = w;
		last_h = h;
		as_wake();
	}

	mpv_render_context_free(p->rc);
	p->rc = NULL;
	for (int i = 0; i < NSLOTS; i++) {
		vslot *v = &p->slots[i];
		if (v->done)
			glDeleteSync(v->done);
		if (v->read)
			glDeleteSync(v->read);
		if (v->tex) {
			glDeleteTextures(1, &v->tex);
			glDeleteFramebuffers(1, &v->fbo);
		}
		memset(v, 0, sizeof *v);
	}
	glFinish();
	eglMakeCurrent(as_egl_dpy, EGL_NO_SURFACE, EGL_NO_SURFACE, EGL_NO_CONTEXT);
	return NULL;
}

/* ---- showing frames (main thread) ------------------------------------- */

static int on_screen(const player *p)
{
	return p->presenter && p->has_rect && p->rect_visible && !p->suspended &&
	       p->rw >= 1 && p->rh >= 1;
}

/*
 * Tells each render thread what the window shows (size, visibility) and
 * returns how long until the next queued frame is due: 0 when one is due
 * now, -1 when none is waiting. Runs every loop iteration.
 */
double player_tick(void)
{
	double s = as_scale, next = -1;
	for (player *p = players; p; p = p->next) {
		if (!p->thread_running)
			continue;
		int vis = as_visible && on_screen(p);
		int w = vis ? (int)lround(p->rw * s) : 0, h = vis ? (int)lround(p->rh * s) : 0;
		int64_t now = mpv_get_time_ns(p->mpv);
		pthread_mutex_lock(&p->lock);
		if (vis != p->want_visible || w != p->want_w || h != p->want_h) {
			p->want_visible = vis;
			p->want_w = w;
			p->want_h = h;
			p->wake = 1;
			pthread_cond_broadcast(&p->cond);
		}
		for (int i = 0; vis && i < NSLOTS; i++) {
			vslot *v = &p->slots[i];
			if (v->state != SLOT_QUEUED)
				continue;
			double in = v->due == 0 ? 0 : (v->due - DUE_LEAD_NS - now) / 1e6;
			if (in < 0)
				in = 0;
			if (next < 0 || in < next)
				next = in;
		}
		pthread_mutex_unlock(&p->lock);
	}
	return next;
}

/* Picks the newest due frame (dropping older ones), releases the previous
 * one, and draws it into the presenter's rectangle. */
void player_draw_all(void)
{
	double s = as_scale;
	for (player *p = players; p; p = p->next) {
		if (!p->thread_running || !as_visible || !on_screen(p))
			continue;
		int64_t now = mpv_get_time_ns(p->mpv);
		GLsync wait = NULL;
		int shown = 0, skipped = 0;
		pthread_mutex_lock(&p->lock);
		int best = -1;
		for (int i = 0; i < NSLOTS; i++) {
			vslot *v = &p->slots[i];
			if (v->state == SLOT_QUEUED && (v->due == 0 || v->due - DUE_LEAD_NS <= now) &&
			    (best < 0 || v->seq > p->slots[best].seq))
				best = i;
		}
		if (best >= 0) {
			for (int i = 0; i < NSLOTS; i++) {
				vslot *v = &p->slots[i];
				if (v->state == SLOT_QUEUED && v->seq < p->slots[best].seq) {
					glDeleteSync(v->done);
					v->done = NULL;
					v->state = SLOT_FREE;
					skipped++;
				}
			}
			if (p->shown >= 0) {
				vslot *old = &p->slots[p->shown];
				/* The render thread waits for this before drawing into it. */
				old->read = glFenceSync(GL_SYNC_GPU_COMMANDS_COMPLETE, 0);
				glFlush();
				old->state = SLOT_FREE;
			}
			p->shown = best;
			p->slots[best].state = SLOT_SHOWN;
			wait = p->slots[best].done;
			p->slots[best].done = NULL;
			shown = 1;
			pthread_cond_broadcast(&p->cond);
		}
		GLuint tex = p->shown >= 0 ? p->slots[p->shown].tex : 0;
		pthread_mutex_unlock(&p->lock);
		stats_video(shown, skipped);
		if (wait) {
			glWaitSync(wait, 0, GL_TIMEOUT_IGNORED);
			glDeleteSync(wait);
		}
		if (!tex)
			continue;
		int x = (int)lround(p->rx * s), y = (int)lround(p->ry * s);
		int w = (int)lround(p->rw * s), h = (int)lround(p->rh * s);
		glBindFramebuffer(GL_FRAMEBUFFER, 0);
		glViewport(0, 0, as_win_w, as_win_h);
		compositor_blit_texture(tex, x, y, w, h);
	}
}

int player_any_playing(void)
{
	for (player *p = players; p; p = p->next)
		if (p->loaded && !p->paused && !p->idle)
			return 1;
	return 0;
}

/* ---- requests ------------------------------------------------------- */

/* Options the page may not set: they would break embedding or reach outside it. */
static int forbidden(const char *name)
{
	static const char *const list[] = {
		"vo", "wid", "gpu-context", "gpu-api", "input-ipc-server", "input-ipc-client",
		"script", "scripts", "script-opts", "load-scripts", "config-dir", "config",
		"include", "use-filedir-conf", "log-file", "dump-stats", "input-conf",
		"stream-dump", "record-file", "o", "terminal", NULL};
	for (int i = 0; list[i]; i++)
		if (!strcasecmp(name, list[i]))
			return 1;
	return 0;
}

static void observe(player *p, const char *name)
{
	for (int i = 0; i < p->nobserved; i++)
		if (!strcmp(p->observed[i], name))
			return;
	if (p->nobserved >= MAX_OBSERVED || strlen(name) >= sizeof p->observed[0])
		return;
	snprintf(p->observed[p->nobserved++], sizeof p->observed[0], "%s", name);
	mpv_observe_property(p->mpv, OBSERVE_PAGE, name, MPV_FORMAT_NODE);
}

/* The render thread frees mpv's render context and the textures. */
static void stop_render_thread(player *p)
{
	if (p->thread_running) {
		pthread_mutex_lock(&p->lock);
		p->quit = 1;
		pthread_cond_broadcast(&p->cond);
		pthread_mutex_unlock(&p->lock);
		pthread_join(p->thread, NULL);
		p->thread_running = 0;
	}
	if (p->gl != EGL_NO_CONTEXT) {
		eglDestroyContext(as_egl_dpy, p->gl);
		p->gl = EGL_NO_CONTEXT;
	}
	pthread_mutex_destroy(&p->lock);
	pthread_cond_destroy(&p->cond);
}

static void destroy(player *p)
{
	player **pp = &players;
	while (*pp && *pp != p)
		pp = &(*pp)->next;
	if (*pp)
		*pp = p->next;
	stop_render_thread(p);
	if (p->mpv)
		mpv_terminate_destroy(p->mpv);
	for (int i = 0; i < p->npend; i++) {
		free(p->pend_name[i]);
		free(p->pend_json[i]);
	}
	free(p->buffering_json);
	free(p);
	update_playing();
	as_request_redraw();
}

static void create(const char *id, yyjson_val *opts, reply_t *r)
{
	if (find(id)) {
		r->value = yyjson_mut_true(r->doc);
		return;
	}
	ensure_audio();
	player *p = calloc(1, sizeof *p);
	snprintf(p->id, sizeof p->id, "%s", id);
	p->paused = 0;
	p->shown = -1;
	p->gl = EGL_NO_CONTEXT;
	pthread_mutex_init(&p->lock, NULL);
	pthread_cond_init(&p->cond, NULL);
	p->mpv = mpv_create();
	if (!p->mpv) {
		stop_render_thread(p);
		free(p);
		reply_error(r, "mpv_create failed");
		return;
	}
	mpv_handle *m = p->mpv;
	mpv_set_option_string(m, "vo", "libmpv");
	mpv_set_option_string(m, "hwdec", "vaapi");
	mpv_set_option_string(m, "terminal", "no");
	mpv_set_option_string(m, "input-default-bindings", "no");
	mpv_set_option_string(m, "input-vo-keyboard", "no");
	mpv_set_option_string(m, "osc", "no");
	mpv_set_option_string(m, "idle", "yes");
	/* Frames come to the render thread up to 0.1 s early, so a heavy shader
	 * chain has time to finish before each frame is due. */
	mpv_set_option_string(m, "video-timing-offset", "0.1");
	/* Picture defaults; Seanime's custom mpv config can change them. */
	mpv_set_option_string(m, "scale", "ewa_lanczossharp");
	/* Stream in large bursts so the Wi-Fi radio can sleep in between. */
	mpv_set_option_string(m, "cache", "yes");
	mpv_set_option_string(m, "demuxer-max-bytes", "256MiB");
	mpv_set_option_string(m, "demuxer-max-back-bytes", "128MiB");
	mpv_set_option_string(m, "demuxer-readahead-secs", "300");
	char cache_dir[4096];
	snprintf(cache_dir, sizeof cache_dir, "%s/mpv-shaders", as_opts->cache_dir);
	mpv_set_option_string(m, "gpu-shader-cache-dir", cache_dir);

	yyjson_val *config = yyjson_obj_get(opts, "config");
	const char *cdir = yyjson_get_str(yyjson_obj_get(config, "directory"));
	if (yyjson_get_bool(yyjson_obj_get(config, "enabled")) && cdir) {
		mpv_set_option_string(m, "config-dir", cdir);
		mpv_set_option_string(m, "config", "yes");
	} else {
		char def[4096];
		snprintf(def, sizeof def, "%s/mpvcore", as_opts->data_dir);
		mpv_set_option_string(m, "config-dir", def);
		mpv_set_option_string(m, "config", "yes");
	}
	if (bridge_logging_enabled()) {
		char log[4096];
		bridge_log_path(log, sizeof log);
		mpv_set_option_string(m, "log-file", log);
	}

	yyjson_val *o = yyjson_obj_get(opts, "options");
	size_t idx, max;
	yyjson_val *k, *v;
	if (yyjson_is_obj(o)) {
		yyjson_obj_foreach(o, idx, max, k, v) {
			char val[4096];
			const char *name = yyjson_get_str(k);
			if (!name || forbidden(name) || json_to_option_string(v, val, sizeof val))
				continue;
			if (mpv_set_option_string(m, name, val) < 0)
				fprintf(stderr, "appshell: mpv option %s=%s rejected\n", name, val);
		}
	}
	yyjson_val *shaders = yyjson_obj_get(opts, "shaders");
	if (yyjson_is_arr(shaders) && yyjson_arr_size(shaders)) {
		char list[16384] = "";
		yyjson_val *s;
		yyjson_arr_foreach(shaders, idx, max, s) {
			if (!yyjson_is_str(s))
				continue;
			if (list[0])
				strncat(list, ":", sizeof list - strlen(list) - 1);
			strncat(list, yyjson_get_str(s), sizeof list - strlen(list) - 1);
		}
		mpv_set_option_string(m, "glsl-shaders", list);
	}

	if (as_debug)
		mpv_request_log_messages(m, "warn");
	if (mpv_initialize(m) < 0) {
		mpv_terminate_destroy(m);
		stop_render_thread(p);
		free(p);
		reply_error(r, "mpv_initialize failed");
		return;
	}
	mpv_set_wakeup_callback(m, on_wakeup, p);

	p->gl = window_create_shared_context();
	int started = -1;
	if (p->gl != EGL_NO_CONTEXT && pthread_create(&p->thread, NULL, render_main, p) == 0) {
		p->thread_running = 1;
		pthread_mutex_lock(&p->lock);
		while (!p->started)
			pthread_cond_wait(&p->cond, &p->lock);
		started = p->started;
		pthread_mutex_unlock(&p->lock);
	}
	if (started < 0) {
		stop_render_thread(p);
		mpv_terminate_destroy(m);
		free(p);
		reply_error(r, "mpv render context failed");
		return;
	}

	mpv_observe_property(m, OBSERVE_INTERNAL, "pause", MPV_FORMAT_NODE);
	mpv_observe_property(m, OBSERVE_INTERNAL, "core-idle", MPV_FORMAT_NODE);
	mpv_observe_property(m, OBSERVE_INTERNAL, "paused-for-cache", MPV_FORMAT_NODE);
	mpv_observe_property(m, OBSERVE_INTERNAL, "current-ao", MPV_FORMAT_NODE);
	/* @mpv-prism/core counts these as observed without asking for them: its
	 * native addon watches them on creation (MpvPrismPlayer.js, 0.1.9). */
	static const char *const prism_defaults[] = {
		"time-pos", "duration", "pause", "speed", "volume", "mute", "idle-active",
		"core-idle", "eof-reached", "demuxer-cache-state", "cache-buffering-state",
		"frame-drop-count", "decoder-frame-drop-count", "mistimed-frame-count",
		"vo-delayed-frame-count", "track-list", "aid", "sid", "vid", "sub-text",
		"sub-start", "sub-end", "sub-visibility", "video-params", "audio-params",
		"path", "media-title", "percent-pos", "avsync", NULL};
	for (int i = 0; prism_defaults[i]; i++)
		observe(p, prism_defaults[i]);
	yyjson_val *obs = yyjson_obj_get(opts, "observe");
	yyjson_val *s;
	if (yyjson_is_arr(obs))
		yyjson_arr_foreach(obs, idx, max, s)
			if (yyjson_is_str(s))
				observe(p, yyjson_get_str(s));

	p->next = players;
	players = p;
	r->value = yyjson_mut_true(r->doc);
}

/*
 * Requests run asynchronously. With the render API, some mpv calls (shader,
 * deband and other video-output changes) wait for the render thread, which is
 * this thread: a synchronous call from here deadlocked the whole window. The
 * page's promise is answered when mpv's reply event arrives
 * (player_process_events).
 */
static uint64_t next_token = REPLY_BASE;

typedef struct pending_reply {
	uint64_t token;
	int64_t id;
	char what[64];
	struct pending_reply *next;
} pending_reply;
static pending_reply *pending_replies;

static uint64_t defer(reply_t *r, const char *what)
{
	uint64_t token = next_token++;
	if (r->id > 0) {
		pending_reply *pr = calloc(1, sizeof *pr);
		pr->token = token;
		pr->id = r->id;
		snprintf(pr->what, sizeof pr->what, "%s", what);
		pr->next = pending_replies;
		pending_replies = pr;
		r->deferred = 1;
	}
	return token;
}

static void answer(uint64_t token, int error, const char *value_json)
{
	for (pending_reply **pp = &pending_replies; *pp; pp = &(*pp)->next) {
		pending_reply *pr = *pp;
		if (pr->token != token)
			continue;
		*pp = pr->next;
		if (error < 0) {
			char msg[200];
			snprintf(msg, sizeof msg, "%s: %s", pr->what, mpv_error_string(error));
			bridge_send_reply(pr->id, NULL, msg);
		} else {
			bridge_send_reply(pr->id, value_json, NULL);
		}
		free(pr);
		return;
	}
}

static void command_strings(player *p, reply_t *r, const char **argv)
{
	int err = mpv_command_async(p->mpv, defer(r, argv[0]), argv);
	if (err < 0) {
		r->deferred = 0;
		reply_error(r, mpv_error_string(err));
	}
}

static void set_string(player *p, reply_t *r, const char *name, const char *value)
{
	char *v = (char *)value;
	int err = mpv_set_property_async(p->mpv, defer(r, name), name, MPV_FORMAT_STRING, &v);
	if (err < 0) {
		r->deferred = 0;
		char msg[200];
		snprintf(msg, sizeof msg, "%s: %s", name, mpv_error_string(err));
		reply_error(r, msg);
	}
}

int player_handle_request(const char *ch, void *args_v, void *reply_v)
{
	yyjson_val *args = args_v;
	reply_t *r = reply_v;
	const char *op = ch + strlen("mpv-prism:");
	const char *id = arg_str(args, 0);

	if (!strcmp(op, "get-linux-backend-diagnostics")) {
		yyjson_mut_val *o = yyjson_mut_obj(r->doc);
		yyjson_mut_obj_add_bool(r->doc, o, "linuxAvailable", 1);
		yyjson_mut_obj_add_str(r->doc, o, "selectedBackend", "egl");
		yyjson_mut_obj_add_str(r->doc, o, "eglPlatform", "x11");
		yyjson_mut_obj_add_bool(r->doc, o, "angleAvailable", 0);
		yyjson_mut_obj_add_strcpy(r->doc, o, "glRenderer", (const char *)glGetString(GL_RENDERER));
		yyjson_mut_obj_add_strcpy(r->doc, o, "glVersion", (const char *)glGetString(GL_VERSION));
		yyjson_mut_obj_add_strcpy(r->doc, o, "glVendor", (const char *)glGetString(GL_VENDOR));
		yyjson_mut_obj_add_str(r->doc, o, "gpuProbeStatus", "verified-angle");
		r->value = o;
		return 0;
	}
	if (!id) {
		reply_error(r, "invalid player id");
		return 0;
	}
	if (strcmp(op, "get-property") && strcmp(op, "observe-property"))
		AS_LOG("mpv-prism %s %s\n", op, id);
	if (!strcmp(op, "create")) {
		create(id, yyjson_arr_get(args, 1), r);
		return 0;
	}
	player *p = find(id);
	if (!strcmp(op, "destroy") || !strcmp(op, "cancel-create")) {
		if (p)
			destroy(p);
		return 0;
	}
	if (!p) {
		reply_error(r, "mpv-prism player is not registered for this renderer");
		return 0;
	}

	if (!strcmp(op, "load") || !strcmp(op, "play"))
		ensure_audio();
	if (!strcmp(op, "load")) {
		const char *uri = arg_str(args, 1);
		const char *argv[] = {"loadfile", uri ? uri : "", "replace", NULL};
		command_strings(p, r, argv);
	} else if (!strcmp(op, "play")) {
		set_string(p, r, "pause", "no");
	} else if (!strcmp(op, "pause")) {
		set_string(p, r, "pause", "yes");
	} else if (!strcmp(op, "stop")) {
		const char *argv[] = {"stop", NULL};
		command_strings(p, r, argv);
	} else if (!strcmp(op, "seek")) {
		char t[64];
		snprintf(t, sizeof t, "%.6f", arg_num(args, 1, 0));
		const char *mode = arg_str(args, 2);
		const char *argv[] = {"seek", t, mode && *mode ? mode : "absolute", NULL};
		command_strings(p, r, argv);
	} else if (!strcmp(op, "set-speed")) {
		char t[64];
		snprintf(t, sizeof t, "%.6f", arg_num(args, 1, 1));
		set_string(p, r, "speed", t);
	} else if (!strcmp(op, "select-track")) {
		const char *kind = arg_str(args, 1);
		const char *prop = !kind ? NULL : !strcmp(kind, "audio") ? "aid"
			: !strcmp(kind, "subtitle") ? "sid" : !strcmp(kind, "video") ? "vid" : NULL;
		yyjson_val *sel = yyjson_arr_get(args, 2);
		char val[128];
		if (!prop) {
			reply_error(r, "invalid track kind");
		} else {
			if (!sel || yyjson_is_null(sel) || (yyjson_is_bool(sel) && !yyjson_get_bool(sel)))
				snprintf(val, sizeof val, "no");
			else if (yyjson_is_bool(sel))
				snprintf(val, sizeof val, "auto");
			else
				json_to_option_string(sel, val, sizeof val);
			set_string(p, r, prop, val);
		}
	} else if (!strcmp(op, "set-property")) {
		const char *name = arg_str(args, 1);
		char val[8192];
		if (!name || json_to_option_string(yyjson_arr_get(args, 2), val, sizeof val))
			reply_error(r, "invalid property");
		else
			set_string(p, r, name, val);
	} else if (!strcmp(op, "set-shaders")) {
		yyjson_val *paths = yyjson_arr_get(args, 1);
		char list[16384] = "";
		size_t idx, max;
		yyjson_val *s;
		if (yyjson_is_arr(paths))
			yyjson_arr_foreach(paths, idx, max, s) {
				if (!yyjson_is_str(s))
					continue;
				if (list[0])
					strncat(list, ":", sizeof list - strlen(list) - 1);
				strncat(list, yyjson_get_str(s), sizeof list - strlen(list) - 1);
			}
		if (list[0]) {
			const char *argv[] = {"change-list", "glsl-shaders", "set", list, NULL};
			command_strings(p, r, argv);
		} else {
			const char *argv[] = {"change-list", "glsl-shaders", "clr", "", NULL};
			command_strings(p, r, argv);
		}
	} else if (!strcmp(op, "get-property")) {
		const char *name = arg_str(args, 1);
		if (!name || mpv_get_property_async(p->mpv, defer(r, name), name, MPV_FORMAT_NODE) < 0) {
			r->deferred = 0;
			r->value = yyjson_mut_null(r->doc);
		}
	} else if (!strcmp(op, "observe-property")) {
		const char *name = arg_str(args, 1);
		if (name)
			observe(p, name);
	} else if (!strcmp(op, "command")) {
		yyjson_val *a = yyjson_arr_get(args, 1);
		size_t n = yyjson_arr_size(a);
		if (!n || n > 64) {
			reply_error(r, "invalid command arguments");
		} else {
			char bufs[64][256];
			const char *argv[65];
			size_t i = 0, idx, max;
			yyjson_val *s;
			yyjson_arr_foreach(a, idx, max, s) {
				if (yyjson_is_str(s)) {
					argv[i] = yyjson_get_str(s);
				} else {
					json_to_option_string(s, bufs[i], sizeof bufs[i]);
					argv[i] = bufs[i];
				}
				i++;
			}
			argv[i] = NULL;
			command_strings(p, r, argv);
		}
	} else if (!strcmp(op, "start-video-presenter")) {
		p->presenter = 1;
		p->suspended = 0;
		as_request_redraw();
	} else if (!strcmp(op, "stop-video-presenter")) {
		p->presenter = 0;
		p->has_rect = 0;
		as_request_redraw();
	} else if (!strcmp(op, "suspend-video-presenter") || !strcmp(op, "resume-video-presenter")) {
		/* Ignored: mpv-prism suspends its web presenter when its <video> gets
		 * no frames for a while ("stall": after a pause or a slow start) and
		 * resumes only when frames reach it again. We draw natively, so they
		 * never would and the picture stayed black. Visibility is handled
		 * here (window_swap, on_screen). */
		as_request_redraw();
	} else if (!strcmp(op, "presenter-rect")) {
		yyjson_val *rect = yyjson_arr_get(args, 2);
		if (!yyjson_is_obj(rect)) {
			p->has_rect = 0;
		} else {
			p->has_rect = 1;
			p->rx = yyjson_get_num(yyjson_obj_get(rect, "left"));
			p->ry = yyjson_get_num(yyjson_obj_get(rect, "top"));
			p->rw = yyjson_get_num(yyjson_obj_get(rect, "width"));
			p->rh = yyjson_get_num(yyjson_obj_get(rect, "height"));
			p->rect_visible = yyjson_get_bool(yyjson_obj_get(rect, "visible"));
		}
		AS_LOG("presenter rect %s: has=%d visible=%d %.0f,%.0f %.0fx%.0f presenter=%d suspended=%d\n", p->id,
		       p->has_rect, p->rect_visible, p->rx, p->ry, p->rw, p->rh, p->presenter, p->suspended);
		as_request_redraw();
	} else {
		reply_error(r, "unknown mpv-prism call");
	}
	return 0;
}

static void reload_audio_outputs(void)
{
	for (player *p = players; p; p = p->next) {
		const char *argv[] = {"ao-reload", NULL};
		mpv_command_async(p->mpv, 0, argv);
	}
}

void player_destroy_all(void)
{
	while (players)
		destroy(players);
}
