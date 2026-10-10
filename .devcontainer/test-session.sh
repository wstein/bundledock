#!/bin/sh
# Run inside a compatible consumer with the tools volume on PATH.
set -eu
directory=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
temporary=$(mktemp -d)
trap 'rm -rf "$temporary"' EXIT
export HOME="$temporary/home" XDG_CONFIG_HOME="$temporary/config"
export SHELL=/bin/bash TERM=xterm-256color
mkdir -p "$HOME"
sh "$directory/setup.sh"
test "$(fish -c 'printf plain')" = plain
fish -c 'test "$SHELL" = /bin/bash'
fish -ic 'functions fish_prompt | string match --quiet "*starship*"; and test "$SHELL" = /bin/bash'
test "$(fish -ic 'herd --version')" = "$(herdr --version)"
bash -c 'test "$SHELL" = /bin/bash; test "$((6 * 7))" = 42'
herdr --version
echo 'Interactive prompt and Bash agent-shell checks passed'
