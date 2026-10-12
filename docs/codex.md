# Optional Codex bundle

`bundles/codex` supplies the official stable prebuilt Codex package for Linux
AMD64 and ARM64, with an independent catalog, lock, carrier image and volume
mounted read-only at `/opt/bundledock/codex`.

<!-- BEGIN PUBLISHED BUNDLE -->
No verified publication receipt exists yet. This carrier is prepared in source;
its initial build and publication require explicit approval of the exact full
source revision and intended operation. Native runtime coverage is unverified.
<!-- END PUBLISHED BUNDLE -->

<!-- BEGIN GENERATED TOOL INVENTORY -->
| Tool | Pinned version |
| --- | --- |
| codex | 0.162.1 |
<!-- END GENERATED TOOL INVENTORY -->

The [official stable release](https://github.com/openai/codex/releases/tag/rust-v0.162.1)
was inspected October 12, 2026. Both complete musl-package downloads match the
publisher's GitHub asset SHA256 digests and the existing toolbox pins. Each
archive contains 54 members, solely regular files and directories, with the
same resource graph on both architectures. Source checks preserve the complete
package layout: `bin/codex`, `bin/codex-code-mode-host`, `codex-package.json`,
`codex-path/rg`, `codex-resources/bwrap` and `codex-resources/zsh/bin/zsh`.
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
`/opt/bundledock/codex` read-only and add `/opt/bundledock/codex/bin` to PATH.
Use a stock language consumer image; the scratch carrier only initializes its
volume. Keep HOME, `CODEX_HOME`, caches and project files writable outside the
bundle. Existing user configuration filenames and environment pass through.
The generic initializer validates exact tree membership, modes and SHA256s on
first and repeated initialization. No special link profile is needed because
these official packages contain only regular files and directories.

The native smoke source initializes twice, checks the retained helper graph,
starts/queries/stops the app-server daemon and runs two concurrent nonroot
consumers against a read-only mount. It has not been run for this carrier.
Source checks establish configuration and shared adapter behavior; image
builds, downloaded-binary execution and native smoke remain deferred.

## Updates and publication

`bundledock tools upgrade --directory bundles/codex` resolves Codex alone.
The daily 06:50 UTC proposal and manual dry-run workflows use the shared
source-only updater. It can change only its own lock and generated inventory.
A later exact-head **wstein** approval follows the shared guarded source CI,
merge and frozen-lock [publication flow](publication.md). It cannot authorize
initial publication. Manual dispatch uses the full approved main SHA; an
absent baseline is accepted only after authenticated registry HTTP 404 in that
manual operation. Successful approved publication includes an annotated
`codex/<immutable-version>` source tag and a verified documentation receipt.

Toolbox still contains Codex, Antigravity and Claude. Prepare Anchorage consumers
before any separately approved removal; no consumer activation is included here.
