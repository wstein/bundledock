#!/usr/bin/env python3
"""Exact owner-merged updater provenance. No variable is authorization proof."""
import argparse
import base64
import importlib.util
import json
import os
import re
import subprocess
import sys
import time
import unicodedata
import urllib.request
from pathlib import Path
sys.dont_write_bytecode = True
spec = importlib.util.spec_from_file_location('bundle_config', Path(__file__).with_name('bundle-config.py'))
config = importlib.util.module_from_spec(spec)
spec.loader.exec_module(config)
REPOSITORY = 'wstein/bundledock'


def require(condition, message):
    if not condition:
        raise ValueError(message)


def sha(value):
    require(isinstance(value, str) and re.fullmatch('[0-9a-f]{40}', value), 'Invalid exact SHA')
    return value


def file_bytes(get, path, revision):
    content = get('/contents/' + path + '?ref=' + sha(revision))
    require(content['encoding'] == 'base64' and not content.get('truncated'), 'Complete file bytes required')
    return base64.b64decode(content['content'].replace('\n', ''), validate=True)


def lock_bytes(get, bundle, revision):
    return file_bytes(get, bundle['lock'], revision)


GATE = 'Source-build publish gate: '
SOURCE_FIELDS = ('version', 'url', 'sha256', 'sha3_256')
VERSION = re.compile(r'v?([0-9]+)\.([0-9]+)\.([0-9]+)')
# Same grammar as toolconfig.ParseDesired: blanks removed, name(==|>=)x.y.z[,<x.y.z].
REQUIREMENT = re.compile(r'([a-z][a-z0-9-]*)(==|>=)([^\s]+)')
LOCK_KEYS = {'schema', 'desired_digest', 'catalog_digest', 'platforms', 'toolchains', 'extra_sources'}
SPEC_KEYS = {'version', 'url', 'sha256', 'sha512', 'sha3_256', 'verification', 'toolchain'}
CATALOG_KEYS = {'schema', 'packages'}
PACKAGE_KEYS = {'channel', 'provider', 'repository', 'tag_prefix', 'base_url', 'metadata_url', 'normalization', 'architectures',
                'asset', 'allowed_hosts', 'verification', 'sidecar', 'install', 'notices', 'source',
                'version_source', 'build', 'extra_sources'}
ARCHITECTURES = {'amd64', 'arm64'}


def strict_object(pairs):
    """The Go decoder matches keys ignoring case and Unicode variants and merges duplicates:
    refuse any object whose keys could shadow one another so the gate and the builder read the same data."""
    seen = set()
    for key, _ in pairs:
        folded = unicodedata.normalize('NFKC', key).casefold()
        require(folded not in seen, GATE + 'duplicate or case/Unicode-shadowed key ' + ascii(key))
        seen.add(folded)
    return dict(pairs)


def known_keys(value, allowed, what):
    require(isinstance(value, dict), GATE + what + ' must be an object')
    unknown = sorted(ascii(key) for key in value if key not in allowed)
    require(not unknown, GATE + what + ' has unknown key ' + ', '.join(unknown))
    return value


def parse_json(data, what):
    try:
        value = json.loads(data, object_pairs_hook=strict_object)
    except ValueError as error:
        if str(error).startswith(GATE):
            raise
        raise ValueError(GATE + what + ' is not valid JSON') from None
    require(isinstance(value, dict), GATE + what + ' must be a JSON object')
    return value


def parse_lock(data, what):
    lock = known_keys(parse_json(data, what), LOCK_KEYS, what)
    platforms = lock.get('platforms')
    known_keys(platforms, ARCHITECTURES, what + ' platforms')
    for arch, tools in platforms.items():
        require(isinstance(tools, dict), GATE + what + ' ' + arch + ' must be an object')
        for tool, spec in tools.items():
            known_keys(spec, SPEC_KEYS, what + ' ' + arch + ' ' + ascii(tool))
    return lock


def parse_catalog(data, what):
    catalog = known_keys(parse_json(data, what), CATALOG_KEYS, what)
    packages = catalog.get('packages')
    require(isinstance(packages, dict), GATE + what + ' packages must be an object')
    for name, package in packages.items():
        known_keys(package, PACKAGE_KEYS, what + ' package ' + ascii(name))
    return catalog


