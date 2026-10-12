#!/bin/sh
# One non-root, offline consumer shares all five read-only mounts and one persistent HOME.
set -eu
export PATH="/opt/bundledock/codex/bin:/opt/bundledock/agy/bin:/opt/bundledock/claude/bin:/opt/bundledock/node/bin:$HOME/.local/bin:/opt/bundledock/toolbox/bin:$PATH"
export CODEX_HOME="$HOME/.codex"
mkdir -p "$CODEX_HOME" "$HOME/.config" "$NPM_CONFIG_CACHE" "$NPM_CONFIG_PREFIX"
case "$VERIFY_PASS" in
    1) printf '%s\n' retained-config > "$HOME/.config/bundledock-interop"
       printf '%s\n' retained-codex-home > "$CODEX_HOME/interop-marker" ;;
    2) test "$(cat "$HOME/.config/bundledock-interop")" = retained-config
       test "$(cat "$CODEX_HOME/interop-marker")" = retained-codex-home ;;
    *) exit 1 ;;
esac
for directory in "$HOME" "$CODEX_HOME" "$HOME/.config" "$NPM_CONFIG_CACHE" "$NPM_CONFIG_PREFIX"; do
    touch "$directory/write-test"
    rm "$directory/write-test"
done
for name in toolbox node codex agy claude; do
    root=/opt/bundledock/$name
    test -f "$root/.ready"
    if touch "$root/unexpected-write" 2>/dev/null; then
        echo "Unexpected write to $name bundle" >&2
        exit 1
    fi
done
test "$(command -v rg)" = /opt/bundledock/toolbox/bin/rg
rg --version
for name in codex agy claude; do
    test "$(command -v "$name")" = "/opt/bundledock/$name/bin/$name"
    "$name" --version
done
test -f /opt/bundledock/codex/codex-package.json
for helper in bin/codex-code-mode-host codex-path/rg codex-resources/bwrap codex-resources/zsh/bin/zsh; do
    test -x "/opt/bundledock/codex/$helper"
done
for name in agy claude; do
    root=/opt/bundledock/$name
    if test -e "$root/licenses.tar.gz" || test -e "$root/licenses.index.json"; then
        # New carriers verify the complete SOURCE through their helper before this consumer.
        test -f "$root/licenses.tar.gz"
        test -f "$root/licenses.index.json"
    else
        # Fixed historical published carriers retain the original loose layout.
        test -f "$root/licenses/$name/SOURCE"
    fi
done
for name in node npm npx; do
    test "$(command -v "$name")" = "/opt/bundledock/node/bin/$name"
    "$name" --version
done
node -e 'if (process.execPath !== "/opt/bundledock/node/payload/bin/node") process.exit(1)'
test "$(npm config get cache)" = "$NPM_CONFIG_CACHE"
test "$(npm config get prefix)" = "$NPM_CONFIG_PREFIX"
mkdir -p "$HOME/project"
printf '%s\n' '{"name":"bundledock-offline-fixture","version":"1.0.0","scripts":{"check":"node -p process.execPath"}}' > "$HOME/project/package.json"
cd "$HOME/project"
test "$(npm --offline --silent run check)" = /opt/bundledock/node/payload/bin/node
test "$(npx --offline --no-install --call 'node -p process.execPath')" = /opt/bundledock/node/payload/bin/node
echo 'Five published bundles work together in the offline stock Go consumer'
