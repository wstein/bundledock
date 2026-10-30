---
title: Nixbox bundle
description: Tools that need glibc or have no static prebuilt, plus the glibc release binaries, delivered in Nix store layout beside the toolbox.
weight: 21
---

# Nixbox bundle

[Back to BundleDock](../README.md) · [All bundles](bundles.md) · [Toolbox](toolbox.md)

Status: published to `quay.io/bundledock/nixbox` with tags `<date>-b<N>`, `stable` and
`latest`; see the record below and the [design](design/nixbox/spec.md). New versions
are published only when the owner selects `nixbox` in a manual dispatch of Publish
bundles (see [CI](#ci)). No consumer pins it yet.

The source now also ships the eight glibc-dynamic official binaries inside nixbox
(see [Tools](#tools)). That change is not published yet: the published nixbox
below still runs them from a mounted toolbox, and the published toolbox still
contains them. Neither new version is published.

<!-- BEGIN PUBLISHED BUNDLE -->
<!-- publication-record: {"attempt":"1","bundle":"nixbox","commit":"a4f4495ab6c03b887ef21c6368c5682fc51a6660","created":"2026-10-30T02:01:24Z","digest":"sha256:c20d3778c2843f66de3739a024c6bafa758afa252e54646321b0558639cdbc9f","image":"quay.io/bundledock/nixbox","lock_sha256":"d2ddff16742c269139281204d4674e0c520c46ca822624a197af0684e5cbefcf","reference":"quay.io/bundledock/nixbox@sha256:c20d3778c2843f66de3739a024c6bafa758afa252e54646321b0558639cdbc9f","run_attempt":"1","run_id":"40989024653","source":"https://github.com/wstein/bundledock","tag":"2026.10.30-b2"} -->
## Published bundle

Version: `2026.10.30-b2`

Index digest: `sha256:c20d3778c2843f66de3739a024c6bafa758afa252e54646321b0558639cdbc9f`

Source: [`a4f4495ab6c03b887ef21c6368c5682fc51a6660`](https://github.com/wstein/bundledock/commit/a4f4495ab6c03b887ef21c6368c5682fc51a6660)

Publication: [run 40989024653, attempt 1](https://github.com/wstein/bundledock/actions/runs/40989024653/attempts/1)

Pull this published carrier by its immutable index digest:

```sh
docker pull quay.io/bundledock/nixbox@sha256:c20d3778c2843f66de3739a024c6bafa758afa252e54646321b0558639cdbc9f
```

The source inventory below describes the current lock; this publication retains its exact source revision.
<!-- END PUBLISHED BUNDLE -->

## What it is

`nixbox` is a second bundle next to the [toolbox](toolbox.md). Its content comes
in Nix store layout, from the distribution binary cache `cache.nixos.org`.
Nothing is compiled. The eight glibc-dynamic official release binaries come from
their pinned releases, not from the cache.

Mount nixbox beside the toolbox when you need one of these:

- `git` without Perl, `rsync`, `zstd`, `less`, an SSH client and CA
  certificates, none of which has an official static Linux prebuilt for both
  architectures.
- The eight official binaries that are glibc-dynamic and do not start on musl
  images such as Alpine: `ast-grep`, `delta`, `difft`, `fastfetch`, `leaf`,
  `zizmor`, `zk` and `hx` (Helix, with its runtime). They left the toolbox. nixbox
  carries their unmodified official bytes.

## Placement rule

| Provider | Rule |
| --- | --- |
| `official` | An official release binary, shipped unmodified from its pinned release in `bundles/nixbox/tools`. Static ones stay in the toolbox; nixbox takes the glibc-dynamic ones. |
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
| Official binaries | `fastfetch`, `ast-grep`, `delta`, `difft`, `leaf`, `zizmor`, `zk` and `hx`, in `/nix/profile/libexec/nixbox`, reached through shims in `/nix/profile/bin`. |

### Official release binaries

<!-- BEGIN GENERATED TOOL INVENTORY -->
| Tool | Pinned version | Homepage | Source |
| --- | --- | --- | --- |
| ast-grep (`ast-grep`) | 0.51.0 | [Homepage](https://ast-grep.github.io/) | [Source](https://github.com/ast-grep/ast-grep) |
| delta (`delta`) | 0.20.1 | [Homepage](https://dandavison.github.io/delta/) | [Source](https://github.com/dandavison/delta) |
| Difftastic (`difft`) | 0.72.0 | [Homepage](https://github.com/Wilfred/difftastic) | [Source](https://github.com/Wilfred/difftastic) |
| Fastfetch (`fastfetch`) | 2.70.0 | [Homepage](https://github.com/fastfetch-cli/fastfetch) | [Source](https://github.com/fastfetch-cli/fastfetch) |
| Helix (`hx`) | 25.07.1 | [Homepage](https://helix-editor.com) | [Source](https://github.com/helix-editor/helix) |
| Leaf (`leaf`) | 1.29.2 | [Homepage](https://leaf.rivolink.mg) | [Source](https://github.com/RivoLink/leaf) |
| zizmor (`zizmor`) | 1.31.1 | [Homepage](https://github.com/zizmorcore/zizmor) | [Source](https://github.com/zizmorcore/zizmor) |
| zk (`zk`) | 0.15.7 | [Homepage](https://github.com/zk-org/zk) | [Source](https://github.com/zk-org/zk) |
<!-- END GENERATED TOOL INVENTORY -->

The official binaries are the toolbox builder's output for the pins in
`bundles/nixbox/tools` (`packages.txt`, `packages.catalog.json` and
`packages.lock.json` in the toolbox schema, with the license and `SOURCE` files in
`licenses/`). The image build downloads each release archive, compares its sha256
with that lock (a mismatch fails the build), unpacks it and installs the result
unmodified in `/nix/profile/libexec/nixbox` (`bin/<tool>`, `lib/helix/hx` with its
runtime and `licenses/`). Helix keeps its calendar tag spelling in the lock.

A shim runs its binary directly on a glibc host, and otherwise (a musl host such as
Alpine) through `/nix/profile/lib/ld.so --library-path ...`. The `hx` shim also sets
`HELIX_RUNTIME` to the shipped runtime. All shims are symlinks to one script that
reads a shim table generated from the catalog; no toolbox mount is needed.
`NIXBOX_LOADER` selects the mode: `auto` (the default: through the loader when a
musl loader such as `/lib/ld-musl-*.so.1` exists, because Alpine with gcompat also
has a glibc-looking loader file; direct on a host with a glibc loader), `always`
or `never`. For `git` over https set
`SSL_CERT_FILE=/nix/profile/etc/ssl/certs/ca-bundle.crt`. Put `/nix/profile/bin`
on `PATH` so the toolbox shell layer finds `hx` and `fastfetch`.

`zizmor` needs glibc 2.34 or newer on both architectures; on Alpine and other musl-based images the nixbox shim runs it through the Nix glibc loader. Run it as `zizmor --offline .github/workflows`; online audits use an optional `GH_TOKEN` or `GITHUB_TOKEN`. `leaf --update` writes its own executable and is unsuitable for the read-only volume; upgrade through the lock. Difftastic's ARM64 release is the official GNU/Linux build.

`nixbox check -tools bundles/nixbox/tools` verifies that every shim runs a file a
recipe installs and every recipe has a shim.

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
signature by a trusted key". The [CI](#ci) section lists the workflows nixbox has.

The lock refresh script runs in the same pinned image and resolves the catalog
against the pinned nixpkgs revision. It also fetches the nixpkgs source from
`releases.nixos.org`. The repository rule for this class of
source is in `AGENTS.md` ("distribution binary cache"); the first publication and
any change to the list of store paths need explicit owner approval. 

## CI

What exists:

- **Build and smoke** (`nixbox-build.yml`): manual `workflow_dispatch`, with the
  same approval guard as the toolbox build (`scripts/approve-build.sh`, the exact
  approved `main` SHA, no bots, no automatic trigger). It builds on native amd64
  and arm64 runners and runs `bundles/nixbox/scripts/smoke.sh`. It signs in nowhere, pushes nothing,
  uses no secrets and publishes nothing.
- **Upgrade proposals** (the `nixbox` option of `propose-upgrades.yml`, selected
  for `all` and the schedule, with `dry_run` defaulting to true): moves the
  catalog's nixpkgs `rev`, release `url` and `narHash` to the head of
  `nixpkgs-unstable` (`scripts/nixbox-bump-rev.py`, `bundles/nixbox/scripts/bump-rev.sh`),
  then refreshes the lock with `bundles/nixbox/scripts/refresh-lock.sh` in the
  pinned `nixos/nix` image, resolves `bundles/nixbox/tools` to the newest stable
  releases with the toolbox resolver, regenerates the inventory above and runs
  `nixbox check -tools`. If nothing moved, no pull request opens. A change becomes a separate
  nixbox-only pull request on an `automation/nixbox-upgrade-<run>-<attempt>`
  topic; its check is the dispatched source CI. The build and smoke run
  (`nixbox-build.yml`) is dispatched by the owner on `main`. Nothing is built
  or published by the updater.
- **Staging cleanup** (`staging-cleanup.yml`): nixbox is in the repository
  list. Before a first publication its Quay repository does not exist (Quay
  answers the robot with 404 or 401), and the run is an empty dry run; the
  exception ends once `docs/nixbox-published.json` records a publication. The
  owner creates `quay.io/bundledock/nixbox` and grants the robot write access
  before the first publication.
- **Publication** (the `nixbox` option of `publish-bundles.yml`): like every
  other bundle, through the shared `bundle-publish.yml`. The owner starts it by
  a manual dispatch with the exact approved `main` SHA, and selecting `nixbox`
  is the approval of that publication. It builds both architectures natively
  from the committed lock, runs the nixbox smoke test (`smoke.sh`), pushes the staging tags
  `staging-<date>-b<N>-<arch>`, and promotes the `<date>-b<N>` version tag with
  the `stable` and `latest` aliases at one verified index digest. A consumer
  pins that digest, and the volume is named `bundledock-nixbox-<digest>`.
  No updater pull request, schedule or CI completion publishes nixbox: its
  `automation/nixbox-upgrade-...` topic is never recognized by the publication
  planner, and a merged lock change waits for the next dispatch. A change to the
  list of store paths still needs the owner's explicit approval, which is that
  dispatch. `published-consumers.yml` verifies only published pins, so nixbox
  joins it once a digest exists.

## Licenses

`bundles/nixbox/licenses.json` records the nixpkgs `meta.license` of every
closure path of the lock, on both architectures. It is generated by
`bundles/nixbox/scripts/record-licenses.sh` in the pinned `nixos/nix` image from
the pinned nixpkgs revision. Each path is matched to the nixpkgs attribute whose
output path equals the store path, so no license is inferred from a name. The
glibc runner is `glibc` (`LGPL-2.0-or-later`) with `gcc-lib` and `gcc-libgcc`
from `stdenv.cc.cc` (`GPL-3.0-or-later`; nixpkgs `meta` does not carry the GCC
Runtime Library Exception). All other closure paths are recorded the same way,
for example `git` (`GPL-2.0-only`), `openssh` (`BSD-2-Clause`), `rsync`
(`GPL-3.0-or-later`), `zstd` (`BSD-3-Clause`), `less` (`GPL-3.0-or-later`) and
`cacert` (`MPL-2.0`).

Not determined: the license of `xgcc-16.2.0-libgcc` (both architectures),
`gmp-with-cxx-6.3.0` (both) and `util-linux-minimal-2.42.4-login` (arm64). No
nixpkgs attribute with the same output path was found for them, so the record
holds `null` for them rather than a guess. The record is not a build or
publication gate.

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
shims (historical: the source no longer mounts a toolbox) (pinned Debian trixie-slim, Fedora 44, openSUSE Leap 16.0 and Alpine 3.23, as
root, plus one uid 1000 run on Debian):

| Platform | Debian | Fedora | openSUSE | Alpine | Debian, uid 1000 |
| --- | --- | --- | --- | --- | --- |
| arm64 | 32 of 32 | 32 of 32 | 32 of 32 | 33 of 33 | 32 of 32 |
| amd64 (Rosetta) | 32 of 32 | 32 of 32 | 32 of 32 | 33 of 33 | 32 of 32 |

After the review fixes, the arm64 smoke test (with stub glibc loader files on
Alpine that simulate gcompat, plus added auto-mode and symlink checks) passed 34 of 34 on Debian, Fedora, openSUSE and Debian uid 1000, and 36 of 36
on Alpine. It was not rerun on amd64. The amd64 loader stub path and the
symlink-with-another-name check were added after that run, so the totals are
not verified for the current script; rerun the smoke test to refresh them.

The lock closure is 52 store paths and 197.3 MiB on arm64, and 51 paths and
180.1 MiB on amd64. The initialized volume is 221 MB on arm64 and 203 MB on amd64.
The local image is 308 MB on arm64 and 291 MB on amd64. The image is not
byte-reproducible because the Nix database holds registration times. The smoke
test reads `NIXBOX_PLATFORM` and `NIXBOX_OFFLINE`; since the move it mounts no
toolbox.

## Combining with a native Nix store

Dev containers mount the toolbox and nixbox side by side (nixbox for the eight glibc binaries and the distribution tools). Later the toolbox is
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
- `leaf` has no nixpkgs attribute; like the other seven it is an official binary.
- Unfree packages are not available from the cache (`claude-code`,
  `antigravity`), and `code-server` is not cached, so those bundles stay as they
  are.
- Open points: Nix in CI, and the redistribution terms of glibc and the closures
  beyond the license names in [Licenses](#licenses), including the three paths
  whose license is not determined there.
