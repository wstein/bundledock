#!/usr/bin/env python3
"""Read-only reconciliation of an exact publication attempt, receipt, OCI index and source tag."""
import argparse
import base64
import importlib.util
import io
import json
from pathlib import Path
import re
import select
import subprocess
import sys
import time
import zipfile

sys.dont_write_bytecode = True


def load(name, filename):
    spec = importlib.util.spec_from_file_location(name, Path(__file__).with_name(filename))
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


approval = load('approval', 'bundle-publication.py')
docs = load('publication_docs', 'publication-docs.py')
tags = docs._TAGS
require = approval.require
REPOSITORY = approval.REPOSITORY
JOBS = ('publication / resolve', 'publication / native (amd64, ubuntu-24.04)',
        'publication / native (arm64, ubuntu-24.04-arm)', 'publication / promote',
        'publication / tag', 'publication / documentation')
SMOKE = 'Build once and smoke on the native architecture'
PROMOTE = 'Verify both platform manifests and promote version and canonical channel aliases'
TAG = 'Tag the verified publication source'
DOCUMENTATION = 'Propose verified publication Markdown source and run source CI'
LIMIT = 8 * 1024 * 1024
TIMEOUT = 60


class Pending(Exception):
    """Evidence is unavailable; never turn absence into a publication claim."""


class GitHub:
    def raw(self, path):
        require(path.startswith('/') and not path.startswith('//') and '#' not in path
                and not any(part in ('.', '..') for part in path.split('/')),
                'Relative fixed-repository API path required')
        # gh owns authentication; do not inspect credentials or echo stderr.
        command = ['gh', 'api', '--hostname', 'github.com', '--method', 'GET',
                   'repos/' + REPOSITORY + path]
        try:
            with subprocess.Popen(command, stdout=subprocess.PIPE, stderr=subprocess.DEVNULL) as child:
                try:
                    deadline = time.monotonic() + TIMEOUT
                    output = bytearray()
                    while True:
                        remaining = deadline - time.monotonic()
                        if remaining <= 0 or not select.select([child.stdout], [], [], remaining)[0]:
                            raise subprocess.TimeoutExpired(command, TIMEOUT)
                        chunk = child.stdout.read1(min(65536, LIMIT + 1 - len(output)))
                        if not chunk:
                            break
                        output.extend(chunk)
                        require(len(output) <= LIMIT, 'Oversized GitHub response')
                    if child.wait(timeout=max(0, deadline - time.monotonic())):
                        raise Pending('GitHub evidence unavailable')
                    return bytes(output)
                finally:
                    if child.poll() is None:
                        child.kill()
                    child.wait()
        except (OSError, subprocess.TimeoutExpired) as error:
            raise Pending('GitHub evidence unavailable') from error

    def get(self, path):
        return json.loads(self.raw(path))

    def listing(self, path, key):
        items = []
        total = None
        for page in range(1, 11):
            data = self.get(path + '?per_page=100&page=' + str(page))
            require(isinstance(data.get(key), list) and isinstance(data.get('total_count'), int),
                    'Invalid GitHub listing')
            if total is None:
                total = data['total_count']
            require(total == data['total_count'] and 0 <= total <= 1000, 'Unstable or excessive pagination')
            items.extend(data[key])
            if len(data[key]) < 100:
                require(len(items) == total, 'Incomplete GitHub listing')
                return items
        require(len(items) == total, 'Incomplete GitHub listing')
        return items


def completed(item, conclusion='success'):
    return item.get('status') == 'completed' and item.get('conclusion') == conclusion


def job_state(jobs, event, conclusion):
    # GitHub collapses an entirely skipped matrix to one unsuffixed job.
    artifact_names = set(JOBS) | {'publication / native'}
    artifact_jobs = [job for job in jobs if job.get('name') in artifact_names]
    names = {job['name'] for job in artifact_jobs}
    noop_names = {JOBS[0], JOBS[3], JOBS[4], JOBS[5]}
    natives = {JOBS[1], JOBS[2]}
    if (event == 'workflow_run' and conclusion == 'success'
            and noop_names <= names
            and (natives <= names or 'publication / native' in names)
            and len(names) == len(artifact_jobs)
            and all(completed(job, 'skipped') for job in artifact_jobs)):
        return 'not-published'
    selected = {name: [job for job in jobs if job.get('name') == name] for name in JOBS}
    require(all(len(matches) <= 1 for matches in selected.values()), 'Duplicate publication jobs')
    if any(not matches for matches in selected.values()):
        return 'pending'
    ordered = [selected[name][0] for name in JOBS]
    known = {'success', 'failure', 'cancelled', 'skipped', 'timed_out', 'action_required', 'neutral', 'stale'}
    if any(job.get('status') != 'completed' or job.get('conclusion') not in known for job in ordered):
        return 'pending'
    require(conclusion != 'success' or not any(job.get('conclusion') in ('failure', 'cancelled', 'timed_out', 'action_required') for job in ordered),
            'Successful run contradicts failed publication jobs')
    if event == 'workflow_run' and conclusion == 'success' and all(completed(job, 'skipped') for job in ordered[1:]):
        return 'not-published'
    if not all(completed(job) for job in ordered[:4]):
        return 'incomplete'
    return 'promoted'


