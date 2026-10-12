# Optional Node.js Current bundle

`bundles/node-current` supplies official stable Node.js Current Linux AMD64
and ARM64 prebuilts and their bundled npm/npx. The existing `node` bundle
continues to supply Node.js 24 LTS with its existing catalog, lock, image,
volume and consumer configuration.

**Planned, unpublished:** no Node Current image or publication receipt has
been verified. First publication requires separate explicit approval of the
exact full source revision and intended build/publication operation. No Quay
repository setup, image build, workflow dispatch, source tag or consumer
activation is part of this source change.

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
read-only at `/opt/bundledock/node-current` in a stock glibc language image.
Add `/opt/bundledock/node-current/bin` to PATH. Keep HOME, npm cache, global
prefix and projects outside the mount; follow the [Node consumer guidance](node.md#manual-consumer-use).
No user configuration filename or default consumer activation changes.

## Updates and publication

`bundledock tools upgrade --directory bundles/node-current` updates this
bundle's lock alone, using canonical tool ID `node`. The thin
`node-current-upgrade.yml` wrapper calls the shared lock-only updater daily at
06:50 UTC and supports manual dry runs. It never builds or dispatches publication.

`bundledock-node-current.yml` calls the existing shared publication workflow
with separate workflow, image (`quay.io/bundledock/node-current`), artifacts,
volume and source tag identity. An absent published baseline stops automatic
updater publication; only separately authorized exact-SHA manual dispatch can
bootstrap publication. After an established published baseline, the normal
[exact-head updater approval flow](publication.md) applies. Version tags use
`26.11.1-bN`, with registry-allocated packaging revisions, and successful approved
publication creates `node-current/<immutable-version>`. No image digest, tag or
successful workflow run is claimed here.
