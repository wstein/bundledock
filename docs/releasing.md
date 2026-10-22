---
title: Releasing the bdock CLI
description: Owner procedure for the tag-driven bdock CLI release.
---

# Releasing the `bdock` CLI

Releases of the `bdock` command-line tool are made by CI from a tag. This page is the owner
procedure; the design and its review record are in
[docs/design/bdock-release-workflow](design/bdock-release-workflow/spec.md). It covers the CLI
archives only. OCI tool images follow [publication](publication.md) and are unaffected.
Agents and contributors never create, move or delete tags, never start the workflow and never
run `make release`.

Approval: there is no standing approval. Every tag and release, including later ones, needs
Werner Stein's explicit approval each time; an approval for one tag covers only that tag.
Without a reviewer rule on the `release` environment, a tag push by the owner that passes the
checks publishes immediately, with no pause. Agents never push tags.

Authorization: the owner decided on 2026-10-17 "bdock wie smill" (recorded in `AGENTS.md`).
Pushing a tag `bdock/vMAJOR.MINOR.PATCH` is the publication approval for that tag's release.

## What the workflow does

`.github/workflows/release.yml` runs on a pushed tag `bdock/vX.Y.Z` (exact pattern; `bdock/v1.2.3-rc1`
and every image tag such as `toolbox/2026.10.15-b98.1` start no release run) and on a manual
dispatch (dry run, never publishes).

1. **verify**: owner-only; annotated tag; tag commit equals the run commit and is on `main`;
   a successful `CI` push run for that commit; no release for the tag yet; a version strictly
   above every other `bdock/vX.Y.Z` tag and release (numeric comparison, so 0.10.0 > 0.9.0); `make check`;
   `go mod verify`.
   The CI guard retries its `head_sha` query up to five times and then filters the successful push runs by commit client side, because the Actions API has answered that query alternately with two results and none.
2. **build** (linux/amd64, linux/arm64, darwin/arm64): `scripts/release.sh` twice per
   platform with an empty second build cache; the trees must be byte-identical and name the
   tagged revision with `source_modified=false`.
3. **smoke**: each archive is extracted and run on a native runner (`ubuntu-24.04`,
   `ubuntu-24.04-arm`, `macos-15`): `bdock --version`, `--help`, `tools --help`,
   `completion bash`. No network, Docker or registry is used, so this does not test
   `tools volume` or `tools upgrade`.
4. **publish**: re-checks the owner, refuses an existing release (drafts included) and a moved tag,
   builds `SHA256SUMS`, attests build provenance for the three archives and `SHA256SUMS`
   (`actions/attest-build-provenance`; only this job holds `id-token: write` and
   `attestations: write`), and then runs `gh release create --verify-tag` with the archives and
   `SHA256SUMS`. A dry run never reaches this job, so it neither attests nor publishes.

Each archive holds `bdock`, `LICENSE` and `PROVENANCE` (source revision, version, toolchain,
flags, platform). Nothing is signed with a key or notarized. The archives and `SHA256SUMS` carry a
GitHub build-provenance attestation; verify a downloaded file with
`gh attestation verify <archive> -R wstein/bundledock`. Attestation applies to every
release created by a tag run of this workflow; the first release, `bdock/v0.1.0`, is tagged after
this change.

## Before the first tag

- [ ] The workflow PR is reviewed and merged by you; `main` carries `release.yml`.
- [ ] The untracked local TODO list is reviewed: each item resolved or explicitly deferred
      ([release review](development.md#release-review)).
- [ ] The `release` environment needs no setup: you decided not to configure a reviewer rule.
      The workflow names `environment: release`; GitHub creates it unprotected on first use, so
      a tag push by you that passes the checks publishes at once. Optional: set the repository variable
      `BDOCK_RELEASE_DRAFT` to `true` to publish drafts you release by hand.
- [ ] Recommended, a repository setting only you change (the workflow and agents never do):
      Settings, Rules, Rulesets, New ruleset, "New tag ruleset". Name it e.g. `bdock release tags`,
      Enforcement status `Active`, target `Include by pattern` with `bdock/v*`, enable the rules
      "Restrict deletions" and "Block force pushes" (this blocks moving or deleting a tag), and
      leave "Restrict creations" off so you can still push new tags (or restrict creations to
      yourself in the bypass list). Create the ruleset before the first tag; a tag cannot be
      protected retroactively against an earlier deletion. The version-order check in `verify`
      does not replace it: it stops a lower version, the ruleset stops tag rewriting. In particular,
      if a tag was deleted and no release exists for it, pushing the same version again still
      passes the check (the listing shows the pushed tag itself, nothing higher); only the ruleset
      guards against re-creating a tag.
- [ ] Rehearse once: Actions, `release`, "Run workflow" on `main` (version `0.0.0-dryrun`).
      Dry-run artifacts live one day and, in a public repository, any signed-in GitHub user can
      download them; they are unsigned rehearsal builds, not releases.

## Release

1. Choose the version: the next `X.Y.Z`; the first is `0.1.0`.
2. Pick a `main` commit whose `CI` push run succeeded (a superseded run is cancelled; re-run it if needed).
3. `git tag -a bdock/v0.1.0 -m "bdock 0.1.0" <commit> && git push origin bdock/v0.1.0`.
4. Watch the `release` run. On success the release has three archives and `SHA256SUMS`.
5. Verify a download: `sha256sum --check --ignore-missing SHA256SUMS` (macOS:
   `shasum -a 256 -c` on the matching line), extract, `./bdock --version`.

## When something fails

A transient failure (runner outage, CI not yet green at tag time) before `publish` created
anything can be fixed by re-running the same workflow run as the owner; the tag is untouched.
If `publish` failed after creating a release with missing assets, delete that release by hand
and re-run, or release the next patch version. Otherwise do not delete and re-create the tag. A failed `verify` or `build` publishes nothing: fix the
cause on `main` and release the next patch version. If `publish` created a wrong release,
delete the release by hand and use the next patch version; the tag stays. If a reviewer rule is ever added to the `release` environment and a run waits for approval
past the one-day artifact retention, `publish` fails at the artifact download; re-run the whole workflow for the same
tag (accepted only while no release exists) or, if in doubt, use the next patch version.
Never delete and re-create a tag. The module path `bundledock` is not fetchable through the Go
proxy, so no module cache can pin a bad tag, but release assets and users' copied checksums
can.

A tag whose version is not above every existing `bdock/v` tag or release fails `verify` with
"not above every existing bdock/v tag": release the next higher version.

Notes: `BDOCK_RELEASE_DRAFT` must be empty, `true` or `false` (anything else, such as `True`,
fails publish before a release is created). A manual dry run started on a tag ref shares that
tag's concurrency group; start dry runs from `main`.

## Maintaining the workflow

`internal/releaseworkflow` pins the workflow with rules and mutants. Bumping an action pin,
renaming a step or reformatting a gate line fails those tests on purpose; update the test
constants in the same commit and have the change reviewed. `go test ./...` runs them; they
build no distribution artifact.
