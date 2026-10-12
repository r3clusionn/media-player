#!/bin/sh
# Saves the app's composited frame (page and video) as a half-size PNG: shot.sh out.png
OUT=${OUT:-/tmp/mp-test}
rm -f "$OUT/mp.ppm"
kill -USR1 "$(pgrep -x media-player)" || exit 1
for i in $(seq 1 50); do [ -s "$OUT/mp.ppm" ] && break; sleep 0.1; done
ffmpeg -loglevel error -y -i "$OUT/mp.ppm" -vf scale=iw/2:-1 "$1"
