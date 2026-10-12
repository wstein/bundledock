#!/bin/sh
set -eu
# Never inspect or clean the caller's repository. All refs belong to this fixture.
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT HUP INT TERM
# All Git commands and git-trim run in a clean child environment. Inherited
# repository/index/object/config selectors must never reach the disposable fixture.
env -i PATH="$PATH" HOME="$work/home" GIT_CONFIG_NOSYSTEM=1 \
    GIT_CONFIG_GLOBAL=/dev/null GIT_TERMINAL_PROMPT=0 sh -eu -c '
work=$1
mkdir "$HOME"
git -c init.templateDir= init --bare "$work/remote"
git -c init.templateDir= init -b main "$work/repo"
cd "$work/repo"
git config user.name BundleDock
git config user.email smoke@bundledock.invalid
git -c core.hooksPath=/dev/null commit --allow-empty -m fixture
git remote add origin "$work/remote"
git -c core.hooksPath=/dev/null push -u origin main
git -C "$work/remote" symbolic-ref HEAD refs/heads/main
git branch fixture-topic
git -c core.hooksPath=/dev/null push -u origin fixture-topic
git for-each-ref > "$work/before"
git -C "$work/remote" for-each-ref > "$work/remote-before"
git-trim --dry-run --no-update --bases main > "$work/plan"
grep -q fixture-topic "$work/plan"
git for-each-ref > "$work/after"
git -C "$work/remote" for-each-ref > "$work/remote-after"
cmp "$work/before" "$work/after"
cmp "$work/remote-before" "$work/remote-after"
' sh "$work"
