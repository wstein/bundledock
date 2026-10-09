#!/bin/sh
set -eu
image=${1:-anchorage/tools:poc}
# Resolve the mutable tag once; both the volume and initializer use this ID.
id=$(docker image inspect --format '{{.Id}}' "$image")
volume="anchorage-tools-${id#sha256:}"
docker volume create "$volume" >/dev/null
docker run --rm --network none --mount "type=volume,src=$volume,dst=/bundle,volume-nocopy" "$id"
printf 'Volume: %s\n' "$volume"
