---
title: Publication to Quay
description: Configure the destination and understand the native smoke and registry publication gates.
weight: 40
---

# Publish the toolbox to Quay

[Back to BundleDock](../README.md)

**Publish bundles** (`publish-bundles.yml`) is the publication entrypoint for
all nine bundles. Successful merged-main CI selects one eligible updater;
manual runs select one bundle and the explicitly approved full source SHA.
Bundle-specific native smoke checks, artifacts and source tag namespaces remain.
**Propose upgrades** (`propose-upgrades.yml`) shares the source-only updater,
retains the daily 06:50 UTC schedule, and offers manual bundle selection with
dry runs enabled by default. Each bundle resolves independently, then one PR
per run carries every bundle that changed in a single commit on
`automation/upgrades-<run>-<attempt>`; the title lists the bundles and the body
has one table per bundle. If a bundle's resolution fails, the others are still
proposed and the failure is named in the PR body and run summary. A run that
changes one bundle keeps that bundle's own `automation/<bundle>-upgrade-...`
topic. The owner merge of either form authorizes publication as described
below: for a combined PR the planner derives the bundles from the locks it
changed, accepts only those bundles' own locks and inventories, and runs one
publication leg per bundle with the same per-bundle checks.

[Nixbox](nixbox.md) is published only by the owner's manual dispatch of
`publish-bundles.yml`: selecting `nixbox` there is the approval, and the
publication is the same shared workflow as for every other bundle. No updater
pull request, schedule or CI completion publishes it. `propose-upgrades.yml`
offers it as a nixpkgs bump plus lock refresh that opens its own
`automation/nixbox-upgrade-...` pull request, which the publication planner does
not recognize; its manual build and smoke is `nixbox-build.yml`. Its tags are
`<date>-b<N>` like the toolbox.

