# Design: Quay support for CI, one tool instead of scattered scripts

Status: proposed, owner direction 2026-10-30 ("full CI related quay.io support":
metadata, read and verify, cleanup). Nothing here is implemented. Secrets,
repository settings and the first live run stay with the owner.

## Problem

Quay access is spread over three places that each carry their own HTTP client,
pagination and error handling:

- `scripts/staging-cleanup.py`: tag inventory (REST, anonymous) and manifest
  deletion (registry v2, robot token).
- `internal/toolpublish` (used by `cmd/tools-publish`): tag allocation, tag absence
  and digest checks.
- `scripts/node-lts-alias.py`: alias retagging.

Repository metadata (the description shown on quay.io, visibility) has no source
in the repository at all; it is edited by hand in the web UI, so it drifts from
the documentation (the toolbox page still named the eight tools that moved to
nixbox).

## Decision

One Go package, `internal/quay`, with a small client and three groups of commands
under `bdock quay`. The CI scripts keep their behavior and call the package step
by step; nothing is rewritten in one go.

| Stage | Command | Mode | Credential |
| --- | --- | --- | --- |
| 1. Metadata | `bdock quay describe <bundle>` (check) and `--apply` | check reads, apply writes | check: none. apply: OAuth token |
| 2. Verify | `bdock quay verify [<bundle>]`, `bdock quay tags <bundle>` | read only | none (the repositories are public) |
| 3. Clean up | `bdock quay clean` (plan) and `--apply` | plan reads, apply deletes | robot token (registry v2) |

### Stage 1: metadata

The description is a reviewed file, `bundles/<name>/description.md`, at most 5000
characters of Markdown, with the links to the documentation site and the source.
`describe` compares it with the live description and reports a difference as a
non-zero exit, so a CI check can fail when the repository and Quay disagree.
`--apply` writes it with `PUT /api/v1/repository/bundledock/<name>`; visibility is
only checked (`public`), never changed implicitly.

Credential limit: a robot password is not an OAuth token and must not be sent to
the REST API (the existing cleanup script already enforces this). Writing the
description therefore needs an OAuth token with "Administer Repositories". It is
read from `QUAY_TOKEN`, never from a file or flag, never printed. Whether this
token lives as a CI secret or stays on the owner's machine is the owner's
decision (open question 1). Check mode needs no credential.

### Stage 2: verify

Read only, anonymous REST, bounded pagination, response size limit and no redirects
(the limits of the existing client carry over).

- `tags <bundle>` lists tags with digests, marks version, alias and staging tags.
- `verify` compares Quay with the committed publication records
  (`docs/<bundle>-published.json`): the version tag and the aliases `latest` and
  `stable` must resolve to the recorded digest, and a staging tag for a published
  version is reported as residue. A bundle that is not yet published is skipped, as
  the cleanup script does.

### Stage 3: cleanup

The plan logic of `scripts/staging-cleanup.py` (seven-day fallback, failed-run
proof, staging-tag pattern only) moves into the package unchanged in behavior; the
script becomes a thin caller until the workflow runs `bdock quay clean`. Deletion
stays limited to tags that match the staging pattern, uses the robot token over
registry v2, and applies only a saved plan after a fresh recheck, as today.

## Safety

- Tokens only from the environment; never in output, errors or the plan file.
- Write commands print the change first and need `--apply` (no implicit write).
- Delete only staging-pattern tags; never version, alias or source tags.
- Tests run against a local fake of the Quay endpoints; CI never calls quay.io for
  tests. A read-only smoke against the public repositories is optional and manual.

## Delivery

Separate PRs, each independently useful:

1. `internal/quay` client and `tags`/`verify` (read only), plus a `verify` step in
   the publication workflow after promotion (reports, does not block).
2. `describe` and the `description.md` files for toolbox and nixbox (apply by the
   owner).
3. Move the cleanup plan into the package; the workflow switches to `bdock quay clean`.

## Open questions for the owner

1. May a Quay OAuth token with repository admin rights become a CI secret, or does
   `describe --apply` stay a local owner command? Recommendation: local command;
   the CI only runs `describe` as a check.
2. Should `verify` after promotion fail the publication run, or only report?
   Recommendation: report; a late registry lag must not turn a good publication red.
3. Replace the Python cleanup script, or keep it beside the Go command? Recommendation:
   replace it in stage 3 after the Go plan matches the script's test cases.
