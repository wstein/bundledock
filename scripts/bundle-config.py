"""Fixed precompiler selection, checked against Go SelectBundle by source tests."""
BUNDLES = {
    name: {'name': name, 'directory': 'bundles/' + name,
           'lock': 'bundles/' + name + '/tools.lock.json',
           'inventory': 'docs/tools.md' if name == 'toolbox' else 'docs/' + name + '.md',
           'workflow': 'bundledock-' + name + '.yml',
           'artifact': 'resolved-tools' if name == 'toolbox' else 'resolved-' + name,
           'proposal_lock': 'tools.lock.json' if name == 'toolbox' else name + '.lock.json'}
    for name in ('toolbox', 'code-server', 'node', 'bun')
}


def select_bundle(name):
    if name not in BUNDLES:
        raise ValueError('Bundle must be toolbox, code-server, node or bun')
    return dict(BUNDLES[name])
