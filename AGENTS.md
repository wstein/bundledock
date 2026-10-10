# BundleDock repository

BundleDock owns the Go CLI, declarative tool inputs, OCI build and smoke tests,
license notices, publication workflow, and brand assets. Anchorage is the
separate umbrella repository that owns consumer devcontainer integration.

Run `make check` and `make build` before handing over Go or build changes.
Run Docker build and smoke checks when Docker is available; report platform
coverage and unavailable checks accurately. Do not publish images or push Git
changes without explicit authorization.

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
| Customize a shell prompt | `starship`. |

Keep HOME, caches, and virtual environments in writable locations outside the
read-only `/opt/workharbor` tool bundle.
