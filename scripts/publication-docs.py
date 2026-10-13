#!/usr/bin/env python3
"""Propose managed publication Markdown and metadata from a verified receipt; never build docs."""
import argparse
import importlib.util
from datetime import datetime
import json
import os
from pathlib import Path
import re
import subprocess
import sys

REPOSITORY = 'wstein/bundledock'
SOURCE = 'https://github.com/' + REPOSITORY
_SPEC = importlib.util.spec_from_file_location('bundle_config', Path(__file__).with_name('bundle-config.py'))
_CONFIG = importlib.util.module_from_spec(_SPEC)
_SPEC.loader.exec_module(_CONFIG)
_TAG_SPEC = importlib.util.spec_from_file_location('bundle_git_tags', Path(__file__).with_name('bundle-git-tags.py'))
_TAGS = importlib.util.module_from_spec(_TAG_SPEC)
_TAG_SPEC.loader.exec_module(_TAGS)
BUNDLES = {name: _CONFIG.select_bundle(name)['inventory'] for name in _CONFIG.BUNDLES}
BEGIN = '<!-- BEGIN PUBLISHED BUNDLE -->'
END = '<!-- END PUBLISHED BUNDLE -->'
SHA = r'[0-9a-f]{40}'
DIGEST = r'sha256:[0-9a-f]{64}'
TAG = r'(\d{4}\.\d{2}\.\d{2})-b([1-9]\d*)\.([1-9]\d*)'
FIELDS = ('image', 'tag', 'commit', 'source', 'created', 'lock_sha256', 'run_id')


def require(condition, message):
    if not condition:
        raise ValueError(message)


def validate(receipt, bundle, image, sha, run_id, attempt):
    require(bundle in BUNDLES, 'Unknown bundle')
    require(re.fullmatch(r'quay\.io/[a-z0-9]+(?:[._-][a-z0-9]+)*/' + re.escape(_CONFIG.select_bundle(bundle)['repository']), image or ''), 'Invalid expected image')
    require(re.fullmatch(SHA, sha or '') and re.fullmatch(r'[1-9]\d*', run_id or '') and re.fullmatch(r'[1-9]\d*', attempt or ''), 'Invalid expected provenance')
    require(isinstance(receipt, dict), 'Receipt must be an object')
    require(receipt.get('image') == image and receipt.get('commit') == sha and receipt.get('source') == SOURCE and receipt.get('run_id') == run_id, 'Wrong publication provenance')
    tag = receipt.get('tag', '')
    require(isinstance(tag, str), 'Invalid publication version')
    version = _TAGS.validate_version(tag, run_id)
    semantic = _TAGS.SINGLETON.fullmatch(tag) is not None
    required_identity = bundle == 'node-current' or (bundle == 'node' and semantic)
    require(receipt.get('bundle', bundle) == bundle and (not required_identity or receipt.get('bundle') == bundle), 'Wrong source bundle identity')
    if semantic:
        require(bundle != 'toolbox' and receipt.get('run_attempt') == attempt, 'Wrong singleton publication attempt')
    else:
        require(str(version['attempt']) == attempt, 'Wrong publication attempt')
    try:
        datetime.strptime(receipt.get('created', ''), '%Y-%m-%dT%H:%M:%SZ')
        if not semantic:
            datetime.strptime(tag[:10], '%Y.%m.%d')
    except (TypeError, ValueError) as error:
        raise ValueError('Invalid publication date') from error
    require(isinstance(receipt.get('lock_sha256'), str) and re.fullmatch(r'[0-9a-f]{64}', receipt['lock_sha256']), 'Invalid lock SHA256')
    digest = receipt.get('digest', '')
    require(isinstance(digest, str) and re.fullmatch(DIGEST, digest), 'Invalid index digest')
    require(receipt.get('reference') == image + '@' + digest, 'Reference must pin the index digest')
    platforms = receipt.get('platforms')
    require(isinstance(platforms, list) and len(platforms) == 2, 'Both platform receipts required')
    require({p.get('architecture') for p in platforms if isinstance(p, dict)} == {'amd64', 'arm64'}, 'Wrong platform set')
    fields = FIELDS + (('run_attempt',) if semantic else ()) + (('bundle',) if 'bundle' in receipt else ())
    for platform in platforms:
        require(all(platform.get(key) == receipt[key] for key in fields), 'Platform provenance differs')
        require(platform.get('smoked') is True, 'Platform smoke missing')
        for key in ('image_id', 'digest'):
            require(isinstance(platform.get(key), str) and re.fullmatch(DIGEST, platform[key]), 'Invalid platform digest')
    return {**{key: receipt[key] for key in fields}, 'digest': digest, 'reference': receipt['reference'], 'attempt': attempt}


def version_key(tag):
    if _TAGS.SINGLETON.fullmatch(tag):
        upstream, revision = tag.split('-b')
        return (1, *map(int, upstream.split('.')), int(revision))
    match = re.fullmatch(TAG, tag)
    require(match is not None, 'Invalid previous publication version')
    return (0, match[1], int(match[2]), int(match[3]))


