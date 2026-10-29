---
title: Nixbox bundle
description: Tools that need glibc or have no static prebuilt, delivered in Nix store layout beside the toolbox.
weight: 21
---

# Nixbox bundle

[Back to BundleDock](../README.md) · [All bundles](bundles.md) · [Toolbox](toolbox.md)

Status: designed and decided, see the [design](design/nixbox/spec.md). The first
implementation is a local build only. Nothing is published: there is no
`quay.io/bundledock/nixbox` image, no tag, and no consumer pins it yet. This
guide describes the intended content and will gain a publication record when the
owner approves a first publication.

## What it is

`nixbox` is a second bundle next to the [toolbox](toolbox.md). Its content comes
in Nix store layout, from the distribution binary cache `cache.nixos.org`.
Nothing is compiled. The toolbox is unchanged.

Mount nixbox together with the toolbox when you need one of these:

- `git` without Perl, `rsync`, `zstd`, `less`, an SSH client and CA
  certificates, none of which has an official static Linux prebuilt for both
  architectures.
- The eight toolbox binaries that are glibc-dynamic and do not start on musl
  images such as Alpine: `ast-grep`, `delta`, `difft`, `fastfetch`, `leaf`,
  `zizmor`, `zk` and the `hx` launcher's `lib/helix/hx`.

## Placement rule

| Provider | Rule |
| --- | --- |
| `official` | An official static prebuilt exists for amd64 and arm64. It stays in the toolbox. |
| `nix` | The tool needs glibc libraries or has no official prebuilt. nixbox delivers it. |

Nix-sourced store paths are limited to the glibc and libgcc runner and to tools
that have no official stable prebuilt for both architectures. A tool with
official prebuilts for both architectures (for example `node` or `opentofu`) stays
on the official route and never enters nixbox.

The toolbox is static and standalone. nixbox is for glibc needs and for tools
sourced from a distribution. BundleDock's own tools (`whr`, `stw`, `smill`,
`cscribe`, `herdr`) stay in the toolbox. The catalog `provider` field records
the placement.

## Layout

The carrier image fills a volume `bundledock-nixbox-<digest>`, which you mount
read-only at `/nix`:

| Path | Content |
| --- | --- |
| `/nix/store/<hash>-<name>` | Store paths and their closures. |
| `/nix/var/nix` | Nix database registrations. |
| `/nix/profile/bin`, `/nix/profile/share` | Symlinks to the tools, and the shims. |
| `/nix/profile/lib/ld.so` | Symlink to the glibc loader (`ld-linux-aarch64.so.1` on arm64, `ld-linux-x86-64.so.2` on amd64). |

The initializer runs as root when it fills a volume. The Nix database files
`big-lock` and `reserved` keep their mode 0600, so a non-root initialization is
not supported. Consumers mount the volume read-only and can run as any uid.

Add `/nix/profile/bin` to `PATH`. The classic `/opt/bdock/<name>` mounts stay as
they are. Because this is a real Nix store, a native store can later replace the
volume without changing paths.

## Tools

| Group | Content |
| --- | --- |
| Distribution tools | `git` without Perl (`git-minimal`), `rsync`, `zstd`, `less`, `ssh` (OpenSSH) and the CA certificate bundle. |
| glibc runner | glibc and libgcc, reached through `/nix/profile/lib/ld.so`. |
| Shims | `fastfetch`, `ast-grep`, `delta`, `difft`, `leaf`, `zizmor`, `zk` and `hx`. |

The shims keep the official binaries from the toolbox unmodified. A shim runs the
toolbox binary directly when the host has a glibc loader, and otherwise through
`/nix/profile/lib/ld.so --library-path ...`. The `hx` shim also sets
`HELIX_RUNTIME`. A shim without the toolbox mounted fails with a clear message.
All shims are symlinks to one script that reads a shim table generated from the
catalog. `BDOCK_TOOLBOX` names the toolbox directory (default `/opt/bdock/toolbox`).
`NIXBOX_LOADER` selects the mode: `auto` (the default: direct on a glibc host,
through the loader otherwise), `always` or `never`. For `git` over https set
`SSL_CERT_FILE=/nix/profile/etc/ssl/certs/ca-bundle.crt`.
The `fastfetch` binary leaves the toolbox only in a later toolbox release, after
consumers mount nixbox.

Choosing the glibc runner over nixpkgs builds of these tools means the eight tools
add only the glibc runner, about 57 MiB, instead of 638 MiB of nixpkgs closures
(987 MiB with nixpkgs `fastfetch`). The 57 MiB is the runner alone. The whole
bundle closure, including `git`, `rsync`, `zstd`, `less` and `openssh`, is 197.3 MiB
on arm64 and 180.1 MiB on amd64 (see "Local build of the bundle").

## Lock and verification

The catalog records the nixpkgs revision, the cache URL and public key, and one
attribute per tool. The lock holds, per architecture, the top-level store paths
and the closure with `narHash` and size from `nix path-info --json`.

The build runs in a Dockerfile stage from a digest-pinned `nixos/nix` image:

