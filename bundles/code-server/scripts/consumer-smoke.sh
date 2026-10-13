#!/bin/sh
# Run only inside an approved native glibc consumer; no external network access.
set -eu
root=/opt/bdock/code-server
node=$root/payload/lib/node
test -f "$root/.ready"
test -x "$node"
test -f "$root/payload/LICENSE"
test -f "$root/payload/ThirdPartyNotices.txt"
test -d "$root/payload/lib/vscode"
test "$(readlink "$root/payload/node_modules/.bin/semver")" = ../semver/bin/semver.js
"$root/bin/code-server" --version
mkdir -p "$HOME/.cache" "$HOME/.local/share/code-server/extensions"
export XDG_CACHE_HOME="$HOME/.cache"
# This disposable fixture password is deliberately unrelated to user credentials.
export PASSWORD=bundledock-native-smoke-only
"$root/bin/code-server" --bind-addr 127.0.0.1:18080 --auth password \
  --disable-telemetry --disable-update-check \
  --user-data-dir "$HOME/.local/share/code-server" \
  --extensions-dir "$HOME/.local/share/code-server/extensions" "$HOME" \
  > "$HOME/server.log" 2>&1 &
server=$!
trap 'kill "$server" 2>/dev/null || :; wait "$server" 2>/dev/null || :' EXIT
"$node" <<'JAVASCRIPT'
const http = require('http');
function get(path) {
  return new Promise((resolve, reject) => {
    const request = http.get({host: '127.0.0.1', port: 18080, path}, response => {
      let text = '';
      response.setEncoding('utf8');
      response.on('data', chunk => { text += chunk; });
      response.on('end', () => resolve({status: response.statusCode, headers: response.headers, text}));
    });
    request.setTimeout(1000, () => request.destroy(new Error('HTTP timeout')));
    request.on('error', reject);
  });
}
(async () => {
  let login;
  for (let attempt = 0; attempt < 100; attempt++) {
    try { login = await get('/login'); if (login.status === 200) break; } catch {}
    await new Promise(resolve => setTimeout(resolve, 200));
  }
  if (!login || login.status !== 200 || !/password/i.test(login.text)) throw new Error('Password login page unavailable');
  const unauthenticated = await get('/');
  if (![302, 303, 307, 308].includes(unauthenticated.status) || !unauthenticated.headers.location?.includes('/login')) throw new Error('Unauthenticated editor access was not rejected');
})().catch(error => { console.error(error.message); process.exitCode = 1; });
JAVASCRIPT
test -f "$HOME/.config/code-server/config.yaml"
test -d "$HOME/.local/share/code-server"
if touch "$root/write-test" 2>/dev/null; then
  echo 'Unexpected write to code-server bundle' >&2
  exit 1
fi
