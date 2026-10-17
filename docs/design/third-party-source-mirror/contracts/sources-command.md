# Contract: `bdock tools sources pin|plan|fetch|sync`

Part of the [spec](../spec.md); data in the [data model](../data-model.md). `check` is in
[sources-check.md](sources-check.md).

```text
bdock tools sources pin   <tool> [--bundle toolbox] [--lock FILE] [--out FILE] [--json]
bdock tools sources plan  --plan FILE [--bundle toolbox] [--lock FILE] [--json]
bdock tools sources fetch (--plan FILE | --for TAG) --out DIR [--json]
bdock tools sources sync  [--bundle toolbox] [--check]
```

`<tool>` is a catalog package with `obligation` `source-offer` or `file-level-source`. `--lock` (default the
bundle's `packages.lock.json`) lets `pin` and `plan` read an updater PR's lock from another path. `fetch
--for <tag>` resolves `plans/<component>/<V>[-r<R>].json` in the current directory from the tag name
(the mirror's build argv, so a workflow needs only the Spec 026 `version`).

## Effects

| Mode | Reads | Network | Writes | Starts processes |
| --- | --- | --- | --- | --- |
| `pin` | catalog, lock | https to the [fetch hosts](../data-model.md#fetch-hosts) and the repository's tag API (read-only); hashes the upstream tarball and pin targets in memory, nothing distributable is written | `--out` plan file (default stdout) | none |
| `plan` | plan, catalog, lock | none | none | none |
| `fetch` | the plan | same hosts, https only, per-hop checks | in `--out`: `<tag>-source.tar.gz`, `<tag>-source-manifest.json`; a private 0700 temp directory beside `--out`, removed on exit | none |
| `sync` | `sources.lock.json`, lock | none | `licenses/<tool>/SOURCE-MIRROR` (`--check`: none) | none |

`fetch` produces distributable files and needs explicit approval each time (`AGENTS.md`). Whether `pin` is
"an artifact build" is an owner decision (plan, open question 7); until decided it is not run by agents
without asking. `fetch` does not write checksums (the release process does, FR-4); it refuses a non-empty
`--out` (`out-not-empty`). The environment variable `BDOCK_SOURCES_OFFLINE=1` makes `pin` and `fetch`
exit 3 (`network-disabled`). `SOURCE_DATE_EPOCH` is taken only from the plan.

## Exit status

One table for every command of the group; precedence from highest to lowest is 3, 2, 1, 0.

| Status | Meaning | Examples |
| --- | --- | --- |
| 3 | usage or policy refusal decided before any work | unknown flag or tool, `recipe-custom`, `network-disabled`, tool with obligation `none` or `undetermined` |
| 2 | internal error | I/O failure, schema bug in our own output |
| 1 | refusal with a code from the spec's [limits](../spec.md#limits) or [failure](../spec.md#failure-modes) tables | `upstream-hash-mismatch`, `lock-arch-mismatch`, `out-not-empty`, `plan-stale`, `version-shape` |
| 0 | success | |

## Human output

`pin`: facts one per line (component, version, tarball URL, digest, tag commit, recipe, count of pins).
`plan`: the same from an existing plan plus `consistent: yes|no` against catalog and lock. `fetch`: progress
on stderr; final lines on stdout list the two files with sizes and SHA256. Per-dependency lines are capped
at 200, the rest summarized by count.

## JSON

`--json` prints the plan (`bundledock.source-plan/v1`) for `pin` and `plan`, and for `fetch`
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
