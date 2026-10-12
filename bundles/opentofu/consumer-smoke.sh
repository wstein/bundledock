#!/bin/sh
# Run only in an explicitly approved native consumer, without networking.
set -eu
root=/opt/bundledock/opentofu
export PATH="$root/bin:$PATH"
test -f "$root/.ready"
test -f "$root/licenses/opentofu/LICENSE"
test -f "$root/licenses/opentofu/README.md"
test -f "$root/licenses/opentofu/CHANGELOG.md"
tofu version
# Consumer-owned configuration, data, plugin cache and local state stay writable.
mkdir -p "$HOME/project" "$HOME/.cache/opentofu/plugins"
export TF_PLUGIN_CACHE_DIR="$HOME/.cache/opentofu/plugins"
export TF_DATA_DIR="$HOME/project/.terraform"
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
test "$(tofu output -raw fixture)" = bundledock-offline
test -f terraform.tfstate
test -d "$TF_DATA_DIR"
cmp "$HOME/.tofurc" "$HOME/config-before"
if touch "$root/write-test" 2>/dev/null; then echo 'Unexpected write to OpenTofu bundle' >&2; exit 1; fi
