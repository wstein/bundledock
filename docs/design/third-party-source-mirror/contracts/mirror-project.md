# Contract: interface between BundleDock and the mirror repository (SourceSteward Spec 026)

Read 2026-10-17 from `codex/026-release-process-v2` at `9a810a2`, `specs/026-release-process/`. It is a
**draft**; its `dispatch`, `build` and workflows wait for a constitution amendment. Statements about it are
as read, unverified against any implementation; revisit when it changes. The BundleDock side does not depend
on it.

## What Spec 026 fixes (read)

| Spec 026 decision | Consequence for the mirror |
| --- | --- |
| D1 build once on commit C of `main`: `workflow_dispatch` inputs `commit` and `version`; gates incl. CI green on C, version greater than every tag/release of its **track**, no release exists | dispatch inputs are commit + version, **not** component + version; the component must be encoded in `version` (below) |
| D1 follow-up commit D with `docs/releases/<version>.json`; the **owner pushes an annotated tag** on D; the tag workflow only verifies and publishes | every mirror release needs three owner-involved steps (approve build, merge D, push tag); no hand-made releases |
| D2 `.stw/release.toml`, one project contract; multi-product layout only reserved (its open question 3) | a multi-component mirror depends on that question |
| D4 no standing approval; immutable releases and tag ruleset are owner settings | matches the plan's settings checklist |
| D6 kinds `file` (name, url template, sha256, size, os/arch) and `oci-image` (with `source_offer`: path or URL string) | mirror assets are `file` targets (os/arch fields are required by its schema: see open item) |
| stw writes `SHA256SUMS` and `build-manifest.json`, and attests | `fetch` writes neither (FR-4) |
| version schemes `semver`, `calver`, `pattern` with `order` groups; the track is the tag with order groups removed | a `pattern` whose first group is the component makes one track per component |

## Proposed contract for the mirror repository (requirements on Spec 026, unverified)

- Scheme `pattern`, tag = version = `<component>-v<V>[-r<R>]`, with a pattern in which the component is a
  non-order group and `V` groups (up to four) and `R` are order groups. Then the track is the component
  and monotonicity is per component (naming table), and the dispatch `version` input is the whole tag name.
  Needs confirmation that Spec 026's pattern scheme accepts this; otherwise one contract per component
  (open question 8 of the plan).
- One `[[build]]` (scope `once`) whose argv is `bdock tools sources fetch --for {version} --out {out_dir}`
  (placeholder names as in Spec 026's config contract, unverified), and `[[target]]`s of kind `file` for
  `*-source.tar.gz` and `*-source-manifest.json`. Spec 026 requires `os` and `arch` on `file` targets;
  the mirror needs a neutral value or a schema relaxation (open item for Spec 026).
- `bdock` is built in the workflow from `bdock_source_commit` with `bdock_go_version` of the plan (both
  checked by the workflow before use), or taken from a released `bdock/vX.Y.Z` archive after checksum
  verification. Building our own CLI is allowed by `AGENTS.md`.
- The plan is part of commit C; `fetch --for` reads `plans/<component>/<V>[-r<R>].json` from the checked-out
  C, so the recipe and digests are reviewed with C.
- A verify step `bdock tools sources check --bundle-dir {out_dir}` runs as an additional reproducible
  check before promotion (additive to stw's `verify`).
- The release body is generated from the manifest (upstream URL and tag, licenses, plan digest).

## BundleDock-side data crossing the boundary

Only: plan JSON (committed in the mirror repo), the two output files, and `source-check/v1` JSON (for the
OCI record's `source_offer`). No shared code.

## Open items for Spec 026 (to raise with its author)

1. Multi-product or pattern-with-component support (above).
2. `file` targets without os/arch.
3. A reference form for `source_offer`.
4. Backfill of older versions: monotonicity requires oldest first; each costs an owner approval, build and
   tag. If the number of backfills is large (WI2/WI9), a batch mode or an exemption may be wanted.
5. Dispatch/build blocked until the constitution amendment is decided.
