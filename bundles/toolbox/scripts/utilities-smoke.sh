#!/bin/sh
set -eu
fzf --version
bat --version
regctl version
# Fixed input and filter avoid TTY interaction and inherited shell integrations.
test "$(printf '%s\n' alpha beta | FZF_DEFAULT_OPTS= FZF_DEFAULT_OPTS_FILE= fzf --filter beta)" = beta
# Never invoke a consumer pager or load user bat configuration.
test "$(printf '%s\n' 'package main' | BAT_CONFIG_PATH=/dev/null BAT_OPTS= bat --paging=never --color=never --style=plain --language=go -)" = 'package main'
# Help and local parsing do not initialize a registry client or read credentials.
regctl --help >/dev/null
regctl image digest --help >/dev/null
test "$(regctl ref --format '{{.Repository}}' example.invalid/team/image:v1)" = team/image
