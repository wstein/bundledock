#!/bin/sh
# Pages has a separate exact-main CI automation exception. Other publication
# callers retain the human exact-SHA dispatch gate below.
set -eu

if [ "${1:-}" = --pages ]; then
    [ "$#" -eq 1 ] || exit 1
    exec python3 -B "$(dirname "$0")/pages-approval.py"
fi
[ "$#" -eq 0 ] || exit 1

reject() {
    echo "Publication requires explicit user approval of this exact main SHA." >&2
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
