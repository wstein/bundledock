#!/bin/sh
# Own-script adapter for .stw/release.toml. This is a CLI artifact build and needs
# explicit operation/revision approval; source tests use a fake compiler only.
# Usage: sh scripts/release-stw.sh OS ARCH VERSION_NUMBER OUTPUT
# Keep scripts/release.sh as the sole archive builder. SourceSteward collects only
# the declared archive and creates its global checksums and build manifest later.
set -eu
root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$root"

fail() {
    printf 'release-stw: %s\n' "$*" >&2
    exit 1
}
[ "$#" -eq 4 ] || fail 'expected OS ARCH VERSION_NUMBER OUTPUT'
os=$1
arch=$2
version=$3
output=$4
case "$os/$arch" in
    linux/amd64|linux/arm64|darwin/arm64) ;;
    *) fail 'unsupported target' ;;
esac
sh .github/scripts/release-verify.sh version "$version"
name=bdock_${version}_${os}_${arch}.tar.gz
[ "$output" = "dist/release/$name" ] || fail 'output must equal the declared archive path'

# Refuse symlinked output parents and any existing destination before the compiler.
for directory in dist dist/release; do
    [ ! -L "$directory" ] || fail "symlinked output directory: $directory"
    mkdir -p -- "$directory"
done
[ ! -e "$output" ] && [ ! -L "$output" ] || fail "output exists: $output"

# Stage beside the output so the final hard link is atomic and cannot overwrite
# an existing path. GNU coreutils ln -T also refuses a concurrently created directory;
# the underlying archive builder already requires GNU tar on its Linux build host.
stage=$(mktemp -d "$root/dist/release/.bdock-stw.XXXXXXXX")
trap 'rm -rf -- "$stage"' EXIT
trap 'exit 1' HUP INT TERM
VERSION=$version TARGETS="$os/$arch" RELEASE_DIR="$stage/package" sh scripts/release.sh
ln -T -- "$stage/package/$name" "$output"
