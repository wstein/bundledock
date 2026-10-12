#!/usr/bin/env python3
"""Verify deployed OCI provenance, then create immutable annotated Git tags.

Default invocation is read-only. --write explicitly creates a remote tag;
this script never builds, publishes an image, moves aliases, or overwrites tags.
"""
import argparse
import base64
import hashlib
import importlib.util
from pathlib import Path
import json
import os
import re
import subprocess
import urllib.parse
import urllib.request
import uuid

SOURCE = 'https://github.com/wstein/bundledock'
_SPEC=importlib.util.spec_from_file_location('bundle_config',Path(__file__).with_name('bundle-config.py'))
_CONFIG=importlib.util.module_from_spec(_SPEC)
_SPEC.loader.exec_module(_CONFIG)
BUNDLES=tuple(_CONFIG.BUNDLES)
DIGEST = re.compile(r'sha256:[0-9a-f]{64}\Z')
VERSION = re.compile(r'(\d{4}\.\d{2}\.\d{2})-b([1-9]\d*)\.([1-9]\d*)\Z')
SINGLETON = re.compile(r'(?:0|[1-9]\d*)\.(?:0|[1-9]\d*)\.(?:0|[1-9]\d*)-b[1-9]\d*\Z')
LEGACY = re.compile(r'(\d{4}\.\d{2}\.\d{2})-([1-9]\d*)-([1-9]\d*)\Z')


def validate_record(record, legacy=False):
    r = dict(record)
    if r.get('bundle') not in BUNDLES or r.get('image') != 'quay.io/bundledock/'+_CONFIG.select_bundle(r['bundle'])['repository']:
        raise ValueError('unexpected official bundle repository')
    if r.get('source') != SOURCE or not re.fullmatch('[0-9a-f]{40}', r.get('commit', '')):
        raise ValueError('unexpected source repository or revision')
    if not DIGEST.fullmatch(r.get('digest', '')) or not re.fullmatch('[1-9][0-9]*', r.get('run_id', '')):
        raise ValueError('invalid index digest or workflow run')
    r.update(validate_version(r.get('tag', ''), r['run_id'], legacy))
    if SINGLETON.fullmatch(r['tag']):
        if r['bundle'] == 'toolbox' or not isinstance(r.get('run_attempt'), str) or not re.fullmatch('[1-9][0-9]*', r['run_attempt']):
            raise ValueError('singleton version requires explicit workflow attempt')
        r['attempt'] = int(r['run_attempt'])
    return r


def validate_version(tag, run_id, legacy=False):
    """Validate immutable version identity independently of deployment namespace."""
    if SINGLETON.fullmatch(tag):
        return {'legacy': False, 'attempt': None, 'run_number': None}
    match = VERSION.fullmatch(tag)
    fields = {'legacy': False}
    if not match and legacy:
        match = LEGACY.fullmatch(tag)
        fields['legacy'] = True
        if match and match[2] != run_id:
            raise ValueError('legacy run ID mismatch')
    if not match:
        raise ValueError('expected an immutable version tag')
    import datetime
    datetime.datetime.strptime(match[1], '%Y.%m.%d')
    fields['attempt'] = int(match[3])
    fields['run_number'] = None if fields['legacy'] else int(match[2])
    return fields


def tag_name(r):
    return r['bundle']+'/'+r['tag']


def annotation(r):
    return (f"BundleDock {tag_name(r)}\n\n"
            f"Image: {r['image']}:{r['tag']}@{r['digest']}\n"
            f"Index-Digest: {r['digest']}\n"
            f"Source: {r['source']}\nRevision: {r['commit']}\n"
            f"Workflow: {SOURCE}/actions/runs/{r['run_id']}/attempts/{r['attempt']}\n")


def decode_digest(data, digest):
    if not DIGEST.fullmatch(digest) or 'sha256:'+hashlib.sha256(data).hexdigest() != digest:
        raise ValueError('registry content digest mismatch')
    return json.loads(data)


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, *args, **kwargs):
        return None


