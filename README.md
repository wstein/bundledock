# Anchorage

Anchorage is an early-stage project for a lightweight, AI-native development
environment built on Docker, inspired by GitHub Codespaces.

The first proof of concept will provide Codex, Claude Code, Google Antigravity
(`agy`), and GitHub CLI (`gh`) inside development containers through a shared
tool volume. GitHub CLI will support repository, issue, and pull request workflows.

## Proof of concept

- Package pinned CLI versions and their required runtimes as an OCI tool bundle.
- Initialize a versioned Docker volume from the bundle and mount it read-only
  into development containers.
- Keep credentials, configuration, caches, and session state separate from the
  shared executables.
- Support upgrades and rollback by selecting a different bundle version.

Start with a Debian/glibc container baseline and platform-specific Linux
artifacts. Validate actual CLI execution and authentication, concurrent use by
two containers, and upgrade and rollback behavior.

The local OCI bundle and volume initializer are implemented. Registry publication
and authenticated agent tasks are still planned. The Anchorage CLI can initialize
and verify the shared tool volume.

## OCI tool bundle

Docker must be running. Build the bundle for the Docker host's architecture:

```sh
make tools-build
make tools-test
make tools-smoke
make tools-volume
# Or, with a local Go toolchain:
make run ARGS="tools volume --image anchorage/tools:poc"
```

`oci/tools.lock.json` pins vendor artifact URLs and checksums for Linux `amd64`
and `arm64`. The bundle contains Codex (including native helpers), Claude Code,
Antigravity, and GitHub CLI. Builds verify every download. The Debian base image
is pinned by digest; Debian packages still come from its current package repositories.

| Tool | Pinned version |
| --- | --- |
| Codex | 0.162.1 |
| Claude Code | 2.1.296 |
| Antigravity | 1.3.2 |
| GitHub CLI | 2.102.0 |

Native binaries and complete Codex runtime assets occupy about 864 MiB uncompressed on
ARM64. The volume is shared across workspaces on the same Docker host.
The initial local build and smoke tests were run on Linux ARM64; AMD64 downloads
are pinned but have not yet been built or runtime-verified. The Go port has been
verified with local HTTP/archive fixtures for both architectures; its Docker build
and runtime smoke test still need a Docker host. Before adopting the new bundle,
run `make tools-build tools-smoke` on each supported architecture.

`tools-volume` prints a volume name derived from the resolved image ID. It copies
the tools once and verifies their checksums on subsequent calls. Incomplete,
nonempty, or mismatched volumes are rejected. Upgrades create a new image and
volume; rollback selects the previous image. Volumes are local to each Docker host.
The Go builder emits deterministic metadata, with its own JSON serialization; it
does not guarantee byte compatibility with legacy Python-built manifests. Build a
new image and use its image-derived volume instead of reusing a legacy volume.
Initialization never removes an existing lock or retries a partial nonempty volume.

Mount the printed volume into a compatible development container:

```sh
docker run --rm -it \
  --mount type=volume,src=<printed-volume-name>,dst=/opt/anchorage,readonly,volume-nocopy \
  --env DISABLE_UPDATES=1 --env DISABLE_AUTOUPDATER=1 \
  mcr.microsoft.com/devcontainers/go:2-1.27-trixie \
  sh -c 'export PATH="/opt/anchorage/bin:$PATH"; exec bash'
```

The mount path must be `/opt/anchorage`. Keep authentication and writable tool
state in the user's home directory or separate private volumes. No credentials
are included in the image. The read-only mount prevents tools from updating the
shared executables; Claude updates are also disabled with environment variables.

The smoke test executes all four version commands as a non-root user in two
concurrent containers with networking disabled. It also starts and stops Codex's
local daemon to verify its complete native package is usable. It checks repeat initialization,
read-only mounts, and rejection of changed, incomplete, or locked volumes. This verifies executable
startup, not authentication, model requests, or agent sandbox behavior.

The supported initial consumer baseline is the Debian Trixie Go devcontainer.
Other images must supply compatible system libraries, Git, and CA certificates;
mounting this volume does not install those dependencies. Override `TOOLS_IMAGE`
or `TOOLS_CONSUMER` when invoking Make to select another image or test consumer.

The OCI build uses the repository root as context with `oci/Dockerfile.dockerignore`
limiting it to the builder/initializer sources and artifact lock. Go compilation
and artifact downloads run on `BUILDPLATFORM`; only the initializer is cross-compiled
for `TARGETARCH`. The image needs no Python or shell initializer.
`oci/volume.sh` remains a small Docker-only host adapter so the devcontainer can
initialize before a Go toolchain or Anchorage binary is available on the host.

## Separate Python runtime bundle

Python has its own image, lock, and volume. Build and initialize it independently:

```sh
make python-build
make python-test
make python-smoke
make python-volume
```

