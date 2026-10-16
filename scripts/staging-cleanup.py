#!/usr/bin/env python3
"""Plan and remove verified Quay staging tags. Never delete by digest or delete blobs."""
import argparse
import base64
from datetime import datetime, timedelta, timezone
from email.utils import parsedate_to_datetime
import importlib.util
import json
import os
from pathlib import Path
import re
import subprocess
import sys
import urllib.error
import urllib.parse
import urllib.request


def load(name, filename):
    spec = importlib.util.spec_from_file_location(name, Path(__file__).with_name(filename))
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


tags = load('cleanup_tags', 'bundle-git-tags.py')
docs = load('cleanup_docs', 'publication-docs.py')
REPOSITORIES = tuple(sorted({tags._CONFIG.select_bundle(b)['repository'] for b in tags.BUNDLES}))
STAGING = re.compile(r'staging-(.+)-(amd64|arm64)\Z')


def require(condition, reason):
    if not condition:
        raise ValueError(reason)


def identity(repository, name):
    require(repository in REPOSITORIES, 'Unknown repository')
    match = STAGING.fullmatch(name)
    require(match is not None, 'Not an architecture staging tag')
    version, arch = match.groups()
    require(any(p.fullmatch(version) for p in (tags.DAILY, tags.VERSION, tags.SINGLETON, tags.LEGACY)), 'Unknown staging version')
    return version, arch


class Quay:
    def __init__(self, username='', password=''):
        self.username, self.password = username, password
        self.opener = urllib.request.build_opener(tags.NoRedirect())

    def _send(self, url, headers=None, method='GET', expected_status=200):
        headers = {'Accept': 'application/json', 'User-Agent': 'BundleDock-staging-cleanup', **(headers or {})}
        req = urllib.request.Request(url, headers=headers, method=method)
        try:
            with self.opener.open(req, timeout=30) as response:
                require(response.status == expected_status, 'Unexpected Quay response status')
                raw = response.read(4 * 1024 * 1024 + 1)
                require(len(raw) <= 4 * 1024 * 1024, 'Quay response too large')
                return json.loads(raw) if method == 'GET' else None
        except urllib.error.HTTPError as error:
            if method == 'DELETE' and error.code == 404:
                return None
            # Never expose credentials, response bodies or authentication tokens.
            raise ValueError('Quay HTTP ' + str(error.code)) from None

    def request(self, path):
        # These official repositories are public. Robot passwords are not API
        # OAuth tokens and must never be sent to the REST inventory endpoint.
        return self._send('https://quay.io/api/v1/repository/bundledock/' + path)

    def list_tags(self, repository, name=None):
        require(repository in REPOSITORIES, 'Unknown repository')
        if name is not None:
            identity(repository, name)
        result, seen = [], set()
        for page in range(1, 1001):
            query = dict(onlyActiveTags='true', limit=100, page=page)
            if name is not None:
                query['specificTag'] = name
            data = self.request(repository + '/tag/?' + urllib.parse.urlencode(query))
            require(data.get('page') == page and isinstance(data.get('tags'), list) and isinstance(data.get('has_additional'), bool), 'Malformed Quay pagination')
            for row in data['tags']:
                require(isinstance(row, dict) and isinstance(row.get('name'), str) and row['name'] not in seen, 'Duplicate or invalid tag')
                require(name is None or row['name'] == name, 'Unexpected specific tag response')
                seen.add(row['name'])
                result.append(row)
            if not data['has_additional']:
                return result
            require(bool(data['tags']), 'Empty additional page')
        raise ValueError('Quay pagination limit exceeded')

    def delete(self, repository, name):
        # Reject digest references and aliases before any authentication request.
        identity(repository, name)
        require(bool(self.username and self.password), 'REPOSITORY_USERNAME and REPOSITORY_TOKEN required for deletion')
        auth = base64.b64encode((self.username + ':' + self.password).encode()).decode()
        query = urllib.parse.urlencode({'service': 'quay.io', 'scope': 'repository:bundledock/' + repository + ':pull,push'})
        data = self._send('https://quay.io/v2/auth?' + query, {'Authorization': 'Basic ' + auth})
        token = data.get('token') if isinstance(data, dict) else None
        require(isinstance(token, str) and re.fullmatch(r'[A-Za-z0-9._~+/-]+=*', token) is not None, 'Invalid registry authentication token')
        # Quay's tag-name route untags only this name. Never substitute the
        # manifest digest: digest deletion would remove other matching tags.
        self._send('https://quay.io/v2/bundledock/' + repository + '/manifests/' + name,
                   {'Authorization': 'Bearer ' + token}, method='DELETE', expected_status=202)