def version_tuple(value, what):
    match = VERSION.fullmatch(value) if isinstance(value, str) else None
    require(match is not None, GATE + what + ' is not a plain x.y.z version')
    return tuple(int(part) for part in match.groups())


def within_constraint(version, constraint, what):
    """constraint is (exact, lower, upper-or-None), as parsed by desired_constraint."""
    exact, lower, upper = constraint
    if exact:
        return version == lower
    return version >= lower and (upper is None or version < upper)


def desired_constraint(packages_txt, tool):
    """Parse packages.txt exactly as toolconfig.ParseDesired does; anything else is rejected."""
    found = None
    for line in packages_txt.decode().splitlines():
        text = ''.join(line.split('#')[0].split())
        if not text:
            continue
        match = REQUIREMENT.fullmatch(text)
        require(match is not None, GATE + 'packages.txt line is not name==x.y.z, name>=x.y.z or name>=x.y.z,<x.y.z (T2)')
        if match[1] != tool:
            continue
        require(found is None, GATE + tool + ' is listed twice in packages.txt (T2)')
        bounds = match[3].split(',<')
        exact = match[2] == '=='
        require(len(bounds) <= (1 if exact else 2), GATE + tool + ' constraint is not understood (T2)')
        found = (exact, version_tuple(bounds[0], tool + ' constraint bound (T2)'), version_tuple(bounds[1], tool + ' constraint bound (T2)') if len(bounds) == 2 else None)
    require(found is not None, GATE + tool + ' has no desired constraint in packages.txt (T2)')
    return found


def derived_url(package, tool, version):
    """Pattern of the one source URL the resolver rule for the tool derives from the version (T3)."""
    kind = (package.get('version_source') or {}).get('kind')
    require(kind == 'sqlite-product-data', GATE + tool + ' has no known URL resolver rule (T3)')
    major, minor, patch = version_tuple(version, tool + ' version')
    numeric = str(major * 1000000 + minor * 10000 + patch * 100)
    asset = package['asset'].replace('{numeric}', numeric)
    return re.compile(re.escape(package['base_url'].rstrip('/')) + r'/20[0-9]{2}/' + re.escape(asset))


def content_check(get, bundle, base, revision):
    """Publish-gate content check (design N2): an updater may only raise source pins.

    base is the merged commit's single parent, revision the merged commit; pull.base.sha is not used.
    """
    before = parse_lock(lock_bytes(get, bundle, base), 'parent lock')
    after = parse_lock(lock_bytes(get, bundle, revision), 'merged lock')
    # G1: toolchains and extra_sources are deeply equal.
    for section in ('toolchains', 'extra_sources'):
        require(before.get(section) == after.get(section), GATE + section + ' changed (G1); explicit owner approval required')
    # G2: no package added or removed and the platform set unchanged.
    old_platforms, new_platforms = before.get('platforms') or {}, after.get('platforms') or {}
    require(set(old_platforms) == set(new_platforms), GATE + 'platform set changed (G2)')
    catalog = parse_catalog(file_bytes(get, bundle['directory'] + '/packages.catalog.json', base), 'catalog')
    packages = catalog.get('packages') or {}
    source_builds = {name for name, package in packages.items() if package.get('provider') == 'source-build'}
    for arch in new_platforms:
        first = sorted((set(new_platforms[arch]) - set(old_platforms[arch])) & source_builds)
        require(not first, GATE + 'first publication of source-build tool ' + ','.join(first) + ' (' + arch + ') needs explicit owner approval (T2/G2)')
        require(set(old_platforms[arch]) == set(new_platforms[arch]), GATE + 'package set changed on ' + arch + ' (G2)')
    changed = [name for name in sorted(source_builds)
               if any(old_platforms[arch].get(name) != new_platforms[arch].get(name) for arch in new_platforms)]
    if not changed:
        return
    desired = file_bytes(get, bundle['directory'] + '/packages.txt', base)
    for tool in changed:
        specs = {arch: (old_platforms[arch][tool], new_platforms[arch][tool]) for arch in new_platforms}
        # T4: both architectures describe the same source.
        require(len(specs) == 2 and len({tuple(new.get(key) for key in SOURCE_FIELDS) for _, new in specs.values()}) == 1, GATE + tool + ' architectures disagree on version, url or hashes (T4)')
        for arch, (old, new) in specs.items():
            # T1: only version, url and the hashes may differ.
            rest = lambda spec: {key: value for key, value in spec.items() if key not in SOURCE_FIELDS}
            require(rest(old) == rest(new), GATE + tool + ' ' + arch + ' changed beyond the source pin (T1)')
            # T2: the version rises and stays within the desired constraint.
            number = version_tuple(new.get('version'), tool + ' new version')
            require(number > version_tuple(old.get('version'), tool + ' old version'), GATE + tool + ' ' + arch + ' version did not rise (T2)')
            require(within_constraint(number, desired_constraint(desired, tool), tool), GATE + tool + ' ' + arch + ' version is outside the desired constraint (T2)')
            # T3: the URL is the one derived from the new version.
            require(isinstance(new.get('url'), str) and derived_url(packages[tool], tool, new['version']).fullmatch(new['url']) is not None, GATE + tool + ' ' + arch + ' url is not derived from the version (T3)')


