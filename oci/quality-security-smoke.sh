#!/bin/sh
set -eu
# Synthetic local fixtures only. HOME and scanner caches must be writable and
# external to the read-only bundle. OCI callers also enforce --network none.
fixture="$HOME/quality-security-smoke"
mkdir -p "$fixture/secret" "$fixture/config" "$fixture/sbom"
cd "$fixture"
typos --version
printf '%s\n' 'teh' > typo.txt
status=0
typos --isolated typo.txt > typos.txt 2>&1 || status=$?
test "$status" = 2
rg -q 'teh.*the' typos.txt
lychee --version
printf '%s\n' '<h1 id="ok">Hello</h1>' > target.html
printf '%s\n' '[local](target.html#ok)' > links.md
lychee --offline --include-fragments links.md

gitleaks version
cat > gitleaks.toml <<'CONFIG'
[[rules]]
id = "bundledock-synthetic"
description = "Synthetic smoke fixture"
regex = "BUNDLEDOCK_SYNTHETIC_[A-Z]{16}"
CONFIG
printf '%s\n' 'BUNDLEDOCK_SYNTHETIC_ABCDEFGHIJKLMNOP' > secret/fixture.txt
status=0
gitleaks dir secret --redact --no-banner --config gitleaks.toml --report-format json --report-path gitleaks.json > gitleaks.txt 2>&1 || status=$?
test "$status" = 1
jq -e 'length == 1 and .[0].RuleID == "bundledock-synthetic" and .[0].Secret == "REDACTED"' gitleaks.json

trivy --version
printf '%s\n' 'FROM alpine:latest' 'USER root' > config/Dockerfile
trivy config --skip-check-update --cache-dir "$HOME/.cache/trivy" --format json config > trivy.json
jq -e '[.Results[].Misconfigurations[]?.ID] | index("DS-0002") != null and index("DS-0026") != null' trivy.json

syft version
printf '%s\n' '{"name":"bundledock-smoke","version":"1.0.0","lockfileVersion":3,"packages":{"":{"name":"bundledock-smoke","version":"1.0.0"},"node_modules/synthetic-package":{"version":"1.2.3","license":"MIT"}}}' > sbom/package-lock.json
SYFT_CHECK_FOR_APP_UPDATE=false syft scan "dir:$fixture/sbom" --output syft-json > syft.json
jq -e 'any(.artifacts[]; .name == "synthetic-package" and .version == "1.2.3")' syft.json
