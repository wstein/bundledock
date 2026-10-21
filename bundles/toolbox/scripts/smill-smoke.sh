#!/bin/sh
set -eu
work=$(mktemp -d "${TMPDIR:-/tmp}/bundledock-smill.XXXXXX")
trap 'rm -rf "$work"' EXIT HUP INT TERM
mkdir -p "$work/.config" "$work/.cache" "$work/.local/share"
# Version only: smill has no --version flag, so use its version subcommand. Do not
# probe the host, load user configuration, start the server or access the network.
env -i PATH="$PATH" HOME="$work" XDG_CONFIG_HOME="$work/.config" \
    XDG_CACHE_HOME="$work/.cache" XDG_DATA_HOME="$work/.local/share" \
    sh -eu -c '
        cd "$HOME"
        version=$(smill version)
        printf "%s\n" "$version"
        case "$version" in "smill v"[0-9]*.[0-9]*.[0-9]*) ;; *) exit 1 ;; esac
    '
