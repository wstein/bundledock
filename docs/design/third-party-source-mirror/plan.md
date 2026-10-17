# Plan: third-party source mirror

Spec: [spec.md](spec.md). Status: draft; no code exists.

## Summary

Add a `source` block to the toolbox catalog, a registry file `sources.lock.json`, three offline/online
modes of `bdock tools sources` that produce and verify deterministic source bundles, and a check in
`make check` that fails when a copyleft tool at a locked version has no mirror release entry. The
bundles are published to `wstein/bundledock-third-party-sources` by SourceSteward's release process
(Spec 026, in flight). `SOURCE` files in the image point to the releases.

## Technical context

- Language/tooling: Go (module `github.com/wstein/bundledock`), existing packages
  `internal/toolconfig` (catalog/lock loading, `Validate`), `internal/toolinventory`,
  `internal/toolbundle`, `scripts/tool-inventory.py`. Catalog: `bundles/toolbox/packages.catalog.json`
  (`schema` 1), lock: `bundles/toolbox/packages.lock.json` (`schema` 2, `desired_digest`,
  `catalog_digest`). (The brief called these `tools.catalog` and `tools.lock`; on `origin/main`
  1cf2173 the names are `packages.*`.)
- New packages: `internal/sourcebundle` (manifest, archive writer, safe reader), `internal/sourcefetch`
  (ecosystem fetchers behind an injected HTTP client), a `tools sources` subcommand group in
  `cmd/bdock`.
