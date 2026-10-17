# Research: third-party source mirror

Measured on 2026-10-17 on Linux arm64 with Go 1.27.2, `curl`, `gh` (read-only API calls), in empty
scratch directories, tarballs extracted with `tar --no-same-owner --no-same-permissions` and never
executed. `cargo`, `cabal` and `stack` were not available; their paths are designed, not measured.

## R1 Classification measurement

`bundles/toolbox/licenses/` on `origin/main` 1cf2173 has 40 directories; the catalog has 41 packages. `fd`
and `rg` have no directory; `xz` is a directory but not a package; 21 packages take notices from the
upstream archive at build time (`install.archive_notices`), so the repository alone does not show their
licenses. A recursive grep for `General Public|Mozilla` over `bundles/*/licenses` hits, in the toolbox,
`fish/` (COPYING, license.rst), `helix/` (LICENSE, GRAMMAR-NOTICES), `shellcheck/LICENSE` and
`typst/ASSETS-NOTICE`; outside the toolbox `opentofu/`, `node/`, `node-current/`; and `bun/` by `LGPL`.
Round 1 corrected an earlier error of this document: **typst is not permissive-only**. Its `SOURCE` states
that the embedded NewCM10-Regular font is "GPL3-or-later with Font and Distribution exceptions" and
`ASSETS-NOTICE` contains the GPL-3.0 text; the earlier claim that the hits were warranty-disclaimer lines
was wrong. `helix`: `GRAMMAR-NOTICES` holds the GPL-3.0 text of
`runtime/grammars/sources/jinja2/LICENSE.md`; the 25.07.1 aarch64 release tarball (SHA256 equals the lock
entry, `ce23fa8d...`) contains `runtime/grammars/jinja2.so`, so the grammar ships. PR #119 adds hadolint
v2.15.1 whose `ThirdPartyNotices.txt` lists HsYAML GPL-2.0-only, ShellCheck GPL-3.0-only, language-docker
GPL-3.0-or-later, colourista and semver MPL-2.0, plus eight further tools not scanned. Embedded
Go/Rust dependency licenses are invisible to a text scan: the round-1 reviewer reported MPL-2.0 crates and
modules in about twenty tools (see the spec); that report is **not** re-measured here, hence WI2.

## R2 What upstream tarballs vendor

