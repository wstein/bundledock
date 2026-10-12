#!/usr/bin/env python3
"""Exact human PR approval and source provenance. No variable is approval proof."""
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
SOURCE_WORKFLOW = 'bundle-review-source.yml'


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


def approved_pr(number, name, get, merged=False):
    bundle = config.select_bundle(name)
    require(isinstance(number, int) and number > 0, 'Invalid PR number')
    require(get('')['full_name'] == REPOSITORY and get('')['default_branch'] == 'main', 'Wrong repository/default branch')
    pr = get('/pulls/' + str(number))
    require(pr['number'] == number and pr['commits'] == 1, 'Exactly one updater commit required')
    require(pr['user'] == {'login': 'github-actions[bot]', 'type': 'Bot'} or (pr['user']['login'] == 'github-actions[bot]' and pr['user']['type'] == 'Bot'), 'Wrong updater author')
    prefix = '(?:toolbox|tool)' if name == 'toolbox' else re.escape(name)
    require(re.fullmatch('automation/' + prefix + r'-upgrade-[1-9][0-9]*-[1-9][0-9]*', pr['head']['ref']), 'Wrong updater branch')
    require(pr['head']['repo']['full_name'] == REPOSITORY and pr['base']['repo']['full_name'] == REPOSITORY and pr['base']['ref'] == 'main', 'Wrong PR repository or base')
    head = sha(pr['head']['sha'])
    require(pr['html_url'] == 'https://github.com/' + REPOSITORY + '/pull/' + str(number), 'Wrong PR URL')
    reviews = get('/pulls/' + str(number) + '/reviews?per_page=100&page=1')
    require(isinstance(reviews, list) and len(reviews) < 100, 'Ambiguous review pagination')
    votes = [review for review in reviews if review['user']['login'] == 'wstein' and review['user']['type'] == 'User' and review['state'] in ('APPROVED', 'CHANGES_REQUESTED', 'DISMISSED')]
    require(bool(votes), 'Explicit wstein approval required')
    latest = max(votes, key=lambda review: review['id'])
    require(latest['state'] == 'APPROVED' and latest['commit_id'] == head, 'Latest wstein approval must cover this exact head')
    files = get('/pulls/' + str(number) + '/files?per_page=100&page=1')
    names = safe_files(files, pr, bundle)
    head_tree = sha(get('/git/commits/' + head)['tree']['sha'])
    if merged:
        require(pr['merged'] is True, 'PR is unmerged')
        commit_sha = sha(pr['merge_commit_sha'])
        commit = get('/commits/' + commit_sha)
        require(sha(commit['commit']['tree']['sha']) == head_tree, 'Merged tree differs from approved tree')
        require(safe_files(commit['files'], pr, bundle) == names, 'Merged files differ from reviewed PR')
        require(len(commit['parents']) == 1, 'Expected rebase merge commit')
        base = sha(commit['parents'][0]['sha'])
    else:
        require(pr['state'] == 'open' and pr['merged'] is False and not pr.get('draft'), 'Open ready updater PR required')
        base = sha(get('/git/ref/heads/main')['object']['sha'])
        comparison = get('/compare/' + base + '...' + head)
        require(comparison['merge_base_commit']['sha'] == base, 'Main changed since approved updater source')
        require(safe_files(comparison['files'], pr, bundle) == names, 'Unreviewed code tree changes')
        commit_sha = head
    require(lock_bytes(get, bundle, commit_sha) != lock_bytes(get, bundle, base), 'Lock is unchanged')
    return pr


def source_run(event, get):
    source = event['workflow_run']
    require(isinstance(source['id'], int) and source['id'] > 0 and isinstance(source['run_attempt'], int) and source['run_attempt'] > 0, 'Complete source run identity required')
    workflow = get('/actions/workflows/' + SOURCE_WORKFLOW)
    run = get('/actions/runs/' + str(source['id']) + '/attempts/' + str(source['run_attempt']))
    require(run['id'] == source['id'] and run['run_attempt'] == source['run_attempt'] and run['workflow_id'] == workflow['id'] and workflow['id'] > 0, 'Wrong source workflow/attempt')
    require(run['path'] == workflow['path'] == '.github/workflows/' + SOURCE_WORKFLOW, 'Wrong source workflow path')
    require(run['repository']['full_name'] == run['head_repository']['full_name'] == REPOSITORY and run['event'] == 'pull_request_review', 'Untrusted source run origin')
    require(run['status'] == 'completed' and run['conclusion'] == 'success', 'Source checks must succeed')
    require(len(run['pull_requests']) == 1, 'Exactly one source PR required')
    number = run['pull_requests'][0]['number']
    name = bundle_for_pr(number, get)
    pr = approved_pr(number, name, get)
    head = pr['head']['sha']
    require(run['pull_requests'][0]['head']['sha'] == head, 'Source run tested an older PR head')
    tested = sha(run['head_sha'])
    if tested != head:
        commit = get('/commits/' + tested)
        require(commit['commit']['tree']['sha'] == get('/git/commits/' + head)['tree']['sha'], 'Synthetic source tree differs from approved head')
        require([parent['sha'] for parent in commit['parents']] == [get('/git/ref/heads/main')['object']['sha'], head], 'Wrong synthetic merge identity')
    return name, pr


def bundle_for_pr(number, get):
    branch = get('/pulls/' + str(number))['head']['ref']
    for name in config.BUNDLES:
        prefix = '(?:toolbox|tool)' if name == 'toolbox' else re.escape(name)
        if re.fullmatch('automation/' + prefix + r'-upgrade-[1-9][0-9]*-[1-9][0-9]*', branch):
            return name
    raise ValueError('Not a bundle updater PR')


