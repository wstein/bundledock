# Tasks: third-party source mirror

Work items in stacking order. Each ends with `make check` green on source only (no image, no artifact build,
no `make docs`). Every behavior item starts with a failing test (TDD) and lists the mutants that must die: a
mutation run (single-edit mutants of the item's code; each must make a test fail) is recorded in the PR
description; survivors are fixed or justified. FR numbers are in the [spec](spec.md); SC in the
[checklist](checklists/requirements.md). Hostile-input work is WI4 and WI6; documentation is WI11. There is
no concurrency in v1 (one process); WI6 tests that two parallel `fetch` runs into different `--out` do not
interfere (private 0700 temp directory per run).

| WI | Work | FR | Tests first (failing) | Mutants that must die |
| --- | --- | --- | --- | --- |
| WI1 | `internal/toolconfig`: parse and validate the catalog `source` block; schema stays 1; regenerate `catalog_digest` without changing versions | FR-1, FR-3, FR-7 | accepts the data-model example; rejects unknown keys, bad enum, `none` with recipe, http URL, `..`, version shape, `cabal-freeze` with neither/both freeze fields | drop each validation; skip the shape regex |
| WI2 | **Measurement and classification** (blocks the backfill list, the asset limit and the enforcement flip): for every shipped binary of both architectures list embedded dependencies and their licenses (Go `go version -m` on the lock binary, Cargo.lock at the tag, `ThirdPartyNotices`, SBOM), include `archive_notices` members, `fd`/`rg`, the eight PR #119 tools and `typst`; apply the designed fetch rule to measure each candidate bundle size; fill `source` for all 41 (+9) packages; record the result as a table in this design directory | FR-1, FR-16 | test: real catalog classified, `source-offer` set equals the recorded table; fixture where a `none` package retains GPL text fails; size table compared to the asset limit | remove marker; mislabel one package |
| WI3 | `internal/sourcebundle`: manifest types and schema validation, deterministic writer (`pax-sorted-v1`, normalized argv, `SOURCE_DATE_EPOCH`), `tar_sha256`/`asset_sha256` | FR-5, FR-6 | golden bundle; invariance to order, mtimes, owners, umask, TZ, `--out` path | each normalization removed; order unsorted; wall-clock mtime; raw argv recorded |
| WI4 | In-memory bounded tar/gz/xz stream reader and parsers for `Cargo.lock`, `go.sum`, `go.mod`, `*.cabal`, dependency report; anomaly recording | FR-8, FR-9 | every row of the [limits](spec.md#limits); real-shape fixtures: absolute symlink, duplicate members (last wins), hardlink, device, long and NUL names, bomb, truncated stream, case-colliding names; nothing written to disk | each constant off by one; last-wins inverted; extraction introduced |
| WI5 | `pin` (network via injected client, peels tags, writes entries into `sources/<component>.lock.json` with digests and `source_date_epoch`), `plan` (offline validation, `--lock`, `--standalone`); `sources.lock.json` loader | FR-3, FR-7 | pin/plan per recipe kind on fixtures; lock arch mismatch; `custom` refused; `plan` fails the test if the transport is used; stale plan | arch check removed; network allowed in plan |
| WI6 | `fetch` with injected HTTP: upstream-tarball-only, go-mod-download (`h1:` dirhash, zip + `.mod` rule), cargo-vendor (`.crate` + checksum, git sources via plan pins), cabal-freeze (sdists, reconstruction); per-hop host checks; failure table | FR-4, FR-6, FR-7, FR-8, FR-11 | fixtures per ecosystem; 404, wrong hash, redirect to a refused host, redirect loop, oversize, partial write cleanup, no process started, non-empty `--out` | hash compare skipped; per-hop check only on first hop; partial output left |
| WI7 | `check` (registry, `--online`, `--bundle-dir`), `sync` / `SOURCE-MIRROR` generator, JSON report, Go-test wiring in `make check`, report-only default; `pin` at lock time for new copyleft tools | FR-2, FR-12, FR-13, FR-14 | one test per finding code in the [check contract](contracts/sources-check.md); tamper cases (changed byte, extra file, missing file, wrong manifest, `MANIFEST.json` mismatch); drift of `SOURCE-MIRROR` | each code unreachable; warning/error swap; tamper accepted |
| WI8 | Flip default to `--enforce` (rollout step 7, after WI10) | FR-2, FR-12 | enforce default; real tree passes | flag ignored |
| WI9 | (gates each component's first release) Count copyleft versions in published carriers from `docs/toolbox-published.json` and the lock at each published source tag; produce the oldest-first backfill list | FR-16 | script test on fixture history | n/a |
| WI10 | `SOURCE` version-independent paragraph, generated `SOURCE-MIRROR`, catalog `notices` entries, `sources.lock.json` entries (after the releases exist); starts with a test that proves the image staging accepts the new notice file | FR-13 | format and consistency tests; hadolint-style `Source archive:` lines untouched | drop sentence; tag typo |
| WI11 | Docs: `docs/third-party-sources.md` (owner procedure, settings, [upgrade sequence](plan.md#upgrade-sequence), retention), `docs/toolbox.md` section, `AGENTS.md` sentences (copyleft bumps need `check --enforce`; pin classification), README link | FR-15, FR-17 | docs link test | n/a |
| WI12 | Mirror repository handoff (not a bundledock PR): contract, workflow, README per [mirror-project.md](contracts/mirror-project.md); blocked on Spec 026 | FR-10 | contract lint in the mirror repo | n/a |
| WI13 | PR #119 follow-up: hadolint plan (API tarball), release, paragraph and `SOURCE-MIRROR`; replace "relies on upstream availability" | FR-2, FR-13 | existing hadolint tests pass; `check --enforce` passes | n/a |

FR-3 is covered by WI1 (shape), WI5 (source-lock paths) and WI7 (tags); FR-15 by WI11 only (policy text).

## Rollout order

[plan.md#rollout-order](plan.md#rollout-order): repository (done) -> owner settings -> WI1, WI3..WI7 -> **WI9, WI2** ->
mirror workflow (WI12, Spec 026) -> approved backfill oldest-first -> WI10, WI8 -> WI13 (hadolint) -> next
approved toolbox publication.
