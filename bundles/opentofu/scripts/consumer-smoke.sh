#!/bin/sh
# Run only in an explicitly approved native consumer, without networking.
set -eu
root=/opt/bundledock/opentofu
fail() {
    printf 'OpenTofu smoke failed: %s\n' "$1" >&2
    exit 1
}
export PATH="$root/bin:$PATH"
test -f "$root/.ready"
test -f "$root/licenses.tar.gz"
test -f "$root/licenses.index.json"
tofu version
# Consumer-owned configuration, data, plugin cache and local state stay writable.
mkdir -p "$HOME/project" "$HOME/.cache/opentofu/plugins"
export TF_PLUGIN_CACHE_DIR="$HOME/.cache/opentofu/plugins"
export TF_DATA_DIR="$HOME/project/.terraform"
# Built-in-only init need not materialize a data directory. The fixture owns it.
mkdir -p "$TF_DATA_DIR" || fail 'prepare data directory'
printf '' > "$TF_DATA_DIR/.bundledock-write-test" || fail 'data directory writability'
rm "$TF_DATA_DIR/.bundledock-write-test" || fail 'data directory cleanup'
printf '%s\n' '# consumer-owned OpenTofu configuration' > "$HOME/.tofurc"
cp "$HOME/.tofurc" "$HOME/config-before"
cat > "$HOME/project/main.tf" <<'CONFIG'
resource "terraform_data" "fixture" {
  input = "bundledock-offline"
}
output "fixture" {
  value = terraform_data.fixture.output
}
CONFIG
cd "$HOME/project"
tofu init -backend=false -input=false
tofu validate
tofu plan -input=false -out=fixture.plan
tofu apply -input=false fixture.plan
fixture_output=$(tofu output -raw fixture) || fail 'fixture output command'
test "$fixture_output" = bundledock-offline || fail 'fixture output mismatch'
test -f terraform.tfstate || fail 'local state file missing'
test -d "$TF_DATA_DIR" || fail 'data directory missing'
cmp -s "$HOME/.tofurc" "$HOME/config-before" || fail 'consumer configuration changed'
if touch "$root/write-test" 2>/dev/null; then fail 'read-only bundle accepted an unexpected write'; fi
