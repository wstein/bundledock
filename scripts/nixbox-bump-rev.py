#!/usr/bin/env python3
"""Resolve the head of the nixpkgs-unstable channel and write it into the nixbox catalog.

  nixbox-bump-rev.py latest CATALOG           print "REV URL" if newer, nothing if current
  nixbox-bump-rev.py set CATALOG REV URL HASH write nixpkgs rev, url and narHash
"""
import json
import re
import sys
import urllib.error
import urllib.request
from pathlib import Path

CHANNEL = 'https://channels.nixos.org/nixpkgs-unstable'


class _NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, *args, **kwargs):
        return None


def fetch(url):
    """Body of url, or for the channel the redirect target (the release directory)."""
    opener = urllib.request.build_opener(_NoRedirect)
    try:
        with opener.open(url, timeout=60) as response:
            return response.read().decode()
    except urllib.error.HTTPError as error:
        if url == CHANNEL and error.code in (301, 302, 303, 307, 308):
            return error.headers['Location']
        raise


def latest(get=fetch):
    release = get(CHANNEL).rstrip('/')
    rev = get(release + '/git-revision').strip()
    if not re.fullmatch(r'[0-9a-f]{40}', rev):
        raise SystemExit(f'Unexpected git-revision from {release}: {rev!r}')
    return rev, release + '/nixexprs.tar.zst'


def pending(catalog_text, get=fetch):
    rev, url = latest(get)
    return None if rev == json.loads(catalog_text)['nixpkgs']['rev'] else (rev, url)


def set_pin(text, rev, url, nar_hash):
    """Replace the three nixpkgs values in place; the rest of the file is untouched."""
    for key, value in (('rev', rev), ('url', url), ('narHash', nar_hash)):
        text, count = re.subn(r'("%s"\s*:\s*")[^"]*(")' % key, lambda m: m.group(1) + value + m.group(2), text, count=1)
        if count != 1:
            raise SystemExit(f'catalog has no "{key}"')
    return text


def main(argv):
    if len(argv) == 3 and argv[1] == 'latest':
        found = pending(Path(argv[2]).read_text())
        if found:
            print(*found)
        return 0
    if len(argv) == 6 and argv[1] == 'set':
        path = Path(argv[2])
        path.write_text(set_pin(path.read_text(), *argv[3:]))
        return 0
    raise SystemExit(__doc__)


if __name__ == '__main__':
    sys.exit(main(sys.argv))
