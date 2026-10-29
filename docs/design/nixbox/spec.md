# Design: nixbox, a Nix-store bundle next to the classic bundles

Status: decided, owner direction 2026-10-29 (name `nixbox`, option B below, hybrid
sources). The first implementation is a local build only. Publication, tags and
consumer pin changes stay with the owner and need their own approval.

## Problem

The classic toolbox ships official prebuilt binaries. Two groups do not fit it:

- Tools without an official Linux prebuilt for both architectures that the
  recovery and devcontainer use cases need: `git` without Perl, `rsync`, `zstd`,
  `less` and an SSH client. None of their projects publishes Linux binaries.
- Eight official binaries are glibc-dynamic and do not start on musl images such as
  Alpine: `ast-grep`, `delta`, `difft`, `fastfetch`, `leaf`, `zizmor`, `zk` and the
  `hx` launcher's `lib/helix/hx`. The other 63 binaries are static.

## Decision

Add a second bundle, `nixbox`, whose content comes in Nix store layout. The classic
toolbox stays as it is. Each tool gets a provider in the catalog:

| Provider | Rule |
| --- | --- |
| `official` | An official static prebuilt exists for amd64 and arm64. Stays in the toolbox. |
| `nix` | The tool needs glibc libraries, or has no official prebuilt. Delivered by nixbox. |

Own tools (`whr`, `stw`, `smill`, `cscribe`, `herdr`) stay in the toolbox.

Placement limit: Nix-sourced store paths are limited to the glibc and libgcc runner
and to tools that have no official stable prebuilt for both architectures. A tool
with official prebuilts for both architectures (for example `node` or `opentofu`)
stays on the official route and never enters nixbox.

## Names and layout

- Repository `quay.io/bundledock/nixbox`, tags in the existing `YYYY.MM.DD-b<N>`
  scheme, so the tag pattern does not change. A separate name replaces the earlier
  idea of an `-n<N>` tag suffix.
- Volume `bundledock-nixbox-<digest>`, mounted read-only at `/nix`. It holds
  `/nix/store/<hash>-<name>` closures, the Nix database and a profile with `bin/`
  and `share/` symlinks. This is a real Nix store, so a native store can replace it
  later without changing paths.
- `PATH` gains the profile `bin` directory. The classic `/opt/bdock/<name>` mounts
  stay unchanged.
- Only one volume can be mounted at `/nix`. nixbox is therefore the profile volume
  that later Nix-delivered bundles join; it is not a fixed tool list.
- A native Nix store on the host works on Linux hosts. On macOS a host store is in
  Darwin format and unusable in Linux containers; there it must be a Linux store in
  a Docker volume or Linux VM. The layout is identical.

## Evidence (spikes of 2026-10-29)

Packages came from `cache.nixos.org` (`nix copy`, nothing built) into a scratch
volume mounted read-only at `/nix/store`, on unmodified images with a read-only
root file system and a tmpfs `/tmp`.

| Platform | Images | Checks |
| --- | --- | --- |
| arm64 | Debian slim, Fedora, openSUSE Leap, Alpine | 64 of 64 (root) |
| amd64 (Rosetta) | Debian slim, Fedora, openSUSE Leap, Alpine | 64 of 64 (root) |
| arm64, uid 1000 | Debian slim | 13 of 16 |

The checks run real operations: `git` commit, `fsck`, bundle, stash, submodule and
`ls-remote` over https; `rsync` copy with progress; `zstd` round trip; `less`;
`ssh` config parsing; and the eight formerly failing tools. The three non-root
failures are test artifacts: an unreadable `/etc/.pwd.lock` as `rsync` source and
OpenSSH's "No user exists for uid 1000" for a uid missing from `/etc/passwd`.

No Perl, Python, Ruby or Tcl appears in the closure of any tested package.

### Closure sizes (arm64, MiB)

| Set | Size |
| --- | --- |
| `git-minimal`, `rsync`, `zstd`, `less`, `openssh`, CA bundle | 197 |
| plus `zk`, `zizmor`, `helix`, `delta`, `difftastic`, `ast-grep` | 638 |
| plus `fastfetch` (442 alone, it links ImageMagick) | 987 |

### Source availability in nixpkgs-unstable

| Bundle | nixpkgs | Pin | Cached |
| --- | --- | --- | --- |
| `codex` | 0.165.0 | 0.165.0 | yes (about 1 GiB closure) |
| `node` | 24.21.0 | 24.22.2 | yes |
| `opentofu` | 1.13.2 | 1.13.1 | yes |
| `claude-code` | 2.1.311 | 2.1.312 | no, unfree |
| `antigravity` | 2.5.5 | agy 1.4.10 | no, unfree |
| `code-server` | 4.143.1 | none | no |