def request(url, headers=None, signed_blob=False):
    req = urllib.request.Request(url, headers=headers or {})
    try:
        response = urllib.request.build_opener(NoRedirect).open(req, timeout=45)
    except urllib.error.HTTPError as error:
        # Registry blobs may redirect to signed object storage. Send no secrets
        # to the new host and never permit an HTTP downgrade or URL credentials.
        if not signed_blob or error.code not in (301, 302, 303, 307, 308):
            raise
        target = urllib.parse.urlsplit(error.headers['Location'])
        if target.scheme != 'https' or not target.hostname or target.username or target.password or target.port is not None:
            raise ValueError('unsafe signed blob redirect') from error
        response = urllib.request.build_opener(NoRedirect).open(target.geturl(), timeout=45)
    with response:
        data = response.read(8*1024*1024+1)
    if len(data) > 8*1024*1024:
        raise ValueError('registry metadata exceeds size limit')
    return data


class Registry:
    def __init__(self, image):
        self.path = image.removeprefix('quay.io/')
        headers = {}
        user, password = os.environ.get('REPOSITORY_USERNAME'), os.environ.get('REPOSITORY_TOKEN')
        if user and password:
            headers['Authorization'] = 'Basic '+base64.b64encode((user+':'+password).encode()).decode()
        url = 'https://quay.io/v2/auth?'+urllib.parse.urlencode({'service':'quay.io', 'scope':'repository:'+self.path+':pull'})
        self.token = json.loads(request(url, headers))['token']

    def get(self, kind, ref):
        if kind not in ('manifests', 'blobs') or not (DIGEST.fullmatch(ref) or VERSION.fullmatch(ref) or SINGLETON.fullmatch(ref) or LEGACY.fullmatch(ref) or ref in ('stable', 'latest', 'lts', 'current')):
            raise ValueError('invalid registry metadata request')
        headers = {'Authorization':'Bearer '+self.token, 'Accept':'application/vnd.oci.image.index.v1+json, application/vnd.docker.distribution.manifest.list.v2+json, application/vnd.oci.image.manifest.v1+json, application/vnd.docker.distribution.manifest.v2+json'}
        return request(f'https://quay.io/v2/{self.path}/{kind}/{ref}', headers, signed_blob=kind=='blobs')


def verify_labels(r, labels):
    identity=labels.get('io.bundledock.bundle')
    if (identity is not None and identity != r['bundle']) or (identity is None and (r['bundle']=='node-current' or (r['bundle']=='node' and SINGLETON.fullmatch(r['tag'])))):
        raise ValueError('OCI source bundle mismatch')
    if SINGLETON.fullmatch(r['tag']) and labels.get('io.bundledock.github.run-attempt') != str(r['attempt']):
        raise ValueError('OCI workflow attempt mismatch')
    for key, field in [('source','source'), ('revision','commit'), ('version','tag')]:
        if labels.get('org.opencontainers.image.'+key) != r[field]:
            raise ValueError('OCI '+key+' mismatch')
    run_id = labels.get('io.bundledock.github.run-id')
    if run_id != r['run_id'] and not (r['legacy'] and run_id is None):
        raise ValueError('OCI workflow run mismatch')


def verify_registry(r, registry, aliases=False):
    index = decode_digest(registry.get('manifests', r['tag']), r['digest'])
    # Fetch through the immutable name too; the version must resolve to it.
    decode_digest(registry.get('manifests', r['digest']), r['digest'])
    seen = set()
    for descriptor in index['manifests']:
        platform = descriptor['platform']
        arch = platform['architecture']
        if platform.get('os') != 'linux' or arch not in ('amd64','arm64') or arch in seen:
            raise ValueError('expected exactly linux amd64 and arm64')
        seen.add(arch)
        manifest = decode_digest(registry.get('manifests', descriptor['digest']), descriptor['digest'])
        config = decode_digest(registry.get('blobs', manifest['config']['digest']), manifest['config']['digest'])
        if config.get('os') != 'linux' or config.get('architecture') != arch:
            raise ValueError('OCI config platform mismatch')
        verify_labels(r, config['config']['Labels'])
    if seen != {'amd64','arm64'}:
        raise ValueError('missing required architecture')
    if aliases:
        for alias in _CONFIG.select_bundle(r['bundle'])['aliases'].split():
            decode_digest(registry.get('manifests', alias), r['digest'])