| Component | Artifact | Result |
| --- | --- | --- |
| fish 4.9.3 | `fish-4.9.3.tar.xz` release asset, 2 628 460 bytes, sha256 `20998a25...f98c`, 2183 files; the archive holds **duplicate members** `Cargo.lock` and `Cargo.toml` with different bytes | No `vendor/`; `Cargo.lock` has 196 packages, 166 from the crates.io registry, 8 git sources (`git+https://github.com/...`, `git+https://codeberg.org/...`). `.cargo/config.toml` only has an alias. Not self-contained. |
| shellcheck v0.11.0 | GitHub API tarball (the `Source SHA256` already in `SOURCE`, `2f77a26b...914b`, was re-downloaded today and still matches) | 88 files; `ShellCheck.cabal`, `stack.yaml` (`resolver: lts-18.15`, stale), no freeze file. Upstream `builders/` Dockerfiles take a `cabal sdist` and build unfrozen. Not self-contained. |
| trufflehog v3.99.2 | GitHub `archive/refs/tags` tarball (no release source asset exists), `acce1a02...37b3`, 3538 files | `go.mod` (go 1.25.0, one `replace`), `go.sum` 1107 lines, no `vendor/`. Release build is GoReleaser with `CGO_ENABLED=0`. |
| hadolint v2.15.1 | the API tarball `48d1ca56...ab01` (the digest recorded in PR #119, **chosen for the plan**); the `archive/refs/tags` tarball `52fbc1c8...ddb` is a different byte string for the same tag | `cabal.project`, `hadolint.cabal`, no freeze, `ThirdPartyNotices.txt` with 126 table lines. Upstream `release.yml`: static Linux build in `alpine:3.24`, GHC 9.10.2 via ghcup, `cabal` against the live index, static `gmp`, `ncurses`, `zlib` libs. |
| helix 25.07.1 | `helix-25.07.1-source.tar.xz` release asset, 170 088 608 bytes, `2d0cf264...544e` (equals `GRAMMAR-NOTICES` header) | 32 451 files; **absolute symlinks** (for example `runtime/grammars/sources/move/queries` to a `/Users/...` path, `hyprlang/build/node_gyp_bins/python3` to `/usr/bin/python3`; 19 symlinks in total); ships 245 grammar source directories under `runtime/grammars/sources/`; `Cargo.lock` 321 packages, 308 from the registry; no `vendor/`. |

Consequences: `upstream-tarball-only` fits none of the five. "Same tag, different bytes" exists for
GitHub-generated archives (hadolint), so the plan pins the exact URL and the digest of the stored bytes. Real
tarballs contain absolute symlinks and duplicate members, so the tarball is stored unextracted and only
resource limits refuse (spec FR-9). Tag references, read with `gh api`: shellcheck `v0.11.0` is an annotated
tag object `b12c2a6a...` whose **peeled commit is `aac0823e6b58f8a499e856e93738082691cbf212`**; helix `25.07.1`
tag object `ac948410...`, commit `a05c151bb6e8e9c65ec390b0ae2afe7a5efd619b`; fish `4.9.3` tag object
`860edd41...`, commit `6cf31373e9468f1a261319aceb8730e5d1c47e58`; trufflehog `v3.99.2` (lightweight) commit
`e76dff8a7e57d6df0c60ad8cbf5d5cba505a8fc9`; hadolint `v2.15.1` (lightweight) commit
`2eece55955ced00200be9729e9728cb7dacca505`. (An earlier draft listed tag object ids as commits; corrected
after round 1.) The mirror stores both `tag_object` and `tag_commit`, and `pin` peels.

Redirects (HEAD requests): release assets go to `release-assets.githubusercontent.com`, tag archives and API
tarballs to `codeload.github.com`; `proxy.golang.org` and `static.crates.io` answered 200 without redirect.

## R3 Go measurement

For trufflehog v3.99.2 (`GOMODCACHE` in scratch, `GOFLAGS=-mod=mod`, `GOTOOLCHAIN=local`, Go 1.27.2):
`go mod download -json` took 21 s and listed 310 modules (all with a `Sum`), 327 512 339 bytes of zips;
`go mod vendor` gave 157 MB and 312 `# ` lines in `vendor/modules.txt`. Round 1 showed this is not the
designed set: `go.sum` has 1107 non-empty lines, of which **369** are zip lines (not ending in `/go.mod`)
and the rest are `/go.mod`-only (re-verified), `go.mod` has **two** `replace` directives
(`jpillora/overseer`, `coinbase/waas-client-library-go`) and `toolchain go1.25.10`. `go mod download` pulls only
what the build graph needs; the designed rule (every zip line plus `.mod` files) is a superset of it and
its size is unmeasured. WI2 measures the designed set for every Go component.

## R4 Spec 026 state

`/workspaces/Anchorage/sourcesteward` branch `codex/026-release-process-v2` was read at `ab77bac` (first
look: only `specs/026-release-hygiene/`) and again at `9a810a2`, where `specs/026-release-process/` exists
(the first reading was stale; round 1 caught it). Facts used are in
[contracts/mirror-project.md](contracts/mirror-project.md). It is a draft; implementation waits for a
constitution amendment (its own notice).

## D6 Determinism boundary

Tar format `pax-sorted-v1` (the name used in the manifest): Go `archive/tar` with `Format: tar.FormatPAX`,
no PAX time or user records (only `path`/`linkpath` where a name exceeds 100 bytes); entries sorted bytewise
by path; uid/gid 0, empty uname/gname, mode 0644 or 0755, mtime `SOURCE_DATE_EPOCH`. Compression: gzip from
Go `compress/gzip` level 9, empty header name and mtime; stable for a fixed Go release, **not claimed stable
across Go releases**, therefore the plan pins the Go version that builds `bdock` (`bdock_go_version`) and the
manifest records `tar_sha256` (the reproducibility claim) next to `asset_sha256`. The recorded fetch
`argv` is normalized (no output path or flags) so two runs in different directories agree. Fetch inputs
are reproducible only while upstream bytes stay the same; the plan digests pin them.

## Alternatives rejected

| Option | Reason |
| --- | --- |
| Rely on upstream tarball URLs only (status quo) | offer fails when upstream moves (scenario 4) |
| Git mirror of upstream repositories | not "one immutable release per version"; large; no dependency capture |
| Attach tarballs to each bundle release in Quay/GitHub | duplicates large assets per bundle; owner chose link |
| Build from source ourselves | forbidden by `AGENTS.md` |
| Run `go mod vendor` / `cargo vendor` | executes toolchain code on hostile input (FR-8), non-hermetic |
| Put the registry inside `packages.lock.json` | changes updater scope and digests semantics |
| Python script | check must run in the Go test suite beside the loaders; fetch needs bounded HTTP/archive code |
