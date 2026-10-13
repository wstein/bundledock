#!/bin/sh
# Only run in an explicitly approved native glibc consumer, without networking.
set -eu
root=/opt/bdock/claude
export PATH="$root/bin:$PATH"
export DISABLE_UPDATES=1 DISABLE_AUTOUPDATER=1
test -f "$root/.ready"
test -f "$root/licenses.tar.gz"
test -f "$root/licenses.index.json"
test -x "$root/bin/claude"
test -w "$HOME"
claude --version
if touch "$root/write-test" 2>/dev/null; then
    echo 'Unexpected write to agent bundle' >&2
    exit 1
fi