class GitHub:
    def __init__(self):
        self.cache = {}

    def _get(self, path, fresh=False):
        if fresh or path not in self.cache:
            result = subprocess.run(['gh', 'api', 'repos/wstein/bundledock/' + path], capture_output=True, text=True, timeout=30)
            require(result.returncode == 0, 'GitHub evidence unavailable')
            require(len(result.stdout.encode()) <= 4 * 1024 * 1024, 'GitHub response too large')
            self.cache[path] = json.loads(result.stdout)
        return self.cache[path]

    def run(self, run_id, fresh=False):
        require(re.fullmatch('[1-9][0-9]*', str(run_id)) is not None, 'Invalid run ID')
        return self._get('actions/runs/' + str(run_id), fresh)

    def attempt(self, run_id, attempt, fresh=False):
        require(re.fullmatch('[1-9][0-9]*', str(run_id)) is not None and type(attempt) is int and attempt > 0, 'Invalid run attempt')
        return self._get('actions/runs/' + str(run_id) + '/attempts/' + str(attempt), fresh)

    def workflow(self, fresh=False):
        return self._get('actions/workflows/publish-bundles.yml', fresh)

    def jobs(self, run_id, attempt, fresh=False):
        require(re.fullmatch('[1-9][0-9]*', str(run_id)) is not None and type(attempt) is int and attempt > 0, 'Invalid jobs attempt')
        jobs, seen, total = [], set(), None
        for page in range(1, 21):
            data = self._get('actions/runs/' + str(run_id) + '/attempts/' + str(attempt) + '/jobs?per_page=100&page=' + str(page), fresh)
            count = data.get('total_count')
            require(type(count) is int and 0 <= count <= 2000 and isinstance(data.get('jobs'), list), 'Malformed jobs pagination')
            require(total is None or count == total, 'Jobs changed during pagination')
            total = count
            for job in data['jobs']:
                require(isinstance(job, dict) and type(job.get('id')) is int and job['id'] not in seen, 'Duplicate or invalid job')
                seen.add(job['id'])
                jobs.append(job)
            require(len(jobs) <= total, 'Jobs exceed reported total')
            if len(jobs) == total:
                return jobs
            require(bool(data['jobs']), 'Missing jobs page')
        raise ValueError('Jobs pagination limit exceeded')


FAILED_CONCLUSIONS = ('failure', 'cancelled', 'timed_out')


def failed_run_proof(github, run_id, attempt, fresh=False, run=None):
    require(re.fullmatch('[1-9][0-9]*', str(run_id)) is not None and type(attempt) is int and attempt > 0, 'Invalid failed run selection')
    run = github.run(run_id, fresh=fresh) if run is None else run
    require(str(run['id']) == str(run_id) and type(run['run_attempt']) is int and run['run_attempt'] == attempt, 'Failed run was rerun or its attempt changed')
    require(run['status'] == 'completed' and run.get('conclusion') in FAILED_CONCLUSIONS, 'Selected run is not a completed failed or cancelled publication')
    require(run['repository']['full_name'] == run['head_repository']['full_name'] == 'wstein/bundledock', 'Foreign run repository')
    require(run['head_branch'] == 'main' and run['event'] in ('workflow_dispatch', 'workflow_run'), 'Not an official main publication event')
    require(run['path'] == '.github/workflows/publish-bundles.yml' and re.fullmatch('[0-9a-f]{40}', run['head_sha']) is not None, 'Not the official publication workflow')
    workflow = github.workflow(fresh=fresh)
    require(workflow['id'] == run['workflow_id'] and workflow['path'] == run['path'] and workflow['state'] == 'active', 'Publication workflow identity unavailable or changed')
    exact = github.attempt(run_id, attempt, fresh=fresh)
    fields = ('id', 'run_attempt', 'status', 'conclusion', 'path', 'event', 'head_branch', 'head_sha', 'workflow_id')
    require(all(exact[k] == run[k] for k in fields), 'Exact attempt evidence differs from latest run')
    jobs = github.jobs(run_id, attempt, fresh=fresh)
    require(isinstance(jobs, list) and 0 < len(jobs) <= 2000, 'Complete job evidence unavailable')
    seen = set()
    for job in jobs:
        require(type(job.get('id')) is int and job['id'] not in seen and str(job.get('run_id')) == str(run_id) and type(job.get('run_attempt')) is int and job['run_attempt'] == attempt, 'Job belongs to another run or attempt')
        require(job.get('status') == 'completed' and isinstance(job.get('conclusion'), str) and bool(job['conclusion']), 'Publication job still active or unverifiable')
        seen.add(job['id'])
    # Resample after the attempt/jobs reads; queued or active reruns invalidate proof.
    latest = github.run(run_id, fresh=True)
    require(all(latest[k] == run[k] for k in fields), 'Publication changed while checking completed jobs')
    return run


