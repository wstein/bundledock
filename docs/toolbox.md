---
title: Toolbox bundle
description: Inspect pinned tool versions and resolve verified upgrades for both architectures.
weight: 20
---

# Toolbox bundle

[Back to BundleDock](../README.md) · [All bundles](bundles.md)

<!-- BEGIN PUBLISHED BUNDLE -->
<!-- publication-record: {"attempt":"1","bundle":"toolbox","commit":"c2e97e9839fee920cc7e94d6bbcc6994b296dec7","created":"2026-10-15T20:58:14Z","digest":"sha256:def3cd1df5b41efdca39f9e281935c454fd2f0d72880607105c1ac2b250e0195","image":"quay.io/bundledock/toolbox","lock_sha256":"8401afc1fcd2b81d96b3a0c6d57f6135244bed87f9f2f7b3086fb19d22a9834f","reference":"quay.io/bundledock/toolbox@sha256:def3cd1df5b41efdca39f9e281935c454fd2f0d72880607105c1ac2b250e0195","run_attempt":"1","run_id":"38805917472","source":"https://github.com/wstein/bundledock","tag":"2026.10.15-b3"} -->
## Published bundle

Version: `2026.10.15-b3`

Index digest: `sha256:def3cd1df5b41efdca39f9e281935c454fd2f0d72880607105c1ac2b250e0195`

