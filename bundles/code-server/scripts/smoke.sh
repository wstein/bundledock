#!/bin/sh
# Manual, explicitly approved native image verification only.
set -eu
script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
image=${1:-bundledock-code-server:poc}
consumer=${2:-mcr.microsoft.com/devcontainers/go:2-1.27-trixie}
image=$(docker image inspect --format '{{.Id}}' "$image")
volume=$(docker volume create)
trap 'docker volume rm "$volume" >/dev/null' EXIT
# The carrier entrypoint supplies the explicit standalone profile.
docker run --rm --network none --mount "type=volume,src=$volume,dst=/bundle,volume-nocopy" "$image"
docker run --rm --network none --mount "type=volume,src=$volume,dst=/bundle,volume-nocopy" "$image"
check() {
    docker run --rm --network none --user 1000:1000 \
      --tmpfs /home/bundledock:exec,uid=1000,gid=1000,mode=0700 \
      --env HOME=/home/bundledock \
      --mount "type=volume,src=$volume,dst=/opt/bdock/code-server,readonly,volume-nocopy" \
      --mount "type=bind,src=$script_dir/consumer-smoke.sh,dst=/code-server-smoke.sh,readonly" \
      --entrypoint /bin/sh "$consumer" /code-server-smoke.sh
}
check &
first=$!
check &
second=$!
result=0
wait "$first" || result=1
wait "$second" || result=1
exit "$result"
