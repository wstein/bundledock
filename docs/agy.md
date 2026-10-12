# Optional Antigravity CLI bundle

`bundles/agy` supplies the official stable Linux AMD64 and ARM64 Antigravity CLI
prebuilt with its own catalog, lock, carrier image, volume and
`/opt/bundledock/agy` root. This preparation copies the existing toolbox's
1.3.3 pins exactly. Toolbox inputs and consumer configuration are unchanged.

<!-- BEGIN GENERATED TOOL INVENTORY -->
| Tool | Pinned version |
| --- | --- |
| agy | 1.3.3 |
<!-- END GENERATED TOOL INVENTORY -->

The official updater's [AMD64 manifest](https://antigravity-cli-auto-updater-974169037036.us-central1.run.app/manifests/linux_amd64.json)
and [ARM64 manifest](https://antigravity-cli-auto-updater-974169037036.us-central1.run.app/manifests/linux_arm64.json),
checked October 12, 2026, match both pinned URLs and publisher SHA512 values.
Both official tar.gz archives were downloaded, SHA512-verified and inspected
without extraction or execution. Each contains exactly one regular executable,
`antigravity`, and no links, other payload files or standalone notices. The
complete executable is installed unchanged as `bin/agy`.

The [upstream README](https://github.com/google-antigravity/antigravity-cli#terms-of-service--data-use)
links [Antigravity terms](https://antigravity.google/terms) and
[Google privacy policy](https://policies.google.com/privacy).
The carrier retains a factual [SOURCE notice](../bundles/agy/licenses/agy/SOURCE)
with these upstream references. Any embedded executable notices remain unchanged.
No separate license file was present in the inspected archives or repository root.

## Manual consumer use

This source preparation has no published image or consumer activation record.
Builds, native smoke execution, publication and consumer activation each require
explicit approval for the intended operation and source revision. After an
approved carrier build, initialize its image-derived volume with:

```sh
bundledock tools volume --image bundledock-agy:poc --bundle agy
```

Mount the printed `bundledock-agy-<full image ID>` volume read-only at
`/opt/bundledock/agy` and prepend `/opt/bundledock/agy/bin` to the consumer's
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

`bundledock tools upgrade --directory bundles/agy` updates this bundle alone.
The `agy-upgrade.yml` wrapper runs daily at 06:50 UTC, supports manual dry runs
by default and proposes only its lock and generated source inventory through
the shared updater. It never builds or dispatches native publication.

`bundledock-agy.yml` uses the shared [publication workflow](publication.md).
Initial native builds and publication require explicit manual approval of the
full source SHA. Future eligible lock-only updater proposals follow the same
exact-head **wstein** approval, source CI, guarded merge and frozen-lock publication
flow as the other bundles. The separate destination is
`quay.io/bundledock/agy`; no initial publication is implied by these source files.
Before the first approved publication, verify the public Quay repository and
robot permissions. An HTTP 401 is ambiguous and supplies no absent-baseline evidence.
Successful approved publication creates an annotated `agy/<immutable-version>`
source tag after verified promotion. Consumer activation is a separate operation.

Source fixtures cover singleton checksum-verified fixture installation, both
architecture catalogs, copied notices and fake Docker orchestration with repeated
initialization and two read-only non-root consumers. The extraction and volume
integrity checks use the existing generic profile. These are source checks,
not native agent or image validation. OCI builds, actual agent execution,
native AMD64/ARM64 consumer smoke and vulnerability scans remain deferred
pending explicit approval.
