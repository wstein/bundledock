#!/bin/sh
# Release notes for a tag on stdout (docs/design/bdock-release-workflow).
#
# Usage: sh release-notes.sh TAG COMMIT GO_VERSION
#
# Run in a repository with full history and tags. The change list is the non-merge
# commit subjects between the nearest earlier strict bdock/vX.Y.Z tag (by version
# order) and COMMIT, or "First release." when there is none. Image tags such as
# toolbox/2026.10.15-b98.1 are never a baseline.
#
# Listed: every feat commit (Features), every breaking commit (!) of any other type (Breaking
# changes), every fix commit (Fixes, in a collapsed <details> block) and perf and refactor commits
# (Other changes, only when present). Documentation and all remaining commits (tests, style,
# build, CI, chores, reverts) are only counted, followed by a link to the full comparison.
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
download; it does not prove who published the release. The archives and SHA256SUMS
carry a GitHub build-provenance attestation that ties each file to this repository's
release workflow run and commit; verify one with the GitHub CLI:

    gh attestation verify ARCHIVE -R wstein/bundledock

This release contains the CLI only; OCI tool images are published separately and are unaffected.

TEXT
if [ -z "$prev" ]; then
	printf '## Changes\n\nFirst release.\n'
	exit 0
fi
printf '## Changes since %s\n\n' "$prev"
range=$prev..$commit
subjects=$(git log --no-merges --format='%s (%h)' "$range")
if [ -z "$subjects" ]; then
	printf 'No changes.\n'
	exit 0
fi
pick() { # pick ERE: the subjects matching ERE, empty when none
	printf '%s\n' "$subjects" | grep -E "$1" || true
}
count() { # count LINES: the number of lines, 0 for none
	if [ -z "$1" ]; then
		printf '0'
	else
		printf '%s\n' "$1" | wc -l | tr -d ' '
	fi
}
bullets() { # bullets LINES
	printf '%s\n' "$1" | sed -e 's/&/\&amp;/g' -e 's/</\&lt;/g' -e 's/^/- /'
}
feats=$(pick '^feat(\([^)]*\))?!?:')
breaking=$(pick '^[A-Za-z][A-Za-z0-9_-]*(\([^)]*\))?!:' | grep -Ev '^feat(\([^)]*\))?!:' || true)
fixes=$(pick '^fix(\([^)]*\))?:')
tidy=$(pick '^(perf|refactor)(\([^)]*\))?:')
docs=$(pick '^docs(\([^)]*\))?:')
ndocs=$(count "$docs")
nall=$(count "$subjects")
nother=$((nall - $(count "$feats") - $(count "$breaking") - $(count "$fixes") - $(count "$tidy") - ndocs))
if [ -n "$feats" ]; then
	printf '### Features (%s)\n\n' "$(count "$feats")"
	bullets "$feats"
	printf '\n'
fi
if [ -n "$breaking" ]; then
	printf '### Breaking changes (%s)\n\n' "$(count "$breaking")"
	bullets "$breaking"
	printf '\n'
fi
if [ -n "$fixes" ]; then
	printf '<details><summary>Fixes (%s)</summary>\n\n' "$(count "$fixes")"
	bullets "$fixes"
	printf '\n</details>\n\n'
fi
if [ -n "$tidy" ]; then
	printf '### Other changes (%s)\n\n' "$(count "$tidy")"
	bullets "$tidy"
	printf '\n'
fi
printf 'Not listed here: %s documentation and %s other commits (tests, style, CI, chores). Full comparison: https://github.com/wstein/bundledock/compare/%s...%s\n' "$ndocs" "$nother" "$prev" "$tag"
