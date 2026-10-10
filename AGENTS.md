When the user invokes $crewbook, use split mode: start crewbook/desk
and one persistent crewbook/dispatch subagent, configured as
gpt-6.1-sol with medium reasoning effort. Retain and resume the same
dispatcher throughout the session.

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
| Query local databases | `sqlite3`. |
| Manage Python environments and run Python tools | `uv` and `uvx`. |
| Agent and GitHub workflows | `codex`, `claude`, `agy`, and `gh`. |
| Manage terminal workspaces and agent sessions | `herdr`. |
| Use the preferred interactive shell | `fish`. |
| Use an alternative structured shell | `nu` (Nushell). |
| Use an alternative terminal multiplexer | `tmux`. |
| Run modern Bash explicitly for automation | `/opt/workharbor/bin/bash --noprofile --norc`. |
| Customize a shell prompt | `starship`. |

Keep HOME, caches, and virtual environments in writable locations outside the
read-only `/opt/workharbor` tool bundle.
