#!/bin/sh
# Manual, explicitly approved native image verification only.
#
# Usage: smoke.sh [NIXBOX_IMAGE [IGNORED]]
#   The second argument is the generic make target's CONSUMER; this smoke test
#   owns its pinned consumer matrix and ignores it.
# Environment:
#   NIXBOX_PLATFORM       docker platform for every container (default: native)
#   NIXBOX_TOOLBOX_IMAGE  toolbox carrier image whose volume is mounted at
#                         /opt/bdock/toolbox for the eight shimmed binaries
#   NIXBOX_OFFLINE=1      skip the one check that needs the network
set -eu
script_dir=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
image=${1:-bundledock-nixbox:poc}
platform=${NIXBOX_PLATFORM:-}
toolbox_image=${NIXBOX_TOOLBOX_IMAGE:-}
offline=${NIXBOX_OFFLINE:-0}
# Pinned consumers (multi-architecture index digests): Debian slim, Fedora,
# openSUSE Leap, Alpine.
consumers='debian@sha256:f6b08b09a05725f9431502d3b5714f5c4576e82c1a84fa1debe0c24a7e0c3ec4
fedora@sha256:43b29f65a41eb9c35e1cd5323e3bdf3b655c2357a9f4f1ff2f9c2798e5045d80
opensuse/leap@sha256:952857883a24c24a64c8bc67d7d1a8f9d4800ad4fbe075fc4566ff9af706cb8b
alpine@sha256:85fe1e81d6758c208f3e1eed4338a1997e19d4be002d4dd32d3100c9a8c010a0'
# The Debian digest is the trixie-slim index, Fedora is 44, Leap is 16.0.
label() {
    case $1 in
    debian@*) printf 'debian-trixie-slim' ;;
    fedora@*) printf 'fedora-44' ;;
    opensuse/leap@*) printf 'opensuse-leap-16.0' ;;
    alpine@*) printf 'alpine-3.23' ;;
    *) printf '%s' "$1" ;;
    esac
}
platform_flag() { if [ -n "$platform" ]; then printf '%s\n' --platform "$platform"; fi; }
# shellcheck disable=SC2046
docker_run() { docker run --rm $(platform_flag) "$@"; }
nix_volume=$(docker volume create)
toolbox_volume=
workdir=$(mktemp -d)
cleanup() {
    rm -rf "$workdir"
    docker volume rm "$nix_volume" >/dev/null 2>&1 || true
    if [ -n "$toolbox_volume" ]; then docker volume rm "$toolbox_volume" >/dev/null 2>&1 || true; fi
}
trap cleanup EXIT

# Initialize the carrier volume twice: the second run verifies instead of copying.
docker_run --network none --mount "type=volume,src=$nix_volume,dst=/bundle,volume-nocopy" "$image"
docker_run --network none --mount "type=volume,src=$nix_volume,dst=/bundle,volume-nocopy" "$image"
toolbox_mount=
if [ -n "$toolbox_image" ]; then
    toolbox_volume=$(docker volume create)
    docker_run --network none --mount "type=volume,src=$toolbox_volume,dst=/bundle,volume-nocopy" "$toolbox_image"
    toolbox_mount="type=volume,src=$toolbox_volume,dst=/opt/bdock/toolbox,readonly,volume-nocopy"
fi

result=0
run_consumer() {
    consumer=$1
    user=$2
    network=bridge
    if [ "$offline" = 1 ]; then network=none; fi
    set -- --network "$network" --read-only --user "$user" \
        --tmpfs /tmp:exec,mode=1777 --tmpfs /home/smoke:exec,mode=1777 \
        --env HOME=/home/smoke --env NIXBOX_OFFLINE="$offline" \
        --mount "type=volume,src=$nix_volume,dst=/nix,readonly,volume-nocopy" \
        --mount "type=bind,src=$script_dir/consumer-smoke.sh,dst=/nixbox-smoke.sh,readonly"
    if [ -n "$toolbox_mount" ]; then set -- "$@" --mount "$toolbox_mount"; fi
    case $consumer in
    alpine@*)
        # Simulate gcompat offline: Alpine then has glibc-looking loader files
        # although its binaries still need the Nix loader.
        : > "$workdir/gcompat-stub"
        set -- "$@" --env NIXBOX_GCOMPAT_SIM=1 \
            --mount "type=bind,src=$workdir/gcompat-stub,dst=/lib/ld-linux-aarch64.so.1,readonly" \
            --mount "type=bind,src=$workdir/gcompat-stub,dst=/lib64/ld-linux-x86-64.so.2,readonly"
        ;;
    esac
    if [ "$user" != 0:0 ]; then
        # OpenSSH refuses a uid without a passwd entry; give the unprivileged
        # run one, read-only, without touching the consumer image.
        passwd=$workdir/passwd
        docker_run --network none --entrypoint /bin/cat "$consumer" /etc/passwd > "$passwd"
        printf 'smoke:x:%s:%s::/home/smoke:/bin/sh\n' "${user%%:*}" "${user##*:}" >> "$passwd"
        set -- "$@" --mount "type=bind,src=$passwd,dst=/etc/passwd,readonly"
    fi
    out=$workdir/output
    if ! docker_run "$@" --entrypoint /bin/sh "$consumer" /nixbox-smoke.sh > "$out" 2>&1; then result=1; fi
    cat "$out"
    line=$(sed -n 's/^RESULT //p' "$out")
    printf 'SMOKE %s %s uid=%s: %s\n' "${platform:-native}" "$(label "$consumer")" "${user%%:*}" "${line:-no result}"
}
for consumer in $consumers; do
    run_consumer "$consumer" 0:0
done
# One unprivileged run on glibc Debian proves the volume needs no root.
run_consumer "$(printf '%s\n' "$consumers" | sed -n 1p)" 1000:1000
exit "$result"
