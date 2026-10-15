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
`/opt/bdock/toolbox`, and new volumes default to `bundledock-toolbox-<digest>`.
Existing volumes remain reusable with their original images: pass the existing
name as the second argument to `sh bundles/toolbox/scripts/volume.sh <image> <volume-name>`.
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
a development version identifier, `bdock tools upgrade [tool]`, and
`bdock tools volume --image <image>`.
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
targets. Builds write `bin/bdock`; `make clean` removes the binary.

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

### Automatic GitHub Pages publication

The [Publish documentation workflow](../.github/workflows/pages.yml) builds and
deploys the Hugo site at `https://wstein.github.io/bundledock/` after successful
`CI` for the exact current `main` revision. This includes merged documentation
PRs. Only main push CI or manually dispatched main CI is eligible. PR and branch
CI, failed CI, and skipped required checks cannot authorize publication.

The Pages-specific mode of `scripts/approve-publication.sh` reads live GitHub API
evidence before both build and deployment. It verifies the official repository,
active `ci.yml` workflow, successful run and exact latest attempt, successful
required `check` job, and matching full source SHA. Jobs evidence is paginated;
missing, truncated or contradictory evidence stops publication. The controller
revision and current main must also equal the verified CI SHA. A main advance or
CI rerun during validation stops that attempt; successful CI for the newer main
revision starts a new publication. A second check before deployment prevents an
older queued publication from replacing a newer site. The concurrency queue
serializes Pages publications without cancelling an in-progress deployment.

The gate runs from the immutable controller revision before build effects, then
the build checks out the verified source SHA explicitly. The build uses `make
docs` and the checksum-verified official Hugo wrapper with the project subpath.
Generated files remain ignored in `public/`. Artifacts include the verified SHA
and publication run attempt in their names; deployment uses only that exact
artifact from the same workflow run. Deployment-only reruns cannot consume an
earlier attempt's artifact: choose **Re-run all jobs** while that SHA remains
current main and its CI evidence remains valid.

Manual publication remains available: obtain explicit approval for documentation
build and deployment at the full current main SHA, select **Run workflow** on
`main`, and enter that SHA as `approved_sha`. Both jobs recheck the exact SHA and
human sender/actor identity. Automation must not impersonate a human dispatch.
This manual path remains independent of the automatic CI authorization path.
Other publication callers retain their existing approval rules.

