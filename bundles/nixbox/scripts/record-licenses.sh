#!/bin/sh
# Record the nixpkgs license of every locked closure path in
# bundles/nixbox/licenses.json. Evaluates the pinned nixpkgs in the digest-pinned
# nixos/nix image; nothing is built or copied. Needs Docker and network access to
# the nixpkgs release host.
set -eu
script_dir=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
repo=$(CDPATH='' cd -- "$script_dir/../../.." && pwd)
nix_image=nixos/nix@sha256:f159a58f390677f41e8e900a74b6e2f9c7f27b364122ae6acf7a652ce6c551c0
docker run --rm --mount "type=bind,src=$repo,dst=/src,readonly" "$nix_image" sh /src/bundles/nixbox/scripts/record-licenses-inner.sh |
    python3 -c 'import json,sys; json.dump(json.load(sys.stdin), sys.stdout, indent=1, sort_keys=True); print()' \
        > "$repo/bundles/nixbox/licenses.json"
printf 'Recorded bundles/nixbox/licenses.json\n'
