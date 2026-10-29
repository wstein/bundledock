# BundleDock repository

BundleDock owns the Go CLI, declarative tool inputs, OCI build and smoke tests,
license notices, publication workflow, and brand assets. Anchorage is the
separate umbrella repository that owns consumer devcontainer integration.

Run `make check` before handing over Go or build changes (with `umask 022`; a
sandbox `umask 0000` breaks the Python tool-state checks). Routine source tests,
vet and their compiler verification may run automatically. Obtain the user's
explicit approval before any artifact, CLI release, OCI image, or documentation
build outside the Pages automation exception and the CLI release workflow exception below, including `make build`,
`make release`, `make release-test`, `make docs`, `make docs-test`, `make docs-serve`, or Docker build/smoke checks. Report deferred
checks and platform coverage accurately; a passing source check is not image or
release readiness.

The Pages-specific automation exception authorizes documentation build and
Pages deployment after successful official `CI` for the exact current `main`
revision, from a main push or manual main CI run. Independently verify the live
CI run, exact latest attempt, successful required `check` job, official repository
and workflow identity, and unchanged current main before building and deploying.
PR/branch CI, stale sources, missing proof and failed/skipped checks do not
authorize publication. Check out the verified SHA and bind the Pages artifact to
that SHA and the publication run attempt. Manual exact-SHA human dispatch remains
available. This exception does not authorize OCI/CLI publication, source merges,
workflow dispatches by agents or repository/environment settings changes.

Builds and releases other than the Pages exception and the CLI release workflow exception require explicit user approval
of the intended operation and source revision. The approved bundle automation
exception covers toolbox,
code-server, Node LTS, Node Current, Bun, Codex, Antigravity, Claude and OpenTofu updater PRs authored by `github-actions[bot]`, changing
only their own lock and generated inventory; a combined updater PR changes the
locks and inventories of several bundles, each checked like a per-bundle updater. The actual human owner `wstein`
merging an eligible updater authorizes publication of that exact frozen lock;
a submitted APPROVED review is not required. Source merges are the human's job:
no review-triggered controller may merge or dispatch builds. The successful
exact merged-main CI and required GitHub Actions `check` must cover that source.
The publisher verifies the actual `merged_by` identity, bot author, branch and
own-file scope of every bundle the PR changes, exact merged source and updater-head tree equality, frozen lock,
and current main before build jobs and promotion. Main may advance during a
build only if the approved source remains its ancestor and the bundle's own lock
is byte-identical there. Changed heads, a diverged or rewritten main, a changed
own lock, unrelated files, ambiguous or unavailable API evidence, and differing
trees stop automatic publication. Existing branch protection and required checks remain.
There is no later environment
approval. Initial publication of Node Current, Codex, Antigravity, Claude and OpenTofu carriers requires
separate explicit approval of the exact full source revision and intended
build/publication operation; an updater PR cannot authorize a first publication.
Verify repository visibility and robot access before that operation. Existing environment settings remain untouched. Manual publication
retains exact full-SHA human dispatch. OpenTofu initial publication requires
this manual gate; a bot updater cannot bootstrap an absent stable baseline. Source PRs, documentation PRs, schedules,
bot activity and local commits alone supply no artifact build approval outside
the Pages-specific exception above and the CLI release workflow exception below.
Tagging and publication must follow the explicitly authorized workflow scope (the CLI release workflow exception below is the only CLI one);
consumer activation remains a separate explicit authorization. Successful approved
bundle publication includes one annotated `<bundle>/<immutable-version>` source tag,
after verification of the final index and image aliases. Never overwrite source
tags or create moving Git aliases. Backfills require separate explicit approval
and independently verified deployed-image/source/workflow provenance. Do not perform
live merges, workflow runs, builds or settings changes while authoring source.

