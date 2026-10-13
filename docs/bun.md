---
title: Bun bundle
description: Use the official Bun runtime and bunx from a shared, read-only carrier.
---

# Optional Bun bundle

[All bundles](bundles.md)

`bundles/bun` supplies official stable prebuilt Bun for Linux AMD64 and ARM64,
with its own catalog, lock, image, volume and `/opt/bundledock/bun` mount.

<!-- BEGIN PUBLISHED BUNDLE -->
<!-- publication-record: {"attempt":"1","commit":"4fd4d161efafea4f25da4839582e22d7e0a8ee91","digest":"sha256:730dd49c9b3c72615977204da4715bb46c828f5c92b962af155ee1099191b8c7","image":"quay.io/bundledock/bun","reference":"quay.io/bundledock/bun@sha256:730dd49c9b3c72615977204da4715bb46c828f5c92b962af155ee1099191b8c7","run_id":"38198677231","source":"https://github.com/wstein/bundledock","tag":"2026.10.12-b1.1"} -->
## Published bundle

Version: `2026.10.12-b1.1`

Index digest: `sha256:730dd49c9b3c72615977204da4715bb46c828f5c92b962af155ee1099191b8c7`

Source: [`4fd4d161efafea4f25da4839582e22d7e0a8ee91`](https://github.com/wstein/bundledock/commit/4fd4d161efafea4f25da4839582e22d7e0a8ee91)

Publication: [run 38198677231, attempt 1](https://github.com/wstein/bundledock/actions/runs/38198677231/attempts/1)

Pull this published carrier by its immutable index digest:

```sh
docker pull quay.io/bundledock/bun@sha256:730dd49c9b3c72615977204da4715bb46c828f5c92b962af155ee1099191b8c7
```

The source inventory below describes the current lock; this publication retains its exact source revision.
<!-- END PUBLISHED BUNDLE -->

<!-- BEGIN GENERATED TOOL INVENTORY -->
| Tool | Pinned version |
| --- | --- |
| bun | 1.4.3 |
<!-- END GENERATED TOOL INVENTORY -->

The [official release](https://github.com/oven-sh/bun/releases/tag/bun-v1.4.3),
checked October 12, 2026, is Bun 1.4.3 (October 10). AMD64 uses
`bun-linux-x64-baseline.zip`, avoiding the optimized build's AVX2 requirement;
ARM64 uses `bun-linux-aarch64.zip`. Both downloads were privately verified
against the publisher's [SHASUMS256.txt](https://github.com/oven-sh/bun/releases/download/bun-v1.4.3/SHASUMS256.txt)
and GitHub asset digests, then inspected without extraction or execution.
Each ZIP contains its platform root directory and one executable, `bun`.
The reviewed range stays below 2.0.0. Prereleases, canaries, source builds and
optimized x64 assets are not selected. A missing architecture or published
checksum fails validation before a new lock is accepted.

Every regular archive resource is preserved under `payload`; the root must
match the pin's exact official platform asset. Archive symlinks, special
files, traversal, duplicate paths, collisions with generated files and
non-executable Bun binaries are rejected. Download and expanded-size bounds
limit resource use. Upstream binaries are unchanged. The ZIPs contain no
license files; `licenses/bun/LICENSE.md` retains the full official source
notice for Bun, JavaScriptCore/WebKit, linked libraries and polyfills, with
upstream source and relinking references. `SOURCE` records provenance.
The binary's embedded notices remain intact. This records upstream evidence
and does not claim a licensing or security audit.

## Manual consumer use

Build, native execution, publication and consumer activation each require
explicit approval for the intended operation and source revision. After an
approved image build:

```sh
make bundle-build BUNDLE=bun
make bundle-smoke BUNDLE=bun
make bundle-volume BUNDLE=bun
```

Mount the printed `bundledock-bun-<full image ID>` volume read-only at
`/opt/bundledock/bun`, and add `/opt/bundledock/bun/bin` to PATH. Use a stock
glibc language consumer image, such as the existing Debian Trixie Go image.
The scratch carrier initializes volumes; it is not a consumer runtime.
The `bun` and `bunx` shell launchers forward every argument and the environment
unchanged. `payload/bunx -> bun` is the single generated relative link, preserving
[Bun's native basename dispatch](https://github.com/oven-sh/bun/blob/bun-v1.4.3/src/runtime/cli/mod.rs).
Only this exact link to a direct executable regular file is permitted by the
Bun profile. The initializer requires explicit `bun-runtime` opt-in and matching
official manifest provenance, and checks link text, hashes, modes and exact
tree membership on first and repeated initialization.

Keep HOME, project files, package caches and global installations outside the
immutable bundle. Bun normally uses `$HOME/.bun` for writable installation
state. To choose other writable locations explicitly in your consumer:

```sh
export BUN_INSTALL="$HOME/.bun"
export BUN_INSTALL_CACHE_DIR="$HOME/.cache/bun/install"
export BUN_INSTALL_GLOBAL_DIR="$HOME/.local/share/bun/install/global"
export BUN_INSTALL_BIN="$HOME/.local/bin"
export PATH="/opt/bundledock/bun/bin:$BUN_INSTALL_BIN:$PATH"
```

Existing Bun environment and `bunfig.toml` settings pass through; no user
configuration is written or silently overridden. Select writable paths that
agree with your existing settings. Native addons may need system libraries
or a compiler in the consumer. Compatibility beyond the selected glibc
consumer remains unverified.

## Updates, publication and evidence

`bdock tools upgrade --directory bundles/bun` updates Bun alone. The
separate upgrade workflow runs daily at 06:50 UTC alongside the other
bundle proposals, and offers manual dry runs by default. It updates only the
Bun lock and generated source inventory. The transfer validates base SHA,
artifact membership and pins before creating a topic. It never builds an
image or dispatches publication.

`bundledock-bun.yml` supports the shared [automatic updater approval
flow](publication.md): one exact-head approval by **wstein** authorizes source
CI, guarded merge and frozen-lock publication. Manual dispatch on default
`main` with the full explicitly approved source SHA matching the run remains
available. It uses the existing
Quay namespace's separate `bun` repository, artifacts and workflow identity.
Only an authenticated exact HTTP 404 for this repository's stable manifest
permits the initial absent baseline. Native AMD64 and ARM64 builds each require
read-only consumer smoke before staging; promotion waits for the whole matrix
and rejects newer eligible runs.

Source fixtures cover both platform roots, hostile ZIP graphs, arguments and
environment forwarding via shell stand-ins, profile/provenance opt-in,
repeated initialization and tampering, registry/workflow identity, and fake
upgrade/CI adapters. Source checks do not verify native execution. OCI builds,
Bun/bunx execution, native consumer smoke and vulnerability scans remain
pending explicit approval. Existing bundle defaults are unchanged; Ruby is
outside this change.

Successful approved publication also creates an annotated `bun/<immutable-version>`
Git tag at the verified image source commit. Its annotation binds the immutable
image digest and workflow attempt; see [source tags](publication.md#source-tags-for-published-bundles)
for verification, conflicts and separately authorized backfills.