Repository administrators must configure
[Settings → Pages](https://github.com/wstein/bundledock/settings/pages) with
**GitHub Actions** as the publishing source. Existing `github-pages` environment
protection rules continue to apply; this workflow does not change settings or
bypass required reviewers. Only deployment receives `pages: write` and
`id-token: write`; source and Actions evidence access remain read-only. No separate
deployment secret is required. Treat the site as published only after deployment
succeeds. Source tests verify these contracts without rendering or deploying the
site; live deployment is verified separately after merge.

### Stable updater compatibility

Stable prebuilt versions permitted by `packages.txt` should pass source checks
without editing tests or licensing metadata for each release. Production-lock
tests check stable ranges, matching AMD64/ARM64 versions, official recipes/URLs,
checksum syntax and publisher provenance. Fixed parser examples and reviewed
license snapshots live under `internal/toolconfig/testdata/`; they do not assert
that a historical fixture is the currently installed release. The deliberately
authorized git-trim source recipe and separate documentation-tool pin retain
their exact identity checks.

`scripts/tests/test_rolling_locks.py` copies only source-test inputs into a temporary
workspace, advances every rolling prebuilt lock on both architectures, and runs
the complete configuration test package. It covers all nine bundles without
changing checked-in locks, recipes, desired inputs or notices. Checksums and URLs
in this simulation are synthetic; it performs no tool downloads, executions,
image builds or upstream verification. It also confirms that corrupt checksums,
disallowed versions, downgraded checksum provenance, wrong-architecture URLs and
changed license bytes remain failures. The test runs offline as part of
`make check`.

Leaf and GitHub CLI SOURCE notices describe the exact release from which their
retained full license was reviewed, including immutable source evidence and a
license-byte checksum. That historical snapshot is independent of the binary
version recorded in the lock and generated bundle manifest. Advancing a binary
pin must not relabel old license evidence as newly verified. The snapshot checks
protect retained bytes; they do not prove that every future upstream release has
unchanged licensing. Actual license changes require review and updated evidence.

Keep source/test fixes in their own PR. After human merge, refresh an existing
bot updater by merging current main into that branch. Its final diff must stay
limited to its own lock and generated inventory to preserve automatic publication
eligibility; do not add source/test commits to the updater branch.

### Bundled Hugo verification

The toolbox installs the official stable **standard** Hugo 0.167.0 prebuilt
release for Linux AMD64 and ARM64. Extended Hugo and source compilation are
excluded. The lock records publisher SHA256 checksums; extraction retains the
upstream Apache-2.0 `LICENSE` and release/checksum provenance under
`licenses/hugo`. At the [Hugo addition revision](https://github.com/wstein/bundledock/commit/c1054282c8a514eb4bc0ca2c6569cd84f0926ae6),
the other 22 tool pins were unchanged. This historical inventory differs from
the [current source catalog](toolbox.md#source-inventory).

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
The consumer smoke uses `/opt/bdock/toolbox/bin/hugo` directly, with
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
Linux AMD64 and ARM64. For `VERSION=0.1.0`, it creates
`bdock_0.1.0_linux_amd64.tar.gz`, `bdock_0.1.0_linux_arm64.tar.gz`, `LICENSE`,
`PROVENANCE`, and `SHA256SUMS`; each archive contains the executable `bdock`,
`LICENSE` and platform-specific `PROVENANCE`. Extract the archive for your
architecture and install `bdock` on your PATH. An existing destination
is refused. Verify the outputs with `cd dist/cli && sha256sum -c SHA256SUMS`.
The target does not create a Git tag or publish a release.

The version defaults to `dev`; choose an identifier for local verification or
an explicitly approved release. The linker sets the CLI's `--version` output.
Provenance records the actual Git revision, whether tracked or untracked source
changes exist, the compiler, version and build flags. A dirty checkout produces
local verification artifacts, not a clean release candidate.

Builds use the selected `GO` executable, `CGO_ENABLED=0`, `-trimpath`,
`GOAMD64=v1`, and `GOARM64=v8.0`, and clear `GOFLAGS` for consistent flags.
Use the pinned compiler in `.go-version`; the Go module's minimum is 1.25.
`make release-test` also requires explicit user approval: it builds both architectures twice, verifies checksums, the
bundled license and build settings, compares repeated outputs, checks the native Linux version when
supported, and tests rejection of existing destinations and invalid versions.
This validates local artifact preparation; native AMD64 execution, Docker image
checks and publication remain separate checks.

Checks target this repository's `cmd` and `internal` Go packages.
`make bundle-test` runs the focused CLI, catalog, extraction, volume, and Docker
adapter tests without building an OCI image.

## Repository layout

- `cmd/bdock`: Go CLI entry point.
- `cmd/tools-bundle`, `cmd/tools-init`, `cmd/tools-publish`: build, initialization, and publication adapters.
- `internal`: package catalog, upstream resolution, bundle extraction, volume validation, and Docker adapters.
- `bundles/toolbox/`: Dockerfile, smoke/volume adapters, licenses and bundle inputs.
- `bundles/toolbox/{packages.txt,packages.catalog.json,packages.lock.json}`: desired versions, trusted recipes, and verified pins.
- `.github/workflows/ci.yml`: pull request and push formatting, source tests and vet.
- `.github/workflows/publish-bundles.yml`: one bundle-selecting publication entrypoint.
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

The [shared bundle publication flow](publication.md) accepts the actual human
owner `wstein` merging an eligible bot updater PR as authorization for that
exact frozen lock. Source merges remain human operations; the review-triggered
merge controller is removed. Successful exact merged-main source CI and its
required GitHub Actions check precede publication. The publisher verifies owner
merge identity, bot branch and own-file scope, exact merged/updater tree equality,
current main and provenance before artifact compilers and promotion. First
publication and manual publication retain their human actor and full run-SHA
gates. Existing required checks and linear-history protection remain; these
workflows change no settings. Ordinary source, documentation and unrelated bot
PRs do not authorize builds. Do not use human credentials to impersonate manual
dispatch. Consumer activation requires separate explicit authorization.

### Verify the existing five published consumers

[Verify published consumers](../.github/workflows/published-consumers.yml) is a
manual-only native Linux AMD64/ARM64 check. After source review and merge, a
human dispatches it from current `main` with the explicitly approved full
`approved_sha`. It rejects a stale main revision and bot actors. It only pulls
existing immutable images; it does not compile, build, publish, tag, log in to
agents, or change Anchorage's configuration.

The fixed toolbox, Node LTS, Codex, Antigravity and Claude pins match Anchorage
revision `86fb5f98ecbd14132be1f27c9b6ff4fb79f4a472`. The stock Go tag
`mcr.microsoft.com/devcontainers/go:2-1.27-trixie` resolved on 2026-10-12 to OCI
index `sha256:8f1c83128e4bce9d745b1dc0f28f8c027e303d3886e2434966ff135296c41770`.
Registry manifest metadata identified Linux AMD64 manifest
`sha256:8f6d52c21037d464b8e31077264e4acedeb14c9d1759d22af90e036b7352cd3e`
and ARM64 manifest
`sha256:c4cd108dd4fe90d6e5b3a940f0fa9ae66e747c193944d8ef4f240c208904f52e`.
This verification pins that index without changing the consumer's tag.

The verifier initializes disposable volumes through `scripts/bundle-volume.sh`
after every pull succeeds. Both offline non-root consumer runs mount all five
bundles read-only at their sibling `/opt/bdock/<bundle>` paths. They share
a disposable writable HOME, npm cache/prefix and retained config markers;
agent commands must resolve from their own mounts ahead of Node and toolbox.
The checks execute versions, validate Codex helper resources and Node's actual
executable path, and run local npm/npx commands offline without packing an
artifact. Cleanup removes only volumes allocated by this verifier. Source-only
fake-Docker tests cover orchestration and failure paths; they do not establish
native runtime compatibility.

[Run 38294738647, attempt 1](https://github.com/wstein/bundledock/actions/runs/38294738647/attempts/1)
passed both native jobs on Linux AMD64 and ARM64 on 2026-10-12 from source
`27e5abc2f4649aec8abc75ac68c647319dc0f931`. This historical run verified the five recorded
image pins together against the pinned stock Go consumer at the previous
`/opt/bundledock/<bundle>` consumer roots. The current verifier's `/opt/bdock`
mounts have source-only fixture coverage until a separately approved native run. It does not verify
a reduced toolbox image, future locks, authenticated agent workflows or model
requests, and does not change consumer activation.

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
changed lock or consumer baseline, run `make bundle-build bundle-smoke` on each
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
[repository policy](../AGENTS.md), with the single reviewed
[git-trim v0.4.3 exception](toolbox.md#git-trim-source-exception). Wait for a patched
stable upstream release for other tools; do not rebuild them from source or patch
any tool binary or dependency.

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

Rechecked on 2026-10-14 using each upstream's latest stable release endpoint,
ten most recent release records, tagged `go.mod`, and publisher asset digests.
[Gitleaks 8.30.1](https://github.com/gitleaks/gitleaks/releases/tag/v8.30.1),
[Syft 1.54.1](https://github.com/anchore/syft/releases/tag/v1.54.1), and
[Trivy 0.75.0](https://github.com/aquasecurity/trivy/releases/tag/v0.75.0)
remain the latest official stable releases. Their tagged dependency manifests
still contain the affected versions listed above. All six official AMD64/ARM64
asset URLs and SHA256 digests match the current toolbox lock; no replacement
stable archives with patched dependencies were found. The lock remains unchanged
and these findings remain open. This recheck inspected upstream metadata and
source manifests; it did not repeat binary inspection, assess exploitability,
or build/publish an image.

Recheck when an official stable release ships updated dependencies, then resolve
and verify its official archives on both supported architectures before updating
the lock. A BundleDock compiler refresh alone cannot remediate these downloaded
binaries.

## Smoke coverage

[Run 38069722044](https://github.com/wstein/bundledock/actions/runs/38069722044)
verified the then-current twenty-two tools and the `fdfind` and `uvx` companions
on native AMD64 and ARM64 at `6d20c7b2a7b976e5940622cd53b14ca90ee4eb31`,
as a non-root user in two concurrent containers with networking disabled.
That inventory included all three agents. The later [published toolbox receipt](toolbox.md#published-bundle)
records a different source revision, which added Hugo and still includes the
agents; it predates fzf, bat and regctl. The [current source catalog](toolbox.md#source-inventory)
excludes the agents and has its own smoke checks. Historical results do not
verify a new source inventory or an unpublished reduced toolbox.

The source smoke checks cover
JSON and YAML processing, text search, file discovery, and Go syntax
search. The quality and security fixtures verify a typo correction, a local HTML anchor,
a redacted synthetic secret, embedded Dockerfile configuration findings, and a
synthetic package in an SBOM. They scan only their generated local fixtures.
Trivy configuration smoke uses embedded checks with check updates disabled and
does not download a vulnerability database. Trivy vulnerability scans require
separately managed databases and writable caches outside `/opt/bdock/toolbox`;
no database is baked into the bundle. Syft update checks are disabled in smoke.
Nushell processes structured records. A headless tmux server uses an
isolated socket and the consumer's `/bin/bash`, then removes its session.
Fish checks a pipeline, arithmetic, embedded functions and completions;
Starship renders a minimal prompt and initializes within Fish using only writable
consumer home state. uv uses a writable cache under each consumer's home; managed Python downloads
are disabled, and its cache command runs offline. The standalone Codex smoke
starts and stops its local daemon to verify the retained native package is usable.
The toolbox smoke now rejects agent executables and Codex resources. It checks repeat initialization,
read-only mounts, and rejection of changed, incomplete, or locked volumes.
Successful native execution verifies executable startup; authentication, model
requests and agent sandbox behavior require separate runtime checks. Herdr
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
