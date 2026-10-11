#!/usr/bin/env bash
# Lock/inventory resolution and PR creation only. Never build, publish or activate tools.
set -euo pipefail
lock=bundles/toolbox/tools.lock.json
inventory=docs/tools.md
: "${UPGRADE_RESULT:?set UPGRADE_RESULT to a temporary directory}"
: "${GITHUB_OUTPUT:?set GITHUB_OUTPUT}"
: "${GITHUB_STEP_SUMMARY:?set GITHUB_STEP_SUMMARY}"

require_clean() {
  if [[ -n $(git status --porcelain --untracked-files=all) ]]; then
    echo 'Tool upgrade requires a clean checkout.' >&2
    exit 1
  fi
}
require_lock_only() {
  local status
  status=$(git status --porcelain --untracked-files=all)
  if [[ -n "$status" && "$status" != " M $lock" ]]; then
    echo 'Refusing upgrade: files other than the existing tool lock changed.' >&2
    printf '%s\n' "$status" >&2
    exit 1
  fi
}

require_proposal_only() {
  local line
  while IFS= read -r line; do
    if [[ -n "$line" && "$line" != " M $lock" && "$line" != " M $inventory" ]]; then
      echo 'Refusing upgrade: only the tool lock and generated inventory may change.' >&2
      exit 1
    fi
  done < <(git status --porcelain --untracked-files=all)
}

case "${1:-}" in
  resolve)
    require_clean
    tool=${TOOL:-}
    # The resolver validates membership/constraints too; reject malformed inputs
    # here before invoking Go. An empty input means all desired tools.
    if [[ -n "$tool" ]]; then
      if [[ ! "$tool" =~ ^[a-z0-9][a-z0-9-]*$ ]] ||
         ! awk -F '[<>=!~[:space:]]' -v tool="$tool" '$1 == tool { found=1 } END { exit !found }' bundles/toolbox/tools.txt; then
        echo 'TOOL must be empty or a tool name from bundles/toolbox/tools.txt.' >&2
        exit 1
      fi
      go run ./cmd/bundledock tools upgrade "$tool"
    else
      go run ./cmd/bundledock tools upgrade
    fi
    require_lock_only
    python3 scripts/tool-inventory.py
    require_proposal_only
    mkdir -p "$UPGRADE_RESULT"
    git diff -- "$lock" "$inventory" > "$UPGRADE_RESULT/lock.diff"
    if git diff --quiet -- "$lock" "$inventory"; then
      echo 'changed=false' >> "$GITHUB_OUTPUT"
      echo 'No tool lock or inventory changes; no branch or pull request will be created.' >> "$GITHUB_STEP_SUMMARY"
    else
      cp "$lock" "$UPGRADE_RESULT/tools.lock.json"
      cp "$inventory" "$UPGRADE_RESULT/tools.md"
      echo 'changed=true' >> "$GITHUB_OUTPUT"
      {
        echo '## Proposed tool lock and inventory changes'
        echo
        echo 'Resolution verified official upstream archives for both supported architectures. Review the lock diff before merging. No image, CLI release, documentation build or activation was run.'
        echo
        echo '```diff'
        cat "$UPGRADE_RESULT/lock.diff"
        echo '```'
      } >> "$GITHUB_STEP_SUMMARY"
    fi
    ;;
  publish)
    require_clean
    : "${GITHUB_REPOSITORY:?set GITHUB_REPOSITORY}"
    : "${DEFAULT_BRANCH:?set DEFAULT_BRANCH}"
    : "${GITHUB_RUN_ID:?set GITHUB_RUN_ID}"
    : "${GITHUB_RUN_ATTEMPT:?set GITHUB_RUN_ATTEMPT}"
    [[ "$GITHUB_RUN_ID" =~ ^[0-9]+$ && "$GITHUB_RUN_ATTEMPT" =~ ^[0-9]+$ ]] || exit 1
    [[ -f "$UPGRADE_RESULT/tools.lock.json" && ! -L "$UPGRADE_RESULT/tools.lock.json" ]] || exit 1
    [[ -f "$UPGRADE_RESULT/tools.md" && ! -L "$UPGRADE_RESULT/tools.md" ]] || exit 1
    cp "$UPGRADE_RESULT/tools.lock.json" "$lock"
    # Generate from the checked-out prose and transferred lock. Never copy an
    # artifact's entire Markdown document into the source tree unchecked.
    python3 scripts/tool-inventory.py
    if ! cmp -s "$inventory" "$UPGRADE_RESULT/tools.md"; then
      echo 'Refusing PR: documentation artifact differs from the generated inventory.' >&2
      exit 1
    fi
    require_proposal_only
    if git diff --quiet -- "$lock" "$inventory"; then
      echo 'No tool lock or inventory changes; no branch or pull request will be created.' >> "$GITHUB_STEP_SUMMARY"
      exit 0
    fi
    branch="automation/tool-upgrade-$GITHUB_RUN_ID-$GITHUB_RUN_ATTEMPT"
    git switch -c "$branch"
    git config user.name 'github-actions[bot]'
    git config user.email '41898282+github-actions[bot]@users.noreply.github.com'
    git add -- "$lock" "$inventory"
    git commit -m 'chore(tools): upgrade stable tool lock'
    # No force pushes, branch reuse, PATs or changes to repository settings.
    if ! git push origin "HEAD:refs/heads/$branch"; then
      echo "Topic push failed. An administrator must explicitly permit GITHUB_TOKEN contents:write under repository/organization Actions policy. This workflow does not change permissions or use a PAT." >&2
      exit 1
    fi
    body="$UPGRADE_RESULT/pr-body.md"
    cat > "$body" <<BODY
Resolve official stable tool releases within the existing tools.txt constraints and update only tools.lock.json and the generated version inventory in docs/tools.md. No other documentation content changes. Both supported architectures were resolved and verified by the existing BundleDock manager.

Review the lock diff and run the required source checks before merging. This workflow performs no image, CLI release or documentation build, publication, or consumer activation. Those operations need separate explicit approval.

Run: https://github.com/$GITHUB_REPOSITORY/actions/runs/$GITHUB_RUN_ID (attempt $GITHUB_RUN_ATTEMPT).

Pull requests created with GITHUB_TOKEN do not automatically trigger ordinary pull_request workflows. Run CI manually on this branch, or follow the repository's human review process before merging.
BODY
    if ! url=$(gh pr create --repo "$GITHUB_REPOSITORY" --base "$DEFAULT_BRANCH" --head "$branch" --title 'chore(tools): upgrade stable tool lock' --body-file "$body" 2>&1); then
      printf '%s\n' "$url" >&2
      echo 'PR creation failed. An administrator must explicitly permit GITHUB_TOKEN contents:write and pull-requests:write, and enable Settings > Actions > General > Workflow permissions > Allow GitHub Actions to create and approve pull requests (subject to organization policy). This workflow never changes those settings or uses a PAT. The topic branch may already exist; inspect it before retrying.' >&2
      exit 1
    fi
    printf '%s\n' "$url" >> "$GITHUB_STEP_SUMMARY"
    ;;
  *) echo 'Usage: tool-upgrade.sh resolve|publish' >&2; exit 2 ;;
esac
