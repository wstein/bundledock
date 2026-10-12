---
title: Bundle overview
description: Choose utilities, runtimes, an editor or agents, and find each bundle's source inventory and publication receipt.
weight: 15
---

# Bundle overview

BundleDock packages pinned development tools into OCI carrier images for Linux
AMD64 and ARM64. Initialize a named volume from a carrier, then mount the tools
read-only in a compatible development container. The carrier supplies files;
your consumer container supplies the operating system and writable workspace.
Use stock language images for language toolchains and add the toolbox for
common utilities. See the [quick start](../README.md#quick-start) and
[consumer requirements](architecture.md#consumer-compatibility-and-build-layout).

Each bundle has its own desired package list, catalog, verified lock, image,
volume and `/opt/bundledock/<bundle>` root. A package is an upstream tool or
runtime inside a bundle; a bundle is the independently versioned carrier that
ships those packages. Select bundles independently with `BUNDLE=<bundle>` when
using the Makefile. Keep HOME, credentials, caches, configuration and project
state writable outside these immutable roots. Each guide explains its specific
consumer requirements and verification limits.

## Utilities

| Bundle | Purpose and guide | Registry tags |
| --- | --- | --- |
| OpenTofu (`opentofu`) | [Infrastructure CLI (`tofu`)](opentofu.md); providers and project state stay with the consumer. Source prepared; publication pending. | [Planned OpenTofu repository](https://quay.io/repository/bundledock/opentofu?tab=tags) |
| Toolbox (`toolbox`) | [Common development utilities](toolbox.md), including search, shells, GitHub/registry tools and source checks. | [Toolbox tags](https://quay.io/repository/bundledock/toolbox?tab=tags) |

## Runtimes

| Bundle | Purpose and guide | Registry tags |
| --- | --- | --- |
| Bun (`bun`) | [Bun runtime and bunx](bun.md). | [Bun tags](https://quay.io/repository/bundledock/bun?tab=tags) |
| Node.js (`node`) | [Node.js LTS with npm/npx](node.md). | [Node LTS tags](https://quay.io/repository/bundledock/node?tab=tags&tag=lts) |
| Node.js Current (`node-current`) | [Node.js Current with npm/npx](node-current.md), resolved independently from LTS. | [Node Current tags](https://quay.io/repository/bundledock/node?tab=tags&tag=current) |

Node LTS and Current have separate locks and volumes, but share the
`quay.io/bundledock/node` repository. Their `lts` and `current` aliases select
the published channels; see the guides for immutable digest references.

## Editors

| Bundle | Purpose and guide | Registry tags |
| --- | --- | --- |
| code-server (`code-server`) | [Standalone browser editor](code-server.md); starting and exposing its service is a consumer choice. | [code-server tags](https://quay.io/repository/bundledock/code-server?tab=tags) |

## Agents

| Bundle | Purpose and guide | Registry tags |
| --- | --- | --- |
| Antigravity (`agy`) | [Official Antigravity CLI](agy.md). | [Antigravity tags](https://quay.io/repository/bundledock/agy?tab=tags) |
| Claude Code (`claude`) | [Official Claude CLI](claude.md). | [Claude Code tags](https://quay.io/repository/bundledock/claude?tab=tags) |
| Codex (`codex`) | [Official Codex package and native helpers](codex.md). | [Codex tags](https://quay.io/repository/bundledock/codex?tab=tags) |

The agent carriers have published receipts in their guides. The current source
toolbox excludes these agents; the older published toolbox still contains them.
Choose the exact carrier recorded in the guide and follow your consumer
project's adoption instructions.

## Source inventory and published images

A guide's generated inventory describes its current source lock. Its published
bundle section records a specific image version, immutable index digest, exact
source commit and publication run. These can differ: editing a lock or merging
source does not rebuild an existing carrier. Follow those receipts when choosing
an image; no overview table duplicates the managed package versions or digests.

The current toolbox source adds utilities and removes agents relative to its
recorded published image. Those source changes have not been published in that
image. OpenTofu has no verified successful publication receipt, image digest or
source tag; its source inventory alone does not establish a usable carrier.

Quay tag listings show published registry state. Moving aliases such as `stable`,
`latest`, `lts` and `current` can change; use an immutable index digest for a
reproducible consumer. Single-package carriers use upstream versions with a
packaging revision, while toolbox versions use the documented date/run scheme.
See [publication and versioning](publication.md),
[lock upgrades](toolbox.md#upgrade-the-lock) and
[development verification](development.md) for the canonical procedures.
Builds, publication, tagging and consumer activation each retain their explicit
approval requirements described in those guides.
