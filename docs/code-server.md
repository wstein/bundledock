---
title: code-server bundle
description: Use the standalone browser editor with consumer-owned configuration and service startup.
---

# Optional code-server bundle

[All bundles](bundles.md)

The separate `bundles/code-server` carrier supplies the official stable
code-server standalone distribution for Linux AMD64 and ARM64. It is optional:
the toolbox catalog, image pins, mount path, volume names and consumer
configuration remain unchanged. No browser service starts automatically.

The current standalone pin comes from this bundle's own lock:

<!-- BEGIN PUBLISHED BUNDLE -->
<!-- publication-record: {"attempt":"1","commit":"9f25e0065885dce912658986bf26c178dddbaf36","digest":"sha256:07d3452052c1c5cb34cd6545b1975cd3dda589b9ee66710e5c765ea541ae49f1","image":"quay.io/bundledock/code-server","reference":"quay.io/bundledock/code-server@sha256:07d3452052c1c5cb34cd6545b1975cd3dda589b9ee66710e5c765ea541ae49f1","run_id":"38192754108","source":"https://github.com/wstein/bundledock","tag":"2026.10.12-b1.1"} -->
## Published bundle

Version: `2026.10.12-b1.1`

Index digest: `sha256:07d3452052c1c5cb34cd6545b1975cd3dda589b9ee66710e5c765ea541ae49f1`

