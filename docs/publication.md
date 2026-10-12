---
title: Publication to Quay
description: Configure the destination and understand the native smoke and registry publication gates.
weight: 40
---

# Publish the toolbox to Quay

[Back to BundleDock](../README.md)

The nine named **Publish toolbox**, **Publish code-server**, **Publish node**, **Publish node-current**,
**Publish bun**, **Publish codex**, **Publish agy**, **Publish claude** and **Publish opentofu** entrypoints call one shared publication workflow. Their existing
names, filenames, bundle-specific native smoke checks and manual exact-SHA
fallback remain. The nine **Propose … upgrades** entrypoints call one shared
source-only updater, retaining the daily 06:50 UTC schedule and manual dry-run
default.

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
`github-actions[bot]` updater PR and approve its exact head as **wstein**. That
single approval authorizes the remaining sequence without another intervention:

1. A read-only review validator accepts only the correct same-repository bot
   branch, its own lock and inventory, and the latest non-dismissed exact-head
   approval. Workflow code changes, renamed/deleted files and missing provenance
   are rejected.
2. A controller running trusted default-main code dispatches **CI** on that exact
   updater branch. The full `expected_head_sha` must match the CI run before
   `make check`. The successful required `check` must belong to GitHub Actions
   and cover the approved head, rather than a synthetic review merge SHA.
3. The controller revalidates approval, head and current-main ancestry, then
   requests GitHub's protected rebase merge with an optimistic exact-head SHA.
   It uses the existing strict required check, administrator enforcement and
   linear-history protection; it does not bypass them or change settings.
4. The merged tree must equal the approved head tree. The controller explicitly
   dispatches source-only **CI** on the merged main SHA, because a merge using
   `GITHUB_TOKEN` does not start ordinary push CI. Its completion permits only
   the matching bundle publisher to copy the exact committed lock bytes.
5. The publisher checks approval, tree equality and current main before artifact
   compilers, native jobs and final registry promotion. Both native smoke checks
   and existing baseline, artifact and registry gates must pass.

A changed head or main advance stops this sequence and requires a new review;
the controller does not rewrite an approved branch. The merge API leases the PR
head, not target main. If target movement races a merge, approved/merged tree
comparison prevents publication of unreviewed source. API failures, ambiguous
pagination and missing required provenance fail closed. The existing
`toolbox-publication` environment is unused by this path and is not changed or
deleted. Source PRs and documentation PRs cannot enter this updater-only flow.

For manual publication, select **Run workflow** on default branch `main` and
enter the explicitly approved full 40-character SHA in `approved_sha`. It must
equal the immutable run SHA. Every build job retains the human actor and SHA
gate. Manual dispatch authorizes fresh upstream resolution, both native builds
and publication even when pins are unchanged. Automation must not impersonate a
human to dispatch this path. Consumer image activation needs separate approval.

Automatic publication compares the frozen lock with the existing published
baseline and skips native work when assets already match. Runs share a
per-bundle concurrency group. Newer automatic requests supersede a publisher
only after successful resolution reaches native work; queued or unapproved
requests do not. Current-main revalidation separately rejects stale sources.
Manual runs and unavailable API responses retain fail-closed newest-run checks.
Reruns must revalidate the same approval and current source; use **Re-run all
jobs** to produce artifacts for the current attempt.

The shared helper validates provenance; variables such as `PUBLICATION_SHA`
are not approval evidence. Tests cover exact review/head/attempt/caller identity,
synthetic merge rejection, required-check application identity, frozen lock
bytes, stale-review writes and approved/merged tree equality. The automatic
sequence remains unverified live until an explicitly authorized configured run
succeeds. No workflow run, image build or publication was performed to author
this source change.

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
aliases have been verified against the version digest.

Toolbox keeps its UTC version format
`YYYY.MM.DD-b<github.run_number>.<github.run_attempt>`, such as
`2026.10.10-b12.4`. Its run number distinguishes normal runs and its attempt
counter distinguishes reruns. Missing, malformed or nonpositive counters are
rejected before resolving tools.

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

After successful image promotion, each of the toolbox, code-server, node, node-current, bun, codex, agy, claude and opentofu
workflows creates an annotated Git tag `<bundle>/<immutable-version>` at the
image's exact source commit, for example `node/24.21.0-b1`. Approval of that
publication includes this source tag. All nine bundles support the exact-head
review and merged-updater approval path described above; the shared read-only
source approval gate is revalidated before automatic tagging. The controller
workflow SHA remains distinct from the approved image source SHA, and both must
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
opens a normal `publication-docs/<bundle>/<version>` PR for the corresponding
bundle guide. Its managed publication section records the immutable version,
index digest, exact source SHA, workflow run and digest-pinned pull example.
The generated tool table remains the current source inventory, separate from
the published record. Most version badges read Quay directly and need no README bot commit. Node
badges read the managed `docs/node-published.json` and
`docs/node-current-published.json` metadata, updated in the same publication PR
as the corresponding guide. They refresh after that PR is merged and the badge
cache expires; they do not display the current source lock.

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
