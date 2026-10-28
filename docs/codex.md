---
title: Codex bundle
description: Use the official Codex package and native helpers with writable consumer state.
---

# Optional Codex bundle

[All bundles](bundles.md)

`bundles/codex` supplies the official stable prebuilt Codex package for Linux
AMD64 and ARM64, with an independent catalog, lock, carrier image and volume
mounted read-only at `/opt/bdock/codex`.

<!-- BEGIN PUBLISHED BUNDLE -->
<!-- publication-record: {"attempt":"1","bundle":"codex","commit":"646ba6eafe1abf7766e84f740c57be4277a98d6a","created":"2026-10-28T15:25:19Z","digest":"sha256:bcb29c3d5d46365f17e1bb0f61b29500e4bd0621cbf3f85be110d75dfd1cf56a","image":"quay.io/bundledock/codex","lock_sha256":"e4a5c57484e684ebf78e88f5d987aafc534301695a2e401c604ed6f2402fb9bc","reference":"quay.io/bundledock/codex@sha256:bcb29c3d5d46365f17e1bb0f61b29500e4bd0621cbf3f85be110d75dfd1cf56a","run_attempt":"1","run_id":"40715314246","source":"https://github.com/wstein/bundledock","tag":"0.165.0-b1"} -->
## Published bundle

Version: `0.165.0-b1`

Index digest: `sha256:bcb29c3d5d46365f17e1bb0f61b29500e4bd0621cbf3f85be110d75dfd1cf56a`

Source: [`646ba6eafe1abf7766e84f740c57be4277a98d6a`](https://github.com/wstein/bundledock/commit/646ba6eafe1abf7766e84f740c57be4277a98d6a)

Publication: [run 40715314246, attempt 1](https://github.com/wstein/bundledock/actions/runs/40715314246/attempts/1)

Pull this published carrier by its immutable index digest:

```sh
docker pull quay.io/bundledock/codex@sha256:bcb29c3d5d46365f17e1bb0f61b29500e4bd0621cbf3f85be110d75dfd1cf56a
```

The source inventory below describes the current lock; this publication retains its exact source revision.
<!-- END PUBLISHED BUNDLE -->

<!-- BEGIN GENERATED TOOL INVENTORY -->
| Tool | Pinned version |
| --- | --- |
| codex | 0.165.0 |
<!-- END GENERATED TOOL INVENTORY -->

The [official stable release](https://github.com/openai/codex/releases/tag/rust-v0.162.1)
was inspected October 12, 2026. Both complete musl-package downloads match the
publisher's GitHub asset SHA256 digests and the existing toolbox pins. Each
archive contains 54 members, solely regular files and directories, with the
same resource graph on both architectures. Source checks preserve the complete
package layout: `bin/codex`, `bin/codex-code-mode-host`, `codex-package.json`,
`codex-path/rg`, `codex-resources/bwrap` and `codex-resources/zsh/bin/zsh` (when bundled).
The existing approved `codex-resources/voice` exclusion is retained. Helpers and
package resources remain beside the native executable; no wrapper relocates
or patches them. The package license, NOTICE and provenance are retained under
`licenses/codex`; upstream attribution embedded in binaries remains unchanged.
Archive inspection is not a licensing or security audit.

## Consumer use after approved publication

After an approved native image build, the shared targets support this carrier:

```sh
make bundle-build BUNDLE=codex
make bundle-smoke BUNDLE=codex
make bundle-volume BUNDLE=codex
```

Mount the printed `bundledock-codex-<full image ID>` volume at
`/opt/bdock/codex` read-only and add `/opt/bdock/codex/bin` to PATH.
Use a stock language consumer image; the scratch carrier only initializes its
volume. Keep HOME, `CODEX_HOME`, caches and project files writable outside the
bundle. Existing user configuration filenames and environment pass through.
The generic initializer validates exact tree membership, modes and SHA256s on
first and repeated initialization. No special link profile is needed because
these official packages contain only regular files and directories.

The native smoke source initializes twice, checks the retained helper graph,
starts/queries/stops the app-server daemon and runs two concurrent nonroot
consumers against a read-only mount. For the published `0.162.1-b1` carrier,
[run 38288791159, attempt 1](https://github.com/wstein/bundledock/actions/runs/38288791159/attempts/1)
completed the native image build and smoke checks on both AMD64 and ARM64
from source `cfdc89d576b8c8534e932d4601dbd973cc6cde4d`.
This runtime evidence applies to that published revision; future revisions
require their own approved native build and smoke checks.

## Updates and publication

`bdock tools upgrade --directory bundles/codex` resolves Codex alone.
The daily 06:50 UTC **Propose upgrades** workflow and manual `bundle=codex` dry runs use the shared
source-only updater. It can change only its own lock and generated inventory.
A later actual human **wstein** merge follows the shared exact merged-main CI
and frozen-lock [publication flow](publication.md). It cannot authorize
initial publication. Manual dispatch uses the full approved main SHA; an
absent baseline is accepted only after authenticated registry HTTP 404 in that
manual operation. Successful approved publication includes an annotated
`codex/<immutable-version>` source tag and a verified documentation receipt.

The source toolbox no longer includes Codex, Antigravity or Claude. Their
standalone pins and notices are unchanged. The previously published toolbox
receipt retains its original contents until a separately approved publication.
