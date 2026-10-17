# Analysis: third-party source mirror

Reviews are independent Opus runs (Agent tool, general-purpose, model opus) that execute checks in private
scratch directories; the author re-measured every claim that was cheap to re-measure.

## Round 0 (author draft, `db727b2`)

Author measurements are in [research.md](research.md); no independent review yet.

## Round 1 (reviewer on `db727b2`): NOT READY (1 critical, 13 high, 8 medium, 4 low)

Disclosure by the reviewer: four read-only GETs to the crates.io API carried a User-Agent with the user's
email address, against the instructions; nothing else was sent. Fixed procedure: reviewers and the author use a
generic User-Agent (`bundledock-source-mirror-review`).

Author re-verification of the reviewer's claims: typst (confirmed: `SOURCE` and `ASSETS-NOTICE`), helix
`jinja2.so` in the shipped aarch64 tarball (confirmed, lock hash equal), peeled commits (confirmed, the earlier
values were tag objects), duplicate `Cargo.lock`/`Cargo.toml` in the fish tarball and helix absolute symlinks
(confirmed), trufflehog 369 zip lines and two `replace` lines (confirmed), 21 of 41 packages with
`archive_notices`, `fd`/`rg` without a directory, `xz` not a package (confirmed), bun/opentofu/node carriers
(confirmed by grep), redirect hosts (measured), Spec 026 at `9a810a2` (read). Not re-measured: the MPL-2.0
dependency scale (stays labelled as reported).

| # | Sev | Finding | Resolution |
| --- | --- | --- | --- |
| 1 | critical | typst wrongly `none`; R1 measurement false | typst `source-offer` pending lawyer (coordinator decision); R1 rewritten with the correction; legal item 6 |
| 2 | high | MPL-2.0 scale unmeasured; rollout enforces too early | WI2 (measurement) gates backfill list, asset limit and enforcement flip; scale reported as unverified in the spec; rollout step order changed |
| 3 | high | FR-1 evidence misses archive notices, fd/rg, xz | FR-1 defines the evidence set; incomplete evidence is `undetermined` |
| 4 | high | FR-13 vs AGENTS.md snapshot rule | `SOURCE` gets a version-independent paragraph; version data in generated `SOURCE-MIRROR` (FR-13, D8) |
| 5 | high | upgrade chicken-and-egg, auto-publication | [upgrade sequence](plan.md#upgrade-sequence) with `--lock` flag; consequence accepted (open question 2) |
| 6 | high | quickstart step 2 vs approval | `pin` separated (writes a plan only, owner decision on classification); quickstart rewritten |
| 7 | high | `plan` not offline | `pin` is the networked writer of tag commit, epoch and digests; `plan` offline validator |
| 8 | high | `cabal-freeze` completeness | dependency-set sources ordered; shellcheck needs a decision (open question 3); revisions recorded as unverified equivalence |
| 9 | high | links and duplicate members vs refusal | FR-9 rewritten: no extraction, stored blob, anomalies recorded, last wins, only resource limits refuse |
| 10 | high | git sources / Hackage not independently verified | [verification classes](spec.md#verification-classes): `lockfile-hash` vs `plan-pin`; threat model states TOFU |
| 11 | high | GitHub redirects vs host allow-list | per-hop allow-list with measured targets ([fetch hosts](data-model.md#fetch-hosts)) |
| 12 | high | Spec 026 stale and conflicting | re-read at `9a810a2`; [mirror-project.md](contracts/mirror-project.md) rewritten (commit+version inputs, record commit, owner tag, track via pattern, stw writes checksums, open items) |
| 13 | high | backfill order vs monotonicity | oldest-first per component (FR-16, rollout step 6); batching raised to Spec 026 |
| 14 | high | other carriers ignored | spec Scope table: deferred, owner decision |
| 15 | medium | argv breaks determinism | normalized argv (FR-6, data model, mutants) |
| 16 | medium | gzip stability unpinned | `bdock_go_version` pinned in the plan (D9) |
| 17 | medium | Go set not measured | R3 rewritten; designed rule (zip lines plus `.mod`); WI2 re-measures |
| 18 | medium | `-rN` naming inconsistent | single [naming table](spec.md#naming-table); registry with revisions |
| 19 | medium | `undetermined` contradictory | one rule: warning with count, never an error |
| 20 | medium | tables vs command contract | `--max-bytes` removed, `network-disabled` defined, codes given statuses, xz/gzip bomb limits, JSON byte caps, exit table unified |
| 21 | medium | legal overreach | FR-13 and caveat reworded; 6(b) vs 6(d), offer scope, AGPL moved to lawyer items |
| 22 | medium | retroactivity vs published images | FR-16 states availability only |
| 23 | low | self-referential checksums | SHA256SUMS is the release process's; `MANIFEST.json` excluded from `files[]` |
| 24 | low | wrong peeled commits | corrected in R2 |
| 25 | low | hadolint example incomplete | semver added; eight PR #119 tools `undetermined`; API tarball chosen |
| 26 | low | small inconsistencies | FR-3 in tasks, undetermined count not fixed, `pax-sorted-v1` everywhere, FR-9 vs byte-identical resolved |
