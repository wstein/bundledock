# Contract: `bdock tools sources check`

```text
bdock tools sources check [--bundle toolbox] [--enforce] [--online] [--bundle-dir DIR] [--json]
```

Offline; starts no process. Exit statuses as in the [command contract](sources-command.md#exit-status):
1 when any error finding exists, 3 for usage errors, 2 internal.

## Registry check (default)

Reads the catalog, `packages.lock.json`, `sources.lock.json` and `licenses/*/{SOURCE,SOURCE-MIRROR}`.
The scope is every catalog package; each code states its own narrower scope in the table (the mirror codes
apply to packages of obligation `source-offer` or `file-level-source` and every locked version, both
architectures):

| Finding code | Condition | Level |
| --- | --- | --- |
| `source-block-missing` | package has no `source` block | error |
| `mirror-entry-missing` | no `current` registry revision for (component, `LV`) | error with `--enforce`, else warning |
| `mirror-tag-mismatch` | registry tag is not the naming table's tag | error |
| `mirror-notice-drift` | `SOURCE-MIRROR` missing or different from what `sync` would write | error with `--enforce`, else warning |
| `source-paragraph-missing` | `SOURCE` lacks the version-independent paragraph | error with `--enforce`, else warning |
| `classification-contradiction` | `obligation: none` but repository-visible evidence has a copyleft marker | error |
| `undetermined-package` | `obligation: undetermined` | warning (counted in the summary) |
| `registry-orphan` | registry entry for a version no longer in the lock | informational (history allowed) |

## Online check (`--online`)

Network, read-only GETs to the mirror's release API; exits 3 (`network-disabled`) under
`BDOCK_SOURCES_OFFLINE=1`. For every registry entry used by the lock: the release exists, is not a draft,
its asset digests equal the registry (source of the digest: the release API `digest` field of each asset, **unverified** to exist for this repository; fallback: download each asset with the response limit of the [spec](../spec.md#limits) and hash it, which can be up to 1.9 GiB per asset, run only by the publication gate, with a 30 minute total timeout), and the manifest `component` and `version` match. Codes
`release-not-published`, `release-hash-mismatch` (spec failure table). The publication gate for a bundle
runs `check --enforce --online`; `make check` never uses `--online`.

## Bundle check (`--bundle-dir`)

Verifies a directory with the two bundle files. Allowed extras: `SHA256SUMS` and `build-manifest.json`
made by the release process (when `SHA256SUMS` is present it must list every file of the directory and only
those). Failures use the codes `bundle-hash-mismatch`, `bundle-file-missing`, `bundle-file-extra`,
`bundle-manifest-mismatch`, `sums-mismatch` of the spec failure table. Checks: manifest schema and the
[limits](../spec.md#limits) (byte caps first); `asset_sha256` and `tar_sha256`; every `files[]` entry
exists with its hash and size and no other entry exists, apart from `MANIFEST.json`, which is checked
by parsing and comparing its non-standalone fields to the standalone manifest; every `dependencies[]` entry
verifies against its recorded class; `upstream.tarball_sha256` equals the stored tarball's hash. Reading
uses the same bounded in-memory reader; nothing is extracted.

## Wiring

`make check` runs the check as a Go test in `internal/toolconfig` (compiler verification only; no artifact);
the CLI command shares the library. `--enforce` is default-off until
[rollout step 7](../plan.md#rollout-order), then default-on.

## JSON

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
  "summary": { "errors": 0, "warnings": 0, "undetermined": 0 }
}
```

`releases` lists the current revision for each copyleft version in the lock, sorted by component.
`summary.undetermined` is the count of `undetermined-package` findings (a number computed at run time; no
fixed value is promised by this design). The OCI record's `source_offer` string references this output (for
example the path of a file holding it); the exact reference form is Spec 026's field ([mirror
contract](mirror-project.md)). Human output prints `level code component version`, at most 200 lines.