def safe_files(files, pr, bundles):
    """Every listed bundle's lock changed; nothing but their locks and inventories did."""
    names = [item['filename'] for item in files]
    require(len(files) == pr['changed_files'] and 0 < len(files) <= 2 * len(bundles), 'Missing or truncated files')
    allowed = {path for bundle in bundles for path in (bundle['lock'], bundle['inventory'])}
    require(len(set(names)) == len(names) and all(bundle['lock'] in names for bundle in bundles) and set(names) <= allowed, 'Only own lock and inventory may change')
    require(all(item['status'] == 'modified' and item['changes'] > 0 and 'previous_filename' not in item for item in files), 'Renamed, deleted or unchanged files forbidden')
    return set(names)


class OwnerMergeRequired(ValueError):
    pass


class MergeMetadataPending(OSError):
    pass


def updater_history(pr, get, base):
    count = pr['commits']
    require(type(count) is int and 1 <= count <= 16, 'Bounded updater commit history required')
    if count == 1:
        return
    commits = get('/pulls/' + str(pr['number']) + '/commits?per_page=100&page=1')
    require(isinstance(commits, list) and len(commits) == count, 'Complete updater commit history required')
    require(len({sha(commit['sha']) for commit in commits}) == count, 'Unique updater commits required')
    previous = None
    for index, commit in enumerate(commits):
        current = sha(commit['sha'])
        parents = commit['parents']
        require(len(parents) == (1 if index == 0 else 2), 'Only one updater commit followed by main refresh merges allowed')
        first = sha(parents[0]['sha'])
        if previous is not None:
            require(first == previous, 'Updater refresh history must be linear')
            ancestor = sha(parents[1]['sha'])
        else:
            ancestor = first
        if ancestor != base:
            comparison = get('/compare/' + ancestor + '...' + base)
            require(comparison['status'] == 'ahead' and sha(comparison['merge_base_commit']['sha']) == ancestor, 'Updater refresh must merge only ancestors of merged main parent')
        previous = current
    require(previous == pr['head']['sha'], 'Updater history does not end at exact PR head')


def updater_members(pr, name, get):
    """Bundles an updater PR claims: its own for a per-bundle topic, else those whose lock it changed."""
    if name != COMBINED:
        return [name]
    files = get('/pulls/' + str(pr['number']) + '/files?per_page=100&page=1')
    require(isinstance(files, list) and len(files) == pr['changed_files'], 'Missing or truncated files')
    changed = {item['filename'] for item in files}
    members = [member for member in config.AUTOMATIC if config.select_bundle(member)['lock'] in changed]
    require(members, 'Combined updater changes no bundle lock')
    return members


