"""Fixed precompiler selection, checked against Go SelectBundle by source tests."""
BUNDLES = {
    name: {'name': name, 'directory': 'bundles/' + name,
           'lock': 'bundles/' + name + '/packages.lock.json',
           'inventory': 'docs/toolbox.md' if name == 'toolbox' else 'docs/' + name + '.md',
           'workflow': 'bundledock-' + name + '.yml',
           'artifact': 'resolved-tools' if name == 'toolbox' else 'resolved-' + name,
           'repository': 'node' if name == 'node-current' else name,
           'aliases': 'current' if name == 'node-current' else 'stable lts latest' if name == 'node' else 'stable latest',
           'proposal_lock': 'packages.lock.json' if name == 'toolbox' else name + '.lock.json'}
    for name in ('toolbox', 'code-server', 'node', 'node-current', 'bun', 'codex', 'agy', 'claude')
}


def select_bundle(name):
    if name not in BUNDLES:
        raise ValueError('Bundle must be toolbox, code-server, node, node-current, bun, codex, agy or claude')
    return dict(BUNDLES[name])

if __name__ == '__main__':
    import argparse
    parser=argparse.ArgumentParser()
    parser.add_argument('--repository', required=True, choices=BUNDLES)
    print(select_bundle(parser.parse_args().repository)['repository'])
