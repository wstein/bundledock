#!/bin/sh
# Only run in an explicitly approved native glibc consumer, without networking.
set -eu
root=/opt/bundledock/bun
export PATH="$root/bin:$PATH"
export BUN_INSTALL="$HOME/.bun"
export BUN_INSTALL_CACHE_DIR="$HOME/.cache/bun/install"
export BUN_INSTALL_GLOBAL_DIR="$HOME/.local/share/bun/install/global"
export BUN_INSTALL_BIN="$HOME/.local/bin"
test -f "$root/.ready"
test -f "$root/licenses.tar.gz"
test -f "$root/licenses.index.json"
test "$(readlink "$root/payload/bunx")" = bun
bun --version
bunx --version
bun -e 'if (process.execPath !== "/opt/bundledock/bun/payload/bun") process.exit(1)'
mkdir -p "$HOME/project" "$BUN_INSTALL_CACHE_DIR" "$BUN_INSTALL_GLOBAL_DIR" "$BUN_INSTALL_BIN"
printf '%s\n' '{"name":"bundledock-bun-offline-fixture","version":"1.0.0","scripts":{"check":"bun check.js"}}' > "$HOME/project/package.json"
printf '%s\n' 'if (!process.execPath.includes("/bun/payload/bun")) process.exit(1);' > "$HOME/project/check.js"
(cd "$HOME/project" && bun install --ignore-scripts && bun run check)
# A local package bin verifies bunx dispatch without registry access.
mkdir -p "$HOME/project/node_modules/.bin"
printf '%s\n' '#!/bin/sh' 'test "$1" = "one two"' > "$HOME/project/node_modules/.bin/bundledock-fixture"
chmod 0755 "$HOME/project/node_modules/.bin/bundledock-fixture"
(cd "$HOME/project" && bunx --no-install bundledock-fixture 'one two')
if touch "$root/write-test" 2>/dev/null; then echo 'Unexpected write to Bun bundle' >&2; exit 1; fi