def render(record):
    run = SOURCE + '/actions/runs/' + record['run_id'] + '/attempts/' + record['attempt']
    return ('\n<!-- publication-record: ' + json.dumps(record, sort_keys=True, separators=(',', ':')) + ' -->\n'
        '## Published bundle\n\n'
        'Version: `' + record['tag'] + '`\n\n'
        'Index digest: `' + record['digest'] + '`\n\n'
        'Source: [`' + record['commit'] + '`](' + SOURCE + '/commit/' + record['commit'] + ')\n\n'
        'Publication: [run ' + record['run_id'] + ', attempt ' + record['attempt'] + '](' + run + ')\n\n'
        'Pull this published carrier by its immutable index digest:\n\n```sh\n'
        'docker pull ' + record['reference'] + '\n```\n\n'
        'The source inventory below describes the current lock; this publication retains its exact source revision.\n')


def replacement(source, record):
    require(source.count(BEGIN) == 1 and source.count(END) == 1, 'Expected exactly one publication marker pair')
    prefix, rest = source.split(BEGIN)
    managed, suffix = rest.split(END)
    previous = re.search(r'<!-- publication-record: (.+) -->', managed)
    if previous:
        old = json.loads(previous[1])
        require(version_key(old['tag']) <= version_key(record['tag']), 'Refusing older publication')
        require(old['tag'] != record['tag'] or all(record.get(key) == value for key, value in old.items()), 'Same version has conflicting publication record')
    return prefix + BEGIN + render(record) + END + suffix


def metadata_path(bundle):
    require(bundle in BUNDLES, 'Unknown bundle')
    return 'docs/' + bundle + '-published.json'


def metadata(bundle, record, previous='{}', evidence=None):
    # Historical date tags do not encode an upstream version. Preserve an
    # independently verified value only for the exact same published identity.
    upstream = record['tag'].split('-b')[0] if _TAGS.SINGLETON.fullmatch(record['tag']) else 'unknown'
    old = json.loads(previous)
    if upstream == 'unknown' and isinstance(old, dict) and all(old.get(key) == record[key] for key in ('tag', 'digest', 'commit', 'image')):
        candidate = old.get('upstream_version', '')
        if isinstance(candidate, str) and re.fullmatch(r'\d+\.\d+\.\d+', candidate):
            upstream = candidate
    result = {key: record[key] for key in ('tag', 'digest', 'commit', 'image', 'run_id')}
    result.update(bundle=bundle, upstream_version=upstream)
    if evidence is not None:
        result['publication_evidence'] = evidence
    elif all(old.get(key) == record[key] for key in ('tag', 'digest', 'commit', 'image', 'run_id')) and 'publication_evidence' in old:
        result['publication_evidence'] = old['publication_evidence']
    return json.dumps(result, sort_keys=True, indent=2) + '\n'


def update(root, bundle, record, evidence=None):
    root = root.resolve(strict=True)
    paths = [BUNDLES[bundle], metadata_path(bundle)]
    changes = []
    for relative in paths:
        path = root / relative
        require(not path.is_symlink() and not path.parent.is_symlink(), 'Linked documentation path forbidden')
        require(path.resolve().is_relative_to(root), 'Documentation path escapes repository')
        source = path.read_bytes().decode('utf-8') if path.exists() else '{}'
        result = replacement(source, record) if relative == BUNDLES[bundle] else metadata(bundle, record, source, evidence)
        if result != source:
            changes.append((path, result))
    for path, result in changes:
        path.write_bytes(result.encode('utf-8'))
    return bool(changes)


def command(*args):
    return subprocess.check_output(args, text=True).strip()


def source_at(revision, path):
    return subprocess.check_output(['git', 'show', revision + ':' + path]).decode('utf-8')


def api(path, payload=None):
    args = ['gh', 'api', 'repos/' + REPOSITORY + '/' + path]
    if payload is not None:
        args.extend(['--method', 'POST', '--input', '-'])
    result = subprocess.run(args, input=json.dumps(payload) if payload is not None else None,
                            text=True, capture_output=True, check=True)
    return json.loads(result.stdout)


