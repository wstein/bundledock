---
title: Tool catalog and upgrades
description: Inspect pinned tool versions and resolve verified upgrades for both architectures.
weight: 20
---

# Tool catalog and upgrades

[Back to BundleDock](../README.md)

## Pinned tools

The default bundle lives in `bundles/toolbox/`: `tools.txt`,
`tools.packages.json`, `tools.lock.json`, its Dockerfile, smoke scripts and licenses.
Shared Go code remains at the repository root.

`tools.lock.json` pins vendor artifact URLs and checksums for Linux `amd64`
and `arm64`; builds verify every download.

<!-- BEGIN GENERATED TOOL INVENTORY -->
| Tool | Pinned version |
| --- | --- |
| Codex | 0.162.1 |
| Claude Code | 2.1.296 |
| Antigravity | 1.3.3 |
| GitHub CLI | 2.102.0 |
| jq | 1.8.2 |
| ripgrep (`rg`) | 15.2.0 |
| fzf (`fzf`) | 0.74.5 |
| bat (`bat`) | 0.26.1 |
| regctl (`regctl`) | 0.11.6 |
| fd (`fd`, `fdfind`) | 10.5.0 |
| ast-grep (`ast-grep`) | 0.45.3 |
| Mike Farah yq (`yq`) | 4.54.1 |
| uv (`uv`, `uvx`) | 0.13.0 |
| Herdr (`herdr`) | 0.9.3 |
| Fish (`fish`) | 4.9.3 |
| Starship (`starship`) | 1.26.0 |
| Nushell (`nu`) | 0.116.1 |
| tmux (`tmux`) | 3.7c (official static builds) |
| typos (`typos`) | 1.51.1 |
| lychee (`lychee`) | 0.24.2 |
| Gitleaks (`gitleaks`) | 8.30.1 |
| Trivy (`trivy`) | 0.75.0 |
| Syft (`syft`) | 1.54.1 |
| bottom (`btm`) | 0.14.9 |
| btop (`btop`) | 1.4.7 |
| Helix (`hx`) | 25.07.1 |
| Hugo (`hugo`, standard) | 0.167.0 |
<!-- END GENERATED TOOL INVENTORY -->

