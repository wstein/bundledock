#!/bin/sh
# Single builder of the bdock CLI archives, shared by local preparation and the
# tag-driven CI release workflow (.github/workflows/release.yml). Running it is a
# CLI release build: it needs the user's approval locally and is run by CI only
# under the authorization recorded in AGENTS.md.
#
# Environment: VERSION (default dev), RELEASE_DIR (default dist/cli), GO, and
# TARGETS, a space-separated subset of "linux/amd64 linux/arm64 darwin/arm64"
# (the default is all three; CI builds one per job). GOFLAGS is cleared on purpose; a
# RELEASE_DIR inside the checkout must be ignored by git or the tree counts as modified.
set -eu
root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$root"
go=${GO:-go}
version=${VERSION:-dev}
destination=${RELEASE_DIR:-dist/cli}
targets=${TARGETS-linux/amd64 linux/arm64 darwin/arm64}
case "$version" in
    ''|*[!A-Za-z0-9._+-]*) echo 'VERSION must contain only letters, digits, dot, underscore, plus or hyphen' >&2; exit 1 ;;
esac
# Only the supported platforms are built; darwin/amd64 is end of life and Windows is out.
list=
for target in $targets; do
    case "$target" in
        linux/amd64|linux/arm64|darwin/arm64) ;;
        *) echo "unsupported target: $target (want linux/amd64, linux/arm64 or darwin/arm64)" >&2; exit 1 ;;
    esac
    list="$list$target
"
done
case "$targets" in *'
'*) echo 'TARGETS must be a space-separated list' >&2; exit 1 ;; esac
list=$(printf '%s' "$list" | LC_ALL=C sort -u)
test -n "$list" || { echo 'TARGETS must name at least one target' >&2; exit 1; }
# Repeatable archives need GNU tar flags. Use `tar` when it is GNU tar; otherwise accept
# Homebrew's `gtar` (macOS ships bsdtar as `tar`). Never fall back to a non-GNU tar.
tar_cmd=
for candidate in tar gtar; do
    if "$candidate" --version 2>/dev/null | grep -q 'GNU tar'; then tar_cmd=$candidate; break; fi
done
test -n "$tar_cmd" || { echo 'GNU tar is required for repeatable archives (on macOS: brew install gnu-tar, which provides gtar)' >&2; exit 1; }
# SHA256SUMS needs a SHA-256 tool: sha256sum (GNU coreutils) or macOS's stock `shasum -a 256`.
# Both print "<hash>  <name>" for the files named here, so the file bytes do not depend on the tool.
if command -v sha256sum >/dev/null 2>&1; then
    sha256_tool=sha256sum
elif command -v shasum >/dev/null 2>&1; then
    sha256_tool=shasum
else
    echo 'sha256sum or shasum is required to write SHA256SUMS (macOS ships shasum; Linux: coreutils)' >&2
    exit 1
fi
sha256_files() {
    case "$sha256_tool" in
        sha256sum) sha256sum "$@" ;;
        *) shasum -a 256 "$@" ;;
    esac
}
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
chmod 0644 "$destination/LICENSE"
cat > "$destination/PROVENANCE" <<PROVENANCE
source_revision=$revision
source_modified=$modified
version=$version
command=bdock
toolchain=$toolchain
build_flags=-trimpath -ldflags=-X main.version=$version
CGO_ENABLED=0
targets=$(printf '%s' "$list" | tr '\n' ' ' | sed 's/ $//')
GOAMD64=v1
GOARM64=v8.0
PROVENANCE
chmod 0644 "$destination/PROVENANCE"
mkdir "$destination/.stage"
archives=
for target in $list; do
    os=${target%/*}
    arch=${target#*/}
    CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" GOAMD64=v1 GOARM64=v8.0 GOFLAGS= \
        "$go" build -trimpath -ldflags "-X main.version=$version" \
        -o "$destination/.stage/bdock" ./cmd/bdock
    chmod 0755 "$destination/.stage/bdock"
    cp "$destination/LICENSE" "$destination/.stage/LICENSE"
    cp "$destination/PROVENANCE" "$destination/.stage/PROVENANCE"
    printf 'GOOS=%s\nGOARCH=%s\n' "$os" "$arch" >> "$destination/.stage/PROVENANCE"
    chmod 0644 "$destination/.stage/LICENSE" "$destination/.stage/PROVENANCE"
    # Normalize archive metadata and gzip headers so equal inputs give equal bytes.
    name=bdock_${version}_${os}_${arch}.tar.gz
    "$tar_cmd" --sort=name --mtime=@0 --owner=0 --group=0 --numeric-owner --format=ustar \
        -cf "$destination/.stage.tar" -C "$destination/.stage" bdock LICENSE PROVENANCE
    gzip -n < "$destination/.stage.tar" > "$destination/$name"
    archives="$archives $name"
done
rm -rf -- "$destination/.stage" "$destination/.stage.tar"
# shellcheck disable=SC2086 # archive names contain no whitespace
(cd "$destination" && sha256_files $archives PROVENANCE LICENSE > SHA256SUMS)
trap - EXIT HUP INT TERM
printf 'CLI artifacts written to %s\n' "$destination"