CLI release workflow exception (owner decision, recorded here). On 2026-10-17
Werner Stein asked in chat "what about https://github.com/wstein/bundledock/tree/main/cmd
cli release?" and, asked whether `bdock` should be released like SheetMill with a
tag-triggered CI release or prepared locally, decided: "bdock wie smill". Read as the
SheetMill model: an owner-pushed annotated tag `bdock/vMAJOR.MINOR.PATCH` on a `main`
commit with a successful `CI` push run is the publication approval for that tag's CLI
archives (linux/amd64, linux/arm64, darwin/arm64) as GitHub Release assets, built and
published only by `.github/workflows/release.yml`; a manual run is an owner-only dry run
that never publishes. Design and review record: `docs/design/bdock-release-workflow/`;
owner procedure: `docs/releasing.md`. This does not authorize: merging the workflow,
creating, moving or deleting any tag, starting a run (the owner does), `make release` or
any other artifact build by an agent, creating the `release` environment, variables or other
repository settings, OCI images or their tags, consumer activation, or any other tag
namespace. Tags are never re-created after a failed or wrong release; use the next patch
version. There is no standing approval (owner decision): every tag and release, including later ones,
needs Werner Stein's explicit approval each time, and an approval for one tag (for example
`bdock/v0.1.1`) covers only that tag. The grant is the workflow's publication on the owner's own
approved tag push under these gates. The `release` environment has no reviewer rule (owner
decision), so a tag push by the owner that passes the checks publishes immediately. Agents
never push tags and never start the workflow.

Third-party tools must use official stable prebuilt releases. Never silently
patch any tool binary or embedded dependency. Building a third-party tool from
source is the exception and is allowed only as a reviewed, recipe-driven, static
source build when all of these hold: (a) the tool lacks an official prebuilt
release for amd64 or for arm64 (a tool with an official prebuilt for both
architectures is never built from source; the review records the publisher
download page checked and the date); (b) the toolchain is pinned by content;
(c) every pinned source (the source archive and any further pinned sources) has a recorded
checksum from publisher data, or, where the publisher offers none, is a
commit-pinned source fixed by commit SHA plus the builder-computed SHA256 of the
fetched files (class `tls-download-sha256`), which is a named exception with its residual risk
recorded and needs owner approval for every bump of that pin; and (d) a test
proves the output is statically linked. Each new case needs its own review.
If a
security fix is not yet available in an official stable prebuilt release, wait
for that release and record the remaining findings in existing documentation.
This does not restrict compiling BundleDock's own CLI, builder, or initializer.

The class "distribution binary cache" is a second, separate exception, used only
by the nixbox bundle. It allows content from the Nix binary cache when all of
these hold: (a) the content is a closed list of store paths in a committed lock,
pinned by nixpkgs revision and `narHash`; (b) it is fetched only from
`cache.nixos.org` with signature checking on, and every path is verified against
the lock; (c) nothing is compiled; (d) unfree packages are excluded; (e) the
licenses of glibc and of every closure path are recorded; and (f) the first
publication and every change to the list of store paths need explicit owner
approval, and an updater PR cannot supply either. Anything outside this class
follows the rules above.

Before the next release, review every item in the local TODO list and its
supporting evidence. Resolve unfinished or blocked items, or obtain an explicit
user decision to defer each one, before creating a release tag or publishing.
Keep the local TODO list untracked.

Make multiple atomic, focused commits using Conventional Commits.
Keep each commit limited to one logical change.

Rolling prebuilt releases within packages.txt constraints are a normal success
path. Tests reading production locks must assert allowed stable versions,
architecture agreement, official recipes and checksum provenance rather than
historical version/checksum constants. Keep exact release/normalization examples
in fixed fixtures. Run the offline synthetic lock-upgrade regression across all
bundles when changing these contracts. Historical license snapshots retain their
reviewed source identity and exact license bytes independently of current binary
pins; manifests and locks describe the installed artifacts. Separate docs-tool pins remain exceptions.

Use test-driven development (TDD): write a failing test before implementing behavior changes, make it pass, then refactor.

Write all English text in US spelling: documentation, comments, commit messages, pull
request text and identifiers (color, behavior, license, canceled, catalog, labeled,
analyzed, organization). This applies to new and changed text. Do not convert existing
British spellings inside an unrelated change; a separate sweep does that. Keep exact
quotations, testdata, goldens and external names unchanged.

