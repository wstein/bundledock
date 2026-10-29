#!/usr/bin/env python3
"""Check that a refreshed nixbox lock and catalog keep the same package list.

An updater may move the nixpkgs pin: the catalog fields nixpkgs.rev,
nixpkgs.narHash and the nixpkgs.url of that release (which names the revision),
and with them the store paths, narHash values and signatures of the lock. It
must not add or remove packages, change attributes, providers, shims, the
platforms, the cache URL or the signing keys. Those changes need explicit owner
approval (AGENTS.md, class "distribution binary cache") and are never proposed.

Usage: nixbox-lock-compat.py OLD_LOCK NEW_LOCK [OLD_CATALOG NEW_CATALOG]
Without catalogs the catalog must be unchanged (equal lock catalog digest).
"""
import json
import re
import sys
from collections import Counter
from pathlib import Path


def package(path):
    """Store path name without hash and version: /nix/store/<hash>-git-minimal-2.56.0 -> git-minimal."""
    name = path.rsplit('/', 1)[-1].split('-', 1)[1]
    return re.sub(r'-\d.*$', '', name)


def keys(platform):
    return {sig.split(':', 1)[0] for info in platform['paths'].values() for sig in info.get('signatures', [])}


PIN_FIELDS = ('rev', 'narHash', 'url')
REV = re.compile(r'^[0-9a-f]{40}$')
SRI = re.compile(r'^sha256-[A-Za-z0-9+/]{43}=$')
URL_PREFIX = 'https://releases.nixos.org/nixpkgs/'


def compare_catalog(old, new):
    """Only the nixpkgs pin may differ; everything else must be identical."""
    def rest(catalog):
        copy = json.loads(json.dumps(catalog))
        pin = copy.get('nixpkgs')
        if isinstance(pin, dict):
            for field in PIN_FIELDS:
                pin.pop(field, None)
        return copy
    problems = []
    if rest(old) != rest(new):
        problems.append('catalog changed beyond nixpkgs rev and narHash (package, attribute, provider, shim, '
                        'platform, cache URL, public key or other field)')
    pin = new.get('nixpkgs') if isinstance(new.get('nixpkgs'), dict) else {}
    before = old.get('nixpkgs') if isinstance(old.get('nixpkgs'), dict) else {}
    if any(pin.get(f) != before.get(f) for f in PIN_FIELDS):
        rev = pin.get('rev', '')
        if not isinstance(rev, str) or not REV.match(rev):
            problems.append('nixpkgs rev is not a 40 digit commit hash')
        elif not (isinstance(pin.get('url'), str) and pin['url'].startswith(URL_PREFIX) and rev[:12] in pin['url']):
            problems.append(f'nixpkgs url is not a {URL_PREFIX} release of the new rev')
        if not isinstance(pin.get('narHash'), str) or not SRI.match(pin['narHash']):
            problems.append('nixpkgs narHash is not an SRI sha256 hash')
    return problems


def compare(old, new, old_catalog=None, new_catalog=None):
    problems = []
    if old.get('schema') != new.get('schema'):
        problems.append('lock schema changed')
    if old_catalog is None or new_catalog is None:
        if old.get('catalog_digest') != new.get('catalog_digest'):
            problems.append('catalog changed (nixpkgs revision pin, cache, key, package or placeholder list)')
    else:
        problems += compare_catalog(old_catalog, new_catalog)
    if set(old['platforms']) != set(new['platforms']):
        problems.append('platform set changed')
        return problems
    for arch in sorted(old['platforms']):
        before, after = old['platforms'][arch], new['platforms'][arch]
        if before.get('system') != after.get('system'):
            problems.append(f'{arch}: platform system changed')
        if set(before['roots']) != set(after['roots']) or \
                Counter(map(package, before['paths'])) != Counter(map(package, after['paths'])) or \
                {k: package(v) for k, v in before['roots'].items()} != {k: package(v) for k, v in after['roots'].items()}:
            gone = sorted((Counter(map(package, before['paths'])) - Counter(map(package, after['paths']))).elements())
            added = sorted((Counter(map(package, after['paths'])) - Counter(map(package, before['paths']))).elements())
            problems.append(f'{arch}: package list changed (removed {gone or "none"}, added {added or "none"})')
        if not keys(after) <= keys(before):
            problems.append(f'{arch}: signing key changed ({sorted(keys(after) - keys(before))})')
        unsigned = sorted(p for p, info in after['paths'].items() if not info.get('signatures'))
        if unsigned:
            problems.append(f'{arch}: {len(unsigned)} paths without a signature')
    return problems


def main(argv):
    if len(argv) not in (3, 5):
        raise SystemExit('Usage: nixbox-lock-compat.py OLD_LOCK NEW_LOCK [OLD_CATALOG NEW_CATALOG]')
    loaded = [json.loads(Path(p).read_text()) for p in argv[1:]]
    problems = compare(*loaded)
    if problems:
        print('Refusing nixbox proposal: ' + '; '.join(problems) + '. Any change to the package list or the catalog beyond the nixpkgs pin needs '
              'explicit owner approval and cannot come from an updater pull request.', file=sys.stderr)
        return 1
    return 0


if __name__ == '__main__':
    sys.exit(main(sys.argv))
