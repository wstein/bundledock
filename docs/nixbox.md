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
| `/nix/profile/lib/ld.so` | Symlink to the glibc loader. |

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
The `fastfetch` binary leaves the toolbox only in a later toolbox release, after
consumers mount nixbox.

Choosing the glibc runner over nixpkgs builds of these tools keeps nixbox at
about 57 MiB instead of 638 MiB (987 MiB with nixpkgs `fastfetch`).

## Lock and verification

The catalog records the nixpkgs revision, the cache URL and public key, and one
attribute per tool. The lock holds, per architecture, the top-level store paths
and the closure with `narHash` and size from `nix path-info --json`.

The build runs in a Dockerfile stage from a digest-pinned `nixos/nix` image:

1. `nix copy --from https://cache.nixos.org --to <image root>` with the pinned
   public key and signature checking on.
2. Every closure path is verified against the lock, and any mismatch fails the
   build.

The lock refresh script runs in the same pinned image and resolves the catalog
against the pinned nixpkgs revision. The repository rule for this class of
source is in `AGENTS.md` ("distribution binary cache"); the first publication and
any change to the list of store paths need explicit owner approval. Verifying
the signature as its own tested step is still an open point: the spikes relied on
`nix copy`.

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
  redistribution terms of glibc and the closures, which the lock must record.
