#!/bin/sh
# A user must manually dispatch the approved default-main revision. This gate
# rejects bots; human credentials must never be used to dispatch automatically.
set -eu

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