Source: [`9f25e0065885dce912658986bf26c178dddbaf36`](https://github.com/wstein/bundledock/commit/9f25e0065885dce912658986bf26c178dddbaf36)

Publication: [run 38192754108, attempt 1](https://github.com/wstein/bundledock/actions/runs/38192754108/attempts/1)

Pull this published carrier by its immutable index digest:

```sh
docker pull quay.io/bundledock/code-server@sha256:07d3452052c1c5cb34cd6545b1975cd3dda589b9ee66710e5c765ea541ae49f1
```

The source inventory below describes the current lock; this publication retains its exact source revision.
<!-- END PUBLISHED BUNDLE -->

<!-- BEGIN GENERATED TOOL INVENTORY -->
| Tool | Pinned version |
| --- | --- |
| code-server | 4.141.0 |
<!-- END GENERATED TOOL INVENTORY -->

The initial 4.141.0 pin was released October 8, 2026, with VS Code 1.141.0;
see the [official release](https://github.com/coder/code-server/releases/tag/v4.141.0).
Each architecture has a pinned SHA-256. The builder verifies the entire
archive before reading members. The full payload retains bundled Node, native
modules, VS Code assets, built-in extensions, MIT license and all third-party
notices under `/opt/bundledock/code-server/payload`.

This standalone release needs a glibc consumer with glibc >= 2.28 and
GLIBCXX >= 3.4.21; use a Debian-based stock language image. The scratch carrier
only initializes a volume. Alpine is used to run the static BundleDock builder,
not to run code-server. There is no npm installation, source build, dependency
patching or additional extension installation. See the upstream
[standalone requirements](https://coder.com/docs/code-server/install#standalone-releases).

## Build and initialize manually

Any image build requires explicit user approval of the operation and exact
source revision. These commands are a manual recipe, not a publication or
activation workflow:

```sh
make bundle-build BUNDLE=code-server
make bundle-smoke BUNDLE=code-server
make bundle-volume BUNDLE=code-server
```

The initializer resolves the image tag once and prints a volume named
`bundledock-code-server-<full image ID>`. The code-server image explicitly enables
its dedicated initializer profile. An arbitrary `tools-init --source` does not
enable symbolic links; both caller opt-in and matching validated manifest
metadata are required. Regular-file toolbox initialization keeps its default
behavior.

Mount the printed volume read-only at `/opt/bundledock/code-server`, alongside
the separate toolbox mount. Add `/opt/bundledock/code-server/bin` to the
consumer's PATH if desired. The forwarding launcher preserves the upstream
launcher's relative access to bundled Node and resources. The initializer
preserves twelve internal `node_modules/.bin` symbolic links with their literal
targets. It rejects links outside that directory, absolute/escaping targets,
chains, directory or missing targets, hardlinks, collisions and special files.
The manifest records link types and target text; checksums cover that text,
regular-file contents and the manifest. Initialization and repeat verification
check exact tree membership, link targets and regular-file modes/content.

## Start an authenticated browser session

Set `CODE_SERVER_VOLUME` to the printed volume name. This disposable manual
example uses the existing stock Go consumer image with a writable temporary
home. It binds the host port only to loopback and retains password
authentication. The upstream service creates its password in the usual
`$HOME/.config/code-server/config.yaml`; read it inside the consumer without
committing it. All configuration, data, cache and extension writes stay outside
the read-only bundle.

```sh
docker run --rm -it \
  --user "$(id -u):$(id -g)" \
  --env HOME=/tmp/code-server-home \
  --tmpfs /tmp:rw,mode=1777 \
  --mount "type=volume,src=$CODE_SERVER_VOLUME,dst=/opt/bundledock/code-server,readonly" \
  --publish 127.0.0.1:8080:8080 \
  --entrypoint /bin/sh \
  mcr.microsoft.com/devcontainers/go:2-1.27-trixie \
  -c 'mkdir -p "$HOME"; exec /opt/bundledock/code-server/bin/code-server --bind-addr 0.0.0.0:8080 --auth password /tmp'
```

For a persistent consumer, use its existing writable home and workspace mounts.
Do not put HOME, configuration, caches, user data or installed extensions in the
bundle. Remote access, reverse proxies, credentials, automatic startup and
consumer configuration changes require a separate explicit decision.

## Update and verification boundary

Resolve only this bundle's inputs with:

```sh
bundledock tools upgrade --directory bundles/code-server
```

The separate code-server upgrade workflow runs daily at 06:50 UTC and offers
manual dry runs by default. It proposes only this lock and its generated source
inventory. The existing toolbox upgrade workflow stays separate. Neither
upgrade workflow builds or publishes images. Future stable archives must pass
the same structural profile; changed archive layouts fail closed until reviewed.

The separate **Publish code-server** workflow
(`.github/workflows/bundledock-code-server.yml`) supports the shared
[automatic updater approval flow](publication.md): one exact-head approval by
**wstein** authorizes source CI, guarded merge and frozen-lock publication.
Manual dispatch on default `main` with the exact full approved SHA remains
available. It uses the
existing `REPOSITORY_URL` Quay namespace and publishes to its separate
`code-server` repository, with distinct run/attempt artifacts. Each native AMD64
or ARM64 image stages only after its own dedicated read-only volume and
password-service smoke passes. Final version, stable and latest tag
promotion requires the entire native matrix to succeed; if one platform fails,
the other platform may already have staged. The publisher checks its own
workflow identity before promotion. For an initial publication, only an
authenticated HTTP 404 for this repository's `stable` manifest permits an absent baseline;
authentication, transport and other baseline failures stay closed. No workflow
was dispatched as part of adding these source definitions.

The initial source implementation was checked with Go 1.27.2 on Linux ARM64.
Both official 4.141.0 archives were downloaded privately, checksum verified,
and inspected without extraction or execution: AMD64 has 7,153 entries, ARM64
has 7,157 entries, and each contains twelve relative symbolic links to direct
regular files. Synthetic source tests cover the complete observed link set
on both architectures and hostile archive/volume changes. This establishes
source and structural evidence only. OCI builds, code-server execution,
read-only consumer/browser smoke tests, vulnerability scans and native runtime
coverage on both architectures remain pending explicit build/runtime approval.
Go 1.25 is the minimum source toolchain; it was not separately tested here.

Successful approved publication also creates an annotated `code-server/<immutable-version>`
Git tag at the verified image source commit. Its annotation binds the immutable
image digest and workflow attempt; see [source tags](publication.md#source-tags-for-published-bundles)
for verification, conflicts and separately authorized backfills.
