# BundleDock

![BundleDock — Build and ship OCI bundles.](assets/banner.svg)

[![CI](https://github.com/wstein/bundledock/actions/workflows/ci.yml/badge.svg)](https://github.com/wstein/bundledock/actions/workflows/ci.yml)
[![Go 1.24 or newer](https://img.shields.io/badge/Go-1.24%2B-00ADD8?logo=go&logoColor=white)](go.mod)
[![Linux amd64 and arm64](https://img.shields.io/badge/Linux-amd64%20%7C%20arm64-0b2545?logo=linux&logoColor=white)](tools.lock.json)
[![OCI toolbox](https://img.shields.io/badge/OCI-toolbox-5eead4?labelColor=0b2545)](oci/Dockerfile)
[![Pinned tool catalog](https://img.shields.io/badge/tools-pinned-5eead4?labelColor=0b2545)](tools.packages.json)

BundleDock builds an OCI toolbox of pinned development CLIs and their required
runtimes for shared, read-only use in Linux containers. The toolbox supplies
common utilities alongside stock language images, such as Go or Rust images,
which provide their own language toolchains.

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
make run ARGS="tools volume --image bundledock-toolbox:poc"
```

`tools.lock.json` pins vendor artifact URLs and checksums for Linux `amd64`
and `arm64`. The bundle contains twenty-two tools: Codex (including native helpers), Claude Code,
Antigravity, GitHub CLI, jq, ripgrep, fd, ast-grep, Mike Farah yq, uv
(with uvx), and [Herdr](https://github.com/herdrdev/herdr) for terminal workspaces
and agent sessions, plus the Fish shell and Starship prompt, with Nushell and tmux available as alternatives, and typos, lychee, Gitleaks, Trivy, and Syft for quality and security checks, plus bottom (`btm`) and btop for process and system monitoring. Builds verify every
download. The Debian base image is pinned by digest; Debian packages still come from its current package repositories.

| Tool | Pinned version |
| --- | --- |
| Codex | 0.162.1 |
| Claude Code | 2.1.296 |
| Antigravity | 1.3.2 |
| GitHub CLI | 2.102.0 |
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
| typos (`typos`) | 1.51.1 |
| lychee (`lychee`) | 0.24.2 |
| Gitleaks (`gitleaks`) | 8.30.1 |
| Trivy (`trivy`) | 0.75.0 |
| Syft (`syft`) | 1.54.1 |
| bottom (`btm`) | 0.14.9 |
| btop (`btop`) | 1.4.7 |

bottom and btop use official static musl release binaries on both architectures;
no extra dynamic runtime packages or language toolchains are needed. btop's
published binaries report `GPU_SUPPORT=false`. The bundle installs the executables
and upstream notices; optional themes and shell completions are not installed.
Process visibility follows the consumer container's permissions.

| Added artifact | AMD64 download / executable bytes | ARM64 download / executable bytes |
| --- | ---: | ---: |
| bottom 0.14.9 | 2,126,967 / 5,231,120 | 1,918,659 / 4,276,136 |
| btop 1.4.7 | 1,047,005 / 2,386,088 | 983,319 / 2,184,904 |

These measured sizes exclude installed license notices and filesystem overhead.
All four archives passed checksum verification and the production extractor.
ARM64 version/help checks and btop's noninteractive `--default-config` passed
with isolated writable HOME. The full native CI smoke also passed on AMD64 and
ARM64 (see the CI evidence below); interactive TUI behavior remains unverified. Repeat the optional native acceptance checks with:

```sh
BUNDLEDOCK_PROCESS_MONITORS_ARCHIVE_DIR=/path/to/archives \
BUNDLEDOCK_PROCESS_MONITORS_SMOKE_PATH=/path/to/bin \
go test ./internal/toolbundle -run 'TestProcessMonitors' -count=1
```

The OCI smoke runs the same bounded version/help/default-config checks without a
TTY in each isolated consumer. Upstream and discovered vendor notices are retained;
this is not a complete audit of all compiled transitive dependency licenses.

Nushell and tmux are opt-in commands. tmux uses the latest published stable
[official static build](https://github.com/tmux/tmux-builds/releases/tag/v3.7c),
3.7c, while the upstream source release is 3.8. Its provider resolves that static
build feed; release suffixes compare numerically (`3.7a` = `3.7.1`, `3.7c` =
`3.7.3`, `3.8` = `3.8.0`) while lock versions and asset URLs retain the upstream tag.
No plugins or user configuration are installed for either alternative.

Consumer devcontainer setup, persistent shell configuration, and image/volume
pins live in the separate Anchorage umbrella repository. This repository owns
the builder and the tools it packages.

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
are retained in `oci/licenses/xz`.

Upgrades fetch fresh release metadata and select the numerically newest stable
version within each requirement. GitHub release history is paginated. Optional `GITHUB_TOKEN` authentication is
sent only to `api.github.com`; artifact and other provider requests never receive
it. HTTP errors, including API rate limits, abort with the request URL and status.
Claude and
Antigravity combine official current manifests with historical GitHub releases.
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
formatting and comments in `tools.txt` do not change its identity. Builds reject stale
identity, incomplete platforms, unsupported recipes, and unsafe archive members.

`tools upgrade` writes only `tools.lock.json`. Review and commit the diff, then run
`make tools-build tools-smoke tools-volume` to build and initialize the new bundle.
Existing images, volumes, running containers, and desired ranges are unaffected by
resolution. `tools volume` remains the existing initialization command; no update
alias or other tool-management commands are added.

### Continuous integration

[The CI workflow](.github/workflows/ci.yml) checks pushes, pull requests, and
manual runs. It runs formatting checks, the full Go test suite, `go vet`, and a
CLI build, then builds the checked-in toolbox lock on native AMD64 and ARM64
runners. Each native job explicitly pulls the consumer baseline and runs the
full existing smoke suite against the immutable ID of its one build. CI neither
upgrades tools nor publishes images and has no registry credentials.

The badge links to the standalone repository's live workflow status.
[The first CI run](https://github.com/wstein/bundledock/actions/runs/38059364460)
passed Go checks and the full native Docker build/smoke suite on AMD64 and ARM64
for commit `d659b81403b6b19caf06e35cb1f57ea5dc2e67bf`. Later documentation-only
commits preserve that tested code and tool lock; their own run status appears in
Actions. Interactive TUI use and authenticated agent workflows remain unverified.

### Weekly image publication

[The publication workflow](.github/workflows/bundledock-toolbox.yml) runs Mondays
at 03:17 UTC and through Actions → **Publish bundledock-toolbox** → **Run workflow**.
Only the repository default branch can publish. GitHub may delay scheduled runs.
Publication runs share one concurrency group; an active run is allowed to finish.
Before promotion, a read-only GitHub API check rejects a run superseded by a newer
eligible run, including one that failed. This prevents an old queued run or rerun
from rolling stable back. Start a fresh manual run to recover; when rerunning a
run, choose **Re-run all jobs** so the resolver creates current-attempt artifacts.

Set up the destination before the first publication:

1. Sign in to [Quay.io](https://quay.io/) and choose the user or organization
   namespace that will own the image. Create a repository named
   `bundledock-toolbox` in that namespace. Choose public visibility for anonymous
   pulls or private visibility when consumers will authenticate; this choice is
   independent of the GitHub source repository's visibility.
2. Create a dedicated robot account in the namespace's **Robot Accounts** tab.
   In the target repository's permissions, grant that robot **Write** access to
   `bundledock-toolbox` only. It does not need namespace administration or access
   to unrelated repositories. Its username has the form `namespace+robotname`.
3. In [BundleDock's Actions settings](https://github.com/wstein/bundledock/settings/secrets/actions),
   add the repository variable and secrets below. Enter the robot token directly
   as the secret value; keep it out of Git, logs, issues, and command arguments.


| Setting | Value |
| --- | --- |
| Repository variable `QUAY_IMAGE` | `quay.io/<namespace>/bundledock-toolbox` |
| Secret `QUAY_USERNAME` | Quay robot account username |
| Secret `QUAY_PASSWORD` | Robot token with write access to that repository |

After setting these values, start **Publish bundledock-toolbox** manually from
the default branch and watch the resolver, both native jobs, and promotion in
[Actions](https://github.com/wstein/bundledock/actions/workflows/bundledock-toolbox.yml).
A successful run publishes a dated run-attempt tag and `stable`; inspect its
summary and artifacts for the immutable digest before choosing a deployment.
Configuring these values also allows the Monday schedule to publish subsequent
updates. No registry publication has been verified until a configured run succeeds.
For a private Quay repository, consumers need their own read credentials.

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
image ID. Use the reported `quay.io/<namespace>/bundledock-toolbox@sha256:...` reference
for explicit deployment or rollback. Publication does not change the devcontainer
pin, initialize host volumes, or activate the new image.

Local verification of the publication adapter uses fake Docker commands in
`make check`; the implementation environment had no Docker. GitHub CI has since
passed real builds and native smoke on both architectures as linked above.
Quay publication still requires the first successful configured publication run.

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

Both complete official archives passed production checksum verification,
extraction and volume initialization without voice. Native ARM64 also passed
`codex --version` and daemon start/version/stop. To repeat these checks, place
both locked archives in one directory and run:

```sh
BUNDLEDOCK_CODEX_ARCHIVE_DIR=/path/to/archives \
BUNDLEDOCK_CODEX_NATIVE_SMOKE=1 \
go test ./internal/toolvolume -run TestCodexOfficialArchivesWithoutVoice -count=1 -v
```

The runtime commands execute only for the host's matching Linux architecture.
The initial local build and smoke tests were run on Linux ARM64. The Go port
also passed local HTTP/archive fixtures for both architectures. The current
locked toolbox has now passed real native Docker build and smoke on AMD64 and
ARM64 in the linked CI run, including syntax search and offline uv cache selection.
Before adopting a changed lock or consumer baseline, run `make tools-build
tools-smoke` on each supported architecture.

For the five quality and security tools, all ten official AMD64/ARM64 archives
were checked against publisher digests and passed the production extractor.
The shared offline fixture script initially passed natively on ARM64 without
network isolation. The linked full CI smoke subsequently passed on AMD64 and
ARM64 with consumer networking disabled. To repeat release acceptance checks with downloaded official archives
and a PATH containing the five executables:

```sh
BUNDLEDOCK_ARCHIVE_DIR=/path/to/archives \
BUNDLEDOCK_SMOKE_PATH=/path/to/bin \
go test ./internal/toolbundle -run 'TestQualitySecurity' -count=1
```


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

The smoke test executes all twenty-two tools and the `fdfind` and `uvx` companions
as a non-root user in two concurrent containers with networking disabled. It checks
JSON and YAML processing, text search, file discovery, and Go syntax
search. The quality and security fixtures verify a typo correction, a local HTML anchor,
a redacted synthetic secret, embedded Dockerfile configuration findings, and a
synthetic package in an SBOM. They scan only their generated local fixtures.
Trivy configuration smoke uses embedded checks with check updates disabled and
does not download a vulnerability database. Trivy vulnerability scans require
separately managed databases and writable caches outside `/opt/workharbor`;
no database is baked into the bundle. Syft update checks are disabled in smoke.
Nushell processes structured records. A headless tmux server uses an
isolated socket and the consumer's `/bin/bash`, then removes its session.
Fish checks a pipeline, arithmetic, embedded functions and completions;
Starship renders a minimal prompt and initializes within Fish using only writable
consumer home state. uv uses a writable cache under each consumer's home; managed Python downloads
are disabled, and its cache command runs offline. It also starts and stops Codex's
local daemon to verify its retained native package is usable. It checks repeat initialization,
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
consumer commands. Upstream notices are included under `/opt/workharbor/licenses`.
uv packages no Python runtime or Python packages; keep its cache, environments,
and any intentionally installed runtimes outside the shared tool volume.
`oci/volume.sh` remains a small Docker-only host adapter so the devcontainer can
initialize before a Go toolchain or BundleDock binary is available on the host.

## Development

Use a local Go 1.24 or newer toolchain. The Anchorage umbrella provides an
optional consumer devcontainer; its integration and pinned image are maintained
there. Make inherits `GOFLAGS`; OCI build commands pass `-trimpath` explicitly.

The output image remains `bundledock-toolbox`, its installed prefix remains
`/opt/workharbor`, and the initializer retains `anchorage-tools-<digest>` volume
names so existing image-derived volumes remain reusable. Writable credentials,
configuration, caches, and environments belong outside the bundle.

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

Checks target this repository's `cmd` and `internal` Go packages.

## Repository layout

- `cmd/bundledock`: Go CLI entry point.
- `cmd/tools-bundle`, `cmd/tools-init`, `cmd/tools-publish`: build, initialization, and publication adapters.
- `internal`: package catalog, upstream resolution, bundle extraction, volume validation, and Docker adapters.
- `tools.txt`, `tools.packages.json`, `tools.lock.json`: desired versions, trusted recipes, and verified pins.
- `oci`: Dockerfile, Docker-only host adapter, smoke test, and upstream license notices.
- `.github/workflows/ci.yml`: pull request and push checks plus native locked-toolbox smoke.
- `.github/workflows/bundledock-toolbox.yml`: weekly two-platform publication workflow.
- `assets`, `brand`: approved brand assets, licensed font sources, and generation provenance.

## Repository history

BundleDock was extracted from Anchorage by a full local clone followed by
ownership cleanup commits. All original shared commits and their SHAs remain
in its ancestry; history was not filtered or squashed. The original shared
history therefore includes former umbrella paths and submodule references.

The standalone repository is [github.com/wstein/bundledock](https://github.com/wstein/bundledock).
GitHub source publication is separate from Quay image publication and consumer
activation.
