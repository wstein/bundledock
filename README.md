# BundleDock

![BundleDock — Build and ship OCI bundles.](assets/banner.svg)

[![CI](https://github.com/wstein/bundledock/actions/workflows/ci.yml/badge.svg)](https://github.com/wstein/bundledock/actions/workflows/ci.yml)
[![Go 1.24 or newer](https://img.shields.io/badge/Go-1.24%2B-00ADD8?logo=go&logoColor=white)](go.mod)
[![Linux amd64 and arm64](https://img.shields.io/badge/Linux-amd64%20%7C%20arm64-0b2545?logo=linux&logoColor=white)](tools.lock.json)
[![OCI toolbox](https://img.shields.io/badge/OCI-toolbox-5eead4?labelColor=0b2545)](oci/Dockerfile)
[![Pinned tool catalog](https://img.shields.io/badge/tools-pinned-5eead4?labelColor=0b2545)](tools.packages.json)

BundleDock builds an OCI toolbox of pinned development CLIs and packaged
runtime assets for shared, read-only use in Linux containers. The toolbox supplies
common utilities alongside stock language images, such as Go or Rust images,
which provide their own language toolchains.

Build one toolbox, share it across your development containers, and keep tool
versions consistent without adding common utilities to every language image.
BundleDock is a proof of concept for Linux AMD64 and ARM64.

## Quick start

You need Docker running and a local clone of this repository. From its root:

```sh
make tools-build
make tools-smoke
make tools-volume
```

The build downloads the pinned tools and verifies their checksums. Smoke checks
exercise them in a compatible consumer container. The last command prints an
image-derived volume name; substitute it below:

```sh
docker run --rm -it \
  --mount type=volume,src=<printed-volume-name>,dst=/opt/workharbor,readonly,volume-nocopy \
  --env DISABLE_UPDATES=1 --env DISABLE_AUTOUPDATER=1 \
  mcr.microsoft.com/devcontainers/go:2-1.27-trixie \
  sh -c 'export PATH="/opt/workharbor/bin:$PATH"; exec bash'
```

Keep credentials, configuration, caches, and environments in writable home
directories or private volumes. The toolbox is read-only and contains no
credentials.

The OCI image is a volume carrier built from `scratch`. Run its tools in a
compatible consumer, which supplies Git, certificates, terminfo, and system
libraries. See [consumer requirements](docs/architecture.md#consumer-compatibility-and-build-layout)
for runtime details. Anchorage owns the separate devcontainer integration and
its image and volume pins. The current devcontainer pins an older image;
rebuild the toolbox and initialize its new volume before adopting these tools.

## Included tools

The bundle pins 22 tools, with these companion commands:

| Use | Tools |
| --- | --- |
| Agents and GitHub | Codex, Claude Code, Antigravity (`agy`), GitHub CLI (`gh`) |
| Search and structured data | ripgrep (`rg`), fd (`fd`, `fdfind`), ast-grep, jq, yq |
| Python environments | uv and uvx |
| Shells and terminal workspaces | [Herdr](https://github.com/herdrdev/herdr), Fish, Starship, Nushell (`nu`), tmux |
| Quality and security | typos, lychee, Gitleaks, Trivy, Syft |
| Process monitoring | bottom (`btm`), btop |

See [pinned versions and upgrades](docs/tools.md). Codex includes its native
helpers; optional voice resources are omitted.

## Next steps

- [Upgrade tools](docs/tools.md#upgrade-the-lock): edit desired ranges, resolve a lock, rebuild, and initialize a new volume.
- [Understand the bundle](docs/architecture.md): shared volumes, rollback, runtime dependencies, and measured voice savings.
- [Develop and verify](docs/development.md): CLI commands, checks, smoke coverage, and revision-specific evidence.
- [Publish to Quay](docs/publication.md): configure a robot account and run the weekly or manual publication workflow.

The [CI workflow](.github/workflows/ci.yml) checks pushes and pull requests and
builds/smokes both native architectures. See the [verification record](docs/development.md#verification-evidence)
for what has actually passed and the remaining limits. Quay publication and
authenticated agent tasks require separate verification.

Upstream tool notices are kept in [oci/licenses](oci/licenses); brand licensing
and asset provenance are documented in [brand/README.md](brand/README.md).
BundleDock's [source and full-history provenance](docs/development.md#repository-history)
are independent of image publication and consumer activation.
