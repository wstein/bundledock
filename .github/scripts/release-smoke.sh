#!/bin/sh
# Exercise a shipped CLI archive on its native host (docs/design/bdock-release-workflow).
#
# Usage: sh release-smoke.sh VERSION COMMIT OS ARCH ARCHIVE
#
# Run in the repository root. ARCHIVE is bdock_<version>_<os>_<arch>.tar.gz next to its
# .sha256 sidecar, or SourceSteward's global SHA256SUMS when no sidecar exists.
# Only hermetic commands run: no network, no Docker, no registry.
# Checks, in order: the checksum; the exact archive members; LICENSE equal to the
# repository's; PROVENANCE for this version, commit, OS and architecture from an
# unmodified checkout; `bdock --version`; `--help`; `tools --help`; `completion bash`.
# Works with POSIX tools on Linux and macOS.
set -eu

fail() {
	printf 'release-smoke: %s\n' "$*" >&2
	exit 1
}

[ "$#" -eq 5 ] || {
	printf 'usage: sh release-smoke.sh VERSION COMMIT OS ARCH ARCHIVE\n' >&2
	exit 2
}
version=$1
commit=$2
os=$3
arch=$4
archive=$5

[ -f "$archive" ] || fail "archive not found: $archive"
[ -f LICENSE ] || fail "LICENSE not found in $(pwd)"
repo_license=$(pwd)/LICENSE
archive=$(cd "$(dirname "$archive")" && pwd)/$(basename "$archive")
file=$(basename "$archive")
[ "$file" = "bdock_${version}_${os}_${arch}.tar.gz" ] || fail "unexpected archive name: $file"

# Preserve legacy sidecar precedence, including refusal of a corrupt sidecar.
# SourceSteward supplies a global file containing other targets and the manifest;
# select exactly one canonical entry for this archive without writing beside it.
if [ -e "$archive.sha256" ] || [ -L "$archive.sha256" ]; then
	[ -f "$archive.sha256" ] || fail "invalid checksum sidecar: $file.sha256"
	checksum=$(cat "$archive.sha256")
else
	checksums=$(dirname "$archive")/SHA256SUMS
	[ -f "$checksums" ] || fail "checksum sidecar or SHA256SUMS missing for $file"
	checksum=$(awk -v name="$file" '
		$2 == name || $2 == "*" name {
			count++
			if (length($1) != 64 || $1 !~ /^[0-9a-f]+$/ || $0 != $1 "  " name) invalid=1
			entry=$0
		}
		END { if (count != 1 || invalid) exit 1; print entry }
	' "$checksums") || fail "expected exactly one valid checksum for $file"
fi
if command -v sha256sum >/dev/null 2>&1; then
	printf '%s\n' "$checksum" | (cd "$(dirname "$archive")" && sha256sum -c - >/dev/null) || fail "checksum mismatch for $file"
else
	printf '%s\n' "$checksum" | (cd "$(dirname "$archive")" && shasum -a 256 -c - >/dev/null) || fail "checksum mismatch for $file"
fi

members=$(tar -tzf "$archive" | LC_ALL=C sort | tr '\n' ' ') || fail "cannot list $file"
[ "$members" = "LICENSE PROVENANCE bdock " ] || fail "unexpected archive members: $members"
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
tar -xzf "$archive" -C "$tmp" || fail "cannot extract $file"
bin=$tmp/bdock
[ -x "$bin" ] || fail "bdock is not executable"
cmp -s "$repo_license" "$tmp/LICENSE" || fail "archive LICENSE differs from the repository LICENSE"

for line in "version=$version" "command=bdock" "source_revision=$commit" "source_modified=false" "GOOS=$os" "GOARCH=$arch" "CGO_ENABLED=0"; do
	grep -Fxq "$line" "$tmp/PROVENANCE" || fail "PROVENANCE lacks $line"
done

got=$("$bin" --version) || fail "bdock --version failed"
[ "$got" = "bdock version $version" ] || fail "bdock --version printed '$got', want 'bdock version $version'"
"$bin" --help >"$tmp/help" || fail "bdock --help failed"
grep -q 'tools' "$tmp/help" || fail "bdock --help does not list tools"
"$bin" tools --help >"$tmp/tools-help" || fail "bdock tools --help failed"
grep -q 'volume' "$tmp/tools-help" || fail "bdock tools --help does not list volume"
grep -q 'upgrade' "$tmp/tools-help" || fail "bdock tools --help does not list upgrade"
"$bin" completion bash >"$tmp/completion" || fail "bdock completion bash failed"
[ -s "$tmp/completion" ] || fail "bdock completion bash printed nothing"
printf 'release-smoke: %s ok\n' "$file"
