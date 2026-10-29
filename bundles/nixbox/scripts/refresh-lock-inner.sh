#!/bin/sh
# Runs inside the digest-pinned nixos/nix image (see refresh-lock.sh).
set -eu
export NIX_CONFIG="experimental-features = nix-command"
catalog=/src/bundles/nixbox/packages.catalog.json
work=/work
for arch in amd64 arm64; do
    export ARCH=$arch
    # Roots: one store path per nix package, evaluated for the target system.
    nix eval --impure --json --expr "
      let
        c = builtins.fromJSON (builtins.readFile $catalog);
        pkgs = import (builtins.fetchTarball { url = c.nixpkgs.url; sha256 = c.nixpkgs.narHash; }) {
          system = c.platforms.\${builtins.getEnv \"ARCH\"};
          config = {};
        };
        pick = p: pkgs.lib.attrByPath (pkgs.lib.splitString \".\" p.attr) (throw \"no attribute \${p.attr}\") pkgs;
      in builtins.mapAttrs (n: p: (pick p).outPath) (pkgs.lib.filterAttrs (n: p: p.provider == \"nix\") c.packages)
    " > "$work/roots-$arch.json"
    paths=$(nix eval --impure --raw --expr "builtins.concatStringsSep \" \" (builtins.attrValues (builtins.fromJSON (builtins.readFile $work/roots-$arch.json)))")
    # Signatures are not trusted here; "nixbox lock" verifies every one offline.
    # shellcheck disable=SC2086
    nix path-info --json --json-format 1 -r --store https://cache.nixos.org $paths > "$work/pathinfo-$arch.json"
done
"$work/nixbox" lock -catalog "$catalog" -input "$work" -output "$work/packages.lock.json"