def executed(job, name):
    steps = [step for step in job.get('steps', []) if step.get('name') == name]
    if not steps:
        raise Pending('Required executed step evidence unavailable')
    require(len(steps) == 1 and completed(steps[0]), 'Required publication step did not execute successfully')


def attempt_evidence(run, jobs):
    """Retain only the exact attempt and executed native/promotion provenance."""
    selected = []
    for name, step in ((JOBS[1], SMOKE), (JOBS[2], SMOKE), (JOBS[3], PROMOTE)):
        matches = [job for job in jobs if job.get('name') == name]
        require(len(matches) == 1, 'Expected exact native/promotion job')
        job = matches[0]
        require(completed(job) and isinstance(job.get('id'), int) and job['id'] > 0
                and job.get('run_id') == run['id'] and job.get('head_sha') == run['head_sha'],
                'Native/promotion job identity mismatch')
        executed(job, step)
        selected.append({**{key: job[key] for key in ('id', 'name', 'run_id', 'head_sha', 'status', 'conclusion')},
                         'steps': [{key: item[key] for key in ('name', 'status', 'conclusion')}
                                   for item in job['steps'] if item.get('name') == step]})
    return {**{key: run[key] for key in ('id', 'run_attempt', 'run_number', 'head_sha', 'event', 'path')},
            'jobs': selected}


def durable_receipt(bundle, run_id, attempt, run, jobs, api, evidence_ref=None):
    path = docs.metadata_path(bundle)
    if evidence_ref is not None:
        comparison = api.get('/compare/' + evidence_ref + '...main')
        require(comparison.get('merge_base_commit', {}).get('sha') == evidence_ref,
                'Durable evidence revision is not on official main')
    file = api.get('/contents/' + path + '?ref=' + (evidence_ref or 'main'))
    require(isinstance(file, dict), 'Invalid tracked metadata response')
    require(file.get('type') == 'file' and file.get('path') == path
            and file.get('encoding') == 'base64', 'Expected tracked publication metadata')
    require(isinstance(file.get('content'), str) and len(file['content']) <= 90000,
            'Invalid or oversized tracked metadata content')
    raw = base64.b64decode(''.join(file['content'].split()), validate=True)
    require(len(raw) <= 65536, 'Oversized tracked publication metadata')
    metadata = json.loads(raw)
    require(isinstance(metadata, dict), 'Tracked publication metadata must be an object')
    evidence = metadata.get('publication_evidence')
    if not evidence or metadata.get('run_id') != run_id:
        raise Pending('Exact durable publication evidence unavailable')
    require(isinstance(evidence, dict), 'Durable publication evidence must be an object')
    stored_attempt = evidence['attempt']
    require(isinstance(stored_attempt, dict), 'Durable attempt must be an object')
    if stored_attempt.get('run_attempt') != int(attempt):
        raise Pending('Exact durable publication attempt unavailable')
    require(stored_attempt == attempt_evidence(run, jobs), 'Durable attempt provenance mismatch')
    receipt = evidence['receipt']
    require(isinstance(receipt, dict), 'Durable receipt must be an object')
    require(metadata.get('bundle') == bundle and all(metadata.get(key) == receipt.get(key)
            for key in ('tag', 'digest', 'commit', 'image', 'run_id')), 'Durable metadata differs from receipt')
    return receipt


def receipt_from_archive(raw):
    require(len(raw) <= LIMIT, 'Oversized artifact archive')
    with zipfile.ZipFile(io.BytesIO(raw)) as archive:
        entries = archive.infolist()
        require(len(entries) == 1 and entries[0].filename == 'publication.json'
                and entries[0].file_size <= 65536 and not entries[0].flag_bits & 1,
                'Expected only bounded canonical publication.json')
        return json.loads(archive.read(entries[0]))


