#!/bin/sh
set -eu
fzf --version
bat --version
lsd --version
regctl version
# Version probes must exit 0 and print an anchored banner; a loader error such as
# an unsupported glibc must not satisfy a loose dotted-number match.
expect_version() {
	pattern=$1
	shift
	out=$("$@") || { echo "$1 version probe failed" >&2; exit 1; }
	printf '%s\n' "$out" | grep -Eq "$pattern" || { echo "unexpected $1 version output" >&2; exit 1; }
}
expect_version '^Version: [0-9]+\.[0-9]+\.[0-9]+' jsonschema-cli --version
expect_version '^Difftastic [0-9]+\.[0-9]+\.[0-9]+' difft --version
expect_version '^zoxide [0-9]+\.[0-9]+\.[0-9]+' zoxide --version
expect_version '^git-cliff [0-9]+\.[0-9]+\.[0-9]+' git-cliff --version
expect_version '^GitVersion: +v[0-9]+\.[0-9]+\.[0-9]+' cosign version
expect_version '^zizmor [0-9]+\.[0-9]+\.[0-9]+' zizmor --version
expect_version '^pinact version [0-9]+\.[0-9]+\.[0-9]+' pinact --version
expect_version '^Haskell Dockerfile Linter [0-9]+\.[0-9]+\.[0-9]+' hadolint --version
expect_version '^[0-9]+\.[0-9]+\.[0-9]+' actionlint -version
# Fixed input and filter avoid TTY interaction and inherited shell integrations.
test "$(printf '%s\n' alpha beta | FZF_DEFAULT_OPTS= FZF_DEFAULT_OPTS_FILE= fzf --filter beta)" = beta
# Never invoke a consumer pager or load user bat configuration.
test "$(printf '%s\n' 'package main' | BAT_CONFIG_PATH=/dev/null BAT_OPTS= bat --paging=never --color=never --style=plain --language=go -)" = 'package main'
# List a private temporary directory without icons, colors or user config.
work=$(mktemp -d "${TMPDIR:-/tmp}/bundledock-lsd-smoke.XXXXXX")
trap 'rm -rf "$work"' EXIT HUP INT TERM
printf '%s\n' smoke > "$work/entry.txt"
test "$(LS_COLORS= lsd --ignore-config --color never --icon never --oneline "$work")" = entry.txt
# Help and local parsing do not initialize a registry client or read credentials.
regctl --help >/dev/null
regctl image digest --help >/dev/null
test "$(regctl ref --format '{{.Repository}}' example.invalid/team/image:v1)" = team/image
