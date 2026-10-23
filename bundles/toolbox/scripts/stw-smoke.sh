#!/bin/sh
set -eu
work=$(mktemp -d "${TMPDIR:-/tmp}/bundledock-stw.XXXXXX")
trap 'rm -rf "$work"' EXIT HUP INT TERM
mkdir -p "$work/.config" "$work/.cache" "$work/.local/share"
# Version only: use the version subcommand (prints "SourceSteward vX.Y.Z"). Do not
# probe the host, load user configuration or access the network.
env -i PATH="$PATH" HOME="$work" XDG_CONFIG_HOME="$work/.config" \
    XDG_CACHE_HOME="$work/.cache" XDG_DATA_HOME="$work/.local/share" \
    sh -eu -c '
        cd "$HOME"
        version=$(stw version)
        printf "%s\n" "$version"
        case "$version" in "SourceSteward v"[0-9]*.[0-9]*.[0-9]*) ;; *) exit 1 ;; esac
    '