The [Codex](codex.md), [Antigravity](agy.md) and [Claude Code](claude.md)
guides record their verified initial publications and native AMD64/ARM64 smoke
coverage. The current source toolbox excludes all three agents; the
[published toolbox](toolbox.md#published-bundle) still contains them until a
separately approved reduced-toolbox publication. Each receipt applies only to
its recorded source revision and image digest. An updater cannot bootstrap an
absent baseline; first publication of a new bundle requires explicitly approved
manual dispatch for the exact source SHA and build/publication operation, plus
verified repository visibility and robot access.

For a later automatic upgrade, review the lock and generated inventory in the
`github-actions[bot]` updater PR, run the required source checks, and merge it as
**wstein** using the repository's protected human merge path. The actual owner
merge authorizes publication of that exact frozen lock. No submitted APPROVED
review is required, and no controller merges source PRs.

Keep source/test fixes in separate PRs. An updater may contain its original
commit followed by merges that refresh it from `main` (at most 16 commits total).
The verifier checks the complete PR commit list, first-parent continuity, exact
head identity, and that every merged parent is an ancestor of the final merged
main parent. Additional ordinary commits, foreign ancestry, incomplete history,
or any final changes outside the bundle's lock and inventory reject publication.
Rebasing to a single updater commit remains supported. Rerun source checks after
refreshing the branch and before the human merge.

The verifier also compares the bundle lock content of the merged commit with that
of its single parent (not the PR base). It rejects a changed `toolchains` or
`extra_sources` section, an added or removed package or platform, and for every
`source-build` tool anything beyond a rise of the source `version`, `url` and
checksums (new version within the `packages.txt` constraint, URL derived from the
version, both architectures in agreement). An updater PR therefore cannot add a
`source-build` tool or change its recipe or toolchain; those changes are owner-authored
PRs. See the generic tool builds design, "Publish-gate content check".

**Approval of a source build.** The owner's merge of the updater or source PR is the
approval; the lock carries no separate approval value (a lock with an `approvals` key is
rejected as an unknown key). The owner's merge of the PR that added the tool, or that
changed its recipe, compiler stage or the builder, is its approval, and a source-build
tool already in the lock on `main` is built and published by the next merged toolbox
updater. `ci.yml` runs only `make check`; the real compile (`--network=none`) and the
native smoke run after the merge, in `bundle-publish.yml` (and in the manually
dispatched `toolbox-build.yml`), before the staging push and before the promotion.
Nothing is promoted when they fail.

An already merged ineligible updater cannot be repaired retroactively; use a
separately approved exact-SHA manual publication of current `main`. Rerunning an
old publication does not bypass current-main checks. `main` may advance during a
long native build (for example another bundle's updater merge) while the exact
approved source stays authorized, provided that source remains an ancestor of
`main` and the bundle's own lock is byte-identical there. A diverged or rewritten
`main`, or a changed own lock, stops publication and requires this manual path.

1. Successful **CI** on the exact merged `main` revision starts **Publish bundles**.
   Its read-only planner identifies the associated updater using the full PR API
   response; the associated-PR list is discovery evidence, not merge authority.
2. The publisher verifies `merged_by.login` is `wstein` with human account type,
   the same-repository bot author and updater branch, and changes limited to that
   bundle's own lock and generated inventory. Renamed/deleted files, missing
   provenance and ambiguous API results stop publication.
3. The merged tree must equal the updater head tree, the lock must change from
   its parent, and the successful required GitHub Actions `check` must cover the
   exact merged main SHA. Current-main identity is rechecked before artifact
   compilers, native jobs, tagging and registry promotion.
4. Both native smoke checks and existing baseline, artifact and registry gates
   must pass. The automatic path copies the committed lock without resolving
   newer upstream releases. First publication retains the separate manual gate.

The verifier pins REST API version `2022-11-28` because GitHub's
[`2026-03-10` breaking changes](https://docs.github.com/en/rest/about-the-rest-api/breaking-changes#version-2026-03-10)
remove `merge_commit_sha` from full and associated PR responses. Controlled
read-only requests for PR #66 and its merged source reproduced the omission
under `2026-03-10` and recovered the exact SHA under `2022-11-28`; retries of a
removed field cannot restore that contract. This explains the reproduced source
of missing proof, without establishing the cause of every historical failure.

A non-ancestor main or source inconsistency stops this sequence. When a trusted
owner-merged bot PR is closed but its merge SHA is missing, a matching closed
association permits at most two further reads of that same full PR, two seconds
apart. The association may name the exact source or lack its merge SHA; it never
supplies merge authority. Known wrong SHAs reject immediately. Stable PR, head,
repository, owner and author identities remain required, and recovery must
establish the exact full PR merge SHA and every normal provenance gate. The
latest CI attempt is checked after delayed reads. Exhausted missing metadata is
pending and cannot authorize builds. Diagnostics record only safe attempt,
reason and expected/observed identity values. The existing
`toolbox-publication` environment is unused and unchanged. Ordinary source and
documentation PRs are ineligible.

For manual publication, select **Run workflow** on default branch `main` and
enter the explicitly approved full 40-character SHA in `approved_sha`. It must
equal the immutable run SHA. Every build job retains the human actor and SHA
gate. Manual dispatch uses the committed frozen lock without fresh upstream
resolution, and authorizes both native builds and publication even when pins
are unchanged. Automation must not impersonate a
human to dispatch this path. Consumer image activation needs separate approval.

Automatic publication compares the frozen lock with the existing published
baseline and skips native work when assets already match. Runs share a
per-bundle concurrency group. Newer automatic requests supersede a publisher
only after successful resolution reaches native work; queued or unapproved
requests do not. Current-main revalidation separately rejects stale sources.
Manual runs and unavailable API responses retain fail-closed newest-run checks.
Reruns must revalidate the same owner merge and current source; use **Re-run all
jobs** to produce artifacts for the current attempt.

The shared helper validates provenance; variables such as `PUBLICATION_SHA`
are not authorization evidence. Summaries distinguish ineligible source,
blocked owner merge, pending or rejected provenance, a promotion gate that
rejected the approved source after a successful build (`source-rejected`, with the
reason and the manual-dispatch recovery), unchanged published assets,
and published output. A successful no-op workflow does not mean publication.
Tests cover exact source/head/attempt/caller and check application identity,
the REST transport contract, full PR versus list-summary evidence, bounded
missing-metadata recovery and pending exhaustion, frozen
lock bytes and merged/updater tree equality. This source change performs no
workflow run, image build, publication or consumer activation; the automatic
sequence needs its own approved live evidence.

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

After setting these values, start **Publish bundles** manually with `bundle=toolbox` from
`main` with the approved full SHA and watch the resolver, both native jobs, and promotion in
[Actions](https://github.com/wstein/bundledock/actions/workflows/publish-bundles.yml).
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
Preflight, resolution, native builds and promotion retain read-only contents
permission. Only the separate post-promotion tag job requests contents write
permission, with read-only Actions and pull-request access for provenance.
Thin caller jobs allow that reusable-workflow permission ceiling; each actual
non-tag job explicitly retains read-only contents permission. It never runs for pull requests. The separate
baseline comparison step uses robot credentials only against `quay.io` to request
a repository-scoped pull-only bearer token and read manifests/configs, including
for private repositories. It never stores those credentials or logs in to Docker.
Service-issued signed blob/archive downloads receive no registry or GitHub
authorization header, and authenticated HTTP requests do not follow redirects.
Docker login and all registry writes occur only after each native smoke passes.
Check [GitHub runner availability and plan limits](https://docs.github.com/en/actions/how-tos/write-workflows/choose-where-workflows-run/choose-the-runner-for-a-job)
for `ubuntu-24.04` (AMD64) and `ubuntu-24.04-arm` (ARM64).

Manual dispatch and merged-upgrade publication both freeze the committed lock
without upstream discovery. Manual resolution rechecks the existing human
full-SHA approval gate. To change tool versions, review and commit an upgraded
lock before approving publication of that exact source revision. Both paths
upload the exact lock bytes and shared source commit, UTC build time, lock
SHA256, run tag and full GitHub run ID (`run_id`).
The lock pins official stable prebuilt releases, or a reviewed static source
build that meets the [repository policy](../AGENTS.md); publication does not
otherwise rebuild tools from source or patch embedded dependencies. A successful publication does not
establish that upstream binaries are free of known vulnerabilities. Review the
[remaining embedded dependency findings](development.md#embedded-dependency-findings)
and wait for patched stable releases to remediate them.
Publication never commits or pushes the lock. Both native jobs consume that
artifact, build `bundles/toolbox/Dockerfile` once, explicitly pull the consumer baseline and
run `bundles/toolbox/scripts/smoke.sh` against the resulting local image ID. OCI labels include the
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
those pins, alongside the full-lock provenance hash. For a lock with source builds
the digest also covers the pinned `toolchains` and `extra_sources`, so a changed
compiler or extra-source hash rebuilds; a lock without them keeps its earlier digest. Both platform config labels
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
aliases have been verified against the version digest.

Toolbox uses `YYYY.MM.DD-b<packaging-revision>`, such as `2026.10.15-b1`.
The date is UTC, frozen when resolution starts. The first packaging revision
for each date is `b1`; further publications on that date advance to `b2`, `b3`,
and so on. The workflow run number does not supply the revision, and the attempt
remains a separate receipt field and OCI label.

Allocation requires a complete authenticated registry tag listing and the source
Git tags. Existing new-format version tags, source tags and architecture staging
tags reserve revisions, including partially uploaded failed runs. Gaps are allowed;
a rerun allocates the next unreserved revision rather than overwriting one.
Historical `YYYY.MM.DD-b<run-number>.<attempt>` and date/run-ID tags remain
unchanged and readable, and do not contribute to the new daily counter. Thus the
first new-format tag can be `2026.10.15-b1` even if `2026.10.15-b98.1` exists.
Missing, malformed or nonpositive workflow counters still fail provenance
validation before resolving tools.

The eight single-package bundles use `<upstream-version>-b<packaging-revision>`.
The version comes from the resolved official lock and must match on AMD64 and
ARM64. Official tag prefixes are removed: Codex `rust-v0.162.1` becomes
`0.162.1-b1`, Bun `bun-v1.4.3` becomes `1.4.3-b1`, and Node `v24.21.0`
becomes `24.21.0-b1`. Antigravity, Claude and code-server likewise start at
`1.3.3-b1`, `2.1.296-b1` and `4.141.0-b1` for those upstream releases.
These examples describe the naming policy and do not claim publication.

A complete authenticated registry tag listing and existing source Git tags
reserve packaging revisions. Repackaging the same upstream release uses the
next revision (`-b2`, `-b3`, and so on); a new upstream version starts at `-b1`.
The workflow run number never supplies this revision. The resolver freezes one
version and lock for both native jobs. Run ID and attempt remain independent
receipt fields and OCI labels, and source tagging verifies the exact workflow
attempt. Existing date tags, source tags and digest references stay available;
reading historical provenance does not rename or overwrite them.

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

Download `resolved-tools-<attempt>` for toolbox or `resolved-<bundle>-<attempt>`
for the other bundles, plus `<bundle>-published-amd64-<attempt>`,
`<bundle>-published-arm64-<attempt>` and `<bundle>-publication-<attempt>`
artifacts from the run (retained for 90 days). The run summary and `publication.json`
report the remote multi-platform registry digest, which differs from a local Docker
image ID. Use the reported `quay.io/<namespace>/toolbox@sha256:...` reference
for explicit deployment or rollback. Publication does not change the devcontainer
pin, initialize host volumes, or activate the new image.

Quay publication requires the first successful configured publication run.
See [development and verification](development.md#verification-evidence) for
local checks and the scope of historical native CI results.

## Source tags for published bundles

After successful image promotion, the shared workflow for toolbox, code-server,
node, node-current, bun, codex, agy, claude and opentofu creates an annotated Git tag `<bundle>/<immutable-version>` at the
image's exact source commit, for example `node/24.21.0-b1`. Approval of that
publication includes this source tag. All nine bundles support the owner-merged updater
path described above; the shared read-only source provenance gate is revalidated
before automatic tagging. The workflow SHA remains distinct from the authorized
image source SHA, and both must
match their own independently verified provenance. The step follows verification of
the final two-platform index and both `stable` and `latest` image aliases. It
never creates moving `stable` or `latest` Git tags.

The annotation records the fully qualified version image, immutable index digest,
source commit and exact workflow run attempt. The helper independently hashes
the index, both architecture manifests and their configs, checks matching OCI
source/revision/version/run labels, and verifies that the source commit is
reachable from official BundleDock `main`. Existing remote tags must be annotated
and match both source and the complete annotation. A conflict stops the run;
tags are never overwritten or force-pushed. Concurrent matching creation is
verified by reading the remote tag back. A tagging failure leaves the promoted
image in place and fails the tag job and workflow; inspect the existing image and
tag before choosing a fresh approved operation.

For separately authorized backfills, use the same helper with `--backfill` and
explicit bundle, version, digest, source SHA and run ID. It requires a completed,
successful publication run matching that provenance. The default is read-only;
`--write` is required to create the remote Git tag. Historical toolbox versions
such as `2026.10.10-38072919085-4` additionally require `--legacy-version` and
retain their exact original version suffix. Inventory all registry tag pages,
ignore staging and mutable aliases, deduplicate immutable versions, then review
the complete proposed tag/image digest/source SHA/run table before writing.
Uncertain provenance or an incomplete inventory blocks the affected backfill.
No image pull, extraction, build, registry write or consumer activation is needed.

```sh
python3 -B scripts/bundle-git-tags.py --backfill --bundle node \
  --tag 2026.10.12-b1.1 --digest sha256:<verified-index-digest> \
  --commit <verified-full-source-sha> --run-id <verified-run-id>
```

For a singleton upstream version, also supply `--run-attempt` with the
independently verified workflow attempt; it cannot be inferred from `-bN`.

Git tag writes are atomic individually. If a later backfill conflicts, previously
verified tags remain; resolve the conflict before continuing. Registry writes
and Git tag writes are separate systems and cannot be one atomic transaction.

## Published guide updates

After successful verified promotion and source tagging, the shared publisher
adds the bundle to one rolling `publication-docs/pending` PR (runs queue under
the `bundledock-publication-docs` concurrency group). Each run rebuilds that
topic as a single commit on current `main` from the pending entries plus its
own, updates the PR title and table, and dispatches source CI for the new head.
It refuses a topic that has a merge commit or touches anything but publication
Markdown and metadata. It re-reads the topic head just before the forced
update, but that check is not atomic; the concurrency group is what keeps runs
from overlapping. If the topic exists without an open PR, delete it and rerun.
The bundle guide's managed publication section records the immutable version,
index digest, exact source SHA, workflow run and digest-pinned pull example.
The generated tool table remains the current source inventory, separate from
the published record. Most version badges read Quay directly and need no README bot commit. Node
badges read the managed `docs/node-published.json` and
`docs/node-current-published.json` metadata, updated in the same publication PR
as the corresponding guide. They refresh after that PR is merged and the badge
cache expires; they do not display the current source lock.

For every bundle, the same PR also writes `docs/<bundle>-published.json`.
Its existing display fields remain compatible with the Node badges. The
`publication_evidence` object retains the complete canonical `publication.json`
receipt and the exact workflow attempt identity: source/controller binding,
event, workflow path, run ID/attempt/number, and both native smoke and promotion
job IDs, conclusions and executed steps. Capture runs while the documentation
job and parent run are still in progress; it requires the preceding native,
promotion and tag jobs to have succeeded, source ancestry on official `main`,
and the matching annotated source tag. The tag job already verified registry
publication; capture adds no registry credentials to the documentation job.
Git retains this source evidence after the 90-day CI artifact expires, once
the publication PR has been merged. Existing historical metadata has no such
evidence until a separately verified source update supplies it.

This job edits Markdown and publication metadata source only. It creates a topic from fresh `main` using
GitHub's create-only reference API, never advances an existing topic, and reuses
an existing PR only when its tree, parent, head and target match. Conflicting
records, newer published versions, moved main or topic refs and malformed
receipts fail closed. Source CI is explicitly dispatched for the exact docs
head because `GITHUB_TOKEN` PR creation does not trigger ordinary PR workflows.
The docs job has contents, pull-request and Actions write permission; earlier
compiler and native jobs retain read permissions. No documentation build,
automatic docs merge or consumer activation is performed by this job.

The [Node Current carrier](node-current.md#published-bundle) has a verified
publication recorded in its guide. First publication requires separate
exact-revision build/publication approval; an updater approval cannot bootstrap
an absent baseline. Consumer activation remains a separate operation.

### Shared Node repository

`node` and `node-current` are canonical source identities sharing the output
repository `quay.io/bundledock/node`. Their publication workflows serialize on
that repository without cancellation. Their newest-run guards retain separate
source workflow identities. Both source tag namespaces reserve packaging
revisions alongside the complete authenticated OCI tag listing; existing
immutable tags are never replaced.

LTS updates only `stable`, `lts`, and `latest`; Current updates only `current`.
Promotion and subsequent source tagging verify that channel's aliases. New
receipts and OCI configs bind the canonical source bundle explicitly; historical
LTS date-tag receipts remain readable. A Current receipt without source identity
is rejected. Alias updates are sequential and cannot roll back an earlier alias
if a later write fails; a successful receipt requires every selected alias to
resolve to the tested immutable index.

The README Node badges label LTS and Current and display only the published
upstream version, without the packaging `-bN` suffix. Their managed JSON metadata
retains the immutable tag, digest, source revision and run alongside that value.
Each channel updates only its own guide and metadata in one create-only PR tree;
merging that publication PR makes the new badge value available without editing
the README. Quay exposes alias digests and per-platform labels separately, so a
single dynamic badge request cannot derive an upstream version from an alias.
The badges link to the matching `lts` and `current` tags in the shared Quay
repository.

Semantic publication receipts supply the upstream version. Historical date tags
alone yield `unknown`; they must never be interpreted as Node versions or
replaced with a desired source pin. The seeded LTS value `24.21.0` was verified
from `io.bundledock.tools.versions` on both published platform manifests
([AMD64](https://quay.io/api/v1/repository/bundledock/node/manifest/sha256:4a6ab359131bb115c5c2281c444eee32922cd70fd3b2102ff8631f04ec8233f9/labels),
[ARM64](https://quay.io/api/v1/repository/bundledock/node/manifest/sha256:a99c41a8b8ec7861867c30b81466ba549c13e0e41435eea267c416075456b5cd/labels))
in the recorded LTS index. Reapplying that exact historical publication preserves
its verified value; a different historical identity falls back to `unknown`.
Unpublished channels have no published metadata and an unavailable badge rather
than a version copied from their source inventory.

## Reconcile an existing publication

A green workflow badge alone does not establish publication. An automatic
`workflow_run` completion can succeed while every artifact job is skipped.
Reconcile the exact bundle, full image source SHA, run ID and attempt using the
read-only helper (Python standard library and an already configured `gh`):

```sh
python3 -B scripts/publication-reconcile.py --bundle opentofu \
  --sha 84216f3a45e35ba741cc8245f96803b5b9a2a31f \
  --run-id 38314225387 --attempt 1
```

This existing manual run verifies `opentofu/1.13.1-b1` and index digest
`sha256:67bd27de607af0d950789a3a0308262141860a331bae31b3f2a9d170c0cb3414`.
The existing automatic run `38314751960`, attempt `1`, at controller SHA
`bde9e76f14665716c2d7d387b1cdfd807dd26101` instead reports `not-published`.
These are historical evidence, not authorization for a new operation.

The helper reads the exact workflow attempt and its jobs, checks that both
native build-and-smoke steps actually executed successfully, and checks promotion,
tagging and documentation proposal steps. It downloads only the uniquely named
`<bundle>-publication-<attempt>` artifact from that run and validates its canonical
`publication.json` with the existing receipt validator. If that artifact has
expired, is missing, or its listing/download is unavailable, it can read the
canonical receipt from the exact matching `publication_evidence` in official
`main`'s `docs/<bundle>-published.json`. The stored attempt and native/promotion
job/step snapshot must equal live GitHub evidence. An available contradictory
artifact, malformed proof or unknown job/expiry state never permits fallback.
The receipt must bind
the requested source, run and attempt, both smoked platforms and immutable index.
The helper independently hashes the live version index, architecture manifests
and configs, compares them to the smoked receipt, verifies OCI source/run/lock
labels, checks that source is on official `main`, and reads the annotated source
tag and complete annotation from GitHub. Automatic controller SHA and image
source SHA remain separate; only a verified receipt establishes `source_sha`.

| State | Meaning |
| --- | --- |
| `published` | Image, source tag and documentation PR step verified; this does not claim the PR was merged or the site rendered. |
| `not-published` | Successful automatic run skipped all native/promotion/tag/documentation work and has no canonical receipt for this attempt. |
| `published-image-and-tag` | Image and tag verified, but documentation step did not succeed. |
| `published-image` | Image verified, but source tag job did not succeed; inspect the tag outcome separately. |
| `pending` | Required evidence is missing, expired, unknown, inaccessible or still running. |
| `incomplete` | Completed native or promotion jobs did not all succeed; publication has not been proved. Registry writes may still have occurred. |
| `rejected` | Evidence contradicts the requested publication identity; resolve the inconsistency before making any claim. |

Exit status is `0` for `published` or `not-published`, `2` for pending or incomplete
work (including a verified image with unfinished tag/docs work), and `1` for
contradictory evidence. Artifact expiry alone can be covered by verified tracked
evidence; unavailable live run/jobs, registry or tag evidence still leaves the
result pending, even if a badge is green. No state proves consumer activation. Immutable version evidence
is checked rather than current moving aliases, which may already identify a
later publication. The helper never dispatches workflows, builds artifacts,
writes registry data, creates tags, opens PRs, merges, or changes settings.

By default, tracked metadata describes the latest merged publication. To read an
older receipt retained in Git history, add `--evidence-ref <full-40-character-SHA>`
with a commit containing that publication's metadata. The helper requires that
commit to be an ancestor of official `main`; arbitrary branches, tags and local
files are not evidence sources. The selected record must still match the exact
requested source/run/attempt and all live checks. A newer or unrelated record
leaves the older attempt pending. This covers CI artifact retention, not permanent
archival of live GitHub job history or registry content; deleted/inaccessible live
proof remains pending.

### Recover a failed or skipped documentation job

If the image was promoted and source-tagged but the `documentation` job failed or
was skipped, the guide and `docs/<bundle>-published.json` are missing from
`main`. The owner repairs this by re-proposing the documentation PR from the
run's receipt; no rebuild or re-promotion is involved.

1. Confirm the state is `published-image-and-tag` with the read-only helper
   described above (`--bundle`, `--sha`, `--run-id`, `--attempt`).
2. Download the run's receipt artifact, named `<bundle>-publication-<attempt>`
   (for example `toolbox-publication-2` or `agy-publication-1`), into an empty
   directory:

   ```sh
   gh run download <run-id> -R wstein/bundledock \
     -n <bundle>-publication-<attempt> -D <empty-dir>
   ```

3. From a clean checkout of `main` (clean working tree, `origin` pointing at
   `wstein/bundledock`), with a `gh` token that can write contents, pull
   requests and Actions, run:

   ```sh
   python3 -B scripts/publication-docs.py \
     --receipt <empty-dir>/publication.json --bundle <bundle> \
     --image quay.io/bundledock/<bundle> --sha <source sha> \
     --run-id <run-id> --attempt <attempt> --propose
   ```

`<source sha>` is the full image source SHA the run was approved for: the
helper's `requested_source_sha` (and `source_sha`), not the automatic
controller SHA, which can differ. For example, toolbox run `40008991372`,
attempt `2`, uses `1e08d92cffad915da515b88bc8d1992eac3ded02`.

`--propose` first re-verifies the evidence as the helper does. It then creates or
rebuilds the rolling `publication-docs/pending` branch and PR from fresh `main`
and dispatches source CI for the new head. This writes a branch and a PR and
starts a workflow, so it is an owner operation that needs explicit approval for
each use. An artifact that expired fails closed; the tracked `publication_evidence`
fallback only applies to the read-only helper.

When several bundles of one combined run need recovery, repeat the download and
command for each bundle with its own receipt, run and attempt. All of them
accumulate in the same single pending PR.

**Re-run failed jobs** does not repair the job after the workflow or this script
changed: GitHub re-uses the original commit, so the old code runs again. Use the
steps above from current `main` instead. A re-run of only the `documentation` job
works when the workflow already contains the receipt selection: it takes the
attempt from the run's single `<bundle>-publication-<attempt>` receipt instead
of the new attempt number. The `tag` and `cleanup` jobs stay bound to the
current attempt; use **Re-run all jobs** for those.

## List unpublished updaters

`reconcile` verifies one known attempt. To find which bundles still need one, run
the read-only survey (Python standard library and an already configured `gh`):

```sh
python3 -B scripts/publication-pending.py [--bundle claude ...]
```

For each bundle it compares the lock on `main` with the lock at the source commit
of the recorded publication (`docs/<bundle>-published.json`) or of its newest
annotated source tags; a tag proves publication before the documentation PR is
merged. It prints one JSON line per bundle: `published`, `unpublished` (with the
latest lock commit and its pull requests), `no-baseline` (never published; first
publication needs the manual gate) or `pending` (evidence unavailable). The exit
status is `0` only when every selected bundle is `published`, otherwise `2`.
A matching lock is evidence of the lock, not of a verified registry image: use
`publication-reconcile.py` for exact attempts. The survey never dispatches,
builds, tags, writes or merges.

## Staging tag cleanup

Successful publication removes only its two `staging-<version>-<architecture>`
tags, after promotion, annotated source tagging and the documentation proposal
succeed. Cleanup independently verifies the canonical receipt, both smoked
platform digests in the published index, current aliases, source ancestry and
annotated tag. It saves a JSON dry-run plan as a 90-day Actions artifact before
attempting deletion, then rechecks each tag's digest, timestamp and run identity.
Cleanup failure is reported separately and does not relabel a published bundle
as a failed build. Deletion uses Quay's registry tag-name route
`DELETE /v2/bundledock/<repository>/manifests/<staging-tag>`, which removes only
the specified tag. Digest references and production aliases are rejected before
authentication; manifest-by-digest and blob deletion are never used. Published
indexes continue to reference their platform manifests.

The `Clean stale staging tags` workflow also runs after `Publish bundles`
completes with `failure`, `cancelled` or `timed_out`. This includes superseded runs
whose promotion was refused. It checks out trusted default-branch cleanup code,
not code or artifacts from the failed run. Before bypassing the seven-day age
limit, it independently verifies the official main workflow identity, exact latest
run attempt, matching attempt evidence, and completion of every paginated job.
Only staging tags labelled for that exact run and attempt qualify. Successful,
active, rerun, foreign or unverifiable attempts remain untouched. Each repository
uses the publisher's lock, saves a dry-run artifact before applying, and rechecks
live run evidence and tag identity before each tag-name deletion. Completion
cleanup applies automatically when the existing robot credentials are configured.

The daily `Clean stale staging tags` workflow handles abandoned staging tags.
Both the tag and its completed workflow run (including the latest rerun) must be
at least seven days old. Active, recently rerun, immutable, malformed, unrecognized
or unverifiable tags remain. Only the eight official repositories are in scope;
both Node channels share one repository lock. Publisher and fallback concurrency
use `queue: max` so cleanup does not replace pending publication work. See
[GitHub concurrency queue semantics](https://docs.github.com/en/actions/how-tos/write-workflows/choose-when-workflows-run/control-workflow-concurrency).

Cleanup reuses the repository variable `REPOSITORY_USERNAME` and secret
`REPOSITORY_TOKEN`, the same Quay robot credentials used by publication. The robot
needs repository write permission. On apply, the script exchanges these
credentials at `https://quay.io/v2/auth` for a registry bearer token scoped to
`pull,push` on the selected official repository, then deletes only the verified
tag name. It requires no additional secret or OAuth API token. Credentials stay
in headers sent to fixed Quay HTTPS endpoints; redirects are rejected. Public
REST inventory reads send no robot credentials and never acquire push tokens.

Without both robot credentials, workflows retain their plans and report
dry-run-only operation. The fallback workflow's manual dispatch defaults to dry
run; `apply: true` explicitly enables deletions. Scheduled runs apply the verified
plan when the existing robot credentials are configured. To inspect a specific
failed attempt manually, supply both `failed_run` and `failed_attempt`; leave
`apply` false to save its dry-run plan. Empty target fields use the seven-day
fallback. A saved plan cannot be applied under a different mode, run, attempt or
repository scope.

For a local read-only preview, for example:

```sh
python3 -B scripts/staging-cleanup.py --repository toolbox \
  --failed-run 38805010929 --failed-attempt 1 --output staging-plan.json
```

Tag-name deletion has no registry compare-and-swap operation. The shared lock
serializes BundleDock publication and cleanup, and each deletion revalidates the
tag digest, timestamp and provenance. Writers outside these workflows must honor
the same coordination; they can otherwise race the final check. No live cleanup
is performed by authoring this change; existing-tag deletion remains pending.

These workflows do not build images, merge source or modify aliases.

For a local inventory (requires authenticated `gh` read access to run evidence):

```sh
python3 -B scripts/staging-cleanup.py --output staging-plan.json
```

Review every plan row, including retained tags and reasons. To apply that saved
fallback plan with `REPOSITORY_USERNAME` and `REPOSITORY_TOKEN` supplied securely
in the environment:

```sh
python3 -B scripts/staging-cleanup.py --apply \
  --approved-plan staging-plan.json --output staging-result.json
```

For immediate cleanup of an existing verified publication, use `--publication
<receipt.json> --bundle <bundle>` on both plan and apply commands, with
`PUBLICATION_SHA`, `GITHUB_RUN_ID` and `GITHUB_RUN_ATTEMPT` set to that publication's
identity. Save and review the plan first. Keep the publisher serialized while
applying locally. The Quay tag-name deletion route has no conditional digest
check: workflows share a repository concurrency lock and the script rechecks immediately before each
tag deletion, but an external registry writer can still race that final check.
Missing tags are idempotent; moved tags stop cleanup. A partially completed pass
can be planned again. Quay retention/time-machine policy controls later storage
reclamation; removing a tag alone does not promise freed storage.
