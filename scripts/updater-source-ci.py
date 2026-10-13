#!/usr/bin/env python3
"""Dispatch source-only CI for one fresh, base-derived bot updater PR."""
import argparse
import json
import os
from pathlib import Path
import re
import runpy
import subprocess

REPOSITORY = 'wstein/bundledock'
BUNDLES = runpy.run_path(str(Path(__file__).with_name('bundle-config.py')))['BUNDLES']


def require(condition, message):
    if not condition:
        raise ValueError(message)


def api(path, payload=None):
    command = ['gh', 'api', 'repos/' + REPOSITORY + path]
    if payload is not None:
        subprocess.run(command + ['--method', 'POST', '--input', '-'],
                       input=json.dumps(payload), text=True, check=True)
        return None
    return json.loads(subprocess.check_output(command, text=True))


def dispatch(args):
    require(os.environ.get('GITHUB_REPOSITORY') == REPOSITORY and
            os.environ.get('DEFAULT_BRANCH') == 'main' and
            os.environ.get('GITHUB_REF') == 'refs/heads/main', 'Trusted main proposer required')
    require(re.fullmatch('[0-9a-f]{40}', args.base) and
            re.fullmatch('[0-9a-f]{40}', args.head) and
            args.base == os.environ.get('GITHUB_SHA'), 'Exact proposer source required')
    run, attempt = os.environ.get('GITHUB_RUN_ID', ''), os.environ.get('GITHUB_RUN_ATTEMPT', '')
    require(re.fullmatch('[1-9][0-9]*', run) and re.fullmatch('[1-9][0-9]*', attempt), 'Invalid proposer identity')
    require(args.branch == 'automation/' + args.bundle + '-upgrade-' + run + '-' + attempt,
            'Only this proposer topic may request CI')
    match = re.fullmatch(r'https://github.com/wstein/bundledock/pull/([1-9][0-9]*)', args.pr_url)
    require(match is not None, 'Invalid proposal URL')
    number = match[1]
    repository = api('')
    require(repository['full_name'] == REPOSITORY and repository['default_branch'] == 'main', 'Wrong repository/default branch')
    pr = api('/pulls/' + number)
    require(pr['number'] == int(number) and pr['html_url'] == args.pr_url and
            pr['state'] == 'open' and not pr['draft'] and not pr['merged'], 'Open ready proposal required')
    require(pr['user']['login'] == 'github-actions[bot]' and pr['user']['type'] == 'Bot', 'Bot proposal required')
    require(pr['head']['repo']['full_name'] == pr['base']['repo']['full_name'] == REPOSITORY and
            pr['base']['ref'] == 'main' and pr['base']['sha'] == args.base and
            pr['head']['ref'] == args.branch and pr['head']['sha'] == args.head,
            'Proposal source/repository changed')
    require(pr['commits'] == 1 and 1 <= pr['changed_files'] <= 2, 'One bounded updater commit required')
    files = api('/pulls/' + number + '/files?per_page=100&page=1')
    allowed = {'bundles/' + args.bundle + '/packages.lock.json', 'docs/' + args.bundle + '.md'}
    require(isinstance(files, list) and len(files) == pr['changed_files'] and
            len({item['filename'] for item in files}) == len(files) and
            all(item['filename'] in allowed and item['status'] == 'modified' for item in files),
            'Only existing own lock/inventory changes permitted')
    comparison = api('/compare/' + args.base + '...' + args.head)
    require(comparison['status'] == 'ahead' and comparison['ahead_by'] == 1 and
            comparison['behind_by'] == 0 and comparison['total_commits'] == 1 and
            len(comparison['commits']) == 1 and comparison['commits'][0]['sha'] == args.head and
            [parent['sha'] for parent in comparison['commits'][0]['parents']] == [args.base],
            'Proposal must derive directly from trusted main source')
    require([(item['filename'], item['status']) for item in comparison['files']] ==
            [(item['filename'], item['status']) for item in files], 'Compared source differs from proposal scope')
    workflow = api('/actions/workflows/ci.yml')
    require(workflow['id'] > 0 and workflow['path'] == '.github/workflows/ci.yml' and
            workflow['state'] == 'active', 'Active source CI required')
    runs = api('/actions/workflows/ci.yml/runs?branch=' + args.branch +
               '&event=workflow_dispatch&per_page=100&page=1')
    require(isinstance(runs['workflow_runs'], list) and runs['total_count'] < 100 and
            runs['total_count'] == len(runs['workflow_runs']), 'Ambiguous source CI history')
    existing = any(run['workflow_id'] == workflow['id'] and run['path'] == workflow['path'] and
                   run['head_sha'] == args.head and run['head_branch'] == args.branch and
                   run['event'] == 'workflow_dispatch' and
                   run['repository']['full_name'] == run['head_repository']['full_name'] == REPOSITORY and
                   (run['status'] in ('queued', 'in_progress', 'requested', 'waiting', 'pending') or
                    (run['status'] == 'completed' and run['conclusion'] == 'success'))
                   for run in runs['workflow_runs'])
    # Re-read both refs immediately before the write. CI also guards the SHA if
    # the topic moves in the remaining API race; no unchecked source can pass.
    require(api('/git/ref/heads/main')['object']['sha'] == args.base, 'Main advanced; refresh proposal first')
    require(api('/git/ref/heads/' + args.branch)['object']['sha'] == args.head, 'Proposal head changed')
    if existing:
        print('Exact proposal source CI already queued, running or successful; no duplicate dispatch.')
        return
    api('/actions/workflows/ci.yml/dispatches', {'ref': args.branch, 'inputs': {'expected_head_sha': args.head}})
    print('Dispatched source CI for ' + args.pr_url + ' at ' + args.head + '.')


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--bundle', required=True, choices=BUNDLES)
    for name in ('branch', 'base', 'head', 'pr-url'):
        parser.add_argument('--' + name, required=True)
    try:
        dispatch(parser.parse_args())
    except (ValueError, KeyError, TypeError, OSError, subprocess.CalledProcessError) as error:
        raise SystemExit('Source CI refused: ' + str(error))


if __name__ == '__main__':
    main()
