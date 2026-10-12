#!/bin/sh
set -eu
script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
image=${1:-bundledock-codex:poc}
consumer=${2:-mcr.microsoft.com/devcontainers/go:2-1.27-trixie}
image=$(docker image inspect --format '{{.Id}}' "$image")
volume=$(docker volume create)
cleanup() { docker volume rm "$volume" >/dev/null; }
trap cleanup EXIT
docker run --rm --network none --mount "type=volume,src=$volume,dst=/bundle,volume-nocopy" "$image"
docker run --rm --network none --mount "type=volume,src=$volume,dst=/bundle,volume-nocopy" "$image"
check() {
    docker run --rm --network none --user 1000:1000 \
        --tmpfs /home/bundledock:exec,uid=1000,gid=1000,mode=0700 \
        --mount "type=volume,src=$volume,dst=/opt/bundledock/codex,readonly,volume-nocopy" \
        --mount "type=bind,src=$script_dir/consumer-smoke.sh,dst=/codex-smoke.sh,readonly" \
        --env HOME=/home/bundledock --env DISABLE_UPDATES=1 --env DISABLE_AUTOUPDATER=1 --entrypoint /bin/sh "$consumer" /codex-smoke.sh
}
check & first=$!
check & second=$!
status=0
wait "$first" || status=1
wait "$second" || status=1
exit "$status"
