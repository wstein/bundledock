# BundleDock

![BundleDock — Build and ship OCI bundles.](assets/banner.svg)

[![CLI CI](https://github.com/wstein/bundledock/actions/workflows/ci.yml/badge.svg)](https://github.com/wstein/bundledock/actions/workflows/ci.yml)
[![Go 1.25 or newer](https://img.shields.io/badge/Go-1.25%2B-00ADD8?logo=go&logoColor=white)](go.mod)
BundleDock packages official stable prebuilt tools and runtimes as OCI volume
carriers for shared, read-only use in Linux development containers. Each bundle
has its own catalog, lock, image and volume. Stock language images supply the
consumer environment; the toolbox supplies common utilities alongside them.
The supported bundle platforms are Linux AMD64 and ARM64. See each guide for
its consumer requirements and measured runtime coverage.

| Bundle | Purpose and guide | Published version | Publication workflow |
| --- | --- | --- | --- |
| Toolbox | [Development CLIs and common utilities](docs/toolbox.md) | [![Published toolbox version](https://img.shields.io/badge/dynamic/regex?url=https%3A%2F%2Fquay.io%2Fapi%2Fv1%2Frepository%2Fbundledock%2Ftoolbox%2Ftag%2F%3FonlyActiveTags%3Dtrue%26limit%3D100%26page%3D1&search=%22name%22%5Cs%2A%3A%5Cs%2A%22%28%5Cd%7B4%7D%5C.%5Cd%7B2%7D%5C.%5Cd%7B2%7D-%28%3F%3Ab%5Cd%2B%5C.%5Cd%2B%7C%5Cd%2B-%5Cd%2B%29%29%22&replace=%241&label=version&color=5eead4&labelColor=0b2545)](https://quay.io/repository/bundledock/toolbox?tab=tags) | [![Publish toolbox](https://github.com/wstein/bundledock/actions/workflows/bundledock-toolbox.yml/badge.svg?branch=main)](https://github.com/wstein/bundledock/actions/workflows/bundledock-toolbox.yml) |
| code-server | [Standalone browser editor](docs/code-server.md) | [![Published code-server version](https://img.shields.io/badge/dynamic/regex?url=https%3A%2F%2Fquay.io%2Fapi%2Fv1%2Frepository%2Fbundledock%2Fcode-server%2Ftag%2F%3FonlyActiveTags%3Dtrue%26limit%3D100%26page%3D1&search=%22name%22%5Cs%2A%3A%5Cs%2A%22%28%5Cd%2B%5C.%5Cd%2B%5C.%5Cd%2B-b%5Cd%2B%7C%5Cd%7B4%7D%5C.%5Cd%7B2%7D%5C.%5Cd%7B2%7D-%28%3F%3Ab%5Cd%2B%5C.%5Cd%2B%7C%5Cd%2B-%5Cd%2B%29%29%22&replace=%241&label=version&color=5eead4&labelColor=0b2545)](https://quay.io/repository/bundledock/code-server?tab=tags) | [![Publish code-server](https://github.com/wstein/bundledock/actions/workflows/bundledock-code-server.yml/badge.svg?branch=main)](https://github.com/wstein/bundledock/actions/workflows/bundledock-code-server.yml) |
| Node.js | [Node.js LTS with npm/npx](docs/node.md) | [![Node LTS publication](https://img.shields.io/badge/Node-LTS-5eead4?labelColor=0b2545)](docs/node.md#published-bundle) | [![Publish node](https://github.com/wstein/bundledock/actions/workflows/bundledock-node.yml/badge.svg?branch=main)](https://github.com/wstein/bundledock/actions/workflows/bundledock-node.yml) |
| Node.js Current | [Node.js Current — planned, unpublished](docs/node-current.md) | [![Node Current publication pending](https://img.shields.io/badge/Current-unpublished-64748b?labelColor=0b2545)](docs/node-current.md#published-bundle) | [![Publish node-current](https://github.com/wstein/bundledock/actions/workflows/bundledock-node-current.yml/badge.svg?branch=main)](https://github.com/wstein/bundledock/actions/workflows/bundledock-node-current.yml) |
| Bun | [Bun runtime and bunx](docs/bun.md) | [![Published bun version](https://img.shields.io/badge/dynamic/regex?url=https%3A%2F%2Fquay.io%2Fapi%2Fv1%2Frepository%2Fbundledock%2Fbun%2Ftag%2F%3FonlyActiveTags%3Dtrue%26limit%3D100%26page%3D1&search=%22name%22%5Cs%2A%3A%5Cs%2A%22%28%5Cd%2B%5C.%5Cd%2B%5C.%5Cd%2B-b%5Cd%2B%7C%5Cd%7B4%7D%5C.%5Cd%7B2%7D%5C.%5Cd%7B2%7D-%28%3F%3Ab%5Cd%2B%5C.%5Cd%2B%7C%5Cd%2B-%5Cd%2B%29%29%22&replace=%241&label=version&color=5eead4&labelColor=0b2545)](https://quay.io/repository/bundledock/bun?tab=tags) | [![Publish bun](https://github.com/wstein/bundledock/actions/workflows/bundledock-bun.yml/badge.svg?branch=main)](https://github.com/wstein/bundledock/actions/workflows/bundledock-bun.yml) |
| Codex | [Official Codex package — planned, no verified publication](docs/codex.md) | [![Registry codex version (publication pending)](https://img.shields.io/badge/dynamic/regex?url=https%3A%2F%2Fquay.io%2Fapi%2Fv1%2Frepository%2Fbundledock%2Fcodex%2Ftag%2F%3FonlyActiveTags%3Dtrue%26limit%3D100%26page%3D1&search=%22name%22%5Cs%2A%3A%5Cs%2A%22%28%5Cd%2B%5C.%5Cd%2B%5C.%5Cd%2B-b%5Cd%2B%7C%5Cd%7B4%7D%5C.%5Cd%7B2%7D%5C.%5Cd%7B2%7D-%28%3F%3Ab%5Cd%2B%5C.%5Cd%2B%7C%5Cd%2B-%5Cd%2B%29%29%22&replace=%241&label=version&color=5eead4&labelColor=0b2545)](https://quay.io/repository/bundledock/codex?tab=tags) | [![Publish codex](https://github.com/wstein/bundledock/actions/workflows/bundledock-codex.yml/badge.svg?branch=main)](https://github.com/wstein/bundledock/actions/workflows/bundledock-codex.yml) |
| Antigravity | [Official Antigravity CLI — planned, no verified publication](docs/agy.md) | [![Registry agy version (publication pending)](https://img.shields.io/badge/dynamic/regex?url=https%3A%2F%2Fquay.io%2Fapi%2Fv1%2Frepository%2Fbundledock%2Fagy%2Ftag%2F%3FonlyActiveTags%3Dtrue%26limit%3D100%26page%3D1&search=%22name%22%5Cs%2A%3A%5Cs%2A%22%28%5Cd%2B%5C.%5Cd%2B%5C.%5Cd%2B-b%5Cd%2B%7C%5Cd%7B4%7D%5C.%5Cd%7B2%7D%5C.%5Cd%7B2%7D-%28%3F%3Ab%5Cd%2B%5C.%5Cd%2B%7C%5Cd%2B-%5Cd%2B%29%29%22&replace=%241&label=version&color=5eead4&labelColor=0b2545)](https://quay.io/repository/bundledock/agy?tab=tags) | [![Publish agy](https://github.com/wstein/bundledock/actions/workflows/bundledock-agy.yml/badge.svg?branch=main)](https://github.com/wstein/bundledock/actions/workflows/bundledock-agy.yml) |
| Claude Code | [Official Claude CLI — planned, no verified publication](docs/claude.md) | [![Registry claude version (publication pending)](https://img.shields.io/badge/dynamic/regex?url=https%3A%2F%2Fquay.io%2Fapi%2Fv1%2Frepository%2Fbundledock%2Fclaude%2Ftag%2F%3FonlyActiveTags%3Dtrue%26limit%3D100%26page%3D1&search=%22name%22%5Cs%2A%3A%5Cs%2A%22%28%5Cd%2B%5C.%5Cd%2B%5C.%5Cd%2B-b%5Cd%2B%7C%5Cd%7B4%7D%5C.%5Cd%7B2%7D%5C.%5Cd%7B2%7D-%28%3F%3Ab%5Cd%2B%5C.%5Cd%2B%7C%5Cd%2B-%5Cd%2B%29%29%22&replace=%241&label=version&color=5eead4&labelColor=0b2545)](https://quay.io/repository/bundledock/claude?tab=tags) | [![Publish claude](https://github.com/wstein/bundledock/actions/workflows/bundledock-claude.yml/badge.svg?branch=main)](https://github.com/wstein/bundledock/actions/workflows/bundledock-claude.yml) |

The three agent carriers are prepared in source and have no verified publication
receipts yet. Their registry badges may be unavailable until an explicitly
approved first publication. Toolbox retains all three agents while Anchorage
consumers are prepared; activation and later removal are separate changes.

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

The current source catalog pins 27 tools, with these companion commands. This
describes the desired source inventory; published images retain their own pins:

| Use | Tools |
| --- | --- |
| Agents and GitHub | [Codex](https://github.com/openai/codex), [Claude Code](https://github.com/anthropics/claude-code), [Antigravity (`agy`)](https://github.com/google-antigravity/antigravity-cli), [GitHub CLI (`gh`)](https://github.com/cli/cli) |
| Search and structured data | [ripgrep (`rg`)](https://github.com/BurntSushi/ripgrep), [fd (`fd`, `fdfind`)](https://github.com/sharkdp/fd), [ast-grep](https://github.com/ast-grep/ast-grep), [jq](https://github.com/jqlang/jq), [yq](https://github.com/mikefarah/yq) |
| Interactive filtering and file viewing | [fzf](https://github.com/junegunn/fzf), [bat](https://github.com/sharkdp/bat) |
| OCI registries | [regctl](https://github.com/regclient/regclient) |
| Python environments | [uv and uvx](https://github.com/astral-sh/uv) |
| Shells and terminal workspaces | [Herdr](https://github.com/herdrdev/herdr), [Fish](https://github.com/fish-shell/fish-shell), [Starship](https://github.com/starship/starship), [Nushell (`nu`)](https://github.com/nushell/nushell), [tmux](https://github.com/tmux/tmux-builds) |
| Quality and security | [typos](https://github.com/crate-ci/typos), [lychee](https://github.com/lycheeverse/lychee), [Gitleaks](https://github.com/gitleaks/gitleaks), [Trivy](https://github.com/aquasecurity/trivy), [Syft](https://github.com/anchore/syft) |
| Terminal editor | [Helix (`hx`)](https://github.com/helix-editor/helix) |
| Static sites | [Hugo (standard)](https://github.com/gohugoio/hugo) |
| Process monitoring | [bottom (`btm`)](https://github.com/ClementTsang/bottom), [btop](https://github.com/aristocratos/btop) |

See [pinned versions and upgrades](docs/toolbox.md). Codex includes its native
helpers; optional voice resources are omitted.

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
