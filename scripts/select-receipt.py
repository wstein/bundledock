#!/usr/bin/env python3
"""Print the attempt and the path of this run's publication receipt: the current attempt's, else the only one."""
import json
import re
import sys
from pathlib import Path


def plain(path):
    return path.is_file() and not path.is_symlink()


def main(directory, bundle, current):
    found = {}
    # One matched artifact is extracted straight into the directory, several
    # into one folder each.
    flat = Path(directory) / 'publication.json'
    if plain(flat):
        attempt = str(json.loads(flat.read_text()).get('run_attempt', ''))
        if not re.fullmatch(r'[1-9][0-9]*', attempt):
            print('The publication receipt has no valid run_attempt', file=sys.stderr)
            return 1
        print(attempt)
        print(flat)
        return 0
    for path in Path(directory).iterdir():
        match = re.fullmatch(re.escape(bundle) + r'-publication-([1-9][0-9]*)', path.name)
        if match and plain(path / 'publication.json') and not path.is_symlink():
            found[match[1]] = path
    # A re-run of failed jobs has a new attempt but keeps the earlier receipt.
    attempt = current if current in found else next(iter(found)) if len(found) == 1 else None
    if attempt is None:
        print('Expected the current attempt or exactly one publication receipt of this run', file=sys.stderr)
        return 1
    print(attempt)
    print(found[attempt] / 'publication.json')
    return 0


if __name__ == '__main__':
    sys.exit(main(*sys.argv[1:4]) if len(sys.argv) == 4 else 2)
