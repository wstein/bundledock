"""Fixed precompiler selection, checked against Go SelectBundle by source tests."""
BUNDLES = {
    name: {'name': name, 'directory': 'bundles/' + name,
           'lock': 'bundles/' + name + '/packages.lock.json',
           'inventory': 'docs/toolbox.md' if name == 'toolbox' else 'docs/' + name + '.md',
           'workflow': 'publish-bundles.yml',
           'artifact': 'resolved-tools' if name == 'toolbox' else 'resolved-' + name,
           'repository': 'node' if name == 'node-current' else name,
           'aliases': 'current' if name == 'node-current' else 'stable lts latest' if name == 'node' else 'stable latest',
           'proposal_lock': 'packages.lock.json' if name == 'toolbox' else name + '.lock.json'}
    for name in ('toolbox', 'code-server', 'node', 'node-current', 'bun', 'codex', 'agy', 'claude', 'opentofu')
}


def select_bundle(name):
    if name not in BUNDLES:
        raise ValueError('Bundle must be toolbox, code-server, node, node-current, bun, codex, agy, claude or opentofu')
    return dict(BUNDLES[name])

if __name__ == '__main__':
    import argparse
    parser=argparse.ArgumentParser()
    import json
    selection = parser.add_mutually_exclusive_group(required=True)
    selection.add_argument('--repository', choices=BUNDLES)
    selection.add_argument('--upgrade-selection', choices=('all', *BUNDLES))
    args = parser.parse_args()
    if args.repository:
        print(select_bundle(args.repository)['repository'])
    else:
        print(json.dumps(list(BUNDLES) if args.upgrade_selection == 'all' else [args.upgrade_selection]))
