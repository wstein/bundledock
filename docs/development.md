---
title: Development and verification
description: Build the CLI, run checks, and review the recorded verification evidence and limits.
weight: 50
---

# Development and verification

[Back to BundleDock](../README.md)

## Development

Use the patched build toolchain in [`.go-version`](../.go-version) for local
builds and checks of BundleDock's own programs. Refreshing that compiler does
not update dependencies or Go runtime code embedded in downloaded third-party
binaries. The module requires Go 1.25 or newer for descriptor-confined symbolic link
operations in the optional code-server bundle profile. The Anchorage umbrella provides an
optional consumer devcontainer; its integration and pinned image are maintained
there. Make inherits `GOFLAGS`; OCI build commands pass `-trimpath` explicitly.

The local output image is `bundledock-toolbox`, its installed prefix is
`/opt/bundledock/toolbox`, and new volumes default to `bundledock-toolbox-<digest>`.
Existing volumes remain reusable with their original images: pass the existing
name as the second argument to `sh bundles/toolbox/volume.sh <image> <volume-name>`.
This selects the existing data without copying or renaming the volume. Anchorage
selects its pinned legacy volume explicitly. Writable credentials, configuration,
caches, and environments belong outside the bundle.

```sh
make run
make run ARGS="--help"
make run ARGS="--version"
make run ARGS="tools volume --image bundledock-toolbox:poc"
make check
```

