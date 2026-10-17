# Data model: third-party source mirror

All JSON is UTF-8, LF, two-space indent, keys sorted, no duplicate keys, unknown keys refused.

## Catalog `source` block

Added to each package in `bundles/toolbox/packages.catalog.json` (catalog `schema` stays 1; the
block is optional only until WI2 completes, then required).

| Field | Type | Rule |
| --- | --- | --- |
| `obligation` | enum | `none`, `source-offer`, `file-level-source`, `undetermined` (spec table) |
| `licenses` | string[] | SPDX identifiers of components in the object code, sorted; required unless `undetermined` |
| `basis` | enum | `declared` (upstream statement) or `scanned` (dependency scan of the shipped binary, WI2) |
| `component` | string | mirror component name; default and required equal to the package name |
| `upstream.tarball` | string | URL template with `{version}` (lock version without leading `v`) and `{tag}`; host in `allowed_hosts` of the package or `api.github.com`/`codeberg.org` where listed |
| `upstream.repository` | string | `https://host/owner/name` for the peeled tag commit |
| `recipe.kind` | enum | `upstream-tarball-only`, `cargo-vendor`, `go-mod-download`, `cabal-freeze` (`custom` reserved, refused) |
| `recipe.excluded` | string[] | free text items to copy into the manifest (toolchain, libc) |
| `recipe.static_system_libs` | string[] | optional, review aid |
| `recipe.freeze_source` | string | `cabal-freeze` only: member path of the dependency report in the tarball (for example `ThirdPartyNotices.txt`) |

Example (hadolint, from PR #119 semantics):

```json
"source": {
  "obligation": "source-offer",
  "licenses": ["GPL-2.0-only", "GPL-3.0-only", "GPL-3.0-or-later", "MPL-2.0"],
  "basis": "declared",
  "component": "hadolint",
  "upstream": {
    "repository": "https://github.com/hadolint/hadolint",
    "tarball": "https://github.com/hadolint/hadolint/archive/refs/tags/v{version}.tar.gz"
  },
  "recipe": {
    "kind": "cabal-freeze",
    "freeze_source": "ThirdPartyNotices.txt",
    "excluded": ["GHC 9.10.2 and boot libraries", "alpine:3.24 toolchain"],
    "static_system_libs": ["gmp", "ncurses", "zlib", "musl", "libffi"]
  }
}
```

Validation (`toolconfig.Validate`): obligation enum; `source-offer`/`file-level-source` require
`upstream`, `recipe`, `licenses`; `none` forbids `recipe`; hosts must be https and from a fixed
allow-list per recipe kind (table below); `component` equals the package key; no `..`/control
characters in templates. A test refuses `none` when a retained license file matches a copyleft marker
(FR-1).

| Recipe kind | Fetch hosts (allow-list, constants) |
| --- | --- |
| all | the tarball host from the plan |
| `go-mod-download` | `proxy.golang.org` |
| `cargo-vendor` | `index.crates.io`, `static.crates.io`, git source hosts listed in `Cargo.lock` that are `https://github.com` or `https://codeberg.org` |
| `cabal-freeze` | `hackage.haskell.org` |

## Sources lock

`bundles/toolbox/sources.lock.json`, schema 1:

```json
{
  "schema": 1,
  "mirror": "https://github.com/wstein/bundledock-third-party-sources",
  "releases": {
    "shellcheck": {
      "v0.11.0": {
        "tag": "shellcheck-v0.11.0",
        "bundle_sha256": "<64 hex>",
        "manifest_sha256": "<64 hex>",
        "sums_sha256": "<64 hex>"
      }
    }
  }
}
```

Keys: component, then lock version string exactly as in `packages.lock.json` (including a leading
`v` when present). `tag` must equal `<component>-v<version without leading v>` optionally `-rN`.
Hashes are lowercase hex. Sorted. One entry per (component, version); a second version of the same
component is allowed (history is retained for published carriers). Entries are never removed.

## Source manifest

Published asset `<component>-v<version>-manifest.json`; also stored inside the bundle at
`MANIFEST.json`. Schema id `bundledock.source-manifest/v1`.

| Field | Type | Meaning |
| --- | --- | --- |
| `schema` | string | `bundledock.source-manifest/v1` |
| `component`, `version`, `release_tag` | string | identity; `version` as in the lock |
| `produced_by` | object | `tool: "bdock"`, `tool_version`, `plan_sha256` |
| `source_date_epoch` | integer | mtime of all bundle entries |
| `upstream` | object | `repository`, `tag`, `tag_commit` (peeled), `tarball_url`, `tarball_sha256`, `tarball_name`, `tarball_bytes` |
| `licenses` | string[] | as catalog |
| `bundle_license_note` | string | fixed text: the bundle keeps upstream licenses; the mirror repository's own files are EUPL-1.2 |
| `recipe` | object | `kind`, `reconstruction` (bool), `argv` (the exact `bdock tools sources fetch ...` vector), `endpoints` (list of base URLs used), `excluded`, `static_system_libs`, `toolchain_notes` |
| `dependencies` | object[] | each: `ecosystem` (`go`, `cargo`, `hackage`), `name`, `version`, `path` (inside bundle), `sha256` (of the stored file), `verified_against` (`go.sum`, `Cargo.lock`, `hackage-index`), `source_url` |
| `files` | object[] | every bundle entry: `path`, `size`, `sha256`, `mode` |
| `bundle` | object | `name`, `tar_sha256`, `asset_sha256`, `asset_bytes`, `format` (`pax-sorted-v1`), `compression` (`gzip-9-go`) |
| `shipped_binary` | object[] | per architecture: lock `url` and `sha256` of the binary this source corresponds to |

`asset_sha256` and `bundle.asset_bytes` are in the standalone manifest only (self-reference is
impossible inside the bundle); the in-bundle copy omits them and `check` accounts for that. Limits
of the [spec](spec.md#limits) apply; `dependencies` max 5 000.

## Plan

`plans/<component>/<version>.json` in the mirror repo, schema `bundledock.source-plan/v1`: identity,
`upstream` (URL, expected `tarball_sha256`, `tag_commit`), `recipe` (kind, parameters), `source_date_epoch`,
`shipped_binary`, `bdock_source_commit`. Written by `plan`; digests present only after a human
pinned them (`--accept-new-digest`).

## SOURCE file mirror block

Lines appended to `licenses/<tool>/SOURCE`; existing lines are untouched (PR #119 tests read
`Source archive:`/`Source SHA256:`; the hadolint line "relies on upstream availability" is the only
sentence replaced, in that PR):

```text
Mirror release: https://github.com/wstein/bundledock-third-party-sources/releases/tag/<component>-v<version>
Mirror bundle: https://github.com/wstein/bundledock-third-party-sources/releases/download/<tag>/<component>-v<version>-source.tar.gz
Mirror bundle SHA256: <64 hex>
Mirror manifest SHA256: <64 hex>
Written offer: BundleDock offers the complete corresponding source of this component, at the
Mirror release above, to anyone who received this binary from BundleDock, for as long as this
binary is distributed and at least three years after the last distribution. BundleDock, not the
upstream project, fulfils this offer.
```

## Check report (JSON)

See [contracts/sources-check.md](contracts/sources-check.md).
