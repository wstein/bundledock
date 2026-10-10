#!/bin/sh
set -eu
config=${XDG_CONFIG_HOME:-"$HOME/.config"}
mkdir -p "$config/fish/conf.d" "$config/herdr"

# Seed defaults once. The persistent home owns subsequent customization.
seed() {
    if test -e "$1" || test -L "$1"; then
        cat >/dev/null
    else
        (set -C; cat > "$1")
    fi
}

seed "$config/fish/conf.d/anchorage-starship.fish" <<'FISH'
if status is-interactive; and type -q starship
    starship init fish | source
end
FISH

seed "$config/fish/conf.d/anchorage-herdr.fish" <<'FISH'
if status is-interactive
    function herd --description 'Replace this shell with Herdr'
        exec herdr $argv
    end
end
FISH

seed "$config/herdr/config.toml" <<'TOML'
[terminal]
default_shell = "/opt/workharbor/bin/fish"
TOML
