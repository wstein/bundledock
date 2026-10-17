# Spec: tag-driven `bdock` CLI release workflow

**Status**: specified and implemented on the branch `feat/bdock-release-workflow`; independent
review rounds are in [analysis.md](analysis.md). Nothing has been run on GitHub; no tag exists.

## Authorization

Recorded in `AGENTS.md`. On 2026-10-17 the owner, Werner Stein, asked in chat:

> what about https://github.com/wstein/bundledock/tree/main/cmd cli release?

and, asked whether `bdock` should be released like SheetMill with a tag-triggered CI release or
prepared locally, decided:

> bdock wie smill

This spec reads the decision as the SheetMill 019 model for the `bdock` CLI: an owner-pushed
annotated tag is the publication approval for that tag's release assets; merging the workflow,
every tag operation, every workflow run and every repository or environment setting stay with the
owner. It does **not** authorize: running the workflow or `make release` (an agent never does),
creating, moving or deleting a tag, adding secrets, variables or environment protection, OCI
image builds, publication or tags, consumer activation, or any other tag namespace. The existing
bundle publication exceptions and the Pages exception in `AGENTS.md` are unchanged.
Whether "bdock wie smill" is a standing approval for every later tag is open decision 3.

## Scenarios

1. **Owner releases.** The workflow is on `main`, CI is green for a `main` commit. The owner
   pushes the annotated tag `bdock/v0.1.0`. The workflow verifies the tag, builds three
   archives twice each (byte-identical), smoke-tests each natively, and creates a GitHub
   release with three archives and `SHA256SUMS`.
2. **A wrong tag is refused.** A lightweight tag, a tag outside `main`, a tag without a green CI
   push run, a tag pushed or re-run by someone other than the repository owner, or a tag that
   already has a release: `verify` or `publish` fails and nothing is published.
3. **Image tags are untouched.** Pushing `toolbox/2026.10.15-b98.1`, `opentofu/1.13.1-b1` or any
   other existing or future bundle tag never starts this workflow.
4. **The owner rehearses.** `workflow_dispatch` by the owner runs the same gates, builds and
   smoke tests as a dry run; `publish` is skipped.
5. **A downloader verifies.** `sha256sum --check` passes; `bdock --version` prints
   `bdock version 0.1.0`; `PROVENANCE` inside the archive names the source revision.

## Requirements

- **FR-1 Tag namespace and version.** Release tags are exactly `bdock/vMAJOR.MINOR.PATCH`
  (`^bdock/v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`), annotated, on a commit
  reachable from `main`. No pre-release or build-metadata tags. The stamped version
  (`-X main.version`, `bdock --version`, archive names, `PROVENANCE`) is the tag without
  `bdock/v`: `0.1.0`. The first release is `bdock/v0.1.0` ([research D2](research.md)).
  A tag is never re-created after a failed or wrong release; the next patch version is used.
- **FR-2 Triggers.** `push` of tags `bdock/v[0-9]+.[0-9]+.[0-9]+` and `workflow_dispatch` with one
  string input `version` (default `0.0.0-dryrun`, must carry a suffix so it can never equal a
  release version). No branch push, no `pull_request`. A dispatch is a dry run and never
  reaches `publish`.
- **FR-3 verify** (`contents: read`, `actions: read`; full history). In order: resolve the
  version (strict tag regex / dry-run regex, input only through `env:`); require
  `github.triggering_actor` and `github.actor` to equal `github.repository_owner` on every run
  kind; on a tag push: fetch the tag object explicitly (forced refspec, `--no-tags`), require
  an annotated `bdock/v…` tag whose commit is an ancestor of `origin/main` and equals
  `GITHUB_SHA`; require a successful `ci.yml` run with `event=push` for the commit; require
  that no release exists for the tag (HTTP 404 only; any other API answer fails closed). In
  every run: `make check` and `go mod verify`. Outputs `version`, `tag`, `dry_run`, `commit`.
- **FR-4 build** (`needs: verify`, `contents: read`). Matrix `linux/amd64`, `linux/arm64`,
  `darwin/arm64` on `ubuntu-24.04`. Each leg asserts `HEAD == COMMIT`, runs the single archive
  builder `scripts/release.sh` with `TARGETS=<os>/<arch>` twice (second time with an empty
  `GOCACHE`, separate output directory outside the checkout), requires `diff -r` to show
  identical trees, and requires `PROVENANCE` to name `source_revision=$COMMIT` and
  `source_modified=false`. `CGO_ENABLED=0`, `GOTOOLCHAIN=local`, `GOFLAGS=-mod=readonly`.
  It uploads the archive and a `.sha256` sidecar (retention 1 day). darwin/amd64 (end of life)
  and Windows are not built.
- **FR-5 smoke** (`needs: [verify, build]`). One job per target on a native runner:
  `ubuntu-24.04`, `ubuntu-24.04-arm`, `macos-15`. `HEAD == COMMIT`. `release-smoke.sh` checks
  the sidecar checksum; exact members `bdock`, `LICENSE`, `PROVENANCE`; `LICENSE` equals the
  repository's; `PROVENANCE` for this version, commit, OS, arch and an unmodified checkout;
  `bdock --version` equals `bdock version <version>`; `--help`; `tools --help` lists `volume`
  and `upgrade`; `completion bash` prints something. Only hermetic commands run: no network,
  no Docker, no registry. This proves the shipped bytes start and carry their stamp on each
  platform; it does **not** prove `tools volume` or `tools upgrade` work (they need Docker and
  the network).