def verify_tag(record, api):
    ref = api.get('/git/ref/tags/' + tags.tag_name(record))
    require(ref.get('ref') == 'refs/tags/' + tags.tag_name(record)
            and ref.get('object', {}).get('type') == 'tag', 'Source tag must be annotated')
    oid = approval.sha(ref['object']['sha'])
    tag = api.get('/git/tags/' + oid)
    require(tag.get('sha') == oid and tag.get('tag') == tags.tag_name(record)
            and tag.get('object') == {'sha': record['commit'], 'type': 'commit',
                                     'url': 'https://api.github.com/repos/' + REPOSITORY + '/git/commits/' + record['commit']}
            and tag.get('message') == tags.annotation(record), 'Source tag provenance mismatch')


def verify_receipt_registry(record, receipt, registry):
    tags.verify_registry(record, registry)
    index = tags.decode_digest(registry.get('manifests', record['digest']), record['digest'])
    platforms = {p['architecture']: p for p in receipt['platforms']}
    for descriptor in index['manifests']:
        platform = platforms[descriptor['platform']['architecture']]
        require(descriptor['digest'] == platform['digest'], 'Registry platform differs from smoked receipt')
        manifest = tags.decode_digest(registry.get('manifests', descriptor['digest']), descriptor['digest'])
        require(manifest['config']['digest'] == platform['image_id'], 'Registry config differs from smoked image')
        config = tags.decode_digest(registry.get('blobs', manifest['config']['digest']), manifest['config']['digest'])
        labels = config['config']['Labels']
        require(labels.get('io.bundledock.tools.lock-sha256') == receipt['lock_sha256']
                and labels.get('org.opencontainers.image.created') == receipt['created'],
                'Registry lock or build time differs from receipt')


def validate_attempt(run, selection, revision, run_id, attempt):
    require(run.get('id') == int(run_id) and run.get('run_attempt') == int(attempt)
            and run.get('repository', {}).get('full_name') == REPOSITORY
            and run.get('head_repository', {}).get('full_name') == REPOSITORY
            and run.get('path') == '.github/workflows/' + selection['workflow']
            and run.get('head_branch') == 'main'
            and run.get('event') in ('workflow_dispatch', 'workflow_run'), 'Workflow attempt identity mismatch')
    # Automatic run head is the controller, not necessarily the approved image source.
    approval.sha(run.get('head_sha'))
    if run['event'] == 'workflow_dispatch':
        require(run['head_sha'] == revision, 'Manual run source mismatch')


def capture_evidence(bundle, revision, run_id, attempt, receipt, api):
    """Freeze verified receipt/provenance before the documentation proposal."""
    docs.validate(receipt, bundle, receipt['image'], revision, run_id, attempt)
    run = api.get('/actions/runs/' + run_id + '/attempts/' + attempt)
    validate_attempt(run, approval.config.select_bundle(bundle), revision, run_id, attempt)
    jobs = api.listing('/actions/runs/' + run_id + '/attempts/' + attempt + '/jobs', 'jobs')
    evidence = attempt_evidence(run, jobs)
    tag_jobs = [job for job in jobs if job.get('name') == JOBS[4]]
    require(len(tag_jobs) == 1 and completed(tag_jobs[0])
            and tag_jobs[0].get('run_id') == int(run_id) and tag_jobs[0].get('head_sha') == run['head_sha'],
            'Expected successful source tag job')
    executed(tag_jobs[0], TAG)
    record = tags.validate_record(tags.bind_publication(receipt, bundle))
    require(record['attempt'] == int(attempt) and (record['run_number'] is None or record['run_number'] == run.get('run_number')),
            'Receipt attempt/run number mismatch')
    comparison = api.get('/compare/' + revision + '...main')
    require(comparison.get('merge_base_commit', {}).get('sha') == revision, 'Published source is not on official main')
    verify_tag(record, api)
    return dict(receipt=receipt, attempt=evidence)


