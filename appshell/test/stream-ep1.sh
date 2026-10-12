#!/bin/sh
# End to end: Frieren -> episode 1 -> SubsPlease batch -> file 01 -> Stream.
set -e
cd "$(dirname "$0")"
node open-entry.mjs ${1:-154587}
node ep1-search.mjs > /dev/null
node pick-torrent.mjs "${2:-[SubsPlease] Sousou no Frieren (01-28)}" 2000 > /dev/null
node click-button.mjs "Stream" 9000 > /dev/null
node pick-torrent.mjs "${3:-Sousou no Frieren - 01 (1080p)}" 1000 > /dev/null
node click-button.mjs "Stream" ${4:-15000} | tail -1 | cut -c1-300
