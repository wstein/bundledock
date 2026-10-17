# Spec: third-party source mirror for copyleft toolbox components

**Status**: draft design (docs only, no code). Review rounds are in [analysis.md](analysis.md).
Nothing has been built, published, tagged or pushed. Companion files: [plan](plan.md),
[research](research.md), [data model](data-model.md), contracts
([command](contracts/sources-command.md), [check](contracts/sources-check.md),
[mirror project contract](contracts/mirror-project.md)), [quickstart](quickstart.md),
[tasks](tasks.md), [requirements checklist](checklists/requirements.md).

This is not legal advice. Items for a lawyer or the Software Freedom Conservancy are listed in
[Legal caveat](#legal-caveat).

## Decision record (binding, owner)

Werner Stein decided option B with C-linking: BundleDock keeps its **own** public mirror
repository, `https://github.com/wstein/bundledock-third-party-sources` (the "mirror"; exists,
public, EUPL-1.2 for its own files, README and LICENSE only, wiki off), with **one immutable
GitHub Release per component version** carrying a **complete source bundle**, a manifest and
`SHA256SUMS`. Every `licenses/<tool>/SOURCE` shipped inside the image points to that release and
states the written offer. The OCI bundle release record links the mirror releases (it does not
re-attach the bundles). It applies retroactively to the existing copyleft components.

Ownership (coordinator proposal to the owner, treated as accepted unless the owner says
otherwise): **BundleDock produces** the bundles (`bdock tools sources plan|fetch|check`), because
only it knows which binary version and digest ships. **SourceSteward's release process
(Spec 026) publishes** them: it records, verifies, attests and promotes, language-agnostic, and
never decides what a bundle contains. Spec 026 is being written in parallel; every statement about
it here is **unverified and in flight** ([Dependencies](#dependencies-and-unverified-items)).

This document authorizes nothing outside source-level work: no image or artifact build, no push, no
tag, no release, no repository setting, no consumer activation (umbrella and repository `AGENTS.md`
unchanged).

## Why

The toolbox image redistributes unmodified upstream binaries. For copyleft licenses the
distributor must offer Corresponding Source. Today `SOURCE` files point only at upstream
(for example `licenses/fish/SOURCE` names the fish tarball URL; `licenses/hadolint/SOURCE` in PR
#119 says "relies on upstream availability"). If upstream deletes a tag, rewrites a release or
changes a GitHub-generated archive, our offer silently fails, and nothing proves we kept the
source. The mirror makes us, not upstream, the party who fulfils the offer.

## Scenarios

1. **Backfill.** The maintainer runs `bdock tools sources plan fish`, reviews the plan, the mirror
   CI runs `fetch` and the release process publishes `fish-v4.9.3` with a bundle, manifest and
   `SHA256SUMS`. `licenses/fish/SOURCE` is updated to name it.
2. **Upgrade.** An updater PR moves shellcheck to a new version. `bdock tools sources check` fails:
   no mirror release entry for `shellcheck` at that version. The bundle cannot be published until a
   human adds the mirror release (FR-12).
3. **Hostile tarball.** An upstream tarball contains `../../x`, a symlink to `/etc`, a 50 GiB
   member or a `Setup.hs`. Extraction refuses with a stable code; nothing is executed (FR-9).
4. **Upstream vanished.** A tarball URL answers 404 or a different SHA256. `fetch` refuses and
   nothing is published (FR-11). A bundle already published is unaffected.
5. **Recipient asks.** A user reads `licenses/trufflehog/SOURCE` inside the container, follows the
   mirror link, downloads the bundle, verifies `SHA256SUMS`, and has everything upstream
   needs to rebuild, with the recipe, without contacting us.

## Requirements

- **FR-1 Classification.** Every catalog package has a `source` block with an `obligation` from the
  [normative table](#obligation-classes). `none` is a claim that must be derivable from the
  retained licenses; a test scans `licenses/<tool>/` for copyleft markers and fails when a tool
  classified `none` retains a GPL/AGPL/LGPL/MPL text ([data model](data-model.md#catalog-source-block)).
- **FR-2 Mirror coverage.** For every package with `obligation != none` and every version present in
  the lock, the registry [`sources.lock.json`](data-model.md#sources-lock) names exactly one mirror
  release whose manifest `component` and `version` match. Enforcement is
  [`bdock tools sources check`](contracts/sources-check.md) in `make check`.
- **FR-3 Release identity.** Tag `<component>-v<version>` in the mirror, where `<component>` is the
  catalog package name and `<version>` the lock version without a leading `v`. A corrected re-issue
  of the same upstream version is `<component>-v<version>-r<N>` (`N >= 2`). One release per tag,
  immutable, never deleted or moved. Versions are monotonic per component (compared numerically on
  the dotted part, then `r<N>`).
- **FR-4 Bundle content.** Complete per the [completeness policy](#completeness-policy). The
  bundle is one asset `<component>-v<version>-source.tar.gz` plus `<component>-v<version>-manifest.json`
  and `SHA256SUMS`, all attached to the release.
- **FR-5 Manifest.** JSON, schema `bundledock.source-manifest/v1`
  ([data model](data-model.md#source-manifest)): component, version, upstream URL and tag (with
  peeled commit), license SPDX expressions, tarball SHA256s, dependency listing with hashes, the
  exact recipe (kind, tool versions, argument vector, fetch endpoints), archive settings and
  `SOURCE_DATE_EPOCH`.
- **FR-6 Deterministic production.** The bundle is a pure function of the plan and of the bytes
  fetched: sorted entries, uid/gid 0, empty names, fixed modes, mtime `SOURCE_DATE_EPOCH`, a
  declared tar format, `tar_sha256` of the uncompressed tar recorded next to the compressed asset
  digest ([research D6](research.md#d6-determinism-boundary)). Network is used only in `fetch`.
- **FR-7 Plan/fetch separation.** `plan` is offline and reads only catalog, lock and
  `sources.lock.json`; `fetch` is the only networked mode; `check` is offline. The fetch endpoints
  are an allow-list declared per recipe kind.
- **FR-8 Never execute upstream code.** No build script, `Setup.hs`, `build.rs`, `go generate`,
  `go build`, `cabal build`, `cargo build`, hook, filter or archive-embedded program runs. The only
  external programs `fetch` may start are none (all fetchers are in-process HTTP + archive code);
  `custom` recipes are refused in v1.
- **FR-9 Extraction safety.** Upstream tarballs are read by a bounded in-process reader
  ([limits table](#limits)); members are either re-emitted unchanged into the bundle or inspected
  (file lists, `Cargo.lock`, `go.sum`); nothing is extracted to disk outside a temp dir that only
  holds validated regular files.
- **FR-10 Publication path.** Mirror releases are published by SourceSteward's release process from
  a commit of the mirror repository ([contracts/mirror-project.md](contracts/mirror-project.md)):
  build (the fetch step) once, verify, promote; attestation per Spec 026. A release is never created
  by hand.
- **FR-11 Failure policy.** [Failure table](#failure-modes). Fail closed; never publish a partial
  bundle; never overwrite a release.
- **FR-12 Order on upgrade.** The mirror release for the new version must exist before any bundle
  containing that version is published. `check` enforces it on the lock; the publisher treats a
  failing `check` as a stop.
- **FR-13 SOURCE files.** Each copyleft tool's `SOURCE` keeps its existing upstream lines and adds
  the [mirror block](data-model.md#source-file-mirror-block): release URL, bundle URL, bundle and
  manifest SHA256 and the written offer text (valid while the binary is distributed and at least three
  years; BundleDock, not upstream, fulfils it). `SOURCE` ships inside the image (`licenses.tar.gz`,
  `--license licenses/<tool>/SOURCE`), which provides the "clear directions next to the object code"
  of GPL-3.0 section 6(d) ([legal caveat](#legal-caveat)).
- **FR-14 OCI record link.** The `oci-image` release record (Spec 026) lists the mirror release URL
  and manifest digest of every copyleft component of the bundle; it does not copy the bundles.
  `bdock tools sources check --json` emits that list ([contract](contracts/sources-check.md)).
- **FR-15 Retention.** Releases are never deleted. See [Retention](#retention).
- **FR-16 Retroactivity.** Components in already-published carriers are backfilled
  ([rollout](plan.md#rollout-order)).
- **FR-17 Documentation.** Owner procedure and repository settings are documentation only
  ([plan: repository settings](plan.md#mirror-repository-settings-for-the-owner)). No automation
  changes repository settings.

## Obligation classes

Normative: one table, referenced everywhere else.

| `obligation` | Meaning | Mirror release required |
| --- | --- | --- |
| `none` | permissive licenses only for the shipped object code (MIT, BSD, Apache-2.0, ISC, 0BSD, Unlicense, CC0) | no |
| `source-offer` | GPL-2.0, GPL-3.0, AGPL-3.0 or LGPL component in the object code | yes |
| `file-level-source` | MPL-2.0 (or EPL/CDDL) covering files in the object code | yes |
| `undetermined` | no review yet | blocks `check` for packages whose notices or dependencies could not be scanned (see FR-1); initial state of newly added tools |

### Classification of today's catalog (measured from `bundles/toolbox/licenses/`, 2026-10-17)

The measurement is a text scan of the retained license directories (40 directories on `origin/main`
1cf2173) plus the catalog; it is evidence, not an audit
([research R1](research.md#r1-classification-measurement)).

| Component | Evidence in the repository | Class |
| --- | --- | --- |
| fish 4.9.3 | `SOURCE`: GPL-2.0-only AND LGPL-2.0-or-later AND MIT AND PSF-2.0 | `source-offer` |
| shellcheck v0.11.0 | `SOURCE`, `LICENSE` GPL-3.0 | `source-offer` |
| trufflehog v3.99.2 | `SOURCE`: AGPL-3.0 | `source-offer` |
| helix 25.07.1 | `SOURCE`: MPL-2.0; `GRAMMAR-NOTICES` contains the GPL-3.0 text of the jinja2 grammar (`runtime/grammars/sources/jinja2/LICENSE.md`, measured in the upstream source tarball, which also ships `runtime/grammars/sources/jinja2`) | `source-offer` (GPL-3.0 grammar) and `file-level-source` (MPL-2.0); recorded as the stronger class `source-offer` with both licenses listed |
| hadolint v2.15.1 (PR #119, not on main) | `SOURCE` GPL-3.0-only; `ThirdPartyNotices.txt` lists GPL-2.0-only HsYAML, GPL-3.0-only ShellCheck, GPL-3.0-or-later language-docker, MPL-2.0 colourista | `source-offer` |
| all other packages | no GPL/AGPL/LGPL/MPL text in their retained notices | `none` **provisionally**: see below |

Provisional `none` is limited: Go and Rust binaries are static and may embed weak-copyleft
dependencies (for example MPL-2.0 Go modules) whose notices upstream did not ship. v1 therefore adds a
one-time review task ([WI2](tasks.md)) that scans each shipped binary's embedded dependency list
(`go version -m`, `cargo auditable`/syft SBOM where present) and records `basis: scanned` or
`basis: declared`. Until WI2 completes, those packages are `undetermined` in the catalog design and
`check` reports them as warnings (not failures) during rollout; the enforcement step
([rollout step 6](plan.md#rollout-order)) turns warnings into failures only for `source-offer`
and `file-level-source` gaps, and `undetermined` stays a warning with a count. Static system
libraries linked into Haskell binaries (GMP is LGPL-3.0-or-later or GPL-2.0-or-later) are a known
open item ([legal caveat](#legal-caveat)).

## Completeness policy

Corresponding Source here means, per GPL-3.0 section 1 and the equivalent text of GPL-2.0 section 3:
the source of the shipped object code, the scripts to control compilation and installation, and
what is needed to build it; excluding standard system libraries and general-purpose tools that
normally accompany the operating system and compiler. We do not rebuild or sign. We provide the
inputs; the manifest records the upstream recipe.

| Ecosystem (`recipe.kind`) | In the bundle | Excluded (and stated in the manifest `excluded`) |
| --- | --- | --- |
| `upstream-tarball-only` | upstream release tarball, byte-identical | nothing; used only when the tarball vendors or has no external dependencies, verified per component ([research R2](research.md#r2-what-upstream-tarballs-vendor)) |
| `cargo-vendor` | tarball; every crate named in `Cargo.lock` as `.crate` files from the registry with the lock's `checksum`; git dependencies as source archives of the locked commit; `Cargo.lock`; `rust-toolchain.toml` | the Rust toolchain, `std`, linker, C compiler, libc |
| `go-mod-download` | tarball; every module in `go.sum` as `.zip`, `.mod`, `.info` in module-cache layout fetched from the module proxy and verified against `go.sum` (`h1:` dirhash); `go.mod`, `go.sum`, `replace` targets | the Go toolchain and standard library |
| `cabal-freeze` | tarball; every Hackage package of the exact dependency set as `.tar.gz` plus its `.cabal` revision; a `cabal.project.freeze` reconstructed from the upstream dependency report or lock | GHC and its boot libraries (named, with versions, but sources not included, unless GHC boot libraries are copyleft); static C libraries (musl, GMP, ncurses, zlib, libffi): listed in `static_system_libs` for review, see legal caveat |
| `custom` | refused in v1 | n/a |

Scripts to control compilation: everything in the upstream tarball (`Cargo.toml`, `build.rs`, `Setup.hs`,
cabal files, `Makefile`, release workflows under `.github/`), plus `recipe/BUILD-NOTES.md` generated
from the manifest (toolchain versions as recorded by upstream workflows, quoted not executed).

Known measured facts that drive the policy ([research R2](research.md#r2-what-upstream-tarballs-vendor)):
none of the five upstream tarballs inspected vendors its dependencies (no `vendor/`; fish and helix
ship `Cargo.lock` only, trufflehog `go.sum` only, hadolint and shellcheck ship `.cabal` files and no
freeze file). Upstream builds hadolint with `cabal` against the live Hackage index on the release day
(`cabal.project.freeze` is produced in CI, not committed), so the exact set is known only from the
retained `ThirdPartyNotices.txt`: the bundle reconstructs the freeze from it and records that this is a
reconstruction ([risk](plan.md#risks)).

## Limits

Normative. Exceeding a limit is a refusal; limits are constants in code with a test per row.

| Limit | Value | Code |
| --- | --- | --- |
| members per upstream tarball | 200 000 | `archive-too-many-members` |
| single member size | 2 GiB | `archive-member-too-large` |
| total uncompressed size per tarball | 8 GiB | `archive-too-large` |
| path length | 4096 bytes, no NUL, no `..`, no absolute | `archive-unsafe-path` |
| link members | symlink/hardlink targets must stay inside the root | `archive-unsafe-link` |
| device, fifo, socket members | refused | `archive-special-member` |
| dependencies per component | 5 000 | `deps-too-many` |
| one release asset | 1.9 GiB (GitHub limit is 2 GiB, unverified here) | `bundle-too-large` |
| HTTP redirects / response size | 5 / limit of the member above | `fetch-redirect-limit`, `fetch-too-large` |

## Failure modes

Normative. All are exit status 1 unless stated; `fetch` publishes nothing on any of them.

| Code | When | Result |
| --- | --- | --- |
| `upstream-missing` | 404/410 for a tarball or dependency | refuse; if the bundle already exists nothing changes; the maintainer chooses a documented alternate source and a new `-rN` plan |
| `upstream-hash-mismatch` | observed SHA256 differs from the plan | refuse; never auto-update the plan |
| `dependency-hash-mismatch` | module/crate/package checksum differs from `go.sum` / `Cargo.lock` / Hackage index | refuse |
| `plan-unpinned` | plan has no `sha256` for the tarball | refuse unless `--accept-new-digest` (prints observed digests; a human commits them) |
| `release-exists` | mirror tag already has a release | refuse (publisher), never overwrite |
| `manifest-schema` | manifest fails the schema | refuse |
| `network-disabled` | `fetch --offline` or env guard | exit 3 (usage) |
| `bundle-too-large` | over the asset limit | refuse; split strategy is future work ([open question 5](plan.md#open-questions-for-the-owner)) |

Upstream vanished **after** we published: nothing to do; the mirror release is the source of truth.
Upstream vanished **before** we mirrored a shipped version: the bundle cannot be made from upstream;
recover from another copy whose SHA256 equals the lock's recorded digests (the shipped binary's
SHA256 is in the lock, the source tarball's digest is in `sources.lock.json` only if previously
pinned: this is why pinning happens at lock time for new tools, [WI7](tasks.md)).

## Retention

Releases and tags in the mirror are never deleted, edited or moved; a wrong bundle is superseded by
`-rN` and the wrong release gets a note (release body edits are the one allowed change, because
immutable releases lock assets and tags, unverified for body edits). Retention is for as long as any
published bundle containing the version may be distributed and at least three years after the last
distribution. Also never delete the mirror repository; if it must move, redirect via GitHub transfer
and update `SOURCE` files in the next bundle. Quay and the images' own retention behavior is
unverified here ([Unverified](#dependencies-and-unverified-items)).

## Threat model

| Threat | Control |
| --- | --- |
| Hostile upstream tarball (path traversal, link tricks, decompression bomb, device files) | bounded in-process reader, [limits table](#limits), no extraction to a user-visible tree, re-emission through our own writer with normalized headers |
| Upstream code execution through the build tooling | FR-8; no `go`, `cargo`, `cabal` invocation; fetchers speak HTTP and parse lock files only |
| Poisoned dependency (a proxy serving other bytes) | every dependency verified against an independent checksum (`go.sum` h1, `Cargo.lock` checksum, Hackage package hash in the reconstructed freeze index) |
| Malicious plan change | plan is committed in the mirror repo and reviewed; fetch refuses digests not in the plan |
| Substituting a released asset | immutable releases, `SHA256SUMS`, attestation by the publisher, hashes also in the image's `SOURCE` and in the OCI record |
| Secrets exposure | `fetch` needs no token for public endpoints; GitHub token only for the publisher, handled by Spec 026 |
| Parser DoS (`Cargo.lock`, `go.sum`, `.cabal`, `ThirdPartyNotices.txt`) | size cap 16 MiB per parsed file, line cap, no regex backtracking constructs (RE2/Go) |

## Legal caveat

This design is engineering policy, not legal advice. To ask a lawyer or the Software Freedom
Conservancy before relying on it:

1. Does the mirror release link plus the `SOURCE` offer inside the image satisfy GPL-3.0 section 6(d)
   and GPL-2.0 section 3(b) for the image as a whole, including the "at least three years" wording
   and distribution of the same binary through other channels (a user who copies the volume)?
2. GPL-2.0-only HsYAML and GPL-3.0-only ShellCheck/language-docker inside one hadolint binary:
   compatibility of the combined work (PR #119 owner acceptance, 2026-10-17, is a business decision,
   not a legal conclusion).
3. Statically linked LGPL components in Haskell/Rust binaries (GMP): relinking ability and whether
   "system library" exclusion applies to libraries we ship inside the object code.
4. AGPL-3.0 (trufflehog): network-use clause is not triggered by a local CLI, but confirm.
5. Whether MPL-2.0 file-level source (helix) needs more than a link, and the GPL-3.0 jinja2
   grammar inside helix's runtime directory (separate work, aggregate or part of the work).
6. Whether a mirror in a different repository than the image's registry counts as "same place".
7. Corresponding Source for binaries built by upstream we only repackage: is upstream's recipe plus
   the dependency set enough when upstream built against an unfrozen index?

## Dependencies and unverified items

Dependencies:

- **SourceSteward Spec 026** (branch `codex/026-release-process-v2` in the parallel worktree): at the
  time of writing, that branch carries `specs/026-release-hygiene/` (release readiness for the `stw`
  CLI); the "release process v2 / G1 / artifact kinds `file` and `oci-image`" text referred to in the
  brief was **not present** in the branch or in `origin/main` of SourceSteward when read (2026-10-17).
  All Spec 026 statements here are therefore requirements this design places on it, not facts about
  it. Alignment points are in [contracts/mirror-project.md](contracts/mirror-project.md). Nothing in
  BundleDock depends on Spec 026 to ship `plan`, `fetch`, `check` or the catalog changes; only
  step "publish through the release process" does.
- PR #119 (hadolint) and the `bdock` CLI release workflow (a tagged `bdock` is needed if the mirror CI
  runs a released binary instead of building the pinned source commit).

Unverified, deliberately:

- Exact Quay behavior (tag retention, manifest deletion, mirroring, rate limits).
- Immutable-release semantics on `github.com` for this repository (release body edits, asset limit
  1.9/2 GiB, attestation display); the owner enables and observes them.
- `cargo vendor` size and timing (no `cargo` in the authoring environment); crate fetching rules are
  designed from `Cargo.lock` only.
- Byte-equality of gzip output across Go versions ([D6](research.md#d6-determinism-boundary)).
- That the Hackage versions in `ThirdPartyNotices.txt` equal the set upstream actually linked.
- How many copyleft versions exist in already-published carriers (counted in WI9 from
  `docs/toolbox-published.json` and the lock history, not here).
