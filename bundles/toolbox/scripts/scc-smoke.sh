#!/bin/sh
set -eu
work=$(mktemp -d "${TMPDIR:-/tmp}/bundledock-scc.XXXXXX")
trap 'rm -rf "$work"' EXIT HUP INT TERM
mkdir -p "$work/.config" "$work/.cache" "$work/.local/share"
# Version only: do not probe the host, load user configuration or access the network.
env -i PATH="$PATH" HOME="$work" XDG_CONFIG_HOME="$work/.config" \
    XDG_CACHE_HOME="$work/.cache" XDG_DATA_HOME="$work/.local/share" \
    sh -eu -c '
        cd "$HOME"
        version=$(scc --version)
        printf "%s\n" "$version"
        case "$version" in "scc version "[0-9]*.[0-9]*.[0-9]*) ;; *) exit 1 ;; esac
    '
