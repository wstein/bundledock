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
# an existing path. GNU-only ln -T is avoided (BSD ln on macOS lacks it). Plain ln refuses an
# existing file but links inside an existing directory, so the destination is re-checked right
# before linking, ln -n closes the symlink-to-directory case, and the result is verified after.
# ln -n (GNU --no-dereference; BSD/macOS: same as -h, "do not follow" a symlink target) treats a
# symlink at the destination as a plain name, so link(2) fails with EEXIST instead of linking
# into the directory it points to. It does NOT help for a real directory: a real directory
# created at $output after the check still receives the link, and only the post-link
# verification below detects and removes it. That narrow race window remains.
stage=$(mktemp -d "$root/dist/release/.bdock-stw.XXXXXXXX")
trap 'rm -rf -- "$stage"' EXIT
trap 'exit 1' HUP INT TERM
VERSION=$version TARGETS="$os/$arch" RELEASE_DIR="$stage/package" sh scripts/release.sh
built=$stage/package/$name
[ ! -e "$output" ] && [ ! -L "$output" ] || fail "output exists: $output"
ln -n -- "$built" "$output" || fail "cannot link output: $output"
# Defense in depth: a real directory created at $output after the check above would receive
# the link instead (ln -n only covers symlinks); only a regular non-symlink file that is our
# staged inode is accepted.
if [ -L "$output" ] || [ -d "$output" ] || [ ! -f "$output" ] || [ ! "$built" -ef "$output" ]; then
    [ ! -d "$output" ] || [ -L "$output" ] || ! [ "$built" -ef "$output/$name" ] || rm -f -- "$output/$name"
    fail "output is not the staged archive: $output"
fi
