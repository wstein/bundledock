# Contract: interface between BundleDock and the mirror repository (SourceSteward Spec 026)

Read 2026-10-17 from `codex/026-release-process-v2` at `ddde764` (`specs/026-release-process/`; it moved from
`9a810a2` and `f7751e4` while this design was written, reviews caught it). It is a **draft**; its `dispatch`, `build` and
workflows wait for a constitution amendment. Statements about it are as read, not checked against any
implementation; re-read when it changes. The BundleDock side does not depend on it.

## What Spec 026 fixes (read)

| Spec 026 | Consequence for the mirror |
| --- | --- |
| D1 build once on commit C of `main` (`workflow_dispatch` inputs `commit`, `version`, and for multi-product repositories `product`, FR-025); gates: CI green on C, version greater than every tag and release of its product, no release exists | the product is the component; the version input is the `version id` |
| D1 follow-up commit D with `docs/releases/<product>/<version>.json`; the **owner pushes an annotated tag** on D; the tag workflow verifies and publishes only | each mirror release needs: approved build, merged D, owner tag push |
| D9, FR-025 one contract file per product, `.stw/release.<component>.toml`; tag grammar `<component>-v{version}`; products' literal tag prefixes must not be equal or prefix-related (`product-tag-overlap`) | components `zlib` and `zlib-v2` could not coexist; the catalog's package names must avoid prefix-related pairs (enforced by the WI1 tests: no `<a>-v` prefix of `<b>-v` among catalog names, and each name matches Spec 026's product id `^[a-z0-9][a-z0-9-]{0,31}$`; measured today on the 41 catalog names plus the nine PR #119 names: both hold) |
| FR-026 fetch builds: `network = "fetch"`, `network_hosts`, `lock_files` (required with `reproducibility.policy = "required"`, else `fetch-unpinned`); the build manifest records hosts and the SHA-256 of each lock file | `sources/<component>.lock.json` is the lock file; per-version plan files are not usable as `lock_files` (fixed paths) |
| targets `kind = "file"` with `os = "any"`, `arch = "any"` | the earlier open item about os/arch is answered |
| stw writes `SHA256SUMS` and `build-manifest.json` and attests | `fetch` writes neither |
| `oci-image` record field `source_offer` (string path or URL) | reference to the bundle's `source-check/v1` output |

## Adopted from worked example 5

For each component the mirror holds `.stw/release.<component>.toml` (written by tooling, JSON in practice):

- `[[build]]` `scope = "once"`, argv `bdock tools sources fetch --component <C> --version {version_number}
  --out {out_dir}` (placeholder spelling from the example), `network = "fetch"`, `network_hosts` listing every first-hop **and** redirect-target host of the
  [fetch hosts](../data-model.md#fetch-hosts) for the component's recipe kind (so a CI egress list does not break `fetch`), `lock_files =
  ["sources/<C>.lock.json"]`.
- Targets: `<C>-{version_number}-sources.tar.gz` and `<C>-{version_number}-sources.manifest.json`, both
  `kind = "file"`, `os`/`arch` = `any`, the second with `role = "manifest"`.
- `tag_template = "<C>-v{version}"`, `tag_pattern = '^<C>-v(?P<version>(?P<g1>[0-9]{1,9})(?:\.(?P<g2>[0-9]{1,9}))?(?:\.(?P<g3>[0-9]{1,9}))?(?:\.(?P<g4>[0-9]{1,9}))?(?:-r(?P<rev>[0-9]{1,9}))?)$'`, `order = ["g1","g2","g3","g4","rev"]` (the example has three groups and no revision; four groups and `rev` are our extension, checked against the Spec 026 text only). Spec 026 requires order groups to be `[0-9]{1,9}` (config.md, spec.md version table), so the pattern cannot exclude `-r0`, `-r1` or leading zeros itself. Those tags are therefore refused by `bdock tools sources plan --standalone` (mirror CI, before any dispatch) and by the owner's tag review; a GitHub tag ruleset can match only fnmatch-style name patterns, so it is a second line at best (unverified), not the enforcement.
- `bdock` is installed by the workflow setup (a pinned released `bdock` archive, verified by checksum, or
  built from the commit named in the entry's `bdock_source_commit` with its `bdock_go_version`; the workflow
  checks both before use).
- Mirror CI additionally runs `bdock tools sources plan --standalone` and, after the build,
  `bdock tools sources check --bundle-dir {out_dir}`.
- The release body is generated from the manifest (upstream URL and tag, licenses, entry digest).

## Remaining items for Spec 026

1. **Re-issue and backfill monotonicity.** "Greater than every existing version of the product" forbids a
   `-rN` of a non-newest version and a late backfill of an older version. Ask for a re-issue/backfill
   exemption, or accept the limitation recorded in the spec (retention).
2. Per-component contract files are generated; a mirror with many components has many files (about twenty
   after WI2 if the MPL-2.0 scale holds) against Spec 026's cap of 64 product files per repository (a ceiling to raise if WI2 exceeds it). Ask whether a generated multi-product index is intended.
3. `source_offer` reference form (string only).
4. Dispatch/build blocked until the constitution amendment is decided.
5. The `bdock` CLI release (a pinned archive) must exist before the first mirror build.

## BundleDock-side data crossing the boundary

Only: `sources/<component>.lock.json` (committed in the mirror), the two output files, and `source-check/v1`
JSON. No shared code.
