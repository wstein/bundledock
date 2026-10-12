---
title: Draft release readiness
description: Unreleased capabilities, evidence limits, and decisions needed before a first tagged CLI release.
weight: 60
---

# v0.1 release readiness — draft

This is a local preparation note, not a release announcement or approval to tag,
publish, or activate an image. It summarizes the repository state inspected on
2026-10-10 at `731c6fdb4b0da3b2c5c8a55b47a84dc8877aceb0`, plus the contribution
and review templates introduced with this note. Later changes need their own
verification; the authoritative tool inventory is the
[catalog and lock](toolbox.md), not a count in these notes.

## Draft release notes

- BundleDock is a standalone repository with the original shared history
  preserved; Anchorage owns consumer devcontainer integration separately.
- The CLI supports tool upgrades and shared volume initialization. Local release
  preparation builds Linux AMD64 and ARM64 binaries with version injection,
  license, checksums, and compiler/source provenance.
- The scratch OCI carrier provides pinned tools for read-only shared volumes.
  Tool inputs and licenses live under `bundles/toolbox/`.
- Publication retains a durable published-tool baseline. The current
  [approval flow](publication.md) accepts one exact-head updater PR approval by
  wstein for source CI, guarded merge and frozen-lock publication across all
  four bundles. Manual publication of an exact approved `main` SHA remains
  available. Native build/smoke checks gate registry writes.
- A local Hugo site renders the existing guides using a verified official
  prebuilt dependency. Host documentation integration checks are opt-in.
- [Contribution guidance](../CONTRIBUTING.md), bug and feature templates, and a
  pull request template document focused changes and review evidence.
- BundleDock's own code is licensed under [EUPL-1.2](../LICENSE); upstream tools
  retain their own notices.

## Evidence and release blockers

The [verification record](development.md#verification-evidence) names historical
native AMD64 and ARM64 CI results and their exact revisions. Those results do
not verify every later change. The [local artifact checks](development.md#local-cli-artifacts)
cover artifact preparation; they do not demonstrate registry publication or
consumer activation. Obtain explicit user approval before collecting artifact, image, release, or
documentation build evidence for the final candidate, including both native
architectures for changed tool/image behavior, before calling it ready. Routine
source tests and vet remain automatic. Pushes, merges, schedules and bots alone
do not authorize builds. Lock-only updaters do not dispatch native builds;
the separate publisher validates the exact-head approval described above.

[Embedded dependency findings](development.md#embedded-dependency-findings)
remain open in Gitleaks, Syft, and Trivy. The recorded upstream inspection found
no patched official stable prebuilt at those pins. Recheck official stable
releases before release review; do not rebuild or patch third-party tools to
close the findings. Checksum and smoke success do not resolve them.

Before any release tag or publication, apply the
[release review requirement](development.md#release-review) to every item in the
untracked local TODO list. Unfinished or blocked items need resolution or an
explicit user decision to defer each one. This note does not defer any item.

A security reporting contact/channel and a code of conduct with an enforcement
owner still require user decisions; no such contact or policy is established by
the contribution templates. Confirm those decisions as part of the remaining
community standards review.

Quay publication, authenticated agent workflows, interactive terminal behavior,
and adoption of a new consumer image have separate verification needs; see the
[development limits](development.md#verification-evidence) and
[publication procedure](publication.md). No release tag, GitHub release, registry
publication, or consumer activation is performed by preparing this draft.