def verify_run(r, backfill):
    headers = {'Accept':'application/vnd.github+json', 'User-Agent':'bundledock-git-tags'}
    if os.environ.get('GITHUB_TOKEN'):
        headers['Authorization'] = 'Bearer '+os.environ['GITHUB_TOKEN']
    run = json.loads(request(f"https://api.github.com/repos/wstein/bundledock/actions/runs/{r['run_id']}/attempts/{r['attempt']}", headers))
    automatic = not backfill and run.get('event') == 'workflow_run'
    controller_sha = os.environ.get('GITHUB_SHA') if automatic else r['commit']
    if (run.get('head_sha') != controller_sha or run.get('head_branch') != 'main'
            or run.get('repository', {}).get('html_url') != SOURCE
            or run.get('path') != f".github/workflows/bundledock-{r['bundle']}.yml"
            or run.get('run_attempt') != r['attempt']
            or str(run.get('id')) != r['run_id']
            or (r['run_number'] is not None and run.get('run_number') != r['run_number'])):
        raise ValueError('workflow attempt provenance mismatch')
    if backfill:
        if run.get('status') != 'completed' or run.get('conclusion') != 'success':
            raise ValueError('backfill requires a successful completed publication')
    else:
        if (os.environ.get('GITHUB_SHA') != controller_sha or os.environ.get('GITHUB_RUN_ID') != r['run_id']
                or os.environ.get('GITHUB_RUN_ATTEMPT') != str(r['attempt'])):
            raise ValueError('future tagging requires the approved current workflow context')
        if run.get('event') == 'workflow_run':
            if (os.environ.get('GITHUB_EVENT_NAME') != 'workflow_run'
                    or os.environ.get('PUBLICATION_SHA') != r['commit']):
                raise ValueError('workflow_run requires the approved bundle source')
            # Reuse the owning source approval gate rather than inventing a
            # second approval mechanism. It is read-only and fails closed.
            environment = dict(os.environ)
            environment.pop('GITHUB_OUTPUT', None)
            environment.pop('GITHUB_STEP_SUMMARY', None)
            gate = subprocess.run(['python3', 'scripts/bundle-publication.py', '--bundle', r['bundle']],
                                  env=environment, capture_output=True, text=True)
            if gate.returncode:
                raise ValueError('approved bundle source revalidation failed')
        elif run.get('event') != 'workflow_dispatch':
            raise ValueError('future tagging requires an approved publication event')


def git(*args, input=None, check=True):
    env = dict(os.environ)
    # Credentials exist only in this subprocess's environment, never arguments,
    # files, persisted checkout configuration, or printed command diagnostics.
    token = env.get('GITHUB_TOKEN')
    if token:
        env.update(GIT_CONFIG_COUNT='1', GIT_CONFIG_KEY_0='http.https://github.com/.extraheader',
                   GIT_CONFIG_VALUE_0='AUTHORIZATION: basic '+base64.b64encode(('x-access-token:'+token).encode()).decode())
    result = subprocess.run(['git', *args], input=input, text=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE, env=env)
    if check and result.returncode:
        raise ValueError('Git operation failed: '+args[0])
    return result


def verify_source(r):
    for arguments in [('remote', 'get-url', '--all', 'origin'), ('remote', 'get-url', '--push', '--all', 'origin')]:
        remotes = git(*arguments).stdout.splitlines()
        if len(remotes) != 1 or remotes[0].removesuffix('.git') not in (SOURCE, 'git@github.com:wstein/bundledock'):
            raise ValueError('origin fetch and push URLs must be the official BundleDock repository')
    ref = 'refs/bundledock-verify/'+uuid.uuid4().hex
    try:
        git('fetch', '--no-tags', 'origin', 'refs/heads/main:'+ref)
        git('cat-file', '-e', r['commit']+'^{commit}')
        git('merge-base', '--is-ancestor', r['commit'], ref)
    finally:
        git('update-ref', '-d', ref)


