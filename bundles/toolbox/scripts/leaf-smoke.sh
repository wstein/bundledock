#!/bin/sh
set -eu
leaf=${1:-leaf}
work=$(mktemp -d "${TMPDIR:-/tmp}/bundledock-leaf-smoke.XXXXXX")
trap 'rm -rf "$work"' EXIT HUP INT TERM
mkdir -p "$work/home" "$work/config" "$work/cache" "$work/data"
# Clear caller overrides; Leaf may read configuration even in inline mode.
env -i PATH="$PATH" HOME="$work/home" XDG_CONFIG_HOME="$work/config" \
    XDG_CACHE_HOME="$work/cache" XDG_DATA_HOME="$work/data" TERM=dumb \
    sh -eu -c '
        cd "$1"
        leaf=$2
        "$leaf" --version
        printf "%s\n" "# bundledock-leaf-smoke" > preview.md
        "$leaf" --inline plain "$PWD/preview.md" > rendered.txt
        grep -qx "bundledock-leaf-smoke" rendered.txt
    ' sh "$work" "$leaf"
