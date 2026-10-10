# Development and verification

[Back to BundleDock](../README.md)

## Development

Use the patched build toolchain in [`.go-version`](../.go-version) for local
builds and checks. The module retains Go 1.24 source compatibility. The Anchorage umbrella provides an
optional consumer devcontainer; its integration and pinned image are maintained
there. Make inherits `GOFLAGS`; OCI build commands pass `-trimpath` explicitly.

The local output image is `bundledock-toolbox`, its installed prefix is
`/opt/bundledock/toolbox`, and new volumes default to `bundledock-toolbox-<digest>`.
Existing volumes remain reusable with their original images: pass the existing
name as the second argument to `sh oci/volume.sh <image> <volume-name>`.
This selects the existing data without copying or renaming the volume. Anchorage
selects its pinned legacy volume explicitly. Writable credentials, configuration,
caches, and environments belong outside the bundle.

```sh
make run
make run ARGS="--help"
make run ARGS="--version"
make run ARGS="tools volume --image bundledock-toolbox:poc"
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
`make tools-test` runs the focused CLI, catalog, extraction, volume, and Docker
adapter tests without building an OCI image.

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

## Continuous integration

[The CI workflow](../.github/workflows/ci.yml) checks pushes, pull requests, and
manual runs. It runs formatting checks, the full Go test suite, `go vet`, and a
CLI build, then builds the checked-in toolbox lock on native AMD64 and ARM64
runners. Each native job explicitly pulls the consumer baseline and runs the
full existing smoke suite against the immutable ID of its one build. CI neither
upgrades tools nor publishes images and has no registry credentials.

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
