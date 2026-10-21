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
QUICKSTART_BEGIN = '<!-- BEGIN PUBLISHED QUICKSTART -->'
QUICKSTART_END = '<!-- END PUBLISHED QUICKSTART -->'
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
    daily = _TAGS.DAILY.fullmatch(tag) is not None
    semantic = not daily and _TAGS.SINGLETON.fullmatch(tag) is not None
    explicit_attempt = daily or semantic
    required_identity = bundle == 'node-current' or (bundle == 'node' and semantic)
    require(receipt.get('bundle', bundle) == bundle and (not required_identity or receipt.get('bundle') == bundle), 'Wrong source bundle identity')
    if explicit_attempt:
        require((bundle == 'toolbox' if daily else bundle != 'toolbox') and receipt.get('run_attempt') == attempt, 'Wrong publication attempt')
    else:
        require(str(version['attempt']) == attempt, 'Wrong publication attempt')
    try:
        datetime.strptime(receipt.get('created', ''), '%Y-%m-%dT%H:%M:%SZ')
        if not semantic:
            datetime.strptime(tag[:10], '%Y.%m.%d')
    except (TypeError, ValueError) as error:
        raise ValueError('Invalid publication date') from error
    if daily:
        require(tag[:10].replace('.', '-') == receipt['created'][:10], 'Publication date differs from tag')
    require(isinstance(receipt.get('lock_sha256'), str) and re.fullmatch(r'[0-9a-f]{64}', receipt['lock_sha256']), 'Invalid lock SHA256')
    digest = receipt.get('digest', '')
    require(isinstance(digest, str) and re.fullmatch(DIGEST, digest), 'Invalid index digest')
    require(receipt.get('reference') == image + '@' + digest, 'Reference must pin the index digest')
    platforms = receipt.get('platforms')
    require(isinstance(platforms, list) and len(platforms) == 2, 'Both platform receipts required')
    require({p.get('architecture') for p in platforms if isinstance(p, dict)} == {'amd64', 'arm64'}, 'Wrong platform set')
    fields = FIELDS + (('run_attempt',) if explicit_attempt else ()) + (('bundle',) if 'bundle' in receipt else ())
    for platform in platforms:
        require(all(platform.get(key) == receipt[key] for key in fields), 'Platform provenance differs')
        require(platform.get('smoked') is True, 'Platform smoke missing')
        for key in ('image_id', 'digest'):
            require(isinstance(platform.get(key), str) and re.fullmatch(DIGEST, platform[key]), 'Invalid platform digest')
    return {**{key: receipt[key] for key in fields}, 'digest': digest, 'reference': receipt['reference'], 'attempt': attempt}


def version_key(tag):
    daily = _TAGS.DAILY.fullmatch(tag)
    if daily:
        return (0, daily[1], 1, int(daily[2]), 0)
    if _TAGS.SINGLETON.fullmatch(tag):
        upstream, revision = tag.split('-b')
        return (1, *map(int, upstream.split('.')), int(revision))
    match = re.fullmatch(TAG, tag)
    require(match is not None, 'Invalid previous publication version')
    return (0, match[1], 0, int(match[2]), int(match[3]))


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


def render_quickstart(record):
    run = SOURCE + '/actions/runs/' + record['run_id'] + '/attempts/' + record['attempt']
    return ('\n```sh\n'
        'TOOLBOX_IMAGE=' + record['reference'] + '\n'
        'docker pull "$TOOLBOX_IMAGE"\n'
        'make bundle-volume BUNDLE=toolbox IMAGE="$TOOLBOX_IMAGE"\n'
        '```\n\n'
        'This example is toolbox version `' + record['tag'] + '`, published from\n'
        '[`' + record['commit'] + '`](' + SOURCE + '/commit/' + record['commit'] + ')\n'
        'in [publication run ' + record['run_id'] + ', attempt ' + record['attempt'] + '](' + run + ').\n'
        'The initializer prints `Volume: bundledock-toolbox-<full image ID>`; use that volume name below:\n')


def replacement_quickstart(source, record):
    require(source.count(QUICKSTART_BEGIN) == 1 and source.count(QUICKSTART_END) == 1,
            'Expected exactly one quickstart marker pair')
    prefix, rest = source.split(QUICKSTART_BEGIN)
    managed, suffix = rest.split(QUICKSTART_END)
    return prefix + QUICKSTART_BEGIN + render_quickstart(record) + QUICKSTART_END + suffix


def metadata_path(bundle):
    require(bundle in BUNDLES, 'Unknown bundle')
    return 'docs/' + bundle + '-published.json'


