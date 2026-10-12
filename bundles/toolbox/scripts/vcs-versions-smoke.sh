#!/bin/sh
set -eu
work=$(mktemp -d "${TMPDIR:-/tmp}/bundledock-vcs-versions.XXXXXX")
trap 'rm -rf "$work"' EXIT HUP INT TERM
mkdir -p "$work/home" "$work/config" "$work/cache"
# Version-only probes: never open a TUI or modify Git hooks/configuration.
env -i PATH="$PATH" HOME="$work/home" XDG_CONFIG_HOME="$work/config" \
    XDG_CACHE_HOME="$work/cache" GIT_CONFIG_NOSYSTEM=1 \
    GIT_CONFIG_GLOBAL=/dev/null sh -eu -c '
        cd "$1"
        version=$(lazygit --version)
        printf "%s\n" "$version"
        case "$version" in *version=[0-9]*) ;; *) exit 1 ;; esac
    ' sh "$work"
