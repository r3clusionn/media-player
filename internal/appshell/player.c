/*
 * player.c: libmpv players behind window.mpvPrism.
 *
 * Each player renders, through libmpv's render API, into an offscreen texture
 * the size of its presenter's rectangle on the page (so mpv's scalers and
 * shaders work at the real display size), which the compositor then draws
 * under the page. Property changes are batched per wakeup and sent to the page
 * as mpv-prism events; the screen saver is held off while anything plays.
 */
#include "appshell.h"
#include "bridge.h"

#include <mpv/client.h>
#include <mpv/render.h>
#include <mpv/render_gl.h>

#include <math.h>
#include <stdatomic.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

#define OBSERVE_PAGE 1     /* reply_userdata of properties the page observes */
#define OBSERVE_INTERNAL 2 /* ours: pause, core-idle */
#define MAX_OBSERVED 96

typedef struct player {
	char id[128];
	mpv_handle *mpv;
	mpv_render_context *rc;
	atomic_int update;    /* set by mpv's render update callback */
	int frame_pending;    /* mpv has a new frame to show */
	int rendered;         /* rendered in the current frame (report swap) */
	int presenter;        /* a presenter is attached */
	int has_rect, rect_visible, suspended;
	double rx, ry, rw, rh; /* presenter rectangle, DIP */
	as_target target;
	int target_valid;       /* target holds the current frame at its size */
	char observed[MAX_OBSERVED][64];
	int nobserved;
	int paused, idle, loaded;
	struct player *next;
} player;

static player *players;

static player *find(const char *id)
{
	for (player *p = players; p; p = p->next)
		if (id && !strcmp(p->id, id))
			return p;
	return NULL;
}

static void *get_proc(void *ctx, const char *name)
{
	(void)ctx;
	return (void *)eglGetProcAddress(name);
}

static void on_update(void *ctx)
{
	player *p = ctx;
	atomic_store(&p->update, 1);
	as_wake();
}

