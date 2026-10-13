---
title: Node.js LTS bundle
description: Use the independent Node.js LTS carrier with npm/npx and a writable consumer workspace.
---

# Optional Node.js bundle

[All bundles](bundles.md)

`bundles/node` supplies the official stable Node.js LTS prebuilt Linux AMD64
and ARM64 runtime and its bundled npm/npx. It has its own catalog, lock, image,
volume and `/opt/bdock/node` root. The toolbox and code-server defaults,
pins and consumer configuration are unchanged. No Ruby or generic language
bundle mechanism is included.

<!-- BEGIN PUBLISHED BUNDLE -->
<!-- publication-record: {"attempt":"1","commit":"9f25e0065885dce912658986bf26c178dddbaf36","digest":"sha256:295cce35e9f580db41dfdc97bd30243d9f0cf388556a8a8d87a6c59ee51e6a8c","image":"quay.io/bundledock/node","reference":"quay.io/bundledock/node@sha256:295cce35e9f580db41dfdc97bd30243d9f0cf388556a8a8d87a6c59ee51e6a8c","run_id":"38192775173","source":"https://github.com/wstein/bundledock","tag":"2026.10.12-b1.1"} -->
## Published bundle

Version: `2026.10.12-b1.1`

Index digest: `sha256:295cce35e9f580db41dfdc97bd30243d9f0cf388556a8a8d87a6c59ee51e6a8c`

Source: [`9f25e0065885dce912658986bf26c178dddbaf36`](https://github.com/wstein/bundledock/commit/9f25e0065885dce912658986bf26c178dddbaf36)

Publication: [run 38192775173, attempt 1](https://github.com/wstein/bundledock/actions/runs/38192775173/attempts/1)

Pull this published carrier by its immutable index digest:

```sh
docker pull quay.io/bundledock/node@sha256:295cce35e9f580db41dfdc97bd30243d9f0cf388556a8a8d87a6c59ee51e6a8c
```

The source inventory below describes the current lock; this publication retains its exact source revision.
<!-- END PUBLISHED BUNDLE -->

<!-- BEGIN GENERATED TOOL INVENTORY -->
| Tool | Pinned version |
| --- | --- |
| node | 24.21.0 |
<!-- END GENERATED TOOL INVENTORY -->

The official [distribution index](https://nodejs.org/dist/index.json), checked
October 12, 2026, identifies v24.21.0 (September 7, 2026) as the latest LTS,
Krypton, with npm 11.19.0. Both official tar.gz downloads are verified against
[SHASUMS256.txt](https://nodejs.org/dist/v24.21.0/SHASUMS256.txt) before reading
archive members. The LTS resolver stays within the reviewed 24.x constraint;
review a major-line change separately. A newer matching LTS missing either
architecture or its checksum fails closed. Current, prerelease, nightly and
source distributions are not selected.

The full archive tree is preserved under `payload`, including headers, npm's
relative JavaScript resources, metadata, and all upstream LICENSE/notices.
Only optional `bin/corepack` and `lib/node_modules/corepack` are omitted;
Corepack is neither required nor activated. Upstream binaries and JavaScript
are unmodified. The npm/npx links retain their exact upstream relative targets.
The Node profile accepts precisely those two links to direct regular files;
it rejects other links, chains, hardlinks, escapes, missing targets, parent
links, special files and collisions. Explicit initializer profile opt-in and
matching official provenance are both required. The existing checksum and
volume verification covers literal link targets and exact tree membership.

## Manual consumer use

Builds, native execution, publication and consumer activation require separate
explicit approval for their intended operation and source revision. After an
approved image build and volume initialization:

```sh
make bundle-build BUNDLE=node
make bundle-smoke BUNDLE=node
make bundle-volume BUNDLE=node
```

Mount the printed image-derived `bundledock-node-<full image ID>` volume
read-only at `/opt/bdock/node`. Use a stock glibc language image, such as
the existing Debian Trixie Go consumer; the scratch carrier only initializes
volumes. Add `/opt/bdock/node/bin` to consumer PATH. The npm/npx forwarding
launchers invoke the bundled Node executable and original npm JavaScript by
absolute path, and prepend this bundle's bin for npm child processes. Arguments
and HOME pass through unchanged. No user configuration file is written.

Keep HOME, npm cache, projects and global packages outside the immutable mount.
npm normally caches under `$HOME/.npm`; configure a writable global prefix in
the consumer explicitly, for example:

```sh
export npm_config_prefix="$HOME/.local/share/npm"
export PATH="/opt/bdock/node/bin:$npm_config_prefix/bin:$PATH"
```

Existing npm environment/configuration overrides are preserved. Select an
appropriate prefix yourself if your existing npmrc already configures one.
Native addons may require the consumer's compiler/system libraries; this
carrier does not install dependencies or build third-party tools from source.
Runtime compatibility beyond the Debian consumer has not been established.

## Updates, publication and evidence

`bdock tools upgrade --directory bundles/node` updates this bundle alone.
The **Propose upgrades** workflow with `bundle=node` runs daily at 06:50 UTC and offers manual dry runs
by default. It proposes only the Node lock and generated source inventory,
validates transferred lock/base SHA and artifact membership, and never builds
or dispatches native publication.

`publish-bundles.yml` supports the shared [automatic updater approval
flow](publication.md): an actual human **wstein** merge authorizes frozen-lock publication after
successful exact merged-main CI. Manual dispatch on default
`main` with the full approved source SHA matching the run remains available.
It publishes to the
existing Quay namespace's separate `node` repository with separate artifacts
and workflow identity. Only an authenticated exact HTTP 404 for this stable
manifest permits the initial absent baseline. Each native AMD64/ARM64 image
stages after its own read-only consumer smoke; version/stable/latest promotion
waits for the complete matrix and checks newer eligible runs.

Both v24.21.0 archives were privately downloaded, publisher SHA256 verified,
and inspected without extraction or execution: each has 5,888 members and
three upstream symlinks (npm, npx, omitted Corepack). Source fixtures cover
both native layouts, hostile graphs, explicit profile and repeated initializer
verification, launchers and fake CI adapters. This is source/structure evidence
only. OCI builds, Node/npm/npx execution, consumer smoke and vulnerability
scans on both native platforms remain deferred pending explicit approval.

Successful approved publication also creates an annotated `node/<immutable-version>`
Git tag at the verified image source commit. Its annotation binds the immutable
image digest and workflow attempt; see [source tags](publication.md#source-tags-for-published-bundles)
for verification, conflicts and separately authorized backfills.

## Node channel aliases

Both Node source bundles target `quay.io/bundledock/node`. An approved LTS
publication updates `:lts`, `:latest` and backward compatible `:stable`; an
approved Current publication updates only `:current`. Thus `node:latest` and
an unqualified Node pull select LTS. Current remains [planned and unpublished](node-current.md).
These source rules do not move any existing alias. The immutable published LTS
reference above remains the verified consumer pin.

Packaging revisions are reserved across all authenticated shared repository
tags and both `node/*` and `node-current/*` source tags. A release later moving
from Current to LTS receives a new unused `upstream-bN` tag. Source identities,
recipes, locks, mount paths and volumes remain separate.
