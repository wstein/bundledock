#!/bin/sh
# Run only as part of an explicitly approved native image smoke operation.
set -eu
hx=${1:-hx}
stage=configuration
diagnostics() {
    status=$?
    if [ "$status" -ne 0 ]; then
        printf 'Helix smoke failed at %s (exit %s)\n' "$stage" "$status" >&2
        for report in "$HOME/helix-health.txt" "$HOME/helix-go-health.txt"; do
            if [ -f "$report" ]; then
                printf '\nHelix report: %s\n' "$report" >&2
                cat "$report" >&2 || true
            fi
        done
    fi
}
trap diagnostics EXIT
# Helix 25.07.1 styles individual health markers even when stdout is redirected.
# Normalize SGR styling before matching; keep raw reports for failure diagnostics.
plain_health() {
    escape=$(printf '\033')
    sed "s/${escape}\[[0-9;]*m//g" "$1" > "$2"
}
export XDG_CONFIG_HOME="$HOME/.config" NO_COLOR=1
mkdir -p "$XDG_CONFIG_HOME/helix"
printf '%s\n' '[editor]' 'line-number = "relative"' > "$XDG_CONFIG_HOME/helix/config.toml"
stage=version
"$hx" --version
stage=general-health
"$hx" --health > "$HOME/helix-health.txt" 2>&1
plain_health "$HOME/helix-health.txt" "$HOME/helix-health-plain.txt"
stage=config-assertion
grep -Fx "Config file: $XDG_CONFIG_HOME/helix/config.toml" "$HOME/helix-health-plain.txt"
stage=go-health
"$hx" --health go > "$HOME/helix-go-health.txt" 2>&1
plain_health "$HOME/helix-go-health.txt" "$HOME/helix-go-health-plain.txt"
stage=go-parser-assertion
grep -Fx 'Tree-sitter parser: ✓' "$HOME/helix-go-health-plain.txt"
stage=go-highlight-assertion
grep -Fx 'Highlight queries: ✓' "$HOME/helix-go-health-plain.txt"
