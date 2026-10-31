---
title: Quay maintenance
description: Inspect, verify and maintain the bundledock repositories on Quay with bdock quay.
weight: 45
---

# Quay maintenance

[Back to BundleDock](../README.md) · [All bundles](bundles.md) · [Publication](publication.md)

`bdock quay` reads and maintains the public `quay.io/bundledock` repositories: it
lists what is published, compares Quay with the committed publication records,
keeps the repository description in sync with the repository, and plans the removal
of leftover staging tags. Run it from the repository root, or pass `--root`.

The design and the owner decisions are in
[the Quay CI support spec](design/quay-ci-support/spec.md).

## Commands

| Command | Does | Credential |
| --- | --- | --- |
| `bdock quay list` | Each bundle with its newest version tag, digest and number of staging tags. | none |
| `bdock quay status [bundle]` | Compares Quay with `docs/<bundle>-published.json`. Exit code 1 on any difference. | none |
| `bdock quay info <bundle>` | All tags with digest, age, architectures and the `org.opencontainers.image.version` and `revision` labels. | none |
| `bdock quay describe [bundle]` | Compares `bundles/<name>/description.md` with the live description. Exit code 1 on a difference. | none |
| `bdock quay describe [bundle] --apply` | Writes the descriptions that differ. | `QUAY_TOKEN` (OAuth token) |
| `bdock quay describe --init <bundle>` | Writes a draft `bundles/<name>/description.md` from the bundle name, the purpose in `docs/<bundle>.md` and the two standard links. Never overwrites a file. | none, no network |
| `bdock quay prune` | Plans the removal of staging tags; changes nothing. | none for the plan |
| `bdock quay prune --apply --approved-plan <file>` | Deletes the eligible tags of a saved plan after a fresh recheck. | `REPOSITORY_USERNAME`, `REPOSITORY_TOKEN` (robot) |

Every command accepts `--json`. It writes one JSON document with a `schema` field
to stdout and all messages to stderr. The exit code does not depend on the flag.

### status

The version tag and the aliases of the bundle (`stable` and `latest`, `current` for
`node-current`, plus `lts` for `node`) must resolve to the digest in the publication
record. A staging tag of a published version is reported as residue. A bundle with
no publication record is skipped. `status` reads only, so it can run after promotion
and report in the run summary without blocking the publication.

### describe

The description is a reviewed Markdown file of at most 5000 characters. Without a
bundle, `describe` checks every bundle that has a description file and skips the
others. A repository that is not public is reported as a difference; visibility is
never changed.

`describe --init <bundle>` starts a new description: it takes the `description` from
the front matter of `docs/<bundle>.md` (or its first paragraph), adds the links to
the documentation site and the source, and fails when the file already exists.
Review and edit the draft before you commit it.

### prune

A staging tag (`staging-<version>-<arch>`) is eligible when its publication run is
complete and the tag and the run are older than seven days, or, with
`--failed-run <id> --failed-attempt <n>`, when it belongs to that exact completed
failed attempt. Save a plan with `--output plan.json`, review it, then apply it:

```sh
bdock quay prune --output plan.json
bdock quay prune --apply --approved-plan plan.json
```

GitHub evidence comes from the `gh` CLI. `--repository <name>` limits the scan to
one Quay repository. `--apply` reads the plan file saved with `--output`; the
`--json` output on stdout is a report, not a plan file. Registry evidence is read
with the robot account when `REPOSITORY_USERNAME` and `REPOSITORY_TOKEN` are set,
else anonymously.

When `GITHUB_STEP_SUMMARY` is set, `prune` appends one section to that file: the
plan (counts and one row per staging tag with its decision and reason) or, with
`--apply`, the actions and, if it stopped, the reason. It only appends, shows no
credential, and lists at most 100 rows (the saved plan has all of them). A
failure to write it is a warning, not a failed cleanup.

#### Publication mode

`bdock quay prune --publication <receipt> --bundle <name>` removes the two staging
tags (`staging-<version>-amd64` and `-arm64`) of one verified publication, right
after promotion, regardless of age. It first proves the receipt:

- The receipt matches the expected provenance in the environment: `PUBLICATION_SHA`
  (the approved source commit), `GITHUB_RUN_ID` and `GITHUB_RUN_ATTEMPT`.
- The commit is an ancestor of `origin/main`, and `origin` is the official
  repository (fetch and push URL).
- The version tag and every alias of the bundle resolve to the recorded index,
  which holds exactly linux/amd64 and linux/arm64 images whose labels match.
- The index platform digests equal the receipt's, and the annotated source tag
  `<bundle>/<version>` exists with the exact provenance message.

A staging tag is then eligible only when its provenance labels, its GitHub run and
its digest equal the receipt. The plan has `"mode": "publication"` and no target.
`--apply` proves the receipt again, rechecks each tag, and accepts only plan rows
for the two staging names of that receipt. `--publication` needs `--bundle`, and
the two cannot be combined with `--failed-run`; `--repository` is ignored.
The publication workflow runs this mode after promotion, and the daily
`staging-cleanup.yml` workflow runs the fallback and failed-run modes, both with
`go run ./cmd/bdock quay prune` from the checked-out source.

## Safety rules

- The REST API is read anonymously. A robot password is never sent to it, and the
  OAuth token and the robot credentials are sent only to `https://quay.io`; there
  is no option to change the endpoint.
- Tokens come from the environment only, never from a flag or file, and never
  appear in output, errors or saved plans. The `QUAY_TOKEN` stays on the owner's
  machine; CI runs `describe` as a check only.
- Write commands change nothing without `--apply`; without it they only report
  the difference or the plan.
- `prune` deletes only tags that match the staging pattern, by tag name, never by
  digest, and never a version, alias or source tag. `--apply` accepts only a saved
  plan and rechecks every tag against fresh registry and GitHub evidence first.
- Responses are limited to 4 MiB and requests time out after 30 seconds. Redirects
  are not followed, except that a registry blob may redirect once to signed https
  storage, which receives no credential.
- Tests use local fakes. They never call quay.io.