static void on_wakeup(void *ctx)
{
	(void)ctx;
	as_wake();
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

static void send_event(player *p, yyjson_mut_doc *d, yyjson_mut_val *ev)
{
	yyjson_mut_val *o = yyjson_mut_obj(d);
	yyjson_mut_obj_add_str(d, o, "event", "mpv-prism:event");
	yyjson_mut_val *data = yyjson_mut_obj_add_obj(d, o, "data");
	yyjson_mut_obj_add_strcpy(d, data, "playerId", p->id);
	yyjson_mut_obj_add_val(d, data, "event", ev);
	char *s = yyjson_mut_val_write(o, 0, NULL);
	if (s) {
		as_page_send_raw(s);
		free(s);
	}
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
	for (player *p = players; p; p = p->next) {
		/* Latest value per property; sent once per batch (time-pos changes every frame). */
		yyjson_mut_doc *d = yyjson_mut_doc_new(NULL);
		const char *names[MAX_OBSERVED];
		yyjson_mut_val *values[MAX_OBSERVED];
		int nprops = 0;
		for (;;) {
			mpv_event *e = mpv_wait_event(p->mpv, 0);
			if (e->event_id == MPV_EVENT_NONE)
				break;
			yyjson_mut_val *ev = NULL;
			switch (e->event_id) {
			case MPV_EVENT_PROPERTY_CHANGE: {
				mpv_event_property *pr = e->data;
				if (e->reply_userdata == OBSERVE_INTERNAL) {
					int flag = pr->format == MPV_FORMAT_NODE &&
						   ((mpv_node *)pr->data)->format == MPV_FORMAT_FLAG &&
						   ((mpv_node *)pr->data)->u.flag;
					if (!strcmp(pr->name, "pause")) p->paused = flag;
					else if (!strcmp(pr->name, "core-idle")) p->idle = flag;
					update_playing();
					break;
				}
				yyjson_mut_val *v = pr->format == MPV_FORMAT_NODE
					? node_to_json(d, pr->data) : yyjson_mut_null(d);
				int i;
				for (i = 0; i < nprops; i++)
					if (!strcmp(names[i], pr->name))
						break;
				if (i == nprops && nprops < MAX_OBSERVED)
					names[nprops++] = yyjson_mut_get_str(yyjson_mut_strcpy(d, pr->name));
				if (i < MAX_OBSERVED)
					values[i] = v;
				break;
			}
			case MPV_EVENT_FILE_LOADED:
				p->loaded = 1;
				ev = yyjson_mut_obj(d);
				yyjson_mut_obj_add_str(d, ev, "type", "file-loaded");
				update_playing();
				break;
			case MPV_EVENT_PLAYBACK_RESTART:
				ev = yyjson_mut_obj(d);
				yyjson_mut_obj_add_str(d, ev, "type", "playback-restart");
				break;
			case MPV_EVENT_END_FILE: {
				mpv_event_end_file *ef = e->data;
				p->loaded = 0;
				ev = yyjson_mut_obj(d);
				yyjson_mut_obj_add_str(d, ev, "type", "end-file");
				yyjson_mut_obj_add_str(d, ev, "reason", end_reason(ef->reason));
				yyjson_mut_obj_add_int(d, ev, "numericReason", ef->reason);
				if (ef->reason == MPV_END_FILE_REASON_ERROR)
					yyjson_mut_obj_add_strcpy(d, ev, "error", mpv_error_string(ef->error));
				update_playing();
				break;
			}
			case MPV_EVENT_SHUTDOWN:
				ev = yyjson_mut_obj(d);
				yyjson_mut_obj_add_str(d, ev, "type", "shutdown");
				break;
			default:
				break;
			}
			if (ev) {
				/* Keep order: flush pending properties before a state event. */
				for (int i = 0; i < nprops; i++) {
					yyjson_mut_val *pe = yyjson_mut_obj(d);
					yyjson_mut_obj_add_str(d, pe, "type", "property-change");
					yyjson_mut_obj_add_str(d, pe, "name", names[i]);
					yyjson_mut_obj_add_val(d, pe, "value", values[i]);
					send_event(p, d, pe);
				}
				nprops = 0;
				send_event(p, d, ev);
			}
		}
		for (int i = 0; i < nprops; i++) {
			yyjson_mut_val *pe = yyjson_mut_obj(d);
			yyjson_mut_obj_add_str(d, pe, "type", "property-change");
			yyjson_mut_obj_add_str(d, pe, "name", names[i]);
			yyjson_mut_obj_add_val(d, pe, "value", values[i]);
			send_event(p, d, pe);
		}
		yyjson_mut_doc_free(d);

		if (atomic_exchange(&p->update, 0) && p->rc &&
		    (mpv_render_context_update(p->rc) & MPV_RENDER_UPDATE_FRAME)) {
			p->frame_pending = 1;
			as_request_redraw();
		}
	}
}

/* ---- rendering ------------------------------------------------------- */

static int on_screen(const player *p)
{
	return p->rc && p->presenter && p->has_rect && p->rect_visible && !p->suspended &&
	       p->rw >= 1 && p->rh >= 1;
}

int player_any_frame_ready(void)
{
	for (player *p = players; p; p = p->next)
		if (p->frame_pending)
			return 1;
	return 0;
}

void player_draw_all(void)
{
	double s = as_scale;
	for (player *p = players; p; p = p->next) {
		p->rendered = 0;
		if (!on_screen(p)) {
			p->frame_pending = 0;
			continue;
		}
		int x = (int)lround(p->rx * s), y = (int)lround(p->ry * s);
		int w = (int)lround(p->rw * s), h = (int)lround(p->rh * s);
		if (p->target.w != w || p->target.h != h)
			p->target_valid = 0;
		compositor_target_size(&p->target, w, h);
		/* Render only for a new frame or a new size: with Anime4K a render
		 * costs ~20 ms of GPU, and the page repaints far more often. */
		if (p->frame_pending || !p->target_valid) {
			mpv_opengl_fbo fbo = {.fbo = (int)p->target.fbo, .w = w, .h = h};
			int flip = 0; /* texture row 0 = top of the image */
			mpv_render_param rp[] = {
				{MPV_RENDER_PARAM_OPENGL_FBO, &fbo},
				{MPV_RENDER_PARAM_FLIP_Y, &flip},
				{0, NULL},
			};
			mpv_render_context_render(p->rc, rp);
			p->rendered = 1;
			p->frame_pending = 0;
			p->target_valid = 1;
		}
		glBindFramebuffer(GL_FRAMEBUFFER, 0);
		glViewport(0, 0, as_win_w, as_win_h);
		compositor_blit_target(&p->target, x, y, w, h);
	}
}

void player_report_swap(void)
{
	for (player *p = players; p; p = p->next)
		if (p->rendered && p->rc)
			mpv_render_context_report_swap(p->rc);
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

static void destroy(player *p)
{
	player **pp = &players;
	while (*pp && *pp != p)
		pp = &(*pp)->next;
	if (*pp)
		*pp = p->next;
	if (p->rc)
		mpv_render_context_free(p->rc);
	if (p->mpv)
		mpv_terminate_destroy(p->mpv);
	if (p->target.tex) {
		glDeleteTextures(1, &p->target.tex);
		glDeleteFramebuffers(1, &p->target.fbo);
	}
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
	player *p = calloc(1, sizeof *p);
	snprintf(p->id, sizeof p->id, "%s", id);
	p->paused = 0;
	p->mpv = mpv_create();
	if (!p->mpv) {
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
	mpv_set_option_string(m, "keep-open", "yes");
	/* Stream in large bursts so the Wi-Fi radio can sleep in between. */
	mpv_set_option_string(m, "cache", "yes");
	mpv_set_option_string(m, "demuxer-max-bytes", "512MiB");
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

	if (mpv_initialize(m) < 0) {
		mpv_terminate_destroy(m);
		free(p);
		reply_error(r, "mpv_initialize failed");
		return;
	}
	mpv_set_wakeup_callback(m, on_wakeup, p);

	mpv_opengl_init_params gl = {.get_proc_address = get_proc};
	int advanced = 1;
	mpv_render_param params[] = {
		{MPV_RENDER_PARAM_API_TYPE, (void *)MPV_RENDER_API_TYPE_OPENGL},
		{MPV_RENDER_PARAM_OPENGL_INIT_PARAMS, &gl},
		{MPV_RENDER_PARAM_X11_DISPLAY, as_dpy},
		{MPV_RENDER_PARAM_ADVANCED_CONTROL, &advanced},
		{0, NULL},
	};
	if (mpv_render_context_create(&p->rc, m, params) < 0) {
		mpv_terminate_destroy(m);
		free(p);
		reply_error(r, "mpv render context failed");
		return;
	}
	mpv_render_context_set_update_callback(p->rc, on_update, p);

	mpv_observe_property(m, OBSERVE_INTERNAL, "pause", MPV_FORMAT_NODE);
	mpv_observe_property(m, OBSERVE_INTERNAL, "core-idle", MPV_FORMAT_NODE);
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

static void command_strings(player *p, reply_t *r, const char **argv)
{
	int err = mpv_command(p->mpv, argv);
	if (err < 0)
		reply_error(r, mpv_error_string(err));
}

static void set_string(player *p, reply_t *r, const char *name, const char *value)
{
	int err = mpv_set_property_string(p->mpv, name, value);
	if (err < 0) {
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
		mpv_node n;
		if (name && mpv_get_property(p->mpv, name, MPV_FORMAT_NODE, &n) >= 0) {
			r->value = node_to_json(r->doc, &n);
			mpv_free_node_contents(&n);
		} else {
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
	} else if (!strcmp(op, "suspend-video-presenter")) {
		p->suspended = 1;
		as_request_redraw();
	} else if (!strcmp(op, "resume-video-presenter")) {
		p->suspended = 0;
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
		as_request_redraw();
	} else {
		reply_error(r, "unknown mpv-prism call");
	}
	return 0;
}

void player_destroy_all(void)
{
	while (players)
		destroy(players);
}
