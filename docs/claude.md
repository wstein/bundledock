# Optional Claude Code bundle

`bundles/claude` supplies the official stable Linux AMD64 and ARM64 Claude Code
prebuilt with its own catalog, lock, carrier image, volume and
`/opt/bundledock/claude` root. This preparation copies the existing toolbox's
2.1.296 pins exactly. Toolbox inputs and consumer configuration are unchanged.

<!-- BEGIN PUBLISHED BUNDLE -->
<!-- publication-record: {"attempt":"1","commit":"6515f2abea611eb117748cfca2ab1d87cbc25377","created":"2026-10-12T17:48:11Z","digest":"sha256:4b02222a4954f174a46af2a7ef732c2f0ff7b7bb7b462875491dd80aabf1940f","image":"quay.io/bundledock/claude","lock_sha256":"2b738df043a9e6cfe45e5a744c7d22358e9b2cf8eaa00a143afa8225e44cefd2","reference":"quay.io/bundledock/claude@sha256:4b02222a4954f174a46af2a7ef732c2f0ff7b7bb7b462875491dd80aabf1940f","run_attempt":"1","run_id":"38286530356","source":"https://github.com/wstein/bundledock","tag":"2.1.296-b1"} -->
## Published bundle

Version: `2.1.296-b1`

Index digest: `sha256:4b02222a4954f174a46af2a7ef732c2f0ff7b7bb7b462875491dd80aabf1940f`

Source: [`6515f2abea611eb117748cfca2ab1d87cbc25377`](https://github.com/wstein/bundledock/commit/6515f2abea611eb117748cfca2ab1d87cbc25377)

Publication: [run 38286530356, attempt 1](https://github.com/wstein/bundledock/actions/runs/38286530356/attempts/1)

Pull this published carrier by its immutable index digest:

```sh
docker pull quay.io/bundledock/claude@sha256:4b02222a4954f174a46af2a7ef732c2f0ff7b7bb7b462875491dd80aabf1940f
```

The source inventory below describes the current lock; this publication retains its exact source revision.
<!-- END PUBLISHED BUNDLE -->

<!-- BEGIN GENERATED TOOL INVENTORY -->
| Tool | Pinned version |
| --- | --- |
| claude | 2.1.296 |
<!-- END GENERATED TOOL INVENTORY -->

The official [2.1.296 manifest](https://downloads.claude.ai/claude-code-releases/2.1.296/manifest.json),
checked October 12, 2026, identifies the `linux-x64` and `linux-arm64` raw
executables and confirms both publisher SHA256 values copied from the toolbox.
The complete executable, including any embedded notices, is retained unchanged
as `bin/claude`. The builder verifies the downloaded binary checksum before
installation. Native offline version invocation was subsequently checked on
both architectures in the publication recorded above.

The carrier includes the unmodified [upstream LICENSE.md](https://github.com/anthropics/claude-code/blob/v2.1.296/LICENSE.md)
from the exact pinned tag and a factual [SOURCE notice](../bundles/claude/licenses/claude/SOURCE).
The upstream notice links [Anthropic Commercial Terms](https://www.anthropic.com/legal/commercial-terms).

## Manual consumer use

The verified publication above supplies the carrier image. Consumer activation
remains a separate operation requiring explicit approval. To initialize an
image-derived volume from the verified carrier:

```sh
bundledock tools volume --image quay.io/bundledock/claude@sha256:4b02222a4954f174a46af2a7ef732c2f0ff7b7bb7b462875491dd80aabf1940f --bundle claude
```

Mount the printed `bundledock-claude-<full image ID>` volume read-only at
`/opt/bundledock/claude` and prepend `/opt/bundledock/claude/bin` to the consumer's
PATH. Use a stock glibc language image, such as the existing Debian Trixie Go
consumer. The scratch carrier initializes volumes; it supplies no interactive
runtime system libraries. Compatibility beyond this consumer remains unverified.

Keep HOME, caches, credentials, projects and existing agent configuration in
writable consumer storage outside the immutable mount. Existing user filenames
and settings pass through unchanged; BundleDock writes no user configuration.
Set `DISABLE_UPDATES=1` and `DISABLE_AUTOUPDATER=1` in the consumer, following the
existing toolbox convention. Carrier environment values do not transfer through
a mounted volume. Authentication and network services remain the consumer's
responsibility; the smoke script only checks offline version invocation.

## Updates, publication and evidence

`bundledock tools upgrade --directory bundles/claude` updates this bundle alone.
The `claude-upgrade.yml` wrapper runs daily at 06:50 UTC, supports manual dry runs
by default and proposes only its lock and generated source inventory through
the shared updater. It never builds or dispatches native publication.

`bundledock-claude.yml` uses the shared [publication workflow](publication.md).
Initial native builds and publication require explicit manual approval of the
full source SHA. Future eligible lock-only updater proposals follow the same
exact-head **wstein** approval, source CI, guarded merge and frozen-lock publication
flow as the other bundles. The separate destination is
`quay.io/bundledock/claude`; the verified initial publication is recorded above.
Registry visibility and robot permissions were verified for that operation.
Successful approved publication creates an annotated `claude/<immutable-version>`
source tag after verified promotion. Consumer activation is a separate operation.

Source fixtures cover singleton checksum-verified fixture installation, both
architecture catalogs, copied notices and fake Docker orchestration with repeated
initialization and two read-only non-root consumers. The extraction and volume
integrity checks use the existing generic profile. The publication above also
completed native Linux AMD64 and ARM64 builds and offline consumer smoke checks,
followed by verified index/alias promotion and the annotated source tag. Its
workflow failed only in the subsequent documentation source proposal because
this guide lacked publication markers; the receipt records the completed
publication. Vulnerability scans and consumer activation remain unverified.