- No new third-party Go dependency except possibly `golang.org/x/mod` (dirhash, module paths):
  decision in [D4](#design-decisions); the alternative is a 60-line in-tree `h1:` implementation.
- Tests: table-driven unit tests with an in-memory HTTP transport and tiny synthetic archives; golden
  bundle fixtures; mutation expectations in [tasks](tasks.md); no network, Docker or image in
  `make check`.

## Constitution check

BundleDock has no constitution file. The rules in `AGENTS.md` are applied instead.

| Rule | Check |
| --- | --- |
| Third-party tools: official stable prebuilt releases, never built from source | Honored: the mirror never builds; bundles are inputs, not outputs |
| No build/release without explicit approval | `fetch` produces a distributable file and is classed as an artifact build requiring approval each time; `plan`, `check` and tests do not |
| Pushes/merges/schedules do not authorize builds | The mirror CI runs only by `workflow_dispatch` by the owner or an authorized dispatch under Spec 026 gates |
| Lock-only updater must not dispatch native builds | Honored; the mirror workflow is never triggered by an updater |
| Updater scope: own lock and generated inventory only | **Tension**: a copyleft upgrade needs a `sources.lock.json` edit; see [open question 2](#open-questions-for-the-owner) |
| Atomic Conventional commits, TDD | [tasks](tasks.md) |
| Keep tools, mount prefix, volume names and config filenames | Untouched |

## Project structure

```text
docs/design/third-party-source-mirror/    this design
bundles/toolbox/packages.catalog.json     + "source" block per package        (WI1, WI2)
bundles/toolbox/sources.lock.json         mirror registry                     (WI7)
bundles/toolbox/licenses/<tool>/SOURCE    mirror block (copyleft tools)       (WI10)
internal/toolconfig/                      parse/validate the source block     (WI1)
internal/sourcebundle/                    manifest, writer, safe reader       (WI3, WI4)
internal/sourcefetch/                     fetchers                            (WI6)
cmd/bdock/tools_sources*.go               plan, fetch, check                  (WI5, WI6, WI7)
docs/third-party-sources.md               owner procedure and settings        (WI11)
mirror repo: plans/, .stw/ contract, workflow                                 (handoff, WI12)
```

## Design decisions

- **D1 Producer in BundleDock, publisher in SourceSteward.** Only `bdock` can map a lock entry to
  the source that corresponds to the shipped binary. SourceSteward stays language-agnostic: it sees a
  `file` artifact whose build command is the fetch step. Interface in
  [contracts/mirror-project.md](contracts/mirror-project.md).
- **D2 Catalog-declared recipes.** One `recipe.kind` per tool, closed vocabulary, no argv. `custom`
  deferred.
- **D3 Plans live in the mirror repo.** `bdock tools sources plan` (offline) writes
  `plans/<component>/<version>.json`; the mirror repo's commit pins the plan and thus the recipe and
  expected digests. The mirror workflow takes `(component, version)` and the plan is read from that
  commit. The bundledock repo's `sources.lock.json` then records the release back.
- **D4 No toolchain invocation.** `go mod download`, `cargo vendor` and `cabal fetch` run
  lock-file-driven code paths of those tools; to honor FR-8 and to be testable offline, fetchers
  re-implement the protocols (Go module proxy zip/mod/info URLs and `h1:` dirhash; crates.io sparse
  index or `static.crates.io` `.crate` URL with the `Cargo.lock` checksum; Hackage
  `package/<pkg>-<ver>.tar.gz` plus index `.cabal` revision). Cost: protocol drift risk; mitigated by
  fixture tests and by recording the fetch URLs in the manifest. Measured for trufflehog v3.99.2:
  `go mod download` of the `go.sum` set is 310 modules and 327 MB of zips; `go mod vendor` gives 157 MB
  and 312 modules in `modules.txt` ([research R3](research.md#r3-go-measurement)). The bundle takes
  the full `go.sum` set (superset, simple, verifiable) and records the vendor subset as unmeasured.
- **D5 Registry in the bundledock repo.** `sources.lock.json` (schema 1) is written by a human/agent
  after the release exists; `check` reads it. Separate from `packages.lock.json` so the updater
  contract and `catalog_digest`/`desired_digest` semantics do not change except for the catalog edit
  itself (the one `catalog_digest` recomputation happens in WI1/WI2 and is a regenerate-lock
  commit with unchanged versions).
- **D6 Check is Go, in `make check`.** `tool-inventory.py --check` stays a doc-drift check; it gains
  nothing. A Go check can reuse `toolconfig` validation.
- **D7 Tag shape** `<component>-v<version>` with `-rN` re-issue; per-component monotonic.
- **D8 Image content.** `SOURCE` is text, rides in `licenses.tar.gz` unchanged mechanism.

## Rollout order

Authoritative order; each step needs the previous. Steps marked (owner) are not done by an agent.

1. (done) Repository `wstein/bundledock-third-party-sources` exists, public, EUPL-1.2 README + LICENSE.
2. (owner) Apply [settings](#mirror-repository-settings-for-the-owner): immutable releases, tag
   ruleset, Actions enabled.
3. Source-level work in bundledock: WI1 to WI7 (merged, `make check` green; `check` runs in
   report-only mode, `--enforce` off).
4. Mirror repo: WI12 (plans directory, project contract, workflow) once Spec 026 is usable. Until
   then the coordinator may run the same `fetch` under the owner's explicit approval and attach
   assets by hand as an interim, recorded in the release body (decision: owner, open question 4).
5. Backfill (each needs explicit approval of the exact operation): `fish-v4.9.3`,
   `shellcheck-v0.11.0`, `trufflehog-v3.99.2`, `helix-v25.07.1`; plus any older copyleft versions found
   in published carriers (WI9 count).
6. WI10: `SOURCE` rewrite and `sources.lock.json` entries for the backfilled set; WI8 flips `check`
   to enforcing.
7. PR #119 (hadolint): rebase after step 6 is in `main`; add `hadolint-v2.15.1` plan and release; its
   `SOURCE` takes the mirror block; its tests (`TestHadolintRetainsGPLSourceRecordAndThirdPartyNotices`)
   keep the `Source archive:` / `Source SHA256:` lines; the "relies on upstream availability" sentence
   is replaced. Hadolint is not publishable in a bundle before its release exists.
8. Next toolbox bundle publication (separately authorized) is the first image whose `SOURCE`
   files carry the mirror block; consumer activation separate.

## Mirror repository settings for the owner

Documentation only; nothing here is automated by this design. Verify in the GitHub UI:

- Public repository; wiki, projects and discussions off; Actions enabled for the repository's own
  workflows only.
- Releases: **immutable releases** on (assets and tag locked after publication).
- Tag ruleset for `*-v*`: restrict creation to the release process identity or the owner, block
  deletion and updates.
- Default-branch protection: require PR and the mirror CI check; no force pushes.
- `LICENSE` EUPL-1.2 applies to the repository's own scripts, workflows, plans and manifests; each
  bundle keeps its upstream licenses (stated in README and in each manifest `bundle_license_note`).
- Environment `release` (name per Spec 026) with no standing approval; no secrets beyond the default
  token unless Spec 026 requires an attestation identity.
- Actions permissions default read; publisher job `contents: write`, `attestations: write`,
  `id-token: write` (to be confirmed against Spec 026).

## Risks

| Risk | Mitigation |
| --- | --- |
| Hadolint freeze reconstructed from a license report, not upstream's real solver output | manifest field `reconstruction: true`, listing source of truth; lawyer item 7; best-effort statement in `SOURCE` |
| Static system libs (GMP etc.) in Haskell binaries missing from bundle | `static_system_libs` listed; open question 3 |
| Protocol drift in fetchers | fixtures, recorded URLs, `fetch` is not in `make check` |
| Spec 026 changes shape | interface is a thin contract file; tasks WI12 is the only coupled one |
| Large assets | limit table; split strategy deferred |
| Updater scope tension | open question 2 |

## Open questions for the owner

1. Re-issue naming `-rN` (recommended) vs a new patch-like component version.
2. Copyleft upgrades: allow a human follow-up commit adding the `sources.lock.json` entry to an
   updater PR (recommended: yes, and copyleft bumps are excluded from automatic publication until
   then), or permit the updater to write that file.
3. Include `static_system_libs` sources for Haskell static builds (recommended: list now, include
   Alpine package sources only after the lawyer answers).
4. Interim manual publication if Spec 026 is late (recommended: no; wait, or run the identical
   `fetch` under explicit approval and record it).
5. Split strategy for bundles over 1.9 GiB (recommended: defer; none known).
6. Make `licenses/<tool>/SOURCE` for permissive tools also point at the mirror (recommended: no).
