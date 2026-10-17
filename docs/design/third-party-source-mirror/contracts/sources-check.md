# Contract: `bdock tools sources check`

```text
bdock tools sources check [--bundle toolbox] [--enforce] [--bundle-dir DIR] [--json]
```

Offline. Two uses.

**Registry check (default).** Reads `packages.catalog.json`, `packages.lock.json`, `sources.lock.json`
and `licenses/*/SOURCE`. For every package whose `obligation` is `source-offer` or `file-level-source`
and every locked version (both architectures):

| Finding code | Condition | Level |
| --- | --- | --- |
| `source-block-missing` | package has no `source` block | error |
| `mirror-entry-missing` | no `sources.lock.json` entry for (component, version) | error with `--enforce`, else warning |
| `mirror-tag-mismatch` | tag is not `<component>-v<version>[-rN]` | error |
| `source-file-missing-block` | `SOURCE` lacks the mirror block, or its tag/hash lines disagree with the registry | error with `--enforce`, else warning |
| `classification-contradiction` | `obligation: none` but a retained license file has a copyleft marker | error |
| `undetermined-package` | `obligation: undetermined` | warning (count in summary) |
| `registry-orphan` | registry entry for a component/version absent from the catalog or lock history | warning (history is allowed; not an error) |

**Bundle check (`--bundle-dir`).** Verifies a directory produced by `fetch` or downloaded from a
release: `SHA256SUMS` covers exactly the three files; manifest passes the schema and limits;
`asset_sha256` and `tar_sha256` match; every `files[]` entry exists with its hash and no other entry
exists; every `dependencies[]` entry is verified against its recorded source; `upstream.tarball_sha256`
matches the upstream member in the bundle; the plan digest matches. Reading uses the same bounded
reader (hostile-input rules). No network; exit codes as in the
[command contract](sources-command.md#exit-status), 1 when any error finding exists.

## Wiring

`make check` runs `go run ./cmd/bdock tools sources check` (compiler verification only, no artifact;
if the project wishes to avoid `go run` in the Makefile, the same logic is a Go test in
`internal/toolconfig`; tasks WI7 pick the test form and the command form shares the library).
`--enforce` is default-off until [rollout step 6](../plan.md#rollout-order), then default-on.

## JSON (also the OCI record input, FR-14)

```json
{
  "schema": "bundledock.source-check/v1",
  "bundle": "toolbox",
  "enforce": true,
  "findings": [],
  "releases": [
    {
      "component": "shellcheck",
      "version": "v0.11.0",
      "obligation": "source-offer",
      "licenses": ["GPL-3.0-only"],
      "release_url": "https://github.com/wstein/bundledock-third-party-sources/releases/tag/shellcheck-v0.11.0",
      "manifest_sha256": "<64 hex>"
    }
  ],
  "summary": { "errors": 0, "warnings": 0, "undetermined": 38 }
}
```

`releases` lists only the versions in the current lock, sorted by component. The `oci-image` record in
Spec 026 embeds this list verbatim (unverified shape on the Spec 026 side). Human output prints each
finding as `level code component version`, bounded to 200 lines.