def propose(root, bundle, record, evidence=None):
    os.chdir(root)
    require(command('git', 'status', '--porcelain') == '', 'Working tree must be clean')
    require(command('gh', 'api', 'repos/' + REPOSITORY, '--jq', '.default_branch') == 'main', 'Wrong default branch')
    require(command('git', 'remote', 'get-url', 'origin') in
            (SOURCE, SOURCE + '.git', 'git@github.com:' + REPOSITORY + '.git'), 'Wrong origin repository')
    command('git', 'fetch', 'origin', 'main')
    base = command('git', 'rev-parse', 'origin/main')
    branch = 'publication-docs/' + bundle + '/' + record['tag']
    path = BUNDLES[bundle]
    source = source_at(base, path)
    desired = replacement(source, record)
    changes = []
    if desired != source:
        changes.append({'path': path, 'mode': '100644', 'type': 'blob', 'content': desired})
    receipt_path = metadata_path(bundle)
    # New bundle metadata may not exist on the base yet; list the exact
    # tree first rather than swallowing a failed Git read.
    exists = command('git', 'ls-tree', '--name-only', base, '--', receipt_path)
    old_metadata = source_at(base, receipt_path) if exists == receipt_path else '{}'
    desired_metadata = metadata(bundle, record, old_metadata, evidence)
    if desired_metadata != old_metadata:
        changes.append({'path': receipt_path, 'mode': '100644', 'type': 'blob', 'content': desired_metadata})
    if not changes:
        return 'Publication source already current'
    tree = api('git/commits/' + base)['tree']['sha']
    require(re.fullmatch(SHA, tree), 'Invalid base tree')
    tree = api('git/trees', {'base_tree': tree, 'tree': changes})['sha']
    require(re.fullmatch(SHA, tree), 'Invalid new tree')
    head = api('git/commits', {'message': 'docs(' + bundle + '): record publication ' + record['tag'] + '\n\nKeep consumer guidance tied to the verified published index and exact source revision.',
                             'tree': tree, 'parents': [base]})['sha']
    require(re.fullmatch(SHA, head), 'Invalid new commit')
    require(command('git', 'ls-remote', 'origin', 'refs/heads/main').split()[0] == base, 'Main moved; retry on fresh base')
    try:
        api('git/refs', {'ref': 'refs/heads/' + branch, 'sha': head})
    except subprocess.CalledProcessError as error:
        # A creation race is never a reason to update a branch. Only the exact
        # expected commit with this fresh parent/tree can reuse a topic.
        require('HTTP 422' in (error.stderr or ''), 'Ref creation failed')
        existing = api('git/ref/heads/' + branch)['object']['sha']
        commit = api('git/commits/' + existing)
        require(commit['tree']['sha'] == tree and
                [parent['sha'] for parent in commit['parents']] == [base],
                'Existing topic conflicts with publication source or base')
        head = existing
    require(api('git/ref/heads/' + branch)['object']['sha'] == head, 'Topic moved before PR creation')
    require(command('git', 'ls-remote', 'origin', 'refs/heads/main').split()[0] == base, 'Main moved before PR creation')
    prs = json.loads(command('gh', 'pr', 'list', '--repo', REPOSITORY, '--state', 'open', '--head', branch,
                             '--json', 'number,headRefName,headRefOid,baseRefName,isCrossRepository'))
    require(len(prs) <= 1, 'Ambiguous existing PRs')
    if prs:
        require(prs[0]['headRefName'] == branch and prs[0]['headRefOid'] == head and
                prs[0]['baseRefName'] == 'main' and not prs[0]['isCrossRepository'], 'Wrong existing PR target')
        result = 'Existing identical publication PR'
    else:
        result = command('gh', 'pr', 'create', '--repo', REPOSITORY, '--base', 'main', '--head', branch,
            '--title', 'docs(' + bundle + '): record publication ' + record['tag'], '--body',
            'Record the verified published version, index digest, exact source and workflow run in the managed bundle guide. This changes Markdown and publication metadata source only; no documentation build or consumer activation is performed.')
    require(api('git/ref/heads/' + branch)['object']['sha'] == head, 'Topic moved before source CI')
    command('gh', 'workflow', 'run', 'ci.yml', '--repo', REPOSITORY, '--ref', branch,
            '-f', 'expected_head_sha=' + head)
    return result


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--receipt', type=Path, required=True)
    parser.add_argument('--bundle', choices=BUNDLES, required=True)
    parser.add_argument('--image', required=True)
    parser.add_argument('--sha', required=True)
    parser.add_argument('--run-id', required=True)
    parser.add_argument('--attempt', required=True)
    parser.add_argument('--root', type=Path, default=Path('.'))
    parser.add_argument('--propose', action='store_true')
    args = parser.parse_args()
    try:
        require(args.receipt.stat().st_size <= 65536, 'Oversized receipt')
        receipt = json.loads(args.receipt.read_text())
        record = validate(receipt, args.bundle, args.image, args.sha, args.run_id, args.attempt)
        evidence = None
        if args.propose:
            spec = importlib.util.spec_from_file_location('publication_reconcile', Path(__file__).with_name('publication-reconcile.py'))
            reconciliation = importlib.util.module_from_spec(spec)
            spec.loader.exec_module(reconciliation)
            try:
                evidence = reconciliation.capture_evidence(args.bundle, args.sha, args.run_id, args.attempt, receipt, reconciliation.GitHub())
            except reconciliation.Pending as error:
                raise ValueError('Required publication evidence unavailable') from error
        result = propose(args.root.resolve(), args.bundle, record, evidence) if args.propose else update(args.root, args.bundle, record)
        print(result)
    except (OSError, ValueError, KeyError, TypeError, subprocess.CalledProcessError) as error:
        print('publication docs: ' + str(error), file=sys.stderr)
        return 1
    return 0


if __name__ == '__main__':
    sys.exit(main())
