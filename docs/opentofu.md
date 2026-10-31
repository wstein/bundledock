---
title: OpenTofu bundle
description: Retired OpenTofu CLI bundle; use tofu from the toolbox, and inspect the last publication receipt of the existing images.
---

# OpenTofu bundle (retired)

[All bundles](bundles.md)

> **Status: retired on 2026-10-31.** Get `tofu` from the [toolbox](toolbox.md#opentofu),
> which ships the same recipe, pin and evidence. This bundle receives no further
> updates and no new publication. Existing images stay on Quay, immutable and
> pullable by digest, so current pins keep working; nothing is deleted. The source
> and this page remain for reference.

`bundles/opentofu` supplies the official stable prebuilt OpenTofu CLI (`tofu`)
for Linux AMD64 and ARM64. It has a separate catalog, lock, image, volume and
`/opt/bdock/opentofu` mount. Providers are not bundled or pinned here;
projects manage provider versions, lock files, credentials and licenses.

The default [toolbox](toolbox.md#opentofu) ships `tofu` at the same pin (owner
decision 2026-10-22). The sections below describe the retired bundle as it was
last published.

<!-- BEGIN PUBLISHED BUNDLE -->
<!-- publication-record: {"attempt":"1","bundle":"opentofu","commit":"e22571142a2f6371844dbd948df94abc602ed278","created":"2026-10-22T13:44:39Z","digest":"sha256:803d6e5d20547e80e1654b417c0406e6d205915dfb65fbdb7d0411ede9ca56bb","image":"quay.io/bundledock/opentofu","lock_sha256":"62fe0970627802f84b6c87d9884763bf27d050cd546ecc0909af3bbe774f86cc","reference":"quay.io/bundledock/opentofu@sha256:803d6e5d20547e80e1654b417c0406e6d205915dfb65fbdb7d0411ede9ca56bb","run_attempt":"1","run_id":"39788387031","source":"https://github.com/wstein/bundledock","tag":"1.13.2-b1"} -->
## Published bundle

Version: `1.13.2-b1`

Index digest: `sha256:803d6e5d20547e80e1654b417c0406e6d205915dfb65fbdb7d0411ede9ca56bb`

Source: [`e22571142a2f6371844dbd948df94abc602ed278`](https://github.com/wstein/bundledock/commit/e22571142a2f6371844dbd948df94abc602ed278)

Publication: [run 39788387031, attempt 1](https://github.com/wstein/bundledock/actions/runs/39788387031/attempts/1)

Pull this published carrier by its immutable index digest:

```sh
docker pull quay.io/bundledock/opentofu@sha256:803d6e5d20547e80e1654b417c0406e6d205915dfb65fbdb7d0411ede9ca56bb
```

The source inventory below describes the current lock; this publication retains its exact source revision.
<!-- END PUBLISHED BUNDLE -->

<!-- BEGIN GENERATED TOOL INVENTORY -->
| Tool | Pinned version |
| --- | --- |
| opentofu | 1.13.2 |
<!-- END GENERATED TOOL INVENTORY -->

The [official v1.13.1 release](https://github.com/opentofu/opentofu/releases/tag/v1.13.1)
was checked October 12, 2026: it is stable, with both
`tofu_1.13.1_linux_amd64.tar.gz` and `tofu_1.13.1_linux_arm64.tar.gz` assets.
Pins use the publisher's [SHA256SUMS](https://github.com/opentofu/opentofu/releases/download/v1.13.1/tofu_1.13.1_SHA256SUMS).
The release also provides GPG and Sigstore signatures. The existing manager
verifies publisher SHA256 values; signature verification is not implemented.
Official prebuilt binaries remain unchanged. No Terraform binaries are
rebranded and no third-party source compilation occurs.

The safe tar extractor selects `tofu` as the regular executable `bin/tofu`,
retaining archive `LICENSE`, `README.md` and `CHANGELOG.md` as
`licenses/opentofu/...` members in `licenses.tar.gz`. The full official MPL-2.0
license and source provenance remain complete. A plain `licenses.index.json`
maps the retained members and their hashes; see [notice access](toolbox.md#retained-notices)
for the offline carrier helper and the change from loose installed paths. Traversal, links, special files, duplicate paths and
non-executable selected files are rejected. Existing regular-file manifests
and volume initialization verify hashes, modes and exact tree membership on
first and repeated initialization. The smoke adapter also reads every OpenTofu
notice through the carrier helper before checking the read-only consumer.
A runtime launcher is unnecessary.

## Consumer use

After explicit approval of the intended build and revision:

```sh
make bundle-build BUNDLE=opentofu
make bundle-smoke BUNDLE=opentofu
make bundle-volume BUNDLE=opentofu
```

Mount the printed `bundledock-opentofu-<full image ID>` volume read-only at
`/opt/bdock/opentofu`, and add `/opt/bdock/opentofu/bin` to PATH.
Use a stock language consumer image; the scratch carrier only initializes
volumes. [Publication run 38314225387, attempt 1](https://github.com/wstein/bundledock/actions/runs/38314225387/attempts/1)
built and passed native smoke on Linux AMD64 and ARM64 with the Debian Trixie
consumer at source
[`84216f3a45e35ba741cc8245f96803b5b9a2a31f`](https://github.com/wstein/bundledock/commit/84216f3a45e35ba741cc8245f96803b5b9a2a31f).
That evidence covers the exact version and immutable index recorded in the
[published bundle receipt](#published-bundle), rather than later source locks.

Keep HOME, project directories, backend/local state, provider plugins, and
caches writable outside the immutable bundle. OpenTofu's existing `.tofurc`,
`TF_CLI_CONFIG_FILE`, provider installation settings and environment pass
through unchanged. The carrier writes no user configuration and sets no
provider, cache, state or data-directory overrides. Project `.terraform` data
and local state normally live in the writable project directory. If your
consumer opts into `TF_DATA_DIR` or `TF_PLUGIN_CACHE_DIR`, create writable
locations that agree with its existing configuration; never point them into
the bundle. Remote backends and provider installation may require networking
and credentials supplied by the consumer.

The offline native smoke uses the built-in `terraform_data` resource
without downloading a provider. It checks init, validate, plan, apply, local
state, output, repeated volume initialization, concurrent read-only consumers,
and preservation of a consumer-owned `.tofurc`.

## Updates and publication

Retired: the **Propose upgrades** workflow no longer resolves this bundle, and
**Publish bundles** neither offers nor accepts it, so no new image or source tag
is created. The toolbox carries `tofu` updates. The remainder of this section
documents how the existing images were produced.

`bdock tools upgrade --directory bundles/opentofu` resolved the CLI alone
within `>=1.13.1,<2.0.0`. The [shared publication flow](publication.md) verified
an actual human **wstein** owner merge, exact merged-main CI and frozen-lock
publication. Native AMD64 and ARM64 smoke had to pass before promotion. Immutable versions
use `<upstream-version>-b<packaging-revision>` (initially `1.13.1-b1` when
unreserved); successful approved publication creates an annotated
`opentofu/<immutable-version>` source tag. Image build, tool execution, native
smoke, publication, tagging and consumer activation remain separately gated.

Source checks cover official recipe/pin rejection, synthetic archives and
notices, repeated volume initialization and tampering, bundle/registry parity,
version allocation, fake Docker adapters and shared upgrade/approval routing.
They establish source behavior only. The native publication evidence above
covers the recorded carrier and offline smoke scenario. External provider
workflows, other consumer environments and vulnerability scans remain
unverified here; future source locks require their own native verification.
