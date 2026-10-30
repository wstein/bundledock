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
    for name in ('toolbox', 'code-server', 'node', 'node-current', 'bun', 'codex', 'agy', 'claude', 'opentofu', 'nixbox')
}
# nixbox is published only when the owner selects it in a manual dispatch of Publish
# bundles. No updater pull request or CI completion ever publishes it: it has its own
# upgrade proposal topic, which the automatic publication path does not recognize.
MANUAL_ONLY = ('nixbox',)
AUTOMATIC = tuple(name for name in BUNDLES if name not in MANUAL_ONLY)
# Bundles tagged <date>-b<N>, with no upstream version in the tag.
DATED = ('toolbox', 'nixbox')


def select_bundle(name):
    if name not in BUNDLES:
        raise ValueError('Bundle must be toolbox, code-server, node, node-current, bun, codex, agy, claude, opentofu or nixbox')
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
