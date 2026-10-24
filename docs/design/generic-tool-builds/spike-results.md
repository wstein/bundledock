# WP2 spike results: `zig cc` static build of the sqlite3 shell

**Status**: measured on 2026-10-24 for [spec.md](spec.md) work package 2. Local compile only, with the
owner's explicit approval of that scope. No Docker, Podman or BuildKit, no image, no `make` target, no
catalog, lock or Dockerfile change, no publication. Anything not listed as measured was not measured.

## Setup

- Host: Debian 13 (trixie), glibc 2.41, kernel 7.2.9 (OrbStack), **aarch64**, 10 cores. The host is
  therefore arm64: aarch64 is the native build here and x86_64 is a cross-compilation. The task text
  expected the reverse.
- Inputs, each fetched into its own empty directory and verified before use:

| Input | Size (bytes) | Verification |
| --- | --- | --- |
| `zig-aarch64-linux-0.17.0.tar.xz` | 52,877,280 | SHA256 `9e8d11661d4ae3bd57702a3832781e23ad151dde5798e16a5ccd503f65234ff8` equals the `shasum` in `ziglang.org/download/index.json`; size equals the index |
| `sqlite-amalgamation-3540100.zip` (`2026/` from the `RELATIVE-URL` column) | 2,990,966 | SHA3-256 `92281c86e2957f9b9305890f9ac04204caa7b677cc868dc326d942689df28e30` equals the product data; SHA256 `0a9c8e34352dad46006aa82836d5383bff9dc6bc9ce5a60553c05aa30e6ad1f9` |
| linenoise `linenoise.c` at `a473823d74b9...` | 82,933 | SHA256 `4bc28faf2a46ccaea11d07aa056e21aa2e9a748b4dc6962346a3f658593103a8` |
| linenoise `linenoise.h` | 4,914 | SHA256 `5f94c6295e3b62e0f8b4b62b0a2f351433d0b6b21029607aec0c9abbc60acff7` |
| linenoise `LICENSE` | 1,408 | SHA256 `efe1692aa5b869edaa0ac88f95eb0276c1125304ffc636a299db3ebbab47f62b` |

  The x86_64 zig archive (57,332,648 bytes, `1cbe9df9...2026` per the index) was **not downloaded**; only
  the aarch64 compiler was used for both targets. The index lists both architectures for 0.17.0.
- The archive extracts to 393 MB. The aarch64 `zig` binary has no `PT_INTERP` and no `DT_NEEDED`.
- Command (flags of the spec, plus `-ffile-prefix-map` where stated):
  `zig cc -target <arch>-linux-musl -static -Os -DHAVE_LINENOISE=1 -DSQLITE_THREADSAFE=1
  -DSQLITE_OMIT_LOAD_EXTENSION=1 -DSQLITE_ENABLE_FTS5=1 -DSQLITE_ENABLE_RTREE=1
  -DSQLITE_ENABLE_DBSTAT_VTAB=1 -DSQLITE_ENABLE_MATH_FUNCTIONS=1 shell.c sqlite3.c linenoise.c -lm -lpthread`.
  `linenoise.c` compiled and linked with `shell.c` without any diagnostic.

## Results

