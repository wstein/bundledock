# BundleDock

![BundleDock — Build and ship OCI bundles.](assets/banner.svg)

[![CLI CI](https://github.com/wstein/bundledock/actions/workflows/ci.yml/badge.svg)](https://github.com/wstein/bundledock/actions/workflows/ci.yml)
[![Go 1.25 or newer](https://img.shields.io/badge/Go-1.25%2B-00ADD8?logo=go&logoColor=white)](go.mod)
[![Documentation deployment](https://github.com/wstein/bundledock/actions/workflows/pages.yml/badge.svg?branch=main)](https://github.com/wstein/bundledock/actions/workflows/pages.yml)
[![License: EUPL 1.2](https://img.shields.io/badge/license-EUPL--1.2-blue)](LICENSE)
[![Supported platforms: Linux AMD64 and ARM64](https://img.shields.io/badge/platforms-Linux%20AMD64%20%7C%20ARM64-blue)](docs/architecture.md#consumer-compatibility-and-build-layout)
[![Container: ready](https://img.shields.io/badge/container-ready-5eead4)](https://quay.io/organization/bundledock)

BundleDock packages official stable prebuilt tools and runtimes, plus the
reviewed [git-trim source exception](docs/toolbox.md#git-trim-source-exception), as OCI volume
carriers for shared, read-only use in Linux development containers. Each bundle
has its own catalog, lock, image and volume. Stock language images supply the
consumer environment; the toolbox supplies common utilities alongside them.
The supported bundle platforms are Linux AMD64 and ARM64. See each guide for
its consumer requirements and measured runtime coverage.

The public CLI is `bdock`. For development, use `make run ARGS="--help"`;
after an approved local build, run `bin/bdock --help`. Existing CLI users should
replace `bundledock` invocations with `bdock`; no compatibility alias is shipped.
The BundleDock project name, repository, image references and volume paths stay
the same.

## Bundles

Published version badges link to [quay.io](https://quay.io/organization/bundledock).
Node badges show the upstream version recorded by the latest merged publication
update; packaging revisions remain in the bundle guides.

### Utilities

| Bundle | Purpose and guide | Published version | Publication workflow |
| --- | --- | --- | --- |
| Toolbox | [Development CLIs and common utilities](docs/toolbox.md) | [![Published toolbox version](https://img.shields.io/badge/dynamic/regex?url=https%3A%2F%2Fquay.io%2Fapi%2Fv1%2Frepository%2Fbundledock%2Ftoolbox%2Ftag%2F%3FonlyActiveTags%3Dtrue%26limit%3D100%26page%3D1&search=%22name%22%5Cs%2A%3A%5Cs%2A%22%28%5Cd%7B4%7D%5C.%5Cd%7B2%7D%5C.%5Cd%7B2%7D-%28%3F%3Ab%5Cd%2B%5C.%5Cd%2B%7C%5Cd%2B-%5Cd%2B%29%29%22&replace=%241&label=version&color=5eead4&labelColor=0b2545)](https://quay.io/repository/bundledock/toolbox?tab=tags) | [![Publish toolbox](https://github.com/wstein/bundledock/actions/workflows/bundledock-toolbox.yml/badge.svg?branch=main)](https://github.com/wstein/bundledock/actions/workflows/bundledock-toolbox.yml) |

### Runtimes

| Bundle | Purpose and guide | Published version | Publication workflow |
| --- | --- | --- | --- |
| Bun | [Bun runtime and bunx](docs/bun.md) | [![Published bun version](https://img.shields.io/badge/dynamic/regex?url=https%3A%2F%2Fquay.io%2Fapi%2Fv1%2Frepository%2Fbundledock%2Fbun%2Ftag%2F%3FonlyActiveTags%3Dtrue%26limit%3D100%26page%3D1&search=%22name%22%5Cs%2A%3A%5Cs%2A%22%28%5Cd%2B%5C.%5Cd%2B%5C.%5Cd%2B-b%5Cd%2B%7C%5Cd%7B4%7D%5C.%5Cd%7B2%7D%5C.%5Cd%7B2%7D-%28%3F%3Ab%5Cd%2B%5C.%5Cd%2B%7C%5Cd%2B-%5Cd%2B%29%29%22&replace=%241&label=version&color=5eead4&labelColor=0b2545)](https://quay.io/repository/bundledock/bun?tab=tags) | [![Publish bun](https://github.com/wstein/bundledock/actions/workflows/bundledock-bun.yml/badge.svg?branch=main)](https://github.com/wstein/bundledock/actions/workflows/bundledock-bun.yml) |
| Node.js | [Node.js LTS with npm/npx](docs/node.md) | [![Published Node LTS version](https://img.shields.io/badge/dynamic/json?url=https%3A%2F%2Fraw.githubusercontent.com%2Fwstein%2Fbundledock%2Fmain%2Fdocs%2Fnode-published.json&query=%24.upstream_version&label=LTS&color=5eead4&labelColor=0b2545)](https://quay.io/repository/bundledock/node?tab=tags&tag=lts) | [![Publish node](https://github.com/wstein/bundledock/actions/workflows/bundledock-node.yml/badge.svg?branch=main)](https://github.com/wstein/bundledock/actions/workflows/bundledock-node.yml) |
| Node.js Current | [Node.js Current with npm/npx](docs/node-current.md) | [![Published Node Current version](https://img.shields.io/badge/dynamic/json?url=https%3A%2F%2Fraw.githubusercontent.com%2Fwstein%2Fbundledock%2Fmain%2Fdocs%2Fnode-current-published.json&query=%24.upstream_version&label=Current&color=5eead4&labelColor=0b2545)](https://quay.io/repository/bundledock/node?tab=tags&tag=current) | [![Publish node-current](https://github.com/wstein/bundledock/actions/workflows/bundledock-node-current.yml/badge.svg?branch=main)](https://github.com/wstein/bundledock/actions/workflows/bundledock-node-current.yml) |

### Infrastructure

| Bundle | Purpose and guide | Published version | Publication workflow |
| --- | --- | --- | --- |
| OpenTofu | [Official OpenTofu CLI — source prepared, publication pending](docs/opentofu.md) | [![Registry OpenTofu version (publication pending)](https://img.shields.io/badge/dynamic/regex?url=https%3A%2F%2Fquay.io%2Fapi%2Fv1%2Frepository%2Fbundledock%2Fopentofu%2Ftag%2F%3FonlyActiveTags%3Dtrue%26limit%3D100%26page%3D1&search=%22name%22%5Cs%2A%3A%5Cs%2A%22%28%5Cd%2B%5C.%5Cd%2B%5C.%5Cd%2B-b%5Cd%2B%7C%5Cd%7B4%7D%5C.%5Cd%7B2%7D%5C.%5Cd%7B2%7D-%28%3F%3Ab%5Cd%2B%5C.%5Cd%2B%7C%5Cd%2B-%5Cd%2B%29%29%22&replace=%241&label=version&color=5eead4&labelColor=0b2545)](https://quay.io/repository/bundledock/opentofu?tab=tags) | [![Publish opentofu](https://github.com/wstein/bundledock/actions/workflows/bundledock-opentofu.yml/badge.svg?branch=main)](https://github.com/wstein/bundledock/actions/workflows/bundledock-opentofu.yml) |

### Editors

| Bundle | Purpose and guide | Published version | Publication workflow |
| --- | --- | --- | --- |
| code-server | [Standalone browser editor](docs/code-server.md) | [![Published code-server version](https://img.shields.io/badge/dynamic/regex?url=https%3A%2F%2Fquay.io%2Fapi%2Fv1%2Frepository%2Fbundledock%2Fcode-server%2Ftag%2F%3FonlyActiveTags%3Dtrue%26limit%3D100%26page%3D1&search=%22name%22%5Cs%2A%3A%5Cs%2A%22%28%5Cd%2B%5C.%5Cd%2B%5C.%5Cd%2B-b%5Cd%2B%7C%5Cd%7B4%7D%5C.%5Cd%7B2%7D%5C.%5Cd%7B2%7D-%28%3F%3Ab%5Cd%2B%5C.%5Cd%2B%7C%5Cd%2B-%5Cd%2B%29%29%22&replace=%241&label=version&color=5eead4&labelColor=0b2545)](https://quay.io/repository/bundledock/code-server?tab=tags) | [![Publish code-server](https://github.com/wstein/bundledock/actions/workflows/bundledock-code-server.yml/badge.svg?branch=main)](https://github.com/wstein/bundledock/actions/workflows/bundledock-code-server.yml) |

### Agents

| Bundle | Purpose and guide | Published version | Publication workflow |
| --- | --- | --- | --- |
| Antigravity | [Official Antigravity CLI](docs/agy.md) | [![Published agy version](https://img.shields.io/badge/dynamic/regex?url=https%3A%2F%2Fquay.io%2Fapi%2Fv1%2Frepository%2Fbundledock%2Fagy%2Ftag%2F%3FonlyActiveTags%3Dtrue%26limit%3D100%26page%3D1&search=%22name%22%5Cs%2A%3A%5Cs%2A%22%28%5Cd%2B%5C.%5Cd%2B%5C.%5Cd%2B-b%5Cd%2B%7C%5Cd%7B4%7D%5C.%5Cd%7B2%7D%5C.%5Cd%7B2%7D-%28%3F%3Ab%5Cd%2B%5C.%5Cd%2B%7C%5Cd%2B-%5Cd%2B%29%29%22&replace=%241&label=version&color=5eead4&labelColor=0b2545)](https://quay.io/repository/bundledock/agy?tab=tags) | [![Publish agy](https://github.com/wstein/bundledock/actions/workflows/bundledock-agy.yml/badge.svg?branch=main)](https://github.com/wstein/bundledock/actions/workflows/bundledock-agy.yml) |
| Claude Code | [Official Claude CLI](docs/claude.md) | [![Published claude version](https://img.shields.io/badge/dynamic/regex?url=https%3A%2F%2Fquay.io%2Fapi%2Fv1%2Frepository%2Fbundledock%2Fclaude%2Ftag%2F%3FonlyActiveTags%3Dtrue%26limit%3D100%26page%3D1&search=%22name%22%5Cs%2A%3A%5Cs%2A%22%28%5Cd%2B%5C.%5Cd%2B%5C.%5Cd%2B-b%5Cd%2B%7C%5Cd%7B4%7D%5C.%5Cd%7B2%7D%5C.%5Cd%7B2%7D-%28%3F%3Ab%5Cd%2B%5C.%5Cd%2B%7C%5Cd%2B-%5Cd%2B%29%29%22&replace=%241&label=version&color=5eead4&labelColor=0b2545)](https://quay.io/repository/bundledock/claude?tab=tags) | [![Publish claude](https://github.com/wstein/bundledock/actions/workflows/bundledock-claude.yml/badge.svg?branch=main)](https://github.com/wstein/bundledock/actions/workflows/bundledock-claude.yml) |
| Codex | [Official Codex package](docs/codex.md) | [![Published codex version](https://img.shields.io/badge/dynamic/regex?url=https%3A%2F%2Fquay.io%2Fapi%2Fv1%2Frepository%2Fbundledock%2Fcodex%2Ftag%2F%3FonlyActiveTags%3Dtrue%26limit%3D100%26page%3D1&search=%22name%22%5Cs%2A%3A%5Cs%2A%22%28%5Cd%2B%5C.%5Cd%2B%5C.%5Cd%2B-b%5Cd%2B%7C%5Cd%7B4%7D%5C.%5Cd%7B2%7D%5C.%5Cd%7B2%7D-%28%3F%3Ab%5Cd%2B%5C.%5Cd%2B%7C%5Cd%2B-%5Cd%2B%29%29%22&replace=%241&label=version&color=5eead4&labelColor=0b2545)](https://quay.io/repository/bundledock/codex?tab=tags) | [![Publish codex](https://github.com/wstein/bundledock/actions/workflows/bundledock-codex.yml/badge.svg?branch=main)](https://github.com/wstein/bundledock/actions/workflows/bundledock-codex.yml) |

The three agent carriers have verified publication receipts in their guides.
The current source toolbox excludes these agents; the published toolbox used
below still contains them and predates the fzf, bat and regctl additions.
A reduced toolbox image requires a separately approved publication. Consumer
activation follows the consumer project’s instructions.

**[Documentation](https://wstein.github.io/bundledock/)** — quick start and guides
for using, maintaining, and publishing each bundle.

## Quick start

You need Docker running and a local clone of this repository. Pull the published
toolbox by its immutable multi-platform index digest, then initialize its volume:

```sh
TOOLBOX_IMAGE=quay.io/bundledock/toolbox@sha256:40bae37ef7fd7b8e7131803850801ed41cfb23a6ef774bc2e40f95ee81e59a1c
docker pull "$TOOLBOX_IMAGE"
make bundle-volume BUNDLE=toolbox IMAGE="$TOOLBOX_IMAGE"
```

This example is toolbox version `2026.10.10-b4.1`, published from
[`812e7caac782595b9302292603bbe49e890ca96e`](https://github.com/wstein/bundledock/commit/812e7caac782595b9302292603bbe49e890ca96e)
in [publication run 38088214128, attempt 1](https://github.com/wstein/bundledock/actions/runs/38088214128/attempts/1).
It predates the source catalog additions of fzf, bat and regctl. The initializer
prints `Volume: bundledock-toolbox-<full image ID>`; use that volume name below:

```sh
docker run --rm -it \
  --mount type=volume,src=<printed-volume-name>,dst=/opt/bundledock/toolbox,readonly,volume-nocopy \
  --env DISABLE_UPDATES=1 --env DISABLE_AUTOUPDATER=1 \
  mcr.microsoft.com/devcontainers/go:2-1.27-trixie \
  sh -c 'export PATH="/opt/bundledock/toolbox/bin:$PATH"; exec bash'
```

Keep credentials, configuration, caches, and environments in writable home
directories or private volumes. The toolbox is read-only and contains no
credentials.

The OCI image is a volume carrier built from `scratch`. Run its tools in a
compatible consumer, which supplies Git, certificates, terminfo, and system
libraries. See [consumer requirements](docs/architecture.md#consumer-compatibility-and-build-layout)
for runtime details. Anchorage owns the separate devcontainer integration and
its [image and volume pins](https://github.com/wstein/Anchorage). Follow that
consumer project’s instructions to adopt a published bundle.

## Toolbox source catalog

The current source catalog pins 33 tools, with these companion commands. This
describes the desired source inventory; published images retain their own pins:

| Use | Tools |
| --- | --- |
| Git branch cleanup plans | [git-trim (`git trim --dry-run --no-update`)](https://github.com/foriequal0/git-trim) |
| GitHub workflows | [GitHub CLI (`gh`)](https://github.com/cli/cli) |
| Search and structured data | [ripgrep (`rg`)](https://github.com/BurntSushi/ripgrep), [fd (`fd`, `fdfind`)](https://github.com/sharkdp/fd), [ast-grep](https://github.com/ast-grep/ast-grep), [jq](https://github.com/jqlang/jq), [yq](https://github.com/mikefarah/yq) |
| Interactive filtering and file viewing | [fzf](https://github.com/junegunn/fzf), [bat](https://github.com/sharkdp/bat) |
| Markdown preview | [Leaf](https://github.com/RivoLink/leaf) |
| Development utilities | [ShellCheck](https://github.com/koalaman/shellcheck), [shfmt](https://github.com/mvdan/sh), [just](https://github.com/casey/just), [Ruff](https://github.com/astral-sh/ruff), [delta](https://github.com/dandavison/delta), [watchexec](https://github.com/watchexec/watchexec), [hyperfine](https://github.com/sharkdp/hyperfine), [age](https://github.com/FiloSottile/age) |
| OCI registries | [regctl](https://github.com/regclient/regclient) |
| Python environments | [uv and uvx](https://github.com/astral-sh/uv) |
| Spec-driven project setup | [Spec Kit (`specify`, pinned runtime launcher)](docs/toolbox.md#spec-kit-launcher-source) |
| Shells and terminal workspaces | [Herdr](https://github.com/herdrdev/herdr), [Fish](https://github.com/fish-shell/fish-shell), [Starship](https://github.com/starship/starship), [Nushell (`nu`)](https://github.com/nushell/nushell), [tmux](https://github.com/tmux/tmux-builds) |
| Quality and security | [typos](https://github.com/crate-ci/typos), [lychee](https://github.com/lycheeverse/lychee), [Gitleaks](https://github.com/gitleaks/gitleaks), [Trivy](https://github.com/aquasecurity/trivy), [Syft](https://github.com/anchore/syft) |
| Optional version manager | [mise](https://github.com/jdx/mise) |
| Terminal editor | [Helix (`hx`)](https://github.com/helix-editor/helix) |
| Static sites | [Hugo (standard)](https://github.com/gohugoio/hugo) |
| Process monitoring | [bottom (`btm`)](https://github.com/ClementTsang/bottom), [btop](https://github.com/aristocratos/btop) |

See [pinned versions and upgrades](docs/toolbox.md). Agents are supplied by the
separate [Codex](docs/codex.md), [Antigravity](docs/agy.md) and
[Claude Code](docs/claude.md) bundles. Codex retains its native helpers while
omitting optional voice resources.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for contribution and verification guidance.
Participation in the BundleDock community follows our [Code of Conduct](CODE_OF_CONDUCT.md).

## Documentation

Browse the [documentation site](https://wstein.github.io/bundledock/) or read the
Markdown sources in this repository:

- [CLI and quick start](https://wstein.github.io/bundledock/cli/) ([source](README.md)): pull the published toolbox and use its shared volume.
- [Tool catalog and upgrades](https://wstein.github.io/bundledock/guides/toolbox/) ([source](docs/toolbox.md)): pinned versions and lock updates.
- [Architecture](https://wstein.github.io/bundledock/guides/architecture/) ([source](docs/architecture.md)): shared volumes, rollback, and consumer requirements.
- [Development and verification](https://wstein.github.io/bundledock/guides/development/) ([source](docs/development.md)): CLI commands, checks, and recorded coverage.
- [Publication](https://wstein.github.io/bundledock/guides/publication/) ([source](docs/publication.md)): Quay setup, updater approval and manual exact-SHA publication.

The [CI workflow](.github/workflows/ci.yml) runs formatting, source tests and vet.
Maintainers can find build approval, publication and verification details in the
[publication guide](docs/publication.md) and [verification record](docs/development.md#verification-evidence).

BundleDock is licensed under the [European Union Public Licence 1.2 (EUPL-1.2)](LICENSE).

Upstream tool notices are kept in [bundles/toolbox/licenses](bundles/toolbox/licenses); brand licensing
and asset provenance are documented in [brand/README.md](brand/README.md).
BundleDock's [source and full-history provenance](docs/development.md#repository-history)
are independent of image publication and consumer activation.
