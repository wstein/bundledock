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

`tools.lock.json` pins vendor artifact URLs and checksums for Linux `amd64`
and `arm64`. The bundle contains Codex (including native helpers), Claude Code,
Antigravity, GitHub CLI, SQLite, jq, ripgrep, fd, ast-grep, Mike Farah yq, and uv
(with uvx). Builds verify every download. The Debian base image
is pinned by digest; Debian packages still come from its current package repositories.

| Tool | Pinned version |
| --- | --- |
| Codex | 0.162.1 |
| Claude Code | 2.1.296 |
| Antigravity | 1.3.2 |
| GitHub CLI | 2.102.0 |
| SQLite (`sqlite3`) | 3.54.0 |
| jq | 1.8.2 |
| ripgrep (`rg`) | 15.2.0 |
| fd (`fd`, `fdfind`) | 10.5.0 |
| ast-grep (`ast-grep`) | 0.45.3 |
| Mike Farah yq (`yq`) | 4.54.1 |
| uv (`uv`, `uvx`) | 0.13.0 |

Run upgrades from the repository root:

```sh
make run ARGS="tools upgrade"       # resolve all tools
make run ARGS="tools upgrade gh"    # resolve only GitHub CLI
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
built-in `sqlite-source` compiler kind. Recipes cannot execute shell commands.
Both upgrade and build validate this catalog; the Dockerfile owns the fixed SQLite
compile command. Review catalog edits as build inputs. A catalog change requires
an upgrade of all tools so preserved pins cannot acquire a different recipe silently.

Upgrades fetch fresh release metadata and select the numerically newest stable
version within each requirement. GitHub release history is paginated. Optional `GITHUB_TOKEN` authentication is
sent only to `api.github.com`; artifact and other provider requests never receive
it. HTTP errors, including API rate limits, abort with the request URL and status.
Claude and
Antigravity combine official current manifests with historical GitHub releases.
SQLite uses official release chronology dates and compact archive naming, including
historical releases and backports. The chosen version must supply both architectures;
missing assets, metadata errors, or checksum failures abort without selecting an
older version. All selected downloads are hashed and verified before the lock is
replaced atomically. A failed resolution leaves the previous lock bytes unchanged;
a targeted upgrade preserves every other tool pin. An all-tool upgrade can create
a missing lock or reconcile catalog additions and removals.

Lock verification provenance distinguishes publisher SHA256/SHA512 and SQLite
SHA3-256 checksums from `tls-download-sha256`: the latter pins bytes downloaded
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

The original four agent tools and complete Codex assets occupied about 864 MiB
uncompressed on ARM64; the added utilities increase that size. The volume is shared across workspaces on the same Docker host.
The initial local build and smoke tests were run on Linux ARM64; AMD64 downloads
are pinned but have not yet been built or runtime-verified. The Go port has been
verified with local HTTP/archive fixtures for both architectures; its Docker build
and runtime smoke test still need a Docker host. The additional utilities have
been exercised natively on ARM64, including SQLite source compilation, queries,
syntax search, and offline uv cache selection; their Docker smoke test and AMD64
execution remain unverified. Before adopting the new bundle,
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

The smoke test executes all eleven tools and the `fdfind` and `uvx` companions
as a non-root user in two concurrent containers with networking disabled. It checks
a SQLite query, JSON and YAML processing, text search, file discovery, and Go syntax
search. uv uses a writable cache under each consumer's home; managed Python downloads
are disabled, and its cache command runs offline. It also starts and stops Codex's
local daemon to verify its complete native package is usable. It checks repeat initialization,
read-only mounts, and rejection of changed, incomplete, or locked volumes. This verifies executable
startup, not authentication, model requests, or agent sandbox behavior.

The supported initial consumer baseline is the Debian Trixie Go devcontainer.
Other images must supply compatible system libraries, Git, and CA certificates;
mounting this volume does not install those dependencies. Override `TOOLS_IMAGE`
or `TOOLS_CONSUMER` when invoking Make to select another image or test consumer.

The OCI build uses the repository root as context with `oci/Dockerfile.dockerignore`
limiting it to the builder/initializer sources and artifact lock. Go compilation
and artifact downloads run on `BUILDPLATFORM`; the initializer is cross-compiled
for `TARGETARCH`. SQLite's checksum-pinned official amalgamation is extracted into
a separate build-only directory and compiled in a target-platform Debian stage,
without readline or other optional libraries. Its default extension loading and
thread safety remain enabled. Final manifest/checksums include the compiled CLI;
source files and build dependencies are excluded from the final tool tree.
Cross-platform image builds need target execution support for that compile stage.
The image needs no Python or shell initializer.

jq, ripgrep, fd, yq, and uv use upstream static binaries. SQLite and ast-grep use
the compatible Debian consumer libraries. `fd` and `fdfind` are identical regular
files so the volume needs no symlinks. ast-grep's `sg` alias is excluded to preserve
consumer commands. Upstream notices are included under `/opt/anchorage/licenses`.
uv packages no Python runtime or Python packages; keep its cache, environments,
and any intentionally installed runtimes outside the shared tool volume.
`oci/volume.sh` remains a small Docker-only host adapter so the devcontainer can
initialize before a Go toolchain or Anchorage binary is available on the host.

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
a development version identifier, `anchorage tools upgrade [tool]`, and
`anchorage tools volume --image <image>`.
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
- `internal/toolconfig`, `internal/toolmanager`: shared declarative inputs and upstream lock resolution.
- `cmd/tools-bundle`, `cmd/tools-init`: standard-library OCI build and initialization commands.
- `internal`: bundle downloads/extraction, volume validation, and host Docker adapter.
- `oci`: Dockerfile, artifact lock, Docker-only host adapter, and smoke test.
- `crewbook`: pinned CrewBook workflow submodule.

To clone with the workflow resources:

```sh
git clone --recurse-submodules <repository-url>
```

For an existing clone, run `git submodule update --init --recursive`.
