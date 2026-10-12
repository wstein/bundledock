# Contributing to BundleDock

BundleDock owns the Go CLI, tool catalog and lock, OCI builder, smoke tests,
publication workflow, licenses, and brand assets. Anchorage owns the separate
consumer devcontainer integration. Read [AGENTS.md](AGENTS.md) before changing
this repository, and see [development and verification](docs/development.md)
for setup and recorded coverage.

Participation in the BundleDock community follows our [Code of Conduct](CODE_OF_CONDUCT.md),
which includes the private reporting contact and enforcement process.

## Make a focused change

For a bug, provide a minimal reproduction, expected and actual behavior, the
BundleDock revision, host architecture, and relevant consumer/image details.
For a feature, explain the problem and a concrete use case before proposing an
implementation. Keep credentials and private configuration out of reports.

Use test-driven development for behavior changes: first add a test that fails
for the reported behavior, then make it pass, then refactor while keeping the
checks green. Documentation-only edits need appropriate content and link checks.
Keep changes scoped to the repository that owns them; when working through
Anchorage, commit submodule changes there before updating its gitlink.

Make atomic, focused [Conventional Commits](https://www.conventionalcommits.org/en/v1.0.0/),
with one logical change per commit. Use a subject such as
`fix(volume): reject incomplete initialization` and a body explaining why the
change is needed. Preserve existing history and attribution; BundleDock's
[full-history provenance](docs/development.md#repository-history) is intentional.

## Verify your change

Use the compiler pinned in [.go-version](.go-version). Run these targets from
the repository root, choosing the additional checks that cover your change:

| Change | Checks |
| --- | --- |
| Go or build behavior | `make check`; `make build` only with explicit user approval |
| Local CLI release artifacts | `make release-test` |
| Documentation site or rendered guides | `make docs-test` |
| Tool extraction, catalog, or volumes | `make bundle-test`, plus Docker checks below |
| OCI image, tool pins, or consumer compatibility | `make bundle-build bundle-smoke` on native Linux AMD64 and ARM64 |

`make check` checks Go formatting, tests, and vet. Routine test compilation is
allowed. All artifact, image, CLI release, and documentation builds require
explicit user approval before execution, including every build-oriented target
in the table and `make docs-test`/`make release-test`. Report checks deferred for
approval accurately. Push, merge, schedule, and bot activity supply no build or
release approval. For the seven bundle updater PRs, one exact-head approval by wstein authorizes the guarded source-CI, merge and frozen-lock publication flow described in [publication](docs/publication.md). Initial agent-carrier publication still requires separate exact-revision operation approval. Other source changes supply no build approval.

`make docs-test` explicitly opts into host Hugo integration checks; generic Go tests skip them. The docs
wrapper uses a verified official prebuilt Hugo and needs network access for an
uncached archive. See the [Makefile](Makefile) and
[verification guide](docs/development.md#local-documentation-site) for details.
Report the exact revision and platform tested, commands and outcomes, and any
unavailable checks. Historical native smoke results do not verify a new lock.

## Tools, security, and licensing

Third-party tools must use official stable prebuilt releases. Do not build them
from source or patch their binaries or embedded dependencies. Follow the
[tool upgrade guide](docs/toolbox.md#upgrade-the-lock), retain upstream notices,
and verify archives on both supported architectures. When no patched stable
prebuilt is available, record the remaining findings in existing documentation;
see [embedded dependency findings](docs/development.md#embedded-dependency-findings).
A checksum or successful smoke test does not establish vulnerability freedom.

BundleDock uses the [EUPL-1.2](LICENSE). Keep project licensing, upstream tool
notices in [bundles/toolbox/licenses](bundles/toolbox/licenses), and
[brand asset provenance](brand/README.md) accurate when adding or changing assets.

## Submit for review

Explain the problem, resulting behavior, and verification evidence in the pull
request. Link related issues and identify remaining limitations. Keep unrelated
cleanup separate so each commit can be reviewed on its own.

Local checks and commits do not authorize pushing, publishing images, tagging a
release, or activating a consumer image. Follow the
[release review requirement](docs/development.md#release-review) before any release:
every local TODO item needs resolution or an explicit user decision to defer it.
Keep that TODO list untracked.
