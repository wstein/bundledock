# Optional Node.js bundle

`bundles/node` supplies the official stable Node.js LTS prebuilt Linux AMD64
and ARM64 runtime and its bundled npm/npx. It has its own catalog, lock, image,
volume and `/opt/bundledock/node` root. The toolbox and code-server defaults,
pins and consumer configuration are unchanged. No Ruby or generic language
bundle mechanism is included.

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
bundledock tools volume --image bundledock-node:poc --bundle node
```

Mount the printed image-derived `bundledock-node-<full image ID>` volume
read-only at `/opt/bundledock/node`. Use a stock glibc language image, such as
the existing Debian Trixie Go consumer; the scratch carrier only initializes
volumes. Add `/opt/bundledock/node/bin` to consumer PATH. The npm/npx forwarding
launchers invoke the bundled Node executable and original npm JavaScript by
absolute path, and prepend this bundle's bin for npm child processes. Arguments
and HOME pass through unchanged. No user configuration file is written.

Keep HOME, npm cache, projects and global packages outside the immutable mount.
npm normally caches under `$HOME/.npm`; configure a writable global prefix in
the consumer explicitly, for example:

```sh
export npm_config_prefix="$HOME/.local/share/npm"
export PATH="/opt/bundledock/node/bin:$npm_config_prefix/bin:$PATH"
```

Existing npm environment/configuration overrides are preserved. Select an
appropriate prefix yourself if your existing npmrc already configures one.
Native addons may require the consumer's compiler/system libraries; this
carrier does not install dependencies or build third-party tools from source.
Runtime compatibility beyond the Debian consumer has not been established.

## Updates, publication and evidence

`bundledock tools upgrade --directory bundles/node` updates this bundle alone.
The Node upgrade workflow runs daily at 07:00 UTC and offers manual dry runs
by default. It proposes only the Node lock and generated source inventory,
validates transferred lock/base SHA and artifact membership, and never builds
or dispatches native publication.

`bundledock-node.yml` is manual-only and requires a human dispatch on default
main with the full approved source SHA matching the run. It publishes to the
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
