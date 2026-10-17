# Contract: `bdock tools sources pin|plan|fetch|sync`

Part of the [spec](../spec.md); data in the [data model](../data-model.md). `check` is in
[sources-check.md](sources-check.md).

```text
bdock tools sources pin   <tool> [--bundle toolbox] [--lock FILE] [--revision R] [--freeze-file PATH]
                                 [--sources DIR] [--json]
bdock tools sources plan  (--component C --version N [--sources DIR] [--bundle toolbox] [--lock FILE] | --standalone [--sources DIR]) [--json]
bdock tools sources fetch --component C --version N --out DIR [--sources DIR] [--json]
bdock tools sources sync  [--bundle toolbox] [--check]
```

`<tool>` is a catalog package with `obligation` `source-offer` or `file-level-source`. `--sources` (default
`sources`) is the directory of the per-component source locks (`sources/<component>.lock.json`). `N` is the
`version id` of the [naming table](../spec.md#naming-table). `--lock` lets `pin` and `plan` read an updater
PR's lock from another path. `plan --standalone` is what mirror CI runs: it validates every source lock
file's schema, internal consistency (digests present, tags and version ids well formed, monotonic order per
component, `revision` consistent with the version id) and needs neither catalog nor lock. `fetch` is the
build command of the mirror's contracts and reads only `sources/<C>.lock.json`.

## Effects

| Mode | Reads | Network | Writes | Starts processes |
| --- | --- | --- | --- | --- |
| `pin` | catalog, lock, the existing source lock | https to the [fetch hosts](../data-model.md#fetch-hosts) and the repository's tag API (read-only); hashes the upstream tarball and pin targets in memory, nothing distributable is written | creates or appends one entry in `<sources>/<component>.lock.json`; an existing version id is never overwritten: re-pinning with identical digests is a no-op (exit 0, file unchanged), differing digests are `upstream-hash-mismatch` (exit 1) | none |
| `plan` | the source lock, catalog, lock | none | none | none |
| `fetch` | the source lock entry | same hosts, https only, per-hop checks | in `--out`: `<component>-<version id>-sources.tar.gz` and `<component>-<version id>-sources.manifest.json`; a private 0700 temp directory beside `--out`, removed on exit | none |
| `sync` | `sources.lock.json`, lock | none | `licenses/<tool>/SOURCE-MIRROR` (`--check`: none) | none |

`fetch` produces distributable files and needs explicit approval each time (`AGENTS.md`). Whether `pin` is
"an artifact build" is an owner decision (plan, open question 7); until decided it is not run by agents
without asking. `fetch` does not write checksums (the release process does, FR-4); it refuses a non-empty
`--out` (`out-not-empty`). The environment variable `BDOCK_SOURCES_OFFLINE=1` makes `pin` and `fetch`
exit 3 (`network-disabled`). `SOURCE_DATE_EPOCH` is taken only from the entry.

## Exit status

One table for every command of the group; precedence from highest to lowest is 3, 2, 1, 0.

| Status | Meaning | Examples |
| --- | --- | --- |
| 3 | usage or policy refusal decided before any work | unknown flag, `recipe-custom`, `network-disabled`, `tool-not-eligible` |
| 2 | internal error | I/O failure, schema bug in our own output |
| 1 | refusal with a code from the spec's [limits](../spec.md#limits) or [failure](../spec.md#failure-modes) tables, or an error finding of `check` | `upstream-hash-mismatch`, `lock-arch-mismatch`, `out-not-empty`, `entry-stale`, `version-shape`, `entry-missing`, `notice-drift` |
| 0 | success | |

## Human output

`pin`: facts one per line (component, version id, tarball URL, digest, tag commit, recipe, count of pins).
`plan`: the same from an existing entry plus `consistent: yes|no` against catalog and lock. `fetch`: progress
on stderr; final lines on stdout list the two files with sizes and SHA256. Per-dependency lines are capped
at 200, the rest summarized by count.

## JSON

`--json` prints the entry for `pin` and `plan`, and for `fetch`
`{"schema":"bundledock.source-fetch/v1","tag","files":[{"name","bytes","sha256"}]}`; same data as the human
output.

## Determinism requirements (tests)

Same plan and same fetched bytes give the same `tar_sha256` (golden fixture). Invariance tests: reordered
upstream members, member mtimes and owners, HTTP arrival order, umask, working directory, `TZ`, locale,
`--out` path. A mutation per normalization (owner, mtime, order, mode, argv normalization) must fail the
golden test.

## Compatibility

New subcommand group; no existing command, flag, schema or file changes except the catalog `source` block
and `notices` entries.
