#!/bin/sh
set -eu
# Keep an existing Python selection; expose the installed Python 3 otherwise.
if command -v python >/dev/null 2>&1; then
    exit 0
fi
python3_path=$(command -v python3)
case "$python3_path" in /*) ;; *) python3_path="$PWD/$python3_path" ;; esac
test -x "$python3_path"
bin_dir=${1:-/usr/local/bin}
target="$bin_dir/python"
# Preserve existing paths, including dangling links; never replace a selection.
if test -e "$target" || test -L "$target"; then
    exit 0
fi
if test -w "$bin_dir"; then
    ln -s "$python3_path" "$target"
else
    sudo ln -s "$python3_path" "$target"
fi
