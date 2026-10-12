# Optional Claude Code bundle

`bundles/claude` supplies the official stable Linux AMD64 and ARM64 Claude Code
prebuilt with its own catalog, lock, carrier image, volume and
`/opt/bundledock/claude` root. This preparation copies the existing toolbox's
2.1.296 pins exactly. Toolbox inputs and consumer configuration are unchanged.

<!-- BEGIN PUBLISHED BUNDLE -->
No verified publication receipt is recorded here yet. Initial publication requires
explicit approval of the exact full source revision and intended operation.
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
installation. This preparation checked publisher metadata; native binary
execution remains unverified.

The carrier includes the unmodified [upstream LICENSE.md](https://github.com/anthropics/claude-code/blob/v2.1.296/LICENSE.md)
from the exact pinned tag and a factual [SOURCE notice](../bundles/claude/licenses/claude/SOURCE).
The upstream notice links [Anthropic Commercial Terms](https://www.anthropic.com/legal/commercial-terms).

## Manual consumer use

This source preparation has no published image or consumer activation record.
Builds, native smoke execution, publication and consumer activation each require
explicit approval for the intended operation and source revision. After an
approved carrier build, initialize its image-derived volume with:

```sh
bundledock tools volume --image bundledock-claude:poc --bundle claude
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
`quay.io/bundledock/claude`; no initial publication is implied by these source files.
Before the first approved publication, verify the public Quay repository and
robot permissions. An HTTP 401 is ambiguous and supplies no absent-baseline evidence.
Successful approved publication creates an annotated `claude/<immutable-version>`
source tag after verified promotion. Consumer activation is a separate operation.

Source fixtures cover singleton checksum-verified fixture installation, both
architecture catalogs, copied notices and fake Docker orchestration with repeated
initialization and two read-only non-root consumers. The extraction and volume
integrity checks use the existing generic profile. These are source checks,
not native agent or image validation. OCI builds, actual agent execution,
native AMD64/ARM64 consumer smoke and vulnerability scans remain deferred
pending explicit approval.
