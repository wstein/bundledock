#!/bin/sh
# Run only as part of an explicitly approved native image smoke operation.
set -eu
hx=${1:-hx}
export XDG_CONFIG_HOME="$HOME/.config" NO_COLOR=1
mkdir -p "$XDG_CONFIG_HOME/helix"
printf '%s\n' '[editor]' 'line-number = "relative"' > "$XDG_CONFIG_HOME/helix/config.toml"
"$hx" --version
"$hx" --health > "$HOME/helix-health.txt"
grep -F "Config file: $XDG_CONFIG_HOME/helix/config.toml" "$HOME/helix-health.txt"
"$hx" --health go > "$HOME/helix-go-health.txt"
grep -F 'Tree-sitter parser: ✓' "$HOME/helix-go-health.txt"
grep -F 'Highlight queries: ✓' "$HOME/helix-go-health.txt"
