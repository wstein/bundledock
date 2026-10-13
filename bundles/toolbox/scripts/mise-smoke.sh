#!/bin/sh
set -eu
# Version inspection only: never activate shells, bootstrap or install runtimes.
home=$(mktemp -d)
trap 'rm -rf "$home"' EXIT HUP INT TERM
mkdir -p "$home/.config" "$home/.cache" "$home/.local/share" "$home/.local/state"
version=$(
    cd "$home"
    env -i PATH="$PATH" HOME="$home" \
        XDG_CONFIG_HOME="$home/.config" XDG_CACHE_HOME="$home/.cache" \
        XDG_DATA_HOME="$home/.local/share" XDG_STATE_HOME="$home/.local/state" \
        mise --version
)
printf '%s\n' "$version"
printf '%s\n' "$version" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+([[:space:]].*)?$'
