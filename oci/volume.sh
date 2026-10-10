#!/bin/sh
set -eu
image=${1:-anchorage/tools:poc}
# Resolve the mutable tag once; both the volume and initializer use this ID.
id=$(docker image inspect --format '{{.Id}}' -- "$image")
digest=${id#sha256:}
case "$id" in sha256:*) ;; *) echo 'Invalid Docker image ID' >&2; exit 1 ;; esac
case "$digest" in ''|*[!0-9a-f]*) echo 'Invalid Docker image ID' >&2; exit 1 ;; esac
test "${#digest}" -eq 64 || { echo 'Invalid Docker image ID' >&2; exit 1; }
volume="anchorage-tools-$digest"
docker volume create "$volume" >/dev/null
docker run --rm --network none --mount "type=volume,src=$volume,dst=/bundle,volume-nocopy" "$id"
printf 'Volume: %s\n' "$volume"