Nushell and tmux are opt-in commands. tmux pins the published stable
[official static build](https://github.com/tmux/tmux-builds/releases/tag/v3.7c),
3.7c, while the upstream source release is 3.8. Its provider resolves that static
build feed; release suffixes compare numerically (`3.7a` = `3.7.1`, `3.7c` =
`3.7.3`, `3.8` = `3.8.0`) while lock versions and asset URLs retain the upstream tag.
No plugins or user configuration are installed for either alternative.

## Propose upgrades with GitHub Actions

The **Propose toolbox upgrades** workflow (`.github/workflows/toolbox-upgrade.yml`)
runs daily at **06:50 UTC** from the repository default branch. Scheduled runs
resolve all desired stable tools within `tools.txt` constraints and open a
reviewable PR only when the lock or its generated inventory changes. GitHub
may delay scheduled runs; the cron time is not a delivery guarantee. The
schedule starts after a human merges this workflow onto the default branch.

For a manual test, open **Actions → Propose toolbox upgrades → Run workflow**,
select the default branch, keep `dry_run=true`, and optionally set `tool=gh`
for a bounded first run. An empty tool input resolves every desired tool.
The workflow refuses non-default-branch runs. A new manual workflow must
exist on the default branch before it can be dispatched.

Dry runs use a read-only token, run the existing `tools upgrade` resolver, and
show the proposed lock and inventory diff in the run summary. Resolution downloads official
prebuilt archives for both AMD64 and ARM64 to verify their checksums; it does
not execute those tools or build images, CLI releases, or documentation. An
upstream error fails the run without creating a PR. No changes produce a
successful no-op. Stale inventory alone is a deliberate repair: it counts as
a change even when the lock versions are already current.

For manual runs, set `dry_run=false` when you want a reviewable lock and
inventory PR; scheduled runs already propose actual changes. A separate job
requests `contents: write` and `pull-requests: write`, transfers only the
resolved lock and generated inventory from that same run attempt, and creates its own
`automation/toolbox-upgrade-<run-id>-<attempt>` branch. It never force-pushes,
reuses another branch, merges a PR, publishes tools, or activates a consumer
image. Rerunning a changed attempt creates a separate topic; review
existing topics before rerunning. An unsuccessful PR creation may leave its
already-pushed topic branch for inspection.

Dry runs work with default read-only workflow permissions and do not require
permission to create pull requests. Before using write mode, an administrator
must explicitly allow the requested token write permissions under any
repository/organization policy and enable **Settings → Actions → General →
Workflow permissions → Allow GitHub Actions to create and approve pull
requests**. The checkbox permits PR creation; this workflow never approves
reviews. If policy blocks the push or PR, the job fails with these prerequisites.
The workflow does not change settings, install a PAT, or bypass policy.

A PR opened with `GITHUB_TOKEN` does not automatically start ordinary
`pull_request` workflows. Run the existing **CI** workflow manually on the
new topic branch and complete human review before merging. A successful
resolver or CI run verifies source and lock behavior, not native image smoke
coverage. Building, publishing, or activating the new lock still requires
explicit human approval of the operation and revision. One exact-head PR approval by `wstein` authorizes the guarded [source CI, merge and frozen-lock publication flow](publication.md) for an eligible bundle updater. It publishes the exact reviewed lock without fresh upstream resolution or a later environment approval. Publication does not activate a consumer image.

## Keep the source inventory consistent

The pinned-version table above is managed between the
`BEGIN GENERATED TOOL INVENTORY` and `END GENERATED TOOL INVENTORY` comments.
Its versions come from `tools.lock.json` for both supported architectures;
existing catalog tag prefixes determine the display version, and existing
labels and row order remain presentation metadata. Missing catalog IDs or
architecture version differences fail generation rather than omitting a tool.

After changing a lock locally, run:

```sh
python3 scripts/tool-inventory.py          # update only the managed table
python3 scripts/tool-inventory.py --check  # fail on drift without writing
make check                                # includes the same source check
```

The upgrade workflow generates this table after resolution. Before opening a
PR, the write job regenerates it from the checked-out documentation and resolved
lock, then compares the transferred inventory byte for byte. It accepts only
the lock and generated table changes; unrelated prose edits are rejected.
Prose outside the markers, historical verification records, badges and consumer
pins are not generated. These commands edit or check Markdown source; they do
not invoke Hugo or render a documentation site. The existing CI workflow calls
`make check`, so a lock-only change with a stale table fails source verification.

## Helix terminal editor

The toolbox includes official stable [Helix 25.07.1](https://github.com/helix-editor/helix/releases/tag/25.07.1)
Linux AMD64 and ARM64 prebuilts. `hx` launches the untouched binary with
`HELIX_RUNTIME` pointing to the same mounted bundle's `lib/helix/runtime`.
The whole runtime, compiled tree-sitter grammars, queries, themes and upstream
notices are retained. User configuration belongs in writable
`$XDG_CONFIG_HOME/helix` (normally `$HOME/.config/helix`); no user configuration
is created by bundle initialization. Language servers, formatters and clipboard
helpers are supplied by the consumer or installed in writable home locations.
No grammar download, compilation or plugin installation is needed for the shipped
runtime. Helix's official GNU/Linux prebuilts require compatible glibc; the
stock Debian consumer remains the integration target.

`tools init` copies and validates volume content; it cannot change the invoking
shell's environment. Anchorage owns its consumer shell seeds, which default
`EDITOR` to `hx` only when `hx` is available and `EDITOR` is unset, preserving
user overrides and `VISUAL`. Existing custom seeds remain user-owned. Other
consumers can opt in with `export EDITOR="${EDITOR-hx}"` after placing toolbox
`bin` on PATH. The existing older pinned consumer image does not gain Helix
until a separately approved build, publication and activation.

Helix uses calendar release tags such as `25.07.1`. Its official-repository-only
`helix-calendar` normalization compares this as `25.7.1`; a two-part release
such as `25.07` compares as `25.7.0`. Desired requirements use canonical numeric
versions (`helix==25.7.1`); the lock and download asset retain the original tag
spelling. Generic SemVer parsing remains strict and prereleases are excluded.
The `helix-runtime` extraction recipe accepts only a single stable Helix Linux
release root, regular files and directories. Production extraction binds that
root to the verified release pin. It does not permit archive links.

## Upgrade the lock

Run upgrades from the repository root:

```sh
make run ARGS="tools upgrade"       # resolve all tools
make run ARGS="tools upgrade gh"    # resolve only GitHub CLI
make run ARGS="tools upgrade --directory bundles/toolbox" # explicit bundle directory
```

`tools.txt` contains one desired requirement per line: `gh>=2.102.0,<3.0.0`
or `gh==2.102.0`. Commas mean AND; ranges require inclusive lower and exclusive
upper bounds. Versions must be complete stable `major.minor.patch` values;
prereleases and build metadata are excluded. Comments start with `#`. The initial
0.x requirements stop at the next minor version; other initial requirements stop
at the next major version. Editing this file changes the desired range, not the
installed tools.

`tools.packages.json` is the trusted declarative package catalog. It names upstream
providers, repositories, tag prefixes, platform aliases, asset templates, supported
raw/tar/ZIP extraction, executable members, regular-file aliases, notices, and the
extraction recipes. Preserved tar recipes can omit exact relative directories
with `exclude_directories`; names are normalized paths, without glob matching.
Every omitted entry still undergoes path, type, duplicate, payload and stream
integrity validation. Recipes cannot execute shell commands.
Both upgrade and build validate this catalog. Review catalog edits as build inputs. A catalog change requires
an upgrade of all tools so preserved pins cannot acquire a different recipe silently.

The native builder supports `tar.xz` with the checksum-pinned pure Go
`github.com/ulikunitz/xz` decoder; no external `xz` command is needed. XZ inputs
are limited to 64 MiB compressed, 256 MiB decoded, 4096 tar members, and 128 MiB
per member. The decoder validates stream integrity through EOF. XZ headers can
request dictionary memory beyond these limits, so the production installer
verifies the exact trusted upstream checksum before decoding; these limits do
not make the decoder suitable for arbitrary untrusted archives. Decoder notices
are retained in `bundles/toolbox/licenses/xz`.

Upgrades fetch fresh release metadata and select the numerically newest stable
version within each requirement. GitHub release history is paginated. Optional `GITHUB_TOKEN` authentication is
sent only to `api.github.com`; artifact and other provider requests never receive
it. HTTP errors, including API rate limits, abort with the request URL and status.
Claude and
Antigravity combine official current manifests with historical GitHub releases.
The chosen version must supply both architectures;
missing assets, metadata errors, or checksum failures abort without selecting an
older version. All selected downloads are hashed and verified before the lock is
replaced atomically. A failed resolution leaves the previous lock bytes unchanged;
a targeted upgrade preserves every other tool pin. An all-tool upgrade can create
a missing lock or reconcile catalog additions and removals.

Lock verification provenance distinguishes publisher SHA256/SHA512 checksums from `tls-download-sha256`: the latter pins bytes downloaded
from official HTTPS when a release has no publisher checksum. It is a reproducibility
pin, not an independent publisher authenticity proof. A present invalid publisher
checksum always fails. Redirects require HTTPS and an official host; GitHub's release
asset CDN is allowed. The migrated existing hashes and artifact URLs remain pinned.
Canonical SHA256 digests bind the lock to desired requirements and catalog content;
formatting and comments in `tools.txt` do not change its identity. Builds reject stale
identity, incomplete platforms, unsupported recipes, and unsafe archive members.

`tools upgrade` writes only `tools.lock.json`. Review and commit the diff, then run
`make tools-build tools-smoke tools-volume` to build and initialize the new bundle.
Existing images, volumes, running containers, and desired ranges are unaffected by
resolution. `tools volume` remains the existing initialization command; no update
alias or other tool-management commands are added.

## Optional toolbox utilities

`fzf` filters lists; for example, `printf 'alpha\nbeta\n' | fzf --filter beta`
works without an interactive terminal. Shell key bindings and completion are
opt-in through `fzf --bash`, `fzf --zsh` or `fzf --fish`; source the chosen output
in your own shell configuration only if desired. BundleDock never edits dotfiles.

`bat` includes upstream embedded syntax and theme assets. Use
`bat --paging=never --style=plain file` for predictable noninteractive output.
Its pager is optional and consumer supplied; no editor or pager default is set.
Upstream completion scripts, manual, README and changelog are retained under
`licenses/bat/` for optional use, alongside the decoded upstream syntax/theme
notices and their inventory. Custom configuration and caches belong in writable
HOME, outside the read-only toolbox.

`regctl` is the official registry client only; regbot and regsync are excluded.
Credentials and state belong in writable HOME (`~/.regctl/`), with optional
`REGCTL_CONFIG` pointing to a writable config file. Offline smoke uses only
version/help and reference parsing; it never logs in or contacts a registry.

The six Linux AMD64/ARM64 release artifacts were downloaded and SHA256 checked
against the official release API digests on 2026-10-12. Both fzf archives contain
one regular binary; both bat archives contain only regular files and one
completion directory. regctl assets are raw executables. These recipes retain
the existing extractor's rejection of archive links. Source tests use synthetic
artifacts and smoke stubs. Native execution, OCI builds/smoke, vulnerability scans
and consumer activation remain unverified pending separate approval.

## Optional browser editor

The [code-server bundle](code-server.md) provides a separate optional OCI
carrier with its own lock and read-only volume. It does not change the toolbox
or start a service automatically.
