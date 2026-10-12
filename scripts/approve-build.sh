#!/bin/sh
# A user must manually dispatch this exact current main revision for a local
# build and smoke test. Human credentials must never dispatch automatically.
set -eu

reject() {
    echo "Build requires explicit user approval of this exact current main SHA." >&2
    exit 1
}

[ "${GITHUB_EVENT_NAME:-}" = workflow_dispatch ] || reject
[ "${DEFAULT_BRANCH:-}" = main ] || reject
[ "${GITHUB_REF:-}" = refs/heads/main ] || reject
[ "${SENDER_TYPE:-}" = User ] || reject
for actor in "${GITHUB_ACTOR:-}" "${TRIGGERING_ACTOR:-}"; do
    case "$actor" in
        ''|*'[bot]'*|github-actions) reject ;;
    esac
done
case "${APPROVED_SHA:-}" in
    ''|*[!0-9a-f]*) reject ;;
esac
[ "${#APPROVED_SHA}" -eq 40 ] || reject
[ "$APPROVED_SHA" = "${GITHUB_SHA:-}" ] || reject

# Check both the source actually on disk and main at the last point before build.
[ "$(git rev-parse HEAD)" = "$APPROVED_SHA" ] || reject
current_main=$(git ls-remote origin refs/heads/main) || reject
[ "$current_main" = "$(printf '%s\trefs/heads/main' "$APPROVED_SHA")" ] || reject
