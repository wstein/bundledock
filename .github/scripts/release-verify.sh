#!/bin/sh
# Local checks of the CLI release workflow (docs/design/bdock-release-workflow).
#
# Usage:
#   sh release-verify.sh version V        V is a strict release version X.Y.Z
#   sh release-verify.sh dryversion V     V is a dry-run label X.Y.Z-label, never a release version
#   sh release-verify.sh tag-version TAG  TAG is bdock/vX.Y.Z; prints X.Y.Z
#   sh release-verify.sh tag TAG MAIN_REF TAG is an annotated bdock/vX.Y.Z tag on a commit
#                                         reachable from MAIN_REF; prints the tagged commit
#   sh release-verify.sh monotonic TAG    TAG is bdock/vX.Y.Z; stdin lists existing tag or release
#                                         names, one per line; fails when any other strict
#                                         bdock/v tag has a version greater than or equal to TAG's.
#                                         Other lines (pre-releases, image tags, blanks) are ignored.
#
# Reads local files and git objects only; no network and no writes. Exit 0 when the
# check holds, 1 when it fails, 2 on a usage error.
set -eu

fail() {
	printf 'release-verify: %s\n' "$*" >&2
	exit 1
}
usage() {
	printf 'usage: sh release-verify.sh version V | dryversion V | tag-version TAG | tag TAG MAIN_REF | monotonic TAG\n' >&2
	exit 2
}

NL='
'
num='(0|[1-9][0-9]*)'
release_re="^$num\\.$num\\.$num\$"
dry_re='^[0-9]+\.[0-9]+\.[0-9]+-[0-9A-Za-z.]+$'
tag_re="^bdock/v$num\\.$num\\.$num\$"

# match RE VALUE: whole-value match; a newline never matches (grep works per line).
match() {
	case $2 in
	*"$NL"*) return 1 ;;
	esac
	printf '%s\n' "$2" | grep -Eq "$1"
}

[ "$#" -ge 1 ] || usage
cmd=$1
shift
case $cmd in
version)
	[ "$#" -eq 1 ] || usage
	match "$release_re" "$1" || fail "not a release version (want MAJOR.MINOR.PATCH): $1"
	;;
dryversion)
	[ "$#" -eq 1 ] || usage
	match "$dry_re" "$1" || fail "not a dry-run version: $1"
	;;
tag-version)
	[ "$#" -eq 1 ] || usage
	match "$tag_re" "$1" || fail "not a CLI release tag (want bdock/vMAJOR.MINOR.PATCH): $1"
	printf '%s\n' "${1#bdock/v}"
	;;
tag)
	[ "$#" -eq 2 ] || usage
	tag=$1
	main=$2
	match "$tag_re" "$tag" || fail "not a CLI release tag (want bdock/vMAJOR.MINOR.PATCH): $tag"
	kind=$(git cat-file -t "refs/tags/$tag" 2>/dev/null) || fail "tag $tag does not exist"
	[ "$kind" = tag ] || fail "tag $tag is not an annotated tag (object type: $kind)"
	commit=$(git rev-parse --verify "refs/tags/$tag^{commit}") || fail "tag $tag does not name a commit"
	git merge-base --is-ancestor "$commit" "$main" || fail "commit $commit of $tag is not an ancestor of $main"
	printf '%s\n' "$commit"
	;;
monotonic)
	[ "$#" -eq 1 ] || usage
	match "$tag_re" "$1" || fail "not a CLI release tag (want bdock/vMAJOR.MINOR.PATCH): $1"
	new=${1#bdock/v}
	# Compare numerically per component; the tag itself may be listed and is skipped.
	awk -v tag="$1" -v new="$new" '
		function cmp(a, b,   x, y, i) {
			split(a, x, "."); split(b, y, ".")
			for (i = 1; i <= 3; i++) { if (x[i] + 0 > y[i] + 0) return 1; if (x[i] + 0 < y[i] + 0) return -1 }
			return 0
		}
		$0 == tag { next }
		$0 ~ /^bdock\/v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$/ {
			v = substr($0, 8)
			if (cmp(v, new) >= 0) { printf "release-verify: existing %s is not below %s\n", $0, tag > "/dev/stderr"; bad = 1 }
		}
		END { exit bad }
	' || fail "version $new is not above every existing bdock/v tag; release the next version"
	;;
*)
	usage
	;;
esac
