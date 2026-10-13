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


def lock_bytes(get, bundle, revision):
    content = get('/contents/' + bundle['lock'] + '?ref=' + sha(revision))
    require(content['encoding'] == 'base64' and not content.get('truncated'), 'Complete lock bytes required')
    return base64.b64decode(content['content'].replace('\n', ''), validate=True)


def safe_files(files, pr, bundle):
    names = [item['filename'] for item in files]
    require(len(files) == pr['changed_files'] and 0 < len(files) <= 2, 'Missing or truncated files')
    require(len(set(names)) == len(names) and bundle['lock'] in names and set(names) <= {bundle['lock'], bundle['inventory']}, 'Only own lock and inventory may change')
    require(all(item['status'] == 'modified' and item['changes'] > 0 and 'previous_filename' not in item for item in files), 'Renamed, deleted or unchanged files forbidden')
    return set(names)


class OwnerMergeRequired(ValueError):
    pass


def merged_updater(pr, name, get, revision):
    bundle = config.select_bundle(name)
    number = pr['number']
    require(isinstance(number, int) and number > 0, 'Invalid PR number')
    repository = get('')
    require(repository['full_name'] == REPOSITORY and repository['default_branch'] == 'main', 'Wrong repository/default branch')
    require(pr['commits'] == 1, 'Exactly one updater commit required')
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
    names = safe_files(files, pr, bundle)
    head_tree = sha(get('/git/commits/' + head)['tree']['sha'])
    commit = get('/commits/' + revision)
    require(sha(commit['commit']['tree']['sha']) == head_tree, 'Merged tree differs from updater head tree')
    require(safe_files(commit['files'], pr, bundle) == names, 'Merged files differ from updater PR')
    require(len(commit['parents']) == 1, 'Expected linear merge commit')
    base = sha(commit['parents'][0]['sha'])
    require(lock_bytes(get, bundle, revision) != lock_bytes(get, bundle, base), 'Lock is unchanged')
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


def bundle_for_branch(branch):
    for name in config.BUNDLES:
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
        def matches(candidate):
            return candidate.get('merged') is True and candidate.get('state') == 'closed' and candidate.get('merge_commit_sha') == revision
        if not matches(pr):
            diagnostic = merge_diagnostic(revision, pr, summary)
            # Retry only a demonstrated inconsistency between two reads of the
            # same exact associated source. A summary is never merge authority.
            proof = summary.get('state') == 'closed' and summary.get('merge_commit_sha') == revision and (summary.get('head') or {}).get('sha') == pr['head']['sha']
            if proof:
                print('Publication provenance read inconsistent; bounded retry: ' + diagnostic, file=sys.stderr)
                refreshed = get('/pulls/' + str(number))
                require(refreshed['number'] == number and refreshed['head']['sha'] == pr['head']['sha'] and refreshed['head']['ref'] == pr['head']['ref'], 'PR identity changed during bounded retry; ' + diagnostic)
                pr = refreshed
            require(matches(pr), 'Claimed updater does not match merged source; ' + merge_diagnostic(revision, pr, summary))
        claimed.append((actual, pr))
    require(len(claimed) <= 1, 'Ambiguous merged updater PRs')
    return claimed


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
    current = get('/actions/runs/' + str(source['id']))
    require(current['id'] == source['id'] and current['run_attempt'] == source['run_attempt'] and current['head_sha'] == revision and current['status'] == 'completed' and current['conclusion'] == 'success', 'Source CI attempt is no longer current successful exact source')
    claimed = associated_updaters(revision, get)
    if not claimed:
        require(allow_ineligible, 'Exactly one merged updater PR required')
        return None
    actual, pr = claimed[0]
    require(get('/git/ref/heads/main')['object']['sha'] == revision, 'Source is no longer current main')
    required_ci_check(source, revision, get)
    pr = merged_updater(pr, actual, get, revision)
    if name is not None and actual != name:
        require(allow_ineligible, 'Merged updater belongs to another bundle')
        return None
    if name is None:
        return actual, revision, pr['html_url']
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
        validate_caller(api.request, bundle, 'workflow_dispatch')
        subprocess.run(['sh', str(Path(__file__).with_name('approve-publication.sh'))], check=True)
        revision = sha(os.environ['APPROVED_SHA'])
        require(api.request('/git/ref/heads/main')['object']['sha'] == revision, 'Source is no longer current main')
        require(subprocess.check_output(['git','rev-parse','HEAD'],text=True).strip() == revision, 'Checkout differs from authorized source')
        output = dict(bundle, bundle=name, sha=revision, eligible='true', status='eligible', reason='Exact full-SHA human manual dispatch verified')
    else:
        require(os.environ['GITHUB_EVENT_NAME'] == 'workflow_run', 'Only CI completion is automatic publication authority')
        validate_caller(api.request, config.select_bundle(args.bundle))
        planning = args.mode == 'plan'
        candidate = validate(event, api.request, None if planning else args.bundle, allow_ineligible=args.mode in ('preflight', 'plan'))
        if candidate is None:
            output = {'eligible':'false', 'status':'ineligible', 'reason':'No matching merged bundle updater'}
        else:
            name, revision, url = candidate if planning else (args.bundle, *candidate)
            require(not os.environ.get('PUBLICATION_SHA') or os.environ['PUBLICATION_SHA'] == revision, 'Authorized source changed')
            require(subprocess.check_output(['git','rev-parse','HEAD'],text=True).strip() == revision, 'Checkout differs from authorized source')
            output = dict(config.select_bundle(name), bundle=name, sha=revision, pr_url=url, eligible='true', status='eligible', reason='Exact owner-merged updater and main CI verified')
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
        emit({'eligible':'false', 'status':status, 'reason':reason})
        detail = str(error) if isinstance(error, ValueError) else reason
        raise SystemExit('Bundle publication ' + status + ': ' + detail)


if __name__ == '__main__':
    run()
