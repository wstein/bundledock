# Plan: third-party source mirror

Spec: [spec.md](spec.md). Status: draft; no code exists.

## Summary

Add a `source` block to the toolbox catalog, a registry file `sources.lock.json`, `pin`, `plan`, `fetch`, `check` and `sync` modes of
`bdock tools sources` that produce and verify deterministic source bundles, and a check in
`make check` that fails when a copyleft tool at a locked version has no mirror release entry. The
bundles are published to `wstein/bundledock-third-party-sources` by SourceSteward's release process
(Spec 026, in flight). The image notices (`SOURCE`, generated `SOURCE-MIRROR`) point to the releases.

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

BundleDock has no constitution file; the rules in `AGENTS.md` (repository and umbrella) are applied.

| Rule | Check |
| --- | --- |
| Third-party tools: official stable prebuilt releases, never built from source | Honored: the mirror never builds a third-party tool; bundles are inputs |
| No artifact build without explicit approval | `fetch` produces distributable files and needs approval each time. `pin` downloads upstream bytes only to hash them in a private temp directory and writes a plan (no distributable output); classifying it as not a build is an **owner decision** ([open question 7](#open-questions-for-the-owner)). `plan`, `check`, `sync` and tests need none |
| Pushes, merges, schedules, bot activity do not authorize builds | The mirror run is a `workflow_dispatch` by an authorized actor under Spec 026 gates; nothing is triggered by an updater |
| Lock-only updater must not dispatch builds; updater scope is its own lock and generated inventory | **Tension, resolved by an explicit human step** in the [upgrade sequence](#upgrade-sequence); copyleft bumps cannot auto-publish |
| Historical license snapshots independent of binary pins | `SOURCE` stays version-independent; version data in generated `SOURCE-MIRROR` (FR-13) |
| Atomic Conventional commits, TDD | [tasks](tasks.md) |
| Keep pinned image, mount prefix, tools, volume names, config filenames | Untouched |

## Project structure

```text
docs/design/third-party-source-mirror/    this design
bundles/toolbox/packages.catalog.json     + "source" block per package        (WI1, WI2)
bundles/toolbox/sources.lock.json         mirror registry                     (WI7)
bundles/toolbox/licenses/<tool>/SOURCE    + version-independent paragraph     (WI10)
bundles/toolbox/licenses/<tool>/SOURCE-MIRROR  generated, version-specific    (WI7 sync, WI10)
internal/toolconfig/                      parse/validate the source block     (WI1)
internal/sourcebundle/                    manifest, writer, safe reader       (WI3, WI4)
internal/sourcefetch/                     fetchers                            (WI6)
cmd/bdock/tools_sources*.go               pin, plan, fetch, check, sync      (WI5, WI6, WI7)
docs/third-party-sources.md               owner procedure and settings        (WI11)
mirror repo: plans/, .stw/ contract, workflow                                 (handoff, WI12)
```

## Design decisions

- **D1 Producer in BundleDock, publisher in SourceSteward.** Interface in
  [contracts/mirror-project.md](contracts/mirror-project.md).
- **D2 Catalog-declared recipes.** One `recipe.kind` per tool, closed vocabulary, no argv; `custom` refused.
- **D3 Plans live in the mirror repo.** `pin` (network) writes `plans/<component>/<V>.json` with digests, the
  peeled tag commit and `source_date_epoch` (commit time). `plan` (offline) validates a plan against
  catalog and lock and renders it. The release-process commit C contains the plan; `fetch` reads it from C.
- **D4 No toolchain invocation.** Fetchers re-implement the protocols (Go module proxy URLs and `h1:`
  dirhash, crates.io `static.crates.io` `.crate` URLs, Hackage sdist URLs). Cost: protocol drift, mitigated
  by fixtures and by recording URLs. Measured for trufflehog v3.99.2 with Go's own tooling: 310 modules,
  327 MB of zips from `go mod download`; but `go.sum` has 1107 lines of which 369 carry a zip `h1:` and the
  rest are `/go.mod`-only, so the designed set (zip entries plus `.mod`) is **not** what was measured
  ([research R3](research.md#r3-go-measurement)); WI2 re-measures it with the designed rule.
- **D5 Registry in the bundledock repo.** `sources.lock.json` (schema 1), separate from
  `packages.lock.json` so updater semantics change only by the one catalog edit.
- **D6 Check is Go.** `tool-inventory.py --check` stays a doc-drift check.
- **D7 Naming** per the [naming table](spec.md#naming-table).
- **D8 Notices.** `SOURCE` version-independent paragraph; generated `SOURCE-MIRROR` (FR-13). Whether the
  image builder stages a new notice file with no code change (the catalog `notices` list) is unverified;
  WI10 starts with a failing test that reads the staging code.
- **D9 Pinned toolchain for reproducibility.** The plan records `bdock_source_commit` and
  `bdock_go_version`; the mirror workflow uses exactly that Go (FR-6).

## Upgrade sequence

Normative. Preconditions: the version is copyleft (`source-offer` or `file-level-source`).

1. The bot updater PR changes `packages.lock.json` (own-file scope). `check --enforce` is red with
   `mirror-entry-missing`; the PR cannot merge.
2. A human or an authorized agent runs `bdock tools sources pin <tool> --lock <path to the PR head's
   lock>` (the lock path is a flag so the PR head is not needed in the working tree), reviews the plan,
   and opens a mirror-repo PR containing `plans/<component>/<V>.json`.
3. After the mirror PR merges, the owner approves the exact build; the release process builds C, drafts,
   records, and the owner pushes the annotated tag (Spec 026 D1/D4).
4. A human pushes a follow-up commit to the updater PR branch with `sources.lock.json` and the regenerated
   `SOURCE-MIRROR` (`bdock tools sources sync`). The PR now has a non-bot commit and files outside the
   updater's own-file scope, so by `AGENTS.md` **automatic publication stops**; publication needs the
   owner's manual exact-SHA dispatch. This is the accepted consequence ([open question 2](#open-questions-for-the-owner)).
5. `check --enforce` is green; the owner merges; bundle publication proceeds under its own approval.

## Rollout order

Authoritative; each step needs the previous. (owner) = not done by an agent.

1. (done) Repository exists, public, EUPL-1.2 README and LICENSE.
2. (owner) Apply the [settings](#mirror-repository-settings-for-the-owner).
3. bundledock source work WI1, WI3 to WI7 (`check` report-only, `--enforce` off).
4. **WI2 measurement and classification** (dependency licenses of every shipped binary; sizes of every
   candidate bundle). Its recorded result fixes: the backfill list (every `source-offer` and
   `file-level-source` component), the asset limit and the need for a split strategy, and the date of the
   enforcement flip. No backfill starts before this step.
5. Mirror repo handoff WI12 once Spec 026 is usable. Interim manual publication is **not** allowed
   (open question 4).
6. Backfill, each release needs explicit approval of the exact operation, **oldest version first per
   component** (monotonic ordering): at least fish, shellcheck, trufflehog, helix, typst (pending lawyer
   item 6) plus the WI2 additions and older versions found by WI9.
7. WI10 notices and registry entries; WI8 flips `check` to `--enforce`.
8. PR #119: rebase after step 7 is on `main`; hadolint plan and release (`hadolint-v2.15.1`; the plan uses
   the **API tarball** `48d1ca56...` that PR #119 already pins, not the `archive/refs/tags` tarball);
   its `SOURCE` keeps `Source archive:` / `Source SHA256:` lines; the "relies on upstream availability"
   sentence is replaced. Not publishable in a bundle before its release exists.
9. The next separately authorized toolbox publication is the first image carrying the new notices;
   consumer activation is separate.

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
| Hadolint/shellcheck dependency set is a reconstruction | `reconstruction: true`; legal item 8; stated in `SOURCE-MIRROR` |
| Scale: MPL-2.0 dependencies in ~20 tools, many large Go module sets | WI2 before backfill; limits depend on it |
| Static system libs in Haskell binaries | listed in `static_system_libs`; legal item 3 |
| Protocol drift in fetchers | fixtures; `fetch` not in `make check` |
| Spec 026 not usable (draft, constitution amendment pending) | work through WI7 is independent; WI12 waits |
| Spec 026 per-contract track model vs one multi-component repo | [mirror-project.md](contracts/mirror-project.md) requirements; open question 8 |
| Copyleft bumps cannot auto-publish | accepted ([upgrade sequence](#upgrade-sequence)) |

## Open questions for the owner

1. Re-issue naming `-rN` (recommended).
2. Copyleft upgrades need a human follow-up and manual exact-SHA publication (recommended: accept; the
   alternative lets the updater write `sources.lock.json`, which needs a release first and is not possible
   offline).
3. Shellcheck dependency set: armv6hf freeze as documented approximation, or owner-reviewed freeze file
   (recommended: reviewed freeze file with `reconstruction: true`).
4. Interim manual publication if Spec 026 is late (recommended: no).
5. Split strategy for bundles over the asset limit (decided after WI2; blocking if any component exceeds).
6. Deferred carriers (`opentofu`, `bun`, `node*`): in scope for a v2? (recommended: opentofu and bun next).
7. Classify `bdock tools sources pin` as not an artifact build (recommended: yes, it writes a plan only).
8. Spec 026 tracks: one contract per component, or a multi-track contract (Spec 026 open question 3).
9. Typst: treat the embedded font as `source-offer` until the lawyer answers (decided by the coordinator).
