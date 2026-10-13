#!/usr/bin/env bash
# Lock/inventory resolution and PR creation only. Never build, publish or activate tools.
set -euo pipefail
: "${UPGRADE_RESULT:?set UPGRADE_RESULT to a temporary directory}"
: "${GITHUB_OUTPUT:?set GITHUB_OUTPUT}"
: "${GITHUB_STEP_SUMMARY:?set GITHUB_STEP_SUMMARY}"
case "${BUNDLE:-toolbox}" in
  toolbox) bundles=(toolbox) ;;
  code-server|node|node-current|bun|codex|agy|claude|opentofu) bundles=("$BUNDLE") ;;
  *) echo 'BUNDLE must be toolbox, code-server, node, node-current, bun, codex, agy, claude or opentofu.' >&2; exit 1 ;;
esac
locks=() inventories=() artifacts=()
for bundle in "${bundles[@]}"; do
  locks+=("bundles/$bundle/packages.lock.json")
  if [[ $bundle == toolbox ]]; then
    inventories+=(docs/toolbox.md)
    artifacts+=(toolbox.lock.json toolbox.md)
  else
    inventories+=("docs/$bundle.md")
    artifacts+=("$bundle.lock.json" "$bundle.md")
  fi
done
paths=("${locks[@]}" "${inventories[@]}")
proposal_name=tool
if [[ ${bundles[0]} == toolbox ]]; then proposal_name=toolbox; fi

require_clean() {
  if [[ -n $(git status --porcelain --untracked-files=all) ]]; then
    echo 'Tool upgrade requires a clean checkout.' >&2; exit 1
  fi
}
require_changes_only() {
  local line candidate allowed
  while IFS= read -r line; do
    [[ -z $line ]] && continue
    allowed=false
    for candidate in "$@"; do
      [[ $line != " M $candidate" ]] || allowed=true
    done
    if [[ $allowed != true ]]; then
      echo 'Refusing upgrade: only the selected existing locks and generated inventories may change.' >&2; exit 1
    fi
  done < <(git status --porcelain --untracked-files=all)
}
# Check the result directory and every existing parent without following links.
# Resolve requires an empty directory; publish permits only the selected files.
check_result() {
  python3 - "$UPGRADE_RESULT" "$1" "${artifacts[@]}" <<'PYTHON'
import os, stat, sys
from pathlib import Path
root=Path(os.path.abspath(sys.argv[1]))
for component in (*reversed(root.parents),root):
    if component.is_symlink(): raise SystemExit('Refusing linked artifact path')
if root.exists() and not root.is_dir(): raise SystemExit('Invalid artifact directory')
if sys.argv[2]=='resolve':
    if root.exists() and any(root.iterdir()): raise SystemExit('Artifact directory must be empty')
else:
    expected=set(sys.argv[3:])
    if any(name.endswith(".lock.json") and name != "toolbox.lock.json" for name in expected): expected.add("base.sha")
    if not root.is_dir(): raise SystemExit('Missing artifact directory')
    actual={p.name for p in root.iterdir()}
    if not expected<=actual or actual-expected-{'lock.diff'}: raise SystemExit('Unexpected or missing artifact files')
    for p in root.iterdir():
        if not stat.S_ISREG(p.lstat().st_mode): raise SystemExit('Artifact must be a regular file')
PYTHON
}

