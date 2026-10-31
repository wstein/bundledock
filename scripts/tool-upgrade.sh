#!/usr/bin/env bash
# Lock/inventory resolution and PR creation only. Never build, publish or activate tools.
set -euo pipefail
: "${UPGRADE_RESULT:?set UPGRADE_RESULT to a temporary directory}"
: "${GITHUB_OUTPUT:?set GITHUB_OUTPUT}"
: "${GITHUB_STEP_SUMMARY:?set GITHUB_STEP_SUMMARY}"
all_bundles=(toolbox code-server node node-current bun codex agy claude)
# opentofu is retired (2026-10-31; the toolbox ships tofu): no updater proposal, no publication.
is_bundle() { local name; for name in "${all_bundles[@]}"; do [[ $1 != "$name" ]] || return 0; done; return 1; }
# nixbox is proposed by its own flow: a lock refresh in the pinned nixos/nix image
# for its Nix store paths, and the official-release resolver for its glibc release
# binaries (bundles/nixbox/tools, the toolbox schema). It is not a publishable
# bundle (all_bundles drives publication) and always gets its own pull request.
nixbox_lock=bundles/nixbox/packages.lock.json
nixbox_catalog=bundles/nixbox/packages.catalog.json
nixbox_tools=bundles/nixbox/tools
nixbox_tools_lock=$nixbox_tools/packages.lock.json
nixbox_inventory=docs/nixbox.md
nixbox_paths=("$nixbox_lock" "$nixbox_catalog" "$nixbox_tools_lock" "$nixbox_inventory")
is_upgrade_bundle() { [[ $1 == nixbox ]] || is_bundle "$1"; }
# resolve handles the one BUNDLE of its matrix job. publish combines every
# bundle whose resolved lock is in the result directory (canonical order).
bundles=()
if [[ ${1:-} == publish ]]; then
  case "${PROPOSAL_GROUP:-}" in
    '') for bundle in "${all_bundles[@]}"; do
          if [[ -e ${UPGRADE_RESULT:-}/$bundle.lock.json || -L ${UPGRADE_RESULT:-}/$bundle.lock.json ]]; then bundles+=("$bundle"); fi
        done ;;
    nixbox) if [[ -e ${UPGRADE_RESULT:-}/nixbox.lock.json || -L ${UPGRADE_RESULT:-}/nixbox.lock.json ]]; then bundles=(nixbox); fi ;;
    *) echo 'PROPOSAL_GROUP must be empty or nixbox.' >&2; exit 1 ;;
  esac
else
  is_upgrade_bundle "${BUNDLE:-toolbox}" || { echo 'BUNDLE must be toolbox, code-server, node, node-current, bun, codex, agy, claude or nixbox.' >&2; exit 1; }
  bundles=("${BUNDLE:-toolbox}")
fi
locks=() inventories=() artifacts=()
for bundle in ${bundles[@]+"${bundles[@]}"}; do
  locks+=("bundles/$bundle/packages.lock.json")
  if [[ $bundle == nixbox ]]; then
    # The Nix lock, the catalog (nixpkgs pin), the lock of the official release binaries
    # and the generated inventory of those binaries are the own files.
    artifacts+=(nixbox.lock.json nixbox.catalog.json nixbox.tools.lock.json nixbox.md)
  elif [[ $bundle == toolbox ]]; then
    inventories+=(docs/toolbox.md)
    artifacts+=(toolbox.lock.json toolbox.md)
  else
    inventories+=("docs/$bundle.md")
    artifacts+=("$bundle.lock.json" "$bundle.md")
  fi
