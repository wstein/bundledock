#!/usr/bin/env python3
"""Authorize Pages for verified current-main CI or an exact human dispatch."""
import json
import os
from pathlib import Path
import re
import subprocess
import sys

REPOSITORY = 'wstein/bundledock'
SHA = re.compile(r'[0-9a-f]{40}\Z')
NUMBER = re.compile(r'[1-9][0-9]*\Z')


def require(condition, message):
    if not condition:
        raise ValueError(message)


def api(path):
    # Fixed repository and read-only endpoints; never dispatch, upload or deploy.
    result = subprocess.run(['gh', 'api', 'repos/' + REPOSITORY + path],
                            capture_output=True, text=True, timeout=30)
    require(result.returncode == 0, 'GitHub evidence unavailable')
    require(len(result.stdout) <= 8 * 1024 * 1024, 'GitHub evidence too large')
    return json.loads(result.stdout)


def validate(env, read):
    try:
        return verify(env, read)
    except (KeyError, TypeError) as error:
        raise ValueError('Required Pages provenance evidence missing or malformed') from error


def verify(env, read):
    require(env.get('GITHUB_REPOSITORY') == REPOSITORY and env.get('DEFAULT_BRANCH') == 'main'
            and env.get('GITHUB_REF') == 'refs/heads/main', 'Pages requires official default main')
    sha = env.get('PUBLICATION_SHA', '')
    require(SHA.fullmatch(sha) is not None, 'Full publication SHA required')
    repository = read('')
    require(repository['full_name'] == REPOSITORY and repository['default_branch'] == 'main', 'Unexpected repository configuration')
    require(read('/branches/main')['commit']['sha'] == sha, 'Publication source is no longer current main')
    event = env.get('GITHUB_EVENT_NAME')
    if event == 'workflow_dispatch':
        require(env.get('APPROVED_SHA') == sha and env.get('GITHUB_SHA') == sha, 'Manual publication requires exact approved run SHA')
        require(env.get('SENDER_TYPE') == 'User', 'Manual publication requires a human sender')
        for key in ('GITHUB_ACTOR', 'TRIGGERING_ACTOR'):
            actor = env.get(key, '')
            require(actor and '[bot]' not in actor and actor != 'github-actions', 'Manual publication requires human actors')
    elif event == 'workflow_run':
        require(env.get('GITHUB_SHA') == sha, 'Pages controller revision differs from CI source')
        run_id, attempt = env.get('CI_RUN_ID', ''), env.get('CI_RUN_ATTEMPT', '')
        require(NUMBER.fullmatch(run_id) is not None and NUMBER.fullmatch(attempt) is not None, 'Exact CI run and attempt required')
        workflow = read('/actions/workflows/ci.yml')
        require(workflow['path'] == '.github/workflows/ci.yml' and workflow['state'] == 'active', 'Unexpected CI workflow')
        run_path = '/actions/runs/' + run_id
        run = read(run_path)
        require(str(run['id']) == run_id and run['run_attempt'] == int(attempt), 'CI run or latest attempt differs from trigger')
        require(run['workflow_id'] == workflow['id'] and run['path'] == '.github/workflows/ci.yml', 'Run is not the official CI workflow')
        require(run['repository']['full_name'] == REPOSITORY and run['head_repository']['full_name'] == REPOSITORY, 'CI repository mismatch')
        require(run['head_branch'] == 'main' and run['head_sha'] == sha and run['event'] in ('push', 'workflow_dispatch'), 'CI must cover this exact main revision')
        require(run['status'] == 'completed' and run['conclusion'] == 'success', 'CI did not complete successfully')
        jobs, seen, total = [], set(), None
        for page in range(1, 101):
            data = read(run_path + '/attempts/' + attempt + '/jobs?per_page=100&page=' + str(page))
            require(isinstance(data['total_count'], int) and 0 < data['total_count'] <= 10000 and isinstance(data['jobs'], list), 'Invalid CI jobs evidence')
            if total is None:
                total = data['total_count']
            require(total == data['total_count'], 'CI jobs changed during verification')
            for job in data['jobs']:
                require(job['id'] not in seen, 'Duplicate CI job evidence')
                seen.add(job['id'])
                jobs.append(job)
            require(len(jobs) <= total, 'Too many CI jobs')
            if len(jobs) == total:
                break
            require(len(data['jobs']) == 100, 'Incomplete CI jobs evidence')
        require(len(jobs) == total, 'CI jobs pagination limit exceeded')
        checks = [job for job in jobs if job['name'] == 'check']
        require(len(checks) == 1 and checks[0]['head_sha'] == sha and checks[0]['status'] == 'completed'
                and checks[0]['conclusion'] == 'success', 'Required check did not succeed for this source')
        # Catch reruns started while reading the attempt's jobs.
        latest = read(run_path)
        require(latest == run, 'CI run changed during verification')
    else:
        raise ValueError('Unsupported Pages publication event')
    require(read('/branches/main')['commit']['sha'] == sha, 'Main advanced during verification')
    return sha


def main():
    sha = validate(os.environ, api)
    output = os.environ.get('GITHUB_OUTPUT')
    if output:
        with Path(output).open('a') as stream:
            stream.write('sha=' + sha + '\n')
    print('Pages publication authorized for ' + sha)


if __name__ == '__main__':
    try:
        main()
    except (ValueError, OSError, subprocess.SubprocessError) as error:
        print('Pages publication stopped: ' + (str(error) if isinstance(error, ValueError) else 'required GitHub evidence unavailable'), file=sys.stderr)
        sys.exit(1)
