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
and source revision. Pushes, merges, schedules, bot updates, and lock-only PRs
must never dispatch a native build or publication automatically. Toolbox
publication is manual-only from default `main`, with the user's full approved
SHA matching the run revision. Do not tag, publish, activate an image, or push
Git changes without explicit authorization. A local commit or merge supplies
none of those approvals.

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
| Find files by name | `fd` or its `fdfind` alias. |
| Query code structure | `ast-grep`; use the full binary name, never `sg`. |
| Process structured data | `jq` for JSON; `yq` for YAML. |
| Manage Python environments and run Python tools | `uv` and `uvx`. |
| Agent and GitHub workflows | `codex`, `claude`, `agy`, and `gh`. |
| Manage terminal workspaces and agent sessions | `herdr`. |
| Use the preferred interactive shell | `fish`. |
| Use an alternative structured shell | `nu` (Nushell). |
| Use an alternative terminal multiplexer | `tmux`. |
| Monitor processes and system resources | `btm` (bottom) and `btop`. |
| Customize a shell prompt | `starship`. |
| Build static sites | `hugo` (standard). |
| Check spelling | `typos`. |
| Check links | `lychee`. |
| Detect secrets | `gitleaks`. |
| Scan vulnerabilities and configuration | `trivy`. |
| Generate software bills of materials | `syft`. |

Keep HOME, caches, and virtual environments in writable locations outside the
read-only `/opt/bundledock/toolbox` tool bundle.
