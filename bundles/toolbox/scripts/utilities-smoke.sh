#!/bin/sh
set -eu
fzf --version
bat --version
lsd --version
regctl version
case "$(cosign version 2>&1)" in *[0-9].[0-9]*.[0-9]*) ;; *) echo "unexpected cosign version output" >&2; exit 1 ;; esac
case "$(zizmor --version 2>&1)" in *[0-9].[0-9]*.[0-9]*) ;; *) echo "unexpected zizmor version output" >&2; exit 1 ;; esac
case "$(pinact --version 2>&1)" in *[0-9].[0-9]*.[0-9]*) ;; *) echo "unexpected pinact version output" >&2; exit 1 ;; esac
case "$(hadolint --version 2>&1)" in *[0-9].[0-9]*.[0-9]*) ;; *) echo "unexpected hadolint version output" >&2; exit 1 ;; esac
case "$(actionlint -version 2>&1)" in *[0-9].[0-9]*.[0-9]*) ;; *) echo "unexpected actionlint version output" >&2; exit 1 ;; esac
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
