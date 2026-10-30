# Design: a generic way to add tools the catalog cannot express

**Status**: owner decisions recorded 2026-10-24 (see [Decisions](#decisions)). Implemented so far: WP1 (rule), WP3/WP3b
(schema), WP4b (publish-gate content check), and WP4 as four stacked changes (lock URL binding and tool digest, product-data
resolver and updater scope, ELF checker and extractor, fetch/compile subcommands and stage layout); see the
"implementation choices" paragraphs below. WP5 is authored as two stacked Draft changes (notices and smoke script; catalog entry, lock, production
Dockerfile stage and inventory) and awaits review and an approved build. Open: WP6 (guides). No image has been built; the WP2 local compile spike (outside the repository, outside
BuildKit) is recorded in [spike-results.md](spike-results.md). Every statement is marked verified (with how) or unverified.

## Context and trigger

On 2026-10-24 the owner asked to add `sqlite3` to the toolbox, asked whether Alpine binaries could be
used, and then asked for a **generic** solution for builds in BundleDock. SQLite had been added on
2026-10-10 by `6aabe3a` (amalgamation ZIP verified, compiled in a Debian `gcc` stage against glibc)
and excluded the same day by `9554982` at the owner's request; `TestSQLiteRemovedFromBundleInputs`
(`internal/toolconfig/shell_inventory_test.go`) enforces the exclusion. The owner has now lifted it
(decision D1).

## Validation of the foreman analysis

| # | Claim | Result | Evidence |
| --- | --- | --- | --- |
| 1a | sqlite.org publishes no linux-arm64 precompiled CLI | **Confirmed** | `https://www.sqlite.org/download.html` fetched 2026-10-24 (SQLite 3.54.1): the product list has `sqlite-tools-linux-x64`, `osx-arm64`, `osx-x64`, `win-*`, `android`, `wasm`; no linux-arm64. The independent review reached the same result |
| 1b | It is not a GitHub release, so the catalog cannot express it | **Confirmed** | `Package.Provider` accepts only `node`, `github`, `claude`, `antigravity` (`internal/toolconfig/config.go`, `validate`); each resolves versions from a release API or fixed metadata. A linux-x64-only asset would also break the "both architectures, same version" rule |
| 2a | The Alpine `sqlite` apk is dynamically linked against musl and will not run on the Debian consumer | **Unverified, plausible** | No apk was inspected. `docs/architecture.md`: "Runtime compatibility with Alpine or arbitrary consumer images is not established". The design does not use Alpine packages for the tool |
| 2b | A static musl build runs on glibc | **Verified on 2026-10-24 for aarch64 only** (native, Debian 13, glibc 2.41, not the consumer image). The x86_64 binary was verified by inspection (ELF headers) and by execution under Rosetta emulation only; native x86_64 execution is not measured | The spike built and ran it ([spike-results.md](spike-results.md)). Earlier indirect evidence: mise ships as a musl raw binary with "no ELF interpreter" (`bundles/toolbox/licenses/mise/SOURCE`). The spike ran the aarch64 binary on this host and recorded the ELF headers of both (see spike-results.md); the first approved smoke on the consumer image is still needed |
| 3 | mise is shipped, optional, an installer/resolver and not a C build engine | **Confirmed** | Catalog `mise`: provider `github`, raw musl binary, v2026.10.18, `publisher-sha256`. mise would not provide SQLite for linux-arm64 either (backends not researched; unverified) |
| 4 | Two separable concerns | **Confirmed**; the acquisition side is deferred (D4) | |

Additional verified findings (2026-10-24 unless noted):

- **The current rule is broader than "tools with prebuilts".** `AGENTS.md` states: "Third-party tools
  must use official stable prebuilt releases. Never build third-party tools from source or silently
  patch any tool binary or embedded dependency. If a security fix is not yet available in an official
  stable prebuilt release, wait for that release and record the remaining findings in existing
  documentation. This does not restrict compiling BundleDock's own CLI, builder, or initializer."
  It forbids every third-party source build today.
- **Name clash.** The catalog `source` block (`internal/toolconfig/source.go`) describes licence
  source offers, not builds. The new block is called `build`.
- **SQLite publishes SHA3-256**, in machine-readable lines `PRODUCT,VERSION,RELATIVE-URL,SIZE-IN-BYTES,SHA3-HASH`
  (for 3.54.1: `2026/sqlite-amalgamation-3540100.zip`, 2,990,966 bytes). Only the current version is
  listed. Review confirmed the downloaded bytes match the hash.
- **The amalgamation ZIP contains five entries** under `sqlite-amalgamation-3540100/`: `sqlite3.c`,
  `shell.c`, `sqlite3.h`, `sqlite3ext.h` (plus the directory). `shell.c` supports `HAVE_LINENOISE` but
  **no `linenoise.c`/`linenoise.h` is in the amalgamation ZIP, the `sqlite-src` ZIP or the autoconf
  tarball** (listings checked 2026-10-24). See D3 and D8.
- **Which linenoise `shell.c` supports** (read from the 3.54.1 `shell.c`): `-DHAVE_LINENOISE=1` selects the
  one-argument completion callback of the "antirez" version, `-DHAVE_LINENOISE=2` the two-argument callback
  of the "msteveb" version (`linenoiseSetCompletionCallback(cb, NULL)`); the shell includes `"linenoise.h"`
  and uses `linenoise`, `linenoiseHistoryAdd/Load/Save/SetMaxLen` and `linenoiseAddCompletion`. The
  `antirez/linenoise` repository (BSD-2-Clause per the GitHub licence field; default branch `master`,
  last push 2026-05-02) at commit `a473823d74b93eab2ba83480df16ed37617493f2` contains `linenoise.c`,
  `linenoise.h`, `LICENSE`, `README.markdown`, `Makefile`, `example.c` and `linenoise-test.c`; its header
  declares `linenoiseCompletionCallback(const char *, linenoiseCompletions *)`, matching `HAVE_LINENOISE=1`,
  and `linenoise.c` includes only libc headers and `termios.h`/`sys/ioctl.h` (no terminfo or ncurses; it
  mentions `TERM` only to refuse the terminals `dumb`, `cons25` and `emacs`). `msteveb/linenoise` has
  no GitHub licence field (not used). That the file compiles under `zig cc` static musl and links with
  `shell.c` is **verified on 2026-10-24** ([spike-results.md](spike-results.md)).
- **The `ziglang.org/download/index.json` lists 0.17.0** (2026-10-01) with `tarball`, `shasum` and
  `size` for `x86_64-linux` and `aarch64-linux`. The aarch64 zig binary is static (checked with `readelf`: no `PT_INTERP`, no `DT_NEEDED`) and `-static` output is non-PIE `ET_EXEC` on both architectures (verified on 2026-10-24, [spike-results.md](spike-results.md)); the x86_64 zig archive was not downloaded and the Go verifier does not exist yet.
- **The Alpine package index keeps one gcc and one musl-dev** (review, 2026-10-24: `gcc` 15.2.0-r2,
  `musl-dev` 1.2.5-r24 for v3.23/main/aarch64), so exact apk pins would break on routine point releases.
- **Publication builds natively**: `bundle-publish.yml` has `native` jobs on `ubuntu-24.04` and
  `ubuntu-24.04-arm`, using `docker build --pull --platform linux/<arch>` (review-verified).
- **Go `crypto/sha3`** exists in the pinned Go 1.27.2 (review-verified; `go.mod` minimum 1.25).

## Goals

1. Add `sqlite3` to the toolbox for amd64 and arm64 as the first user.
2. One reviewed, declarative, closed-vocabulary way to compile a tool that has no official prebuilt for
   both architectures, with a pinned toolchain, publisher-hashed sources and a proof of static linking.
3. Existing bundles, locks and tools stay byte-identical when the feature lands.
4. Small, single-purpose PRs, the rule change first.

## Non-goals

- No change to existing tools, versions or install paths; tools with official prebuilts for both
  architectures keep using them (a source build for those stays forbidden).
- No build systems (make, cmake, cargo, autotools), no dependency resolution, no free-form shell in
  the catalog, no patching of sources.
- No mise provider now (D4). No runtime download, no network at consumer runtime, no compiler or
  Alpine runtime in the final image.
- No build, release, tag, publication or consumer activation by this document or its PRs.

## Invariants (unchanged by this design)

- Digest-pinned build-stage images; final image `scratch` with the bundle and static initializer only.
  The Dockerfile frontend (`# syntax=docker/dockerfile:1`) floats today and is not digest-pinned;
  this design does not change that and the implementation notes it where it relies on a frontend feature.
- Mount prefix `/opt/bdock/toolbox`, the `/opt/bundledock` link, `bundledock-toolbox-<digest>` volumes,
  user configuration filenames (`packages.txt`, `packages.catalog.json`, `packages.lock.json`).
- All existing tools, versions, recipes and locks.
- Every download verified by the static Go builder before use, against publisher data (or, for the named
  commit-pinned exception of D1(c)/D8, the commit-pinned builder-computed SHA256); nothing is
  executed before verification; no network at consumer runtime.
- Agents do not build, publish, tag or activate; the lock-only updater never dispatches native builds.

## Decisions

Recorded from the owner on 2026-10-24, with the reasoning kept.

- **D1 Rule change (was OQ-1).** The SQLite exclusion is lifted. `AGENTS.md` "never build third-party
  tools from source" is amended **generally, with conditions**: a reviewed, recipe-driven, static
  source build is allowed only when (a) no official prebuilt exists for both amd64 and arm64, (b) the
  toolchain is pinned by content (D6), (c) the source checksum comes from publisher data; where the publisher offers none (a pinned commit of a source repository), the source is pinned by commit SHA plus the builder-computed SHA256 of the fetched files, class `tls-download-sha256`, recorded as a named exception with its residual risk, and every bump of such a pin needs owner approval, and (d) a
  test proves static linking. Every new case needs its own review. *Reasoning:* the catalog cannot
  express SQLite and the owner wants a generic mechanism rather than an exception.
- **D2 Updater scope (was OQ-2).** The bot-updater exception covers a source-build lock **only when
  just the source version and checksum rise**, with the recipe and the toolchain unchanged. Any recipe
  or toolchain change and the **first publication** of a source-build tool need explicit owner
  approval. See [Update flow](#update-flow) for the consequence of `AGENTS.md` and what the PR shows.
- **D3 SQLite options (was OQ-3).** "Practical" build: `SQLITE_OMIT_LOAD_EXTENSION`, FTS5, RTREE,
  DBSTAT_VTAB and MATH_FUNCTIONS enabled, no `sqldiff`, no `sqlite3_analyzer`. Line editing: see D8
  (the amalgamation does not contain linenoise).
- **D4 mise provider deferred (was OQ-4).** No mise provider, no schema for it beyond the reserved
  provider name `mise`. See [Later](#later-acquisition-providers).
- **D5 Attestations recorded (was OQ-5).** If a publisher offers attestations they are recorded in the
  lock (kind, subject digest, signer identity) and **not verified by the builder**. *Residual risk:* an
  attestation in the lock proves nothing by itself; a compromised publisher that also controls its
  attestation passes the hash check. The hash from publisher data is the only enforced control.
  Applies to later acquisition providers; SQLite publishes none that I found (unverified).
- **D6 Compiler (was OQ-6).** The compiler is **one pinned `zig cc` archive**, fetched by the verifying
  Go fetcher with the publisher checksum from `ziglang.org/download/index.json` and pinned in the lock.
  It replaces Alpine apk pins, which cannot last (the index keeps only current versions). A **spike**
  precedes the recipe PR.
- **D7 Hash class (was OQ-7).** `publisher-sha3-256` is its own class in `ValidateSpec`; the lock also
  keeps the builder's SHA256 of the same bytes.
- **D8 Line editing (was OQ-A, 2026-10-24).** The sqlite3 recipe has a **second pinned source**: linenoise
  (BSD-2-Clause), not readline or libedit. Details in [Second source: linenoise](#second-source-linenoise). This is the owner-accepted instance of the named exception in D1(c): linenoise has no publisher checksum, so
  it is pinned by commit SHA and the builder-computed SHA256, class `tls-download-sha256`, with the
  residual risk in that section; every bump needs owner approval.

## Source-build design

### Stages

```text
downloads (BUILDPLATFORM, network)       tools-bundle fetches + verifies the source archive(s), the linenoise files and the zig archive
compile-<tool> (TARGETPLATFORM, RUN --network=none)  runs zig cc on the verified inputs
builder (FROM downloads, existing)       tools-bundle verifies the output is static, writes manifest
```

`compile` uses the already pinned `alpine:3.23@sha256:...` digest only as an execution root file
system: nothing is installed with `apk`. `tools-bundle` runs in `builder` (`FROM downloads`), not in
`downloads`; the real stage names are `compiler`, `downloads`, `builder` and the final `scratch`.

- **Network.** The only network step is the Go fetcher. `RUN --network=none` is documented as stable
  since Dockerfile 1.3 and needing no entitlement (Dockerfile reference), **from documentation; not run
  in this repository**. The spike must run it on the real frontend before the recipe PR relies on it.
- **Cross-architecture.** Native stage per target architecture, matching the native publication jobs.
  `zig` for the host architecture is fetched (`x86_64-linux` or `aarch64-linux`) and targets the same
  architecture (`x86_64-linux-musl`, `aarch64-linux-musl`). Zig can cross-compile, so a single host
  could build both, but publication already runs natively and native output is what the smoke tests.
  Emulation is for local development only and never for published output.
- **Reproducibility.** Measured on 2026-10-24 ([spike-results.md](spike-results.md)): four variants per architecture (and a fifth, warm-cache build for aarch64) were byte-identical across build paths, cache directories, `SOURCE_DATE_EPOCH` values and network namespace, on one host with one zig; other hosts and `-g` are not measured. Fixed flags, no timestamps (`SOURCE_DATE_EPOCH` from the recipe),
  `-ffile-prefix-map`, sorted inputs.
- **Build time and cache.** About 7 to 9 seconds wall time per target on 10 cores, cold or warm zig cache, outside BuildKit (verified on 2026-10-24, [spike-results.md](spike-results.md)); BuildKit time and cache are not measured. The compile stage depends only on source hash, zig pin and
  recipe, so BuildKit caching is keyed on those.

### Recipe format

Closed vocabulary in `packages.catalog.json`, covered by `catalog_digest`; no free-form shell. One kind,
`c-amalgamation`, with `sources` (archive members), `defines` and `cflags` and `libs` from Go
allow-lists, `static: true` (mandatory), `output`. SQLite example below. Later kinds need their own design.

### Static linking and the verifier (F4)

The recipe passes `-static` and pins the output form after the spike: **either** static non-PIE
(`e_type == ET_EXEC`) **or** static-PIE (`ET_DYN`); the spike records which zig produces for
`x86_64-linux-musl` and `aarch64-linux-musl` and the recipe states it. The verifier is written to accept
exactly the form the recipe declares (`output_form: "exec" | "static-pie"`), so a change of the compiler's
default is caught rather than silently accepted. A plain "no PT_DYNAMIC" rule is wrong for static-pie and
is not used.

`tools-bundle` runs a pure-Go ELF checker (`debug/elf`, no C compiler) on every `source-build` output
before the manifest is written. It checks: ELFCLASS64, little endian, `e_machine` equals the target
(`EM_X86_64` or `EM_AARCH64`), `e_type` equals the declared form, **no `PT_INTERP`**, **no `DT_NEEDED`**,
no `DT_RPATH`/`DT_RUNPATH`, `PT_GNU_STACK` present and non-executable, and for `exec` no `PT_DYNAMIC`.

What it **proves**: the file is a self-contained ELF of the right architecture that needs no dynamic
loader or shared library. What it **does not prove**: that the program runs correctly on the glibc
consumer (kernel interface, locale, name services), that no dependency was statically embedded with an
unwanted licence, or that the build is reproducible. Only the consumer smoke and the notices review do.
Behavioural note: in the 3.54.1 `shell.c`, `find_home_dir` calls `getpwuid(getuid())` first and falls back to `$HOME`; the result locates the home directory for `~` handling, the rc file and the history file (callers at lines 31565, 38707, 38747 and 39700, not traced individually). There is no `~user` expansion. Static musl resolves from local files only (no glibc NSS modules such as LDAP; the musl implementation was not read, unverified), so a user known only through NSS may get its home from `$HOME` instead; accepted for the shell.

## Catalog and lock schema

**Existing locks stay byte-identical (F5).** All new fields are optional and `omitempty`; the lock
schema number stays `2`; `catalog_digest` of every existing catalog is unchanged because the new
`Package` fields are `omitempty`. Nine bundle locks exist today. Changes:

| Struct | Change |
| --- | --- |
| `Lock` | optional `toolchains` and `extra_sources` maps (only the toolbox lock carries them) |
| `Spec` | optional `sha3_256`, optional `toolchain` (name); a per-file spec shape for `extra_sources` |
| `Package` | provider value `source-build`; optional `version_source` (`kind`, `url`), `build` (`kind`, `toolchain`, `archive_prefix`, `sources`, `extra_sources`, `defines`, `cflags`, `libs`, `static`, `output_form`, `output`) and `extra_sources` (`repository`, `commit`, `files`, `url`, `allowed_hosts`); `install.kind: "build-output"`; normalization `semver` unchanged |
| `ValidateSpec` (`config.go` "exactly one checksum") | still exactly one of SHA256/SHA512 for existing classes; `publisher-sha3-256` requires `sha256` **and** `sha3_256` and is allowed only for `source-build` |
| `validate` | `source-build` branch; `Lock.Schema != 2` check unchanged |

*Compatibility:* an older `bdock` rejects a lock with unknown fields (`Decode` uses
`DisallowUnknownFields`, deliberately). Only the toolbox lock gains new fields, only when `sqlite3` is
added; older binaries fail loudly on that lock. Say so in the release notes of the PR that adds `sqlite3`.

*How "byte-identical" is tested:* (1) the existing strict `Load` keeps passing for every bundle, which
re-checks `catalog_digest`; (2) a new test loads each bundle lock (fixtures and production) and re-encodes
it with the repository serializer, asserting equal bytes for locks without new fields; (3) the PRs
WP1-WP2 show `git diff --stat origin/main -- bundles` empty (review evidence, not a unit test).

*WP3 implementation choices* (`internal/toolconfig/source_build.go`; the design left them open and the
code fixes them as closed Go lists, so a new entry is a reviewed code change):

- Toolchain names and their lock-URL hosts: `zig-cc` with `ziglang.org`. A lock `toolchains` entry for a
  name no `source-build` package uses is rejected, as is one that is not pinned for exactly `amd64` and
  `arm64` with one version and a `publisher-sha256` or `publisher-sha512` checksum.
- Allowed `defines`: exactly the seven of the sqlite3 example. `cflags`: `-Os`, `-O2`. `libs`: `m`, `pthread`.
  `kind`: `c-amalgamation`. `version_source.kind`: `sqlite-product-data`. `output_form`: `exec`, `static-pie`.
- A `source-build` package allows only `semver` normalization, `install.kind: build-output` with the single
  member equal to `build.output`, no install profile, and no repository, tag prefix, metadata URL or sidecar.
  The asset template may use `{numeric}` only there. `build.sources` are base names ending in `.c`.
- Main-archive classes: `publisher-sha256`, `publisher-sha512` or `publisher-sha3-256`; `tls-download-sha256`
  is rejected for it. `sha3_256` is valid only with `publisher-sha3-256`, which in turn needs the builder
  `sha256` and is valid only for `source-build`.
- WP3b (multi-source recipes): `build.extra_sources` lists exactly the package `extra_sources` entries
  (names `^[a-z][a-z0-9-]*$`); each has `owner/repo`, a full 40-hex lowercase `commit`, `files`
  (repository path to a build-directory base name, unique across all extra sources), a URL template with
  exactly `{commit}` and `{file}`, and `allowed_hosts` that are also in the package `allowed_hosts`. Each
  extra source must include a file named `LICENSE` or `COPYING`. `HAVE_LINENOISE=1` needs `linenoise.c`
  and `linenoise.h` from an extra source. The lock `extra_sources` key is `<tool>/<name>`; the lock commit
  equals the catalog commit, the locked files equal the catalog files, each locked URL is the template at
  that commit, and each file carries a 64-hex `sha256` with class `tls-download-sha256`. Whether the other
  `build.sources` really exist in the archive cannot be checked statically; the WP4 extractor fails on a
  missing member and must also fail when an archive member has the same base name as an extra-source build
  file (compared case-insensitively here, so no two build names differ only by case). The URL template must be
  exactly `https://<allowed host>/<repository>/{commit}/{file}`; repository file paths allow only letters,
  digits and `_.+-` with `/` separators, no segment may start with a dot or look like a 40-hex commit.

*WP4a implementation choices* (`source_build.go`, `internal/toolpublish/baseline.go`): a source-build lock URL
must be `base_url/` plus a path matching the version source's grammar (SQLite: `^20[0-9]{2}/sqlite-amalgamation-[0-9]{7}\.zip$`)
whose last segment is the asset template expanded with the locked version's numeric form; a toolchain lock
URL must be the exact archive of its version and architecture (`zig-cc`:
`https://ziglang.org/download/<v>/zig-<x86_64|aarch64>-linux-<v>.tar.xz`); both architectures pin the same
source entry. `ToolDigest` (the `platforms-sha256` label) covers `toolchains` and `extra_sources` when present
and is byte-identical to the earlier digest for the nine existing locks (tested).

*WP4 resolver and updater implementation choices* (`internal/toolmanager`): the SQLite resolver reads the product-data comment of
`download.html` (exact header line, five fields per row, exactly one `sqlite-amalgamation` row, positive size,
lower-case 64-hex SHA3-256, path grammar and numeric match, version inside the `packages.txt` range), downloads
the archive once and pins both architectures with the same entry. `bdock tools upgrade` copies `toolchains` and
`extra_sources` from the existing lock unchanged (a first lock without them fails validation by name), refuses
a lower version or other bytes under the locked version, and a changed catalog extra-source commit fails
`Validate` until the owner edits the lock. It writes the pin lines of the tool only.

*WP4c implementation choices* (`internal/toolbundle`): `CheckStaticELF(file, arch, form)` reads the program
headers (not sections) of a pure-Go `debug/elf` parse and rejects non-ELF input, a class other than ELFCLASS64, big
endian, a machine other than the target, a type other than the declared form, `PT_INTERP`, a missing `PT_LOAD` or
`PT_GNU_STACK`, an executable stack, `PT_DYNAMIC` in an `exec` output, and `DT_NEEDED`, `DT_RPATH` or `DT_RUNPATH`
in a `static-pie` one; its fixtures are synthetic ELF images, no compiler. `ExtractBuildSources` validates the whole
ZIP before writing: it installs the listed sources that no extra source provides, plus the headers, found directly
below `archive_prefix`, by base name with mode 0644; it fails on a missing source, a same-named member at another
depth, any member whose base name equals an extra-source build file (case-insensitive, anywhere in the archive),
case-only duplicates, unsafe or duplicate paths and non-regular entries.

*WP4d implementation choices* (`internal/toolbundle`, `cmd/tools-bundle`): two subcommands split the stages.
`tools-bundle fetch-sources -tool T -arch A -work W` runs in `downloads` (network): it downloads the zig archive for
`A` (publisher checksum), the source archive (builder SHA256 and publisher SHA3-256) and each extra-source file
(`tls-download-sha256`), verifies every checksum before it reads or unpacks the file, and writes `W/toolchain` and
`W/src` (`ExtractBuildSources`, the fail-closed rules above). The toolchain unpack accepts one top directory of
directories and regular files only, at most 50,000 members, 256 MiB per member and 1 GiB in total (the 0.17.0
aarch64 archive has 20,777 members, no links, 360 MB, the compiler 172 MB; measured 2026-10-24).
`tools-bundle compile -tool T -arch A -work W -output O` runs in `compile-<tool>` under `RUN --network=none`: it builds
the command line from fixed flags and the recipe's allow-listed values (`zig cc -target <arch>-linux-musl -static
<cflags> -ffile-prefix-map=<src>=. -D<define>... <sources in recipe order> -l<lib>... -o <output>`; `-static-pie` for
that output form; the vocabulary check is repeated), runs it in `W/src` with only `SOURCE_DATE_EPOCH=1` and the two zig
cache variables in the environment, and requires a non-empty regular output. The `builder` stage receives the output
(`--build-outputs`), `CheckStaticELF` accepts it for the target and declared form, and only then is it installed and
the manifest written. The production Dockerfile has no source-build tool yet; `cmd/tools-bundle/testdata/source-build.Dockerfile`
is the reviewed stage layout (never built): tests check it against the compile-stage rules, against the real flag
parser, and against the production Dockerfile, which it may differ from only by the source-build lines. WP5 adds the
stage to `bundles/toolbox/Dockerfile` with the catalog entry. `RUN --network=none` on the real BuildKit frontend is
still untested.

*WP5 implementation choices* (`bundles/toolbox`): the catalog entry is the example below, appended to the catalog, with the
package `allowed_hosts` `www.sqlite.org` and `raw.githubusercontent.com`. The notice installer copies each listed notice to
`licenses/<tool>/<base name>`, so the nested path of the proposal is not possible and the notices are flat:
`NOTICE` (SQLite public-domain dedication and the list of the combined works), `SOURCE` (pins and provenance),
`LINENOISE-LICENSE` (the very file the lock pins, tested), `MUSL-COPYRIGHT` and `ZIG-LICENSE` (read from the pinned zig 0.17.0
archive; the musl `COPYRIGHT` is byte-identical in the x86_64 and aarch64 archives). No catalog `source` block is added: no
toolbox package has one yet. The lock was filled from publisher data and checked against it: the SHA3-256 and size of the
amalgamation equal the `download.html` product-data row; the SHA256 covers the same bytes; both zig archives equal the `shasum`
and `size` of `ziglang.org/download/index.json`; the three linenoise files are `tls-download-sha256` (builder-computed, no
publisher value exists). The production Dockerfile has the fetch line, the `compile-sqlite3` stage and the builder copy; the
fixture of WP4d is removed and its tests read the production Dockerfile. `sqlite3-smoke.sh` runs in the consumer smoke.
Not done and needing the owner's approval: any build, the BuildKit `RUN --network=none` run, native smoke on both architectures,
publication.

### Catalog: sqlite3

```json
"sqlite3": {
  "provider": "source-build",
  "normalization": "semver",
  "version_source": { "kind": "sqlite-product-data", "url": "https://www.sqlite.org/download.html" },
  "architectures": { "amd64": "x86_64", "arm64": "aarch64" },
  "asset": "sqlite-amalgamation-{numeric}.zip",
  "base_url": "https://www.sqlite.org",
  "allowed_hosts": ["www.sqlite.org"],
  "verification": "official-or-download",
  "build": {
    "kind": "c-amalgamation",
    "toolchain": "zig-cc",
    "archive_prefix": "sqlite-amalgamation-{numeric}/",
    "sources": ["shell.c", "sqlite3.c", "linenoise.c"],
    "extra_sources": ["linenoise"],
    "defines": ["HAVE_LINENOISE=1", "SQLITE_THREADSAFE=1", "SQLITE_OMIT_LOAD_EXTENSION=1", "SQLITE_ENABLE_FTS5=1",
                "SQLITE_ENABLE_RTREE=1", "SQLITE_ENABLE_DBSTAT_VTAB=1", "SQLITE_ENABLE_MATH_FUNCTIONS=1"],
    "cflags": ["-Os"],
    "libs": ["m", "pthread"],
    "static": true,
    "output_form": "exec",
    "output": "sqlite3"
  },
  "extra_sources": {
    "linenoise": {
      "repository": "antirez/linenoise",
      "commit": "a473823d74b93eab2ba83480df16ed37617493f2",
      "files": { "linenoise.c": "linenoise.c", "linenoise.h": "linenoise.h", "LICENSE": "LICENSE" },
      "url": "https://raw.githubusercontent.com/antirez/linenoise/{commit}/{file}",
      "allowed_hosts": ["raw.githubusercontent.com"]
    }
  },
  "install": { "kind": "build-output", "members": { "sqlite3": "bin/sqlite3" } },
  "notices": ["licenses/sqlite3/NOTICE", "licenses/sqlite3/SOURCE", "licenses/sqlite3/LINENOISE-LICENSE",
              "licenses/sqlite3/MUSL-COPYRIGHT", "licenses/sqlite3/ZIG-LICENSE"]
}
```

The `build.extra_sources` list names the entries of the package-level `extra_sources` map that the recipe
consumes (shown as proposals; the implementation may fold both into one structure). The `allowed_hosts`
of the package must also contain `raw.githubusercontent.com`.

Field names are proposals within the same semantics; `output_form` is settled by the spike. The numeric
form (`3.54.1` to `3540100`, `major*1000000+minor*10000+patch*100`) is checked in a fixed fixture
against the product data.

**URL year (F6).** The path `2026/` is not derivable from the version. The resolver takes the
`RELATIVE-URL` column from the product data for the `sqlite-amalgamation` row and validates it with the
fixed regex `^20[0-9]{2}/sqlite-amalgamation-[0-9]{7}\.zip$`, then builds `https://www.sqlite.org/` +
path, which must pass `ValidateURL` with `allowed_hosts`. The numeric part must equal the numeric form of
the product-data version. The path is stored only as the URL in the lock; the catalog `asset` is the file-name template
the path's last segment must match. Only
the current version is listed, so a lock can be built only for the current release; older locks keep
their stored URL (that sqlite.org retains old release files is unverified). Archive members sit under
`sqlite-amalgamation-<numeric>/` and are extracted by base name with the existing safe extractor
(`ValidPath`, duplicates and non-regular entries rejected).

### Lock fragment

```json
{
  "schema": 2,
  "desired_digest": "...", "catalog_digest": "...",
  "toolchains": {
    "zig-cc": {
      "amd64": { "version": "0.17.0", "url": "https://ziglang.org/download/0.17.0/zig-x86_64-linux-0.17.0.tar.xz",
                 "sha256": "<64 hex from the publisher index>", "verification": "publisher-sha256" },
      "arm64": { "version": "0.17.0", "url": "https://ziglang.org/download/0.17.0/zig-aarch64-linux-0.17.0.tar.xz",
                 "sha256": "<64 hex from the publisher index>", "verification": "publisher-sha256" }
    }
  },
  "platforms": {
    "amd64": { "sqlite3": { "version": "3.54.1",
      "url": "https://www.sqlite.org/2026/sqlite-amalgamation-3540100.zip",
      "sha256": "<64 hex, builder-computed>", "sha3_256": "<64 hex, from product data>",
      "verification": "publisher-sha3-256", "toolchain": "zig-cc" } },
    "arm64": { "sqlite3": { "...": "same source, same hashes" } }
  },
  "extra_sources": {
    "sqlite3/linenoise": {
      "commit": "a473823d74b93eab2ba83480df16ed37617493f2",
      "files": {
        "linenoise.c": { "url": "https://raw.githubusercontent.com/antirez/linenoise/a473823d74b93eab2ba83480df16ed37617493f2/linenoise.c",
                         "sha256": "<64 hex, builder-computed>", "verification": "tls-download-sha256" },
        "linenoise.h": { "...": "same shape" },
        "LICENSE":     { "...": "same shape" }
      }
    }
  }
}
```

The hashes shown are placeholders; this design records none that it computed except the facts above. The
zig hashes in the publisher index (0.17.0, 2026-10-24) are real but not copied here to avoid a stale pin.

## Second source: linenoise

1. **Which.** `antirez/linenoise` at the full commit `a473823d74b93eab2ba83480df16ed37617493f2`
   (HEAD of `master` on 2026-10-24; pushed 2026-05-02; BSD-2-Clause). Chosen because `shell.c`
   documents the two supported APIs and this one is `HAVE_LINENOISE=1`; its header matches (read, not
   compiled). The pin is a commit SHA in the catalog, never a branch or tag.
2. **Fetch and trust class.** GitHub commit pages carry no publisher checksum, so this is **not** the
   publisher-hash class of sqlite.org. The builder fetches the three files individually from
   `raw.githubusercontent.com/antirez/linenoise/<commit>/<file>` (content addressed by the commit),
   computes SHA256 and the lock stores it with the existing class `tls-download-sha256` (existing in
   `ValidateSpec`; the commit SHA in the URL is validated against `^[0-9a-f]{40}$` and must equal the
   catalog commit). The tarball/zipball endpoints are not used. *Residual risk:* trust is TLS to GitHub plus
   the integrity of that repository's history; an upstream compromise that rewrites history or removes the
   commit fails closed (404 or hash mismatch), but a malicious commit chosen at pin time is not detected by
   any control. Mitigation: the pin is reviewed once by the owner (the diff of two files of a few
   thousand lines), and there is no publisher signature to verify. That git's SHA-1 commit id is the
   only content address is accepted because the lock hashes the file bytes with SHA256.
3. **Bumps.** The pin is part of the **recipe** (catalog), not a rolling updater input: the updater never
   changes it, so under D2 a linenoise bump is a recipe change that needs explicit owner approval. The PR
   must show `gh api repos/antirez/linenoise/compare/<old>...<new>` file changes and the new hashes.
4. **Licence.** The upstream `LICENSE` is the BSD 2-clause licence ("Copyright (c) 2010-2014, Salvatore
   Sanfilippo; Copyright (c) 2010-2013, Pieter Noordhuis", conditions: retain the copyright notice and
   disclaimer in source and **in binary form in the documentation/materials**). It is retained verbatim
   at `licenses/sqlite3/LINENOISE-LICENSE` (flat, because notices are installed by base name; fetched as a locked file above), referenced by the `SOURCE`
   notice. SQLite stays public domain; the binary is therefore a public-domain plus BSD-2-Clause plus
   (musl, MIT) work, listed in the notice. Whether the `source` licence block accepts a multi-licence
   list as written is checked in WP5.
5. **Recipe.** Second entry in the source set, `-DHAVE_LINENOISE=1`, compiled with `zig cc` in the same
   static, network-less `compile` stage; `linenoise.c` needs only libc headers, so there is no terminfo
   or ncurses dependency (it emits VT100 escapes itself; verified at run time on 2026-10-24, with the caveat that it queries the cursor position (`ESC [ 6 n`) when the terminal size is unknown, that is `TIOCGWINSZ` fails or reports 0 columns; any optional pty smoke must set a window size or `LINENOISE_COLS`; D8 stays, no patch; see [spike-results.md](spike-results.md)). The ELF checker's
   `DT_NEEDED` rule proves no `libncurses`/`libtinfo` reference.
6. **Behaviour and smoke.** `shell.c` uses line editing only for an interactive stdin
   (`stdin_is_interactive`; its exact tty detection was not traced, unverified). The smoke runs, from a
   non-tty (pipe and `</dev/null`): `echo 'select 1+1;' | sqlite3 :memory:` prints `2`, with `TERM`
   unset and with `TERM=dumb`, and `sqlite3 :memory: 'select 1'` exits 0; none may fail for a missing or
   unsupported terminal. A pty-based interactive check (completion, history recall, editing keys) passed on 2026-10-24 with a scripted 24x80 pty that answered no query ([spike-results.md](spike-results.md)); saving the history file was not observed. It remains optional in the consumer smoke.

## Validation rules and tests

Validation: `source-build` requires `build`, `install.kind: build-output`, `build.static == true`, a
`toolchain` present in the lock for both architectures with a publisher-class hash, `sha3_256` rules above,
allow-listed `defines` (including `HAVE_LINENOISE=1`), `cflags`, `libs`; multi-source recipes: each extra source has a 40-hex commit equal in catalog, URL and lock, listed `files` only with `validPath` names, `tls-download-sha256` class (valid only for commit-pinned extra sources, never for the main archive), hosts in `allowed_hosts`, and every `build.sources` entry must come from the archive or a declared extra source (no `-include`, `-I` outside the extraction directory, `-Wl,`
pass-through), a declared `output_form`, and no `build` on other providers. Whether no official prebuilt exists for
both architectures (D1 a) is judged by the review of each new case, not by code.

Tests (TDD, failing first):

1. Replace `TestSQLiteRemovedFromBundleInputs` with `TestSQLiteIsBundledAsStaticSourceBuild`: catalog and
   `packages.txt` contain `sqlite3`, provider `source-build`, `static`, same version on both
   architectures, publisher class; asserts allowed versions, not constants.
2. Rejected-recipe table (unknown flag, `-include`, missing `static`, unknown toolchain, toolchain
   missing on one architecture, hash class mismatch).
3. `TestSourceBuildOutputsAreStatic`: for every `source-build` entry the checker accepts fixture ELFs of
   the declared form and rejects: an interpreter, `DT_NEEDED`, `DT_RPATH`, executable stack, wrong
   machine, wrong class, a PT_DYNAMIC in an `exec` fixture, and a non-ELF file.
4. Product-data parser and `{numeric}` fixtures, including the `RELATIVE-URL` regex, malformed lines,
   duplicate products and a wrong-length hash.
5. Safe extraction tests (zip-slip, duplicate, non-regular, missing member) and multi-source tests: duplicate file names across sources, a branch name instead of a commit, a URL commit differing from the catalog, a missing `LICENSE`.
6. Byte-identical round-trip of existing locks (above) and the offline synthetic lock-upgrade regression
   across all bundles, extended with a `source-build` fixture whose version bump leaves `toolchains` equal.
7. Updater test: a lock-only update leaves the `toolchains` section and the recipe unchanged, changes
   only lock and inventory, and starts no workflow.
8. Replacement of the guard tests (WP1): `internal/toolconfig/config_test.go:42-57`
   (`TestRepositoryIdentityAndBothArchitectures`: no git-trim, no `"source"` text in the lock. A `source-build`
   lock contains no `"source"` token, so that check would not fire; the test fails on the unknown provider
   in `Validate` instead, and it also pins the catalog size at 63 packages) and `internal/toolpublish/carrier_test.go:18-38` (exactly
   four Dockerfile stages, "the toolbox builds no source recipe", forbids `apk add --no-cache gcc`). The
   Dockerfile test must be rewritten to allow the new `compile` stage and to assert it: digest-pinned
   base, `--network=none`, no `apk add`, nothing but the verified inputs.
9. Smoke `bundles/toolbox/scripts/sqlite3-smoke.sh`, only after an approved build, on the consumer:
   `sqlite3 --version` equals the lock, in-memory create/insert/select, `PRAGMA compile_options` shows
   the defines and `THREADSAFE=1`, FTS5 and RTREE usable, `load_extension` unavailable, and the
   non-tty/no-terminal checks of the linenoise section.

## Notices and licence handling

SQLite is dedicated to the public domain (source header and `https://www.sqlite.org/copyright.html`;
not re-read here, unverified). Retain `licenses/sqlite3/NOTICE` and a `SOURCE` file with the pinned
version and URL, through the existing notice path. The `source` licence block: `obligation: none`,
basis `declared`, with whatever identifier `validSourceLicense` accepts for a public-domain dedication
(unverified; settled in WP5, the PR that adds the notices). A zig-built static binary embeds the musl C library (MIT); its notice must
be retained in the same notice tree, and that is a standing obligation of every `source-build` tool. The
zig compiler itself is build-only and not shipped, so it adds no shipped notice; this must be confirmed
by the notice test (the output may embed compiler runtime code; that licence check moves to WP5, which plans the musl `COPYRIGHT` and zig (compiler_rt) MIT notices next to the linenoise BSD-2-Clause licence).

## Security analysis

| What runs | Where | Network | Attacker with control |
| --- | --- | --- | --- |
| `tools-bundle` fetch + verify | `downloads` | `allowed_hosts` only | Altered file fails the lock hash; changing the hash needs a reviewed PR |
| zig archive extraction | `downloads` | same | Same; publisher checksum from `ziglang.org` index, pinned in lock |
| `zig cc` compile | `compile` | **none** (`--network=none`; documented, to be tested in the spike) | A malicious source or compiler can attack only the build container; no secrets, no network; output is verified static and hashed in the manifest |
| linenoise file fetch | `downloads` | `raw.githubusercontent.com` only | A rewritten or removed commit fails closed; a malicious commit chosen at pin time is not detected (trust class `tls-download-sha256`) |
| ELF checker | `builder` | none | Rejects dynamic or wrong-machine output |

- **Supply chain residual risk.** Hashes come from the publisher's own page (sqlite.org, ziglang.org),
  so a compromised publisher that changes both file and listing is not detected. Attestations are
  recorded, not verified (D5). The compiler is a large third-party binary executed at build time; it
  is pinned by hash and runs without network.
- **Updater.** Lock-only; never dispatches builds. See below for what a merge does.

## Update flow

`bdock tools upgrade` reads SQLite's product data, picks the newest version in the desired range and
writes the URL, SHA3-256 (publisher) and SHA256 (computed) for both architectures. It never edits
`toolchains` or the catalog.

**Consequence (F3).** Under `AGENTS.md` the actual owner `wstein` merging an eligible toolbox updater
PR authorizes publication of that exact frozen lock; therefore a merged SQLite bump **is compiled and
published automatically**. D2 limits that to source version and checksum bumps. The updater PR for a
source-build tool must therefore show, in its body and generated inventory: the tool, old and new
version, the new source URL and both hashes, and explicit lines "recipe digest: unchanged",
"toolchain: zig 0.17.0 unchanged (sha256 ...)" and "this merge compiles <tool> from source for amd64 and
arm64". The publish gate must stop automatic publication when anything but the source pins changed, by the
content check below. Those other changes are owner-authored PRs and need an explicit publication approval. The **first** `sqlite3`
publication likewise needs explicit approval of the exact source revision, like a first carrier
publication, and cannot ride an updater PR.

### Owner approval (WP5c, superseded)

An earlier design bound a per-tool `approved_digest` (`lock.approvals`) to the recipe, pins and driver
files and refused publication without it. The owner removed it: BundleDock is treated like a package
manager, the flow is "updater proposes, CI runs, owner merges", and the owner's merge is the approval. CI
builds and smoke-tests the source-build tool before the merge. The first publication of a source-build tool
and any recipe or toolchain change stay owner-authored; the content check below (G1-G4, T1-T4) is unchanged.
The lock has no `approvals` map; a lock that carries one is rejected as an unknown key by Go and by the gate.

### Publish-gate content check (N2)

`scripts/bundle-publication.py` `safe_files` (lines 37-43) checks file **names** only: a PR may change only
each listed bundle's lock and inventory. That already excludes catalog changes but not a changed
`toolchains` or `extra_sources` inside the lock. The reference for a content check is the **merged commit's single parent**, which the gate already
uses (`bundle-publication.py` lines 115-119: it requires `len(commit['parents']) == 1`, sets
`base = commit['parents'][0]` and compares `lock_bytes(get, bundle, revision)` with `lock_bytes(get, bundle, base)`).
`pull.base.sha` is not used: it can lag after a branch refresh or a rebase merge, and `main` after the
merge already contains the change.

"Only the source version and checksum rose" (D2) is defined on the parent lock B and the merged lock H.
**Global conditions** (every bundle, every tool): (G1) `H.toolchains == B.toolchains` and
`H.extra_sources == B.extra_sources`, deeply equal; (G2) no package is added or removed and the platform
set is unchanged; (G3) the catalog and `packages.txt` are unchanged (the existing name check; recipes
are thus unchanged); (G4) all existing conditions of the gate. **Per source-build tool T** (only
packages whose catalog provider is `source-build`; other tools keep the existing gate): (T1) every
`Spec` field other than `version`, `url`, `sha256`, `sha3_256` is equal, including `verification` and
`toolchain`; (T2) the new `version` is greater than the old and within the desired constraint of
`packages.txt`; (T3) `url` equals the URL the resolver rule for T derives from the new version (SQLite:
the product-data `RELATIVE-URL` regex and numeric match); (T4) amd64 and arm64 agree on version and, for
SQLite, both hashes. Any failure stops automatic publication with a named reason. Tests in
`scripts/bundle_approval_test.py`: accept a pure source bump (updater PR with only a source bump);
accept it under a **rebase merge** and under a **refresh merge** (the parent differs from
`pull.base.sha`; the check uses the parent); reject a changed `toolchains` entry, a changed
`extra_sources` hash or commit, a changed `verification`, a downgrade, a URL not derived from the
version, a version outside the constraint, an architecture disagreement and an added package; reject the
**first publication** of a source-build tool (B has no entry for T, so T2/G2 fail and explicit owner
approval is required). This check lands in its own work package (WP4b) **before the first source-build publication**.

## Later: acquisition providers

Out of scope (D4). The provider name `mise` is reserved in the catalog vocabulary so a later design does
not collide with it. If revisited: a `direct` provider for non-GitHub publishers (no mise needed) comes
first; a mise-assisted resolver would run at lock time only, never in the image build, with its
registry pinned by commit and only publisher-class hashes accepted. `mise.lock` and aqua checksum and
attestation semantics are unverified and need a spike. Recorded attestations follow D5.

## History lesson: the git-trim source exception

A digest-pinned Rust source recipe for git-trim was added on 2026-10-12 (`c2adaf5`, `f5a4ce6`) with a
long list of per-dependency notices, and removed on 2026-10-16 (`5d3f37c`, `2b1748b`, `0a71577`) with the
statement "Third-party tools are official stable prebuilt releases without exception". The commit
messages record the decision but not its reasons; I did not find them (unverified). The design takes
these precautions from that history: the vocabulary admits only dependency-free C amalgamations, so no
transitive notice tree or lockfile review is needed; the toolchain is a single hash-pinned archive;
every new tool needs its own review (D1); and the rule is amended in `AGENTS.md` *before* any code
(WP1) so that tests and rule never disagree. Whether the owner's earlier reason (maintenance cost,
trust, policy) still applies is OQ-B.

## Work packages

Each is one small PR; the first is a prerequisite and merges before the rest.

1. **WP1 rule change.** Amend `AGENTS.md` per D1 (owner-approved wording; the quoted rule above is
   replaced by the conditional rule, worded for reviewed static builds from pinned **sources** (plural: the archive and any further pinned sources such as linenoise), each with a recorded checksum: publisher data where offered, otherwise a commit-pinned builder-computed SHA256 recorded as a named exception with its residual risk, every bump needing owner approval), the two guard tests (`config_test.go:42-57`,
   `carrier_test.go:18-38`) so they permit `source-build` entries that satisfy the conditions and still
   reject any other source recipe, and the places that state the rule: `README.md` (line 12 "official
   stable prebuilt tools"), `docs/publication.md` (lines 193-194), `docs/release-readiness.md` (the
   "must not be closed by rebuilding or patching third-party tools" bullet) and `CONTRIBUTING.md`
   (line 61). No code for the provider.
2. **WP2 spike.** On a scratch branch, as read-only evidence in the PR: test-compile SQLite static for
   amd64 and arm64 with the pinned zig, report the ELF form (exec or static-pie), run the proposed
   verifier, confirm `RUN --network=none`, compare two builds for reproducibility (measured on 2026-10-24), and note zig's licence/runtime-embedding. Any image or local build needs explicit owner
   approval; the spike's findings update this document.
3. **WP3 schema and validation** (optional fields, `source-build` vocabulary, `sha3_256`, byte-identical
   round-trip tests; no tool uses it).
   **WP3b multi-source recipes** (separate small PR): package-level extra sources pinned by commit,
   per-file locked hashes with `tls-download-sha256` as the named exception, their validation and tests above; still no tool.
4. **WP4 source-build provider** (product-data resolver, `compile` stage, ELF checker, updater scope
   rules; no catalog entry).
   **WP4b publish-gate content check** (scripts and tests above; own PR, before WP5 can be published).
5. **WP5 sqlite3 recipe** (catalog with both sources, `packages.txt`, lock, notices (SQLite public-domain
   notice, linenoise BSD-2-Clause text, musl MIT; licence placement is in this PR, not WP4), smoke, replacement test, inventory).
6. **WP6 docs** (architecture, toolbox, development guides, this design status).

## Acceptance criteria

- WP3-WP5 pass `make check` (with `umask 022`); no existing lock or catalog entry changes
  (byte-identical test and empty `git diff --stat -- bundles` outside the toolbox).
- The locked `sqlite3` source SHA3-256 equals the publisher's and the SHA256 covers the same bytes; the
  builder refuses mismatches before extraction.
- Every `source-build` output passes the ELF checker; deliberately dynamic fixtures fail.
- After an owner-approved build: native smoke passes on both architectures and the consumer.
- An updater PR for SQLite bumps shows the required lines and leaves `toolchains` untouched.

## Risks

- Zig as a large third-party compiler (393 MB extracted); the static output form is measured (non-PIE `ET_EXEC`, [spike-results.md](spike-results.md)); BuildKit behaviour is still untested.
- Reproducibility measured on one host only; BuildKit build time unmeasured.
- A closed vocabulary will be too small for the next tool; each kind needs a design.
- Public-domain licence identifier and the musl notice obligation need a licence check.
- Re-running an exception the owner removed four days after adding it (git-trim): maintenance appetite.

## Open questions

- **OQ-B** The git-trim exception was removed after four days; is any condition of that decision still
  relevant (for example maintenance cost) that D1 should encode?
