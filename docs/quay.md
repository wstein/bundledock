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
one Quay repository.

## Safety rules

- The REST API is read anonymously. A robot password is never sent to it.
- Tokens come from the environment only, never from a flag or file, and never
  appear in output, errors or saved plans. The `QUAY_TOKEN` stays on the owner's
  machine; CI runs `describe` as a check only.
- Write commands print the change first and write only with `--apply`.
- `prune` deletes only tags that match the staging pattern, by tag name, never by
  digest, and never a version, alias or source tag. `--apply` accepts only a saved
  plan and rechecks every tag against fresh registry and GitHub evidence first.
- Responses are limited to 4 MiB, redirects are not followed, and requests time
  out after 30 seconds.
- Tests use local fakes. They never call quay.io.