The CLI uses [Cobra](https://github.com/spf13/cobra) and currently provides help,
a development version identifier, `bundledock tools upgrade [tool]`, and
`bundledock tools volume --image <image>`.
The volume command uses the host Docker CLI, resolves the image once to its immutable
ID, and runs the initializer with networking disabled.

Routine source tests, formatting checks and vet may run automatically, including
their compiler verification. Obtain the user's explicit approval for the intended
operation and source revision before any artifact, image, release, or
documentation build. This includes `make build`, `make release`, `make release-test`,
Docker build/smoke, all documentation targets below, and optional tests that
render a site. A push, merge, schedule, bot update or local commit is not approval.
Report builds deferred for approval separately from passing source checks.

Make targets are thin wrappers around Go commands. Run `make help` for available
targets. Builds write `bin/bundledock`; `make clean` removes the binary.

### Local documentation site

After explicit user approval, run `make docs` to build the static documentation into ignored `public/`, or
`make docs-serve` to preview it at `http://localhost:1313/`. `make docs-test`
builds the site and checks guide content, local links and fragments, project
subpath URLs, and unchanged brand assets. Generic Go test runs skip these host
integration checks; `make docs-test` explicitly enables them with
`BUNDLEDOCK_DOCS_TEST=1`. The README and these four guides are
mounted directly by Hugo; edit the original Markdown to update both views.

The docs wrapper downloads only the official stable prebuilt Hugo 0.167.0 for
Linux AMD64 or ARM64, verifies the publisher SHA256 in `.config/hugo.sha256`
on every run, and extracts to a temporary directory removed when it exits.
It needs Bash, curl, tar, and sha256sum; the documentation tests use Go.
The toolbox also includes the official standard Hugo release. The docs wrapper
keeps its verified fallback independent of the host PATH, so a different installed
Hugo cannot silently change the documentation build.
No Node, third-party theme, source compilation, deployment, or credentials are
needed. The first build downloads its pinned archive; later builds use the
verified cache in `.cache/hugo` (override with `HUGO_CACHE_DIR`). Remove that
directory to clear downloaded archives while preserving the `public/` preview.
To review a project-subpath build, run
`bash scripts/hugo.sh --baseURL https://example.org/bundledock/`.

### Manual GitHub Pages publication

The [Publish documentation workflow](../.github/workflows/pages.yml) prepares
the existing Hugo site for `https://wstein.github.io/bundledock/`. It runs only
by manual dispatch from default branch `main`, with the full approved commit
SHA in `approved_sha`. Pushes, pull requests, merges and schedules do not build
or deploy documentation. Preparing or committing the workflow supplies no
approval to run it, enable Pages, or change repository settings.

Before the first deployment, the repository administrator must separately
configure [Settings → Pages](https://github.com/wstein/bundledock/settings/pages)
with **GitHub Actions** as the publishing source. Review the `github-pages`
environment's deployment branches and any required reviewers in repository
settings. This workflow names that environment but does not configure its
protection rules or claim that required reviewers are present. See GitHub's
[custom workflow requirements](https://docs.github.com/en/pages/getting-started-with-github-pages/using-custom-workflows-with-github-pages).

For each publication:

1. Review the exact `main` revision and obtain explicit user approval for both
   the documentation build and the Pages deployment at that full SHA.
2. Open [Publish documentation in Actions](https://github.com/wstein/bundledock/actions/workflows/pages.yml),
   choose **Run workflow** on `main`, and enter that SHA as `approved_sha`.
   If `main` has moved, review and obtain approval for the new revision first.
3. Inspect the build and deployment results and the environment URL. Treat the
   site as published only after the deployment succeeds. Obtain fresh explicit
   approval for reruns of the original SHA and choose **Re-run all jobs** so the
   build creates the artifact for the new run attempt. A deployment-only rerun
   cannot reuse an earlier attempt's artifact.

Both jobs check out the immutable run SHA and independently run the existing
publication approval gate before Hugo, artifact upload, or deployment. It
rejects non-user senders, bot actors and bot reruns, including partial reruns.
The build uses `make docs` and the checksum-verified official Hugo wrapper;
`HUGO_BASEURL` supplies the project subpath while the local preview remains
unchanged. Generated files remain ignored in `public/`. No Node installation,
third-party theme, or separate deployment secret is required. Only the deploy
job receives `pages: write` and `id-token: write`; source access stays read-only.

The actor checks cannot distinguish a person's manual dispatch from automation
using that person's credentials. Automation must never dispatch this workflow,
including with a human-owned token; restrict dispatch credentials and repository
writers separately. An optional reviewer gate on `github-pages` protects the
deployment job only, after the documentation build. The manual dispatch and
exact-SHA approval remain necessary before any build. Source tests check these
workflow contracts without rendering or publishing the site; they do not
establish that repository Pages settings or a live deployment work.

### Bundled Hugo verification

The toolbox installs the official stable **standard** Hugo 0.167.0 prebuilt
release for Linux AMD64 and ARM64. Extended Hugo and source compilation are
excluded. The lock records publisher SHA256 checksums; extraction retains the
upstream Apache-2.0 `LICENSE` and release/checksum provenance under
`licenses/hugo`. The other 22 tool pins are unchanged.

Measured official archive and installed executable sizes:

| Platform | Compressed archive bytes | Extracted `hugo` bytes |
| --- | ---: | ---: |
| AMD64 | 20,970,860 | 60,371,104 |
| ARM64 | 19,354,090 | 57,082,016 |

These sizes exclude installed notices and filesystem overhead. Both official
archives passed locked-hash verification and extraction through the OCI builder.
Native ARM64 version, minimal offline site generation, and the current BundleDock
documentation build passed using the extracted binary's explicit path.
Native AMD64 execution and Docker consumer smoke were unavailable in this ARM64
host session; both platforms have a native smoke matrix in the manual publication workflow,
which requires explicit user approval before it runs.
The consumer smoke uses `/opt/bundledock/toolbox/bin/hugo` directly, with
`--network none` and writable HOME/cache/output outside the read-only bundle.

To repeat the optional official archive check without upstream requests, place
the two locked archives in one directory and run:

```sh
BUNDLEDOCK_HUGO_ARCHIVE_DIR=/path/to/archives go test ./internal/toolbundle -run TestHugoOfficialArchives -v -count=1
```

This optional check requires explicit user approval because it verifies and
extracts both architectures and executes version, minimal
site, and current documentation builds on the native Linux architecture.

### Release review

Before artifact, image, documentation, or release build operations, obtain
explicit user approval of the operation and source revision. Before the next release, review every item in the local TODO list and its
supporting evidence. Resolve unfinished or blocked items, or obtain an explicit
user decision to defer each one, before creating a release tag or publishing.
Keep the local TODO list untracked.

### Local CLI artifacts

After explicit user approval, `make release VERSION=dev RELEASE_DIR=dist/cli` builds BundleDock's own CLI for
Linux AMD64 and ARM64. It creates `bundledock-linux-amd64`,
`bundledock-linux-arm64`, `LICENSE`, `PROVENANCE`, and `SHA256SUMS`; an existing destination
is refused. Verify the outputs with `cd dist/cli && sha256sum -c SHA256SUMS`.
The target does not create a Git tag or publish a release.

The version defaults to `dev`; choose an identifier for local verification or
an explicitly approved release. The linker sets the CLI's `--version` output.
Provenance records the actual Git revision, whether tracked or untracked source
changes exist, the compiler, version and build flags. A dirty checkout produces
local verification artifacts, not a clean release candidate.

Builds use the selected `GO` executable, `CGO_ENABLED=0`, `-trimpath`,
`GOAMD64=v1`, and `GOARM64=v8.0`, and clear `GOFLAGS` for consistent flags.
Use the pinned compiler in `.go-version`; the Go module's minimum remains 1.24.
`make release-test` also requires explicit user approval: it builds both architectures twice, verifies checksums, the
bundled license and build settings, compares repeated outputs, checks the native Linux version when
supported, and tests rejection of existing destinations and invalid versions.
This validates local artifact preparation; native AMD64 execution, Docker image
checks and publication remain separate checks.

Checks target this repository's `cmd` and `internal` Go packages.
`make tools-test` runs the focused CLI, catalog, extraction, volume, and Docker
adapter tests without building an OCI image.

## Repository layout

- `cmd/bundledock`: Go CLI entry point.
- `cmd/tools-bundle`, `cmd/tools-init`, `cmd/tools-publish`: build, initialization, and publication adapters.
- `internal`: package catalog, upstream resolution, bundle extraction, volume validation, and Docker adapters.
- `bundles/toolbox/`: Dockerfile, smoke/volume adapters, licenses and bundle inputs.
- `bundles/toolbox/{tools.txt,tools.packages.json,tools.lock.json}`: desired versions, trusted recipes, and verified pins.
- `.github/workflows/ci.yml`: pull request and push formatting, source tests and vet.
- `.github/workflows/bundledock-toolbox.yml`: user-approved manual two-platform build and publication.
- `assets`, `brand`: approved brand assets, licensed font sources, and generation provenance.

## Repository history

BundleDock was extracted from Anchorage by a full local clone followed by
ownership cleanup commits. All original shared commits and their SHAs remain
in its ancestry; history was not filtered or squashed. The original shared
history therefore includes former umbrella paths and submodule references.

The standalone repository is [github.com/wstein/bundledock](https://github.com/wstein/bundledock).
GitHub source publication is separate from Quay image publication and consumer
activation.

## Continuous integration

[The CI workflow](../.github/workflows/ci.yml) checks pushes, pull requests, and
manual runs with formatting checks, the full source test suite, and `go vet`.
Its test compilation verifies source without creating CLI release artifacts,
OCI images, or a documentation site. Automatic native image and CLI artifact
builds are disabled.

The [manual publication workflow](publication.md) requires a human user's
explicit approval for its exact default-`main` SHA before any build. It resolves
one shared lock, builds and smokes both native architectures, then applies the
existing registry gates. It has no schedule or merge trigger. Future lock-only
PR automation must not dispatch native builds. Human-token automation cannot be
identified from GitHub actor context alone; do not use such credentials for
automated dispatch. A protected-environment gate is not configured here.

## Verification evidence

The badge in the README shows the live workflow status; evidence below names
the revision that was actually tested.

[The first CI run](https://github.com/wstein/bundledock/actions/runs/38059364460)
passed Go checks and full native Docker build/smoke on AMD64 and ARM64 for
`d659b81403b6b19caf06e35cb1f57ea5dc2e67bf`. That revision used the earlier
carrier and included Codex voice resources. It is historical evidence, not
verification of the later scratch carrier or voice omission.

[The scratch and voice-free CI run](https://github.com/wstein/bundledock/actions/runs/38069722044)
passed formatting, Go tests, vet, CLI build, and full native Docker build/smoke
on AMD64 and ARM64 for `6d20c7b2a7b976e5940622cd53b14ca90ee4eb31`.
That run verifies the scratch carrier and retained Codex runtime without voice
resources against the unchanged Debian Trixie Go consumer.

Local Docker was unavailable in the implementation environment. Publication
adapter checks use fake Docker commands in `make check`; they do not establish
Quay publication. Interactive TUI use, authenticated agent workflows, model
requests, and agent sandbox behavior remain unverified. Before adopting a
changed lock or consumer baseline, run `make tools-build tools-smoke` on each
supported architecture.

### Process monitors

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
with isolated writable HOME. The historical native CI run above covered these monitor pins on AMD64 and
ARM64; interactive TUI behavior remains unverified. Repeat the optional native acceptance checks with:

```sh
BUNDLEDOCK_PROCESS_MONITORS_ARCHIVE_DIR=/path/to/archives \
BUNDLEDOCK_PROCESS_MONITORS_SMOKE_PATH=/path/to/bin \
go test ./internal/toolbundle -run 'TestProcessMonitors' -count=1
```

The OCI smoke runs the same bounded version/help/default-config checks without a
TTY in each isolated consumer. Upstream and discovered vendor notices are retained;
this is not a complete audit of all compiled transitive dependency licenses.

### Codex archive acceptance

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
The initial local build and smoke tests used Linux ARM64; the Go port also
passed local HTTP/archive fixtures for both architectures.

### Quality and security tools

For the five quality and security tools, all ten official AMD64/ARM64 archives
were checked against publisher digests and passed the production extractor.
The shared offline fixture script initially passed natively on ARM64 without
network isolation. The historical CI run above subsequently passed full smoke on AMD64 and
ARM64 with consumer networking disabled. To repeat release acceptance checks with downloaded official archives
and a PATH containing the five executables:

```sh
BUNDLEDOCK_ARCHIVE_DIR=/path/to/archives \
BUNDLEDOCK_SMOKE_PATH=/path/to/bin \
go test ./internal/toolbundle -run 'TestQualitySecurity' -count=1
```

### Embedded dependency findings

Third-party tools use official stable prebuilt releases under the
[repository policy](../AGENTS.md). Wait for a patched stable upstream release;
do not rebuild the tools from source or patch their binaries or dependencies.

The 2026-10-10 inspection of Go build information in both official AMD64 and
ARM64 archives confirmed the following dependencies in the locked releases:

| Tool | Embedded dependencies with reported findings |
| --- | --- |
| Gitleaks 8.30.1 | `golang.org/x/crypto` v0.35.0, `github.com/ulikunitz/xz` v0.5.12, `github.com/nwaples/rardecode/v2` v2.1.0 |
| Syft 1.54.1 | `golang.org/x/net` v0.59.0 |
| Trivy 0.75.0 | `golang.org/x/net` v0.59.0 |

The official latest stable releases checked on that date were still these
versions; no patched stable prebuilt release was available. Build information
confirms the embedded dependency versions, not an independent validation of
every reported vulnerability or its exploitability. These remain open findings;
archive checksum verification and smoke success do not resolve them.

Recheck when an official stable release ships updated dependencies, then resolve
and verify its official archives on both supported architectures before updating
the lock. A BundleDock compiler refresh alone cannot remediate these downloaded
binaries.

## Smoke coverage

The smoke test executes all twenty-two tools and the `fdfind` and `uvx` companions
as a non-root user in two concurrent containers with networking disabled. It checks
JSON and YAML processing, text search, file discovery, and Go syntax
search. The quality and security fixtures verify a typo correction, a local HTML anchor,
a redacted synthetic secret, embedded Dockerfile configuration findings, and a
synthetic package in an SBOM. They scan only their generated local fixtures.
Trivy configuration smoke uses embedded checks with check updates disabled and
does not download a vulnerability database. Trivy vulnerability scans require
separately managed databases and writable caches outside `/opt/bundledock/toolbox`;
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

## Helix source verification

Helix 25.07.1 official AMD64 and ARM64 release archives were downloaded read-only
and verified against GitHub's published SHA256 asset digests. Each complete tar
graph contains 1,523 entries (1,236 regular files and 287 directories), no links,
and respectively 212,082,486 / 212,500,574 regular-file bytes. Both fit the existing
XZ limits. Archive graph inspection does not demonstrate native runtime behavior.
The checksum-verified official source archive supplies retained compiled grammar
license notices; its sources are not built. MPL-2.0, grammar notices and provenance
are in `bundles/toolbox/licenses/helix`, and all runtime notices remain in the payload.

Source tests exercise synthetic dual-architecture release layouts, relocated
launcher runtime selection, writable HOME and quoted arguments, full grammar/query/
theme notice preservation, rejected foreign roots and links, calendar normalization
and updater asset spelling. The smoke script's parser/query failures are tested with
synthetic executables. The OCI smoke source now checks `hx --version`, writable
Helix configuration, and Go grammar and highlight-query loading offline.

No real Helix binary was executed or production archive extracted during this
source-only change. Native AMD64/ARM64 image builds, read-only volume smoke, terminal
editing and consumer compatibility remain deferred until explicit build approval
for the intended source revision. Source checks do not establish release readiness.
