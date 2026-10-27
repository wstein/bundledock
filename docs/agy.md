---
title: Antigravity bundle
description: Use the official Antigravity CLI with its independent inventory and publication receipt.
---

# Optional Antigravity CLI bundle

[All bundles](bundles.md)

`bundles/agy` supplies the official stable Linux AMD64 and ARM64 Antigravity CLI
prebuilt with its own catalog, lock, carrier image, volume and
`/opt/bdock/agy` root. Its 1.3.3 pins are retained unchanged from the original
toolbox split. The source toolbox no longer includes this agent.

<!-- BEGIN PUBLISHED BUNDLE -->
<!-- publication-record: {"attempt":"1","bundle":"agy","commit":"095e16c019e55eacb79b2eb3d1242d012e7de271","created":"2026-10-24T11:51:39Z","digest":"sha256:608745b1e1f3a9407c69268b5e48ae24bbba206da6112c31da24a438c949eb17","image":"quay.io/bundledock/agy","lock_sha256":"efcc3d7711f980b15e1bd71c54c0d012bb42836bfa92f5bc3b1e13f17d73ed24","reference":"quay.io/bundledock/agy@sha256:608745b1e1f3a9407c69268b5e48ae24bbba206da6112c31da24a438c949eb17","run_attempt":"1","run_id":"40091432271","source":"https://github.com/wstein/bundledock","tag":"1.4.7-b1"} -->
## Published bundle

Version: `1.4.7-b1`

Index digest: `sha256:608745b1e1f3a9407c69268b5e48ae24bbba206da6112c31da24a438c949eb17`

Source: [`095e16c019e55eacb79b2eb3d1242d012e7de271`](https://github.com/wstein/bundledock/commit/095e16c019e55eacb79b2eb3d1242d012e7de271)

Publication: [run 40091432271, attempt 1](https://github.com/wstein/bundledock/actions/runs/40091432271/attempts/1)

Pull this published carrier by its immutable index digest:

```sh
docker pull quay.io/bundledock/agy@sha256:608745b1e1f3a9407c69268b5e48ae24bbba206da6112c31da24a438c949eb17
```

The source inventory below describes the current lock; this publication retains its exact source revision.
<!-- END PUBLISHED BUNDLE -->

<!-- BEGIN GENERATED TOOL INVENTORY -->
| Tool | Pinned version |
| --- | --- |
| agy | 1.4.8 |
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

The verified publication above supplies the carrier image. Consumer activation
remains a separate operation requiring explicit approval. To initialize an
image-derived volume from the verified carrier:

```sh
bdock tools volume --image quay.io/bundledock/agy@sha256:a96463845e3948487029f72ef1a242996b986c4bc50e42d68707308652284496 --bundle agy
```

Mount the printed `bundledock-agy-<full image ID>` volume read-only at
`/opt/bdock/agy` and prepend `/opt/bdock/agy/bin` to the consumer's
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

`bdock tools upgrade --directory bundles/agy` updates this bundle alone.
The `propose-upgrades.yml` workflow runs daily at 06:50 UTC, supports manual dry runs
by default and proposes only its lock and generated source inventory through
the shared updater. It never builds or dispatches native publication.

`publish-bundles.yml` uses the shared [publication workflow](publication.md).
Initial native builds and publication require explicit manual approval of the
full source SHA. Future eligible lock-only updater proposals follow the same
actual human **wstein** merge, exact merged-main CI and frozen-lock publication
flow as the other bundles. The separate destination is
`quay.io/bundledock/agy`; the verified initial publication is recorded above.
Registry visibility and robot permissions were verified for that operation.
Successful approved publication creates an annotated `agy/<immutable-version>`
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