def merged_updater(pr, name, get, revision, members=None):
    members = members or [name]
    bundles = [config.select_bundle(member) for member in members]
    number = pr['number']
    require(isinstance(number, int) and number > 0, 'Invalid PR number')
    repository = get('')
    require(repository['full_name'] == REPOSITORY and repository['default_branch'] == 'main', 'Wrong repository/default branch')
    require(pr['user']['login'] == 'github-actions[bot]' and pr['user']['type'] == 'Bot', 'Wrong updater author')
    require(bundle_for_branch(pr['head']['ref']) == name, 'Wrong updater branch')
    require(pr['head']['repo']['full_name'] == REPOSITORY and pr['base']['repo']['full_name'] == REPOSITORY and pr['base']['ref'] == 'main', 'Wrong PR repository or base')
    head = sha(pr['head']['sha'])
    require(pr['html_url'] == 'https://github.com/' + REPOSITORY + '/pull/' + str(number), 'Wrong PR URL')
    owner = pr.get('merged_by') or {}
    if owner.get('login') != 'wstein' or owner.get('type') != 'User':
        raise OwnerMergeRequired('Eligible updater requires actual wstein human merge; pr=' + str(number) + ' expected_source=' + revision)
    require(pr['merged'] is True and pr['state'] == 'closed' and pr['merge_commit_sha'] == revision, 'Closed exact-source merged PR required')
    files = get('/pulls/' + str(number) + '/files?per_page=100&page=1')
    names = safe_files(files, pr, bundles)
    head_tree = sha(get('/git/commits/' + head)['tree']['sha'])
    commit = get('/commits/' + revision)
    require(sha(commit['commit']['tree']['sha']) == head_tree, 'Merged tree differs from updater head tree')
    require(safe_files(commit['files'], pr, bundles) == names, 'Merged files differ from updater PR')
    require(len(commit['parents']) == 1, 'Expected linear merge commit')
    base = sha(commit['parents'][0]['sha'])
    updater_history(pr, get, base)
    for bundle in bundles:
        require(lock_bytes(get, bundle, revision) != lock_bytes(get, bundle, base), 'Lock is unchanged')
        content_check(get, bundle, base, revision)
    return pr


def required_ci_check(source, revision, get):
    jobs = get('/actions/runs/' + str(source['id']) + '/attempts/' + str(source['run_attempt']) + '/jobs?per_page=100&page=1')['jobs']
    require(isinstance(jobs, list) and len(jobs) < 100, 'Ambiguous source CI jobs')
    checks = [job for job in jobs if job['name'] == 'check']
    require(len(checks) == 1 and checks[0]['status'] == 'completed' and checks[0]['conclusion'] == 'success' and checks[0]['head_sha'] == revision, 'Required main check must succeed on exact source')
    match = re.fullmatch(r'https://api.github.com/repos/wstein/bundledock/check-runs/([1-9][0-9]*)', checks[0]['check_run_url'])
    require(match is not None, 'Required check identity missing')
    check = get('/check-runs/' + match[1])
    require(check['app']['id'] == 15368 and check['name'] == 'check' and check['head_sha'] == revision and check['status'] == 'completed' and check['conclusion'] == 'success', 'Required GitHub Actions app check must succeed on exact source')


def validate_caller(get, bundle, event_name='workflow_run'):
    identity = os.environ
    require(re.fullmatch('[1-9][0-9]*', identity.get('GITHUB_RUN_ID', '')) and re.fullmatch('[1-9][0-9]*', identity.get('GITHUB_RUN_ATTEMPT', '')), 'Complete caller identity required')
    run = get('/actions/runs/' + identity['GITHUB_RUN_ID'] + '/attempts/' + identity['GITHUB_RUN_ATTEMPT'])
    workflow = get('/actions/workflows/' + bundle['workflow'])
    require(run['id'] == int(identity['GITHUB_RUN_ID']) and run['run_attempt'] == int(identity['GITHUB_RUN_ATTEMPT']) and run['workflow_id'] == workflow['id'] and workflow['id'] > 0, 'Wrong publication caller workflow/attempt')
    require(run['path'] == workflow['path'] == '.github/workflows/' + bundle['workflow'] and run['event'] == event_name and run['head_sha'] == identity['GITHUB_SHA'], 'Wrong real publication caller identity')
    require(run['repository']['full_name'] == run['head_repository']['full_name'] == REPOSITORY, 'Wrong publisher repository')


COMBINED = 'upgrades'