def remote_tag(r):
    name = 'refs/tags/'+tag_name(r)
    found = git('ls-remote', '--refs', 'origin', name).stdout.strip()
    if not found:
        return False
    oid, actual = found.split()
    if actual != name:
        raise ValueError('unexpected remote ref')
    ref = 'refs/bundledock-verify/'+uuid.uuid4().hex
    try:
        git('fetch', '--no-tags', 'origin', name+':'+ref)
        if git('rev-parse', ref).stdout.strip() != oid:
            raise ValueError('remote tag changed during verification')
        raw = git('cat-file', 'tag', ref).stdout
        header, message = raw.split('\n\n', 1)
        fields = dict(line.split(' ', 1) for line in header.splitlines())
        if fields.get('object') != r['commit'] or fields.get('type') != 'commit' or fields.get('tag') != tag_name(r) or message != annotation(r):
            raise ValueError('existing remote tag conflicts with verified provenance')
        return True
    finally:
        git('update-ref', '-d', ref)


def create_tag(r):
    if remote_tag(r):
        return 'already present'
    identity = git('var', 'GIT_COMMITTER_IDENT').stdout.strip()
    raw = f"object {r['commit']}\ntype commit\ntag {tag_name(r)}\ntagger {identity}\n\n"+annotation(r)
    oid = git('mktag', input=raw).stdout.strip()
    # An explicit non-force tag refspec never replaces an existing remote tag.
    # A concurrent matching tag is accepted only after full semantic reread.
    git('push', '--no-follow-tags', 'origin', oid+':refs/tags/'+tag_name(r), check=False)
    if not remote_tag(r):
        raise ValueError('remote tag creation failed')
    return 'created or concurrently verified'


def bind_publication(record, bundle):
    """Bind the source selector without inferring a shared Node semantic channel."""
    if record.get('bundle', bundle) != bundle:
        raise ValueError('publication receipt belongs to another source bundle')
    required = bundle == 'node-current' or (bundle == 'node' and SINGLETON.fullmatch(record.get('tag', '')))
    if required:
        platforms = record.get('platforms')
        if (record.get('bundle') != bundle or not isinstance(platforms, list) or len(platforms) != 2
                or {p.get('architecture') for p in platforms if isinstance(p, dict)} != {'amd64', 'arm64'}
                or any(p.get('bundle') != bundle for p in platforms)):
            raise ValueError('Node semantic publication source identity missing or inconsistent')
    return dict(record, bundle=bundle)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--publication', type=argparse.FileType('r'))
    parser.add_argument('--bundle', required=True, choices=BUNDLES)
    for name in ('tag','digest','commit','run-id','run-attempt'):
        parser.add_argument('--'+name)
    parser.add_argument('--backfill', action='store_true')
    parser.add_argument('--legacy-version', action='store_true')
    parser.add_argument('--write', action='store_true')
    args = parser.parse_args()
    if args.publication:
        record = json.load(args.publication)
        record = bind_publication(record, args.bundle)
    else:
        record = dict(bundle=args.bundle, image='quay.io/bundledock/'+_CONFIG.select_bundle(args.bundle)['repository'],
                      source=SOURCE, tag=args.tag, digest=args.digest, commit=args.commit, run_id=args.run_id, run_attempt=args.run_attempt)
    if args.legacy_version and not args.backfill:
        parser.error('--legacy-version is restricted to backfill')
    r = validate_record(record, legacy=args.legacy_version)
    verify_source(r)
    verify_registry(r, Registry(r['image']), aliases=not args.backfill)
    verify_run(r, args.backfill)
    existing = remote_tag(r)
    print(json.dumps(dict(git_tag=tag_name(r), image=r['image']+':'+r['tag'], digest=r['digest'],
                          source_sha=r['commit'], workflow=f"{SOURCE}/actions/runs/{r['run_id']}/attempts/{r['attempt']}",
                          existing=existing), sort_keys=True))
    if args.write:
        print(create_tag(r))


if __name__ == '__main__':
    try:
        main()
    except (ValueError, KeyError, TypeError, urllib.error.URLError) as error:
        raise SystemExit('bundle tag verification failed: '+str(error))
