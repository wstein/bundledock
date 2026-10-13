#!/bin/sh
set -eu
root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$root"
go=${GO:-go}
version=${VERSION:-dev}
destination=${RELEASE_DIR:-dist/cli}
case "$version" in
    ''|*[!A-Za-z0-9._+-]*) echo 'VERSION must contain only letters, digits, dot, underscore, plus or hyphen' >&2; exit 1 ;;
esac
# Capture source state before creating output, including untracked files.
revision=$(git rev-parse HEAD)
modified=false
if test -n "$(git status --porcelain --untracked-files=normal)"; then modified=true; fi
toolchain=$("$go" version)
mkdir -p -- "$(dirname -- "$destination")"
# mkdir refuses an existing file or directory, including concurrent invocations.
mkdir -- "$destination"
destination=$(CDPATH= cd -- "$destination" && pwd)
trap 'rm -rf -- "$destination"' EXIT
trap 'exit 1' HUP INT TERM
cp LICENSE "$destination/LICENSE"
cat > "$destination/PROVENANCE" <<PROVENANCE
source_revision=$revision
source_modified=$modified
version=$version
command=bdock
toolchain=$toolchain
build_flags=-trimpath -ldflags=-X main.version=$version
CGO_ENABLED=0
GOOS=linux
GOAMD64=v1
GOARM64=v8.0
PROVENANCE
mkdir "$destination/.stage"
for arch in amd64 arm64; do
    CGO_ENABLED=0 GOOS=linux GOARCH="$arch" GOAMD64=v1 GOARM64=v8.0 GOFLAGS= \
        "$go" build -trimpath -ldflags "-X main.version=$version" \
        -o "$destination/.stage/bdock" ./cmd/bdock
    cp "$destination/LICENSE" "$destination/.stage/LICENSE"
    cp "$destination/PROVENANCE" "$destination/.stage/PROVENANCE"
    printf 'GOARCH=%s\n' "$arch" >> "$destination/.stage/PROVENANCE"
    # Normalize archive metadata and gzip headers for repeatable local outputs.
    tar --sort=name --mtime=@0 --owner=0 --group=0 --numeric-owner \
        -cf "$destination/.stage.tar" -C "$destination/.stage" bdock LICENSE PROVENANCE
    gzip -n < "$destination/.stage.tar" > "$destination/bdock_${version}_linux_${arch}.tar.gz"
done
rm -rf -- "$destination/.stage" "$destination/.stage.tar"
(cd "$destination" && sha256sum "bdock_${version}_linux_amd64.tar.gz" "bdock_${version}_linux_arm64.tar.gz" PROVENANCE LICENSE > SHA256SUMS)
trap - EXIT HUP INT TERM
printf 'CLI artifacts written to %s\n' "$destination"
