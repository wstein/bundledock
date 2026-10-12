#!/bin/sh
set -eu
script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
# Preserve the Docker-only toolbox adapter and optional legacy volume name.
exec sh "$script_dir/../../../scripts/bundle-volume.sh" "${1:-bundledock-toolbox:poc}" toolbox "${2:-}"
