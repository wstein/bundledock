# Publish the toolbox to Quay

[Back to BundleDock](../README.md)

[The publication workflow](../.github/workflows/bundledock-toolbox.yml) runs Mondays
at 03:17 UTC and through Actions → **Publish bundledock-toolbox** → **Run workflow**.
Only the repository default branch can publish. GitHub may delay scheduled runs.
Publication runs share one concurrency group; an active run is allowed to finish.
Before promotion, a read-only GitHub API check rejects a run superseded by a newer
eligible run, including one that failed. This prevents an old queued run or rerun
from rolling stable back. Start a fresh manual run to recover; when rerunning a
run, choose **Re-run all jobs** so the resolver creates current-attempt artifacts.

Set up the destination before the first publication:

1. Sign in to [Quay.io](https://quay.io/) and choose the user or organization
   namespace that will own the image. Create a repository named
   `bundledock-toolbox` in that namespace. Choose public visibility for anonymous
   pulls or private visibility when consumers will authenticate; this choice is
   independent of the GitHub source repository's visibility.
2. Create a dedicated robot account in the namespace's **Robot Accounts** tab.
   In the target repository's permissions, grant that robot **Write** access to
   `bundledock-toolbox` only. It does not need namespace administration or access
   to unrelated repositories. Its username has the form `namespace+robotname`.
3. In [BundleDock's Actions settings](https://github.com/wstein/bundledock/settings/secrets/actions),
   add the repository variable and secrets below. Enter the robot token directly
   as the secret value; keep it out of Git, logs, issues, and command arguments.


| Setting | Value |
| --- | --- |
| Repository variable `QUAY_IMAGE` | `quay.io/<namespace>/bundledock-toolbox` |
| Secret `QUAY_USERNAME` | Quay robot account username |
| Secret `QUAY_PASSWORD` | Robot token with write access to that repository |

After setting these values, start **Publish bundledock-toolbox** manually from
the default branch and watch the resolver, both native jobs, and promotion in
[Actions](https://github.com/wstein/bundledock/actions/workflows/bundledock-toolbox.yml).
A successful run publishes a dated run-attempt tag and `stable`; inspect its
summary and artifacts for the immutable digest before choosing a deployment.
Configuring these values also allows the Monday schedule to publish subsequent
updates. No registry publication has been verified until a configured run succeeds.
For a private Quay repository, consumers need their own read credentials.

The Quay namespace is deployment configuration, with no default in the workflow.
Use a dedicated [Quay robot account](https://docs.quay.io/glossary/robot-accounts.html).
The workflow uses read-only contents permission and read-only Actions permission
in the promotion job for the newest-run check. It never runs for pull
requests, and supplies registry credentials only after each native smoke passes.
Check [GitHub runner availability and plan limits](https://docs.github.com/en/actions/how-tos/write-workflows/choose-where-workflows-run/choose-the-runner-for-a-job)
for `ubuntu-24.04` (AMD64) and `ubuntu-24.04-arm` (ARM64).

One resolver runs `bundledock tools upgrade` for all tools, then uploads the exact
lock bytes and shared source commit, UTC build time, lock SHA256 and run tag.
It never commits or pushes the resulting lock. Both native jobs consume that
artifact, build `oci/Dockerfile` once, explicitly pull the consumer baseline and
run `oci/smoke.sh` against the resulting local image ID. OCI labels include the
image title, source, revision, build time, run tag, lock hash and tool versions.

A native job pushes its exact tested image to `staging-<run-tag>-<architecture>`.
If either architecture fails, final promotion is skipped. Once both pass, the
adapter verifies each remote registry manifest references its smoked image config,
creates the two-platform manifest and verifies its platform digests before updating
`stable`. The version tag is `YYYY.MM.DD-<github.run_id>-<github.run_attempt>`, so
reruns use distinct tags. These names are unique per attempt; registry-enforced
immutability requires separate Quay configuration. The lock hash is provenance,
not a build identifier: base layers or Debian packages can change for the same lock.

Download `resolved-tools-<attempt>`, `published-amd64-<attempt>`,
`published-arm64-<attempt>` and `publication-<attempt>` artifacts from the run (retained for 90 days). The run summary and `publication.json`
report the remote multi-platform registry digest, which differs from a local Docker
image ID. Use the reported `quay.io/<namespace>/bundledock-toolbox@sha256:...` reference
for explicit deployment or rollback. Publication does not change the devcontainer
pin, initialize host volumes, or activate the new image.

Quay publication requires the first successful configured publication run.
See [development and verification](development.md#verification-evidence) for
local checks and the scope of historical native CI results.