def validate(event, get, name='toolbox'):
    config.select_bundle(name)
    require(event['repository']['full_name'] == REPOSITORY, 'Wrong event repository')
    source = event['workflow_run']
    revision = sha(source['head_sha'])
    require(get('/git/ref/heads/main')['object']['sha'] == revision, 'Source is no longer current main')
    workflow = get('/actions/workflows/ci.yml')
    require(isinstance(source['id'], int) and source['id'] > 0 and isinstance(source['run_attempt'], int) and source['run_attempt'] > 0, 'Complete CI identity required')
    run = get('/actions/runs/' + str(source['id']) + '/attempts/' + str(source['run_attempt']))
    require(workflow['id'] > 0 and run['id'] == source['id'] and run['run_attempt'] == source['run_attempt'] and run['workflow_id'] == workflow['id'], 'Wrong CI workflow/attempt')
    require(run['path'] == workflow['path'] == '.github/workflows/ci.yml', 'Wrong CI path')
    require(run['head_sha'] == revision and run['head_branch'] == 'main' and run['event'] in ('push', 'workflow_dispatch') and run['status'] == 'completed' and run['conclusion'] == 'success', 'Trusted exact main source CI required')
    require(run['repository']['full_name'] == run['head_repository']['full_name'] == REPOSITORY, 'Wrong CI repository')
    associated = get('/commits/' + revision + '/pulls?per_page=100&page=1')
    require(isinstance(associated, list) and len(associated) < 100, 'Ambiguous associated PRs')
    candidates = [get('/pulls/' + str(item['number'])) for item in associated]
    candidates = [pr for pr in candidates if pr.get('merged') is True and pr.get('merge_commit_sha') == revision]
    require(len(candidates) == 1, 'Exactly one merged updater PR required')
    pr = approved_pr(candidates[0]['number'], name, get, merged=True)
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
        request = urllib.request.Request('https://api.github.com/repos/' + REPOSITORY + path, data=body, method=method, headers={'Authorization':'Bearer '+self.token,'Accept':'application/vnd.github+json','Content-Type':'application/json','X-GitHub-Api-Version':'2026-03-10'})
        with self.opener.open(request, timeout=30) as response:
            data = response.read(8*1024*1024+1)
        require(len(data) <= 8*1024*1024, 'Oversized GitHub response')
        return json.loads(data) if data else None


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--bundle', default='toolbox')
    parser.add_argument('--mode', choices=('publication','review','merge','config'), default='publication')
    args = parser.parse_args()
    require(os.environ.get('GITHUB_REPOSITORY') == REPOSITORY, 'Wrong runtime repository')
    event = json.loads(Path(os.environ['GITHUB_EVENT_PATH']).read_text())
    api = GitHub(os.environ['GITHUB_TOKEN'])
    output = {}
    if args.mode == 'config':
        output = config.select_bundle(args.bundle)
    elif args.mode == 'review':
        require(os.environ['GITHUB_EVENT_NAME'] == 'pull_request_review' and event['review']['user']['login'] == 'wstein' and event['review']['state'] == 'approved', 'Only explicit wstein approval may request source checks')
        number = event['pull_request']['number']
        name = bundle_for_pr(number, api.request)
        pr = approved_pr(number, name, api.request)
        require(event['review']['commit_id'] == pr['head']['sha'], 'Review event covers old head')
        output = {'bundle':name,'head':pr['head']['sha']}
    elif args.mode == 'merge':
        require(os.environ['GITHUB_EVENT_NAME'] == 'workflow_run', 'Trusted source completion required')
        name, pr = source_run(event, api.request)
        # Revalidate immediately before the optimistic PR-head lease. GitHub
        # cannot lease target main; merged-tree equality prevents publication
        # if concurrent target changes introduce unreviewed source.
        pr = approved_pr(pr['number'], name, api.request)
        result = api.request('/pulls/'+str(pr['number'])+'/merge', 'PUT', {'sha':pr['head']['sha'],'merge_method':'rebase'})
        require(result['merged'] is True, 'Merge was refused')
        revision = sha(result['sha'])
        merged = approved_pr(pr['number'], name, api.request, merged=True)
        require(merged['merge_commit_sha'] == revision and api.request('/git/ref/heads/main')['object']['sha'] == revision, 'Main moved after merge')
        api.request('/actions/workflows/ci.yml/dispatches', 'POST', {'ref':'main','inputs':{'source_sha':revision}})
        output = {'bundle':name,'sha':revision}
    else:
        require(os.environ['GITHUB_EVENT_NAME'] == 'workflow_run', 'Only CI completion is automatic publication authority')
        revision, url = validate(event, api.request, args.bundle)
        require(not os.environ.get('PUBLICATION_SHA') or os.environ['PUBLICATION_SHA'] == revision, 'Approved source changed')
        require(subprocess.check_output(['git','rev-parse','HEAD'],text=True).strip() == revision, 'Checkout differs from approved source')
        output = dict(config.select_bundle(args.bundle), sha=revision, pr_url=url)
    if os.environ.get('GITHUB_OUTPUT'):
        with open(os.environ['GITHUB_OUTPUT'],'a') as destination:
            for key, value in output.items():
                require('\n' not in str(value) and '\r' not in str(value), 'Unsafe output')
                destination.write(key+'='+str(value)+'\n')


if __name__ == '__main__':
    try:
        main()
    except (ValueError, KeyError, TypeError, OSError) as error:
        raise SystemExit('Bundle approval rejected: '+str(error))
