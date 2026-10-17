#!/bin/sh
# Release notes for a tag on stdout (docs/design/bdock-release-workflow).
#
# Usage: sh release-notes.sh TAG COMMIT GO_VERSION
#
# Run in a repository with full history and tags. The change list is the non-merge
# commit subjects between the nearest earlier strict bdock/vX.Y.Z tag (by version
# order) and COMMIT, or "First release." when there is none. Image tags such as
# toolbox/2026.10.15-b98.1 are never a baseline.
set -eu

[ "$#" -eq 3 ] || {
	printf 'usage: sh release-notes.sh TAG COMMIT GO_VERSION\n' >&2
	exit 2
}
tag=$1
commit=$2
gover=$3
plain=${tag#bdock/v}

prev=
if git rev-parse -q --verify "$commit^" >/dev/null; then
	prev=$(git tag --merged "$commit^" --list 'bdock/v*' --sort=-version:refname |
		grep -E '^bdock/v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$' | grep -vx "$tag" | head -n 1 || true)
fi

cat <<TEXT
bdock v$plain

Commit: $commit
Built with Go $gover, CGO disabled, -trimpath, version stamped as $plain.
Platforms: linux/amd64, linux/arm64, darwin/arm64.
Archives: bdock_${plain}_<os>_<arch>.tar.gz, each with bdock, LICENSE and PROVENANCE.

Verify a download in the directory that holds the archives and SHA256SUMS:

    sha256sum --check --ignore-missing SHA256SUMS
    # macOS, for one archive: grep ' ARCHIVE\$' SHA256SUMS | shasum -a 256 --check

then extract and run \`bdock --version\`, which prints \`bdock version $plain\`.

The binaries are not signed or notarized. SHA256SUMS detects a damaged or wrong
download; it does not prove who published the release. This release contains the
CLI only; OCI tool images are published separately and are unaffected.

TEXT
if [ -n "$prev" ]; then
	printf '## Changes since %s\n\n' "$prev"
	git log --no-merges --format='- %s (%h)' "$prev..$commit"
else
	printf '## Changes\n\nFirst release.\n'
fi
