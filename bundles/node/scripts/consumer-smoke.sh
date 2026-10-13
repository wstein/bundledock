#!/bin/sh
# Only run in an explicitly approved native glibc consumer, without networking.
set -eu
root=/opt/bdock/node
export PATH="$root/bin:$PATH"
export npm_config_cache="$HOME/.npm"
export npm_config_prefix="$HOME/.local/share/npm"
test -f "$root/.ready"
test -f "$root/payload/LICENSE"
test ! -e "$root/payload/bin/corepack"
test "$(readlink "$root/payload/bin/npm")" = ../lib/node_modules/npm/bin/npm-cli.js
test "$(readlink "$root/payload/bin/npx")" = ../lib/node_modules/npm/bin/npx-cli.js
node --version
npm --version
npx --version
node -e 'if (process.execPath !== "/opt/bdock/node/payload/bin/node") process.exit(1)'
test "$(npm config get cache)" = "$npm_config_cache"
test "$(npm config get prefix)" = "$npm_config_prefix"
mkdir -p "$npm_config_cache" "$npm_config_prefix" "$HOME/project"
printf '%s\n' '{"name":"bundledock-offline-fixture","version":"1.0.0","scripts":{"check":"node -e '\''process.exit(process.execPath.includes(\"/node/payload/bin/node\") ? 0 : 1)'\''"}}' > "$HOME/project/package.json"
(cd "$HOME/project" && npm --offline run check && npm --offline pack --ignore-scripts)
if touch "$root/write-test" 2>/dev/null; then echo 'Unexpected write to Node bundle' >&2; exit 1; fi
