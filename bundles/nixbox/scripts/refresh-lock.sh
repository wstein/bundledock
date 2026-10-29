#!/bin/sh
# Refresh bundles/nixbox/packages.lock.json from the pinned nixpkgs revision of
# the catalog. Resolves attributes in the digest-pinned nixos/nix image and reads
# path information from cache.nixos.org; nothing is built or copied. Needs
# Docker and network access to the nixpkgs release host and cache.nixos.org.
set -eu
script_dir=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
repo=$(CDPATH='' cd -- "$script_dir/../../.." && pwd)
nix_image=nixos/nix@sha256:f159a58f390677f41e8e900a74b6e2f9c7f27b364122ae6acf7a652ce6c551c0
go_image=golang:1.27.2-alpine@sha256:f92b6ef800e499660581efdabdf25d9d817a9d124eaf900924f0504e7e27e12d
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
# The tool is built for the Docker host architecture, which also runs the nix image.
docker run --rm --network none --mount "type=bind,src=$repo,dst=/src,readonly" \
    --mount "type=bind,src=$work,dst=/out" -w /src -e CGO_ENABLED=0 \
    --tmpfs /gocache -e GOCACHE=/gocache "$go_image" go build -trimpath -o /out/nixbox ./cmd/nixbox
docker run --rm --mount "type=bind,src=$repo,dst=/src,readonly" \
    --mount "type=bind,src=$work,dst=/work" "$nix_image" sh /src/bundles/nixbox/scripts/refresh-lock-inner.sh
cp "$work/packages.lock.json" "$repo/bundles/nixbox/packages.lock.json"
printf 'Refreshed bundles/nixbox/packages.lock.json\n'
