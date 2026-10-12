#!/usr/bin/env python3
"""Manual, one-release repair: copy the published Node index to :lts only.

No builds, Git tags, configurable repository/digest, or other alias writes.
The shared Node publisher concurrency group serializes this workflow with both
channels. External writers must remain quiescent; registry tags have no CAS.
"""
import base64
import importlib.util
import json
import os
from pathlib import Path
import subprocess
import urllib.error
import urllib.parse
import urllib.request

_SPEC = importlib.util.spec_from_file_location('bundle_tags', Path(__file__).with_name('bundle-git-tags.py'))
tags = importlib.util.module_from_spec(_SPEC)
_SPEC.loader.exec_module(tags)
INDEX_TYPE = 'application/vnd.oci.image.index.v1+json'
INDEX_TYPES = (INDEX_TYPE, 'application/vnd.docker.distribution.manifest.list.v2+json')
RECORD = dict(bundle='node', image='quay.io/bundledock/node', source=tags.SOURCE,
              tag='2026.10.12-b1.1', commit='9f25e0065885dce912658986bf26c178dddbaf36',
              digest='sha256:295cce35e9f580db41dfdc97bd30243d9f0cf388556a8a8d87a6c59ee51e6a8c',
              run_id='38192775173', run_attempt='1')


def guard():
    if os.environ.get('GITHUB_REPOSITORY') != 'wstein/bundledock':
        raise ValueError('official repository required')
    subprocess.run(['sh', str(Path(__file__).with_name('approve-publication.sh'))], check=True)
    headers = {'Accept': 'application/vnd.github+json', 'User-Agent': 'bundledock-node-lts-alias'}
    if os.environ.get('GITHUB_TOKEN'):
        headers['Authorization'] = 'Bearer ' + os.environ['GITHUB_TOKEN']
    main = json.loads(tags.request('https://api.github.com/repos/wstein/bundledock/git/ref/heads/main', headers))
    if main['object']['sha'] != os.environ['APPROVED_SHA']:
        raise ValueError('approved execution revision is no longer main')


class Registry:
    """Fixed Quay manifests/PUT; signed blob reads never forward credentials."""
    def __init__(self):
        user, password = os.environ['REPOSITORY_USERNAME'], os.environ['REPOSITORY_TOKEN']
        if not user or not password:
            raise ValueError('registry credentials required')
        headers = {'Authorization': 'Basic ' + base64.b64encode((user + ':' + password).encode()).decode()}
        query = urllib.parse.urlencode({'service': 'quay.io', 'scope': 'repository:bundledock/node:pull,push'})
        self.token = json.loads(tags.request('https://quay.io/v2/auth?' + query, headers))['token']
        if not isinstance(self.token, str) or not self.token:
            raise ValueError('registry token missing')

    def get(self, kind, ref):
        if kind not in ('manifests', 'blobs') or not (tags.DIGEST.fullmatch(ref) or ref in (RECORD['tag'], 'stable', 'latest', 'lts')):
            raise ValueError('unexpected metadata reference')
        headers = {'Authorization': 'Bearer ' + self.token, 'Accept': ', '.join(INDEX_TYPES) + ', application/vnd.oci.image.manifest.v1+json, application/vnd.docker.distribution.manifest.v2+json'}
        # Reuse the provenance reader for public signed blobs, without sending
        # registry credentials to storage. Auth/manifests/PUT never redirect.
        return tags.request(f'https://quay.io/v2/bundledock/node/{kind}/{ref}', headers, signed_blob=kind == 'blobs')

    def put_lts(self, data):
        index = tags.decode_digest(data, RECORD['digest'])
        if index.get('mediaType') not in INDEX_TYPES:
            raise ValueError('expected image index')
        headers = {'Authorization': 'Bearer ' + self.token, 'Content-Type': index['mediaType']}
        request = urllib.request.Request('https://quay.io/v2/bundledock/node/manifests/lts',
                                         data=data, headers=headers, method='PUT')
        with urllib.request.build_opener(tags.NoRedirect()).open(request, timeout=45) as response:
            if response.status != 201 or response.headers.get('Docker-Content-Digest') != RECORD['digest']:
                raise ValueError('registry did not confirm exact index copy')


def promote(registry, before_write=lambda: None):
    r = tags.validate_record(RECORD)
    tags.verify_registry(r, registry)
    data = registry.get('manifests', r['digest'])
    index = tags.decode_digest(data, r['digest'])
    if index.get('schemaVersion') != 2 or index.get('mediaType') not in INDEX_TYPES:
        raise ValueError('expected image index')
    for descriptor in index['manifests']:
        manifest = tags.decode_digest(registry.get('manifests', descriptor['digest']), descriptor['digest'])
        config = tags.decode_digest(registry.get('blobs', manifest['config']['digest']), manifest['config']['digest'])
        if config['config']['Labels'].get('io.bundledock.github.run-attempt') != '1':
            raise ValueError('OCI workflow attempt mismatch')
    for ref in ('stable', 'latest'):
        tags.decode_digest(registry.get('manifests', ref), r['digest'])
    absent = False
    try:
        tags.decode_digest(registry.get('manifests', 'lts'), r['digest'])
    except urllib.error.HTTPError as error:
        # Only an explicit missing manifest permits a write. Auth, redirects,
        # backend errors, malformed responses and other 404s remain failures.
        if error.code != 404:
            raise
        errors = json.loads(error.read(8192)).get('errors')
        if not isinstance(errors, list) or len(errors) != 1 or errors[0].get('code') != 'MANIFEST_UNKNOWN':
            raise
        absent = True
    before_write()
    if absent:
        registry.put_lts(data)
    for ref in (r['tag'], r['digest'], 'stable', 'latest', 'lts'):
        tags.decode_digest(registry.get('manifests', ref), r['digest'])
    return 'copied' if absent else 'already present'


def main():
    guard()
    tags.verify_run(tags.validate_record(RECORD), backfill=True)
    print(promote(Registry(), before_write=guard))


if __name__ == '__main__':
    try:
        main()
    except (ValueError, KeyError, TypeError, AttributeError, subprocess.CalledProcessError, urllib.error.URLError):
        # No credentials, response bodies or signed URLs in diagnostics.
        raise SystemExit('Node LTS alias verification/copy failed; no automatic retry')
