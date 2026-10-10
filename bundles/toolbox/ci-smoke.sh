#!/bin/sh
# Build the checked-in lock once; test its immutable image ID without publishing.
set -eu
script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
iidfile=$(mktemp "${RUNNER_TEMP:-${TMPDIR:-/tmp}}/bundledock-ci-image.XXXXXX")
trap 'rm -f "$iidfile"' EXIT
consumer=mcr.microsoft.com/devcontainers/go:2-1.27-trixie
docker build -f bundles/toolbox/Dockerfile --iidfile "$iidfile" .
image=$(cat "$iidfile")
docker pull "$consumer"
sh "$script_dir/smoke.sh" "$image" "$consumer"