| # | Question | Result |
| --- | --- | --- |
| 1 | Zig download | 52,877,280 bytes (aarch64), checksum and size match the publisher index |
| 2 | Output form | **Non-PIE static executable (`ET_EXEC`)** on both targets. No `PT_INTERP`, no `PT_DYNAMIC`, no dynamic section; `PT_GNU_STACK` flags `RW` (not executable). This answers the static-PIE question: zig `-static` gives `exec`, so the recipe declares `output_form: "exec"` |
| 3 | Size | aarch64 1,429,368 bytes; x86_64 1,438,192 bytes (`-Os`, not stripped) |
| 4 | Compile time | 7.2 to 9.0 s wall (user 20 to 24 s) per target with an empty zig cache, which includes building musl; 7.0 s with a warm cache. Outside BuildKit |
| 5 | Runs here | `--version` prints `3.54.1 2026-10-16 ... (64-bit)`. With `TERM` unset and stdin `/dev/null`: an in-memory FTS5 table match, `sqrt(16)` = 4.0, `pi()`, an RTREE table and `dbstat` all work. Piped `select 1+1;` prints `2` with `TERM` unset, with `TERM=dumb` and with default `TERM`; `sqlite3 :memory: 'select 1'` exits 0. The aarch64 binary ran natively on a glibc Debian aarch64 host |
| 6 | x86_64 binary | ELF64 little endian, `Advanced Micro Devices X86-64`, `EXEC`, no interpreter, no dynamic section. It also executed here with identical output, but only under **Apple Rosetta** translation (OrbStack: the process maps showed `/mnt/rv/[rosetta]`; `binfmt_misc` is not mounted inside the container, which is why it looked empty). That is not native x86_64: x86_64 is verified by inspection and by Rosetta execution only; native x86_64 execution is **not measured** |
| 7 | aarch64 binary | ELF64 little endian, `AArch64`, `EXEC`, static, executed natively (see above). The consumer glibc Debian arm64 image was **not tested** |
| 8 | Reproducibility | See below: four variants per architecture plus a fifth for aarch64, all byte-identical |
| 9 | linenoise | Compiles and links as the spec assumes (`HAVE_LINENOISE=1` against the antirez header). Pty check below |
| 10 | Options | `PRAGMA compile_options` shows `OMIT_LOAD_EXTENSION`, `ENABLE_FTS5`, `ENABLE_RTREE`, `ENABLE_DBSTAT_VTAB`, `ENABLE_MATH_FUNCTIONS`, `THREADSAFE=1`, `COMPILER=clang-22.1.8`. `.load` is refused (`unknown command or invalid arguments: "load"`) and `load_extension()` is `no such function`. Same on both architectures |
| 11 | Network | With `unshare -rn` (a new network namespace with only a down `lo`; `curl` to ziglang.org failed to resolve) the compile succeeded from an empty zig cache, so no network is needed once the files are present. **BuildKit `RUN --network=none` was not tested** (no Docker) |

## Reproducibility

SHA256 of the output, same host and same zig, built from freshly copied sources. `$T` is `aarch64` or
`x86_64`; `$W` is the work directory; `CC` is `zig cc -target $T-linux-musl -static -Os
-DHAVE_LINENOISE=1 -DSQLITE_THREADSAFE=1 -DSQLITE_OMIT_LOAD_EXTENSION=1 -DSQLITE_ENABLE_FTS5=1
-DSQLITE_ENABLE_RTREE=1 -DSQLITE_ENABLE_DBSTAT_VTAB=1 -DSQLITE_ENABLE_MATH_FUNCTIONS=1`, run in the build
directory as `CC [extra] shell.c sqlite3.c linenoise.c -lm -lpthread -o <out>` with
`ZIG_GLOBAL_CACHE_DIR=<cache>/g ZIG_LOCAL_CACHE_DIR=<cache>/l`.

| Variant | Build dir, cache, extra flags, environment |
| --- | --- |
| 1 | `$W/b1-$T`, `$W/c1-$T`, `-ffile-prefix-map=$W/b1-$T=.`, `SOURCE_DATE_EPOCH=1700000000` |
| 2 | `$W/b2-$T`, `$W/c2-$T`, `-ffile-prefix-map=$W/b2-$T=.`, `SOURCE_DATE_EPOCH=1700000000`, under `unshare -rn` |
| 3 | `$W/b3-$T/deeper/x`, `$W/c3-$T`, no extra flags, `SOURCE_DATE_EPOCH=1` |
| 4 | `$W/b4-$T`, `$W/c4-$T`, no extra flags, `SOURCE_DATE_EPOCH` unset |

Result for variants 1 to 4 (all four equal per architecture):

- aarch64: `5567e7519290cb1831908a215164997f18a12cd7ffd42845f28141894a9a57c0`
- x86_64: `f27660cb2c753dbac7aa3aef411ebcff5335ef06bc309a27ac4f2154e0374a10`

(The independent review reproduced both hashes.) The x86_64 hash is a property of the bytes; the binary
itself was not run natively.

