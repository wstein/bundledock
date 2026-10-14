---
title: Toolbox bundle
description: Inspect pinned tool versions and resolve verified upgrades for both architectures.
weight: 20
---

# Toolbox bundle

[Back to BundleDock](../README.md) · [All bundles](bundles.md)

<!-- BEGIN PUBLISHED BUNDLE -->
<!-- publication-record: {"attempt":"1","bundle":"toolbox","commit":"5e86f1741dcbe2c5f3515290645d2ffdb85e590f","created":"2026-10-14T09:33:19Z","digest":"sha256:83ffe1d41549402bb765d4a98698a15d4196efe437af028b3e8d91a2f5c1e0c6","image":"quay.io/bundledock/toolbox","lock_sha256":"a4d61e20a0c75a3fd3ed4d63b28315aba5c4d62f2f2a8998a78267acd5bb891e","reference":"quay.io/bundledock/toolbox@sha256:83ffe1d41549402bb765d4a98698a15d4196efe437af028b3e8d91a2f5c1e0c6","run_id":"38543029970","source":"https://github.com/wstein/bundledock","tag":"2026.10.14-b80.1"} -->
## Published bundle

Version: `2026.10.14-b80.1`

Index digest: `sha256:83ffe1d41549402bb765d4a98698a15d4196efe437af028b3e8d91a2f5c1e0c6`

