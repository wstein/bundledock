---
title: Architecture and consumer runtime
description: Understand the carrier image, shared volumes, bundle layout, and consumer requirements.
weight: 30
---

# Architecture and consumer runtime

[Back to BundleDock](../README.md)

## Toolbox carrier

Builds verify every download. The final image uses `scratch` and contains only the bundle and its
statically compiled volume initializer. Build-only stages use digest-pinned
multi-platform official Alpine images: Go 1.27.2 for compilation and Alpine 3.23
for downloads. The download stage installs only CA certificates from Alpine's
package repositories; the static Go builder handles HTTPS downloads, checksum
verification and archive extraction without adding curl, tar or a C compiler.
CI uses the same patched Go release from `.go-version`; `go.mod` retains the
minimum supported source toolchain of Go 1.25. Alpine is a build dependency,
independent of the consumer's Debian runtime
libraries and compatibility requirements below.

The image is a carrier for volume initialization. Direct execution of packaged
tools inside the carrier is unsupported: the consumer supplies its shell, Git,
CA certificates, terminfo and compatible system libraries, including libstdc++,
libgcc, libatomic and libsecret when required. The supported Debian Trixie Go
consumer remains unchanged. The initializer uses an absolute entrypoint and
only source/target filesystem access; it needs no shell, network or temporary
directory. Smoke mutation probes run in the consumer, while repeat and rejected
initialization still use the carrier's initializer.

## Codex without voice

Codex's optional voice helper and private runtime under `codex-resources/voice`
are omitted. The CLI, code mode host, packaged ripgrep, bwrap, zsh and package
metadata remain; voice features requiring that helper are unavailable. For the
locked 0.162.1 official archives, the 38 omitted regular files save:

| Platform | Omitted unpacked bytes | Share of Codex regular-file bytes |
| --- | ---: | ---: |
| AMD64 | 77,181,888 | 17.06% |
| ARM64 | 80,512,453 | 19.73% |

These are measured file payload savings, excluding filesystem overhead. The
original compressed downloads (162,905,879 bytes AMD64; 153,057,922 bytes ARM64)
and their hashes remain unchanged. Compressed OCI image savings are unmeasured.
The volume is shared across workspaces on the same Docker host.

## Shared volumes

`bundle-volume` prints a volume name derived from the resolved image ID. It copies
the tools once and verifies their checksums on subsequent calls. Incomplete,
nonempty, or mismatched volumes are rejected. Upgrades create a new image and
volume; rollback selects the previous image. Volumes are local to each Docker host.
The Go builder emits deterministic metadata, with its own JSON serialization; it
does not guarantee byte compatibility with legacy Python-built manifests. Build a
new image and use its image-derived volume instead of reusing a legacy volume.
Initialization never removes an existing lock or retries a partial nonempty volume.

Follow the [quick start](../README.md#quick-start) to mount the printed volume.

Bundle roots use `/opt/bundledock/<bundle-name>`. For example, mount the
`toolbox` read-only at `/opt/bundledock/toolbox` and put
`/opt/bundledock/toolbox/bin` on PATH. The optional [code-server](code-server.md),
[Node.js LTS](node.md), [Node.js Current](node-current.md), [Bun](bun.md),
[Codex](codex.md), [Antigravity](agy.md), [Claude](claude.md) and
[OpenTofu](opentofu.md) bundles use sibling roots and separate
volumes. Each bundle keeps its own metadata, assets and licenses inside its root.
The builder accepts `--destination` for another root. Bundle metadata and packaged
assets use relative paths, so an existing image-derived volume can be mounted there
without changing its image or volume identity. The initializer still mounts the volume at `/bundle` while copying or
verifying it. New images read their source tree from `/opt/bundledock/toolbox`; the
initializer accepts `--source` and `--target` overrides for other locations.
Older images retain their own initializer and source prefix; their relative
volume contents can also be mounted at `/opt/bundledock/toolbox`. New volume names use
`bundledock-toolbox-<digest>`. To select an existing legacy volume by name, run
`sh bundles/toolbox/scripts/volume.sh <original-image> <existing-volume-name>`.
Keep authentication and writable tool state in the user's home directory or
separate private volumes. No credentials are included in the image. The read-only mount prevents tools from updating the
shared executables; Claude updates are also disabled with environment variables.

## Consumer compatibility and build layout

The supported initial consumer baseline is the Debian Trixie Go devcontainer.
Other images must supply compatible system libraries, Git, and CA certificates;
mounting this volume does not install those dependencies. Override `IMAGE`
or `CONSUMER` when invoking Make to select another image or test consumer.
The shared `bundle-build`, `bundle-smoke` and `bundle-volume` targets accept
`BUNDLE=toolbox|code-server|node|node-current|bun|codex|agy|claude|opentofu` (default `toolbox`). Each bundle uses its own
Dockerfile and smoke script; volume initialization uses the resolved immutable
image ID and preserves the `bundledock-<bundle>-<digest>` naming convention.

The OCI build uses the repository root as context with `bundles/toolbox/Dockerfile.dockerignore`
limiting it to the builder/initializer sources and artifact lock. Go compilation
and artifact downloads run on `BUILDPLATFORM`; the initializer is cross-compiled
for `TARGETARCH`. Final manifests and checksums include the extracted executables
and notices; build dependencies are excluded from the final tool tree.
The image needs no Python or shell initializer.

jq, ripgrep, fd, yq, uv, Fish, Starship, Nushell, tmux, typos, lychee,
Gitleaks, Trivy, and Syft use upstream static binaries.
Only Nushell's `nu` executable is installed; its optional plugins are omitted.
tmux requires the consumer's terminfo database, such as Debian's `ncurses-base`;
keep `TERM` appropriate for the terminal. Runtime compatibility with Alpine or
arbitrary consumer images is not established.
Fish uses the official standalone archive with embedded functions and completions;
its [tagged source](https://github.com/fish-shell/fish-shell/blob/4.9.3/src/autoload.rs)
embeds `share`, so no external runtime data directory is required. External manual
pages and local HTML documentation from a full Fish installation are omitted. ast-grep uses
the compatible Debian consumer libraries. `fd` and `fdfind` are identical regular
files so the volume needs no symlinks. ast-grep's `sg` alias is excluded to preserve
consumer commands. Upstream notices are included under `/opt/bundledock/toolbox/licenses`.
uv packages no Python runtime or Python packages; keep its cache, environments,
and any intentionally installed runtimes outside the shared tool volume.
`bundles/toolbox/scripts/volume.sh` remains a small Docker-only host adapter so the devcontainer can
initialize before a Go toolchain or BundleDock binary is available on the host.
