#!/bin/sh
set -eu
# Version-only probes: no HTTP requests, directory scans, .envrc evaluation,
# key service requests, or configuration changes. Consumer networking is disabled.
export SOPS_DISABLE_VERSION_CHECK=1
smoke_stage=initialization
finish() {
    status=$?
    if test "$status" -ne 0; then
        printf 'HTTP/disk/environment/secrets smoke failed: %s (exit %s)\n' "$smoke_stage" "$status" >&2
    fi
    exit "$status"
}
trap finish EXIT
trap 'exit 129' HUP
trap 'exit 130' INT
trap 'exit 143' TERM
smoke_stage='xh version'
output=$(timeout 10 xh --version)
smoke_stage='xh version assertion'
printf '%s\n' "$output" | grep -Eq '^xh [0-9]+[.][0-9]+[.][0-9]+'
smoke_stage='dust version'
output=$(timeout 10 dust --version)
smoke_stage='dust version assertion'
printf '%s\n' "$output" | grep -Eq '^Dust [0-9]+[.][0-9]+[.][0-9]+'
smoke_stage='direnv version'
output=$(timeout 10 direnv version)
smoke_stage='direnv version assertion'
printf '%s\n' "$output" | grep -Eq '^[0-9]+[.][0-9]+[.][0-9]+'
smoke_stage='sops version'
output=$(timeout 10 sops --disable-version-check --version)
smoke_stage='sops version assertion'
printf '%s\n' "$output" | grep -Eq '^sops [0-9]+[.][0-9]+[.][0-9]+'
printf '%s\n' 'HTTP/disk/environment/secrets version smoke passed'
