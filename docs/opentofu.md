---
title: OpenTofu bundle
description: Inspect the prepared OpenTofu source carrier and its pending publication status.
---

# Optional OpenTofu bundle

[All bundles](bundles.md)

`bundles/opentofu` supplies the official stable prebuilt OpenTofu CLI (`tofu`)
for Linux AMD64 and ARM64. It has a separate catalog, lock, image, volume and
`/opt/bundledock/opentofu` mount. Providers are not bundled or pinned here;
projects manage provider versions, lock files, credentials and licenses.

<!-- BEGIN PUBLISHED BUNDLE -->
## Publication pending

This bundle is prepared in source. No image, native smoke result, source tag
or verified publication receipt is available. The planned repository is
`quay.io/bundledock/opentofu`; repository preparation is separate from source
verification. Initial publication requires explicit approval of the full main
source SHA through the manual publisher.
<!-- END PUBLISHED BUNDLE -->

<!-- BEGIN GENERATED TOOL INVENTORY -->
| Tool | Pinned version |
| --- | --- |
| opentofu | 1.13.1 |
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
retaining archive `LICENSE`, `README.md` and `CHANGELOG.md` under
`licenses/opentofu`. The full official MPL-2.0 license and source provenance
are also retained there. Traversal, links, special files, duplicate paths and
non-executable selected files are rejected. Existing regular-file manifests
and volume initialization verify hashes, modes and exact tree membership on
first and repeated initialization. A runtime launcher is unnecessary.

## Consumer use

After explicit approval of the intended build and revision:

```sh
make bundle-build BUNDLE=opentofu
make bundle-smoke BUNDLE=opentofu
make bundle-volume BUNDLE=opentofu
```

Mount the printed `bundledock-opentofu-<full image ID>` volume read-only at
`/opt/bundledock/opentofu`, and add `/opt/bundledock/opentofu/bin` to PATH.
Use a stock language consumer image; the scratch carrier only initializes
volumes. Native verification is prepared for the existing Debian Trixie
consumer on both supported architectures and remains unexecuted.

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

The prepared offline native smoke uses the built-in `terraform_data` resource
without downloading a provider. It checks init, validate, plan, apply, local
state, output, repeated volume initialization, concurrent read-only consumers,
and preservation of a consumer-owned `.tofurc`.

## Updates and publication

`bundledock tools upgrade --directory bundles/opentofu` resolves the CLI alone
within `>=1.13.1,<2.0.0`. The thin upgrade workflow runs at 06:50 UTC and manual
dry runs default to true. It changes only this lock and generated inventory;
upgrades do not build or dispatch publication.

The [shared publication flow](publication.md) preserves exact-head **wstein**
approval, source CI, SHA-guarded merge and frozen-lock publication for eligible
bot updater PRs. Manual publication requires the exact approved full main SHA.
Native AMD64 and ARM64 smoke must pass before promotion. Immutable versions
use `<upstream-version>-b<packaging-revision>` (initially `1.13.1-b1` when
unreserved); successful approved publication creates an annotated
`opentofu/<immutable-version>` source tag. Image build, tool execution, native
smoke, publication, tagging and consumer activation remain separately gated.

Source checks cover official recipe/pin rejection, synthetic archives and
notices, repeated volume initialization and tampering, bundle/registry parity,
version allocation, fake Docker adapters and shared upgrade/approval routing.
They establish source behavior only. Runtime compatibility, provider workflows,
vulnerability scans and native platform execution remain unverified.
