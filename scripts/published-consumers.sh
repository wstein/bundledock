#!/bin/sh
# Manual verification of the five already-published Anchorage consumer pins.
# No image overrides: reviewing this source fixes the entire execution scope.
set -eu
if test "$#" -ne 0; then
    echo 'Published consumer verification takes no image overrides' >&2
    exit 1
fi
script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
consumer=mcr.microsoft.com/devcontainers/go@sha256:8f1c83128e4bce9d745b1dc0f28f8c027e303d3886e2434966ff135296c41770
pins='toolbox:40bae37ef7fd7b8e7131803850801ed41cfb23a6ef774bc2e40f95ee81e59a1c
node:295cce35e9f580db41dfdc97bd30243d9f0cf388556a8a8d87a6c59ee51e6a8c
codex:eb5ec6514b7476c55d1d2c0c1def871b982026f0a75144ddab33e9af4a3f40ec
agy:a96463845e3948487029f72ef1a242996b986c4bc50e42d68707308652284496
claude:4b02222a4954f174a46af2a7ef732c2f0ff7b7bb7b462875491dd80aabf1940f'
# Validate the entire fixed set before the first Docker operation.
for pin in $pins; do
    name=${pin%%:*}
    digest=${pin#*:}
    case "$name" in toolbox|node|codex|agy|claude) ;; *) exit 1 ;; esac
    case "$digest" in ''|*[!0-9a-f]*) exit 1 ;; esac
    test "${#digest}" -eq 64
done
for pin in $pins; do
    docker pull "quay.io/bundledock/${pin%%:*}@sha256:${pin#*:}"
done
docker pull "$consumer"
volumes=''
cleanup() {
    for volume in $volumes; do docker volume rm "$volume" >/dev/null; done
}
trap cleanup EXIT
# Allocate only disposable anonymous volumes; never reuse or remove a user's.
for pin in $pins; do
    name=${pin%%:*}
    volume=$(docker volume create)
    volumes="$volumes $volume"
    sh "$script_dir/bundle-volume.sh" "quay.io/bundledock/$name@sha256:${pin#*:}" "$name" "$volume"
    case "$name" in
        agy|claude)
            # Historical carrier helpers lack the archive flags. New helpers must
            # verify SOURCE; any verification failure stops instead of falling back.
            license_help=$(docker run --rm --network none "quay.io/bundledock/$name@sha256:${pin#*:}" --help 2>&1)
            case "$license_help" in
                *-list-licenses*) docker run --rm --network none "quay.io/bundledock/$name@sha256:${pin#*:}" --license "licenses/$name/SOURCE" >/dev/null ;;
            esac
            ;;
    esac
    set -- "$@" --mount "type=volume,src=$volume,dst=/opt/bundledock/$name,readonly,volume-nocopy"
done
home_volume=$(docker volume create)
volumes="$volumes $home_volume"
# A private disposable HOME survives both non-root consumer invocations.
set -- "$@" --mount "type=volume,src=$home_volume,dst=/home/vscode,volume-nocopy"
docker run --rm --network none --user 0:0 \
    --mount "type=volume,src=$home_volume,dst=/home/vscode,volume-nocopy" \
    --entrypoint /bin/sh "$consumer" -c 'chown 1000:1000 /home/vscode'
for pass in 1 2; do
    docker run --rm --network none --user 1000:1000 \
        --env HOME=/home/vscode --env VERIFY_PASS="$pass" \
        --env DISABLE_UPDATES=1 --env DISABLE_AUTOUPDATER=1 \
        --env NPM_CONFIG_CACHE=/home/vscode/.cache/npm --env NPM_CONFIG_PREFIX=/home/vscode/.local \
        "$@" --mount "type=bind,src=$script_dir/published-consumer-check.sh,dst=/consumer-check.sh,readonly" \
        --entrypoint /bin/sh "$consumer" /consumer-check.sh
done
