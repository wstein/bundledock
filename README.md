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

The local OCI bundle and volume initializer are implemented. Registry publication,
authenticated agent tasks, and integration into the Anchorage CLI are still planned.

## OCI tool bundle

Docker must be running. Build the bundle for the Docker host's architecture:

```sh
make tools-build
make tools-test
make tools-smoke
make tools-volume
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
are pinned but have not yet been built or runtime-verified.

`tools-volume` prints a volume name derived from the resolved image ID. It copies
the tools once and verifies their checksums on subsequent calls. Incomplete,
nonempty, or mismatched volumes are rejected. Upgrades create a new image and
volume; rollback selects the previous image. Volumes are local to each Docker host.

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

## Development

The repository includes a Go devcontainer configuration. Open it with an editor
that supports development containers, or use a local Go 1.24 or newer toolchain.

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

The CLI uses [Cobra](https://github.com/spf13/cobra) and currently provides help
and a development version identifier. Environment management commands are planned.

Make targets are thin wrappers around Go commands. Run `make help` for available
targets. Builds write `bin/anchorage`; `make clean` removes the binary.

Checks target `cmd` explicitly because the CrewBook submodule contains Go
evaluation fixtures that are not Anchorage packages.

## Repository layout

- `cmd/anchorage`: Go CLI entry point.
- `.devcontainer`: development container configuration.
- `oci`: tool bundle, artifact lock, volume initializer, and smoke test.
- `crewbook`: pinned CrewBook workflow submodule.

To clone with the workflow resources:

```sh
git clone --recurse-submodules <repository-url>
```

For an existing clone, run `git submodule update --init --recursive`.
