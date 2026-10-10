#!/bin/sh
set -eu
directory=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
temporary=$(mktemp -d)
trap 'rm -rf "$temporary"' EXIT
export HOME="$temporary/home" XDG_CONFIG_HOME="$temporary/config"
mkdir -p "$HOME" "$XDG_CONFIG_HOME/fish"
printf 'user fish settings\n' > "$XDG_CONFIG_HOME/fish/config.fish"

sh "$directory/setup.sh"
test "$(cat "$XDG_CONFIG_HOME/fish/config.fish")" = 'user fish settings'
test -f "$XDG_CONFIG_HOME/fish/conf.d/anchorage-starship.fish"
grep -q 'status is-interactive' "$XDG_CONFIG_HOME/fish/conf.d/anchorage-starship.fish"
grep -q 'starship init fish' "$XDG_CONFIG_HOME/fish/conf.d/anchorage-starship.fish"
grep -q 'exec herdr \$argv' "$XDG_CONFIG_HOME/fish/conf.d/anchorage-herdr.fish"
grep -q 'default_shell = "/opt/workharbor/bin/fish"' "$XDG_CONFIG_HOME/herdr/config.toml"
test ! -e "$XDG_CONFIG_HOME/starship.toml"

# User edits survive both repeated setup and devcontainer recreation.
printf 'user herdr settings\n' > "$XDG_CONFIG_HOME/herdr/config.toml"
printf 'user prompt settings\n' > "$XDG_CONFIG_HOME/fish/conf.d/anchorage-starship.fish"
sh "$directory/setup.sh"
test "$(cat "$XDG_CONFIG_HOME/herdr/config.toml")" = 'user herdr settings'
test "$(cat "$XDG_CONFIG_HOME/fish/conf.d/anchorage-starship.fish")" = 'user prompt settings'

# An existing persistent home also works without XDG overrides.
unset XDG_CONFIG_HOME
sh "$directory/setup.sh"
test -f "$HOME/.config/herdr/config.toml"
test -f "$HOME/.config/fish/conf.d/anchorage-starship.fish"
echo 'Devcontainer setup checks passed'
