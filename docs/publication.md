---
title: Publication to Quay
description: Configure the destination and understand the native smoke and registry publication gates.
weight: 40
---

# Publish the toolbox to Quay

[Back to BundleDock](../README.md)

[The publication workflow](../.github/workflows/bundledock-toolbox.yml) supports
manual publication and a human approval request after a merged toolbox upgrade.
For manual publication, use Actions → **Publish toolbox** → **Run workflow**,
select default branch `main`, and enter the explicitly approved full
40-character commit in `approved_sha`. That input must equal the run's
`github.sha`. Other branches, bots and bot reruns are rejected before compilation.
Each build job rechecks this approval. Manual dispatch keeps its existing fresh
upstream resolution and does not require the environment described below.

After **CI** succeeds for a push to current `main`, read-only preflight accepts
only a merged `github-actions[bot]` toolbox updater PR with an
`automation/toolbox-upgrade-*` branch (including the previous
`automation/tool-upgrade-*` prefix). Both the PR and actual merged commit must
change only the toolbox lock and its optional generated `docs/tools.md`
inventory, with different lock bytes from the merge's first parent. Failed CI,
unmerged PRs, unrelated changes, missing provenance and stale main revisions
are rejected. The request displays the exact SHA and links the merged PR.

Before this path can run, an administrator must configure the existing GitHub
**toolbox-publication** environment with exactly one required reviewer, the
user **wstein**, **Prevent self-review** disabled, administrator bypass disabled,
and exactly one custom deployment branch policy: branch **main**. Allowing
self-review lets the maintainer approve an update they merged; approval is still
an explicit human action. No workflow creates or configures this environment.
Missing protection, additional reviewers or teams, unavailable API access, or
broader branch policies stop the request before any artifact compilation.
Environment support and read-only API visibility depend on the repository's
GitHub plan and permissions; a denied query fails closed.

In Actions, inspect **Approve toolbox <full SHA>**, follow its merged PR link,
and explicitly approve the environment deployment to authorize the toolbox
build and publication for that revision. The approval job rechecks provenance,
current main, and environment protection. Every later compiler job rechecks
them, and promotion checks again immediately before registry writes. If main
moves while waiting or building, the old source cannot be promoted. The approved
path copies the committed lock bytes without upstream discovery; all native
jobs and promotion consume that one hashed artifact. A matching published tool
baseline skips native builds and publication.

These helper checks are provenance checks, not authorization on their own.
The immutable reviewed workflow's dependency on GitHub's protected environment
supplies approval; setting `PUBLICATION_SHA` or invoking a helper does not.
Source fixtures verify rejection behavior and job dependency conditions. Live
approval, native builds and publication remain unverified until a separately
authorized configured run succeeds. Other bundle publishers remain manual-only.

A manual dispatch expresses approval for both native image builds/smokes and
publication, including the fresh upstream resolution within the reviewed tool
ranges. It is not approval to activate a consumer image. Automation must never
invoke manual dispatch, including with a human-owned token. GitHub event context
cannot distinguish a person's UI dispatch from automation using that person's
credentials. Restrict workflow dispatch credentials and repository writers
separately. The daily lock-only PR workflow stops at source review; the separate
protected-environment approval authorizes later toolbox native builds.

Publication runs share one concurrency group; an active run is allowed to finish.
Before promotion, a read-only GitHub API check rejects a run superseded by a newer
eligible run, including one whose native build failed. A newer run with successful
resolution and skipped native jobs is a proven no-op and does not supersede an
older changed publication. Newer automatic CI handoffs supersede only after
successful resolution reaches native work: queued, rejected or unapproved
requests supply no build authority. Current-main revalidation separately prevents
older automatic sources from promoting after main moves. Unknown manual-run
eligibility and unavailable API queries fail closed, with bounded queries for
the exact run attempt. This prevents an old queued run or rerun
from rolling stable back. Obtain explicit user approval before a fresh manual run or rerun. When rerunning,
choose **Re-run all jobs** so the resolver creates current-attempt artifacts;
that approval applies to the same original run SHA.