Source: [`5e86f1741dcbe2c5f3515290645d2ffdb85e590f`](https://github.com/wstein/bundledock/commit/5e86f1741dcbe2c5f3515290645d2ffdb85e590f)

Publication: [run 38543029970, attempt 1](https://github.com/wstein/bundledock/actions/runs/38543029970/attempts/1)

Pull this published carrier by its immutable index digest:

```sh
docker pull quay.io/bundledock/toolbox@sha256:83ffe1d41549402bb765d4a98698a15d4196efe437af028b3e8d91a2f5c1e0c6
```

The source inventory below describes the current lock; this publication retains its exact source revision.
<!-- END PUBLISHED BUNDLE -->

## Spec Kit launcher (source)

The source supplies an owned `specify` launcher for `specify-cli==1.1.3`,
separate from downloaded catalog tools and absent from older published images.
It uses plain pip in a dedicated writable virtual environment. Supply `python3`
3.11+ with `venv` and pip from the consumer's stock language image; the launcher
rejects virtual environments and recognized uv-managed interpreters, including
`UV_PYTHON_INSTALL_DIR`. Arbitrarily relocated interpreter provenance cannot be
inferred, so provide the image's own Python.

```sh
specify --help
specify init my-project
```

First use needs network access to the official PyPI index and compatible
prebuilt wheels for the image's Python version, ABI and platform. Source builds,
Python downloads, global installs and project environments are never used.
The exact top-level wheel and SHA256 are recorded in bundled requirements;
**transitive dependency versions remain unfrozen** and are resolved on first
installation. A missing compatible wheel fails clearly. A completed environment
can run offline; pip's download cache alone is not a completed environment.

Use writable `HOME` outside the toolbox. Environments live under
`${XDG_DATA_HOME:-$HOME/.local/share}/bundledock/tools/specify`, keyed by package
metadata, helper source, interpreter and ABI. Downloads use
`${XDG_CACHE_HOME:-$HOME/.cache}/bundledock/pip/specify`. XDG overrides must be
absolute paths outside the toolbox. Concurrent first uses serialize, interrupted
installs retry safely, and changed metadata or Python creates a separate
environment. Earlier environments remain available. The launcher ignores pip
configuration, clears installer and Python environment overrides, uses only
wheels, and preserves the working directory, arguments and exit status. Explicit
commands such as `specify init` can create projects and agent configuration.

[Official PyPI metadata](https://pypi.org/pypi/specify-cli/1.1.3/json) records
Python `>=3.11` and the universal wheel `specify_cli-1.1.3-py3-none-any.whl`, SHA256
`9cc0712dbdffb8a035f1d3f4b80f678eeeddc14b3cf1437d26056df2c60ee127`.
The immutable bundle contains the owned launcher, shared Python helper and
requirements metadata, with no installed Python runtime or packages. Installed
package notices stay in the writable environment. `launchers.specify` records
the pin separately from downloaded `tools`; `SHA256SUMS` covers all launcher
source and requirements. The catalog updater does not change this pin.

Source tests execute stdlib Python and synthetic venv/pip fixtures without
network access or package installation. Native consumer smoke and real first-use
resolution remain separate checks requiring an approved image build.

## tldr launcher (source)

The source supplies `tldr` through the [official Python client](https://github.com/tldr-pages/tldr-python-client),
version 3.4.4. It requires `python3` 3.10+ with `venv` and pip from the
consumer's stock language image. The launcher uses the same isolated plain-pip
helper as Spec Kit, with a separate tool directory and download cache:

```sh
tldr --help
tldr tar
tldr --update
```

Explicit first use installs exactly three reviewed universal wheels: `tldr`
3.4.4, `shtab` 1.12.1 and `termcolor` 3.3.0. Each wheel's official URL and
SHA256 are pinned in `share/tldr/tldr.requirements.txt`; pip installs with
`--no-deps` and `--only-binary=:all:`. The base distribution already provides
the CLI; its optional `cli` extra contains development tools and is excluded.
The immutable carrier retains the unchanged MIT/MPL-2.0 notices and source
record through its license archive. Installed notices remain in the environment.

Environments live under `${XDG_DATA_HOME:-$HOME/.local/share}/bundledock/tools/tldr`
and pip downloads under `${XDG_CACHE_HOME:-$HOME/.cache}/bundledock/pip/tldr`.
Keep these paths and `HOME` writable outside the toolbox. First installation
needs network access to the pinned official PyPI files; a completed environment
can launch offline. tldr separately fetches example pages and caches them in
`${XDG_CACHE_HOME:-$HOME/.cache}/tldr`. Use `TLDR_NETWORK_ENABLED=0` with cached
pages for offline lookups; a missing page is unavailable offline. Package pins
do not freeze the community page content. Upstream language, color and page
source configuration remains available.

[PyPI release metadata](https://pypi.org/pypi/tldr/3.4.4/json) and all three
wheel manifests were inspected and hashes verified without installing or
executing third-party code. Source fixtures cover first use, reuse, Python
compatibility, argument/status forwarding and altered requirements. Real package
installation, first page download and native consumer smoke remain unverified
until a separately approved image build and runtime check. Older published
images do not include this launcher.

## Package-installed tool defaults

Python command-line additions use a dedicated per-tool virtual environment and
plain pip, with reviewed requirements shipped beside the owned launcher.
Package installation occurs on explicit first use, outside the read-only bundle.
Stock language images provide Python, `venv` and pip; BundleDock does not install
an interpreter. See the source-only [package installation policy](package-tools.md)
for dependency pins, npm defaults and the existing-tool inventory.

## Retained notices

New bundles store all staged `licenses/<package>/...` files, including full
license texts, source records and vendored dependency notices, in one
`licenses.tar.gz` at the bundle root. Original member paths and bytes are
preserved. `licenses.index.json` remains plain text for offline discovery: it
maps every member to its package, size and SHA256 and records the archive
SHA256. `SHA256SUMS` covers both files. Payload-internal notices stay with their
unchanged runtime assets. This changes the installed loose notice paths; the
source notice directories in this repository remain unchanged. The published
image above predates this source change.

The existing carrier's `init-volume` helper can list the index or print one
complete notice without external gzip/tar tools, network access or volume writes:

```sh
docker run --rm --network none "$IMAGE" --list-licenses
docker run --rm --network none "$IMAGE" --license licenses/gh/LICENSE
```

Use a carrier built from this source change as `IMAGE`; earlier published
carriers retain their existing notice layout and initializer flags. The helper
lives in the carrier, not the mounted tool volume. A mounted consumer can read
`licenses.index.json` directly, or read `licenses.tar.gz` with compatible archive
tools. For a mounted root, the carrier helper accepts `--source` with that
mounted path. Requests require the complete member path from the index,
avoiding ambiguous basenames across packages. The helper checks the index/archive
checksums, member hashes and sizes, rejects unsafe or duplicate paths, and
bounds expansion before printing bytes. Retaining these notices does not
establish a complete transitive license audit.

## Source inventory

The default bundle lives in `bundles/toolbox/`: `packages.txt`,
`packages.catalog.json`, `packages.lock.json`, its Dockerfile, smoke scripts and licenses.
Shared Go code remains at the repository root. The source toolbox contains 41
downloaded tools plus the two first-use Python launchers listed below. Codex, Antigravity and Claude are supplied by their separate
[Codex](codex.md), [Antigravity](agy.md) and [Claude Code](claude.md) bundles.
The published toolbox recorded above still includes those agents; this source
removal has not been built or published.

`packages.lock.json` pins vendor artifact URLs and checksums for Linux `amd64`
and `arm64`; builds verify every download. Launcher versions come from their
separate wheel inputs. Homepage and Source links are generated from reviewed
upstream metadata and persist when the native lock is refreshed.

TruffleHog is included alongside Gitleaks, using official stable Linux AMD64 and
ARM64 binaries. Its full AGPL-3.0 license and upstream README are retained under
`licenses/trufflehog`. For a local scan without update checks or credential
verification requests:

```sh
trufflehog filesystem --no-update --no-verification --fail --json /path/to/project
```

Without `--no-verification`, TruffleHog can contact providers to verify detected
credentials. Gitleaks configurations and rules do not transfer automatically.
The consumer smoke uses only a synthetic custom detector and clean control,
isolated configuration and HOME, with updates/verification disabled and consumer
networking disabled. Native smoke acceptance remains part of an approved build.

<!-- BEGIN GENERATED TOOL INVENTORY -->
| Tool | Pinned version | Homepage | Source |
| --- | --- | --- | --- |
| age (`age`, `age-keygen`, `age-inspect`, plugins) | 1.3.2 | [Homepage](https://age-encryption.org) | [Source](https://github.com/FiloSottile/age) |
| ast-grep (`ast-grep`) | 0.50.0 | [Homepage](https://ast-grep.github.io/) | [Source](https://github.com/ast-grep/ast-grep) |
| bat (`bat`) | 0.26.1 | [Homepage](https://github.com/sharkdp/bat) | [Source](https://github.com/sharkdp/bat) |
| bottom (`btm`) | 0.14.9 | [Homepage](https://bottom.pages.dev) | [Source](https://github.com/ClementTsang/bottom) |
| btop (`btop`) | 1.4.7 | [Homepage](https://github.com/aristocratos/btop) | [Source](https://github.com/aristocratos/btop) |
| delta (`delta`) | 0.20.1 | [Homepage](https://dandavison.github.io/delta/) | [Source](https://github.com/dandavison/delta) |
| fd (`fd`, `fdfind`) | 10.5.0 | [Homepage](https://github.com/sharkdp/fd) | [Source](https://github.com/sharkdp/fd) |
| Fish (`fish`) | 4.9.3 | [Homepage](https://fishshell.com) | [Source](https://github.com/fish-shell/fish-shell) |
| fzf (`fzf`) | 0.74.5 | [Homepage](https://junegunn.github.io/fzf/) | [Source](https://github.com/junegunn/fzf) |
| Git LFS (`git-lfs`) | 3.8.0 | [Homepage](https://git-lfs.com) | [Source](https://github.com/git-lfs/git-lfs) |
| git-trim (`git-trim`, reviewed source exception) | 0.4.3 | [Homepage](https://github.com/foriequal0/git-trim) | [Source](https://github.com/foriequal0/git-trim) |
| GitHub CLI | 2.102.0 | [Homepage](https://cli.github.com) | [Source](https://github.com/cli/cli) |
| Gitleaks (`gitleaks`) | 8.30.1 | [Homepage](https://gitleaks.io) | [Source](https://github.com/gitleaks/gitleaks) |
| Helix (`hx`) | 25.07.1 | [Homepage](https://helix-editor.com) | [Source](https://github.com/helix-editor/helix) |
| Herdr (`herdr`) | 0.9.4 | [Homepage](https://herdr.dev) | [Source](https://github.com/herdrdev/herdr) |
| Hugo (`hugo`, standard) | 0.168.0 | [Homepage](https://gohugo.io) | [Source](https://github.com/gohugoio/hugo) |
| hyperfine (`hyperfine`) | 2.0.0 | [Homepage](https://github.com/sharkdp/hyperfine) | [Source](https://github.com/sharkdp/hyperfine) |
| jq | 1.8.2 | [Homepage](https://jqlang.org) | [Source](https://github.com/jqlang/jq) |
| Jujutsu (`jj`) | 0.46.0 | [Homepage](https://jj-vcs.dev) | [Source](https://github.com/jj-vcs/jj) |
| just (`just`) | 1.58.0 | [Homepage](https://just.systems) | [Source](https://github.com/casey/just) |
| lazygit (`lazygit`) | 0.66.1 | [Homepage](https://github.com/jesseduffield/lazygit) | [Source](https://github.com/jesseduffield/lazygit) |
| Leaf (`leaf`) | 1.29.0 | [Homepage](https://leaf.rivolink.mg) | [Source](https://github.com/RivoLink/leaf) |
| lychee (`lychee`) | 0.24.2 | [Homepage](https://lychee.cli.rs) | [Source](https://github.com/lycheeverse/lychee) |
| Mike Farah yq (`yq`) | 4.54.1 | [Homepage](https://mikefarah.gitbook.io/yq/) | [Source](https://github.com/mikefarah/yq) |
| mise (`mise`) | 2026.10.9 | [Homepage](https://mise.jdx.dev) | [Source](https://github.com/jdx/mise) |
| navi (`navi`) | 2.24.0 | [Homepage](https://github.com/denisidoro/navi) | [Source](https://github.com/denisidoro/navi) |
| Nushell (`nu`) | 0.116.1 | [Homepage](https://www.nushell.sh/) | [Source](https://github.com/nushell/nushell) |
| regctl (`regctl`) | 0.11.6 | [Homepage](https://regclient.org) | [Source](https://github.com/regclient/regclient) |
| ripgrep (`rg`) | 15.2.0 | [Homepage](https://github.com/BurntSushi/ripgrep) | [Source](https://github.com/BurntSushi/ripgrep) |
| Ruff (`ruff`) | 0.17.0 | [Homepage](https://docs.astral.sh/ruff) | [Source](https://github.com/astral-sh/ruff) |
| ShellCheck (`shellcheck`) | 0.11.0 | [Homepage](https://www.shellcheck.net) | [Source](https://github.com/koalaman/shellcheck) |
| shfmt (`shfmt`) | 3.14.1 | [Homepage](https://pkg.go.dev/mvdan.cc/sh/v3) | [Source](https://github.com/mvdan/sh) |
| Spec Kit (`specify`, first-use pip launcher) | 1.1.3 | [Homepage](https://github.github.io/spec-kit/) | [Source](https://github.com/github/spec-kit) |
| Starship (`starship`) | 1.26.0 | [Homepage](https://starship.rs) | [Source](https://github.com/starship/starship) |
| Syft (`syft`) | 1.54.1 | [Homepage](https://github.com/anchore/syft) | [Source](https://github.com/anchore/syft) |
| tldr (`tldr`, first-use pip launcher) | 3.4.4 | [Homepage](https://pypi.org/project/tldr/) | [Source](https://github.com/tldr-pages/tldr-python-client) |
| tmux (`tmux`) | 3.7c (official static builds) | [Homepage](https://github.com/tmux/tmux-builds) | [Source](https://github.com/tmux/tmux-builds) |
| Trivy (`trivy`) | 0.75.0 | [Homepage](https://trivy.dev) | [Source](https://github.com/aquasecurity/trivy) |
| TruffleHog (`trufflehog`) | 3.99.2 | [Homepage](https://github.com/trufflesecurity/trufflehog) | [Source](https://github.com/trufflesecurity/trufflehog) |
| typos (`typos`) | 1.51.1 | [Homepage](https://github.com/crate-ci/typos) | [Source](https://github.com/crate-ci/typos) |
| Typst (`typst`) | 0.15.1 | [Homepage](https://typst.app) | [Source](https://github.com/typst/typst) |
| uv (`uv`, `uvx`) | 0.13.0 | [Homepage](https://docs.astral.sh/uv) | [Source](https://github.com/astral-sh/uv) |
| watchexec (`watchexec`) | 2.8.0 | [Homepage](https://watchexec.github.io) | [Source](https://github.com/watchexec/watchexec) |
<!-- END GENERATED TOOL INVENTORY -->

Nushell and tmux are opt-in commands. tmux pins the published stable
[official static build](https://github.com/tmux/tmux-builds/releases/tag/v3.7c),
3.7c, while the upstream source release is 3.8. Its provider resolves that static
build feed; release suffixes compare numerically (`3.7a` = `3.7.1`, `3.7c` =
`3.7.3`, `3.8` = `3.8.0`) while lock versions and asset URLs retain the upstream tag.
No plugins or user configuration are installed for either alternative.

## Propose upgrades with GitHub Actions

The **Propose upgrades** workflow with `bundle=toolbox` (`.github/workflows/propose-upgrades.yml`)
runs daily at **06:50 UTC** from the repository default branch. Scheduled runs
resolve all desired stable tools within `packages.txt` constraints and open a
reviewable PR only when the lock or its generated inventory changes. GitHub
may delay scheduled runs; the cron time is not a delivery guarantee. The
schedule starts after a human merges this workflow onto the default branch.

For a manual test, open **Actions → Propose upgrades → Run workflow**,
select the default branch and `bundle=toolbox`, keep `dry_run=true`, and optionally set `tool=gh`
for a bounded first run. An empty tool input resolves every desired tool.
The workflow refuses non-default-branch runs. A new manual workflow must
exist on the default branch before it can be dispatched.

Dry runs use a read-only token, run the existing `tools upgrade` resolver, and
show the proposed lock and inventory diff in the run summary. Resolution downloads official
prebuilt archives for both AMD64 and ARM64 to verify their checksums. The
git-trim exception only verifies its pinned source archive and Cargo.lock; a
newer stable tag stops with a source-review-required error. Resolution never
executes upstream code or build images, CLI releases, or documentation. An
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
`pull_request` workflows. Run the required source checks and merge the eligible
updater as **wstein** using the protected human merge path. That actual owner
merge authorizes publication of the exact frozen lock after successful merged
main CI. See the [owner-merge publication flow](publication.md). Unrelated source
changes cannot enter this path. Source checks do not establish native image
smoke coverage. Initial publication retains explicit full-SHA manual approval,
and publication does not activate a consumer image.

## Keep the source inventory consistent

The pinned-version table above is managed between the
`BEGIN GENERATED TOOL INVENTORY` and `END GENERATED TOOL INVENTORY` comments.
Its versions come from `packages.lock.json` for both supported architectures;
existing catalog tag prefixes determine the display version, and existing
labels remain presentation metadata. Rows sort alphabetically by the displayed
label, ignoring case. Missing catalog IDs or
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
versions (`helix>=25.7.1`); the lock and download asset retain the original tag
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

`packages.txt` contains one desired requirement per line: `gh>=2.102.0`
or `helix>=25.7.1` for a minimum, `node>=24.0.0,<25.0.0` for a bounded range, or
`gh==2.102.0` for an exact pin. Commas mean AND; lower bounds are inclusive
and upper bounds are exclusive. Versions must be complete stable
`major.minor.patch` values; prereleases and build metadata are excluded.
Comments start with `#`. Generic CLI requirements, including Helix, use minimum
bounds so future stable releases remain eligible. The minimum records the
existing accepted baseline; it does not upgrade an installed tool. Node retains its selected LTS
major, Bun its runtime major, and code-server its standalone carrier major.
These major bounds need separate compatibility review before changing. Helix
retains its original calendar tag spelling in the lock and requires a matching
complete runtime and compiled grammars from the same official stable archive.
Provider tag normalization, archive recipes and both-architecture checks still apply to every
candidate; a minimum does not permit unsupported release formats.
Editing this file changes the desired range, not the installed tools.

`packages.catalog.json` is the trusted declarative package catalog. It names upstream
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
The standalone Claude and
Antigravity bundles combine official current manifests with historical GitHub releases.
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
formatting and comments in `packages.txt` do not change its identity. Builds reject stale
identity, incomplete platforms, unsupported recipes, and unsafe archive members.

`tools upgrade` writes only `packages.lock.json`. Review and commit the diff, then run
`make bundle-build bundle-smoke bundle-volume` to build and initialize the new bundle.
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

## Git Large File Storage

The source toolbox includes [Git LFS 3.8.0](https://github.com/git-lfs/git-lfs/releases/tag/v3.8.0)
from official Linux AMD64 and ARM64 prebuilts. `git-lfs` supplies Git large file
storage commands. Repository setup is opt-in: BundleDock does not run
`git lfs install`, install hooks, change Git configuration or track file patterns.
The archive's `install.sh` is excluded. Consult the [upstream documentation](https://github.com/git-lfs/git-lfs/tree/v3.8.0/docs)
when choosing repository-specific setup.

Both binary archives were SHA256 checked against official release API digests
and the contents of `sha256sums.asc` on 2026-10-12; its PGP signature was not
verified. The archives omit a license file, so the exact source tag's full
`LICENSE.md`, including its Go-derived code notice, is retained locally. The
archive README, changelog and base manual are retained under `licenses/git-lfs/`.
No separate runtime tree is required by this recipe. Source tests cover stable
resolution, both installation recipes, and license retention. Consumer smoke
uses only `git-lfs version` in an isolated temporary directory. Native execution,
OCI build/smoke, repository hooks and network transfers remain unverified;
the pinned consumer image is unchanged.

## lazygit terminal interface

The source toolbox includes [lazygit 0.66.1](https://github.com/jesseduffield/lazygit/releases/tag/v0.66.1)
from official Linux AMD64 and ARM64 prebuilts. Run `lazygit` inside a Git repository
to open its terminal interface. User configuration belongs in writable HOME;
BundleDock does not open the interface or change shell/editor defaults.

Both archives were SHA256 checked against the official release API digests and
`checksums.txt` on 2026-10-12. The full upstream MIT license is retained. Source
tests cover stable resolution and both installation recipes. Consumer smoke
uses only `lazygit --version` in a private temporary directory with a cleared
environment; no interactive session is started. Native execution and OCI
build/smoke remain unverified until separately authorized. The pinned consumer
image is unchanged.

## Jujutsu version control

The source toolbox includes [Jujutsu (`jj`) 0.46.0](https://github.com/jj-vcs/jj/releases/tag/v0.46.0)
from the official Linux AMD64 and ARM64 musl prebuilts. `jj` provides a Git-backed
version control interface. BundleDock installs `bin/jj` and the full upstream
Apache-2.0 license; it does not initialize or migrate consumer repositories or
edit user configuration. Keep repository state and user configuration in writable
locations outside the toolbox mount. See the [upstream tutorial](https://docs.jj-vcs.dev/latest/tutorial/)
for an introduction.

Both archives were downloaded and SHA256 checked against official release API
digests on 2026-10-12. They contain `jj`, `LICENSE` and `README.md`, with no
separate runtime tree. Source tests cover stable release resolution, both
architecture recipes, checksum rejection and exact license retention. The
consumer smoke creates a private temporary Git-backed repository, runs `jj new`
and checks its description through `jj log`; inherited Git selectors and Jujutsu
configuration are cleared before tool execution. Source tests exercise this
smoke with synthetic Jujutsu commands and an isolated Git fixture. No upstream
Jujutsu binary was executed during source verification. Native AMD64/ARM64
execution and OCI build/smoke remain unverified until separately authorized;
the pinned consumer image does not acquire Jujutsu from this source change.

## Shell, recipe, Python and file utilities

The source catalog also includes ShellCheck, shfmt, just, Ruff, delta,
watchexec, hyperfine and age. ShellCheck checks shell scripts; shfmt formats
shell source; just runs explicitly selected project recipes; Ruff lints and
formats Python. Delta renders diffs, watchexec runs commands when files change,
hyperfine benchmarks commands, and age encrypts files. Age includes the official
`age-keygen`, `age-inspect` and bundled `age-plugin-*` companions. These additions
are source pins and do not change the previously published toolbox receipt.

No shell hooks, Git pager, project recipes, editor settings or user configuration
are enabled automatically. Use delta with `--paging=never` for noninteractive
output. Ruff caches, watchexec state, hyperfine exports and age keys belong in a
writable project directory or HOME, outside the read-only toolbox. Never store
private encryption keys in the bundle.

All sixteen Linux AMD64/ARM64 assets were SHA256 checked against official stable
release API digests and inspected without executing them on 2026-10-12. Shfmt
uses raw executables; other additions use official tar archives. Delta uses a
musl AMD64 asset and a GNU libc ARM64 asset because upstream supplies no ARM64
musl archive. ARM64 consumers therefore require compatible GNU libc and its
runtime libraries; Alpine/musl ARM64 support is not established. This limitation
must be rechecked against upstream assets before changing the recipe or claiming
broader consumer support. Full upstream license texts, Ruff's bundled notices,
delta's bat 0.26.1 syntax/theme notices and age's distributed license are retained.

Source tests exercise synthetic archives and reject incorrect smoke outputs.
The offline smoke script checks shell formatting, an explicit temporary just
recipe, isolated Ruff formatting, a fixed diff, one watchexec command, a hyperfine
JSON export and age encryption/decryption with a temporary key. Native AMD64 and
ARM64 execution, OCI builds/smoke and vulnerability scans remain unverified and
require separately approved builds before release or consumer activation.

## Optional browser editor

The [code-server bundle](code-server.md) provides a separate optional OCI
carrier with its own lock and read-only volume. It does not change the toolbox
or start a service automatically.


## git-trim source exception

The user authorized this one source-build exception because git-trim v0.4.3 has
no official Linux ARM64 prebuilt. Both architectures use unchanged upstream commit
`65288617a637029573a5c3517abcb95b050ea02b`. The source archive SHA256, Cargo.lock
SHA256, commit time, fixed recipe identity and official Rust 1.75.0 Bookworm
multiarchitecture image digest are locked. All other packages retain their
existing official stable prebuilt contracts. A newer git-trim release, retagged
commit, changed lock or builder recipe requires source review and fails closed.

The fixed native Docker stage verifies the source archive, fetches the exact Git
commit, compares the source tree and supplies fixed shallow Git metadata for
upstream vergen without patching build.rs. It vendors the 117 checksum-locked
registry dependencies into versioned directories, then compiles with frozen,
offline Cargo and Docker networking disabled. Native headers and static
OpenSSL/zlib libraries come from the immutable stock compiler image. No mutable
system-package or Rust-toolchain installation occurs. The installer allows only
the package-specific git-trim source profile and a payload from this trusted build
stage; the catalog cannot supply compiler commands.

The staged receipt records the source pins and measured compiled-file hashes.
It checks local stage integrity; it does not independently authenticate that a
binary was compiled from those sources. The trusted fixed Docker stage supplies
that relationship, and publication binds its BundleDock source revision and lock.
The manifest preserves source provenance, the receipt is retained under
`licenses/git-trim/BUILD-PROVENANCE.json`, and SHA256SUMS covers installed output.

Full vendored Rust and bundled native-library notices are retained, alongside
Rust runtime and static OpenSSL/zlib notices from the compiler image. Several
crates omit full notices: retained upstream source-commit notices cover enum-iterator
and Windows targets. paw macro/raw notices come from the same project's
checksum-verified paw 1.0.0 release; their historical GitHub commits are unavailable,
as recorded in each SOURCE file. The locked 2022 dependency graph is unchanged;
these checksum and license checks do not establish vulnerability freedom.

Start branch planning with `git trim --dry-run --no-update`. The smoke fixture
creates only disposable local Git repositories, disables remote updates, runs
only dry-run planning, and checks that local and remote refs remain unchanged.
It never cleans an existing user repository. The GNU binary requires a compatible
glibc consumer; native Linux AMD64 and ARM64 builds, linkage inspection, the
read-only consumer smoke and repeated-build comparisons remain deferred until
explicit approval of the exact source revision and intended operation. Source
checks alone establish neither runtime compatibility nor bit-for-bit reproducibility.

## Leaf Markdown preview

The source toolbox includes [Leaf 1.29.0](https://github.com/RivoLink/leaf/releases/tag/1.29.0)
from the official Linux AMD64 and ARM64 raw binaries, installed as `bin/leaf`
with the full MIT license from the same upstream tag. Run `leaf README.md` for
an interactive terminal preview or `leaf --inline plain README.md` for plain
rendered output. Keep HOME and XDG configuration/cache directories writable
outside the read-only toolbox mount. Upgrade through BundleDock's lock;
`leaf --update` writes its executable and is unsuitable for the mounted toolbox.

Both downloaded binary hashes matched the official release API and
`checksums.txt` on 2026-10-12. Read-only ELF inspection confirmed x86-64 and
AArch64; both binaries require glibc, libgcc_s and libm with their native loaders.
No upstream binary was executed. Source fixtures cover resolution and raw
installation on both architectures and the isolated noninteractive smoke's
`--version` and `--inline plain` rendering. Native runtime compatibility and
image smoke on both architectures remain unverified until an explicitly
approved build and smoke run. The smoke clears caller configuration overrides
and confines Leaf's HOME, XDG paths and Markdown fixture to a temporary directory.

## Optional mise version manager

The source toolbox includes [mise 2026.10.8](https://github.com/jdx/mise/releases/tag/v2026.10.8)
as `bin/mise`, using the official stable raw Linux MUSL executables for AMD64
and ARM64. The desired constraint is `mise>=2026.10.8`; the lock pins both
architectures and retains the full upstream MIT license. Upgrade the executable
through BundleDock's lock rather than using `mise self-update` on the read-only
mount.

Use stock language containers as the default source of language toolchains.
Mise is an optional version manager for projects that deliberately choose it.
Adding it to the toolbox does not activate it, bootstrap runtimes, modify shell
configuration, or run `mise use` or `mise install`. Any chosen runtime installation
and shell integration is a separate user action. Keep HOME, XDG configuration,
cache, data and state directories writable outside `/opt/bdock/toolbox`.

On 2026-10-13, the downloaded raw binary hashes matched both the official release
API digests and `SHASUMS256.txt`. Read-only ELF inspection confirmed x86-64 and
AArch64 and no ELF interpreter; no upstream executable was run. Source fixtures
cover resolution, the minimum version, both architecture installation recipes,
and an isolated, noninteractive `mise --version` smoke that clears inherited
configuration and runs from a disposable HOME. Native runtime compatibility
and read-only image smoke on both architectures remain deferred until explicit
approval of the exact source revision and intended build/smoke operation.

## Navi interactive cheatsheets

The source toolbox includes [navi 2.24.0](https://github.com/denisidoro/navi/releases/tag/v2.24.0)
from official Linux AMD64 musl and ARM64 GNU prebuilts. It uses the bundled
`fzf`. Cheatsheet commands can execute shell code: choose trusted sheets and
review commands before running them. BundleDock does not fetch cheatsheets,
execute examples, install shell hooks or activate Navi at shell startup.
Keep HOME and any Navi configuration/cache in writable consumer storage.

The release supplies no publisher checksum; the lock records SHA256 observed
from official HTTPS downloads. Both binaries are UPX packed, so read-only ELF
inspection cannot establish the original loader/library requirements. ARM64
consumer compatibility remains unverified until an approved image build and
native smoke run. The smoke probe checks only `navi --version`, without opening
the interface or fetching sheets. This source addition has not been published.

## Typst typesetting

The source toolbox includes [Typst 0.15.1](https://github.com/typst/typst/releases/tag/v0.15.1)
from official Linux AMD64 and ARM64 musl prebuilts, pinned with publisher SHA256
verification. The unmodified binary includes upstream embedded fonts. Its full
archive LICENSE and NOTICE and the version-bound `typst-assets` NOTICE are
retained, including the fonts' separate license terms and exceptions.

Keep HOME, `XDG_CACHE_HOME` and Typst package caches in writable consumer storage
outside the read-only toolbox. Typst can use system fonts and explicit user
font directories via `--font-path` or `TYPST_FONT_PATHS`; choose those directories
in writable consumer storage when adding fonts. BundleDock does not download
fonts or render documents automatically. User-requested documents may resolve
Typst packages over the network and cache them under the user's cache directory.
The smoke probe uses only `typst --version`; compilation, rendering and native
runtime compatibility remain unverified pending an explicitly approved image
build and native smoke run. This source addition has not been published.
