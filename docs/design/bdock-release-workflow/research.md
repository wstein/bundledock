# Research: `bdock` CLI release workflow

Facts marked "checked" were verified on 2026-10-17 in this worktree or with `git ls-remote`;
others come from GitHub documentation as the author knows it and are listed in the spec under
known limitations.

## D1 Tag namespace

Checked: `git ls-remote --tags origin` lists 16 tags (32 lines with peeled entries), all image tags `<bundle>/<version>`
(`agy`, `bun`, `claude`, `code-server`, `codex`, `node`, `node-current`, `opentofu`,
`toolbox`), none starting with `bdock/`. Image tags are created by `scripts/bundle-git-tags.py`
only for the exact name `<bundle>/<tag>` of a verified publication; no script enumerates tags.

| Option | Result |
| --- | --- |
| **`bdock/vX.Y.Z`** | Chosen. Same `<name>/<version>` shape as the image tags, so no new convention; `bdock` is not a bundle name, so it cannot collide; the GitHub glob `bdock/v[0-9]+.[0-9]+.[0-9]+` matches no image tag (they never begin with `bdock/`). |
| `vX.Y.Z` | Rejected: invites confusion with the image versions and with a future Go module version; the repository is not a single-artifact repository. |
| `cli/vX.Y.Z` | Rejected: `bdock` is the product name users see. |

Workflow triggers (checked, and pinned by `TestReleaseWorkflowDoesNotTriggerOnOtherTags`):
`ci.yml` has an unfiltered `push:` so it already runs on every tag push, image tags
included, and will on `bdock/` tags. No other workflow has a `push` trigger. Workflows started
by `workflow_run` of CI (`pages.yml`, `publish-bundles.yml`) require `head_branch == main`
(`pages.yml` as a trigger filter, `publish-bundles.yml` in the job condition; a tag run's head
branch is the tag name), so a tag push publishes no documentation and no image; this is
unchanged behaviour for the existing image tags (read from the workflow files, not exercised).
The strict pattern means `bdock/v1.2.3-rc1` starts
nothing. `release.yml` itself triggers on no image tag.

## D2 Version scheme

`MAJOR.MINOR.PATCH`, first release **v0.1.0**. Justification: no `bdock/` tag exists;
`docs/development.md` and the tests already use `0.1.0` and `release-readiness.md` is titled
"v0.1"; `0.x` signals that the CLI surface (`tools volume`, `tools upgrade`) may still change.
The stamped version has no `v` (`bdock version 0.1.0`), as `docs/development.md`
documents; the tag has the `v`.

Go proxy hazard (checked in `go.mod`): the module path is `bundledock`, whose first element has
no dot, so it is not a fetchable module path: `proxy.golang.org` and `sum.golang.org` cannot
resolve it and `go install bundledock/cmd/bdock@…` cannot work. Unlike SheetMill
(`github.com/wstein/sheetmill`, where a published `v0.1.0` is permanently cached by the
proxy), a retag here would not poison any module cache. The rule "never re-create a tag" is
kept anyway, because GitHub release assets, checksums that users copied and the provenance
line `source_revision` all name the first commit. A bad release gets the next patch version.

## D3 One archive builder

`scripts/release.sh` already builds versioned, trimmed, CGO-free archives with `PROVENANCE`
and checksums, and its tests exist. Writing a second packaging script for CI would let local
and CI archives drift. It is extended (darwin/arm64, `TARGETS`, explicit modes, `ustar`) and CI
calls it, one platform per matrix leg. The archive layout stays flat (no top directory), which
users of earlier local archives already know. CI does not use `make release`: the
Makefile target is the approval-gated local entry point; the workflow calls the script and a rule
forbids `make release` in run blocks.

## D4 Determinism

Go with `CGO_ENABLED=0 -trimpath` and a fixed `-X main.version` is deterministic for a given
toolchain and module set; the VCS stamp is deterministic per commit when the tree is clean,
which `source_modified=false` asserts. Archives are `tar --sort=name --mtime=@0 --owner=0
--group=0 --numeric-owner --format=ustar` piped through `gzip -n`; the builder writes the files
with explicit modes. The workflow builds each platform twice (second with an empty `GOCACHE`)
and requires `diff -r` equality of the output trees. This proves no hidden cache or path
dependence on one runner, not independent reproduction.

## D5 Smoke content

`bdock` has no network-free subcommand that does real work: `tools volume` needs Docker and
`tools upgrade` downloads. The smoke runs `--version` (proves the stamp reached the shipped
bytes), `--help`, `tools --help` (proves the cobra tree and both subcommands are registered)
and `completion bash` (exercises cobra's generator), plus archive structure, `LICENSE` and
`PROVENANCE` identity. Rejected: `tools upgrade` against a local fixture (needs HTTP stubs in
a shell script) and Docker on the macOS runner (not available).

## D6 Runners

| Target | Runner | Why |
| --- | --- | --- |
| build (all) | `ubuntu-24.04` | cross-compiles with CGO off; GNU tar for deterministic archives |
| smoke linux/amd64 | `ubuntu-24.04` | native |
| smoke linux/arm64 | `ubuntu-24.04-arm` | native arm64 (free for public repositories; unverified for this repo, a missing label queues and fails closed) |
| smoke darwin/arm64 | `macos-15` | Apple silicon; darwin/amd64 is end of life |

Versions are pinned, not `-latest`, so an image roll cannot change a release silently.

## D7 Verify: proof of "green"

Chosen: query `ci.yml` runs for the commit with `event=push` and `conclusion == success`
(`actions: read`). CI has one job, `check`, so a successful run means `make check` passed.
`cancel-in-progress` is per ref, so tagging a superseded commit may require re-running CI first;
that is a feature. `verify` also runs `make check` and `go mod verify` itself because a dry run
has no tag-commit evidence. Not copied from SheetMill: the `.go-version == go.mod` check,
because here they differ on purpose (`.go-version` 1.27.2 compiles releases; `go.mod` states
the 1.25 minimum, see `docs/architecture.md`).

## D8 Environment and draft

`environment: release` follows the SheetMill model; the tag is the approval and the workflow
works unprotected. `BDOCK_RELEASE_DRAFT=true` makes the release a draft for a manual final
look. The workflow creates neither the environment nor the variable.

## D9 Test dependency

The workflow tests parse YAML. `go.yaml.in/yaml/v3` v3.0.5 is added to `go.mod`; only the
test-only package `internal/releaseworkflow` imports it, so the `bdock` binary does not
change. The module was already in the module cache (hash identical to the SheetMill pin).
Alternatives (PyYAML, absent; hand-parsing YAML) were worse.

## D10 What this release is not

The embedded dependency findings in `docs/development.md` (Gitleaks, Syft, Trivy) concern
third-party binaries inside the OCI toolbox image, which this CLI release neither contains nor
publishes; they do not block or qualify a CLI-only release and are not relabelled here.
