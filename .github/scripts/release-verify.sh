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
#                                         names, one per line, and must include TAG itself; fails when any other strict
#                                         bdock/v tag has a version greater than or equal to TAG's.
#                                         Other lines (pre-releases, image tags, blanks) are ignored.
#   sh release-verify.sh ci-green SHA     a successful push run of ci.yml exists for the 40-hex commit SHA.
#                                         Needs gh and REPO (owner/name) and reads the Actions API only.
#                                         The head_sha query is retried (CI_ATTEMPTS, default 5, CI_SLEEP seconds,
#                                         default 2) because the API has answered the same query with 2 and 0
#                                         results alternately; then up to CI_PAGES (default 5) pages of 100, the newest 500
#                                         successful push runs, are filtered by head_sha client side (an older
#                                         commit must re-run CI first). Every run is re-checked here for the
#                                         commit, a success conclusion and the push event.
#
# POSIX sh (tested with dash and bash).
# Except for ci-green, reads local files and git objects only; no network and no writes. Exit 0 when the
# check holds, 1 when it fails, 2 on a usage error.
set -eu

fail() {
	printf 'release-verify: %s\n' "$*" >&2
	exit 1
}
usage() {
	printf 'usage: sh release-verify.sh version V | dryversion V | tag-version TAG | tag TAG MAIN_REF | monotonic TAG | ci-green SHA\n' >&2
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
	# Compare numerically per component. The listing must show the pushed tag itself (ls-remote
	# always does), which proves it is the real listing; empty or garbled input fails closed.
	awk -v tag="$1" -v new="$new" '
		function cmp(a, b,   x, y, i) {
			split(a, x, "."); split(b, y, ".")
			for (i = 1; i <= 3; i++) { if (x[i] + 0 > y[i] + 0) return 1; if (x[i] + 0 < y[i] + 0) return -1 }
			return 0
		}
		$0 == tag { seen = 1; next }
		$0 ~ /^bdock\/v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$/ {
			v = substr($0, 8)
			if (cmp(v, new) >= 0) { printf "release-verify: existing %s is not below %s\n", $0, tag > "/dev/stderr"; bad = 1 }
		}
		END { if (!seen) { printf "release-verify: the listing does not show %s; refusing to guess\n", tag > "/dev/stderr"; exit 1 } exit bad }
	' || fail "version $new is not above every existing bdock/v tag; release the next version"
	;;
ci-green)
	[ "$#" -eq 1 ] || usage
	sha=$1
	match '^[0-9a-f]{40}$' "$sha" || fail "not a full commit id: $sha"
	: "${REPO:?REPO must be owner/name}"
	attempts=${CI_ATTEMPTS:-5}
	sleep_s=${CI_SLEEP:-2}
	pages=${CI_PAGES:-5}
	runs="repos/$REPO/actions/workflows/ci.yml/runs"
	# Neither the head_sha filter nor the status filter of the server is trusted: every run is
	# checked here for the commit, a success conclusion and the push event.
	hf='[.workflow_runs[] | select(.head_sha == "'$sha'" and .conclusion == "success" and .event == "push")] | length'
	ff='"\(.workflow_runs | length) \(.workflow_runs | map(select(.head_sha == "'$sha'" and .conclusion == "success" and .event == "push")) | length)"'
	api_ok=0
	i=0
	while [ "$i" -lt "$attempts" ]; do
		i=$((i + 1))
		if n=$(gh api "$runs?head_sha=$sha&event=push&status=success" --jq "$hf"); then
			api_ok=1
			if [ "$n" -ge 1 ] 2>/dev/null; then exit 0; fi
		fi
		if [ "$i" -lt "$attempts" ]; then sleep "$sleep_s"; fi
	done
	# The head_sha filter was unreliable: list the newest successful push runs and filter by hand.
	p=0
	while [ "$p" -lt "$pages" ]; do
		p=$((p + 1))
		t=0
		r=
		while [ "$t" -lt "$attempts" ]; do
			t=$((t + 1))
			if r=$(gh api "$runs?event=push&status=success&per_page=100&page=$p" --jq "$ff"); then
				api_ok=1
				break
			fi
			r=
			if [ "$t" -lt "$attempts" ]; then sleep "$sleep_s"; fi
		done
		[ -n "$r" ] || break
		found=${r#* }
		if [ "$found" -ge 1 ] 2>/dev/null; then exit 0; fi
		total=${r% *}
		[ "$total" -ge 100 ] 2>/dev/null || break
	done
	if [ "$api_ok" = 0 ]; then
		fail "the GitHub API kept failing for $sha after $attempts attempts: see the errors above"
	fi
	fail "no successful CI run on push for $sha; wait for or re-run CI for that commit first ($attempts attempts of the head_sha query and a client-side listing of up to $pages pages of 100 found none; older commits than the newest $((pages * 100)) successful push runs must re-run CI first)"
	;;
*)
	usage
	;;
esac
