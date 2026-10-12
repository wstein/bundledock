#!/bin/sh
set -eu
# Listing only: never invoke branch deletion, even in this disposable fixture.
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT HUP INT TERM
mkdir -p "$work/home" "$work/repo"
export HOME="$work/home" XDG_CONFIG_HOME="$work/home/.config"
export GIT_CONFIG_NOSYSTEM=1 GIT_CONFIG_GLOBAL=/dev/null
export GIT_AUTHOR_NAME=BundleDock GIT_AUTHOR_EMAIL=smoke@example.invalid
export GIT_COMMITTER_NAME="$GIT_AUTHOR_NAME" GIT_COMMITTER_EMAIL="$GIT_AUTHOR_EMAIL"
export GIT_AUTHOR_DATE=2000-01-01T00:00:00Z
export GIT_COMMITTER_DATE="$GIT_AUTHOR_DATE"
deadbranch --version | grep -E '^deadbranch [0-9]+\.[0-9]+\.[0-9]+'
cd "$work/repo"
git -c init.templateDir= init -q -b main
git -c core.hooksPath=/dev/null commit -q --allow-empty -m smoke
git branch feature/deadbranch-smoke
git for-each-ref --format='%(refname) %(objectname)' > "$work/before"
deadbranch list --local --days 30 > "$work/list"
grep -F feature/deadbranch-smoke "$work/list"
git for-each-ref --format='%(refname) %(objectname)' > "$work/after"
cmp "$work/before" "$work/after"