def assess(repository, row, registry, github, now, publication=None, fresh=False, failed_run=None):
    result = {key: row.get(key) for key in ('name', 'manifest_digest', 'last_modified')}
    result.update(repository=repository, eligible=False)
    try:
        version, arch = identity(repository, row['name'])
        require(not row.get('is_manifest_list') and not row.get('immutable'), 'Index or immutable tag retained')
        digest = row['manifest_digest']
        manifest = tags.decode_digest(registry.get('manifests', digest), digest)
        require('manifests' not in manifest, 'Index retained')
        config_digest = manifest['config']['digest']
        config = tags.decode_digest(registry.get('blobs', config_digest), config_digest)
        require(config.get('os') == 'linux' and config.get('architecture') == arch, 'Platform differs from staging name')
        labels = config['config']['Labels']
        bundle = labels.get('io.bundledock.bundle')
        # Historical single-channel images may predate the explicit bundle label.
        if bundle is None and repository != 'node':
            bundle = repository
        require(bundle in tags.BUNDLES and tags._CONFIG.select_bundle(bundle)['repository'] == repository, 'Source bundle unavailable or mismatched')
        run_id = labels['io.bundledock.github.run-id']
        record = tags.validate_record(dict(bundle=bundle, image='quay.io/bundledock/' + repository,
            tag=version, digest=digest, source=labels['org.opencontainers.image.source'],
            commit=labels['org.opencontainers.image.revision'], run_id=run_id,
            run_attempt=labels.get('io.bundledock.github.run-attempt')), legacy=True)
        tags.verify_labels(record, labels)
        require(labels.get('org.opencontainers.image.version') == version, 'Version differs from staging name')
        attempt = record['attempt']
        if labels.get('io.bundledock.github.run-attempt') is not None:
            require(labels['io.bundledock.github.run-attempt'] == str(attempt), 'Attempt mismatch')
        if failed_run is not None:
            require(publication is None, 'Conflicting cleanup modes')
            require(run_id == failed_run[0] and attempt == failed_run[1], 'Tag belongs to another run or attempt')
        run = github.run(run_id, fresh=fresh)
        require(str(run['id']) == run_id and run['repository']['full_name'] == 'wstein/bundledock' and run['head_branch'] == 'main', 'Wrong run provenance')
        require(run['path'] in ('.github/workflows/publish-bundles.yml', '.github/workflows/bundledock-' + bundle + '.yml'), 'Not a publication workflow')
        require(run['event'] in ('workflow_dispatch', 'workflow_run'), 'Unexpected run event')
        if run['event'] == 'workflow_dispatch':
            require(run['head_sha'] == record['commit'], 'Run source mismatch')
        require(int(run['run_attempt']) >= attempt, 'Run attempt mismatch')
        if record['run_number'] is not None:
            require(run['run_number'] == record['run_number'], 'Run number mismatch')
        result.update(bundle=bundle, run_id=run_id, attempt=attempt, commit=record['commit'], version=version, architecture=arch)
        modified = parsedate_to_datetime(row['last_modified'])
        if modified.tzinfo is None:
            modified = modified.replace(tzinfo=timezone.utc)
        modified = modified.astimezone(timezone.utc)
        if publication is not None:
            require(all(str(record[k]) == str(publication[k]) for k in ('bundle', 'tag', 'commit', 'run_id', 'attempt')), 'Receipt identity mismatch')
            expected = {p['architecture']: p['digest'] for p in publication['platforms']}
            require(expected.get(arch) == digest and int(run['run_attempt']) == attempt, 'Receipt platform or current attempt mismatch')
            require(modified <= now, 'Future staging timestamp')
        elif failed_run is not None:
            failed_run_proof(github, *failed_run, fresh=fresh, run=run)
            require(modified <= now, 'Future staging timestamp')
            updated = datetime.fromisoformat(run['updated_at'].replace('Z', '+00:00'))
            require(updated.tzinfo is not None and updated <= now, 'Future or invalid completed-run timestamp')
        else:
            cutoff = now - timedelta(days=7)
            require(modified <= cutoff, 'Staging tag younger than seven days')
            require(run['status'] == 'completed' and run.get('conclusion') is not None, 'Publication run still active')
            updated = datetime.fromisoformat(run['updated_at'].replace('Z', '+00:00'))
            require(updated <= cutoff, 'Run or rerun younger than seven days')
        result.update(eligible=True, reason='Verified publication staging tag' if publication else ('Verified exact completed failed publication attempt' if failed_run else 'Verified completed run and tag older than seven days'))
    except (ValueError, KeyError, TypeError, OSError, subprocess.SubprocessError, urllib.error.URLError) as error:
        # Never include response bodies, tokens or signed URLs in audit output.
        result['reason'] = str(error) if isinstance(error, ValueError) else 'Required provenance evidence unavailable'
    return result


