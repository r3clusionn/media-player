#!/bin/sh
# Starts the built media-player with the test hooks on (DevTools on 9333,
# SIGUSR1 screenshots) and waits until DevTools answers.
# Output: $OUT/mp.log, screenshots via shot.sh. OUT defaults to /tmp/mp-test.
OUT=${OUT:-/tmp/mp-test}
mkdir -p "$OUT"
pkill -x media-player
for i in $(seq 1 20); do pgrep -x media-player >/dev/null || break; sleep 0.25; done
pkill -9 -x media-player 2>/dev/null; sleep 0.3
cd "$(dirname "$0")/../../build/dist"
DISPLAY=${DISPLAY:-:0} APPSHELL_DEBUG=1 APPSHELL_SHOT="$OUT/mp.ppm" APPSHELL_REMOTE_DEBUG=${CDP_PORT:-9333} \
	setsid ./media-player > "$OUT/mp.log" 2>&1 < /dev/null &
for i in $(seq 1 80); do curl -s -m 2 http://127.0.0.1:${CDP_PORT:-9333}/json >/dev/null 2>&1 && exit 0; sleep 0.25; done
echo "media-player did not start; see $OUT/mp.log" >&2
exit 1