def bundle_for_branch(branch):
    """A bundle name for its own updater topic, COMBINED for the shared one, else None."""
    if re.fullmatch(r'automation/upgrades-[1-9][0-9]*-[1-9][0-9]*', branch):
        return COMBINED
    require(not re.match(r'automation/upgrades', branch), 'Malformed combined updater branch')
    for name in config.AUTOMATIC:
        prefix = '(?:toolbox|tool)' if name == 'toolbox' else re.escape(name)
        if re.fullmatch('automation/' + prefix + r'-upgrade-[1-9][0-9]*-[1-9][0-9]*', branch):
            return name
    require(not re.match(r'automation/(?:toolbox|tool|code-server|node|node-current|bun|codex|agy|claude|opentofu)-upgrade', branch), 'Malformed bundle updater branch')
    return None


def safe_identity(value):
    # Never include arbitrary API content, URLs, credentials or multiline text.
    if value is None or isinstance(value, (bool, int)):
        return str(value)
    if isinstance(value, str) and (re.fullmatch('[0-9a-f]{40}', value) or value in ('open', 'closed')):
        return value
    return 'unknown'


def merge_diagnostic(revision, pr, summary):
    fields = [('expected_source', revision), ('pr', pr.get('number')), ('observed_merge', pr.get('merge_commit_sha')), ('observed_merged', pr.get('merged')), ('observed_state', pr.get('state')), ('observed_head', (pr.get('head') or {}).get('sha')), ('associated_merge', summary.get('merge_commit_sha'))]
    return ' '.join(key + '=' + safe_identity(value) for key, value in fields)


def associated_updaters(revision, get):
    associated = get('/commits/' + revision + '/pulls?per_page=100&page=1')
    require(isinstance(associated, list) and len(associated) < 100, 'Ambiguous associated PRs')
    numbers = [item['number'] for item in associated]
    require(all(isinstance(number, int) and number > 0 for number in numbers) and len(set(numbers)) == len(numbers), 'Ambiguous associated PR identities')
    claimed = []
    for summary in associated:
        number = summary['number']
        pr = get('/pulls/' + str(number))
        require(pr['number'] == number, 'Associated PR identity changed')
        actual = bundle_for_branch(pr['head']['ref'])
        if actual is None:
            continue
        require(summary.get('merge_commit_sha') in (None, revision), 'Associated PR contradicts merged source; ' + merge_diagnostic(revision, pr, summary))
        def matches(candidate):
            return candidate.get('merged') is True and candidate.get('state') == 'closed' and candidate.get('merge_commit_sha') == revision
        if not matches(pr):
            # Missing merge metadata is not a mismatched source. Reread only
            # an already owner-merged bot PR with a matching closed association;
            # no source SHA is inferred from that association or the CI event.
            def trusted_missing(candidate):
                return (candidate.get('merged') is True and candidate.get('state') == 'closed'
                        and candidate.get('merge_commit_sha') is None
                        and (candidate.get('merged_by') or {}).get('login') == 'wstein'
                        and candidate['merged_by'].get('type') == 'User'
                        and (candidate.get('user') or {}).get('login') == 'github-actions[bot]'
                        and candidate['user'].get('type') == 'Bot'
                        and candidate['head']['repo']['full_name'] == REPOSITORY
                        and candidate['base']['repo']['full_name'] == REPOSITORY
                        and candidate['base']['ref'] == 'main')
            missing = (summary.get('state') == 'closed' and summary.get('merge_commit_sha') in (None, revision)
                       and (summary.get('head') or {}).get('sha') == pr['head']['sha'] and trusted_missing(pr))
            def identity(candidate):
                return (candidate['number'], candidate['head']['sha'], candidate['head']['ref'],
                        candidate['head']['repo']['full_name'], candidate['base']['ref'],
                        candidate['base']['repo']['full_name'], candidate['merged_by']['login'],
                        candidate['merged_by']['type'], candidate['user']['login'], candidate['user']['type'])
            if missing:
                original = identity(pr)
                for attempt in (2, 3):
                    print('Publication provenance bounded retry: attempt=' + str(attempt) + '/3 reason=missing_merge_metadata ' + merge_diagnostic(revision, pr, summary), file=sys.stderr)
                    time.sleep(2)
                    pr = get('/pulls/' + str(number))
                    require(identity(pr) == original, 'PR identity changed during bounded retry; ' + merge_diagnostic(revision, pr, summary))
                    if not trusted_missing(pr):
                        break
                if trusted_missing(pr):
                    raise MergeMetadataPending('Merge metadata unavailable after 3 reads; ' + merge_diagnostic(revision, pr, summary))
            require(matches(pr), 'Claimed updater does not match merged source; ' + merge_diagnostic(revision, pr, summary))
        claimed.append((actual, pr))
    require(len(claimed) <= 1, 'Ambiguous merged updater PRs')
    return claimed