`oci/python/python.lock.json` pins CPython **3.15.0**, including upstream pip
**26.2.1**, from the immutable
[python-build-standalone 20261009 release](https://github.com/astral-sh/python-build-standalone/releases/tag/20261009).
Both Linux `amd64` and `arm64` downloads are verified against the release's
[SHA256SUMS](https://github.com/astral-sh/python-build-standalone/releases/download/20261009/SHA256SUMS).
The complete `install_only` tree includes the standard library, native extension
modules, shared libraries, headers, licenses, and upstream packaging tools.
No project packages are installed. The four-agent bundle and devcontainer keep
their existing lock, image, volume, and mount configuration.

Mount the printed `anchorage-python-<image-id>` volume at `/opt/anchorage-python`:

```sh
docker run --rm -it \
  --mount type=volume,src=<printed-python-volume-name>,dst=/opt/anchorage-python,readonly,volume-nocopy \
  --env PYTHONDONTWRITEBYTECODE=1 \
  debian:trixie-slim \
  sh -c 'export PATH="/opt/anchorage-python/bin:$PATH"; exec bash'
```

Use `python3 -m pip` to select this interpreter explicitly. Create project
environments with `python3 -m venv /workspace/.venv`, then install dependencies
through `/workspace/.venv/bin/python -m pip`. Keep the workspace, home, caches,
temporary files, and virtual environments writable outside the shared runtime.
Do not install packages into the shared prefix. Consumers need compatible glibc
and system CA certificates for HTTPS; the intended container baseline is Debian
Trixie. Override `PYTHON_IMAGE` or `PYTHON_CONSUMER` to select another image.

The extractor strips only the required `python/` prefix and validates the entire
archive namespace before writing output. Bounded file symlinks and hardlinks
are materialized as regular files with normalized permissions; escaping,
cyclic, missing, directory, conflicting, and special entries are rejected.
Every final file is checksummed. Materializing aliases increases runtime size.
On ARM64, `bin/python` and `bin/python3` each duplicate the 76,681,128-byte
interpreter, and `lib/libpython3.15.so` duplicates a 77,151,080-byte library.
The complete developer libraries and assets are retained:

| Measured artifact | ARM64 | AMD64 |
| --- | ---: | ---: |
| Upstream compressed archive | 62,185,467 bytes | 81,201,491 bytes |
| Archive regular file contents | 191,631,610 bytes | 273,540,500 bytes |
| Bundle regular files including metadata | 423,774,142 bytes (404.1 MiB) | 593,874,894 bytes (566.4 MiB) |
| Local filesystem allocation (`du -s -B1`) | 436,310,016 bytes | 606,416,896 bytes |

Both real upstream archives were checksum-verified and assembled locally. Each
bundle contains 4,640 regular files and no symlinks. On Debian 13 ARM64 with
glibc 2.41, the relocated runtime imported `ssl`, `sqlite3`, `ctypes`, `venv`,
`zlib`, `bz2`, and `lzma`; `python3 -m pip` and the upstream `pip3` launcher ran.
Two concurrent non-root consumers created independent offline virtual
environments with pip while Unix permissions protected the shared prefix.
AMD64 execution and Docker image size, build, and read-only volume behavior
remain unverified because this verification host has no Docker executable.
Run `make python-build python-smoke` on a Docker host for each architecture
before adoption. The Docker smoke harness runs two non-root consumers with
networking disabled and a read-only volume; it checks offline venv/pip,
repeat initialization, and rejection of changed, incomplete, or locked volumes.

The Python Dockerfile uses its own context ignore file and the standard-library
`cmd/python-bundle` builder. The shared Go initializer accepts
`--source /opt/anchorage-python`; its default remains `/opt/anchorage`.
Registry publication and automatic Python devcontainer mounting are not included.

## Development

The repository includes a Go devcontainer configuration. Open it with an editor
that supports development containers, or use a local Go 1.24 or newer toolchain.
The devcontainer sets `GOFLAGS=-trimpath` so Go builds omit local filesystem paths.
Make inherits `GOFLAGS`; OCI build commands also pass `-trimpath` explicitly.

The devcontainer pins the current local tool image by ID and initializes its
read-only volume on the host before starting. This image must already exist in
the host's Docker image store. Rebuild the devcontainer to activate the mount.
When selecting another bundle, update both the image ID in `initializeCommand`
and the matching volume name in `.devcontainer/devcontainer.json`. Credentials
remain in the container user's home directory; the shared tool volume stores
executables only. A separate writable volume, `anchorage-home-${devcontainerId}`,
persists `/home/vscode` across rebuilds and keeps each devcontainer's user state
separate. Docker seeds a new home volume from the image's existing home directory.

```sh
make run
make run ARGS="--help"
make run ARGS="--version"
make check
make build
```

The CLI uses [Cobra](https://github.com/spf13/cobra) and currently provides help,
a development version identifier, and `anchorage tools volume --image <image>`.
The volume command uses the host Docker CLI, resolves the image once to its immutable
ID, and runs the initializer with networking disabled. Environment management
commands are planned.

Make targets are thin wrappers around Go commands. Run `make help` for available
targets. Builds write `bin/anchorage`; `make clean` removes the binary.

Checks target `cmd` and `internal` explicitly because the CrewBook submodule
contains Go evaluation fixtures that are not Anchorage packages.

## Repository layout

- `cmd/anchorage`: Go CLI entry point.
- `.devcontainer`: development container configuration.
- `cmd/tools-bundle`, `cmd/tools-init`: standard-library OCI build and initialization commands.
- `internal`: bundle downloads/extraction, volume validation, and host Docker adapter.
- `oci`: Dockerfile, artifact lock, Docker-only host adapter, and smoke test.
- `crewbook`: pinned CrewBook workflow submodule.

To clone with the workflow resources:

```sh
git clone --recurse-submodules <repository-url>
```

For an existing clone, run `git submodule update --init --recursive`.
