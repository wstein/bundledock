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

| Tool | Pinned version |
| --- | --- |
| Codex | 0.162.1 |
| Claude Code | 2.1.296 |
| Antigravity | 1.3.2 |
| GitHub CLI | 2.102.0 |
| jq | 1.8.2 |
| ripgrep (`rg`) | 15.2.0 |
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
| Hugo (`hugo`, standard) | 0.167.0 |

Nushell and tmux are opt-in commands. tmux pins the published stable
[official static build](https://github.com/tmux/tmux-builds/releases/tag/v3.7c),
3.7c, while the upstream source release is 3.8. Its provider resolves that static
build feed; release suffixes compare numerically (`3.7a` = `3.7.1`, `3.7c` =
`3.7.3`, `3.8` = `3.8.0`) while lock versions and asset URLs retain the upstream tag.
No plugins or user configuration are installed for either alternative.

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
