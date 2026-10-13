---
title: Node.js Current bundle
description: Use the independent Node.js Current carrier and distinguish its channel from LTS.
---

# Optional Node.js Current bundle

[All bundles](bundles.md)

`bundles/node-current` supplies official stable Node.js Current Linux AMD64
and ARM64 prebuilts and their bundled npm/npx. The existing `node` bundle
continues to supply Node.js 24 LTS with its existing catalog, lock, image,
volume and consumer configuration.

<!-- BEGIN PUBLISHED BUNDLE -->
<!-- publication-record: {"attempt":"1","bundle":"node-current","commit":"27e5abc2f4649aec8abc75ac68c647319dc0f931","created":"2026-10-12T19:03:26Z","digest":"sha256:3231e7f0ab7d9c3b9debcd52d4b53935b094d775b1f5ad3c25f415fec246a1f7","image":"quay.io/bundledock/node","lock_sha256":"6fd1b9be4ee66931f794c8c3bb69f5d9f72a53d52dba6769a64addb31b6d45df","reference":"quay.io/bundledock/node@sha256:3231e7f0ab7d9c3b9debcd52d4b53935b094d775b1f5ad3c25f415fec246a1f7","run_attempt":"1","run_id":"38294995668","source":"https://github.com/wstein/bundledock","tag":"26.11.1-b1"} -->
## Published bundle

Version: `26.11.1-b1`

Index digest: `sha256:3231e7f0ab7d9c3b9debcd52d4b53935b094d775b1f5ad3c25f415fec246a1f7`

Source: [`27e5abc2f4649aec8abc75ac68c647319dc0f931`](https://github.com/wstein/bundledock/commit/27e5abc2f4649aec8abc75ac68c647319dc0f931)

Publication: [run 38294995668, attempt 1](https://github.com/wstein/bundledock/actions/runs/38294995668/attempts/1)

Pull this published carrier by its immutable index digest:

```sh
docker pull quay.io/bundledock/node@sha256:3231e7f0ab7d9c3b9debcd52d4b53935b094d775b1f5ad3c25f415fec246a1f7
```

The source inventory below describes the current lock; this publication retains its exact source revision.
<!-- END PUBLISHED BUNDLE -->

<!-- BEGIN GENERATED TOOL INVENTORY -->
| Tool | Pinned version |
| --- | --- |
| node | 26.11.1 |
<!-- END GENERATED TOOL INVENTORY -->

The official [distribution index](https://nodejs.org/dist/index.json), checked
October 12, 2026, identifies v26.11.1 (October 7, 2026) as Current, with npm
11.20.0. The shared Node provider selects only stable releases whose `lts`
field is explicitly `false`, within the reviewed 26.x constraint. The existing
LTS provider selection remains unchanged. Missing, null, malformed or LTS
channel markers, prereleases and nightly releases cannot become Current.
A matching newer release missing either native prebuilt or publisher checksum
fails closed instead of falling back.

Both official tar.gz archives were downloaded and verified against
[SHASUMS256.txt](https://nodejs.org/dist/v26.11.1/SHASUMS256.txt), then read without
extracting or executing their contents. Each has 5,828 members and two symlinks
(npm and npx). The unmodified upstream LICENSE is retained. The existing
[node-runtime profile](node.md) preserves the full payload, validates those
relative links and supplies npm/npx launchers using this carrier's Node.
Only optional Corepack is omitted. Third-party binaries are never compiled or
patched. Native OCI builds, Node/npm execution, smoke and vulnerability scans
on both architectures remain pending explicit approval.

## Manual consumer use

After explicit approval of the exact revision and intended operation:

```sh
make bundle-build BUNDLE=node-current
make bundle-smoke BUNDLE=node-current
make bundle-volume BUNDLE=node-current
```

Mount the printed image-derived `bundledock-node-current-<full image ID>` volume
read-only at `/opt/bdock/node-current` in a stock glibc language image.
Add `/opt/bdock/node-current/bin` to PATH. Keep HOME, npm cache, global
prefix and projects outside the mount; follow the [Node consumer guidance](node.md#manual-consumer-use).
No user configuration filename or default consumer activation changes.

## Updates and publication

`bdock tools upgrade --directory bundles/node-current` updates this
bundle's lock alone, using canonical tool ID `node`. The
`propose-upgrades.yml` workflow selects `bundle=node-current` through the shared lock-only updater daily at
06:50 UTC and supports manual dry runs. It never builds or dispatches publication.

`publish-bundles.yml` calls the existing shared publication workflow
with `bundle=node-current`, sharing caller workflow identity while retaining
distinct artifacts, volume and source tag identity. Both Node
channels publish to `quay.io/bundledock/node`. Current promotion updates only
`:current`; LTS promotion updates `:lts`, `:latest` and the backward compatible
`:stable`. An unqualified Node pull therefore follows LTS. An absent published baseline stops automatic
updater publication; only separately authorized exact-SHA manual dispatch can
bootstrap publication. After an established published baseline, the normal
[owner-merged updater publication flow](publication.md) applies. Version tags use
`26.11.1-bN`, with packaging revisions allocated across the complete shared registry tag list
and both `node/*` and `node-current/*` source tag namespaces, and successful approved
publication creates `node-current/<immutable-version>`. No image digest, tag or
successful workflow run is claimed here.