Set up the destination before the first publication:

1. Sign in to [Quay.io](https://quay.io/) and choose the user or organization
   namespace that will own the image. Create a repository named
   `toolbox` in that namespace. Choose public visibility for anonymous
   pulls or private visibility when consumers will authenticate; this choice is
   independent of the GitHub source repository's visibility.
2. Create a dedicated robot account in the namespace's **Robot Accounts** tab.
   In the target repository's permissions, grant that robot **Write** access to
   `toolbox` only. It does not need namespace administration or access
   to unrelated repositories. Its username has the form `namespace+robotname`.
3. In [BundleDock's Actions settings](https://github.com/wstein/bundledock/settings/secrets/actions),
   add the repository variables and secret below. Enter the robot token directly
   as the secret value; keep it out of Git, logs, issues, and command arguments.


| Setting | Value |
| --- | --- |
| Repository variable `REPOSITORY_URL` | `quay.io/bundledock/` (namespace prefix) |
| Repository variable `REPOSITORY_USERNAME` | `bundledock+github-actions` (Quay robot username) |
| Secret `REPOSITORY_TOKEN` | Robot token with write access to `quay.io/bundledock/toolbox` |

The workflow accepts the namespace prefix with or without one trailing slash and
appends `toolbox`, so `quay.io/bundledock` and `quay.io/bundledock/` both publish
to `quay.io/bundledock/toolbox`. Do not include an HTTPS scheme, image name, tag,
or digest in `REPOSITORY_URL`. Other namespaces use the same prefix format and
a robot username from that namespace.

After setting these values, start **Publish toolbox** manually from
`main` with the approved full SHA and watch the resolver, both native jobs, and promotion in
[Actions](https://github.com/wstein/bundledock/actions/workflows/bundledock-toolbox.yml).
A successful run publishes a dated run-attempt tag, `stable` and `latest`; inspect its
summary and artifacts for the immutable digest before choosing a deployment.
Configuring these values does not authorize or schedule a build or publication.
An approved manual dispatch always builds and publishes after validating the
baseline, even when tool assets are unchanged, so builder or base-image changes
can be released deliberately. No registry publication has been verified until a
configured run succeeds.
For a private Quay repository, consumers need their own read credentials.

The Quay namespace is deployment configuration, with no default in the workflow.
These settings replace `QUAY_IMAGE`, `QUAY_USERNAME`, and `QUAY_PASSWORD`; the
workflow does not read the old names. Keep an existing `REPOSITORY_TOKEN` secret
in place when it already holds the robot token; no token needs to be copied into
source or inspected.
Use a dedicated [Quay robot account](https://docs.quay.io/glossary/robot-accounts.html).
The workflow uses read-only contents permission and read-only Actions permission
in resolution and promotion. It never runs for pull requests. The separate
baseline comparison step uses robot credentials only against `quay.io` to request
a repository-scoped pull-only bearer token and read manifests/configs, including
for private repositories. It never stores those credentials or logs in to Docker.
Service-issued signed blob/archive downloads receive no registry or GitHub
authorization header, and authenticated HTTP requests do not follow redirects.
Docker login and all registry writes occur only after each native smoke passes.
Check [GitHub runner availability and plan limits](https://docs.github.com/en/actions/how-tos/write-workflows/choose-where-workflows-run/choose-the-runner-for-a-job)
for `ubuntu-24.04` (AMD64) and `ubuntu-24.04-arm` (ARM64).

For manual dispatch, one resolver runs `bundledock tools upgrade` for all tools;
the merged-upgrade path freezes the committed lock. Both upload the exact
lock bytes and shared source commit, UTC build time, lock SHA256, run tag and full
GitHub run ID (`run_id`).
The resolver uses official stable prebuilt releases; it does not rebuild tools
from source or patch embedded dependencies. A successful publication does not
establish that upstream binaries are free of known vulnerabilities. Review the
[remaining embedded dependency findings](development.md#embedded-dependency-findings)
and wait for patched stable releases to remediate them.
It never commits or pushes the resulting lock. Both native jobs consume that
artifact, build `bundles/toolbox/Dockerfile` once, explicitly pull the consumer baseline and
run `bundles/toolbox/smoke.sh` against the resulting local image ID. OCI labels include the
image title, source, revision, build time, run tag, lock hash, tool versions and
`io.bundledock.github.run-id`. Artifact checks still bind to the full
`GITHUB_RUN_ID`, run number, attempt and source commit. The newest-run gate uses
the full run ID to query GitHub; shortening the tag does not change that check.

The adapter retains the published-tool baseline comparison and fails closed if
the existing `stable` provenance cannot be validated. Approved manual runs
publish even when tool pins are unchanged. Merged toolbox updates compare against the published baseline and skip unchanged
tool assets after approval. Historical scheduled no-op logic does not enable a
schedule or authorize a build.
New images carry `io.bundledock.tools.platforms-sha256`, a canonical digest of
those pins, alongside the full-lock provenance hash. Both platform config labels
must agree, and downloaded immutable manifests/configs must match their digests.

For older images without that semantic label, the adapter identifies their
successful GitHub run and exact attempt from provenance, downloads its
`resolved-tools-<attempt>` artifact and verifies the lock bytes against the
image's full-lock SHA256 before comparing tool pins. The legacy tag format is
supported even when the image predates the separate run-ID label. That migration
depends on the old artifact remaining available; after a new image is published,
the durable OCI label removes that retention dependency. Missing `stable`,
expired/deleted legacy artifacts, mismatched provenance, authentication failures
and network errors stop publication, including manual dispatch. They never imply
an empty baseline or silently bootstrap a first publication. A new destination
requires a separately reviewed initial publication before this workflow can
validate subsequent approved publications.

A native job pushes its exact tested image to `staging-<run-tag>-<architecture>`.
If either architecture fails, final promotion is skipped. Once both pass, the
adapter verifies each remote registry manifest references its smoked image config,
creates the two-platform manifest and verifies its platform digests before updating
`stable` and `latest` to that verified digest. An untagged pull such as
`docker pull quay.io/bundledock/toolbox` resolves to `:latest`. Explicit `:stable`
and digest references remain available. Alias updates are sequential, so a later
failure can leave an earlier tag updated. Success is reported only after both
aliases have been verified against the version digest. The UTC version tag is
`YYYY.MM.DD-b<github.run_number>.<github.run_attempt>`, for example
`2026.10.10-b12.4`. The run number distinguishes normal runs and the attempt
counter distinguishes reruns. Missing, malformed or nonpositive counters are
rejected before resolving tools.

Workflow recreation can restart its run counter. Before each staging push and
again before creating the final version index, an authenticated Quay manifest
check requires HTTP 404 for the version tag. An existing tag, authentication
failure or network error aborts that operation without updating the version or
either mutable alias; start a fresh workflow run to obtain another tag. Registry write operations
remain gated on native smoke passing. This is a collision check,
not an atomic reservation: registry-enforced immutability requires separate Quay
configuration, and unrelated registry writers must be controlled separately.
The lock hash is provenance, not a build identifier: base layers or Debian
packages can change for the same lock.

Download `resolved-tools-<attempt>`, `published-amd64-<attempt>`,
`published-arm64-<attempt>` and `publication-<attempt>` artifacts from the run (retained for 90 days). The run summary and `publication.json`
report the remote multi-platform registry digest, which differs from a local Docker
image ID. Use the reported `quay.io/<namespace>/toolbox@sha256:...` reference
for explicit deployment or rollback. Publication does not change the devcontainer
pin, initialize host volumes, or activate the new image.

Quay publication requires the first successful configured publication run.
See [development and verification](development.md#verification-evidence) for
local checks and the scope of historical native CI results.