done
paths=(${locks[@]+"${locks[@]}"})
if [[ ${#inventories[@]} -gt 0 ]]; then paths+=("${inventories[@]}"); fi
proposal_name=tool
if [[ ${bundles[0]:-} == toolbox ]]; then proposal_name=toolbox; fi
needs_base=false
for bundle in ${bundles[@]+"${bundles[@]}"}; do [[ $bundle == toolbox ]] || needs_base=true; done

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
  python3 - "$UPGRADE_RESULT" "$1" ${artifacts[@]+"${artifacts[@]}"} <<'PYTHON'
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
    if not root.exists() and not expected: raise SystemExit(0)
    if not root.is_dir(): raise SystemExit('Missing artifact directory')
    actual={p.name for p in root.iterdir()}
    if not expected<=actual or actual-expected-{'lock.diff'}: raise SystemExit('Unexpected or missing artifact files')
    for p in root.iterdir():
        if not stat.S_ISREG(p.lstat().st_mode): raise SystemExit('Artifact must be a regular file')
PYTHON
}

# Move the nixpkgs pin to the head of nixpkgs-unstable, refresh the lock in the
# pinned nixos/nix image, and resolve the official release binaries to their newest
# stable releases. `nixbox check` is the sanity check. Nothing is built.
nixbox_resolve() {
  if [[ -n ${TOOL:-} ]]; then
    echo 'TOOL is not supported for nixbox: its lock is one closed list.' >&2; exit 1
  fi
  sh bundles/nixbox/scripts/bump-rev.sh
  sh bundles/nixbox/scripts/refresh-lock.sh
  go run ./cmd/bdock tools upgrade --directory "$nixbox_tools"
  require_changes_only "$nixbox_lock" "$nixbox_catalog" "$nixbox_tools_lock"
  go run ./cmd/nixbox check -catalog "$nixbox_catalog" -lock "$nixbox_lock" -tools "$nixbox_tools"
  python3 scripts/tool-inventory.py --bundle nixbox
  require_changes_only "${nixbox_paths[@]}"
  mkdir -p "$UPGRADE_RESULT"
  git diff -- "${nixbox_paths[@]}" > "$UPGRADE_RESULT/lock.diff"
  if git diff --quiet -- "${nixbox_paths[@]}"; then
    echo 'changed=false' >> "$GITHUB_OUTPUT"
    echo 'No nixbox changes; no branch or pull request will be created.' >> "$GITHUB_STEP_SUMMARY"
  else
    cp "$nixbox_lock" "$UPGRADE_RESULT/nixbox.lock.json"
    cp "$nixbox_catalog" "$UPGRADE_RESULT/nixbox.catalog.json"
    cp "$nixbox_tools_lock" "$UPGRADE_RESULT/nixbox.tools.lock.json"
    cp "$nixbox_inventory" "$UPGRADE_RESULT/nixbox.md"
    git rev-parse HEAD > "$UPGRADE_RESULT/base.sha"
    echo 'changed=true' >> "$GITHUB_OUTPUT"
    {
      echo '## Proposed nixbox update'
      echo
      echo '```diff'
      head -c 500000 "$UPGRADE_RESULT/lock.diff"
      echo '```'
    } >> "$GITHUB_STEP_SUMMARY"
  fi
}

# One nixbox-only pull request on its own topic (publication never recognizes it).
nixbox_publish() {
  : "${GITHUB_REPOSITORY:?set GITHUB_REPOSITORY}"
  : "${DEFAULT_BRANCH:?set DEFAULT_BRANCH}"
  : "${GITHUB_RUN_ID:?set GITHUB_RUN_ID}"
  : "${GITHUB_RUN_ATTEMPT:?set GITHUB_RUN_ATTEMPT}"
  [[ $GITHUB_RUN_ID =~ ^[0-9]+$ && $GITHUB_RUN_ATTEMPT =~ ^[0-9]+$ ]] || exit 1
  check_result publish
  if [[ ${#bundles[@]} -eq 0 ]]; then
    echo 'No nixbox changes; no branch or pull request will be created.' >> "$GITHUB_STEP_SUMMARY"; exit 0
  fi
  local base; base=$(cat "$UPGRADE_RESULT/base.sha")
  if [[ ! $base =~ ^[0-9a-f]{40}$ || $base != "$(git rev-parse HEAD)" ]]; then
    echo 'Refusing stale proposal: artifact base differs from checkout.' >&2; exit 1
  fi
  local old_rev new_rev
  old_rev=$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["nixpkgs"]["rev"])' "$nixbox_catalog")
  cp "$UPGRADE_RESULT/nixbox.lock.json" "$nixbox_lock"
  cp "$UPGRADE_RESULT/nixbox.catalog.json" "$nixbox_catalog"
  cp "$UPGRADE_RESULT/nixbox.tools.lock.json" "$nixbox_tools_lock"
  new_rev=$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["nixpkgs"]["rev"])' "$nixbox_catalog")
  go run ./cmd/nixbox check -catalog "$nixbox_catalog" -lock "$nixbox_lock" -tools "$nixbox_tools"
  # Generate from checked-out prose. Never accept unrelated artifact prose.
  python3 scripts/tool-inventory.py --bundle nixbox
  if ! cmp -s "$nixbox_inventory" "$UPGRADE_RESULT/nixbox.md"; then
    echo 'Refusing PR: documentation artifact differs from the generated inventory.' >&2; exit 1
  fi
  require_changes_only "${nixbox_paths[@]}"
  if git diff --quiet -- "${nixbox_paths[@]}"; then
    echo 'No nixbox changes; no branch or pull request will be created.' >> "$GITHUB_STEP_SUMMARY"; exit 0
  fi
  local tool_lines
  tool_lines=$(python3 - "$nixbox_tools_lock" <<'PYTHON'
import json, subprocess, sys
path = sys.argv[1]
old = json.loads(subprocess.check_output(['git', 'show', 'HEAD:' + path], text=True))['platforms']['amd64']
new = json.load(open(path))['platforms']['amd64']
for tool in sorted(new):
    before = old.get(tool, {}).get('version')
    if before != new[tool]['version']:
        print(f"- {tool}: {before} -> {new[tool]['version']}")
PYTHON
)
  local branch="automation/nixbox-upgrade-$GITHUB_RUN_ID-$GITHUB_RUN_ATTEMPT"
  local body="$UPGRADE_RESULT/pr-body.md" title
  if [[ $old_rev != "$new_rev" && -n $tool_lines ]]; then title="chore(nixbox): bump nixpkgs to ${new_rev:0:12} and upgrade official tools"
  elif [[ -n $tool_lines ]]; then title='chore(nixbox): upgrade official tools'
  else title="chore(nixbox): bump nixpkgs to ${new_rev:0:12}"; fi
  git switch -c "$branch"
  git config user.name 'github-actions[bot]'
  git config user.email '41898282+github-actions[bot]@users.noreply.github.com'
  git add -- "${nixbox_paths[@]}"
  git commit -m "$title"
  {
    if [[ $old_rev != "$new_rev" ]]; then
      printf 'Bumps nixpkgs from `%s` to `%s` (head of nixpkgs-unstable) and refreshes `%s`.\n\n' "$old_rev" "$new_rev" "$nixbox_lock"
    fi
    if [[ -n $tool_lines ]]; then
      printf 'Upgrades the official release binaries in `%s`:\n\n%s\n\n' "$nixbox_tools_lock" "$tool_lines"
    fi
  } > "$body"
  cat >> "$body" <<BODY
The check is the dispatched source CI. The build and smoke (\`nixbox-build.yml\`) is an owner-dispatched run on main; nothing was built or published here.

Run: https://github.com/$GITHUB_REPOSITORY/actions/runs/$GITHUB_RUN_ID (attempt $GITHUB_RUN_ATTEMPT)
BODY
  if ! git push origin "HEAD:refs/heads/$branch"; then
    echo 'Topic push failed. An administrator must explicitly permit GITHUB_TOKEN contents:write under repository/organization Actions policy. This workflow does not change permissions or use a PAT.' >&2; exit 1
  fi
  local url
  if ! url=$(gh pr create --repo "$GITHUB_REPOSITORY" --base "$DEFAULT_BRANCH" --head "$branch" --title "$title" --body-file "$body" 2>&1); then
    printf '%s\n' "$url" >&2
    echo 'PR creation failed. An administrator must explicitly permit GITHUB_TOKEN contents:write and pull-requests:write, and enable Settings > Actions > General > Workflow permissions > Allow GitHub Actions to create and approve pull requests (subject to organization policy). This workflow never changes those settings or uses a PAT. The topic branch may already exist; inspect it before retrying.' >&2; exit 1
  fi
  printf '%s\n' "$url" >> "$GITHUB_STEP_SUMMARY"
  python3 scripts/updater-source-ci.py --bundles nixbox --branch "$branch" \
    --base "$base" --head "$(git rev-parse HEAD)" --pr-url "$url" >> "$GITHUB_STEP_SUMMARY"
}

case "${1:-}" in
  resolve)
    require_clean
    check_result resolve
    tool=${TOOL:-}
    if [[ ${bundles[0]} == nixbox ]]; then nixbox_resolve; exit 0; fi
    for bundle in ${bundles[@]+"${bundles[@]}"}; do
      if [[ -n $tool ]]; then
        if [[ ! $tool =~ ^[a-z0-9][a-z0-9-]*$ ]] ||
           ! awk -F '[<>=!~[:space:]]' -v tool="$tool" '$1 == tool { found=1 } END { exit !found }' "bundles/$bundle/packages.txt"; then
          echo "TOOL must be empty or a tool name from bundles/$bundle/packages.txt." >&2; exit 1
        fi
      fi
      args=(); [[ -z $tool ]] || args+=("$tool")
      [[ $bundle == toolbox ]] || args+=(--directory "bundles/$bundle")
      go run ./cmd/bdock tools upgrade ${args[@]+"${args[@]}"}
    done
    require_changes_only ${locks[@]+"${locks[@]}"}
    for bundle in ${bundles[@]+"${bundles[@]}"}; do
      python3 scripts/tool-inventory.py --bundle "$bundle"
    done
    require_changes_only ${paths[@]+"${paths[@]}"}
    mkdir -p "$UPGRADE_RESULT"
    git diff -- ${paths[@]+"${paths[@]}"} > "$UPGRADE_RESULT/lock.diff"
    if git diff --quiet -- ${paths[@]+"${paths[@]}"}; then
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
    if [[ ${PROPOSAL_GROUP:-} == nixbox ]]; then nixbox_publish; exit 0; fi
    : "${GITHUB_REPOSITORY:?set GITHUB_REPOSITORY}"
    : "${DEFAULT_BRANCH:?set DEFAULT_BRANCH}"
    : "${GITHUB_RUN_ID:?set GITHUB_RUN_ID}"
    : "${GITHUB_RUN_ATTEMPT:?set GITHUB_RUN_ATTEMPT}"
    [[ $GITHUB_RUN_ID =~ ^[0-9]+$ && $GITHUB_RUN_ATTEMPT =~ ^[0-9]+$ ]] || exit 1
    check_result publish
    # Bundles whose resolution failed leave one marker file named after them.
    # They are not part of the proposal; report them and keep the rest.
    failed=()
    if [[ -n ${FAILED_BUNDLES:-} && -d $FAILED_BUNDLES ]]; then
      for marker in "$FAILED_BUNDLES"/*; do
        [[ -e $marker ]] || continue
        is_bundle "$(basename "$marker")" || { echo 'Refusing unknown failed-bundle marker.' >&2; exit 1; }
      done
      for bundle in "${all_bundles[@]}"; do [[ ! -e $FAILED_BUNDLES/$bundle ]] || failed+=("$bundle"); done
    fi
    failed_list=$(IFS=,; echo "${failed[*]-}")
    if [[ ${#failed[@]} -gt 0 ]]; then
      echo "Resolution failed for: ${failed_list//,/, }. The other bundles are still proposed; a later run retries these." >> "$GITHUB_STEP_SUMMARY"
    fi
    if [[ ${#bundles[@]} -eq 0 ]]; then
      echo 'No lock or inventory changes; no branch or pull request will be created.' >> "$GITHUB_STEP_SUMMARY"; exit 0
    fi
    if [[ $needs_base == true ]]; then
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
    require_changes_only ${paths[@]+"${paths[@]}"}
    if git diff --quiet -- ${paths[@]+"${paths[@]}"}; then
      echo 'No lock or inventory changes; no branch or pull request will be created.' >> "$GITHUB_STEP_SUMMARY"; exit 0
    fi
    proposal_base=$(git rev-parse HEAD)
    # One bundle keeps its own topic name (publication recognises it); several share one.
    branch="automation/upgrades-$GITHUB_RUN_ID-$GITHUB_RUN_ATTEMPT"
    if [[ ${#bundles[@]} -eq 1 ]]; then
      branch="automation/${bundles[0]}-upgrade-$GITHUB_RUN_ID-$GITHUB_RUN_ATTEMPT"
      if [[ ${bundles[0]} == toolbox ]]; then branch="automation/toolbox-upgrade-$GITHUB_RUN_ID-$GITHUB_RUN_ATTEMPT"; fi
    fi
    git switch -c "$branch"
    git config user.name 'github-actions[bot]'
    git config user.email '41898282+github-actions[bot]@users.noreply.github.com'
    git add -- ${paths[@]+"${paths[@]}"}
    body="$UPGRADE_RESULT/pr-body.md"
    commit_msg_file=$(mktemp)
    trap 'rm -f "$commit_msg_file"' EXIT
    bundle_list=$(IFS=,; echo "${bundles[*]-}")
    title=$(python3 - "$bundle_list" "$proposal_base" "$body" "$commit_msg_file" "$GITHUB_REPOSITORY" "$GITHUB_RUN_ID" "$GITHUB_RUN_ATTEMPT" "$failed_list" <<'PYTHON'
import json, subprocess, sys
from pathlib import Path

bundle_list, base, pr_body_path, commit_msg_path, repo, run_id, attempt, failed_list = sys.argv[1:]
bundles = bundle_list.split(',')


def describe(bundle):
    """Return (title, commit message, table, bullets) for one bundle."""
    lock_path = Path('bundles') / bundle / 'packages.lock.json'
    catalog_path = Path('bundles') / bundle / 'packages.catalog.json'
    old_lock = json.loads(subprocess.check_output(['git', 'show', f'{base}:{lock_path}'], text=True))
    new_lock = json.loads(lock_path.read_text())
    catalog = json.loads(catalog_path.read_text())['packages']

    changes = []
    for tool in sorted(new_lock['platforms']['amd64']):
        old_raw = old_lock['platforms']['amd64'][tool]['version']
        new_raw = new_lock['platforms']['amd64'][tool]['version']
        if old_raw == new_raw:
            continue
        pkg = catalog.get(tool, {})
        prefix = pkg.get('tag_prefix', '')
        old_ver = old_raw.removeprefix(prefix)
        new_ver = new_raw.removeprefix(prefix)
        provider = pkg.get('provider', '')
        upstream = pkg.get('repository', '')
        if provider == 'github' and upstream:
            source = f'[{upstream}@{new_raw}](https://github.com/{upstream}/releases/tag/{new_raw})'
        elif upstream:
            source = f'[{upstream}](https://github.com/{upstream})'
        elif provider == 'node':
            source = f'[nodejs.org@{new_raw}](https://nodejs.org/dist/{new_raw}/)'
        else:
            source = f'`{new_raw}`'
        changes.append((tool, old_ver, new_ver, source))

    if not changes:
        title = f'chore({bundle}): sync generated tool inventory'
        return title, title + '\n', f'Sync generated `docs/{bundle}.md` inventory with `{lock_path}`.', ['- sync generated inventory']
    if len(changes) <= 3:
        title = f'chore({bundle}): upgrade ' + ', '.join(f'{t} {o} -> {n}' for t, o, n, _ in changes)
    else:
        title = f'chore({bundle}): upgrade {len(changes)} tools (' + ', '.join(t for t, _, _, _ in changes) + ')'
    bullets = [f'- {t}: {o} -> {n}' for t, o, n, _ in changes]
    commit_msg = title + '\n' if len(changes) == 1 else title + '\n\n' + '\n'.join(bullets) + '\n'
    rows = ['| Tool | Previous | New | Source |', '| --- | --- | --- | --- |']
    rows.extend(f'| `{t}` | `{o}` | `{n}` | {src} |' for t, o, n, src in changes)
    return title, commit_msg, '\n'.join(rows), bullets


parts = [describe(bundle) for bundle in bundles]
if len(bundles) == 1:
    title, commit_msg, table, _ = parts[0]
else:
    title = 'chore(upgrades): upgrade ' + ', '.join(bundles)
    sections = [f'{name}:\n' + '\n'.join(part[3]) for name, part in zip(bundles, parts)]
    commit_msg = title + '\n\n' + '\n\n'.join(sections) + '\n'
    table = '\n\n'.join(f'### {name}\n\n{part[2]}' for name, part in zip(bundles, parts))
if failed_list:
    table += f'\n\nResolution failed for: {failed_list.replace(",", ", ")}. These bundles are not part of this pull request; a later run retries them.'
Path(commit_msg_path).write_text(commit_msg)
Path(pr_body_path).write_text(f'{table}\n\nRun: https://github.com/{repo}/actions/runs/{run_id} (attempt {attempt})\n')
print(title)
PYTHON
)
    git commit -F "$commit_msg_file"
    if ! git push origin "HEAD:refs/heads/$branch"; then
      echo 'Topic push failed. An administrator must explicitly permit GITHUB_TOKEN contents:write under repository/organization Actions policy. This workflow does not change permissions or use a PAT.' >&2; exit 1
    fi
    if ! url=$(gh pr create --repo "$GITHUB_REPOSITORY" --base "$DEFAULT_BRANCH" --head "$branch" --title "$title" --body-file "$body" 2>&1); then
      printf '%s\n' "$url" >&2
      echo 'PR creation failed. An administrator must explicitly permit GITHUB_TOKEN contents:write and pull-requests:write, and enable Settings > Actions > General > Workflow permissions > Allow GitHub Actions to create and approve pull requests (subject to organization policy). This workflow never changes those settings or uses a PAT. The topic branch may already exist; inspect it before retrying.' >&2; exit 1
    fi
    printf '%s\n' "$url" >> "$GITHUB_STEP_SUMMARY"
    python3 scripts/updater-source-ci.py --bundles "$bundle_list" --branch "$branch" \
      --base "$proposal_base" --head "$(git rev-parse HEAD)" --pr-url "$url" \
      >> "$GITHUB_STEP_SUMMARY"
    ;;
  *) echo 'Usage: tool-upgrade.sh resolve|publish' >&2; exit 2 ;;
esac
