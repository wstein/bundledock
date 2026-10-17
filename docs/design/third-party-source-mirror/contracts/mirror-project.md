# Contract: interface between BundleDock and the mirror repository (SourceSteward release process)

Everything about Spec 026 here is a **requirement placed on it**, not a fact: when read
(2026-10-17) the in-flight branch did not yet contain the `file` / `oci-image` artifact kinds (see
[research R4](../research.md#r4-spec-026-state)). Align when it lands; do not depend on it for the
BundleDock-side work.

## What BundleDock provides

| Item | Where | Notes |
| --- | --- | --- |
| Plan file | mirror repo `plans/<component>/<version>.json` | produced by `bdock tools sources plan`, reviewed in a mirror-repo PR |
| `bdock` | built from the pinned bundledock source commit named in the plan (`bdock_source_commit`) with `go build` in the workflow, or a released `bdock/vX.Y.Z` archive verified against its checksum | building our own CLI is allowed by `AGENTS.md`; which of the two is open |
| Fetch step | `bdock tools sources fetch --plan plans/<c>/<v>.json --out out/` | the only networked step; documented effect |
| Verify step | `bdock tools sources check --bundle-dir out/` | offline, hostile-input safe |

## What the mirror's SourceSteward project contract must declare

(Format names are placeholders for the Spec 026 contract file; unverified.)

- One product `mirror`, multi-component: tag template `<component>-v<version>` with optional `-r<N>`,
  per-component monotonic ordering (bundledock FR-3 gives the comparison rule).
- Artifact kind `file`, three files per release: `*-source.tar.gz`, `*-manifest.json`, `SHA256SUMS`.
- `build` = the fetch step above, run in the mirror repository's own CI via `workflow_dispatch`
  with inputs `component` and `version`, on a commit of the mirror repository (the plan is read from
  that commit; any plan not on the default branch is refused).
- `verify` = the verify step plus SHA256SUMS agreement.
- Build once, promote: the promote job attaches the verified files to the immutable release and
  creates the attestation (identity and predicate per Spec 026); it never re-fetches.
- The release body lists upstream URL, tag, license list and the plan digest.
- Gates that apply (names from Spec 026 G-series, unverified): authorization of the exact operation,
  successful CI on the commit, owner as actor.

## What the OCI bundle's record does

The `oci-image` record of a toolbox publication includes `bdock tools sources check --json`
`releases[]` for the locked versions (FR-14). The publication fails when `check --enforce` has errors
(FR-12). The record links; it does not attach bundles.

## Data crossing the boundary

Only: plan JSON (in), three files (out), `source-check/v1` JSON (to the OCI record). No shared
code. Either side can change internals without the other.