def metadata(bundle, record, previous='{}', evidence=None):
    # Historical date tags do not encode an upstream version. Preserve an
    # independently verified value only for the exact same published identity.
    upstream = record['tag'].split('-b')[0] if _TAGS.SINGLETON.fullmatch(record['tag']) and not _TAGS.DAILY.fullmatch(record['tag']) else 'unknown'
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
    if bundle == 'toolbox' and (root / 'README.md').exists():
        paths.append('README.md')
    changes = []
    for relative in paths:
        path = root / relative
        require(not path.is_symlink() and not path.parent.is_symlink(), 'Linked documentation path forbidden')
        require(path.resolve().is_relative_to(root), 'Documentation path escapes repository')
        source = path.read_bytes().decode('utf-8') if path.exists() else '{}'
        if relative == BUNDLES[bundle]:
            result = replacement(source, record)
        elif relative == 'README.md':
            result = replacement_quickstart(source, record)
        else:
            result = metadata(bundle, record, source, evidence)
        if result != source:
            changes.append((path, result))
    for path, result in changes:
        path.write_bytes(result.encode('utf-8'))
    return bool(changes)


def command(*args):
    return subprocess.check_output(args, text=True).strip()


def source_at(revision, path):
    return subprocess.check_output(['git', 'show', revision + ':' + path]).decode('utf-8')


def api(path, payload=None, method='POST'):
    args = ['gh', 'api', 'repos/' + REPOSITORY + '/' + path]
    if payload is not None:
        args.extend(['--method', method, '--input', '-'])
    result = subprocess.run(args, input=json.dumps(payload) if payload is not None else None,
                            text=True, capture_output=True, check=True)
    return json.loads(result.stdout)


# Every publication run adds its bundle to ONE rolling topic and pull request, so
# merging one docs PR never leaves another behind main. Workflow concurrency
# serialises the runs; the topic is rebuilt as a single commit on fresh main.
PENDING = 'publication-docs/pending'


def allowed_paths():
    return set(BUNDLES.values()) | {metadata_path(name) for name in BUNDLES} | {'README.md'}


def record_of(source):
    """The publication record inside the managed section, or None."""
    if source.count(BEGIN) != 1 or source.count(END) != 1:
        return None
    managed = source.split(BEGIN)[1].split(END)[0]
    found = re.search(r'<!-- publication-record: (.+) -->', managed)
    return json.loads(found[1]) if found else None


def check_record(record):
    require(isinstance(record, dict) and all(isinstance(record.get(key), str) for key in FIELDS + ('digest', 'reference', 'attempt')), 'Invalid pending record')
    require(re.fullmatch(SHA, record['commit']) and re.fullmatch(DIGEST, record['digest']) and
            re.fullmatch(r'[1-9]\d*', record['run_id']) and re.fullmatch(r'[1-9]\d*', record['attempt']) and
            record['source'] == SOURCE and record['reference'] == record['image'] + '@' + record['digest'], 'Invalid pending record')
    version_key(record['tag'])
    return record


def pending_entries(base, head):
    """Entries carried by the open topic: records that differ from main."""
    entries = {}
    for name, path in BUNDLES.items():
        record = record_of(source_at(head, path))
        if record is not None and record != record_of(source_at(base, path)):
            evidence = json.loads(source_at(head, metadata_path(name))).get('publication_evidence')
            entries[name] = (check_record(record), evidence)
    return entries


def merge_entry(entries, bundle, record, evidence):
    entries = dict(entries)
    if bundle in entries:
        old = entries[bundle][0]
        require(version_key(old['tag']) <= version_key(record['tag']), 'Refusing older publication')
        require(old['tag'] != record['tag'] or all(record.get(key) == value for key, value in old.items()), 'Same version has conflicting publication record')
    entries[bundle] = (record, evidence)
    return entries


def build_changes(base, entries):
    changes = []
    for name in BUNDLES:
        if name not in entries:
            continue
        record, evidence = entries[name]
        path = BUNDLES[name]
        source = source_at(base, path)
        desired = replacement(source, record)
        if desired != source:
            changes.append({'path': path, 'mode': '100644', 'type': 'blob', 'content': desired})
        if name == 'toolbox':
            readme = source_at(base, 'README.md')
            desired_readme = replacement_quickstart(readme, record)
            if desired_readme != readme:
                changes.append({'path': 'README.md', 'mode': '100644', 'type': 'blob', 'content': desired_readme})
        receipt_path = metadata_path(name)
        # New bundle metadata may not exist on the base yet; list the exact
        # tree first rather than swallowing a failed Git read.
        exists = command('git', 'ls-tree', '--name-only', base, '--', receipt_path)
        old_metadata = source_at(base, receipt_path) if exists == receipt_path else '{}'
        desired_metadata = metadata(name, record, old_metadata, evidence)
        if desired_metadata != old_metadata:
            changes.append({'path': receipt_path, 'mode': '100644', 'type': 'blob', 'content': desired_metadata})
    require(all(change['path'] in allowed_paths() for change in changes), 'Only publication Markdown and metadata may change')
    return changes