def source_on_main(get, name, revision):
    """The approved source stays valid while main advances only past it.

    Another bundle's updater or an unrelated merge during a long native build
    must not discard approved work. Main must still descend from the exact
    source and this bundle's own frozen lock must be byte-identical there.
    """
    head = sha(get('/git/ref/heads/main')['object']['sha'])
    if head == revision:
        return
    comparison = get('/compare/' + revision + '...' + head)
    require(comparison['status'] == 'ahead' and sha(comparison['merge_base_commit']['sha']) == revision, 'Source is no longer an ancestor of current main')
    bundle = config.select_bundle(name)
    require(lock_bytes(get, bundle, head) == lock_bytes(get, bundle, revision), 'Bundle lock changed on current main')


def validate(event, get, name='toolbox', allow_ineligible=False):
    if name is not None:
        config.select_bundle(name)
    require(event['repository']['full_name'] == REPOSITORY, 'Wrong event repository')
    source = event['workflow_run']
    revision = sha(source['head_sha'])
    workflow = get('/actions/workflows/ci.yml')
    require(isinstance(source['id'], int) and source['id'] > 0 and isinstance(source['run_attempt'], int) and source['run_attempt'] > 0, 'Complete CI identity required')
    run = get('/actions/runs/' + str(source['id']) + '/attempts/' + str(source['run_attempt']))
    require(workflow['id'] > 0 and run['id'] == source['id'] and run['run_attempt'] == source['run_attempt'] and run['workflow_id'] == workflow['id'], 'Wrong CI workflow/attempt')
    require(run['path'] == workflow['path'] == '.github/workflows/ci.yml', 'Wrong CI path')
    require(run['head_sha'] == revision and run['head_branch'] == 'main' and run['event'] in ('push', 'workflow_dispatch') and run['status'] == 'completed' and run['conclusion'] == 'success', 'Trusted exact main source CI required')
    require(run['repository']['full_name'] == run['head_repository']['full_name'] == REPOSITORY, 'Wrong CI repository')
    claimed = associated_updaters(revision, get)
    # Delayed provenance reads must not retain an older CI-attempt observation.
    current = get('/actions/runs/' + str(source['id']))
    require(current['id'] == source['id'] and current['run_attempt'] == source['run_attempt'] and current['head_sha'] == revision and current['status'] == 'completed' and current['conclusion'] == 'success', 'Source CI attempt is no longer current successful exact source')
    if not claimed:
        require(allow_ineligible, 'Exactly one merged updater PR required')
        return None
    actual, pr = claimed[0]
    members = updater_members(pr, actual, get)
    for member in members if name is None else [member for member in members if member == name]:
        source_on_main(get, member, revision)
    required_ci_check(source, revision, get)
    pr = merged_updater(pr, actual, get, revision, members)
    if name is not None and name not in members:
        require(allow_ineligible, 'Merged updater belongs to another bundle')
        return None
    if name is None:
        return members, revision, pr['html_url']
    return revision, pr['html_url']


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        return None


