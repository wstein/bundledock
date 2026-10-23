#!/bin/sh
set -eu
work=$(mktemp -d "${TMPDIR:-/tmp}/bundledock-bdock.XXXXXX")
trap 'rm -rf "$work"' EXIT HUP INT TERM
mkdir -p "$work/.config" "$work/.cache" "$work/.local/share"
# Version only: use --version (prints "bdock version X.Y.Z"; bdock has no version subcommand). Do not
# probe the host, load user configuration or access the network.
env -i PATH="$PATH" HOME="$work" XDG_CONFIG_HOME="$work/.config" \
    XDG_CACHE_HOME="$work/.cache" XDG_DATA_HOME="$work/.local/share" \
    sh -eu -c '
        cd "$HOME"
        version=$(bdock --version)
        printf "%s\n" "$version"
        case "$version" in "bdock version "[0-9]*.[0-9]*.[0-9]*) ;; *) exit 1 ;; esac
    '
