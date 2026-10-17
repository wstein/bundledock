# Analysis: `bdock` CLI release workflow

## Gap analysis of `docs/release-readiness.md` (draft of 2026-10-10)

| Draft statement or gap | State at 2026-10-17 | Action |
| --- | --- | --- |
| Local release builds Linux AMD64 and ARM64 only | Owner platform set is linux/amd64, linux/arm64, darwin/arm64 (darwin/amd64 EOL, Windows out) | `scripts/release.sh` builds darwin/arm64 |
| Version injection | Already present (`-X main.version`, tested) | kept; smoke asserts it on shipped bytes |
| Provenance and checksums | `PROVENANCE` with revision/modified/toolchain/flags and `SHA256SUMS` exist | gained `GOOS`, `targets`; CI asserts clean tree and revision |
| Reproducibility | local `make release-test` only (needs approval) | CI double build, byte-identical |
| Native execution evidence | "native AMD64 execution ... separate checks" | native smoke on three runners |
| Release is local, no CI path | owner decided "bdock wie smill" | `release.yml` |
| "Embedded dependency findings remain open in Gitleaks, Syft, Trivy" | concern the OCI toolbox image, not the CLI | stated honestly; not a CLI release blocker, still open for the image |
| Release review of the untracked TODO list | rule in `AGENTS.md` and `docs/development.md` | kept as a precondition in `docs/releasing.md`; not machine-checkable |
| Code of conduct enforcement owner | `CODE_OF_CONDUCT.md` exists with a conduct-report contact | closed |
| Security (vulnerability) reporting channel | still none: no `SECURITY.md` | kept open: spec open decision 7 |
| Interactive terminal behavior, `development.md#verification-evidence` link, draft feature summary | image/consumer topics or release-note prose | dropped on purpose: not CLI-release facts (verification evidence stays in development.md) |
| Quay publication, consumer adoption | separate (`publication.md`) | unchanged, out of scope |
| Tag namespace and version | undefined | `bdock/vX.Y.Z`, first `0.1.0` ([research](research.md)) |

The draft's revision pin (731c6fd) and dated evidence statements were removed rather than
refreshed: a note that names a revision goes stale again.

## Coverage (requirement to evidence)

| Requirement | Evidence |
| --- | --- |
| FR-1 | `TestVerifyVersionIsStrict`, `TestVerifyTagNamespace`, `TestVerifyTagChecksAnnotationAndMain` |
| FR-2 | rules `trigger`, mutants for widened/narrowed patterns |
| FR-3 | rules `owner`, `tag`, `ci`, `exists`, `guard`, `verify`, `order` |
| FR-4 | rules `build`, `matrix`, `artifact`; `scripts/release_test.py` |
| FR-5 | `TestSmoke*` (good archives and 10 mutated archives) |
| FR-6 | `scripts/release_test.py` (modes, owner, mtime, determinism, targets) |
| FR-7 | rules `publish`, `publish-if`, `environment`, `needs` |
| FR-8 | rules `permissions`, `pin`, `with`, `swallow`, `injection`, `reassign`, `download` |
| FR-9 | the package itself |
| FR-10 | `TestCIVetsEveryReleasePlatform` |

## Verification by the author

Run (source-level only): `gofmt -l`, `go vet ./...`, `go test ./...`, the Python unit tests
of `make check`, `actionlint` v1.7.7 on `release.yml` and `ci.yml`, `go vet` with
`GOOS`/`GOARCH` for the three platforms. `go test ./internal/toolbundle` fails in this
environment on `main` as well (Python launcher state ownership under the scratch worktree).
Not run, by rule: `make build`, `make release`, `make release-test`, `make docs*`, any Docker
check, any workflow, any tag operation. One scratch `go build -o` to a temporary directory was made only to read the CLI's help output
(the coordinator's brief allowed it); it was not packaged or kept.

## Review rounds

Each round is an independent Opus review (subagent type general-purpose) of a `git archive`
extraction, read-only on the worktree, executing checks in a private scratch directory.

### Round 1 (two reviewers on 96e2be3): both NOT READY, no run-time logic bug

Runtime reviewer: every offline attack on verify/publish failed closed (wrong, lightweight,
off-main, image and pre-release tags, existing/draft release, 404 vs 502, moved tag, actor
mismatch); real double builds of all three platforms were byte-identical; 34 of 70 fresh
mutants survived the tests. Policy reviewer: the general approval sentences contradicted the
exception.

| # | Sev | Finding | Resolution |
| --- | --- | --- | --- |
| 1 | high | AGENTS.md paragraphs 2 and 4 and CONTRIBUTING still demanded per-operation approval for all CLI release builds | Exception named beside Pages in all three places |
| 2 | med | No golden change detector; 34/70 mutants alive | `testdata/release.golden.json` (change detector only), more rules |
| 3 | med | Gate variables (`n`, `existing`, `remote`...) re-assignable; `update-ref` could redirect main | one assignment per variable, `:-`, `update-ref` and other ref edits banned |
| 4 | med | Stale CI statement in development.md; security-reporting gap dropped wrongly | corrected; open decision 7 |
| 5 | med | Exception paragraph contradicted itself on later tags | reworded: the grant is publication on the owner's tag; re-confirmation is open decision 3 |
| 6 | low | `fail-fast` default passed; publish counted archives by glob only | `*bool` must be false; exact names checked in publish |
| 7 | low | Script test gaps (shasum fallback, notes baseline/sort/pre-release, vet `\|\| true`) | tests added |
| 8 | low | 32 tags is 16; placeholder section; failure handling; badge label; missing open decisions | fixed or added |
| 9 | info | `release.sh` clears `GOFLAGS`; RELEASE_DIR inside the checkout makes the tree dirty | documented in the script header |

Harmless survivors (fail closed at run time or equivalent) were not chased individually.