- **FR-6 Archives.** `bdock_<version>_<os>_<arch>.tar.gz`, flat: `bdock` (0755), `LICENSE`
  and per-platform `PROVENANCE` (0644), owner 0, mtime 0, `ustar`, `gzip -n`. This is the
  layout `scripts/release.sh` has always produced; it is the only archive builder, shared by
  local preparation and CI. `SHA256SUMS` in the release lists exactly the three archives.
- **FR-7 publish** (`needs: [verify, build, smoke]`; only on a tag push with `dry_run ==
  false`; `contents: write`; `environment: release`). Re-checks both owner conditions (a
  re-run by someone else does not reuse the owner's approval); lists all releases including
  drafts and fails on the tag; requires the remote tag still to peel to the verified commit;
  requires `HEAD == COMMIT`; exactly three archives and three checksums; `sha256sum --check`;
  notes from `release-notes.sh`; `gh release create "$TAG" --verify-tag`. Not a draft unless
  the repository variable `BDOCK_RELEASE_DRAFT` is `true`.
- **FR-8 Security posture.** Top-level `contents: read`; only `publish` has write; no secrets;
  `GH_TOKEN` is `github.token`; every action pinned to a full SHA from the allow-list and
  only with the exact `with:` keys; `persist-credentials: false` on every checkout; timeouts;
  no `continue-on-error`, `if: false`, job-level `env`, `${{ }}` inside `run:`, network
  fetches other than GitHub, and no `make release` (the builder is `scripts/release.sh`
  directly). Gates must stay literal `test … || { echo … >&2; exit 1; }` lines; `exit 0`,
  `set +e`, `|| true`, `|| echo` are forbidden in run blocks.
- **FR-9 Tests.** `internal/releaseworkflow` (test-only; adds `go.yaml.in/yaml/v3`, which no
  shipped package imports): rules over `release.yml`, about 60 single-edit mutants that must
  each be caught, script tests in throwaway git repositories (version and tag rules, notes
  baseline, smoke against archives made by the real `scripts/release.sh` with a fake
  compiler and mutated archives), a check that no other workflow starts on `bdock/` tags,
  and the CI platform-vet step. `scripts/release_test.py` covers the builder (three targets,
  modes, determinism, `TARGETS` validation). All run in `go test ./...` and `make check`
  without building a distribution artifact.
- **FR-10 Pipeline.** `ci.yml` keeps its job name `check` and gains a step that runs `go vet`
  for the three release platforms (compiler verification, no artifact). It already runs on
  every push including tag pushes; the release requires its success for the tagged commit.
- **FR-11 Documentation.** `docs/releasing.md` (owner procedure), this spec set,
  `docs/release-readiness.md` rewritten, `docs/development.md`, `README.md`, `CONTRIBUTING.md`,
  `AGENTS.md`.

## Known limitations (accepted)

- Workflow tests are string- and line-level rules plus mutants, not a shell parser; they catch
  casual weakening. The reviews and the owner's review of the workflow PR are the real gate.
- Reproducibility is shown on one runner with one toolchain (two builds, one fresh cache);
  independent reproduction is not claimed.
- Nothing is signed, notarized or attested. macOS may quarantine a browser download.
- Owner checks compare with `github.repository_owner`; if the repository moves to an
  organization they fail closed until rewritten.
- Not exercisable before the first real tag: annotated-tag fetch, the CI-run query, the
  release-exists API call for a tag containing a slash, `ls-remote`, the `release`
  environment, `gh release create`, runner labels. The `releases/tags/bdock/vX.Y.Z` route is
  unverified; if it answered 404 for an existing release, the publish job's list check
  still refuses.
- The `ci.yml` run for the tag push itself is unrelated to `main` CI; the gate counts any
  successful push run of the same commit, which is the same code.

## Out of scope

Windows, darwin/amd64, linux/386, Homebrew/apt/Docker/`go install` publication, SBOMs,
attestations or signatures, pre-release tags, automatic version bumps, changelog files,
OCI image releases (see [publication](../../publication.md)), running the workflow,
tagging and merging.

## Open decisions (recommendations)

1. **Confirm `v0.1.0` as the first version.** Recommended: yes (no `bdock/` tag exists, docs
   already use `0.1.0`, [research D2](research.md)).
2. **The `release` environment.** `publish` names `environment: release`, as SheetMill does. If
   it does not exist GitHub creates it unprotected on first use. Recommended: the owner creates
   it beforehand with himself as required reviewer and a deployment-tag rule `bdock/v*`; the
   tag is the approval either way and the workflow does not depend on the protection.
3. **Standing approval.** Does "bdock wie smill" cover every later `bdock/vX.Y.Z` tag the owner
   pushes (this spec's reading, as for SheetMill) or only the first? Recommended: standing, with
   the owner confirming before the second release. Until confirmed it is a reading, not a grant.
4. **Amend the `AGENTS.md` rule wording** that CLI release builds need per-operation approval:
   done here as an exception paragraph; the owner may prefer to phrase it differently.
5. **Tag ruleset.** Block deletion and update of `bdock/v*` tags after release. Recommended.
6. **Attestations** (build provenance) after the first release. Recommended later.
7. **Review of the local TODO list** (`AGENTS.md`) before the first tag is a precondition the
   owner checks; no tool can check an untracked list.
