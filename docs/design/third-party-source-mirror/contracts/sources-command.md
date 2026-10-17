# Contract: `bdock tools sources plan|fetch`

Part of [spec](../spec.md); data in [data-model](../data-model.md). `check` is in
[sources-check.md](sources-check.md).

```text
bdock tools sources plan  <tool> [--bundle toolbox] [--version V] [--out FILE] [--json]
bdock tools sources fetch --plan FILE --out DIR [--accept-new-digest] [--max-bytes N] [--json]
```

`<tool>` is a catalog package with `obligation` other than `none`. `--version` defaults to the lock
version (amd64 and arm64 must agree; otherwise `lock-arch-mismatch`). `--bundle` is a catalog directory
under `bundles/`; only `toolbox` has a `source` block in v1.

## Effects

| Mode | Reads | Network | Writes | Starts processes |
| --- | --- | --- | --- | --- |
| `plan` | catalog, lock, `sources.lock.json` | none | `--out` file only (default stdout) | none |
| `fetch` | the plan | only hosts in the [allow-list](../data-model.md#catalog-source-block), https only | `--out` directory: `<component>-v<version>-source.tar.gz`, `<component>-v<version>-manifest.json`, `SHA256SUMS`; a private temp directory beside `--out`, removed on exit | none |

`fetch` produces a distributable artifact: under `AGENTS.md` it needs explicit approval each time.
The command refuses to write into a non-empty `--out` (`out-not-empty`). Working files are mode
0600/0700. Environment: reads `SOURCE_DATE_EPOCH` only if the plan lacks `source_date_epoch`
(`plan` always writes it from the peeled tag commit time).

## Exit status

| Status | Meaning | Precedence |
| --- | --- | --- |
| 0 | success | 4 |
| 1 | refusal with a code from the [failure table](../spec.md#failure-modes) or [limits](../spec.md#limits) | 2 |
| 2 | internal error (I/O, schema bug) | 3 |
| 3 | usage error (flags, unknown tool, `custom` recipe, `network-disabled`) | 1 (highest) |

## Human output

`plan`: one line per fact: `component`, `version`, `tarball`, `recipe`, `digest: pinned|unpinned`.
`fetch`: progress lines on stderr, final lines on stdout: three file names with sizes and SHA256.
Output is bounded (no per-dependency lines above 200; the rest summarized with a count).

## JSON

`--json` prints exactly the plan or a `fetch` result `{ "schema": "bundledock.source-fetch/v1",
"component", "version", "files": [{"name","bytes","sha256"}], "observed_digests": {...} }`, same data
as the human output.

## Determinism requirements (tests)

Same plan and same fetched bytes produce the same `tar_sha256` (golden fixture); fixture variations:
reordered upstream members, different member mtimes and owners, different HTTP arrival order, different
umask and working directory, `TZ` and locale: all give an identical bundle. A mutation test per
normalization (owner, mtime, order, mode) must fail the golden test.

## Compatibility

New subcommand group; no existing command, flag, schema or file changes. Catalog `schema` stays 1.
