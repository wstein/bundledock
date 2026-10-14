#!/bin/sh
set -eu
trufflehog=${1:-trufflehog}
work=$(mktemp -d "${TMPDIR:-/tmp}/bundledock-trufflehog-smoke.XXXXXX")
trap 'rm -rf "$work"' EXIT HUP INT TERM
mkdir -p "$work/home" "$work/config" "$work/cache" "$work/secret" "$work/clean"
# Only synthetic local input. No caller credentials/configuration, updates or
# verification requests; the OCI consumer also enforces --network none.
env -i PATH="$PATH" HOME="$work/home" XDG_CONFIG_HOME="$work/config" \
    XDG_CACHE_HOME="$work/cache" TERM=dumb sh -eu -s "$work" "$trufflehog" <<'SMOKE'
cd "$1"
trufflehog=$2
"$trufflehog" --no-update --version
cat > detector.yaml <<"CONFIG"
detectors:
  - name: bundledock-synthetic
    keywords:
      - bundledock_synthetic_
    regex:
      token: "BUNDLEDOCK_SYNTHETIC_[A-Z]{16}"
CONFIG
printf "%s\n" "BUNDLEDOCK_SYNTHETIC_ABCDEFGHIJKLMNOP" > secret/fixture.txt
printf "%s\n" "No credentials here." > clean/fixture.txt
status=0
"$trufflehog" filesystem --no-update --no-verification --fail --fail-on-scan-errors --json \
    --config "$PWD/detector.yaml" "$PWD/secret" > found.json 2> found.log || status=$?
test "$status" = 183
jq -se 'length == 1 and .[0].Raw == "BUNDLEDOCK_SYNTHETIC_ABCDEFGHIJKLMNOP" and .[0].Verified == false' found.json
"$trufflehog" filesystem --no-update --no-verification --fail --fail-on-scan-errors --json \
    --config "$PWD/detector.yaml" "$PWD/clean" > clean.json 2> clean.log
jq -se 'length == 0' clean.json
SMOKE
