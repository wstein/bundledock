#!/bin/sh
set -eu
# Call with a writable, isolated HOME. No terminal or process actions required.
export TERM=xterm-256color
scratch=$(mktemp -d "$HOME/process-monitors-smoke.XXXXXX")
trap 'rm -rf "$scratch"' EXIT

timeout 10 btm --version > "$scratch/bottom-version"
grep -Eq '^bottom [0-9]+[.][0-9]+[.][0-9]+' "$scratch/bottom-version"
timeout 10 btm --help > "$scratch/bottom-help"
grep -q 'Usage: btm' "$scratch/bottom-help"
grep -q -- '--read_only' "$scratch/bottom-help"
timeout 10 btop --version > "$scratch/btop-version"
grep -Eq '[0-9]+[.][0-9]+[.][0-9]+' "$scratch/btop-version"
timeout 10 btop --help > "$scratch/btop-help"
grep -q -- '--default-config' "$scratch/btop-help"
timeout 10 btop --default-config > "$scratch/btop-config"
grep -Eq '^update_ms[[:space:]]*=' "$scratch/btop-config"
echo 'Process monitor version/help/default-config smoke passed'
