# Publish the toolbox to Quay

[Back to BundleDock](../README.md)

[The publication workflow](../.github/workflows/bundledock-toolbox.yml) checks all
tools for their latest official stable prebuilt release within `tools.txt` ranges
daily at 06:00 UTC (`0 6 * * *`), and through Actions → **Publish toolbox** →
**Run workflow**. Only the repository default branch can publish. GitHub may delay
scheduled runs; this is not an exact wall-clock delivery guarantee.
Publication runs share one concurrency group; an active run is allowed to finish.
Before promotion, a read-only GitHub API check rejects a run superseded by a newer
eligible run, including one whose native build failed. A newer run with successful
resolution and skipped native jobs is a proven no-op and does not supersede an
older changed publication. Unknown or unfinished eligibility fails closed, with
bounded queries for the exact run attempt. This prevents an old queued run or rerun
from rolling stable back. Start a fresh manual run to recover; when rerunning a
run, choose **Re-run all jobs** so the resolver creates current-attempt artifacts.

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
the default branch and watch the resolver, both native jobs, and promotion in
[Actions](https://github.com/wstein/bundledock/actions/workflows/bundledock-toolbox.yml).
A successful run publishes a dated run-attempt tag, `stable` and `latest`; inspect its
summary and artifacts for the immutable digest before choosing a deployment.
Configuring these values also allows the daily schedule to publish subsequent
tool updates. A manual dispatch always builds and publishes after validating the
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

One resolver runs `bundledock tools upgrade` for all tools, then uploads the exact
lock bytes and shared source commit, UTC build time, lock SHA256, run tag and full
GitHub run ID (`run_id`).
The resolver uses official stable prebuilt releases; it does not rebuild tools
from source or patch embedded dependencies. A successful publication does not
establish that upstream binaries are free of known vulnerabilities. Review the
[remaining embedded dependency findings](development.md#embedded-dependency-findings)
and wait for patched stable releases to remediate them.
It never commits or pushes the resulting lock. Both native jobs consume that
artifact, build `oci/Dockerfile` once, explicitly pull the consumer baseline and
run `oci/smoke.sh` against the resulting local image ID. OCI labels include the
image title, source, revision, build time, run tag, lock hash, tool versions and
`io.bundledock.github.run-id`. Artifact checks still bind to the full
`GITHUB_RUN_ID`, run number, attempt and source commit. The newest-run gate uses
the full run ID to query GitHub; shortening the tag does not change that check.

Scheduled publication compares the resolved tool and asset pins on both
architectures with the existing `stable` image's published baseline, rather than
the committed lock, which may predate earlier automated upgrades. An unchanged
schedule skips both native jobs and promotion, including Docker builds, smoke,
login and pushes. Input range or catalog metadata changes alone do not count as
tool updates; changes to inventory, versions, URLs, checksums or verification do.
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
compare daily updates.

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