case "${1:-}" in
  resolve)
    require_clean
    check_result resolve
    tool=${TOOL:-}
    for bundle in "${bundles[@]}"; do
      if [[ -n $tool ]]; then
        if [[ ! $tool =~ ^[a-z0-9][a-z0-9-]*$ ]] ||
           ! awk -F '[<>=!~[:space:]]' -v tool="$tool" '$1 == tool { found=1 } END { exit !found }' "bundles/$bundle/packages.txt"; then
          echo "TOOL must be empty or a tool name from bundles/$bundle/packages.txt." >&2; exit 1
        fi
      fi
      args=(); [[ -z $tool ]] || args+=("$tool")
      [[ $bundle == toolbox ]] || args+=(--directory "bundles/$bundle")
      go run ./cmd/bdock tools upgrade "${args[@]}"
    done
    require_changes_only "${locks[@]}"
    for bundle in "${bundles[@]}"; do
      python3 scripts/tool-inventory.py --bundle "$bundle"
    done
    require_changes_only "${paths[@]}"
    mkdir -p "$UPGRADE_RESULT"
    git diff -- "${paths[@]}" > "$UPGRADE_RESULT/lock.diff"
    if git diff --quiet -- "${paths[@]}"; then
      echo 'changed=false' >> "$GITHUB_OUTPUT"
      echo "No $proposal_name lock or inventory changes; no branch or pull request will be created." >> "$GITHUB_STEP_SUMMARY"
    else
      for i in "${!bundles[@]}"; do
        cp "${locks[i]}" "$UPGRADE_RESULT/${artifacts[2*i]}"
        cp "${inventories[i]}" "$UPGRADE_RESULT/${artifacts[2*i+1]}"
      done
      if [[ ${bundles[0]} != toolbox ]]; then git rev-parse HEAD > "$UPGRADE_RESULT/base.sha"; fi
      echo 'changed=true' >> "$GITHUB_OUTPUT"
      {
        echo "## Proposed $proposal_name lock and inventory changes"
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
    [[ $GITHUB_RUN_ID =~ ^[0-9]+$ && $GITHUB_RUN_ATTEMPT =~ ^[0-9]+$ ]] || exit 1
    check_result publish
    if [[ ${bundles[0]} != toolbox ]]; then
      base=$(cat "$UPGRADE_RESULT/base.sha")
      if [[ ! $base =~ ^[0-9a-f]{40}$ || $base != "$(git rev-parse HEAD)" ]]; then
        echo 'Refusing stale proposal: artifact base differs from checkout.' >&2; exit 1
      fi
    fi
    for i in "${!bundles[@]}"; do
      bundle=${bundles[i]}
      cp "$UPGRADE_RESULT/${artifacts[2*i]}" "${locks[i]}"
      if [[ $bundle != toolbox ]]; then go run ./cmd/tools-publish validate --bundle "$bundle"; fi
      # Generate from checked-out prose. Never accept unrelated artifact prose.
      python3 scripts/tool-inventory.py --bundle "$bundle"
      if ! cmp -s "${inventories[i]}" "$UPGRADE_RESULT/${artifacts[2*i+1]}"; then
        echo 'Refusing PR: documentation artifact differs from the generated inventory.' >&2; exit 1
      fi
    done
    require_changes_only "${paths[@]}"
    if git diff --quiet -- "${paths[@]}"; then
      echo "No $proposal_name lock or inventory changes; no branch or pull request will be created." >> "$GITHUB_STEP_SUMMARY"; exit 0
    fi
    branch="automation/toolbox-upgrade-$GITHUB_RUN_ID-$GITHUB_RUN_ATTEMPT"
    if [[ ${bundles[0]} != toolbox ]]; then branch="automation/${bundles[0]}-upgrade-$GITHUB_RUN_ID-$GITHUB_RUN_ATTEMPT"; fi
    git switch -c "$branch"
    git config user.name 'github-actions[bot]'
    git config user.email '41898282+github-actions[bot]@users.noreply.github.com'
    git add -- "${paths[@]}"
    title='chore(toolbox): upgrade stable toolbox lock'
    if [[ ${bundles[0]} != toolbox ]]; then title="chore(${bundles[0]}): upgrade stable runtime lock"; fi
    git commit -m "$title"
    if ! git push origin "HEAD:refs/heads/$branch"; then
      echo 'Topic push failed. An administrator must explicitly permit GITHUB_TOKEN contents:write under repository/organization Actions policy. This workflow does not change permissions or use a PAT.' >&2; exit 1
    fi
    body="$UPGRADE_RESULT/pr-body.md"
    cat > "$body" <<BODY
Resolve official stable releases within the selected bundles' existing packages.txt constraints and update only their locks and generated source version inventories. Selected bundles: ${bundles[*]}. Both supported architectures were resolved and verified by the existing BundleDock manager, and transferred locks were revalidated before this proposal.

Review the lock diff and run the required source checks. After an established published baseline, the actual human owner wstein merging this eligible updater authorizes frozen-lock publication after successful CI on the exact merged main source. A submitted APPROVED review is not required. Source merges remain human operations; no review controller merges PRs. Authorization covers only this updater lock and generated inventory. First publication requires separate explicit approval of the exact full source revision and intended build/publication operation; this updater cannot bootstrap an absent baseline. This proposal performs no artifact build; CLI releases, documentation builds, and consumer activation require separate explicit authorization.

Run: https://github.com/$GITHUB_REPOSITORY/actions/runs/$GITHUB_RUN_ID (attempt $GITHUB_RUN_ATTEMPT).

Pull requests created with GITHUB_TOKEN do not automatically trigger ordinary pull_request workflows. Run source checks through the established protected human merge path. Successful CI on the exact owner-merged main revision starts the read-only publication planner, which verifies actual owner merge identity, exact source/tree and own-file scope before frozen-lock publication. Main movement, unavailable provenance or source inconsistencies stop automatic publication.

BODY
    if ! url=$(gh pr create --repo "$GITHUB_REPOSITORY" --base "$DEFAULT_BRANCH" --head "$branch" --title "$title" --body-file "$body" 2>&1); then
      printf '%s\n' "$url" >&2
      echo 'PR creation failed. An administrator must explicitly permit GITHUB_TOKEN contents:write and pull-requests:write, and enable Settings > Actions > General > Workflow permissions > Allow GitHub Actions to create and approve pull requests (subject to organization policy). This workflow never changes those settings or uses a PAT. The topic branch may already exist; inspect it before retrying.' >&2; exit 1
    fi
    printf '%s\n' "$url" >> "$GITHUB_STEP_SUMMARY"
    ;;
  *) echo 'Usage: tool-upgrade.sh resolve|publish' >&2; exit 2 ;;
esac
