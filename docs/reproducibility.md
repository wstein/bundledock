---
title: Image layer determinism
description: How toolbox and nixbox layers get fixed timestamps, how to compare two builds, and what stays non-deterministic.
weight: 61
---

# Image layer determinism

[Back to BundleDock](../README.md)

Identical build inputs must give identical layer digests, so that versions share
layers in a registry and on a host. A layer is a tar archive; its digest depends
on every file's path, content, mode, owner and modification time.

## What is fixed

- `SOURCE_DATE_EPOCH` is the constant `1` (`toolpublish.SourceDateEpoch`). It is
  not derived from the version or the build date: a per-version time would give
  every version different layers, including for unchanged tools. `1` is also the
  canonical mtime of a Nix store path.
- The publisher passes `--build-arg SOURCE_DATE_EPOCH=1` to `docker build` for
  all 10 bundles (`publish.go`), including those whose Dockerfiles do not use it. The toolbox and nixbox Dockerfiles declare `ARG SOURCE_DATE_EPOCH=1`, so
  `make bundle-build` gives the same result.
- Before the final stage, `tools-bundle normalize-mtimes -root <dir>` sets the
  mtime of the directory and of every entry below it (symlinks themselves, not
  their targets) to the epoch. This runs in the build stage on the staged roots
  (`/image-root` and `/init-root` in the toolbox, `/out` and `/init-root` in
  nixbox). It needs no BuildKit exporter option, but the Dockerfiles themselves
  need BuildKit (`# syntax=` and `RUN --network=none`).
- The final stage copies only whole normalized roots (`COPY --from=… /root/ /`).
  A test (`TestRealDockerfilesNormalizeAfterTheLastWrite`) reads the Dockerfile
  text and fails when a copied root has no `normalize-mtimes -root <root>` call
  that runs as a command (at the start of a `RUN` or after `&&` or `;`, followed
  by `&&`, `;` or the end of the instruction; not `echo`, not after a `#`, not
  followed by `|| true`), or when a later part of the stage mentions that root
  again. A copied root is covered by a call for the same path or a parent path.
  The test cannot see relative paths, `$VAR` or `ARG` expansion, globs, writes
  through `tar -C /`, conditionals or subshells; it does not look at the built
  image, and it promises nothing beyond these checks. The design avoids implicit
  parent directories such as `/usr/local/bin` that a `COPY` may create with the
  build time; whether it does is UNKNOWN until a build.
- File content comes from checksum-pinned downloads or from the pinned Go
  sources compiled with `-trimpath`.

## Remaining non-determinism

| Item | Cause | Effect |
| --- | --- | --- |
| nixbox `var/nix/db/db.sqlite` | The Nix database stores a registration time per store path. | The nixbox `/opt` layer differs between builds. Not normalized. |
| Image config and image ID | `created` and history times of the image config. | Config digest and image ID may differ; layer digests do not. The effect of the build argument on `created` is UNKNOWN. |
| Parent directories created by `COPY` | Not fixed by Dockerfile text alone. | UNKNOWN until two builds are compared; the staged-root design avoids them. |
| Go compiler output | The init binary is rebuilt in every build. | Byte-identical for the same source and `-trimpath`; unverified until two builds are compared. |

## Compare two builds

Each build needs the owner's approval (AGENTS.md). With two local images `A` and
`B` built from the same commit:

```sh
for i in A B; do docker image inspect --format '{{json .RootFS.Layers}}' "$i" | jq -r '.[]'; done | sort | uniq -c
```

Every layer digest must appear with a count of 2. On a containerd image store,
`docker image inspect` reports the same diff IDs. For pushed images compare
`regctl manifest get <ref> --format '{{jsonPretty .Layers}}'` or the layer digests
in the registry manifest. Record the remaining differing layers and their cause
in the table above.
