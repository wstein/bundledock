#!/usr/bin/env python3
"""Read-only list of bundles whose lock on main has no verified publication.

A bundle counts as published when its lock bytes on main equal the lock at the
source commit of the recorded publication (docs/<bundle>-published.json) or of
one of its annotated source tags. Nothing is dispatched, built, tagged,
written or merged.
"""
import argparse
import base64
import importlib.util
import json
from pathlib import Path
import sys

sys.dont_write_bytecode = True


def load(name, filename):
    spec = importlib.util.spec_from_file_location(name, Path(__file__).with_name(filename))
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


reconcile = load('publication_reconcile', 'publication-reconcile.py')
approval = reconcile.approval
require = approval.require
Pending = reconcile.Pending
TAG_LIMIT = 10


def metadata_commit(api, bundle):
    path = 'docs/' + bundle + '-published.json'
    names = {item.get('name') for item in api.get('/contents/docs?ref=main')}
    if bundle + '-published.json' not in names:
        return None
    file = api.get('/contents/' + path + '?ref=main')
    require(file.get('type') == 'file' and file.get('encoding') == 'base64' and len(file.get('content', '')) <= 90000,
            'Invalid tracked publication metadata')
    data = json.loads(base64.b64decode(''.join(file['content'].split()), validate=True))
    require(isinstance(data, dict) and data.get('bundle') == bundle, 'Metadata belongs to another bundle')
    return approval.sha(data['commit'])


def tag_commits(api, bundle):
    refs = api.get('/git/matching-refs/tags/' + bundle + '/')
    require(isinstance(refs, list), 'Invalid tag listing')
    commits = []
    for ref in sorted(refs, key=lambda item: item['ref'], reverse=True)[:TAG_LIMIT]:
        obj = ref['object']
        if obj['type'] == 'tag':
            obj = api.get('/git/tags/' + approval.sha(obj['sha']))['object']
        if obj['type'] == 'commit':
            commits.append(approval.sha(obj['sha']))
    return commits


def inspect(bundle, api):
    selection = approval.config.select_bundle(bundle)
    head = approval.sha(api.get('/git/ref/heads/main')['object']['sha'])
    current = approval.lock_bytes(api.get, selection, head)
    candidates = []
    recorded = metadata_commit(api, bundle)
    if recorded:
        candidates.append(recorded)
    candidates.extend(commit for commit in tag_commits(api, bundle) if commit not in candidates)
    result = {'bundle': bundle, 'main': head}
    if not candidates:
        return dict(result, state='no-baseline')
    for commit in candidates:
        try:
            published = approval.lock_bytes(api.get, selection, commit)
        except Pending:
            # Historical tag commits may predate this lock path. The current
            # lock and main were read above, so an outage would already be pending.
            continue
        if published == current:
            return dict(result, state='published', source_sha=commit)
    latest = api.get('/commits?path=' + selection['lock'] + '&sha=' + head + '&per_page=1')
    require(isinstance(latest, list) and len(latest) == 1, 'Lock history unavailable')
    source = approval.sha(latest[0]['sha'])
    pulls = api.get('/commits/' + source + '/pulls?per_page=100&page=1')
    numbers = [item['number'] for item in pulls] if isinstance(pulls, list) else []
    return dict(result, state='unpublished', lock_commit=source, pull_requests=numbers)


def survey(bundles, api):
    results = []
    for bundle in bundles:
        try:
            results.append(inspect(bundle, api))
        except Pending:
            results.append({'bundle': bundle, 'state': 'pending'})
    return results


def exit_code(results):
    return 0 if all(item['state'] == 'published' for item in results) else 2


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--bundle', action='append', choices=list(approval.config.BUNDLES))
    args = parser.parse_args()
    try:
        results = survey(args.bundle or list(approval.config.BUNDLES), reconcile.GitHub())
    except (ValueError, KeyError, TypeError):
        print(json.dumps({'state': 'rejected', 'reason': 'Evidence contradicts expected publication shape'}))
        return 1
    for item in results:
        print(json.dumps(item, sort_keys=True))
    return exit_code(results)


if __name__ == '__main__':
    sys.exit(main())
