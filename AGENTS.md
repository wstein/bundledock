# BundleDock repository

BundleDock owns the Go CLI, declarative tool inputs, OCI build and smoke tests,
license notices, publication workflow, and brand assets. Anchorage is the
separate umbrella repository that owns consumer devcontainer integration.

Run `make check` before handing over Go or build changes. Routine source tests,
vet and their compiler verification may run automatically. Obtain the user's
explicit approval before any artifact, CLI release, OCI image, or documentation
build, including `make build`, `make release`, `make release-test`, `make docs`,
`make docs-test`, `make docs-serve`, or Docker build/smoke checks. Report deferred
checks and platform coverage accurately; a passing source check is not image or
release readiness.

Builds and releases require explicit user approval of the intended operation
and source revision. The approved automation exception covers toolbox,
code-server, Node LTS, Node Current, Bun, Codex, Antigravity, Claude and OpenTofu updater PRs authored by `github-actions[bot]`, changing
only their own lock and generated inventory. A latest non-dismissed `wstein`
APPROVED review of the exact updater head authorizes source CI, a protected
SHA-guarded rebase merge, and publication of that exact frozen lock. Source CI
must succeed on the approved head and GitHub's required `check` protection must
be satisfied. Changed heads, a main advance, unrelated files, stale approvals,
and an approved tree differing from the merged tree stop automation; do not
rewrite the updater head or bypass protection. There is no later environment
approval. Initial publication of Node Current, Codex, Antigravity, Claude and OpenTofu carriers requires
separate explicit approval of the exact full source revision and intended
build/publication operation; an updater PR cannot authorize a first publication.
Verify repository visibility and robot access before that operation. Existing environment settings remain untouched. Manual publication
retains exact full-SHA human dispatch. OpenTofu initial publication requires
this manual gate; a bot updater cannot bootstrap an absent stable baseline. Source PRs, documentation PRs, schedules,
bot activity and local commits alone supply no artifact build approval.
Tagging and publication must follow the explicitly authorized workflow scope;
consumer activation remains a separate explicit authorization. Successful approved
publication includes one annotated `<bundle>/<immutable-version>` source tag,
after verification of the final index and image aliases. Never overwrite source
tags or create moving Git aliases. Backfills require separate explicit approval
and independently verified deployed-image/source/workflow provenance. Do not perform
live merges, workflow runs, builds or settings changes while authoring source.

Third-party tools must use official stable prebuilt releases. Never build them
from source or silently patch their binaries or embedded dependencies. If a
security fix is not yet available in an official stable prebuilt release, wait
for that release and record the remaining findings in existing documentation.
This does not restrict compiling BundleDock's own CLI, builder, or initializer.

Before the next release, review every item in the local TODO list and its
supporting evidence. Resolve unfinished or blocked items, or obtain an explicit
user decision to defer each one, before creating a release tag or publishing.
Keep the local TODO list untracked.

Make multiple atomic, focused commits using Conventional Commits.
Keep each commit limited to one logical change.

Use test-driven development (TDD): write a failing test before implementing behavior changes, make it pass, then refactor.

## Common CLI tools

The tools OCI image bundles the commands below. Rebuild the tools image and
reinitialize its volume to make additions available; the current devcontainer
still pins an older image, so do not assume these commands exist in a running session.

| Task | Tool |
| --- | --- |
| Search text | Reach first for `rg`; use `rg --files` to list files. |
| Filter lists interactively | `fzf`. |
| View highlighted file content | `bat` (optional pager). |
| Inspect OCI registries | `regctl`. |
| Find files by name | `fd` or its `fdfind` alias. |
| Query code structure | `ast-grep`; use the full binary name, never `sg`. |
| Process structured data | `jq` for JSON; `yq` for YAML. |
| Manage Python environments and run Python tools | `uv` and `uvx`. |
| GitHub workflows | `gh`. |
| Inspect stale Git branches | `deadbranch` (start with `deadbranch list`). |
| Manage terminal workspaces and agent sessions | `herdr`. |
| Use the preferred interactive shell | `fish`. |
| Use an alternative structured shell | `nu` (Nushell). |
| Use an alternative terminal multiplexer | `tmux`. |
| Monitor processes and system resources | `btm` (bottom) and `btop`. |
| Customize a shell prompt | `starship`. |
| Edit text in the terminal | `hx` (Helix). |
| Build static sites | `hugo` (standard). |
| Check shell scripts | `shellcheck`, `shfmt`. |
| Run project recipes | `just`. |
| Lint and format Python | `ruff`. |
| View diffs | `delta`. |
| Watch files and benchmark commands | `watchexec`, `hyperfine`. |
| Encrypt files | `age`, `age-keygen`, `age-inspect`. |
| Check spelling | `typos`. |
| Check links | `lychee`. |
| Detect secrets | `gitleaks`. |
| Scan vulnerabilities and configuration | `trivy`. |
| Generate software bills of materials | `syft`. |

Keep HOME, caches, and virtual environments in writable locations outside the
read-only `/opt/bundledock/toolbox` tool bundle.
