# Quickstart: third-party source mirror

Commands exist only after the work items in [tasks.md](tasks.md); this is the intended use. Steps 1, 3, 5 and 6
are offline and run without approval in a source checkout. Step 2 (`pin`) uses the network and writes the
source lock, so agents do not run it without asking ([effects](contracts/sources-command.md#effects));
step 4 (`fetch`) is a build and needs explicit approval each time. Use a temporary
directory for outputs.

1. Inspect what the catalog says (offline):

   ```bash
   bdock tools sources check --json | jq '.summary'
   ```

2. Create a source-lock entry (network; reads upstream, writes only the entry into the source lock
   under `--sources`; the owner decides whether this needs approval, plan open question 7):

   ```bash
   out="$(mktemp -d)"
   bdock tools sources pin shellcheck --sources "$out/sources"
   jq '.versions' "$out/sources/shellcheck.lock.json"
   ```

   Review the digest against an independent download or upstream's published value, then commit the
   source lock to the mirror repository by PR.
3. Render an existing plan against the catalog and lock (offline; `consistent: yes` expected):

   ```bash
   bdock tools sources plan --component shellcheck --version 0.11.0 --sources "$out/sources"
   ```

4. (Approval required) the build step, run by the release process in the mirror repo, locally equivalent to:

   ```bash
   bdock tools sources fetch --component shellcheck --version 0.11.0 --sources "$out/sources" --out "$out/bundle"
   ```

   Run it twice into different directories: both manifests report the same `tar_sha256`.
5. Verify (offline):

   ```bash
   bdock tools sources check --bundle-dir "$out/bundle"
   ```

6. After the release exists, add the registry entry, run `bdock tools sources sync`, and
   `bdock tools sources check --enforce` exits 0.
7. Upgrade rehearsal: bump a copyleft tool in a scratch copy of the lock; `check --enforce` exits 1 with
   `mirror-entry-missing` (the [upgrade sequence](plan.md#upgrade-sequence) continues from there).
