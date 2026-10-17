# Quickstart: third-party source mirror

Commands exist only after the work items in [tasks.md](tasks.md); this is the intended use. Steps 1,
2 and 5 are offline and run automatically; step 3 is a build and needs explicit approval each time.
Use a temporary directory for outputs, never the repository tree.

1. Inspect the plan for a copyleft tool (offline):

   ```bash
   out="$(mktemp -d)"
   bdock tools sources plan shellcheck --out "$out/plan.json"
   jq '{component,version,recipe: .recipe.kind, pinned: (.upstream.tarball_sha256 != null)}' "$out/plan.json"
   ```

2. Pin the digest the first time (network, still read-only on the repository; human compares the
   printed digest against upstream's published value or a second download):

   ```bash
   bdock tools sources fetch --plan "$out/plan.json" --out "$out/first" --accept-new-digest
   ```

   Commit the plan, with the digest filled in, to the mirror repository by PR.
3. (Approval required) the mirror workflow runs the same fetch: `workflow_dispatch(component=shellcheck,
   version=v0.11.0)`. Locally equivalent:

   ```bash
   bdock tools sources fetch --plan plans/shellcheck/v0.11.0.json --out "$out/bundle"
   ```

4. Compare two runs: both `tar_sha256` values in the manifests are equal.
5. Verify (offline):

   ```bash
   (cd "$out/bundle" && sha256sum --check SHA256SUMS)
   bdock tools sources check --bundle-dir "$out/bundle"
   ```

6. After the release exists, add the registry entry and the `SOURCE` mirror block; then
   `bdock tools sources check --enforce` exits 0.
7. Upgrade rehearsal: bump a copyleft tool in a scratch copy of the lock; `check --enforce` exits 1 with
   `mirror-entry-missing`.
