#!/bin/sh
# Runs only during explicitly approved native image smoke.
set -eu
root=/opt/bundledock/codex
export PATH="$root/bin:$PATH"
export CODEX_HOME="$HOME/.codex"
test -f "$root/.ready"
test -f "$root/codex-package.json"
test ! -e "$root/codex-resources/voice"
for helper in bin/codex-code-mode-host codex-path/rg codex-resources/bwrap codex-resources/zsh/bin/zsh; do
    test -x "$root/$helper"
done
codex --version
trap 'codex app-server daemon stop >/dev/null 2>&1 || :' EXIT
timeout 45 codex app-server daemon start
codex app-server daemon version
codex app-server daemon stop
if touch "$root/unexpected-write" 2>/dev/null; then
    echo 'Unexpected write to read-only Codex bundle' >&2
    exit 1
fi
