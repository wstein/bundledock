#!/usr/bin/env python3
"""Print the attempt of this run's publication receipt: the current attempt's, else the only one."""
import re
import sys
from pathlib import Path


def main(directory, bundle, current):
    found = {}
    for path in Path(directory).iterdir():
        match = re.fullmatch(re.escape(bundle) + r'-publication-([1-9][0-9]*)', path.name)
        if match and (path / 'publication.json').is_file() and not path.is_symlink() and not (path / 'publication.json').is_symlink():
            found[match[1]] = path
    # A re-run of failed jobs has a new attempt but keeps the earlier receipt.
    attempt = current if current in found else next(iter(found)) if len(found) == 1 else None
    if attempt is None:
        print('Expected the current attempt or exactly one publication receipt of this run', file=sys.stderr)
        return 1
    print(attempt)
    return 0


if __name__ == '__main__':
    sys.exit(main(*sys.argv[1:4]) if len(sys.argv) == 4 else 2)
