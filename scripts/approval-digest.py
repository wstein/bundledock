#!/usr/bin/env python3
"""Print the approved_digest of a source-build tool for the owner to commit as lock.approvals.<tool>.

Run it only after the approved real build and smoke of the exact revision checked out here. It reads the
bundle's packages.catalog.json and packages.lock.json from the working tree with the publish gate's strict
parser and digest, plus the file inputs listed in APPROVAL_INPUT_FILES; it changes nothing."""
import argparse
import importlib.util
import sys
from pathlib import Path
sys.dont_write_bytecode = True
spec = importlib.util.spec_from_file_location('gate', Path(__file__).with_name('bundle-publication.py'))
gate = importlib.util.module_from_spec(spec)
spec.loader.exec_module(gate)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--root', type=Path, default=Path('.'))
    parser.add_argument('--bundle', default='toolbox')
    parser.add_argument('tool', nargs='?')
    parser.add_argument('--list-inputs', action='store_true', help='print the closed list of file inputs and exit')
    args = parser.parse_args()
    if args.list_inputs:
        print('\n'.join(gate.APPROVAL_INPUT_FILES + tuple(d + '/*.go (not *_test.go)' for d in gate.APPROVAL_INPUT_DIRECTORIES) + tuple('per tool: ' + kind for kind in gate.APPROVAL_INPUT_KINDS)))
        return 0
    if not args.tool:
        parser.error('a tool name or --list-inputs is required')
    bundle = gate.config.select_bundle(args.bundle)
    try:
        lock = gate.parse_lock((args.root / bundle['lock']).read_bytes(), 'lock')
        catalog = gate.parse_catalog((args.root / bundle['directory'] / 'packages.catalog.json').read_bytes(), 'catalog')
        package = (catalog.get('packages') or {}).get(args.tool)
        gate.require(package is not None and package.get('provider') == 'source-build', args.tool + ' is not a source-build tool of ' + args.bundle)
        read = lambda path: (args.root / path).read_bytes()
        names = lambda directory: [entry.name for entry in (args.root / directory).iterdir() if entry.is_file()]
        print(gate.approval_digest(args.tool, package, lock, gate.approval_inputs(read, names, bundle, args.tool, package)))
    except (OSError, ValueError) as error:
        print('approval digest: ' + str(error), file=sys.stderr)
        return 1
    return 0


if __name__ == '__main__':
    sys.exit(main())
