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
  (`/layers` in the toolbox, `/out` and `/init-root` in nixbox). It needs no
  BuildKit exporter option, but the Dockerfiles themselves need BuildKit
  (`# syntax=` and `RUN --network=none`).
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
- The nixbox build creates `var/nix/db/db.sqlite` with `nix copy --to` in the
  fetch stage. `tools-bundle normalize-nix-db -db <db.sqlite>` then sets
  `registrationTime` of every `ValidPaths` row to the epoch, the database
  header's file change counter and version-valid-for number to 1, and the
  unallocated gap of every `ValidPaths` table page to zero, before
  `normalize-mtimes`. The gap lies between the cell pointer array and the cell
  content; with `secure_delete` off SQLite leaves the old bytes of moved rows
  there (a root split already does). It is an assumption, not checked, that the
  SQLite inside the pinned `nixos/nix` image builds with `SQLITE_SECURE_DELETE`
  (nixpkgs does), in which case the gaps are already zero. The patch is in place
  in the file bytes (no SQLite library, no new dependency): Nix writes the build
  time as a 4-byte integer, which keeps its width, so no page or offset moves.
  The file is replaced by rename of a synced temporary file. The command refuses,
  and leaves the file unchanged, when a non-empty `-wal` or `-journal` file
  exists, the freelist is not empty, the header page count differs from the file
  size, a `ValidPaths` page has free blocks or fragments (a history of deletes
  and updates), the time lies on an overflow page, or the epoch does not fit the
  field width. The tests use fixtures built from NixOS/nix `schema.sql` (commit
  5286c0477d77, with its SQL comments); whether the schema in the pinned image
  matches is UNKNOWN.
  A test (`TestRealNixboxDockerfileNormalizesTheNixDatabase`) requires the step
  in the Dockerfile text, after every other mention of the database directory or
  of a parent of it up to the copied root, and before the `normalize-mtimes` step
  that covers it. It cannot see variables, globs or relative paths.
- File content comes from checksum-pinned downloads or from the pinned Go
  sources compiled with `-trimpath`.

## Remaining non-determinism

| Item | Cause | Effect |
| --- | --- | --- |
| nixbox `db.sqlite`: row ids and page layout | `nix copy` assigns `ValidPaths.id` in registration order and lays pages out in insertion order; `normalize-nix-db` does not reorder or rebuild. | The nixbox `/opt` layer differs when two builds register the paths in a different order. UNKNOWN until two builds are compared. |
| nixbox `db.sqlite`: gaps of other pages | Only `ValidPaths` pages are cleared; stale bytes in the gaps of index, `Refs` or overflow pages are not looked at, and the unused tail of a reused overflow page is not cleared. | These bytes do not hold registration times unless a row moved there; UNKNOWN until two builds are compared. |
| nixbox `db.sqlite`: SQLite version number | Header bytes 96-99 hold the version of the SQLite that last wrote the file. | Fixed by the pinned `nixos/nix` image digest; changes when that image changes. |
| nixbox `var/nix` other than the database | Files such as `temproots/<pid>` or lock files, if Nix leaves any. | UNKNOWN; not inspected without a build. |
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
