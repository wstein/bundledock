#!/bin/sh
set -eu
root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
test_dir=$(mktemp -d)
trap 'rm -rf "$test_dir"' EXIT
trap 'exit 1' HUP INT TERM
cd "$root"
modified=false
if test -n "$(git status --porcelain --untracked-files=normal)"; then modified=true; fi
# Exercise the public Make target with a linker-injected version.
make release VERSION=local-test RELEASE_DIR="$test_dir/first"
(cd "$test_dir/first" && sha256sum -c SHA256SUMS)
cmp LICENSE "$test_dir/first/LICENSE"
grep -Eq '^[0-9a-f]{64}  LICENSE$' "$test_dir/first/SHA256SUMS"
for arch in amd64 arm64; do
    binary="$test_dir/first/bundledock-linux-$arch"
    test -x "$binary"
    go version -m "$binary" > "$test_dir/buildinfo-$arch"
    grep -q 'CGO_ENABLED=0' "$test_dir/buildinfo-$arch"
    grep -q -- '-trimpath=true' "$test_dir/buildinfo-$arch"
done
grep -q 'GOAMD64=v1' "$test_dir/buildinfo-amd64"
grep -q 'GOARM64=v8.0' "$test_dir/buildinfo-arm64"
grep -Fx "source_revision=$(git rev-parse HEAD)" "$test_dir/first/PROVENANCE"
grep -Fx "source_modified=$modified" "$test_dir/first/PROVENANCE"
grep -Fx 'version=local-test' "$test_dir/first/PROVENANCE"
if test "$(go env GOHOSTOS)" = linux; then
    case $(go env GOHOSTARCH) in
        amd64|arm64)
            test "$("$test_dir/first/bundledock-linux-$(go env GOHOSTARCH)" --version)" = 'bdock version local-test'
            ;;
    esac
fi
make release VERSION=local-test RELEASE_DIR="$test_dir/second"
for name in bundledock-linux-amd64 bundledock-linux-arm64 SHA256SUMS PROVENANCE LICENSE; do
    cmp "$test_dir/first/$name" "$test_dir/second/$name"
done
if make release VERSION=local-test RELEASE_DIR="$test_dir/first"; then
    echo 'release unexpectedly overwrote an existing destination' >&2
    exit 1
fi
if make release VERSION='../invalid version' RELEASE_DIR="$test_dir/invalid"; then
    echo 'release unexpectedly accepted an invalid version' >&2
    exit 1
fi
test ! -e "$test_dir/invalid"
(cd "$test_dir/first" && sha256sum -c SHA256SUMS)
echo 'Release artifact checks passed'