class GitHub:
    def __init__(self, token):
        require(bool(token), 'GitHub token required')
        self.token = token
        self.opener = urllib.request.build_opener(NoRedirect())
    def request(self, path, method='GET', payload=None):
        require(path.startswith('/') or path == '', 'Relative API path required')
        body = None if payload is None else json.dumps(payload).encode()
        # 2026-03-10 removes merge_commit_sha from PR responses; exact merged
        # source authorization requires the supported 2022-11-28 contract.
        request = urllib.request.Request('https://api.github.com/repos/' + REPOSITORY + path, data=body, method=method, headers={'Authorization':'Bearer '+self.token,'Accept':'application/vnd.github+json','Content-Type':'application/json','X-GitHub-Api-Version':'2022-11-28'})
        with self.opener.open(request, timeout=30) as response:
            data = response.read(8*1024*1024+1)
        require(len(data) <= 8*1024*1024, 'Oversized GitHub response')
        return json.loads(data) if data else None


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--bundle', default='toolbox')
    parser.add_argument('--mode', choices=('publication','preflight','plan','config'), default='publication')
    args = parser.parse_args()
    require(os.environ.get('GITHUB_REPOSITORY') == REPOSITORY, 'Wrong runtime repository')
    event = json.loads(Path(os.environ['GITHUB_EVENT_PATH']).read_text())
    api = GitHub(os.environ['GITHUB_TOKEN'])
    output = {}
    if args.mode == 'config':
        output = config.select_bundle(args.bundle)
    elif args.mode == 'plan' and os.environ.get('GITHUB_EVENT_NAME') == 'workflow_dispatch':
        name = os.environ.get('BUNDLE', '')
        bundle = config.select_bundle(name)
        require(name not in config.RETIRED, name + ' is retired and is no longer published; its images stay available on Quay')
        validate_caller(api.request, bundle, 'workflow_dispatch')
        subprocess.run(['sh', str(Path(__file__).with_name('approve-publication.sh'))], check=True)
        revision = sha(os.environ['APPROVED_SHA'])
        require(api.request('/git/ref/heads/main')['object']['sha'] == revision, 'Source is no longer current main')
        require(subprocess.check_output(['git','rev-parse','HEAD'],text=True).strip() == revision, 'Checkout differs from authorized source')
        output = dict(bundle, bundle=name, bundles=json.dumps([name]), sha=revision, eligible='true', status='eligible', reason='Exact full-SHA human manual dispatch verified')
    else:
        require(os.environ['GITHUB_EVENT_NAME'] == 'workflow_run', 'Only CI completion is automatic publication authority')
        require(args.bundle not in config.RETIRED, args.bundle + ' is retired and is no longer published; its images stay available on Quay')
        require(args.bundle not in config.MANUAL_ONLY, args.bundle + ' publishes only by manual dispatch')
        validate_caller(api.request, config.select_bundle(args.bundle))
        planning = args.mode == 'plan'
        candidate = validate(event, api.request, None if planning else args.bundle, allow_ineligible=args.mode in ('preflight', 'plan'))
        if candidate is None:
            output = {'eligible':'false', 'status':'ineligible', 'reason':'No matching merged bundle updater'}
        else:
            names, revision, url = candidate if planning else ([args.bundle], *candidate)
            require(not os.environ.get('PUBLICATION_SHA') or os.environ['PUBLICATION_SHA'] == revision, 'Authorized source changed')
            require(subprocess.check_output(['git','rev-parse','HEAD'],text=True).strip() == revision, 'Checkout differs from authorized source')
            output = dict(bundles=json.dumps(names), sha=revision, pr_url=url, eligible='true', status='eligible', reason='Exact owner-merged updater and main CI verified')
            if not planning:
                output = dict(config.select_bundle(args.bundle), bundle=args.bundle, **output)
    emit(output)


def emit(output):
    if os.environ.get('GITHUB_OUTPUT'):
        with open(os.environ['GITHUB_OUTPUT'], 'a') as destination:
            for key, value in output.items():
                require('\n' not in str(value) and '\r' not in str(value), 'Unsafe output')
                destination.write(key + '=' + str(value) + '\n')
    if os.environ.get('GITHUB_STEP_SUMMARY') and 'status' in output:
        with open(os.environ['GITHUB_STEP_SUMMARY'], 'a') as destination:
            destination.write('Publication preflight: **' + output['status'] + '**. ' + output['reason'] + '.\n')


def run():
    try:
        main()
    except (ValueError, KeyError, TypeError, OSError, subprocess.CalledProcessError) as error:
        if isinstance(error, OwnerMergeRequired):
            status, reason = 'blocked_owner_merge', 'Actual human owner merge not verified'
        elif isinstance(error, (OSError, KeyError, TypeError)):
            status, reason = 'pending', 'Required API or source provenance unavailable; no publication authorized'
        else:
            status, reason = 'rejected', 'Source or approval provenance rejected; no publication authorized'
        detail = str(error) if isinstance(error, (ValueError, MergeMetadataPending)) else reason
        emit({'eligible':'false', 'status':status, 'reason':reason, 'detail':' '.join(detail.split())[:300]})
        raise SystemExit('Bundle publication ' + status + ': ' + detail)


if __name__ == '__main__':
    run()
