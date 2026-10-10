# BundleDock

BundleDock is an early-stage OCI bundle builder. Its current tools profile packages
pinned development CLIs and their required runtimes into a shared, read-only
bundle for Linux containers. Additional bundle profiles are planned.

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

The local OCI tools bundle, version resolver, volume initializer, and registry
publication workflow are implemented. Authenticated agent tasks remain unverified.
The BundleDock CLI can initialize and verify the shared tool volume.

## OCI tool bundle

Docker must be running. Build the bundle for the Docker host's architecture:

```sh
make tools-build
make tools-test
make tools-smoke
make tools-volume
# Or, with a local Go toolchain:
make run ARGS="tools volume --image workharbor-tools:poc"
```

`tools.lock.json` pins vendor artifact URLs and checksums for Linux `amd64`
and `arm64`. The bundle contains Codex (including native helpers), Claude Code,
Antigravity, GitHub CLI, SQLite, jq, ripgrep, fd, ast-grep, Mike Farah yq, uv
(with uvx), and [Herdr](https://github.com/herdrdev/herdr) for terminal workspaces
and agent sessions, plus the Fish shell and Starship prompt, with Nushell and tmux available as alternatives. Builds verify every
download. The Debian base image is pinned by digest; Debian packages still come from its current package repositories.

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
| Herdr (`herdr`) | 0.9.3 |
| Fish (`fish`) | 4.9.3 |
| Starship (`starship`) | 1.26.0 |
| Nushell (`nu`) | 0.116.1 |
| tmux (`tmux`) | 3.7c (official static builds) |

Nushell and tmux are opt-in commands. tmux uses the latest published stable
[official static build](https://github.com/tmux/tmux-builds/releases/tag/v3.7c),
3.7c, while the upstream source release is 3.8. Its provider resolves that static
build feed; release suffixes compare numerically (`3.7a` = `3.7.1`, `3.7c` =
`3.7.3`, `3.8` = `3.8.0`) while lock versions and asset URLs retain the upstream tag.
No plugins or user configuration are installed for either alternative.

The devcontainer uses Fish for interactive VS Code terminals and offers separate
Herdr and Bash profiles. Select Herdr from the terminal profile menu or run
`herdr` in the workspace directory. In interactive Fish, `herd` runs
`exec herdr`, forwarding arguments and replacing the current shell. New Herdr configs select Fish for interactive
panes; existing user settings take precedence. Starship uses its default prompt.
No custom Starship theme or plugin manager is installed.

On container creation, `.devcontainer/setup.sh` seeds a guarded Starship snippet
at `~/.config/fish/conf.d/anchorage-starship.fish` and a minimal Herdr config at
`~/.config/herdr/config.toml`. Existing files are preserved, including changes
made after the first setup. `XDG_CONFIG_HOME` is respected. Run
`sh .devcontainer/setup.sh` inside the container to apply setup to an existing home.

The current Debian devcontainer keeps `SHELL=/bin/bash` and explicitly selects
Bash for VS Code automation. Fish is limited to interactive use; the login shell
is unchanged. An Alpine variant should select `/bin/ash` for agent and automation
shells. Alpine runtime compatibility is not established by this Debian setup.

Configuration, history, caches, and Herdr session state live in the writable home
volume. Sessions can survive client detachment while the container runs; running
processes do not survive container recreation. To customize the prompt, create
`~/.config/starship.toml`; `STARSHIP_CONFIG` and `STARSHIP_CACHE` can select other
writable locations. Fonts are configured in the client terminal.

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

The native builder supports `tar.xz` with the checksum-pinned pure Go
`github.com/ulikunitz/xz` decoder; no external `xz` command is needed. XZ inputs
are limited to 64 MiB compressed, 256 MiB decoded, 4096 tar members, and 128 MiB
per member. The decoder validates stream integrity through EOF. XZ headers can
request dictionary memory beyond these limits, so the production installer
verifies the exact trusted upstream checksum before decoding; these limits do
not make the decoder suitable for arbitrary untrusted archives. Decoder notices
are retained in `oci/licenses/xz`.

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

### Weekly image publication

[The publication workflow](.github/workflows/workharbor-tools.yml) runs Mondays
at 03:17 UTC and through Actions → **Publish workharbor-tools** → **Run workflow**.
Only the repository default branch can publish. GitHub may delay scheduled runs.
Publication runs share one concurrency group; an active run is allowed to finish.
Before promotion, a read-only GitHub API check rejects a run superseded by a newer
eligible run, including one that failed. This prevents an old queued run or rerun
from rolling stable back. Start a fresh manual run to recover; when rerunning a
run, choose **Re-run all jobs** so the resolver creates current-attempt artifacts.

Before enabling it, create the Quay repository and set these GitHub Actions values:

| Setting | Value |
| --- | --- |
| Repository variable `QUAY_IMAGE` | `quay.io/<namespace>/workharbor-tools` |
| Secret `QUAY_USERNAME` | Quay robot account username |
| Secret `QUAY_PASSWORD` | Robot token with write access to that repository |

The Quay namespace is deployment configuration, with no default in the workflow.
Use a dedicated [Quay robot account](https://docs.quay.io/glossary/robot-accounts.html).
The workflow uses read-only contents permission and read-only Actions permission
in the promotion job for the newest-run check. It never runs for pull
requests, and supplies registry credentials only after each native smoke passes.
Check [GitHub runner availability and plan limits](https://docs.github.com/en/actions/how-tos/write-workflows/choose-where-workflows-run/choose-the-runner-for-a-job)
for `ubuntu-24.04` (AMD64) and `ubuntu-24.04-arm` (ARM64).

One resolver runs `bundledock tools upgrade` for all tools, then uploads the exact
lock bytes and shared source commit, UTC build time, lock SHA256 and run tag.
It never commits or pushes the resulting lock. Both native jobs consume that
artifact, build `oci/Dockerfile` once, explicitly pull the consumer baseline and
run `oci/smoke.sh` against the resulting local image ID. OCI labels include the
image title, source, revision, build time, run tag, lock hash and tool versions.

A native job pushes its exact tested image to `staging-<run-tag>-<architecture>`.
If either architecture fails, final promotion is skipped. Once both pass, the
adapter verifies each remote registry manifest references its smoked image config,
creates the two-platform manifest and verifies its platform digests before updating
`stable`. The version tag is `YYYY.MM.DD-<github.run_id>-<github.run_attempt>`, so
reruns use distinct tags. These names are unique per attempt; registry-enforced
immutability requires separate Quay configuration. The lock hash is provenance,
not a build identifier: base layers or Debian packages can change for the same lock.

Download `resolved-tools-<attempt>`, `published-amd64-<attempt>`,
`published-arm64-<attempt>` and `publication-<attempt>` artifacts from the run (retained for 90 days). The run summary and `publication.json`
report the remote multi-platform registry digest, which differs from a local Docker
image ID. Use the reported `quay.io/<namespace>/workharbor-tools@sha256:...` reference
for explicit deployment or rollback. Publication does not change the devcontainer
pin, initialize host volumes, or activate the new image.

Local verification of the CI adapter uses fake Docker commands in `make check`.
Real builds, native smoke execution and Quay publication still require the first
successful configured workflow run; the implementation environment had no Docker.

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
  --mount type=volume,src=<printed-volume-name>,dst=/opt/workharbor,readonly,volume-nocopy \
  --env DISABLE_UPDATES=1 --env DISABLE_AUTOUPDATER=1 \
  mcr.microsoft.com/devcontainers/go:2-1.27-trixie \
  sh -c 'export PATH="/opt/workharbor/bin:$PATH"; exec bash'
```

Use `/opt/workharbor` as the consumer mount path and put its `bin` directory on
PATH. Bundle metadata and packaged assets use relative paths, so an existing
image-derived volume can be mounted there without changing its image or volume
identity. The initializer still mounts the volume at `/bundle` while copying or
verifying it. New images read their source tree from `/opt/workharbor`; the
initializer accepts `--source` and `--target` overrides for other locations.
Keep authentication and writable tool state in the user's home directory or
separate private volumes. No credentials are included in the image. The read-only mount prevents tools from updating the
shared executables; Claude updates are also disabled with environment variables.

The smoke test executes all sixteen tools and the `fdfind` and `uvx` companions
as a non-root user in two concurrent containers with networking disabled. It checks
a SQLite query, JSON and YAML processing, text search, file discovery, and Go syntax
search. Fish checks a pipeline, arithmetic, embedded functions and completions;
Starship renders a minimal prompt and initializes within Fish using only writable
consumer home state. uv uses a writable cache under each consumer's home; managed Python downloads
are disabled, and its cache command runs offline. It also starts and stops Codex's
local daemon to verify its complete native package is usable. It checks repeat initialization,
read-only mounts, and rejection of changed, incomplete, or locked volumes. This verifies executable
startup, not authentication, model requests, or agent sandbox behavior. Herdr
only runs `--version`; its interactive UI, server, and agent integration need
separate runtime testing.

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

jq, ripgrep, fd, yq, uv, Fish, and Starship use upstream static binaries.
Fish uses the official standalone archive with embedded functions and completions;
its [tagged source](https://github.com/fish-shell/fish-shell/blob/4.9.3/src/autoload.rs)
embeds `share`, so no external runtime data directory is required. External manual
pages and local HTML documentation from a full Fish installation are omitted. SQLite and ast-grep use
the compatible Debian consumer libraries. `fd` and `fdfind` are identical regular
files so the volume needs no symlinks. ast-grep's `sg` alias is excluded to preserve
consumer commands. Upstream notices are included under `/opt/workharbor/licenses`.
uv packages no Python runtime or Python packages; keep its cache, environments,
and any intentionally installed runtimes outside the shared tool volume.
`oci/volume.sh` remains a small Docker-only host adapter so the devcontainer can
initialize before a Go toolchain or BundleDock binary is available on the host.

## Development

The repository includes a Go devcontainer configuration. Open it with an editor
that supports development containers, or use a local Go 1.24 or newer toolchain.
The devcontainer sets `GOFLAGS=-trimpath` so Go builds omit local filesystem paths.
Make inherits `GOFLAGS`; OCI build commands also pass `-trimpath` explicitly.

The devcontainer pins the current local tool image by ID and initializes its
read-only volume on the host before starting. This image must already exist in
the host's Docker image store. Rebuild the devcontainer to activate the mount.
The current bundle and its consumer mount both use `/opt/workharbor`. The pinned
inventory includes Herdr and the common CLI utilities listed above. Updating the
configuration requires a container rebuild; reopening a terminal or reusing the
old container preserves its existing volume mount.

When selecting another bundle, update both the image ID in `initializeCommand`
and the matching volume name in `.devcontainer/devcontainer.json`. Credentials
remain in the container user's home directory; the shared tool volume stores
executables only. A separate writable volume, `anchorage-home-${devcontainerId}`,
persists `/home/vscode` across rebuilds and keeps each devcontainer's user state
separate. Docker seeds a new home volume from the image's existing home directory.

The legacy `anchorage-tools-<digest>` volume prefix, `anchorage-home-` home volume,
and `anchorage-starship.fish` / `anchorage-herdr.fish` configuration filenames are
retained intentionally so the rename reuses existing tool volumes, user state,
and customized shell configuration. The output image remains `workharbor-tools`
and its installed prefix remains `/opt/workharbor`.

```sh
make run
make run ARGS="--help"
make run ARGS="--version"
make check
make build
```

The CLI uses [Cobra](https://github.com/spf13/cobra) and currently provides help,
a development version identifier, `bundledock tools upgrade [tool]`, and
`bundledock tools volume --image <image>`.
The volume command uses the host Docker CLI, resolves the image once to its immutable
ID, and runs the initializer with networking disabled.

Make targets are thin wrappers around Go commands. Run `make help` for available
targets. Builds write `bin/bundledock`; `make clean` removes the binary.

Checks target `cmd` and `internal` explicitly because the CrewBook submodule
contains Go evaluation fixtures that are not BundleDock packages.

## Repository layout

- `cmd/bundledock`: Go CLI entry point.
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
