#!/bin/sh
set -eu
jj=${1:-jj}
work=$(mktemp -d "${TMPDIR:-/tmp}/bundledock-jj-smoke.XXXXXX")
trap 'rm -rf "$work"' EXIT HUP INT TERM
mkdir -p "$work/home" "$work/config" "$work/cache"
# Clear inherited Git repository/index/object/config selectors and Jujutsu
# overrides before even --version. All state belongs to this private fixture.
env -i PATH="$PATH" HOME="$work/home" XDG_CONFIG_HOME="$work/config" \
    XDG_CACHE_HOME="$work/cache" JJ_CONFIG= JJ_USER='BundleDock smoke' \
    JJ_EMAIL=smoke@example.invalid GIT_CONFIG_NOSYSTEM=1 \
    GIT_CONFIG_GLOBAL=/dev/null sh -eu -c '
        cd "$1"
        jj=$2
        "$jj" --version
        "$jj" --no-pager git init --no-colocate repo
        cd repo
        test -d .jj
        "$jj" --no-pager new --message bundledock-jj-smoke
        test "$("$jj" --no-pager log --no-graph --revisions @ --template description)" = bundledock-jj-smoke
    ' sh "$work" "$jj"