## Pull requests and history

Agents push only their own topic branches, rewrite their own branches with an
explicit lease (a rewritten head of a branch that changes code, tests, scripts or generated
files is independently reviewed at its exact new SHA before the lease push; a branch
that changes only specs or documentation needs before it only checks and, for a pure
rebase, `git range-diff` equality), and open pull requests for reviewed work within the requested task. They never push the default branch or tags, never
merge, and never rewrite commits they did not author. The owner merges. A containing
workspace may add owner-workflow rules in its own `AGENTS.md`; follow them when present.

## Common CLI tools

The tools OCI image bundles the commands below. Rebuild the tools image and
reinitialize its volume to make additions available; the current devcontainer
still pins an older image, so do not assume these commands exist in a running session.

| Task | Tool |
| --- | --- |
| Search text | Reach first for `rg`; use `rg --files` to list files. |
| Filter lists interactively | `fzf`. |
| View highlighted file content | `bat` (optional pager). |
| List directory contents | `lsd`. |
| Inspect OCI registries | `regctl`. |
| Find files by name | `fd` or its `fdfind` alias. |
| Jump to frequently used directories | `zoxide`; the opted-in shared shell layer initializes interactive `cd`/`cdi` (disable with `BDOCK_ZOXIDE=0`). |
| Query code structure | `ast-grep`; use the full binary name, never `sg`. |
| Process structured data | `jq` for JSON; `yq` for YAML. |
| Validate JSON instances against JSON Schema | `jsonschema-cli`. |
| Manage Python environments and run Python tools | `uv` and `uvx`. |
| GitHub workflows | `gh`; `gh stack` is shipped at `libexec/gh-stack` and needs a consumer symlink (docs/toolbox.md). |
| Work with Jujutsu repositories | `jj`. |
| Browse Git repositories interactively | `lazygit`. |
| Manage Git large file storage | `git-lfs` (opt-in repository setup). |
| Manage terminal workspaces and agent sessions | `herdr`. |
| Use the preferred interactive shell | `fish`. |
| Use an alternative structured shell | `nu` (Nushell). |
| Use an alternative terminal multiplexer | `tmux`. |
| Monitor processes and system resources | `btm` (bottom) and `btop`. |
| Customize a shell prompt | `starship`. |
| Share Git/jj shortcuts and the prompt across bash, zsh, fish and nu | `share/shell` and the XDG vendor files; see the shared shell layer in docs/toolbox.md. |
| Preview Markdown in the terminal | `leaf`. |
| Edit text in the terminal | `hx` (Helix). |
| Build static sites | `hugo` (standard). |
| Check shell scripts | `shellcheck`, `shfmt`. |
| Run project recipes | `just`. |
| Lint and format Python | `ruff`. |
| Generate changelogs from Conventional Commits | `git-cliff`. |
| View diffs | `delta`. |
| Compare files structurally | `difft`. |
| Watch files and benchmark commands | `watchexec`, `hyperfine`. |
| Encrypt files | `age`, `age-keygen`, `age-inspect`. |
| Check spelling | `typos`. |
| Check links | `lychee`. |
| Detect secrets | `gitleaks`. |
| Audit GitHub Actions workflows for security issues | `zizmor` (use `--offline` to avoid GitHub API calls). |
| Pin GitHub Actions to commit SHAs | `pinact` (`pinact run` edits workflow files; use `--check` to verify only). |
| Lint Dockerfiles | `hadolint`. |
| Lint GitHub Actions workflows | `actionlint`. |
| Scan Git history and files for credentials | `trufflehog` (disable verification for offline scans). |
| Scan vulnerabilities and configuration | `trivy`. |
| Generate software bills of materials | `syft`. |
| Verify and sign container images and artifacts | `cosign`. |

Keep HOME, caches, and virtual environments in writable locations outside the
read-only `/opt/bundledock/toolbox` tool bundle.
