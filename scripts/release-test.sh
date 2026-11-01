#!/bin/sh
set -eu
root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
test_dir=$(mktemp -d)
trap 'rm -rf "$test_dir"' EXIT
trap 'exit 1' HUP INT TERM
cd "$root"
# Verifying SHA256SUMS needs a SHA-256 tool: sha256sum (GNU coreutils) or macOS's stock `shasum -a 256`.
if command -v sha256sum >/dev/null 2>&1; then
    sha256_tool=sha256sum
elif command -v shasum >/dev/null 2>&1; then
    sha256_tool=shasum
else
    echo 'sha256sum or shasum is required to verify SHA256SUMS (macOS ships shasum; Linux: coreutils)' >&2
    exit 1
fi
sha256_check() {
    case "$sha256_tool" in
        sha256sum) sha256sum -c "$@" ;;
        *) shasum -a 256 -c "$@" ;;
    esac
}
modified=false
if test -n "$(git status --porcelain --untracked-files=normal)"; then modified=true; fi
# Exercise the public Make target with a linker-injected version.
make release VERSION=local-test RELEASE_DIR="$test_dir/first"
(cd "$test_dir/first" && sha256_check SHA256SUMS)
cmp LICENSE "$test_dir/first/LICENSE"
grep -Eq '^[0-9a-f]{64}  LICENSE$' "$test_dir/first/SHA256SUMS"
for target in linux_amd64 linux_arm64 darwin_arm64; do
    arch=$target # per-target directory name
    mkdir "$test_dir/$arch"
    tar -xzf "$test_dir/first/bdock_local-test_${target}.tar.gz" -C "$test_dir/$arch"
    binary="$test_dir/$arch/bdock"
    cmp LICENSE "$test_dir/$arch/LICENSE"
    grep -Fx "GOOS=${target%_*}" "$test_dir/$arch/PROVENANCE"
    grep -Fx "GOARCH=${target#*_}" "$test_dir/$arch/PROVENANCE"
    grep -Fx 'command=bdock' "$test_dir/$arch/PROVENANCE"
    test -x "$binary"
    go version -m "$binary" > "$test_dir/buildinfo-$arch"
    grep -q 'CGO_ENABLED=0' "$test_dir/buildinfo-$arch"
    grep -q -- '-trimpath=true' "$test_dir/buildinfo-$arch"
done
grep -q 'GOAMD64=v1' "$test_dir/buildinfo-linux_amd64"
grep -q 'GOARM64=v8.0' "$test_dir/buildinfo-linux_arm64"
grep -q 'GOARM64=v8.0' "$test_dir/buildinfo-darwin_arm64"
grep -Fx "source_revision=$(git rev-parse HEAD)" "$test_dir/first/PROVENANCE"
grep -Fx "source_modified=$modified" "$test_dir/first/PROVENANCE"
grep -Fx 'version=local-test' "$test_dir/first/PROVENANCE"
if test "$(go env GOHOSTOS)" = linux; then
    case $(go env GOHOSTARCH) in
        amd64|arm64)
            test "$("$test_dir/linux_$(go env GOHOSTARCH)/bdock" --version)" = 'bdock version local-test'
            ;;
    esac
fi
make release VERSION=local-test RELEASE_DIR="$test_dir/second"
for name in bdock_local-test_darwin_arm64.tar.gz bdock_local-test_linux_amd64.tar.gz bdock_local-test_linux_arm64.tar.gz SHA256SUMS PROVENANCE LICENSE; do
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
(cd "$test_dir/first" && sha256_check SHA256SUMS)
echo 'Release artifact checks passed'
