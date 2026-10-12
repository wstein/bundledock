#!/usr/bin/env python3
"""Read-only provenance gate. Only the reviewed workflow's environment dependency
supplies human approval; calling this helper or setting variables never does.
"""
import base64
import json
import os
import re
import urllib.error
import urllib.request
from pathlib import Path

REPOSITORY = 'wstein/bundledock'
ENVIRONMENT = 'toolbox-publication'
LOCK = 'bundles/toolbox/tools.lock.json'


def require(condition, message):
    if not condition:
        raise ValueError(message)


def validate(event, get):
    require(event['repository']['full_name'] == REPOSITORY, 'Wrong event repository')
    require(get('')['default_branch'] == 'main', 'Default branch must be main')
    source = event['workflow_run']
    sha = source['head_sha']
    require(isinstance(sha, str) and re.fullmatch('[0-9a-f]{40}', sha), 'Invalid source SHA')
    require(isinstance(source['id'], int) and source['id'] > 0, 'Invalid CI run ID')
    require(get('/git/ref/heads/main')['object']['sha'] == sha, 'Source is no longer current main')
    workflow = get('/actions/workflows/ci.yml')
    run = get('/actions/runs/' + str(source['id']))
    require(workflow['path'] == '.github/workflows/ci.yml' and workflow['id'] > 0, 'Untrusted CI workflow')
    require(run['id'] == source['id'] and run['workflow_id'] == workflow['id'] and run['path'] == workflow['path'], 'Wrong CI run workflow')
    require(run['head_sha'] == sha and run['head_branch'] == 'main' and run['event'] == 'push' and run['status'] == 'completed' and run['conclusion'] == 'success', 'CI must succeed for this exact main push')
    require(run['repository']['full_name'] == REPOSITORY and run['head_repository']['full_name'] == REPOSITORY, 'Wrong CI repository')
    associated = get('/commits/' + sha + '/pulls?per_page=100&page=1')
    require(isinstance(associated, list) and len(associated) < 100, 'Ambiguous PR pagination')
    candidates = []
    for summary in associated:
        number = summary['number']
        require(isinstance(number, int) and number > 0, 'Invalid PR number')
        pr = get('/pulls/' + str(number))
        if pr.get('merged') is True and pr.get('merge_commit_sha') == sha:
            candidates.append(pr)
    require(len(candidates) == 1, 'Exactly one merged updater PR required')
    pr = candidates[0]
    require(pr['user']['login'] == 'github-actions[bot]' and pr['user']['type'] == 'Bot', 'PR must be authored by github-actions[bot]')
    # Legacy prefix remains valid for proposals created before the naming update.
    require(re.fullmatch(r'automation/(?:toolbox|tool)-upgrade-[1-9][0-9]*-[1-9][0-9]*', pr['head']['ref']), 'Wrong updater branch')
    require(pr['head']['repo']['full_name'] == REPOSITORY and pr['base']['repo']['full_name'] == REPOSITORY and pr['base']['ref'] == 'main', 'Wrong PR source or target')
    number = pr['number']
    url = 'https://github.com/' + REPOSITORY + '/pull/' + str(number)
    require(pr['html_url'] == url, 'Wrong PR URL')
    files = get('/pulls/' + str(number) + '/files?per_page=100&page=1')
    require(len(files) == pr['changed_files'] and 0 < len(files) <= 2, 'Missing or truncated PR files')
    names = [item['filename'] for item in files]
    require(len(set(names)) == len(names) and LOCK in names and set(names) <= {LOCK, 'docs/tools.md'}, 'Only toolbox lock and inventory may change')
    require(all(item['status'] == 'modified' and item['changes'] > 0 and 'previous_filename' not in item for item in files), 'Renamed, deleted or unchanged files forbidden')
    commit = get('/commits/' + sha)
    merged_files = commit['files']
    require({item['filename'] for item in merged_files} == set(names) and len(merged_files) == len(files), 'Merged commit differs from reviewed PR files')
    require(all(item['status'] == 'modified' and item['changes'] > 0 and 'previous_filename' not in item for item in merged_files), 'Unsafe merged commit changes')
    parents = commit['parents']
    require(0 < len(parents) <= 2 and re.fullmatch('[0-9a-f]{40}', parents[0]['sha']), 'Invalid merge parent')
    def lock_bytes(revision):
        content = get('/contents/' + LOCK + '?ref=' + revision)
        require(content['encoding'] == 'base64' and not content.get('truncated'), 'Missing complete lock bytes')
        return base64.b64decode(content['content'].replace('\n', ''), validate=True)
    require(lock_bytes(sha) != lock_bytes(parents[0]['sha']), 'Merged lock is unchanged')
    environment = get('/environments/' + ENVIRONMENT)
    require(environment.get('can_admins_bypass') is False, 'Environment must prevent admin bypass')
    rules = [rule for rule in environment['protection_rules'] if rule['type'] == 'required_reviewers']
    require(len(rules) == 1, 'Required human review protection missing')
    reviewers = rules[0]['reviewers']
    require(len(reviewers) == 1 and reviewers[0]['type'] == 'User' and reviewers[0]['reviewer']['login'] == 'wstein', 'Only wstein may approve publication')
    # The maintainer may approve their own merged update; a second person is not required.
    require(rules[0].get('prevent_self_review') is False, 'Self-review policy must allow the maintainer to approve their merge')
    require(environment['deployment_branch_policy'] == {'protected_branches': False, 'custom_branch_policies': True}, 'Custom main-only branch restriction required')
    policies = get('/environments/' + ENVIRONMENT + '/deployment-branch-policies?per_page=100&page=1')
    require(policies['total_count'] == 1 and len(policies['branch_policies']) == 1, 'Exactly one branch policy required')
    require(policies['branch_policies'][0]['name'] == 'main' and policies['branch_policies'][0]['type'] == 'branch', 'Only main branch may deploy')
    return sha, url


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        return None


