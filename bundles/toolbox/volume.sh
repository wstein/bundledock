#!/bin/sh
set -eu
image=${1:-bundledock-toolbox:poc}
# Resolve the mutable tag once; both the volume and initializer use this ID.
id=$(docker image inspect --format '{{.Id}}' -- "$image")
digest=${id#sha256:}
case "$id" in sha256:*) ;; *) echo 'Invalid Docker image ID' >&2; exit 1 ;; esac
case "$digest" in ''|*[!0-9a-f]*) echo 'Invalid Docker image ID' >&2; exit 1 ;; esac
test "${#digest}" -eq 64 || { echo 'Invalid Docker image ID' >&2; exit 1; }
# An explicit name keeps pre-rename volumes selectable without copying data.
volume=${2:-"bundledock-toolbox-$digest"}
case "$volume" in
    ''|[!a-zA-Z0-9]*|*[!a-zA-Z0-9_.-]*) echo 'Invalid Docker volume name' >&2; exit 1 ;;
esac
docker volume create "$volume" >/dev/null
docker run --rm --network none --mount "type=volume,src=$volume,dst=/bundle,volume-nocopy" "$id"
printf 'Volume: %s\n' "$volume"