def apply_plan(plan, recheck, quay, write=False, report=None):
    require(plan.get('schema') == 1 and plan.get('mode') in ('fallback', 'publication', 'failed-run') and isinstance(plan.get('rows'), list), 'Invalid cleanup plan')
    target = plan.get('target')
    if plan['mode'] == 'failed-run':
        require(isinstance(target, dict) and re.fullmatch('[1-9][0-9]*', str(target.get('run_id'))) is not None and isinstance(target.get('run_id'), str) and type(target.get('attempt')) is int and target['attempt'] > 0, 'Invalid failed-run plan target')
    approved, seen = [], set()
    for row in plan['rows']:
        if row.get('eligible') is not True:
            continue
        identity(row['repository'], row['name'])
        if plan['mode'] == 'failed-run':
            require(row.get('run_id') == target['run_id'] and type(row.get('attempt')) is int and row['attempt'] == target['attempt'], 'Plan row outside exact failed attempt')
        require(tags.DIGEST.fullmatch(row['manifest_digest']) is not None, 'Invalid approved digest')
        key = row['repository'], row['name']
        require(key not in seen, 'Duplicate approved tag')
        seen.add(key)
        approved.append(row)
    actions = []
    for row in approved:
        current = quay.list_tags(row['repository'], row['name'])
        if not current:
            actions.append(dict(repository=row['repository'], name=row['name'], action='already absent'))
            if report:
                report(actions)
            continue
        require(len(current) == 1 and all(current[0].get(k) == row[k] for k in ('name', 'manifest_digest', 'last_modified')), 'Staging tag changed since dry run')
        verified = recheck(row['repository'], current[0])
        require(verified.get('eligible') is True and all(verified.get(k) == row.get(k) for k in ('repository', 'name', 'manifest_digest', 'last_modified', 'bundle', 'run_id', 'attempt')), 'Staging tag no longer eligible')
        if write:
            quay.delete(row['repository'], row['name'])
        actions.append(dict(repository=row['repository'], name=row['name'], action='deleted' if write else 'would delete'))
        if report:
            report(actions)
    return actions


