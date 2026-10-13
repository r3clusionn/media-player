/*
 * stats.c: frame timing counters for tests (the "appshell:stats" call).
 *
 * What the screen shows is what matters, so this measures on the main
 * thread: how long the loop goes without running (input and CEF wait that
 * long), how many swaps happen, how long a page frame from CEF waits until a
 * swap puts it on screen, and how many video frames are shown or skipped.
 * Each call returns the numbers since the previous one.
 */
#include "appshell.h"
#include "bridge.h"

#include <stdlib.h>

#define MAX_SAMPLES 8192

static double since, last_tick, paint_at;
static double loop_max;
static int loop_over20, swaps, video_shown, video_skipped;
static float lat[MAX_SAMPLES];
static int nlat;

void stats_loop_tick(void)
{
	double now = as_now_ms();
	if (last_tick > 0) {
		double gap = now - last_tick;
		if (gap > loop_max)
			loop_max = gap;
		if (gap > 20)
			loop_over20++;
	}
	last_tick = now;
}

/* The loop sleeps in poll() on purpose; that isn't a stall. */
void stats_loop_idle(void) { last_tick = 0; }

void stats_page_painted(void)
{
	if (paint_at == 0)
		paint_at = as_now_ms();
}

void stats_swapped(void)
{
	swaps++;
	if (paint_at > 0) {
		if (nlat < MAX_SAMPLES)
			lat[nlat++] = (float)(as_now_ms() - paint_at);
		paint_at = 0;
	}
}

void stats_video(int shown, int skipped)
{
	video_shown += shown;
	video_skipped += skipped;
}

static int cmpf(const void *a, const void *b)
{
	float x = *(const float *)a, y = *(const float *)b;
	return x < y ? -1 : x > y;
}

void *stats_take(void *doc_v)
{
	yyjson_mut_doc *d = doc_v;
	double now = as_now_ms();
	yyjson_mut_val *o = yyjson_mut_obj(d);
	yyjson_mut_obj_add_real(d, o, "seconds", since > 0 ? (now - since) / 1000 : 0);
	yyjson_mut_obj_add_int(d, o, "swaps", swaps);
	yyjson_mut_obj_add_real(d, o, "loopMaxMs", loop_max);
	yyjson_mut_obj_add_int(d, o, "loopOver20ms", loop_over20);
	yyjson_mut_obj_add_int(d, o, "videoShown", video_shown);
	yyjson_mut_obj_add_int(d, o, "videoSkipped", video_skipped);
	qsort(lat, nlat, sizeof lat[0], cmpf);
	yyjson_mut_val *l = yyjson_mut_obj(d);
	yyjson_mut_obj_add_int(d, l, "n", nlat);
	yyjson_mut_obj_add_real(d, l, "p50", nlat ? lat[nlat / 2] : 0);
	yyjson_mut_obj_add_real(d, l, "p95", nlat ? lat[nlat * 95 / 100] : 0);
	yyjson_mut_obj_add_real(d, l, "max", nlat ? lat[nlat - 1] : 0);
	yyjson_mut_obj_add_val(d, o, "pageToScreenMs", l);
	since = now;
	loop_max = 0;
	loop_over20 = swaps = video_shown = video_skipped = nlat = 0;
	return o;
}
