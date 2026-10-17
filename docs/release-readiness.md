---
title: Release readiness
description: State of the bdock CLI release path and what remains the owner's decision.
weight: 60
---

# `bdock` CLI release readiness

Updated 2026-10-17. This replaces the draft of 2026-10-10. It is a status note, not a release
announcement and not approval to tag, publish or activate an image. The gap analysis against
the earlier draft is in [the design analysis](design/bdock-release-workflow/analysis.md); the
authoritative tool inventory is the [catalog and lock](toolbox.md).

## CLI release

- Owner decision 2026-10-17: "bdock wie smill". The CLI is released by a tag-driven CI workflow
  (`.github/workflows/release.yml`), for linux/amd64, linux/arm64 and darwin/arm64. Tags are
  `bdock/vMAJOR.MINOR.PATCH`; the first planned version is `0.1.0`. The owner procedure is
  [releasing](releasing.md); the design is [docs/design/bdock-release-workflow](design/bdock-release-workflow/spec.md).
- State: the workflow is **implemented on a branch and not merged**; no tag exists and no
  workflow has run. Archive building, version injection (`-X main.version`), `PROVENANCE`
  and `SHA256SUMS` are in `scripts/release.sh`, shared by local preparation and CI.
- Unverified until the first real run: the GitHub-side tag path (annotated-tag fetch, CI-run
  query, release lookup, `gh release create`), runner labels such as `ubuntu-24.04-arm`, and
  the `release` environment. The workflow's dry run covers verify, build and smoke only.
- Native evidence for the CLI will come from the smoke job on three native runners. It runs
  hermetic commands only and says nothing about `tools volume` or `tools upgrade`.

## What this release does not cover

- The OCI tool images and their registry publication, which follow [publication](publication.md).
- [Embedded dependency findings](development.md#embedded-dependency-findings) in Gitleaks,
  Syft and Trivy concern third-party binaries inside the toolbox image. The CLI archives do
  not contain them, so they neither block nor qualify a CLI release; they stay open for the
  image and must not be closed by rebuilding or patching third-party tools.
- Consumer activation of any image; Quay publication; authenticated agent workflows.

## Before the first tag

Apply the [release review requirement](development.md#release-review) to the untracked local
TODO list: every item resolved or explicitly deferred by the owner. Report builds and
workflow runs honestly: a passing source check is not release readiness, and pushes, merges,
schedules and bots do not authorize builds outside the exceptions in `AGENTS.md`.

Community files exist: [contributing guide](../CONTRIBUTING.md), templates, and the
[code of conduct](../CODE_OF_CONDUCT.md) with its reporting contact. BundleDock's own code is
licensed under [EUPL-1.2](../LICENSE); upstream tools keep their notices.
