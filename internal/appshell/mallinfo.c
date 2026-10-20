/*
 * mallinfo.c: glibc's mallinfo() for CEF, without the int overflow.
 *
 * Chromium's memory dump provider (the MemoryInfra thread, on a timer) reads
 * glibc's heap totals with the old mallinfo(), whose fields are ints, and
 * checks they aren't negative. Once a process has 2 GiB of malloc arenas the
 * sum wraps and the check kills it: mpv's demuxer cache of each stream stays
 * in glibc's arenas, and the sixth BD remux of a session crashed the app.
 * This definition comes first in the symbol lookup, so libcef.so gets it;
 * the totals come from mallinfo2() and stop at INT_MAX.
 *
 * Linked into media-player and appshell-helper (CEF's other processes).
 */
#include <limits.h>
#include <malloc.h>

static int clamp(size_t v)
{
	return v > INT_MAX ? INT_MAX : (int)v;
}

struct mallinfo mallinfo(void)
{
	struct mallinfo2 m = mallinfo2();
	return (struct mallinfo){
		.arena = clamp(m.arena),
		.ordblks = clamp(m.ordblks),
		.smblks = clamp(m.smblks),
		.hblks = clamp(m.hblks),
		.hblkhd = clamp(m.hblkhd),
		.usmblks = clamp(m.usmblks),
		.fsmblks = clamp(m.fsmblks),
		.uordblks = clamp(m.uordblks),
		.fordblks = clamp(m.fordblks),
		.keepcost = clamp(m.keepcost),
	};
}