A fifth aarch64 build (warm cache) was also identical. `strings` finds no build path and no build date in
the aarch64 binary. Reasons it holds here: no `-g` (so no DWARF paths), no `__DATE__` or `__TIME__` use in
the sources, and zig's content-addressed cache. What to pin for bit-identical output, as far as measured:
the zig archive hash, the source hashes, the exact flag list and order, the target triple, the source file
order, and **no `-g`**. `-ffile-prefix-map` and `SOURCE_DATE_EPOCH` made no difference but cost nothing.
**Not measured:** builds on a different host, a different CPU count, a different zig version, `-g`, a
different user or umask, and whether the zig cache directory location matters when it sits under the
build path.

## ELF inspection

Output of `readelf` on variant 1 (the Go checker does not exist yet; the dynamic section is absent).

### aarch64

```text
$ readelf -hW sqlite3
ELF Header:
  Magic:   7f 45 4c 46 02 01 01 00 00 00 00 00 00 00 00 00
  Class:                             ELF64
  Data:                              2's complement, little endian
  Version:                           1 (current)
  OS/ABI:                            UNIX - System V
  ABI Version:                       0
  Type:                              EXEC (Executable file)
  Machine:                           AArch64
  Version:                           0x1
  Entry point address:               0x105f540
  Start of program headers:          64 (bytes into file)
  Start of section headers:          1428600 (bytes into file)
  Flags:                             0x0
  Size of this header:               64 (bytes)
  Size of program headers:           56 (bytes)
  Number of program headers:         9
  Size of section headers:           64 (bytes)
  Number of section headers:         12
  Section header string table index: 11
$ readelf -lW sqlite3

Elf file type is EXEC (Executable file)
Entry point 0x105f540
There are 9 program headers, starting at offset 64

Program Headers:
  Type           Offset   VirtAddr           PhysAddr           FileSize MemSize  Flg Align
  PHDR           0x000040 0x0000000001000040 0x0000000001000040 0x0001f8 0x0001f8 R   0x8
  LOAD           0x000000 0x0000000001000000 0x0000000001000000 0x04f540 0x04f540 R   0x10000
  LOAD           0x04f540 0x000000000105f540 0x000000000105f540 0x109438 0x109438 R E 0x10000
  LOAD           0x158978 0x0000000001178978 0x0000000001178978 0x000038 0x000688 RW  0x10000
  LOAD           0x1589b0 0x00000000011889b0 0x00000000011889b0 0x004240 0x00ded8 RW  0x10000
  TLS            0x158978 0x0000000001168978 0x0000000001168978 0x000000 0x000004 R   0x4
  GNU_RELRO      0x158978 0x0000000001178978 0x0000000001178978 0x000038 0x000688 R   0x1
  GNU_EH_FRAME   0x02a720 0x000000000102a720 0x000000000102a720 0x0061c4 0x0061c4 R   0x4
  GNU_STACK      0x000000 0x0000000000000000 0x0000000000000000 0x000000 0x1000000 RW  0

 Section to Segment mapping:
  Segment Sections...
   00
   01     .rodata .eh_frame_hdr .eh_frame
   02     .text
   03     .got .relro_padding
   04     .data .bss
   05     .tbss
   06     .got .relro_padding
   07     .eh_frame_hdr
   08
$ readelf -dW sqlite3

There is no dynamic section in this file.
```

### x86_64

