#!/bin/sh
# Runs inside the digest-pinned nixos/nix image (see record-licenses.sh).
#
# Each locked store path is matched to the nixpkgs attribute that produces it: the
# catalog attributes, the aliases below, and the path name with trailing segments
# removed. A match needs the attribute's output path to equal the store path, so a
# license is never guessed. A path with no match is recorded with a null license.
set -eu
export NIX_CONFIG="experimental-features = nix-command"
nix eval --impure --json --expr '
  let
    catalog = builtins.fromJSON (builtins.readFile /src/bundles/nixbox/packages.catalog.json);
    lock = builtins.fromJSON (builtins.readFile /src/bundles/nixbox/packages.lock.json);
    src = builtins.fetchTarball { url = catalog.nixpkgs.url; sha256 = catalog.nixpkgs.narHash; };
    # store path names that differ from their attribute names
    aliases = [ "stdenv.cc.cc" "libgcc" "systemdLibs" "bashNonInteractive" "gmpxx" "db48" "libcap_ng" "util-linux" ];
    perArch = arch:
      let
        pkgs = import src { system = catalog.platforms.${arch}; config = {}; };
        lib = pkgs.lib;
        name = p: builtins.substring 44 (builtins.stringLength p) p;
        segments = s: lib.splitString "-" s;
        prefixes = s: let ps = segments s; in map (i: lib.concatStringsSep "-" (lib.take i ps)) (lib.range 1 (builtins.length ps));
        nixAttrs = map (p: p.attr) (builtins.filter (p: p.provider == "nix") (builtins.attrValues catalog.packages));
        get = a: let r = builtins.tryEval (lib.attrByPath (lib.splitString "." a) null pkgs); in if r.success then r.value else null;
        outputs = d: map (o: d.${o}) (d.outputs or [ "out" ]);
        produces = p: a:
          let d = get a; in
          d != null && (builtins.tryEval (lib.any (o: o.outPath == p) (outputs d))).value or false;
        attrFor = p: lib.findFirst (produces p) null (lib.unique (aliases ++ nixAttrs ++ prefixes (name p)
          ++ map (lib.replaceStrings [ "-" ] [ "" ]) (prefixes (name p))));
        norm = x: if builtins.isAttrs x then (x.spdxId or x.shortName or x.fullName or "unknown") else toString x;
        licenses = a: let l = (get a).meta.license or null; in
          if l == null then null else if builtins.isList l then map norm l else [ (norm l) ];
      in
        lib.genAttrs (builtins.attrNames lock.platforms.${arch}.paths)
          (p: let a = attrFor p; in { attr = a; license = if a == null then null else licenses a; });
  in {
    schema = 1;
    source = "nixpkgs meta.license of the attribute whose output path equals the store path; null where no attribute matched";
    nixpkgs_rev = catalog.nixpkgs.rev;
    platforms = { amd64 = perArch "amd64"; arm64 = perArch "arm64"; };
  }
'