def reconcile(bundle, revision, run_id, attempt, api, registry_factory=tags.Registry, evidence_ref=None):
    approval.sha(revision)
    if evidence_ref is not None:
        approval.sha(evidence_ref)
    require(re.fullmatch('[1-9][0-9]*', run_id or '') and re.fullmatch('[1-9][0-9]*', attempt or ''),
            'Exact positive run ID and attempt required')
    selection = approval.config.select_bundle(bundle)
    path = '/actions/runs/' + run_id + '/attempts/' + attempt
    run = api.get(path)
    validate_attempt(run, selection, revision, run_id, attempt)
    result = dict(bundle=bundle, requested_source_sha=revision, controller_sha=run['head_sha'], run_id=run_id, run_attempt=attempt,
                  event=run['event'], conclusion=run.get('conclusion'), state='pending')
    if run.get('status') != 'completed' or run.get('conclusion') not in ('success', 'failure', 'cancelled', 'timed_out', 'action_required', 'neutral', 'stale'):
        return result
    jobs = api.listing(path + '/jobs', 'jobs')
    require(all(job.get('run_id') == int(run_id) and job.get('head_sha') == run['head_sha']
                for job in jobs), 'Job source/run mismatch')
    state = job_state(jobs, run['event'], run['conclusion'])
    result.update(state=state, jobs={job['name']: job.get('conclusion') for job in jobs})
    if state in ('pending', 'incomplete'):
        return result
    try:
        artifacts = api.listing('/actions/runs/' + run_id + '/artifacts', 'artifacts')
    except Pending:
        if state == 'not-published':
            raise
        artifacts = []
    matches = [item for item in artifacts if item.get('name') == bundle + '-publication-' + attempt]
    require(len(matches) <= 1, 'Ambiguous canonical publication artifact')
    if state == 'not-published':
        require(not matches, 'Skipped publisher has a publication receipt')
        try:
            durable_receipt(bundle, run_id, attempt, run, jobs, api, evidence_ref)
        except Pending:
            return result
        raise ValueError('Skipped publisher has durable publication evidence')
    by_name = {job['name']: job for job in jobs}
    for name in JOBS[1:3]:
        executed(by_name[name], SMOKE)
    executed(by_name[JOBS[3]], PROMOTE)
    unavailable = object()
    receipt = unavailable
    if matches:
        artifact = matches[0]
        require(artifact.get('workflow_run', {}).get('id') == int(run_id)
                and artifact['workflow_run'].get('head_sha') == run['head_sha']
                and isinstance(artifact.get('id'), int) and artifact['id'] > 0,
                'Artifact run/source identity mismatch')
        if not isinstance(artifact.get('expired'), bool):
            raise Pending('Unknown artifact expiry state')
        if not artifact['expired']:
            try:
                receipt = receipt_from_archive(api.raw('/actions/artifacts/' + str(artifact['id']) + '/zip'))
            except Pending:
                pass
    if receipt is unavailable:
        receipt = durable_receipt(bundle, run_id, attempt, run, jobs, api, evidence_ref)
    image = 'quay.io/bundledock/' + selection['repository']
    docs.validate(receipt, bundle, image, revision, run_id, attempt)
    record = tags.validate_record(tags.bind_publication(receipt, bundle))
    require(record['attempt'] == int(attempt) and (record['run_number'] is None or record['run_number'] == run.get('run_number')),
            'Receipt attempt/run number mismatch')
    comparison = api.get('/compare/' + revision + '...main')
    require(comparison.get('merge_base_commit', {}).get('sha') == revision, 'Published source is not on official main')
    verify_receipt_registry(record, receipt, registry_factory(image))
    result.update(state='published-image', source_sha=revision, image=receipt['reference'], version=receipt['tag'],
                  source_tag='pending', documentation='pending')
    if completed(by_name[JOBS[4]]):
        executed(by_name[JOBS[4]], TAG)
        verify_tag(record, api)
        result.update(state='published-image-and-tag', source_tag='verified')
    else:
        result['source_tag'] = by_name[JOBS[4]].get('conclusion')
    if completed(by_name[JOBS[5]]):
        require(result['source_tag'] == 'verified', 'Documentation succeeded without verified source tag')
        executed(by_name[JOBS[5]], DOCUMENTATION)
        result.update(state='published', documentation='pr-step-succeeded')
    else:
        result['documentation'] = by_name[JOBS[5]].get('conclusion')
    return result


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--bundle', required=True, choices=tags.BUNDLES)
    parser.add_argument('--sha', required=True)
    parser.add_argument('--run-id', required=True)
    parser.add_argument('--attempt', required=True)
    parser.add_argument('--evidence-ref', help='Full official-main ancestor SHA holding historical publication metadata')
    args = parser.parse_args()
    try:
        result = reconcile(args.bundle, args.sha, args.run_id, args.attempt, GitHub(), evidence_ref=args.evidence_ref)
        print(json.dumps(result, sort_keys=True))
        return 0 if result['state'] in ('published', 'not-published') else 2
    except Pending:
        print(json.dumps({'state': 'pending', 'reason': 'Required evidence unavailable'}))
        return 2
    except (ValueError, KeyError, TypeError, zipfile.BadZipFile):
        # Never echo response bodies, URLs, subprocess diagnostics or tokens.
        print(json.dumps({'state': 'rejected', 'reason': 'Evidence contradicts exact publication identity'}))
        return 1
    except OSError:
        print(json.dumps({'state': 'pending', 'reason': 'Required evidence unavailable'}))
        return 2


if __name__ == '__main__':
    sys.exit(main())
