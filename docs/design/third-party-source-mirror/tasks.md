# Tasks: third-party source mirror

Work items in stacking order. Each ends with `make check` green on source only (no image, no
artifact build, no `make docs`). Every behavior item starts with a failing test (TDD) and lists the
mutants that must be caught: a mutation run (single-edit mutants of the item's code, each must make
a test fail) is recorded in the PR description; survivors are fixed or justified. Tags: FR numbers
from the [spec](spec.md); SC are in the [checklist](checklists/requirements.md).

| WI | Work | FR | Tests first (failing) | Mutants that must die |
| --- | --- | --- | --- | --- |
| WI1 | `internal/toolconfig`: parse and validate the catalog `source` block; schema stays 1; regenerate `catalog_digest` without changing versions | FR-1, FR-7 | accepts the data-model example; rejects unknown keys, bad enum, `none` with recipe, host outside allow-list, http URL, `component` != key, `..` in template | drop each validation; widen allow-list; skip `component` check |
| WI2 | Fill `source` for all 41 catalog packages (provisional classes), `undetermined` where unscanned; add the license-marker contradiction test; one-time dependency review (`go version -m`, SBOM) for static Go/Rust tools recorded as `basis: scanned` | FR-1 | test fails for a fixture where a `none` tool retains GPL text; real catalog test lists exactly fish, shellcheck, trufflehog, helix as `source-offer` | remove marker; mislabel one tool |
| WI3 | `internal/sourcebundle`: manifest types, schema validation, deterministic writer (sorted, normalized headers, `SOURCE_DATE_EPOCH`), `tar_sha256`/`asset_sha256` | FR-5, FR-6 | golden bundle from a synthetic tree; invariance to input order, mtimes, owners, umask, TZ | each normalization removed; order unsorted; mtime from wall clock |
| WI4 | Bounded safe reader for upstream tarballs (tar, tar.gz, tar.xz) and parsers for `Cargo.lock`, `go.sum`, `.cabal` dependency report | FR-8, FR-9 | every row of the [limits table](spec.md#limits) plus traversal, absolute path, symlink out, hardlink out, device, long names, NUL, bomb, truncated stream, duplicate members, case-colliding names | each limit constant off by one; link check inverted |
| WI5 | `plan` mode (offline), including tag peeling input and `source_date_epoch`; `sources.lock.json` loader | FR-7 | plan for each recipe kind from fixtures; arch mismatch refusal; `custom` refused; no network (transport that fails the test if used) | arch check removed; network allowed |
| WI6 | `fetch` mode with injected HTTP client: upstream-tarball-only, go-mod-download (`h1:` dirhash), cargo-vendor (`.crate` + checksum, git sources), cabal-freeze (Hackage tarballs, reconstruction flag); `--accept-new-digest`; failure table | FR-4, FR-6, FR-7, FR-8, FR-11 | fixtures per ecosystem; 404, wrong hash, redirect loop, oversize, wrong host, partial write; no process started (hook in runner); output dir non-empty | hash compare skipped; host list widened; partial output left on failure |
| WI7 | `check` mode (registry and `--bundle-dir`), JSON report, `make check` wiring, report-only default; pin the source tarball digest for new copyleft tools at lock time | FR-2, FR-12, FR-14 | one test per finding code in the [check contract](contracts/sources-check.md); bundle tamper cases (changed byte, extra file, missing file, wrong manifest) | each code unreachable; warning/error swap; tamper accepted |
| WI8 | Flip default to `--enforce` (after rollout step 6) | FR-2, FR-12 | existing tests with enforce default; real tree passes | enforce flag ignored |
| WI9 | Count copyleft versions in published carriers from `docs/toolbox-published.json` and lock history at each published source tag; write the backfill list | FR-16 | script test on a fixture history | n/a (analysis, no behavior) |
| WI10 | `SOURCE` mirror blocks and `sources.lock.json` entries for fish, shellcheck, trufflehog, helix (after their releases exist); format test | FR-13 | test requires the block, URL/tag/hash consistency, offer sentence | drop sentence; tag typo |
| WI11 | Docs: `docs/third-party-sources.md` (owner procedure, settings checklist, upgrade procedure, retention), `docs/toolbox.md` section, `AGENTS.md` sentence (bundle publication needs a passing `check --enforce`), README link | FR-15, FR-17 | docs link test | n/a |
| WI12 | Mirror repository handoff (not a bundledock PR): `plans/`, project contract, workflow, README settings text per [mirror-project.md](contracts/mirror-project.md); depends on Spec 026 | FR-10 | contract lint in the mirror repo | n/a |
| WI13 | PR #119 follow-up: hadolint plan, release, `SOURCE` block, replace the "relies on upstream availability" sentence | FR-2, FR-13 | existing hadolint tests still pass; `check --enforce` passes | n/a |

Documentation tasks are WI11; hostile-input cases are WI4/WI6; there is no concurrency in v1 (single
process), the temp directory is private (0700) and unique per run (test: two parallel `fetch` runs into
different `--out` do not interfere).

## Rollout order

Defined in [plan.md#rollout-order](plan.md#rollout-order): repository (done) -> owner settings ->
WI1..WI7 -> mirror workflow (WI12, Spec 026) -> approved backfill of fish, shellcheck, trufflehog,
helix -> WI10, WI8 -> WI13 (hadolint) -> next approved toolbox publication.
