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
<!-- publication-record: {"attempt":"1","bundle":"node-current","commit":"8b283660858c413f09d6eba3121557a5fb02186d","created":"2026-10-21T14:17:59Z","digest":"sha256:5ae42b68a755674753faa95bf71f324971fcaf59d6ec03c76b25d129ede5efed","image":"quay.io/bundledock/node","lock_sha256":"65ad022f717934d44ed4cf7f96c65d752674b3fb90d6b74fb0a269b392c2efca","reference":"quay.io/bundledock/node@sha256:5ae42b68a755674753faa95bf71f324971fcaf59d6ec03c76b25d129ede5efed","run_attempt":"1","run_id":"39615361999","source":"https://github.com/wstein/bundledock","tag":"26.12.1-b1"} -->
## Published bundle

Version: `26.12.1-b1`

Index digest: `sha256:5ae42b68a755674753faa95bf71f324971fcaf59d6ec03c76b25d129ede5efed`

Source: [`8b283660858c413f09d6eba3121557a5fb02186d`](https://github.com/wstein/bundledock/commit/8b283660858c413f09d6eba3121557a5fb02186d)

Publication: [run 39615361999, attempt 1](https://github.com/wstein/bundledock/actions/runs/39615361999/attempts/1)

Pull this published carrier by its immutable index digest:

```sh
docker pull quay.io/bundledock/node@sha256:5ae42b68a755674753faa95bf71f324971fcaf59d6ec03c76b25d129ede5efed
```

The source inventory below describes the current lock; this publication retains its exact source revision.
<!-- END PUBLISHED BUNDLE -->

<!-- BEGIN GENERATED TOOL INVENTORY -->
| Tool | Pinned version |
| --- | --- |
| node | 26.12.1 |
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
