# Data model: third-party source mirror

All JSON is UTF-8, LF, two-space indent, keys sorted, no duplicate keys, unknown keys refused.
Names (`component`, `V`, `R`, tag, assets, source-lock path) follow the [naming table](spec.md#naming-table).

## Catalog `source` block

Added to each package in `bundles/toolbox/packages.catalog.json` (catalog `schema` stays 1; optional
until WI2 completes, required afterwards).

| Field | Type | Rule |
| --- | --- | --- |
| `obligation` | enum | `none`, `source-offer`, `file-level-source`, `undetermined` ([table](spec.md#obligation-classes)) |
| `licenses` | string[] | SPDX identifiers found in the evidence, sorted; required unless `undetermined` |
| `basis` | enum | `declared` (upstream statement/notices) or `scanned` (WI2 dependency scan of the shipped binary) |
| `upstream.repository` | string | `https://host/owner/name` |
| `upstream.tarball` | string | URL template with `{version}` (`V`) and `{lock_version}` (`LV`); https; first-hop host in the [fetch hosts](#fetch-hosts) table |
| `recipe.kind` | enum | `upstream-tarball-only`, `cargo-vendor`, `go-mod-download`, `cabal-freeze` (`custom` reserved, refused) |
| `recipe.excluded` | string[] | copied to the manifest |
| `recipe.static_system_libs` | string[] | optional review aid |
| `recipe.freeze_source` | string | `cabal-freeze`: member path of the dependency report in the tarball |
| `recipe.freeze_file` | string | `cabal-freeze`: name of an owner-reviewed freeze file supplied to `pin` with `--freeze-file PATH` (its SHA256 is recorded in the source-lock entry; `reconstruction` is `true` for every `cabal-freeze` component, because no upstream freeze file for the exact target is used) |
| `version_regex` | string | optional; one capture group on `LV` yielding `V` ([naming table](spec.md#naming-table)); Go RE2; each numeric group at most 9 digits |

`component` is the package key (no separate field). Validation (`toolconfig.Validate`): enum values; copyleft
classes require `upstream`, `recipe`, `licenses`; `none` forbids `recipe`; https only; no `..` or control
characters in templates; `cabal-freeze` requires exactly one of `freeze_source`, `freeze_file`. A test
refuses `none` when repository-visible evidence has a copyleft marker (FR-1).

Amendments recorded with WI1 (`internal/toolconfig`, implemented and tested there):

- `basis` and `licenses` are required for `none` as well as for the copyleft classes (only `undetermined` may omit
  them). Any `recipe` requires `upstream`; `none` forbids a `recipe`.
- Each `licenses` entry is an SPDX expression shape: identifiers joined by single `AND`, `OR` or `WITH` operators,
  with balanced parentheses (`GPL-3.0-or-later WITH GCC-exception-3.1`). Only the shape is checked, not the SPDX
  list. The list is sorted and unique.
- `upstream.tarball` must contain `{version}` or `{lock_version}`, no other placeholder, and after expansion must be
  https with a first-hop host from the [fetch hosts](#fetch-hosts) table and no userinfo, port, query (not even an
  empty `?`), fragment (not even an empty `#`), backslash, space, `..` segment, control or format character
  (bidi overrides, zero width) or percent-encoded control byte. The catalog check only sees a sample expansion:
  `pin` and `plan` (WI5, WI6) must validate the URL again after expanding it with the real lock version, and must
  not fetch `upstream.repository` without its own host check.
- Free-form text is bounded: at most 64 entries in `licenses`, `excluded` and `static_system_libs`, each at most 200
  bytes, `excluded` and `static_system_libs` without duplicates; `version_regex` at most 200 bytes.
- `version_regex` has full-match semantics: the pattern must match the whole lock version (an unanchored pattern
  cannot drop a prerelease suffix). `version-shape` is raised by `pin` and `plan` through
  `Package.SourceVersion`; catalog validation only checks that the pattern compiles with exactly one capture group.

Example (hadolint; the tarball is the GitHub API tarball that PR #119 pins):

```json
"source": {
  "obligation": "source-offer",
  "licenses": ["GPL-2.0-only", "GPL-3.0-only", "GPL-3.0-or-later", "MPL-2.0"],
  "basis": "declared",
  "upstream": {
    "repository": "https://github.com/hadolint/hadolint",
    "tarball": "https://api.github.com/repos/hadolint/hadolint/tarball/{lock_version}"
  },
  "recipe": {
    "kind": "cabal-freeze",
    "freeze_source": "ThirdPartyNotices.txt",
    "excluded": ["GHC 9.10.2 and boot libraries", "alpine:3.24 toolchain"],
    "static_system_libs": ["gmp", "ncurses", "zlib", "musl", "libffi"]
  }
}
```

### Fetch hosts

Normative constants, checked **per hop** (every redirect target), https only, no userinfo, no IP literals.
First hops and their measured redirect targets (HEAD requests, 2026-10-17):

| First hop | Redirect target (measured) | Used for |
| --- | --- | --- |
| `github.com` (`/releases/download/`) | `release-assets.githubusercontent.com` | release assets |
| `github.com` (`/archive/refs/tags/`) | `codeload.github.com` | tag archives |
| `api.github.com` (`/repos/.../tarball/`) | `codeload.github.com` | API tarballs |
| `proxy.golang.org` | none (HTTP 200) | Go modules |
| `static.crates.io` | none (HTTP 200) | crates |
| `hackage.haskell.org` | none (HTTP 200, measured) | Hackage sdists |
| `codeberg.org` | none (HTTP 200, measured) | Cargo git sources hosted there |
| `github.com` (`/<owner>/<repo>/archive/<commit>.tar.gz`) | `codeload.github.com` (same family as tag archives; not measured for commit archives) | Cargo git sources |

Caveat for Cargo git sources: a host archive of a commit omits submodules and honours `export-ignore`,
while `cargo` clones the repository. No current instance (fish's `rust-pcre2` has an empty `.gitmodules`
and vendors PCRE2 as a tree, per round-2), but `fetch` records the archive URL and hash as `plan-pin`.

The list may grow only by a code change with a test; other hosts refuse with `fetch-host-refused`.

## Sources lock

`bundles/toolbox/sources.lock.json`, schema 1:

```json
{
  "schema": 1,
  "mirror": "https://github.com/wstein/bundledock-third-party-sources",
  "releases": {
    "shellcheck": {
      "v0.11.0": {
        "revisions": [
          {
            "revision": 1,
            "tag": "shellcheck-v0.11.0",
            "status": "current",
            "bundle_sha256": "<64 hex>",
            "manifest_sha256": "<64 hex>"
          }
        ]
      }
    }
  }
}
```

Component, then `LV` exactly as in `packages.lock.json`. Exactly one revision per `LV` has `status:
current` (highest `revision`); older ones are `superseded` and never removed. `tag` must equal the naming
table's tag for that revision. Hashes lowercase hex. Sorted. Entries are retained for every version ever
published in a carrier. The release process's own `SHA256SUMS` hash is not stored here (it is in the
release).

## Source manifest

Published asset `<component>-<version id>-sources.manifest.json`; also stored in the bundle as `MANIFEST.json` **without** the
fields marked (standalone only), because a file cannot contain its own hash. Schema id
`bundledock.source-manifest/v1`.

| Field | Meaning |
| --- | --- |
| `schema` | `bundledock.source-manifest/v1` |
| `component`, `version` (`LV`), `release_tag`, `revision` | identity |
| `produced_by` | `tool: "bdock"`, `tool_version`, `bdock_source_commit`, `bdock_go_version`, `entry_sha256` (SHA-256 of the canonical JSON of the single source-lock entry: keys sorted, two-space indent, LF, one trailing newline, no HTML escaping of `<`, `>`, `&` in strings (Go `SetEscapeHTML(false)`), other escapes per RFC 8259; never of the whole file, so later versions do not change older bundles) |
| `source_date_epoch` | mtime of all bundle entries; the peeled upstream tag commit time written by `pin` |
| `upstream` | `repository`, `tag`, `tag_object` (annotated tag object or null), `tag_commit` (peeled), `tarball_url`, `tarball_name`, `tarball_sha256`, `tarball_bytes` |
| `archive_anomalies`, `anomalies_truncated` | `anomalies_truncated` is an integer count of entries beyond the first 1 000; `archive_anomalies` objects: absolute or escaping symlink targets, duplicate member names, special members (bounded to 1 000 entries) |
| `licenses` | as catalog |
| `bundle_license_note` | fixed text: bundles keep upstream licenses; the mirror's own files are EUPL-1.2 |
| `recipe` | `kind`, `reconstruction` (bool), `argv` (normalized: `bdock tools sources fetch --component <C> --version <version id>`, no output path), `endpoints`, `excluded`, `static_system_libs`, `toolchain_notes`, `cabal_revisions` |
| `dependencies[]` | `ecosystem` (`go`, `cargo`, `hackage`), `name`, `version`, `path` (in bundle), `sha256` (of the stored file), `verified_against` (`lockfile-hash` or `plan-pin`, [classes](spec.md#verification-classes)), `source_url` |
| `files[]` | every bundle entry **except `MANIFEST.json`**: `path`, `size`, `sha256`, `mode` |
| `bundle` (standalone only) | `name`, `tar_sha256`, `asset_sha256`, `asset_bytes`, `format` (`pax-sorted-v1`), `compression` (`gzip-9-go`) |
| `shipped_binary[]` | per architecture: lock `url` and `sha256` of the binary this source corresponds to |

`tar_sha256` in the bundle is computed over the tar **including** `MANIFEST.json` (so it is derived after
the manifest is final); the standalone manifest repeats it. Limits are in the [spec](spec.md#limits).

## Source lock and plan entries

Mirror-repository file `sources/<component>.lock.json` (the `lock_files` input that Spec 026 example 5
declares for a fetch build; fixed path per component), schema `bundledock.source-lock/v1`:
`{"schema", "component", "versions": {"<version id>": <entry>}}`. Each entry is a **plan** written only by
`pin`: `component`, `version` (`LV`), `revision`, `upstream` (`tarball_url`, `tarball_sha256`, `tag_object`,
`tag_commit`), `recipe` (`kind`, parameters, `pins[]` of `{url, sha256}` for `plan-pin` dependencies,
`freeze_file_sha256` if any), `source_date_epoch`, `shipped_binary`, `bdock_source_commit`,
`bdock_go_version`. Adding a version changes the file, so the fetch build input is the reviewed state of C.
`pin --revision R` creates a re-issue entry (`R >= 2`); a missing flag means `R = 1`.

## Version-independent `SOURCE` paragraph

Appended once to the `SOURCE` of each copyleft tool (the same text for all, only the component name
differs; no version, tag, hash):

```text
Source mirror: https://github.com/wstein/bundledock-third-party-sources
Source release tag rule: <component>-v<version of the installed binary> (see SOURCE-MIRROR for this installation)
Written offer: BundleDock offers the complete corresponding source of this component, as published in the
Source mirror, to anyone who asks, for as long as this binary is distributed and at least three years
after the last distribution. BundleDock, not the upstream project, fulfils this offer.
```

(Wording is a [lawyer item](spec.md#legal-caveat); the design fixes only where the text lives. PR #119's
tests read `Source archive:` and `Source SHA256:` lines, which stay.)

## `SOURCE-MIRROR` (generated)

`licenses/<tool>/SOURCE-MIRROR`, written by `bdock tools sources sync` from `sources.lock.json` and the
lock, listed in the catalog `notices`, verified by `check`:

```text
Installed version: v0.11.0
Mirror release: https://github.com/wstein/bundledock-third-party-sources/releases/tag/shellcheck-v0.11.0
Mirror bundle: https://github.com/wstein/bundledock-third-party-sources/releases/download/shellcheck-v0.11.0/shellcheck-0.11.0-sources.tar.gz
Mirror bundle SHA256: <64 hex>
Mirror manifest SHA256: <64 hex>
Reconstruction: true
```

## Check report (JSON)

See [contracts/sources-check.md](contracts/sources-check.md#json).