def describe(entries):
    names = [name for name in BUNDLES if name in entries]
    if len(names) == 1:
        tag = entries[names[0]][0]['tag']
        title = 'docs(' + names[0] + '): record publication ' + tag
    else:
        title = 'docs(publication): record ' + ', '.join(name + ' ' + entries[name][0]['tag'] for name in names)
    rows = ['| Bundle | Version | Index digest | Source | Run |', '| --- | --- | --- | --- | --- |']
    for name in names:
        record = entries[name][0]
        rows.append('| `' + name + '` | `' + record['tag'] + '` | `' + record['digest'] + '` | [`' + record['commit'][:12] + '`](' + SOURCE + '/commit/' + record['commit'] +
                    ') | [run ' + record['run_id'] + '](' + SOURCE + '/actions/runs/' + record['run_id'] + '/attempts/' + record['attempt'] + ') |')
    body = ('Record the verified published versions, index digests, exact sources and workflow runs in the managed bundle guides.\n\n' +
            '\n'.join(rows) + '\n\nThis rolling PR collects the publications since the last merge: each publication run rebuilds it as one commit on current main. ' +
            'It changes Markdown and publication metadata source only; no documentation build or consumer activation is performed.')
    return title, body


def ref_exists(branch):
    try:
        api('git/ref/heads/' + branch)
    except subprocess.CalledProcessError as error:
        require('404' in (error.stderr or ''), 'Topic lookup failed')
        return False
    return True


def propose(root, bundle, record, evidence=None):
    os.chdir(root)
    require(command('git', 'status', '--porcelain') == '', 'Working tree must be clean')
    require(command('gh', 'api', 'repos/' + REPOSITORY, '--jq', '.default_branch') == 'main', 'Wrong default branch')
    require(command('git', 'remote', 'get-url', 'origin') in
            (SOURCE, SOURCE + '.git', 'git@github.com:' + REPOSITORY + '.git'), 'Wrong origin repository')
    command('git', 'fetch', 'origin', 'main')
    base = command('git', 'rev-parse', 'origin/main')
    branch = PENDING
    prs = json.loads(command('gh', 'pr', 'list', '--repo', REPOSITORY, '--state', 'open', '--head', branch,
                             '--json', 'number,headRefName,headRefOid,baseRefName,isCrossRepository'))
    require(len(prs) <= 1, 'Ambiguous existing PRs')
    entries, head, head_tree = {}, None, None
    if prs:
        pr = prs[0]
        head = pr['headRefOid']
        require(re.fullmatch(SHA, head) and pr['headRefName'] == branch and pr['baseRefName'] == 'main' and not pr['isCrossRepository'], 'Wrong existing PR target')
        require(api('git/ref/heads/' + branch)['object']['sha'] == head, 'Topic moved before rebuild')
        command('git', 'fetch', 'origin', 'refs/heads/' + branch)
        require(command('git', 'rev-parse', 'FETCH_HEAD') == head, 'Topic moved before rebuild')
        # Only the publication commit this workflow wrote may be rebuilt.
        require(len(command('git', 'rev-list', '--parents', '-n', '1', head).split()) == 2, 'Pending topic must be one commit')
        touched = command('git', 'diff-tree', '--no-commit-id', '--name-only', '-r', head).split('\n')
        require(all(path in allowed_paths() for path in touched if path), 'Pending topic changes files outside publication source')
        entries = pending_entries(base, head)
        head_tree = api('git/commits/' + head)['tree']['sha']
    else:
        require(not ref_exists(branch), 'Pending topic exists without an open PR; delete it and rerun')
    entries = merge_entry(entries, bundle, record, evidence)
    changes = build_changes(base, entries)
    if not changes:
        return 'Publication source already current'
    title, body = describe(entries)
    tree = api('git/commits/' + base)['tree']['sha']
    require(re.fullmatch(SHA, tree), 'Invalid base tree')
    tree = api('git/trees', {'base_tree': tree, 'tree': changes})['sha']
    require(re.fullmatch(SHA, tree), 'Invalid new tree')
    if head is not None and tree == head_tree and command('git', 'rev-parse', head + '^') == base:
        new = head
    else:
        new = api('git/commits', {'message': title + '\n\nKeep consumer guidance tied to the verified published index and exact source revision.',
                                  'tree': tree, 'parents': [base]})['sha']
        require(re.fullmatch(SHA, new), 'Invalid new commit')
        require(command('git', 'ls-remote', 'origin', 'refs/heads/main').split()[0] == base, 'Main moved; retry on fresh base')
        if head is None:
            try:
                api('git/refs', {'ref': 'refs/heads/' + branch, 'sha': new})
            except subprocess.CalledProcessError as error:
                raise ValueError('Pending topic appeared concurrently') from error
        else:
            # Lease: replace the topic only if it is still the head just read.
            require(api('git/ref/heads/' + branch)['object']['sha'] == head, 'Topic moved before update')
            api('git/refs/heads/' + branch, {'sha': new, 'force': True}, 'PATCH')
    require(api('git/ref/heads/' + branch)['object']['sha'] == new, 'Topic moved before PR update')
    if prs:
        command('gh', 'pr', 'edit', str(prs[0]['number']), '--repo', REPOSITORY, '--title', title, '--body', body)
        result = 'Updated publication PR'
    else:
        result = command('gh', 'pr', 'create', '--repo', REPOSITORY, '--base', 'main', '--head', branch, '--title', title, '--body', body)
    command('gh', 'workflow', 'run', 'ci.yml', '--repo', REPOSITORY, '--ref', branch,
            '-f', 'expected_head_sha=' + new)
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
