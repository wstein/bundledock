#!/bin/sh
# Move the nixpkgs pin of the catalog to the head of nixpkgs-unstable: new rev,
# release url and narHash. The narHash comes from nix in the digest-pinned image.
# Nothing is built; run refresh-lock.sh afterwards. Needs Docker and network.
set -eu
script_dir=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
repo=$(CDPATH='' cd -- "$script_dir/../../.." && pwd)
catalog=$repo/bundles/nixbox/packages.catalog.json
nix_image=$(sed -n 's/^nix_image=//p' "$script_dir/refresh-lock.sh")
latest=$(python3 "$repo/scripts/nixbox-bump-rev.py" latest "$catalog")
if [ -z "$latest" ]; then
    printf 'nixpkgs is already at the head of nixpkgs-unstable\n'
    exit 0
fi
set -- $latest
rev=$1 url=$2
hash=$(docker run --rm "$nix_image" nix --extra-experimental-features nix-command \
    store prefetch-file --unpack --json "$url" | python3 -c 'import json,sys; print(json.load(sys.stdin)["hash"])')
python3 "$repo/scripts/nixbox-bump-rev.py" set "$catalog" "$rev" "$url" "$hash"
printf 'Moved nixpkgs to %s\n' "$rev"
