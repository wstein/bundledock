# Spec: third-party source mirror for copyleft toolbox components

**Status**: draft design (docs only, no code). Review rounds are in [analysis.md](analysis.md).
Nothing has been built, published, tagged or pushed. Companion files: [plan](plan.md),
[research](research.md), [data model](data-model.md), contracts
([command](contracts/sources-command.md), [check](contracts/sources-check.md),
[mirror project](contracts/mirror-project.md)), [quickstart](quickstart.md),
[tasks](tasks.md), [requirements checklist](checklists/requirements.md).

This is not legal advice. Legal questions are collected in [Legal caveat](#legal-caveat); no
requirement below asserts that a legal obligation is satisfied.

## Decision record (binding, owner)

Werner Stein decided option B with C-linking: BundleDock keeps its **own** public mirror
repository, `https://github.com/wstein/bundledock-third-party-sources` (the "mirror"; exists, public,
EUPL-1.2 for its own files, README and LICENSE only, wiki off), with **one immutable GitHub Release per
component version** carrying a **complete source bundle**, a manifest and checksums. Each copyleft
tool's retained notices in the image point to that release and state the written offer. The OCI
bundle release record links the mirror releases (it does not re-attach the bundles). The mirror
covers the existing copyleft components retroactively as far as **source availability** goes
([FR-16](#requirements)).

Ownership (coordinator proposal, treated as accepted unless the owner says otherwise): **BundleDock
produces** the bundles (`bdock tools sources pin|plan|fetch|check`), because only it knows which binary
version and digest ships. **SourceSteward's release process (Spec 026) publishes** them: it
records, verifies, attests and promotes, language-agnostic, and never decides what a bundle
contains. The mirror is multi-component (tag prefix per component) and its `build` is the fetch step.

This document authorizes nothing outside source-level work: no image or artifact build, no push, no
tag, no release, no repository setting, no consumer activation (`AGENTS.md` unchanged).

## Scope

v1 covers the **toolbox bundle only** (`bundles/toolbox`, 41 catalog packages). Other carriers contain
copyleft or weak-copyleft text (measured by grep of `bundles/*/licenses`) and are **deferred, owner
decision**:

| Carrier | Evidence | Status |
| --- | --- | --- |
| `opentofu` | `licenses/opentofu/SOURCE`: "License: Mozilla Public License 2.0 (MPL-2.0)" | deferred, owner decision |
| `bun` | `licenses/bun/LICENSE.md`: statically linked JavaScriptCore/WebKit under LGPL-2, relinking text | deferred, owner decision |
| `node`, `node-current` | `licenses/node/LICENSE` contains GPL text (7 matches of "GNU General" in `node`); whether GPL components are in the shipped runtime is unverified | deferred, owner decision |
| `code-server`, `codex`, `claude`, `agy` | not examined | deferred |

The component model (`<component>-v<version>`) and the registry shape do not preclude them; each would
add a catalog-equivalent `source` block in its own bundle config.

## Why

The toolbox image redistributes unmodified upstream binaries. For copyleft licenses the distributor
may owe recipients Corresponding Source or an offer for it. Today `SOURCE` files point only at upstream
(`licenses/fish/SOURCE` names the fish tarball URL; `licenses/hadolint/SOURCE` in PR #119 says "relies
on upstream availability"). If upstream deletes a tag, rewrites a release or changes a GitHub-generated
archive, that pointer silently fails, and nothing proves we kept the source. The mirror makes BundleDock
hold the bytes.

## Scenarios

1. **Backfill.** The maintainer pins and plans `fish` (`bdock tools sources pin fish`), reviews the plan
   in a mirror PR; on the owner's approval the release process builds (the fetch step) and, after the
   owner pushes the tag, publishes `fish-v4.9.3`. The generated `licenses/fish/SOURCE-MIRROR` names it.
2. **Upgrade.** An updater PR moves shellcheck. `bdock tools sources check` fails
   (`mirror-entry-missing`). The [upgrade sequence](plan.md#upgrade-sequence) has a human step; the bundle
   cannot be published until the mirror release exists.
3. **Hostile tarball.** An upstream tarball holds `../../x`, an absolute symlink, duplicate members, or a
   decompression bomb. The tarball is stored unextracted and byte-identical; only resource limits refuse
   ([limits](#limits)); nothing is executed or extracted to disk.
4. **Upstream vanished.** A URL answers 404 or a different SHA256. `fetch` refuses
   (`upstream-missing`, `upstream-hash-mismatch`) and nothing is published.
5. **Recipient asks.** A user reads `licenses/trufflehog/SOURCE` and `SOURCE-MIRROR` inside the container,
   follows the mirror link, downloads the bundle, verifies hashes, and has the inputs listed by the manifest.

## Requirements

- **FR-1 Classification.** Every catalog package of the bundle has a `source` block with an
  `obligation` from the [obligation table](#obligation-classes). The evidence set for a package is the
  full retained-notice set: files in `licenses/<package>/` (absent for `fd` and `rg`) **plus** the
  `install.archive_notices` members of the locked artifacts (21 of 41 packages take notices from the
  upstream archive at build time) **plus** the dependency scan of WI2. A package whose evidence is
  incomplete is `undetermined`. A test fails when a package classified `none` has a copyleft marker in
  the repository-visible part of its evidence. The marker set is normative, case-insensitive substrings:
  `GNU General Public`, `GNU Lesser General Public`, `GNU Library General Public`, `GNU Affero`, `GPL-`,
  `GPL3`, `GPLv`, `LGPL`, `AGPL`, `Mozilla Public License`, `MPL-`, `Eclipse Public License`, `EPL-`,
  `CDDL`; the test also has a negative fixture (warranty-disclaimer text must not match). Packages not in the catalog (`licenses/xz`) are ignored.
- **FR-2 Mirror coverage.** For every package with `obligation` `source-offer` or `file-level-source`
  and every locked version, [`sources.lock.json`](data-model.md#sources-lock) names a mirror release
  whose manifest `component` and `version` match. [`bdock tools sources check`](contracts/sources-check.md)
  enforces it in `make check`.
- **FR-3 Naming.** One normative [naming table](#naming-table). Versions are numeric dotted groups only;
  other shapes refuse (`version-shape`) until the catalog declares a `version_regex` that yields a numeric shape. Measured lock versions: `lychee-v0.24.2` and `jq-1.8.2` need a regex; `v3.7c` (tmux) cannot be mirrored until a letter-suffix rule exists.
- **FR-4 Bundle content.** Complete per the [completeness policy](#completeness-policy). Release assets
  (the release process adds its own checksum and build manifest, FR-10): the bundle and the source manifest.
- **FR-5 Manifest.** JSON, schema `bundledock.source-manifest/v1` ([data model](data-model.md#source-manifest)).
- **FR-6 Deterministic production.** The bundle is a pure function of the plan and the bytes fetched:
  sorted entries, uid/gid 0, fixed modes, mtime `SOURCE_DATE_EPOCH`, declared tar format; the recorded
  `argv` is the **normalized** command (`bdock tools sources fetch --component C --version N`, without the output path); the Go
  toolchain used to build `bdock` is pinned in the plan (`bdock_go_version`). The reproducibility claim is
  for `tar_sha256`; the compressed digest is recorded separately ([research D6](research.md#d6-determinism-boundary)).
- **FR-7 Modes.** `pin` (network, read-only on the repository, writes a source-lock entry), `plan` (offline, validates and
  renders an entry against catalog and lock, or standalone for mirror CI), `fetch` (network, produces the bundle), `check` (offline).
  Per-mode effects are the table in the [command contract](contracts/sources-command.md#effects).
- **FR-8 Never execute upstream code.** No build script, `Setup.hs`, `build.rs`, `go generate`, `go
  build`, `cabal`, `cargo`, `go` or other process is started by `pin`, `plan`, `fetch` or `check`; fetchers
  speak HTTPS and parse lock files in-process. `recipe.kind: custom` is refused (`recipe-custom`).
- **FR-9 No extraction.** The upstream tarball is stored byte-identical (it is a release asset input) and
  is read only as a stream in memory to list members and to parse named regular files (`Cargo.lock`,
  `go.sum`, `go.mod`, `*.cabal`, the dependency report). Nothing is written to disk from inside it.
  Links, special members and duplicate member names do not refuse (real tarballs contain them,
  [research R2](research.md#r2-what-upstream-tarballs-vendor)); they are listed in the manifest
  `archive_anomalies`. For a duplicated name the last member wins, as for `tar`. Only resource
  [limits](#limits) refuse. The same no-extraction, in-memory, limited rule applies to the dependency
  archives the fetchers read (module zips for `h1:`, `.crate` files, git archives, Hackage sdists), and any
  bundle entry path built from lockfile content (module path, crate name, git URL, package name) is
  validated against `^[A-Za-z0-9._~@+/-]+$`, no `..` segment, no leading `/`, at most 512 bytes
  (`dep-unsafe-path`).
- **FR-10 Publication path.** Mirror releases are published by SourceSteward's release process
  ([contracts/mirror-project.md](contracts/mirror-project.md)); a release is never created by hand.
- **FR-11 Failure policy.** [Failure table](#failure-modes). Fail closed; no partial bundle; no
  overwrite.
- **FR-12 Order on upgrade.** The mirror release for a new version must exist before any bundle
  containing that version is published. The offline `check --enforce` only sees the hand-edited registry
  entry; the rule is made machine-checkable by `check --online` (read-only GET of the release: published
  not draft, asset digests equal the registry, manifest `component` and `version` match), which the bundle
  publication gate runs.
- **FR-13 Notices in the image.** Each `source-offer` / `file-level-source` tool's `SOURCE` gets a
  **version-independent** paragraph (mirror repository URL, the tag rule of the [naming table](#naming-table),
  the written-offer text). Version-specific data (tag, asset URL, SHA256s) live in a **generated**
  `licenses/<tool>/SOURCE-MIRROR` derived from `sources.lock.json` by `bdock tools sources sync` and
  verified by `check`, listed in the catalog `notices`. This keeps `SOURCE` a reviewed snapshot as `AGENTS.md`
  requires ("historical license snapshots retain their reviewed source identity ... independently of
  current binary pins; manifests and locks describe the installed artifacts"): `SOURCE-MIRROR` describes
  the installed artifacts like the lock does. Both files ship in the image
  (`licenses.tar.gz`). Whether these texts meet any license's form-of-offer rule is a
  [lawyer item](#legal-caveat), not a claim of this design.
- **FR-14 OCI record link.** The `oci-image` record (Spec 026) carries a `source_offer` reference
  (a string, path or URL, field exists in Spec 026) pointing at the `bundledock.source-check/v1`
  `releases[]` output for the bundle's lock ([contract](contracts/sources-check.md#json)).
- **FR-15 Retention.** [Retention](#retention): never delete.
- **FR-16 Retroactivity.** Backfill covers **source availability** for every copyleft version found in
  published carriers (WI9 counts them). It does not change already-published images: their `SOURCE`
  files are immutable and keep upstream-only offers. Backfill order is **oldest version first per
  component** so monotonic ordering holds, and the complete WI9 list for a component is a hard gate before
  that component's first release ([plan](plan.md#rollout-order)).
- **FR-17 Documentation.** Owner procedure and repository settings are docs only
  ([plan](plan.md#mirror-repository-settings-for-the-owner)).

## Naming table

Normative; the source-lock path, assets, tag, registry key and URL templates derive from it.

| Concept | Rule | Example |
| --- | --- | --- |
| `component` | catalog package name | `shellcheck` |
| lock version `LV` | exactly as in `packages.lock.json` | `v0.11.0`, `25.07.1` |
| `V` | derived from `LV` by the package's `source.version_regex` (one capture group; default `^v?([0-9]+(\.[0-9]+){0,3})$`); the result must match `^[0-9]+(\.[0-9]+){0,3}$` or the package is blocked (`version-shape`) | `0.11.0` |
| revision `R` | integer `>= 1`; a corrected re-issue of the same `V` | `2` |
| `release tag` | `<component>-v<V>` for `R=1`, `<component>-v<V>-r<R>` for `R>=2` | `shellcheck-v0.11.0-r2` |
| `version id` | `<V>` for `R=1`, `<V>-r<R>` for `R>=2` (Spec 026 `{version_number}`) | `0.11.0-r2` |
| assets | `<component>-<version id>-sources.tar.gz` and `<component>-<version id>-sources.manifest.json` (the names in Spec 026 worked example 5); the release process adds `SHA256SUMS` and `build-manifest.json` | `shellcheck-0.11.0-sources.tar.gz` |
| source lock (mirror repo) | one file per component, `sources/<component>.lock.json`, holding an entry per `version id` ([data model](data-model.md#source-lock-and-plan-entries)) | |
| registry key | `component` -> `LV` -> `revisions[]` (see [data model](data-model.md#sources-lock)) | |
| ordering (per component) | numeric tuple `(groups of V..., R)`, missing groups sort below present ones | `0.11.0 < 0.11.0-r2 < 0.11.1` |
| dispatch `version` input (Spec 026) | the `version id`; the product (`<component>`) is a separate Spec 026 input and the tag is `<component>-v<version id>` | |

## Obligation classes

| `obligation` | Meaning | Mirror release required |
| --- | --- | --- |
| `none` | all evidence shows permissive licenses only | no |
| `source-offer` | GPL, AGPL or LGPL component in the object code (or in assets that the binary embeds) | yes |
| `file-level-source` | MPL-2.0 (or EPL/CDDL) files in the object code, no stronger license | yes |
| `undetermined` | evidence incomplete or not reviewed | no, but `check` reports it as a **warning** with a count (never an error); enforcement of `undetermined` is a future decision |

### Classification of today's catalog (measured 2026-10-17 on `origin/main` 1cf2173)

| Component | Evidence | Class |
| --- | --- | --- |
| fish 4.9.3 | `SOURCE`: GPL-2.0-only AND LGPL-2.0-or-later AND MIT AND PSF-2.0 | `source-offer` |
| shellcheck v0.11.0 | `licenses/shellcheck/LICENSE` is GPL-3.0 | `source-offer` |
| trufflehog v3.99.2 | `SOURCE`: AGPL-3.0 | `source-offer` |
| helix 25.07.1 | `SOURCE` MPL-2.0; `GRAMMAR-NOTICES` carries copyleft license texts for the grammars jinja2, amber, blueprint, hare (GPL-3.0 per round-2 reviewer), prolog (AGPL-3.0), vala (LGPL), inko and nim (MPL-2.0), and a GPL text for nginx; the shipped aarch64 tarball (SHA256 equals the lock) contains `runtime/grammars/{jinja2,amber,gas,blueprint,hare,prolog,vala,inko,nim}.so` (nginx and the exact SPDX per grammar not verified; WI2 resolves them) | `source-offer` (GPL/AGPL/LGPL grammars, plus MPL-2.0) |
| typst v0.15.1 | `SOURCE`: the embedded NewCM10-Regular font is "GPL3-or-later with Font and Distribution exceptions"; `ASSETS-NOTICE` holds the GPL-3.0 text | `source-offer` pending lawyer ([legal item 6](#legal-caveat)); the font exceptions may remove the source obligation |
| hadolint v2.15.1 (PR #119, not on main) | GPL-3.0-only; `ThirdPartyNotices.txt`: HsYAML GPL-2.0-only, ShellCheck GPL-3.0-only, language-docker GPL-3.0-or-later, colourista MPL-2.0, semver MPL-2.0 | `source-offer` |
| actionlint, cosign, difftastic, git-cliff, jsonschema-cli, pinact, zizmor, zoxide (PR #119) | not scanned | `undetermined` |
| all other packages | repository-visible notices have no copyleft marker, but the evidence is incomplete ([FR-1](#requirements)) | `undetermined` |

Scale risk, **reported by the round-1 reviewer and not re-measured by the author**: `Cargo.lock` at the
locked tags lists MPL-2.0 crates (`option-ext` in uv, ruff, jj, nu, starship, lychee, delta, mise, bottom,
lsd, just, watchexec, ast-grep and typst; `colored`, `uluru`, `cssparser`/`selectors` in several) and
`go.mod` lists MPL-2.0 HashiCorp modules for gh, trivy, syft, hugo, gitleaks, git-lfs, yq and trufflehog.
Whether each is linked into the shipped binary is unverified. If so, about twenty more tools become
`file-level-source` and the mirror grows accordingly. Therefore **WI2 (a measurement and classification
work item) precedes the backfill list, the bundle size limit and the enforcement flip**: those three
depend on WI2's recorded result ([plan](plan.md#rollout-order)).

## Completeness policy

Corresponding Source here is the working definition of this design (an engineering reading of GPL-3.0
section 1, not a legal conclusion): the source of the shipped object code, the scripts that control
compilation and installation, and what is needed to build it, excluding standard system libraries and
general-purpose tools that normally accompany the operating system and compiler. We do not rebuild or
sign; we keep the inputs and record the upstream recipe.

| `recipe.kind` | In the bundle | Excluded (copied to manifest `excluded`) |
| --- | --- | --- |
| `upstream-tarball-only` | the tarball, byte-identical | nothing; allowed only when the tarball is verified self-contained in WI2 (none of today's five is) |
| `cargo-vendor` | tarball; every `registry` crate of `Cargo.lock` as `.crate` verified against the lock `checksum`; every git source as the archive of the locked commit (pinned by digest, see [verification classes](#verification-classes)); `Cargo.lock`; `rust-toolchain.toml` if present | Rust toolchain, `std`, linker, C compiler, libc |
| `go-mod-download` | tarball; for every `go.sum` line with a zip `h1:` the module `.zip` and `.mod`, and for `/go.mod`-only lines the `.mod`; module-cache layout; `go.mod`, `go.sum`, `replace` targets (module paths come from `go.mod`) | Go toolchain and standard library (the `toolchain` line is recorded) |
| `cabal-freeze` | tarball; the Hackage sdist (`.tar.gz`, containing the original `.cabal`) of every package in the dependency set; a reconstructed `cabal.project.freeze`; `reconstruction: true` in the manifest | GHC and boot libraries (named, versions listed), static C libraries (listed in `static_system_libs`; legal item 3) |
| `custom` | refused | |

Dependency-set sources for `cabal-freeze`, in this order of preference: a committed freeze file for the
exact target; a dependency report shipped in the tarball (hadolint: `ThirdPartyNotices.txt`, 126 table
lines); otherwise the component is **not eligible** for `cabal-freeze` and stays a stated best-effort
item. Measured: shellcheck v0.11.0 has no freeze file for `linux.x86_64` (only
`builders/linux.armv6hf/cabal.project.freeze`) and its `builders/linux.x86_64/build` runs `cabal update`
and an unfrozen build, so shellcheck needs a decision: use the armv6hf freeze as a documented
approximation, or add an owner-reviewed freeze file to the plan (`recipe.freeze_file`) with
`reconstruction: true` ([open question 3](plan.md#open-questions-for-the-owner)). Which `.cabal` revision
upstream used cannot be determined from these inputs; the manifest stores the sdist's original `.cabal` and
the list of revision numbers observed on Hackage at `pin` time (`cabal_revisions`), marked unverified
equivalence.

Known facts from [research R2](research.md#r2-what-upstream-tarballs-vendor): none of the five inspected
tarballs vendors its dependencies.

## Verification classes

Normative; each dependency in the manifest carries one of these in `verified_against`.

| Class | Meaning | Ecosystems |
| --- | --- | --- |
| `lockfile-hash` | independent hash inside upstream's lock file | Go (`go.sum` `h1:`), Cargo registry crates (`Cargo.lock` `checksum`) |
| `plan-pin` | digest recorded in the reviewed plan at `pin` time (trust on first use, human reviewed); no independent check | Cargo git sources (fish has 8 git packages from github.com and codeberg.org), Hackage sdists, `/go.mod`-only lines recorded by file hash |

The threat model states this honestly: `plan-pin` protects against later substitution, not against a
hostile first fetch.

## Limits

Normative constants in code; each row has a test. Status 1 = refusal exit.

| Limit | Value | Code | Status |
| --- | --- | --- | --- |
| members listed per upstream tarball | 200 000 | `archive-too-many-members` | 1 |
| size of a parsed member (`Cargo.lock`, `go.sum`, `*.cabal`, report) | 16 MiB | `parse-too-large` | 1 |
| total uncompressed bytes read from one tarball | 8 GiB | `archive-too-large` | 1 |
| xz dictionary size / gzip expansion ratio | 256 MiB / 1000:1 | `archive-bomb` | 1 |
| path length of a listed member | 4096 bytes | `archive-unsafe-path` (refused only when the path is used as a parse key) | 1 |
| dependencies per component | 5 000 | `deps-too-many` | 1 |
| one release asset | 1.9 GiB provisional (WI2 confirms; GitHub documents 2 GiB per asset, unverified here) | `bundle-too-large` | 1 |
| HTTP redirects per request / hosts | 5 / [per-hop host allow-list](data-model.md#fetch-hosts) | `fetch-redirect-limit`, `fetch-host-refused` | 1 |
| single response body | 2 GiB for tarballs and zips; 16 MiB for JSON | `fetch-too-large` | 1 |
| `<tag>-sources.manifest.json`, in-bundle `MANIFEST.json`, source-lock file read by `check` | 16 MiB / 16 MiB / 1 MiB, also line count 1 000 000 | `parse-too-large` | 1 |
| `archive_anomalies` entries | 1 000 recorded; further ones only counted (`anomalies_truncated: N`), never a refusal | n/a | n/a |
| members read from one dependency archive (module zip, `.crate`, git archive, sdist) | same limits as the first three rows, per archive | `archive-too-many-members`, `archive-too-large`, `archive-bomb` | 1 |
| lockfile-derived entry path | see FR-9 | `dep-unsafe-path` | 1 |

There is no `--max-bytes` flag; limits are constants.

## Failure modes

Normative; exit status 1 unless stated; `fetch` publishes nothing on any of them.

| Code | When | Result |
| --- | --- | --- |
| `upstream-missing` | 404/410 for a tarball or dependency | refuse; existing releases unaffected |
| `upstream-hash-mismatch` | observed SHA256 differs from the plan | refuse; plan never auto-updated |
| `dependency-hash-mismatch` | differs from the `lockfile-hash` source or the plan pin | refuse |
| `plan-unpinned` | plan lacks a digest | refuse; `pin` is the only writer of digests |
| `plan-stale` | plan disagrees with catalog or lock | refuse (`plan`, `fetch`) |
| `lock-arch-mismatch` | amd64 and arm64 lock versions differ | refuse |
| `version-shape` | `V` does not match the naming table | refuse |
| `recipe-custom` | recipe kind `custom` | exit 3 |
| `out-not-empty` | `--out` not empty | refuse |
| `manifest-schema` | schema violation | refuse |
| `network-disabled` | `BDOCK_SOURCES_OFFLINE=1` set for `pin`, `fetch` or `check --online` | exit 3 |
| `tool-not-eligible` | `pin`/`fetch` for a package whose obligation is `none` or `undetermined`, or unknown tool | exit 3 |
| `tag-unresolved` | `fetch --component/--version` finds no entry in `sources/<component>.lock.json` | refuse |
| `notice-drift` | `sync --check` finds `SOURCE-MIRROR` different from the generated text | refuse |
| `bundle-hash-mismatch`, `bundle-file-missing`, `bundle-file-extra`, `bundle-manifest-mismatch`, `sums-mismatch` | `check --bundle-dir` tamper cases (changed byte; entry listed but absent; entry present but unlisted; standalone manifest and `MANIFEST.json` disagree; `SHA256SUMS` disagrees or lists other files). `SHA256SUMS` and the release process's `build-manifest.json` are allowed extras | refuse |
| `release-not-published`, `release-hash-mismatch` | `check --online`: release missing or draft; asset digests differ from the registry | refuse |
| `release-exists` | publisher found the tag | refused by the publisher (Spec 026 gate), not by bdock |

Upstream vanished before we mirrored a shipped version: the bundle cannot be made from upstream; recover
from another copy whose digest equals the one in the plan if a plan exists, otherwise record the gap.
Pinning at lock time for every new copyleft tool avoids this ([WI7](tasks.md)).

## Retention

Releases and tags in the mirror are never deleted, edited or moved. A wrong bundle is superseded by a new
revision (`-rN`) **only while no greater version of the component has been released**, because Spec 026
requires each new version to exceed all existing ones of its product. After that a wrong bundle cannot be
superseded under the current monotonicity rule; this is a stated limitation and a request to Spec 026 for a
re-issue exemption (plan open question 8). A superseded release stays and its registry entry is marked
`superseded`. Retention
lasts as long as any published bundle containing the version may be distributed and at least three years
after the last distribution (a policy of this design; the legal duration is a lawyer item). The mirror
repository is never deleted; if it must move, GitHub transfer keeps URLs working and the next bundle's
notices change. Quay retention is unverified.

## Threat model

| Threat | Control |
| --- | --- |
| Hostile upstream tarball (traversal, links, duplicates, bombs, special files) | no extraction ([FR-9](#requirements)); in-memory streaming with [limits](#limits); the stored blob is untouched |
| Upstream code execution through build tooling | [FR-8](#requirements); no `go`, `cargo`, `cabal` |
| Poisoned dependency | [verification classes](#verification-classes); `plan-pin` is trust on first use |
| Malicious plan change | plan is a reviewed file in the mirror repo; `fetch` refuses digests not in the plan |
| Redirect to an unexpected host | per-hop allow-list ([fetch hosts](data-model.md#fetch-hosts)) |
| Substituting a released asset | immutable releases, release-process checksums and attestation, hashes in `SOURCE-MIRROR` and the OCI record |
| Secrets or identity leakage in requests | no token for public endpoints; fixed User-Agent `bundledock-source-mirror/<bdock version>`; no user identifiers in any header |
| Parser DoS | size and line caps; Go RE2 only |

## Legal caveat

This design is engineering policy, not legal advice. Questions for a lawyer or the Software Freedom
Conservancy:

1. Do a link plus a written offer in the image's notices satisfy the form-of-offer rules (GPL-3.0
   section 6(b) versus 6(d), GPL-2.0 section 3), including whether the offer must be to "any third
   party", whether the three-year minimum and the "as long as distributed" wording are right, and
   distribution through other channels (a copied volume)?
2. A GPL-2.0-only dependency (HsYAML) combined with GPL-3.0-only code inside one hadolint binary:
   compatibility. (The PR #119 owner acceptance of 2026-10-17 is a business decision, not a legal
   conclusion.)
3. Statically linked LGPL components in Haskell and Rust binaries (GMP, and in bun JavaScriptCore): relinking
   ability, and whether the "system library" exclusion covers libraries we ship inside the object code.
4. AGPL-3.0 (trufflehog, and the AGPL-3.0 prolog grammar shipped in helix): scope of the network-interaction clause for how the tools are used in the toolbox.
5. MPL-2.0 file-level source: is a link enough, and what does the scale finding imply?
6. typst's embedded GPL-3.0-or-later font with Font and Distribution exceptions, and helix's GPL-3.0 jinja2
   grammar `.so` in its runtime directory: separate work or part of the program?
7. Is a mirror in a different repository than the image's registry acceptable as the place of the source?
8. Corresponding Source for binaries built by upstream that we only repackage: is upstream's recipe plus a
   reconstructed dependency set (hadolint, shellcheck) enough?

## Dependencies and unverified items

- **SourceSteward Spec 026** (read at `codex/026-release-process-v2` `f7751e4`,
  `specs/026-release-process/`, 2026-10-17; it moved from `9a810a2` during this design): kinds `file` and
  `oci-image` (record field `source_offer`), build once and promote, products with one contract file each
  (D9, FR-025), fetch builds with `network = "fetch"`, `network_hosts` and `lock_files` (FR-026), `os`/`arch`
  `any`, worked example 5 for this mirror. It is a **draft**; `dispatch`, `build` and workflows wait for a
  constitution amendment (its own notice). The interface and the three points where this design adopts
  its example 5 (argv, asset names, lock file layout) are in [contracts/mirror-project.md](contracts/mirror-project.md).
  The bundledock-side work (`pin`, `plan`, `fetch`, `check`, catalog) does not depend on it.
- PR #119 (hadolint) and the `bdock` CLI release workflow (a released `bdock` or a pinned source commit
  is needed by the mirror CI).

Unverified, deliberately: Quay behavior; GitHub immutable-release semantics and asset size limit;
`cargo vendor`/`cabal` behavior (neither available while authoring); gzip byte-stability across Go
versions; whether Hackage versions in `ThirdPartyNotices.txt` equal what upstream linked; the MPL-2.0
scale finding above; whether Hackage serves historical revisions by index-state; the count of
published carriers per copyleft version (WI9).