Static variants (`pkgsStatic`) are cached only for `zstd`; `git-minimal`, `rsync`,
`less` and `openssh` would have to be built.

## fastfetch and the other glibc binaries

The official `fastfetch` needs only `libc` and `libm` and loads about 30 optional
libraries with `dlopen`, which is why it is dynamic and why the toolbox copy needs
no large closure. nixpkgs links those libraries, hence 442 MiB.

An unmodified official binary runs on Alpine through the Nix glibc loader:
`ld-linux-aarch64.so.1 --library-path <glibc>/lib:<gcc-lib>/lib <binary>`. All eight
binaries passed on arm64 (the `hx` launcher needs the loader prefix on its inner
binary). The needed files are about 3.5 MiB, or about 57 MiB as whole store paths.
Not verified: a trimmed copy, the amd64 equivalent, and gcompat's segfault cause.

Decision (owner, 2026-10-29): option B. `fastfetch` and the other seven tools stay
official binaries; nixbox adds glibc, libgcc and shims. `fastfetch` leaves the
toolbox in a later toolbox release, after consumers mount nixbox.

| Option | Content of nixbox | Cost |
| --- | --- | --- |
| A. nixpkgs builds (rejected) | the tools' closures | 638 MiB, 987 with `fastfetch` |
| B. glibc runner (chosen) | glibc and libgcc, shims for the official binaries | about 57 MiB for the glibc runner (the bundle closure is 197.3 MiB on arm64, 180.1 MiB on amd64), official bytes stay unmodified |

## Intended use

Dev containers mount the classic toolbox and nixbox side by side. Later the toolbox
bundle is combined with a native Nix store in place of nixbox. Recovery containers
mount both. No temporary workaround is needed in between.

## First implementation (one PR, a commit series)

1. Bundle `bundles/nixbox` with a catalog (nixpkgs revision, cache URL and public
   key, one attribute per tool) and a lock (per architecture: top-level store paths
   and the closure with `narHash` and size from `nix path-info --json`).
2. Build in a Dockerfile stage from a digest-pinned `nixos/nix` image: `nix copy
   --from https://cache.nixos.org --to <image root>` with the pinned public key and
   signature checking on, then verify every closure path against the lock and fail on
   any mismatch. Nothing is compiled.
3. Image layout: `/nix/store`, `/nix/var/nix` (registrations), and a profile at
   `/nix/profile` with `bin/`, `share/`, `lib/ld.so` (symlink to the glibc loader)
   and the shims. The carrier image fills a volume that is mounted at `/nix`.
4. Shims in `/nix/profile/bin` for `fastfetch`, `ast-grep`, `delta`, `difft`, `leaf`,
   `zizmor`, `zk` and `hx`. A shim runs the toolbox binary directly when the host has a
   glibc loader and otherwise through `/nix/profile/lib/ld.so --library-path ...`. The
   `hx` shim also sets `HELIX_RUNTIME`. A shim without the toolbox mounted fails with a
   clear message. The toolbox itself is not changed.
5. A lock refresh script (runs in the pinned `nixos/nix` image) that resolves the
   catalog against the pinned nixpkgs revision. No updater bot change.
6. A smoke test that mounts the built volume on pinned Debian slim, Fedora, openSUSE
   Leap and Alpine images and runs the checks of the spikes, on arm64 and amd64.
7. Documentation `docs/nixbox.md`, cross references in `docs/toolbox.md` and the
   README bundle table, and the `AGENTS.md` rule for the class "distribution binary
   cache".
8. Tests in the existing patterns for the lock verification and the shims.

Out of scope: publication, tags, quay.io pushes, pins in consumer repositories, removal
of tools from the toolbox, changes to the `whr` recovery command.

## Rollout

1. Done: arm64 and amd64 spikes above.
2. The first implementation above, built locally.
3. Owner approval to publish, then pins in the dev container and the `whr` recovery
   command mount nixbox.
4. Remove the eight tools from the toolbox in a later toolbox release, after
   consumers mount nixbox. The catalog `provider` field records the placement.
5. Move further bundles per tool when their Nix flavor passes its smoke test.

## Rule change

`AGENTS.md` allows only official stable prebuilt releases and, since 2026-10-24,
declared static source builds. nixbox adds a class "distribution binary cache": a
closed list of store paths pinned by path and nixpkgs revision, verified against the
cache signature. This needs an owner decision.

## Open points

- Signature verification as its own tested step (the spikes relied on `nix copy`).
- The pinned nixpkgs revision and its update flow; versions lag upstream (`node`).
- Nix in CI.
- `leaf` has no nixpkgs attribute; it would use option B or stay classic.
- Licenses and redistribution of glibc and the nixpkgs closures. The lock records no
  licenses; recording them, with explicit owner approval, is a precondition of the
  first publication, not a build gate.