Source: [`c2e97e9839fee920cc7e94d6bbcc6994b296dec7`](https://github.com/wstein/bundledock/commit/c2e97e9839fee920cc7e94d6bbcc6994b296dec7)

Publication: [run 38805917472, attempt 1](https://github.com/wstein/bundledock/actions/runs/38805917472/attempts/1)

Pull this published carrier by its immutable index digest:

```sh
docker pull quay.io/bundledock/toolbox@sha256:def3cd1df5b41efdca39f9e281935c454fd2f0d72880607105c1ac2b250e0195
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
| actionlint (`actionlint`) | 1.7.12 | [Homepage](https://github.com/rhysd/actionlint) | [Source](https://github.com/rhysd/actionlint) |
| age (`age`, `age-keygen`, `age-inspect`, plugins) | 1.3.2 | [Homepage](https://age-encryption.org) | [Source](https://github.com/FiloSottile/age) |
| ast-grep (`ast-grep`) | 0.51.0 | [Homepage](https://ast-grep.github.io/) | [Source](https://github.com/ast-grep/ast-grep) |
| bat (`bat`) | 0.26.1 | [Homepage](https://github.com/sharkdp/bat) | [Source](https://github.com/sharkdp/bat) |
| bottom (`btm`) | 0.14.9 | [Homepage](https://bottom.pages.dev) | [Source](https://github.com/ClementTsang/bottom) |
| btop (`btop`) | 1.4.7 | [Homepage](https://github.com/aristocratos/btop) | [Source](https://github.com/aristocratos/btop) |
| Cosign (`cosign`) | 3.1.3 | [Homepage](https://github.com/sigstore/cosign) | [Source](https://github.com/sigstore/cosign) |
| delta (`delta`) | 0.20.1 | [Homepage](https://dandavison.github.io/delta/) | [Source](https://github.com/dandavison/delta) |
| Difftastic (`difft`) | 0.71.0 | [Homepage](https://github.com/Wilfred/difftastic) | [Source](https://github.com/Wilfred/difftastic) |
| fd (`fd`, `fdfind`) | 10.5.0 | [Homepage](https://github.com/sharkdp/fd) | [Source](https://github.com/sharkdp/fd) |
| Fish (`fish`) | 4.9.3 | [Homepage](https://fishshell.com) | [Source](https://github.com/fish-shell/fish-shell) |
| fzf (`fzf`) | 0.74.5 | [Homepage](https://junegunn.github.io/fzf/) | [Source](https://github.com/junegunn/fzf) |
| Git LFS (`git-lfs`) | 3.8.0 | [Homepage](https://git-lfs.com) | [Source](https://github.com/git-lfs/git-lfs) |
| git-cliff (`git-cliff`) | 2.14.2 | [Homepage](https://github.com/orhun/git-cliff) | [Source](https://github.com/orhun/git-cliff) |
| GitHub CLI | 2.102.0 | [Homepage](https://cli.github.com) | [Source](https://github.com/cli/cli) |
| Gitleaks (`gitleaks`) | 8.30.1 | [Homepage](https://gitleaks.io) | [Source](https://github.com/gitleaks/gitleaks) |
| Hadolint (`hadolint`) | 2.15.1 | [Homepage](https://github.com/hadolint/hadolint) | [Source](https://github.com/hadolint/hadolint) |
| Helix (`hx`) | 25.07.1 | [Homepage](https://helix-editor.com) | [Source](https://github.com/helix-editor/helix) |
| Herdr (`herdr`) | 0.9.5 | [Homepage](https://herdr.dev) | [Source](https://github.com/herdrdev/herdr) |
| Hugo (`hugo`, standard) | 0.168.0 | [Homepage](https://gohugo.io) | [Source](https://github.com/gohugoio/hugo) |
| hyperfine (`hyperfine`) | 2.0.0 | [Homepage](https://github.com/sharkdp/hyperfine) | [Source](https://github.com/sharkdp/hyperfine) |
| jq | 1.8.2 | [Homepage](https://jqlang.org) | [Source](https://github.com/jqlang/jq) |
| jsonschema-cli (`jsonschema-cli`) | 0.58.5 | [Homepage](https://github.com/Stranger6667/jsonschema) | [Source](https://github.com/Stranger6667/jsonschema) |
| Jujutsu (`jj`) | 0.46.0 | [Homepage](https://jj-vcs.dev) | [Source](https://github.com/jj-vcs/jj) |
| just (`just`) | 1.59.0 | [Homepage](https://just.systems) | [Source](https://github.com/casey/just) |
| lazygit (`lazygit`) | 0.66.2 | [Homepage](https://github.com/jesseduffield/lazygit) | [Source](https://github.com/jesseduffield/lazygit) |
| Leaf (`leaf`) | 1.29.1 | [Homepage](https://leaf.rivolink.mg) | [Source](https://github.com/RivoLink/leaf) |
| lsd (`lsd`) | 1.2.0 | [Homepage](https://github.com/lsd-rs/lsd) | [Source](https://github.com/lsd-rs/lsd) |
| lychee (`lychee`) | 0.24.2 | [Homepage](https://lychee.cli.rs) | [Source](https://github.com/lycheeverse/lychee) |
| Mike Farah yq (`yq`) | 4.54.1 | [Homepage](https://mikefarah.gitbook.io/yq/) | [Source](https://github.com/mikefarah/yq) |
| mise (`mise`) | 2026.10.11 | [Homepage](https://mise.jdx.dev) | [Source](https://github.com/jdx/mise) |
| navi (`navi`) | 2.24.0 | [Homepage](https://github.com/denisidoro/navi) | [Source](https://github.com/denisidoro/navi) |
| Nushell (`nu`) | 0.116.1 | [Homepage](https://www.nushell.sh/) | [Source](https://github.com/nushell/nushell) |
| pinact (`pinact`) | 5.0.0 | [Homepage](https://github.com/suzuki-shunsuke/pinact) | [Source](https://github.com/suzuki-shunsuke/pinact) |
| regctl (`regctl`) | 0.11.6 | [Homepage](https://regclient.org) | [Source](https://github.com/regclient/regclient) |
| ripgrep (`rg`) | 15.2.0 | [Homepage](https://github.com/BurntSushi/ripgrep) | [Source](https://github.com/BurntSushi/ripgrep) |
| Ruff (`ruff`) | 0.17.1 | [Homepage](https://docs.astral.sh/ruff) | [Source](https://github.com/astral-sh/ruff) |
| ShellCheck (`shellcheck`) | 0.11.0 | [Homepage](https://www.shellcheck.net) | [Source](https://github.com/koalaman/shellcheck) |
| shfmt (`shfmt`) | 3.14.1 | [Homepage](https://pkg.go.dev/mvdan.cc/sh/v3) | [Source](https://github.com/mvdan/sh) |
| Spec Kit (`specify`, first-use pip launcher) | 1.1.3 | [Homepage](https://github.github.io/spec-kit/) | [Source](https://github.com/github/spec-kit) |
| Starship (`starship`) | 1.26.0 | [Homepage](https://starship.rs) | [Source](https://github.com/starship/starship) |
| Syft (`syft`) | 1.54.1 | [Homepage](https://github.com/anchore/syft) | [Source](https://github.com/anchore/syft) |
| tldr (`tldr`, first-use pip launcher) | 3.4.4 | [Homepage](https://pypi.org/project/tldr/) | [Source](https://github.com/tldr-pages/tldr-python-client) |
| tmux (`tmux`) | 3.7c (official static builds) | [Homepage](https://github.com/tmux/tmux-builds) | [Source](https://github.com/tmux/tmux-builds) |
| Trivy (`trivy`) | 0.76.0 | [Homepage](https://trivy.dev) | [Source](https://github.com/aquasecurity/trivy) |
| TruffleHog (`trufflehog`) | 3.99.2 | [Homepage](https://github.com/trufflesecurity/trufflehog) | [Source](https://github.com/trufflesecurity/trufflehog) |
| typos (`typos`) | 1.51.1 | [Homepage](https://github.com/crate-ci/typos) | [Source](https://github.com/crate-ci/typos) |
| Typst (`typst`) | 0.15.1 | [Homepage](https://typst.app) | [Source](https://github.com/typst/typst) |
| uv (`uv`, `uvx`) | 0.13.1 | [Homepage](https://docs.astral.sh/uv) | [Source](https://github.com/astral-sh/uv) |
| watchexec (`watchexec`) | 2.8.0 | [Homepage](https://watchexec.github.io) | [Source](https://github.com/watchexec/watchexec) |
| zizmor (`zizmor`) | 1.31.1 | [Homepage](https://github.com/zizmorcore/zizmor) | [Source](https://github.com/zizmorcore/zizmor) |
| zoxide (`zoxide`) | 0.10.0 | [Homepage](https://github.com/ajeetdsouza/zoxide) | [Source](https://github.com/ajeetdsouza/zoxide) |
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
prebuilt archives for both AMD64 and ARM64 to verify their checksums. Resolution never
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

`lsd` lists directory contents with optional colors, icons and tree views. Use
`lsd --ignore-config --color never --icon never --oneline` for predictable
noninteractive output without user configuration or icon font requirements.
No `ls` shell alias is installed automatically; user configuration belongs in
writable `$XDG_CONFIG_HOME/lsd` or `$HOME/.config/lsd`, outside the read-only
toolbox. Full upstream Apache-2.0 `LICENSE` and `README.md` are retained under
`licenses/lsd/` from each official Linux AMD64 and ARM64 musl release archive,
which were downloaded and SHA256 verified against the official release API
digests on 2026-10-16.

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

`actionlint` lints GitHub Actions workflow files statically: `actionlint .github/workflows/ci.yml`. It runs `shellcheck` and `pyflakes` only when they are on `PATH`. It needs no network access or user configuration.

`hadolint` lints Dockerfiles: `hadolint Dockerfile`. Keep a project `.hadolint.yaml` in the repository. It is an unmodified official GPL-3.0 release binary; the full upstream `LICENSE`, the upstream `ThirdPartyNotices.txt` and a corresponding-source record (tarball URL and SHA256) are retained under `licenses/hadolint/`. The static binary links GPL-3.0-only ShellCheck, GPL-3.0-or-later language-docker and GPL-2.0-only HsYAML. **Owner acceptance required before publication:** the combined GPL license mix (including GPL-2.0-only HsYAML) has not been decided and must be accepted by the repository owner before any publication.

`pinact` pins GitHub Actions and reusable workflows to commit SHAs. `pinact run` rewrites workflow files and, unless offline, queries the GitHub API (set `GITHUB_TOKEN` in the environment); `pinact run --check` reports without writing. The release archive's `third_party_licenses/` tree is not extracted (the catalog retains notices by file name only), so the dependency licenses remain available only from the official release archive.

`zizmor` statically audits GitHub Actions workflows for security issues: `zizmor --offline .github/workflows`. Online audits query the GitHub API with an optional `GH_TOKEN` or `GITHUB_TOKEN` from the environment. The official Linux archives are GNU/glibc builds; the archive carries only the executable, so the upstream MIT `LICENSE` is retained separately.

`cosign` signs and verifies container images and other artifacts. Verification contacts registries and Sigstore services and signing can open a browser; neither runs at image build or shell startup. Keys, tokens and the TUF cache belong in writable HOME (`~/.sigstore`), outside the read-only toolbox.

`git-cliff` generates changelogs from Git history and Conventional Commits: `git-cliff --config cliff.toml`. Keep the configuration in the repository. Release archives also carry completions, man pages and `git-cliff-completions`/`git-cliff-mangen` generators; only the `git-cliff` executable, both licenses and README are installed.

`zoxide` is a directory jumper. BundleDock installs the executable only: no shell hook, alias or `cd` replacement is added. To opt in, source `zoxide init <shell>` output from your own shell configuration. Its database lives in `$_ZO_DATA_DIR` or the XDG data directory under writable HOME, outside the read-only toolbox. The nested completion `README.md` is not extracted, to avoid a name clash with the top-level README.

`difft` (Difftastic) compares files by syntax structure: `difft old.go new.go`. Use `--color never` for predictable output. As an optional Git external diff, set `GIT_EXTERNAL_DIFF=difft` in your own environment; BundleDock sets no Git configuration. The ARM64 release is the official GNU/Linux build (no musl asset exists); the archive carries only the executable, so the upstream MIT `LICENSE` is retained separately.

`jsonschema-cli` validates JSON instances against JSON Schema: `jsonschema-cli schema.json -i instance.json`. It is selected from the repository's `cli-v` releases, which are separate from the library's `rust-v`, `python-v` and `ruby-v` releases (the repository's latest-release marker points at a non-CLI release). Remote `$ref` resolution may use the network when a schema asks for it.

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

## Shared shell layer

The toolbox carries one set of Git, Jujutsu, `lsd` listing, search, disk and
directory shortcuts (95 in all), four navigation names and a
starship prompt for bash, zsh, fish and Nushell. Every shell gets the same names, and each name
runs the same command with the same arguments: `gst` is `git status`, `gpsup`
pushes the current branch with an upstream, `jst` is `jj status`, in all four.
The table is `internal/shelllayer/shortcuts.tsv`; Git names follow
[jhillyerd/plugin-git](https://github.com/jhillyerd/plugin-git) (MIT, notice
retained under `licenses/shell-layer`). The set is curated: no `--no-verify`,
git-svn, git-flow or forge-specific entries and no irreversible clean or reset
shortcuts. Names use `[a-z0-9]` only, so `gc!`-style names are not offered.

The listing family is read-only and runs [lsd](https://github.com/lsd-rs/lsd),
which the toolbox ships (`lsd` in `packages.txt`; a source test requires every
shortcut command to be a shipped tool or a base command on an explicit,
justified allowlist): `ll` (`lsd -l`), `la` (`-A`),
`lla` (`-lA`), `lt` (`-lt`, long, sorted by modification time), `ltr` (`-ltr`,
reversed), `l1` (`-1`, one entry per line), `ltree` (`--tree`) and `llg` (`-l
--group-dirs first`), each with `--color auto` and any extra arguments
appended. `ltree` is the tree view because `lt` already means the time-sorted
long listing. Stock `ls` is left untouched and is never aliased: in Nushell it
is a built-in that returns a table, an alias would replace it for the whole
session, and scripts expect the real `ls`; the table parser rejects any
shortcut named like a Nushell built-in. The flags come from the lsd 1.2.0
documentation (`doc/lsd.md`) and were not verified against a running lsd here,
including the sort direction of `-t`. The family is opted out with the rest of
the shortcuts (`BDOCK_SHORTCUTS=0`; in Nushell, which cannot define aliases
conditionally, by leaving the toolbox out of `XDG_DATA_DIRS`, where the aliases
call the external `^lsd`).

Everyday shortcuts cover search, disk and memory reports, and directories:
`rgi` (`rg -i`), `rgl` (`rg -l`), `rgf` (`rg --files`), `fdh` (`fd --hidden`),
`dfh` (`df -h`), `duh` (`du -h -d 1`), `fr` (`free -h`), `md` (`mkdir -p`) and
`rd` (`rmdir`). `rg` and `fd` are toolbox tools, covered by the `packages.txt`
test. `df`, `du`, `free`, `mkdir` and `rmdir` come from the base image (`free` from procps, which slim images may omit, so `fr` needs it) and are on
that test's explicit allowlist with a reason each: `df`, `du` and `free` only
report, `mkdir -p` only creates and never overwrites, and `rmdir` removes only an
empty directory. None of the names is an executable on `PATH` or in the
toolbox's `bin`; a test checks that. `cp -i`, `mv -i` and every `rm` variant are
deliberately absent: the interactive prompts hang agent terminals that cannot
answer, and `rm` is irreversible. `du -d 1` and `mkdir -p` work in GNU coreutils
and busybox alike.

Navigation names `..` (`cd ..`), `...` (`cd ../..`), `....` (`cd ../../..`) and
`-` (`cd -`) are not table rows: they have their own short fixed list in the
generator with strict patterns and per-shell rendering, and `validName` and
`validWord` still reject such names and path words. The layer defines a name only
where the shell does not run it natively and accepts the definition; otherwise it
leaves the name alone and says so here. Measured in an interactive
pseudo-terminal, and re-checked by a test that runs the real `cd` in a temporary
tree in each installed shell and asserts `pwd`:

| Shell (version) | `..` | `...` | `....` | `-` |
| --- | --- | --- | --- | --- |
| bash 5.2.37 | defined by the layer | defined by the layer | defined by the layer | defined by the layer |
| zsh 5.9 | defined by the layer | defined by the layer | defined by the layer | unsupported |
| fish 4.9.3 | native | defined by the layer | defined by the layer | defined by the layer |
| Nushell 0.116.1 | native | native | native | unsupported |

bash and zsh define aliases for the dotted names, and bash also for `-` (`alias -- -='cd -'`). In zsh `-` is a built-in precommand modifier (`- cmd` runs `cmd` with a leading dash in its name), and an alias would replace it, so the layer leaves it alone; `autocd` and `auto_cd` would
only cover `..`, so they are not used. fish defines abbreviations. Nushell's
implicit `cd` works only for a bare path typed alone in the interactive REPL, not
under `nu -c`, `-e` or after `;`. In Nushell `-` is unsupported: `alias - = cd -`
and `def "-"` are accepted but never run, because the parser treats a lone `-` as
a string, so the layer defines nothing there. A test requires this table to
match what the generator emits. The navigation names follow the same opt-out as
the shortcuts (`BDOCK_SHORTCUTS=0` in bash, zsh and fish).

| Shell | File in the toolbox | How it loads | Shortcuts |
| --- | --- | --- | --- |
| fish | `share/fish/vendor_conf.d/bdock-shell.fish` | `XDG_DATA_DIRS` | abbreviations, expanded in place |
| Nushell | `share/nushell/vendor/autoload/bdock-shell.nu` | `XDG_DATA_DIRS` | aliases and wrapped commands |
| zsh | `share/shell/init.zsh` | one line in the system zshrc | aliases, expanded in place on space |
| bash | `share/shell/init.bash` | one line in the system bashrc | aliases |

Nothing is enabled automatically. A consumer opts in by putting the toolbox's
`share` first in `XDG_DATA_DIRS` (fish and Nushell) and sourcing the bash or zsh
file from the image's system rc file, guarded so a missing toolbox is harmless:

```sh
XDG_DATA_DIRS=/opt/bundledock/toolbox/share:/usr/local/share:/usr/share
[ -r /opt/bundledock/toolbox/share/shell/init.zsh ] && . /opt/bundledock/toolbox/share/shell/init.zsh
```

Shortcuts that need a branch (`gpsup`, `grbm`, `gswm`) are functions in every
shell: they resolve the branch first and stop with `bdock: not on a branch
(detached HEAD)` before running Git, which matters in jj-colocated repositories.
The default branch is `origin/HEAD`, else `main`, else `master`. All files load
only in interactive shells. `BDOCK_SHORTCUTS=0` skips the shortcuts,
`BDOCK_STARSHIP=0` the prompt and `BDOCK_ZSH_EXPAND=0` zsh's expansion on space
(Ctrl+Space inserts a plain space). A prompt already initialized by the user's
own configuration (`STARSHIP_SHELL` set) is left alone. Nushell definitions
cannot be conditional, so there the shortcuts are opted out by leaving the
toolbox out of `XDG_DATA_DIRS`. Nushell's prompt setup is a static copy of what
`starship init nu` produces, because autoload files cannot evaluate generated
code.

The generator renders the four files from the table and they are committed.
The layer belongs to the toolbox alone: only the toolbox Dockerfile copies
`internal/shelllayer` into its build and passes it to `tools-bundle --shell-layer`,
which copies the files without running any shell. A toolbox build without the
layer fails, any other bundle refuses it, and neither the builder nor the
volume initializer that every bundle image ships imports the layer. After
editing the table, run
`go test ./internal/shelllayer -run TestGeneratedFilesAreCurrent -update`.
`TestShortcutsBehaveTheSameInEveryShell` runs every shortcut in each installed
shell against stub commands and requires identical arguments;
`TestBranchShortcutsRefuseDetachedHead` checks the detached-HEAD refusal.
Verified on 2026-10-15 with bash 5.2.37, zsh 5.9 and the pinned fish 4.9.3,
Nushell 0.116.1 and starship 1.26.0, including starship initialization in all
four shells, Nushell vendor autoload through `XDG_DATA_DIRS` and zsh's
expansion in a pseudo-terminal. zsh is not part of the toolbox: no official
upstream prebuilt release exists, so it comes from the consumer image.

## Optional browser editor

The [code-server bundle](code-server.md) provides a separate optional OCI
carrier with its own lock and read-only volume. It does not change the toolbox
or start a service automatically.


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
