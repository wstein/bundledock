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
for arch in amd64 arm64; do
    CGO_ENABLED=0 GOOS=linux GOARCH="$arch" GOAMD64=v1 GOARM64=v8.0 GOFLAGS= \
        "$go" build -trimpath -ldflags "-X main.version=$version" \
        -o "$destination/bundledock-linux-$arch" ./cmd/bundledock
done
cat > "$destination/PROVENANCE" <<PROVENANCE
source_revision=$revision
source_modified=$modified
version=$version
toolchain=$toolchain
build_flags=-trimpath -ldflags=-X main.version=$version
CGO_ENABLED=0
GOOS=linux
GOAMD64=v1
GOARM64=v8.0
PROVENANCE
(cd "$destination" && sha256sum bundledock-linux-amd64 bundledock-linux-arm64 PROVENANCE > SHA256SUMS)
trap - EXIT HUP INT TERM
printf 'CLI artifacts written to %s\n' "$destination"
