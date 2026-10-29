#!/usr/bin/env python3
"""Check that a refreshed nixbox lock keeps the same package list.

An updater may change the nixpkgs revision, store paths, narHash values and
signatures. It must not add or remove packages, change the catalog, the
platforms or the signing keys. Those changes need explicit owner approval
(AGENTS.md, class "distribution binary cache") and are never proposed.
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


def compare(old, new):
    problems = []
    if old.get('schema') != new.get('schema'):
        problems.append('lock schema changed')
    if old.get('catalog_digest') != new.get('catalog_digest'):
        problems.append('catalog changed (nixpkgs revision pin, cache, key, package or placeholder list)')
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
    if len(argv) != 3:
        raise SystemExit('Usage: nixbox-lock-compat.py OLD_LOCK NEW_LOCK')
    old, new = (json.loads(Path(p).read_text()) for p in argv[1:])
    problems = compare(old, new)
    if problems:
        print('Refusing nixbox proposal: ' + '; '.join(problems) + '. Any change to the package list needs '
              'explicit owner approval and cannot come from an updater pull request.', file=sys.stderr)
        return 1
    return 0


if __name__ == '__main__':
    sys.exit(main(sys.argv))