def publication_proof(path, bundle, github):
    receipt = json.loads(Path(path).read_text())
    attempt = os.environ['GITHUB_RUN_ATTEMPT']
    docs.validate(receipt, bundle, 'quay.io/bundledock/' + tags._CONFIG.select_bundle(bundle)['repository'],
                  os.environ['PUBLICATION_SHA'], os.environ['GITHUB_RUN_ID'], attempt)
    record = tags.validate_record(dict(receipt, bundle=bundle))
    registry = tags.Registry(record['image'])
    tags.verify_source(record)
    tags.verify_registry(record, registry, aliases=True)
    index = tags.decode_digest(registry.get('manifests', record['digest']), record['digest'])
    require({d['platform']['architecture']: d['digest'] for d in index['manifests']} ==
            {p['architecture']: p['digest'] for p in receipt['platforms']}, 'Published platform digests differ from receipt')
    require(tags.remote_tag(record), 'Verified source tag missing')
    receipt.update(bundle=bundle, attempt=int(attempt))
    return receipt


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--repository', choices=('all',) + REPOSITORIES, default='all')
    parser.add_argument('--publication')
    parser.add_argument('--failed-run', help='Exact completed failed publication run ID')
    parser.add_argument('--failed-attempt', help='Exact latest run attempt (requires --failed-run)')
    parser.add_argument('--bundle', choices=tuple(tags.BUNDLES))
    parser.add_argument('--output', required=True)
    parser.add_argument('--apply', action='store_true')
    parser.add_argument('--approved-plan')
    args = parser.parse_args()
    require(not args.apply or args.approved_plan, '--apply requires a saved dry-run plan')
    require(not args.publication or args.bundle, '--publication requires --bundle')
    failed_run = None
    if args.failed_run is not None or args.failed_attempt is not None:
        require(not args.publication, 'Conflicting cleanup modes')
        require(re.fullmatch('[1-9][0-9]*', args.failed_run or '') is not None and re.fullmatch('[1-9][0-9]*', args.failed_attempt or '') is not None, 'Both failed run and attempt must be positive integers')
        failed_run = (args.failed_run, int(args.failed_attempt))
    username, password = os.environ.get('REPOSITORY_USERNAME', ''), os.environ.get('REPOSITORY_TOKEN', '')
    require(not args.apply or (username and password), 'REPOSITORY_USERNAME and REPOSITORY_TOKEN required for deletion')
    quay, github = Quay(username, password), GitHub()
    if failed_run is not None:
        failed_run_proof(github, *failed_run, fresh=True)
    target = dict(run_id=failed_run[0], attempt=failed_run[1]) if failed_run else None
    publication = publication_proof(args.publication, args.bundle, github) if args.publication else None
    repos = [tags._CONFIG.select_bundle(args.bundle)['repository']] if publication else (REPOSITORIES if args.repository == 'all' else [args.repository])
    registries = {repo: tags.Registry('quay.io/bundledock/' + repo) for repo in repos}
    now = datetime.now(timezone.utc)
    def check(repo, row):
        require(repo in repos, 'Plan repository outside selected scope')
        return assess(repo, row, registries[repo], github, datetime.now(timezone.utc), publication, fresh=True, failed_run=failed_run)
    mode = 'publication' if publication else ('failed-run' if failed_run else 'fallback')
    if args.apply:
        plan = json.loads(Path(args.approved_plan).read_text())
        require(plan.get('mode') == mode, 'Plan mode differs from request')
        require(plan.get('target') == target, 'Plan target differs from request')
        require(all(row.get('repository') in repos for row in plan['rows'] if row.get('eligible') is True), 'Plan repository outside selected scope')
        result = dict(schema=1, mode=mode, generated_at=now.isoformat(), actions=[])
        if target is not None:
            result['target'] = target
        def report(actions):
            result['actions'] = actions
            Path(args.output).write_text(json.dumps(result, indent=2, sort_keys=True) + '\n')
        report([])
        apply_plan(plan, check, quay, write=True, report=report)
    else:
        rows = []
        for repo in repos:
            for row in quay.list_tags(repo):
                if not row['name'].startswith('staging-'):
                    continue
                if publication and row['name'] not in ['staging-' + publication['tag'] + '-' + a for a in ('amd64', 'arm64')]:
                    continue
                rows.append(assess(repo, row, registries[repo], github, now, publication, failed_run=failed_run))
        result = dict(schema=1, mode=mode, generated_at=now.isoformat(), rows=sorted(rows, key=lambda r: (r['repository'], r['name'])))
    if target is not None:
        result['target'] = target
    Path(args.output).write_text(json.dumps(result, indent=2, sort_keys=True) + '\n')
    if os.environ.get('GITHUB_STEP_SUMMARY'):
        count = sum(row['eligible'] for row in result['rows']) if 'rows' in result else len(result['actions'])
        with open(os.environ['GITHUB_STEP_SUMMARY'], 'a') as summary:
            summary.write(f'\nStaging cleanup ({mode}): {count} eligible tags or completed actions. Full evidence is in the saved JSON artifact.\n')
    print(json.dumps({k: v for k, v in result.items() if k not in ('rows', 'actions')}))
    if 'rows' in result:
        print(f"Staging tags: {len(result['rows'])}; eligible: {sum(r['eligible'] for r in result['rows'])}; dry run saved to {args.output}")
    else:
        print(f"Cleanup actions: {len(result['actions'])}; saved to {args.output}")


if __name__ == '__main__':
    try:
        main()
    except (ValueError, KeyError, OSError, urllib.error.URLError, subprocess.SubprocessError) as error:
        print('Cleanup stopped: ' + (str(error) if isinstance(error, ValueError) else 'required evidence or transport unavailable'), file=sys.stderr)
        sys.exit(1)
