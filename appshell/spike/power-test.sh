#!/bin/sh
# power-test.sh - battery draw of the phase 0 workloads, one after another.
# Unplug, backlight at 70%, hands off. Each case: start it, wait 10 s, sample
# power_now every second for 45 s, stop it. Prints average and median watts.
# Usage: power-test.sh RUNTIME_DIR TESTCLIP ANIME4K_DIR [seanime URL]
set -u
R=$1 CLIP=$2 A4K=$3 URL=${4:-}
B=/sys/class/power_supply/BAT0
[ "$(cat /sys/class/power_supply/ACAD/online)" = 0 ] || { echo "unplug the charger first" >&2; exit 1; }
export DISPLAY=${DISPLAY:-:0}

sample() { # $1 = label
	sleep 10
	i=0
	: > /tmp/power-test.$$
	while [ $i -lt 45 ]; do cat $B/power_now >> /tmp/power-test.$$; sleep 1; i=$((i + 1)); done
	sort -n /tmp/power-test.$$ | awk -v l="$1" '{v[NR]=$1; s+=$1} END {printf "%-34s avg %5.2f W  median %5.2f W  (%d samples)\n", l, s/NR/1e6, v[int((NR+1)/2)]/1e6, NR}'
	rm -f /tmp/power-test.$$
}

run() { # $1 = label, rest = command
	l=$1; shift
	"$@" > /dev/null 2>&1 &
	pid=$!
	sample "$l"
	kill $pid 2>/dev/null; wait $pid 2>/dev/null
	sleep 3
}

hq="$A4K/Anime4K_Clamp_Highlights.glsl:$A4K/Anime4K_Restore_CNN_VL.glsl:$A4K/Anime4K_Upscale_CNN_x2_VL.glsl:$A4K/Anime4K_Restore_CNN_M.glsl:$A4K/Anime4K_AutoDownscalePre_x2.glsl:$A4K/Anime4K_AutoDownscalePre_x4.glsl:$A4K/Anime4K_Upscale_CNN_x2_M.glsl"
fast="$A4K/Anime4K_Clamp_Highlights.glsl:$A4K/Anime4K_Restore_CNN_M.glsl:$A4K/Anime4K_Upscale_CNN_x2_M.glsl:$A4K/Anime4K_Restore_CNN_S.glsl:$A4K/Anime4K_AutoDownscalePre_x2.glsl:$A4K/Anime4K_AutoDownscalePre_x4.glsl:$A4K/Anime4K_Upscale_CNN_x2_S.glsl"
deband="--deband=yes --deband-iterations=2 --deband-threshold=35 --deband-range=16 --deband-grain=4"

echo "backlight $(( 100 * $(cat /sys/class/backlight/*/brightness | head -1) / $(cat /sys/class/backlight/*/max_brightness | head -1) ))%, battery $(cat $B/capacity)%, profile $(cat /sys/class/platform-profile/*/profile | tr '\n' ' ')"
run "idle (nothing started)" sleep 60
run "embedded mpv: plain + deband" env SPIKE_FILE="$CLIP" SPIKE_A4K=none SPIKE_SECONDS=60 SPIKE_VSYNC=1 SPIKE_SHADERS="$A4K" "$R/mpv_spike"
run "embedded mpv: A+A (Fast) + deband" env SPIKE_FILE="$CLIP" SPIKE_A4K=aafast SPIKE_SECONDS=60 SPIKE_VSYNC=1 SPIKE_SHADERS="$A4K" "$R/mpv_spike"
run "embedded mpv: A+A (HQ) + deband" env SPIKE_FILE="$CLIP" SPIKE_A4K=aa SPIKE_SECONDS=60 SPIKE_VSYNC=1 SPIKE_SHADERS="$A4K" "$R/mpv_spike"
run "mpv program: A+A (Fast), gpu-next" mpv --no-config --fs --loop-file=inf --vo=gpu-next --hwdec=vaapi $deband --glsl-shaders="$fast" "$CLIP"
run "mpv program: plain, your mpv.conf" mpv --fs --loop-file=inf "$CLIP"
if [ -n "$URL" ]; then
	run "CEF window: Seanime Discover" env SPIKE_URL="$URL" SPIKE_SECONDS=60 "$R/osr_spike"
fi