def main():
    require(os.environ.get('GITHUB_EVENT_NAME') == 'workflow_run', 'Only workflow_run provenance is supported')
    require(os.environ.get('GITHUB_REPOSITORY') == REPOSITORY, 'Wrong runtime repository')
    token = os.environ['GITHUB_TOKEN']
    require(bool(token), 'Read-only GitHub token required')
    opener = urllib.request.build_opener(NoRedirect())
    def get(path):
        request = urllib.request.Request('https://api.github.com/repos/' + REPOSITORY + path, headers={'Authorization': 'Bearer ' + token, 'Accept': 'application/vnd.github+json', 'X-GitHub-Api-Version': '2026-03-10'})
        with opener.open(request, timeout=30) as response:
            data = response.read(8 * 1024 * 1024 + 1)
        require(len(data) <= 8 * 1024 * 1024, 'GitHub response too large')
        return json.loads(data)
    sha, url = validate(json.loads(Path(os.environ['GITHUB_EVENT_PATH']).read_text()), get)
    require(not os.environ.get('PUBLICATION_SHA') or os.environ['PUBLICATION_SHA'] == sha, 'Approved source SHA changed')
    # Every caller checks out the exact validated revision before this gate.
    import subprocess
    require(subprocess.check_output(['git', 'rev-parse', 'HEAD'], text=True).strip() == sha, 'Checkout differs from approved source')
    if os.environ.get('GITHUB_OUTPUT'):
        with open(os.environ['GITHUB_OUTPUT'], 'a') as output:
            output.write('sha=' + sha + '\npr_url=' + url + '\n')
    if os.environ.get('GITHUB_STEP_SUMMARY'):
        with open(os.environ['GITHUB_STEP_SUMMARY'], 'a') as summary:
            summary.write('Toolbox publication source: `' + sha + '`\n\nMerged updater PR: ' + url + '\n\nHuman approval is required before artifact compilation or native work.\n')


if __name__ == '__main__':
    try:
        main()
    except (ValueError, KeyError, TypeError, OSError, urllib.error.URLError) as error:
        raise SystemExit('Toolbox publication provenance rejected: ' + str(error))