```text
$ readelf -hW sqlite3
ELF Header:
  Magic:   7f 45 4c 46 02 01 01 00 00 00 00 00 00 00 00 00
  Class:                             ELF64
  Data:                              2's complement, little endian
  Version:                           1 (current)
  OS/ABI:                            UNIX - System V
  ABI Version:                       0
  Type:                              EXEC (Executable file)
  Machine:                           Advanced Micro Devices X86-64
  Version:                           0x1
  Entry point address:               0x1053e30
  Start of program headers:          64 (bytes into file)
  Start of section headers:          1437552 (bytes into file)
  Flags:                             0x0
  Size of this header:               64 (bytes)
  Size of program headers:           56 (bytes)
  Number of program headers:         7
  Size of section headers:           64 (bytes)
  Number of section headers:         10
  Section header string table index: 9
$ readelf -lW sqlite3

Elf file type is EXEC (Executable file)
Entry point 0x1053e30
There are 7 program headers, starting at offset 64

Program Headers:
  Type           Offset   VirtAddr           PhysAddr           FileSize MemSize  Flg Align
  PHDR           0x000040 0x0000000001000040 0x0000000001000040 0x000188 0x000188 R   0x8
  LOAD           0x000000 0x0000000001000000 0x0000000001000000 0x052e2c 0x052e2c R   0x1000
  LOAD           0x052e30 0x0000000001053e30 0x0000000001053e30 0x107e65 0x107e65 R E 0x1000
  LOAD           0x15aca0 0x000000000115cca0 0x000000000115cca0 0x004258 0x00df68 RW  0x1000
  TLS            0x15ac98 0x000000000115bc98 0x000000000115bc98 0x000000 0x000004 R   0x4
  GNU_EH_FRAME   0x02ecb0 0x000000000102ecb0 0x000000000102ecb0 0x005e9c 0x005e9c R   0x4
  GNU_STACK      0x000000 0x0000000000000000 0x0000000000000000 0x000000 0x1000000 RW  0

 Section to Segment mapping:
  Segment Sections...
   00
   01     .rodata .eh_frame_hdr .eh_frame
   02     .text
   03     .data .bss
   04     .tbss
   05     .eh_frame_hdr
   06
$ readelf -dW sqlite3

There is no dynamic section in this file.
```

## Pty check (linenoise)

A scripted pty (`pty.fork`, `TERM=xterm`, a fresh `HOME`) ran the aarch64 binary: a query, up-arrow history
recall, Ctrl-A and Ctrl-K editing, and Tab completion (`sel` became `SELECT`) all worked. This was run twice:
with the default 0x0 window (needs query replies, see below) and, after review, with a 24x80 window
(`TIOCSWINSZ`) and no replies to any query, where the same steps worked and no `ESC [ 6 n` was sent.

One finding for the design: `getColumns` in `linenoise.c` (lines 652 to 669, per the review) sends the cursor-position
query `ESC [ 6 n` (with `ESC [ 999 C`) only when `TIOCGWINSZ` on fd 1 fails or reports 0 columns. Python's
`pty.fork()` gives a 0x0 window, so the first run hit that path and, with no reply, the first typed query did
not run as typed (only `sel` and its completion appeared). With a 24x80 window no query is sent and editing
works. Use without a terminal is unaffected. D8 stands with no linenoise patch; an optional pty smoke must
set a window size (or `LINENOISE_COLS`). Bracketed-paste mode is also enabled. Saving the history file
was not observed. `expect` is not installed; the check used Python's `pty`.

## Not measured

BuildKit and `RUN --network=none`; execution on the consumer image or a real arm64 container; the
x86_64 zig archive and an x86_64 host; the repository Go ELF checker (it does not exist yet, `readelf`
was used and no `debug/elf` code was written); build time inside BuildKit; zig's licence and the
runtime code (compiler-rt, musl) embedded in the output (planned for WP5: musl `COPYRIGHT` and zig MIT notices); the history file; behaviour with `-g`.

## Consequences for the design

1. The recipe declares `output_form: "exec"` for both architectures.
2. The linenoise statement "emits VT100 escapes itself" needs the window-size caveat above.
3. No other statement of the design was contradicted.

## Addendum: shape of the zig archive (2026-10-24, WP4d)

Read-only listing of `zig-aarch64-linux-0.17.0.tar.xz` (downloaded again, SHA256 `9e8d1166...4ff8` equals the publisher index; the file was then deleted): 20,777 members under the single top directory `zig-aarch64-linux-0.17.0/`, of which 1,272 directories and 19,505 regular files; **no symbolic links, hard links or special files**; 360,344,860 bytes unpacked; the largest member is 171,691,896 bytes (the compiler). The `tar` listing was used, not the Go extractor. The x86_64 archive was not downloaded. The extractor's bounds (50,000 members, 256 MiB per member, 1 GiB) are set from these figures.