1. `nix copy --from https://cache.nixos.org --to <image root>` with the pinned
   public key and signature checking on.
2. Every closure path is verified against the lock, and any mismatch fails the
   build.

Signatures are checked three ways: an ed25519 check of every closure path against
the pinned key, `nix copy` with `require-sigs`, and the NAR-hash comparison of the
copied store with the lock. A wrong key makes `nix copy` fail with "lacks a
signature by a trusted key". The nixbox bundle is excluded from the updater and
publication machinery.

The lock refresh script runs in the same pinned image and resolves the catalog
against the pinned nixpkgs revision. It also fetches the nixpkgs source from
`releases.nixos.org`. The repository rule for this class of
source is in `AGENTS.md` ("distribution binary cache"); the first publication and
any change to the list of store paths need explicit owner approval. 

## Evidence

The spikes of 2026-10-29 took packages from `cache.nixos.org` (nothing built)
into a scratch volume mounted read-only at `/nix/store`, on unmodified images
with a read-only root file system and a tmpfs `/tmp`.

| Platform | Images | Checks |
| --- | --- | --- |
| arm64 | Debian slim, Fedora, openSUSE Leap, Alpine | 64 of 64 (root) |
| amd64 (Rosetta) | Debian slim, Fedora, openSUSE Leap, Alpine | 64 of 64 (root) |
| arm64, uid 1000 | Debian slim | 13 of 16 |

The checks run real operations: `git` commit, `fsck`, bundle, stash, submodule
and `ls-remote` over https; `rsync` copy with progress; `zstd` round trip;
`less`; `ssh` config parsing; and the eight formerly failing tools. The three
non-root failures are test artifacts: an unreadable `/etc/.pwd.lock` as `rsync`
source and OpenSSH's "No user exists for uid 1000" for a uid missing from
`/etc/passwd`. No Perl, Python, Ruby or Tcl appears in the closure of any tested
package.

Closure sizes on arm64 for the nixpkgs-built alternative that was rejected:

| Set | Size (MiB) |
| --- | --- |
| `git-minimal`, `rsync`, `zstd`, `less`, `openssh`, CA bundle | 197 |
| plus `zk`, `zizmor`, `helix`, `delta`, `difftastic`, `ast-grep` | 638 |
| plus `fastfetch` (442 alone, it links ImageMagick) | 987 |

An unmodified official binary runs on Alpine through the Nix glibc loader. All
eight binaries passed on arm64. The needed files are about 3.5 MiB, or about
57 MiB as whole store paths. Not verified: a trimmed copy, the amd64 equivalent,
and the cause of gcompat's segfault.

### Local build of the bundle

The first implementation was built locally from the lock and run through
`bundles/nixbox/scripts/smoke.sh`, with the published toolbox volume mounted for the
shims (pinned Debian trixie-slim, Fedora 44, openSUSE Leap 16.0 and Alpine 3.23, as
root, plus one uid 1000 run on Debian):

| Platform | Debian | Fedora | openSUSE | Alpine | Debian, uid 1000 |
| --- | --- | --- | --- | --- | --- |
| arm64 | 32 of 32 | 32 of 32 | 32 of 32 | 33 of 33 | 32 of 32 |
| amd64 (Rosetta) | 32 of 32 | 32 of 32 | 32 of 32 | 33 of 33 | 32 of 32 |

The lock closure is 52 store paths and 197.3 MiB on arm64, and 51 paths and
180.1 MiB on amd64. The initialized volume is 221 MB on arm64 and 203 MB on amd64.
The local image is 308 MB on arm64 and 291 MB on amd64. The image is not
byte-reproducible because the Nix database holds registration times. The smoke
test reads `NIXBOX_PLATFORM`, `NIXBOX_TOOLBOX_IMAGE` and `NIXBOX_OFFLINE`.

## Combining with a native Nix store

Dev containers mount the toolbox and nixbox side by side. Later the toolbox is
combined with a native Nix store in place of nixbox. On a Linux host a native
store works. On macOS a host store is in Darwin format and unusable in Linux
containers; there it must be a Linux store in a Docker volume or Linux VM. The
layout is identical.

## Known limits

- The consumer image needs a `/nix` mount point, and only one volume can be
  mounted at `/nix`. nixbox is therefore the profile volume that later
  Nix-delivered bundles join; it is not a fixed tool list.
- nixpkgs versions lag upstream. The spike found `node` 24.21.0 in nixpkgs
  against a pinned 24.22.2.
- OpenSSH fails with "No user exists for uid ..." when the container uid has no
  `/etc/passwd` entry.
- `leaf` has no nixpkgs attribute, so it relies on the shim approach.
- Unfree packages are not available from the cache (`claude-code`,
  `antigravity`), and `code-server` is not cached, so those bundles stay as they
  are.
- Open points: nixpkgs revision update flow, Nix in CI, and the licenses and
  redistribution terms of glibc and the closures. The lock does not record
  licenses. Recording them, with the owner's approval, is an open precondition of
  the first publication (`AGENTS.md`, condition (e)); it is not a build gate, and
  the build and the smoke test do not check it.
