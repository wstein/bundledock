# Research: third-party source mirror

Measured on 2026-10-17 on Linux arm64 with Go 1.27.2, `curl`, `gh` (read-only API calls), in empty
scratch directories, tarballs extracted with `tar --no-same-owner --no-same-permissions` and never
executed. `cargo`, `cabal` and `stack` were not available; their paths are designed, not measured.

## R1 Classification measurement

`bundles/toolbox/licenses/` on `origin/main` 1cf2173 has 40 directories. A recursive grep for
`GNU (Affero |Lesser |Library )?General Public`, `Mozilla Public License`, `AGPL`, `LGPL`, `MPL-2` found
hits in `fish/` (COPYING, license.rst, SOURCE), `helix/` (GRAMMAR-NOTICES, LICENSE, SOURCE),
`shellcheck/LICENSE`, `trufflehog/SOURCE`, and `typst/ASSETS-NOTICE`. The `typst` hits are the
warranty-disclaimer lines of font licenses, not copyleft (read, lines 88, 174, 223, 282); typst is
`none`. `helix/GRAMMAR-NOTICES` holds the full GPL-3.0 text of
`runtime/grammars/sources/jinja2/LICENSE.md`; the 25.07.1 source tarball contains that grammar and
`languages.toml` selects `grammar = "jinja2"`. Whether the compiled jinja2 grammar is in the shipped
runtime is **not verified** (the shipped binary tarball was not unpacked); the retained notice file is
named "compiled grammar notices", which is the reason to treat it as shipped.
PR #119 (head `feat/toolbox-batch-1`) adds hadolint v2.15.1 whose `ThirdPartyNotices.txt` lists
`HsYAML` GPL-2.0-only, `ShellCheck` GPL-3.0-only, `language-docker` GPL-3.0-or-later and
`colourista` MPL-2.0. A text scan cannot see licenses of embedded Go/Rust dependencies whose notices
upstream omitted, hence provisional `none`.

## R2 What upstream tarballs vendor

| Component | Artifact | Result |
| --- | --- | --- |
| fish 4.9.3 | `fish-4.9.3.tar.xz` release asset, 2 628 460 bytes, sha256 `20998a25...f98c`, 2183 files | No `vendor/`; `Cargo.lock` has 196 packages, 166 from the crates.io registry, 8 git sources (`git+https://github.com/...`, `git+https://codeberg.org/...`). `.cargo/config.toml` only has an alias. Not self-contained. |
| shellcheck v0.11.0 | GitHub API tarball (the `Source SHA256` already in `SOURCE`, `2f77a26b...914b`, was re-downloaded today and still matches) | 88 files; `ShellCheck.cabal`, `stack.yaml` (`resolver: lts-18.15`, stale), no freeze file. Upstream `builders/` Dockerfiles take a `cabal sdist` and build unfrozen. Not self-contained. |
| trufflehog v3.99.2 | GitHub `archive/refs/tags` tarball (no release source asset exists), `acce1a02...37b3`, 3538 files | `go.mod` (go 1.25.0, one `replace`), `go.sum` 1107 lines, no `vendor/`. Release build is GoReleaser with `CGO_ENABLED=0`. |
| hadolint v2.15.1 | `archive/refs/tags` tarball `52fbc1c8...ddb`; the API tarball `48d1ca56...ab01` (the digest recorded in PR #119) is a different byte string for the same tag | `cabal.project`, `hadolint.cabal`, no freeze, `ThirdPartyNotices.txt` with 126 table lines. Upstream `release.yml`: static Linux build in `alpine:3.24`, GHC 9.10.2 via ghcup, `cabal` against the live index, static `gmp`, `ncurses`, `zlib` libs. |
| helix 25.07.1 | `helix-25.07.1-source.tar.xz` release asset, 170 088 608 bytes, `2d0cf264...544e` (equals `GRAMMAR-NOTICES` header) | 32 451 files; ships 245 grammar source directories under `runtime/grammars/sources/`; `Cargo.lock` 321 packages, 308 from the registry; no `vendor/`. |

Consequences: `upstream-tarball-only` fits none of the five today. Two kinds of "same tag, different
bytes" exist for GitHub-generated archives (hadolint above), so the plan must pin the exact URL **and**
the digest of the bytes the mirror stores; the manifest also records the tag's peeled commit
(shellcheck `b12c2a6a...`, trufflehog `e76dff8a...`, hadolint `2eece559...`, helix `ac948410...`,
fish `860edd41...`, read with `gh api repos/.../git/ref/tags/...`, annotated tag objects not peeled
by that call, to be peeled in WI5).

## R3 Go measurement

For trufflehog v3.99.2 (`GOMODCACHE` in scratch, `GOFLAGS=-mod=mod`, `GOTOOLCHAIN=local`):
`go mod download -json` finished in 21 s, 310 modules, each with a `Sum`, 327 512 339 bytes of zips
(1.5 GB cache including extracted trees); `go mod vendor` produced a 157 MB tree and 312 lines
starting with `# ` in `vendor/modules.txt`. These ran Go's own tooling and are measurements for
sizing; the shipped design does not invoke `go` (D4).

## R4 Spec 026 state

`/workspaces/Anchorage/sourcesteward` branch `codex/026-release-process-v2` (head `ab77bac`)
contains `specs/026-release-hygiene/` (release hygiene for the `stw` CLI: gates G01..G14,
`scripts/release.py plan|build|verify`). It has no artifact kinds `file` or `oci-image` and no
"G1 build-once-promote" text. The brief says Spec 026 v2 is being written in parallel; this design
treats it as not yet existing and states what it needs from it
([contract](contracts/mirror-project.md)).

## D6 Determinism boundary

Tar: PAX-free USTAR where names fit, GNU long-name extension avoided; Go's `archive/tar` with
`Format: tar.FormatPAX` and no PAX time records is used only for names over 100 bytes; entries sorted
bytewise by path; uid/gid 0, empty uname/gname, mode 0644/0755, mtime = `SOURCE_DATE_EPOCH`.
Compression: gzip from Go's `compress/gzip` level 9, empty header name/mtime; stable for a fixed Go
release, **not claimed stable across Go releases**. Therefore the manifest records `tar_sha256`
(uncompressed, the reproducibility claim) and `asset_sha256` (the published bytes). Reproducibility of
the fetch inputs depends on upstream bytes staying the same; the plan's digests pin them.

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
