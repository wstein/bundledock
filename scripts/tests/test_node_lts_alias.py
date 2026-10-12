"""Offline tests: no registry writes or builds."""
import hashlib
import io
import importlib.util
import json
import os
from pathlib import Path
import subprocess
import unittest
from unittest.mock import patch
import urllib.error

ROOT = Path(__file__).resolve().parents[2]
SPEC = importlib.util.spec_from_file_location('node_lts_alias', ROOT / 'scripts/node-lts-alias.py')
alias = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(alias)


def encoded(value):
    return json.dumps(value).encode()


def digest(data):
    return 'sha256:' + hashlib.sha256(data).hexdigest()


class FakeRegistry:
    def __init__(self, architectures=('amd64', 'arm64'), labels=None, config_arch=None):
        self.data = {}
        self.writes = []
        descriptors = []
        for arch in architectures:
            config = encoded({'os': 'linux', 'architecture': config_arch or arch, 'config': {'Labels': {
                'org.opencontainers.image.source': alias.tags.SOURCE,
                'org.opencontainers.image.revision': alias.RECORD['commit'],
                'org.opencontainers.image.version': alias.RECORD['tag'],
                'io.bundledock.github.run-id': alias.RECORD['run_id'],
                'io.bundledock.github.run-attempt': '1', **(labels or {}),
            }}})
            self.data['blobs', digest(config)] = config
            manifest = encoded({'config': {'digest': digest(config)}})
            self.data['manifests', digest(manifest)] = manifest
            descriptors.append({'platform': {'os': 'linux', 'architecture': arch}, 'digest': digest(manifest)})
        self.index = encoded({'schemaVersion': 2, 'mediaType': alias.INDEX_TYPE, 'manifests': descriptors})
        self.digest = digest(self.index)
        for ref in (alias.RECORD['tag'], self.digest, 'stable', 'latest'):
            self.data['manifests', ref] = self.index

    def get(self, kind, ref):
        if (kind, ref) not in self.data:
            raise urllib.error.HTTPError('https://quay.io', 404, 'missing', {}, io.BytesIO(encoded({'errors': [{'code': 'MANIFEST_UNKNOWN'}]})))
        return self.data[kind, ref]

    def put_lts(self, data):
        self.writes.append(data)
        self.data['manifests', 'lts'] = data


class AliasTests(unittest.TestCase):
    def setUp(self):
        self.registry = FakeRegistry()
        self.record = patch.dict(alias.RECORD, digest=self.registry.digest)
        self.record.start()
        self.addCleanup(self.record.stop)

    def test_fixed_release_identity(self):
        self.assertEqual(alias.RECORD['commit'], '9f25e0065885dce912658986bf26c178dddbaf36')
        self.assertEqual(alias.RECORD['tag'], '2026.10.12-b1.1')
        self.assertEqual(alias.RECORD['run_id'], '38192775173')
        self.assertEqual(alias.RECORD['image'], 'quay.io/bundledock/node')

    def test_copy_exact_index_only_to_lts_and_idempotent(self):
        alias.promote(self.registry)
        self.assertEqual(self.registry.writes, [self.registry.index])
        alias.promote(self.registry)
        self.assertEqual(len(self.registry.writes), 1)

    def test_conflicting_source_or_alias_blocks_write(self):
        for ref in (alias.RECORD['tag'], 'stable', 'latest', 'lts'):
            with self.subTest(ref=ref):
                registry = FakeRegistry()
                registry.data['manifests', ref] = b'{}'
                with self.assertRaises(ValueError):
                    alias.promote(registry)
                self.assertEqual(registry.writes, [])

    def test_both_platforms_and_provenance_required(self):
        cases = [dict(architectures=('amd64',)), dict(architectures=('amd64', 'amd64')),
                 dict(architectures=('amd64', 's390x')), dict(config_arch='arm64')]
        for key in ('org.opencontainers.image.source', 'org.opencontainers.image.revision',
                    'org.opencontainers.image.version', 'io.bundledock.github.run-id',
                    'io.bundledock.github.run-attempt', 'io.bundledock.bundle'):
            cases.append(dict(labels={key: 'wrong'}))
        for case in cases:
            with self.subTest(case=case):
                registry = FakeRegistry(**case)
                with patch.dict(alias.RECORD, digest=registry.digest), self.assertRaises(ValueError):
                    alias.promote(registry)
                self.assertEqual(registry.writes, [])

    def test_stale_revalidation_and_malformed_404_block_write(self):
        def stale():
            raise ValueError('main advanced')
        with self.assertRaises(ValueError):
            alias.promote(self.registry, before_write=stale)
        self.assertEqual(self.registry.writes, [])
        get = self.registry.get
        for body in (b'{}', b'not json', encoded({'errors': [{'code': 'UNAUTHORIZED'}]})):
            def missing(kind, ref):
                if ref == 'lts':
                    raise urllib.error.HTTPError('https://quay.io', 404, 'error', {}, io.BytesIO(body))
                return get(kind, ref)
            self.registry.get = missing
            with self.assertRaises((ValueError, urllib.error.HTTPError)):
                alias.promote(self.registry)
            self.assertEqual(self.registry.writes, [])

    def test_auth_and_other_errors_never_mean_absent(self):
        for code in (401, 403, 429, 500, 302):
            registry = FakeRegistry()
            get = registry.get
            def failed(kind, ref):
                if ref == 'lts':
                    raise urllib.error.HTTPError('https://quay.io', code, 'error', {}, None)
                return get(kind, ref)
            registry.get = failed
            with self.assertRaises(urllib.error.HTTPError):
                alias.promote(registry)
            self.assertEqual(registry.writes, [])

    def test_preserved_aliases_checked_after_write(self):
        def changed(data):
            self.registry.data['manifests', 'lts'] = data
            self.registry.data['manifests', 'stable'] = b'{}'
        self.registry.put_lts = changed
        with self.assertRaises(ValueError):
            alias.promote(self.registry)

    def test_registry_rejects_arbitrary_sources_targets_and_redirects(self):
        with patch.dict(os.environ, REPOSITORY_USERNAME='fake', REPOSITORY_TOKEN='fake'), patch.object(alias.tags, 'request', return_value=b'{"token":"test"}') as request:
            registry = alias.Registry()
        self.assertIn('repository%3Abundledock%2Fnode%3Apull%2Cpush', request.call_args.args[0])
        for kind, ref in (('manifests', 'current'), ('manifests', 'evil/path'), ('other', 'lts')):
            with self.assertRaises(ValueError):
                registry.get(kind, ref)
        with patch.object(alias.urllib.request, 'build_opener') as opener:
            response = opener.return_value.open.return_value.__enter__.return_value
            response.status = 201
            response.headers = {'Docker-Content-Digest': self.registry.digest}
            registry.put_lts(self.registry.index)
            req = opener.return_value.open.call_args.args[0]
            self.assertEqual(req.full_url, 'https://quay.io/v2/bundledock/node/manifests/lts')
            self.assertEqual(req.method, 'PUT')
            self.assertEqual(req.data, self.registry.index)
            self.assertIsInstance(opener.call_args.args[0], alias.tags.NoRedirect)
        with patch.object(alias.tags, 'request', return_value=b'{}') as request:
            registry.get('blobs', 'sha256:' + 'a'*64)
        self.assertTrue(request.call_args.kwargs['signed_blob'])

    def test_signed_blob_redirect_never_forwards_registry_credentials(self):
        for target in ('http://cdn01.quay.io/blob', 'https://user:pass@cdn01.quay.io/blob',
                       'https://cdn01.quay.io:443/blob'):
            error = urllib.error.HTTPError('https://quay.io', 302, 'redirect', {'Location': target}, None)
            with patch.object(alias.tags.urllib.request, 'build_opener') as opener:
                opener.return_value.open.side_effect = error
                with self.assertRaises(ValueError):
                    alias.tags.request('https://quay.io/v2/bundledock/node/blobs/test',
                                       {'Authorization': 'Bearer secret'}, signed_blob=True)
                self.assertEqual(opener.return_value.open.call_count, 1)
        target = 'https://cdn01.quay.io/blob?public-test=1'
        error = urllib.error.HTTPError('https://quay.io', 302, 'redirect', {'Location': target}, None)
        with patch.object(alias.tags.urllib.request, 'build_opener') as opener:
            response = opener.return_value.open.return_value
            response.read.return_value = b'config'
            opener.return_value.open.side_effect = [error, response]
            self.assertEqual(alias.tags.request('https://quay.io/v2/bundledock/node/blobs/test',
                             {'Authorization': 'Bearer secret'}, signed_blob=True), b'config')
            self.assertEqual(opener.return_value.open.call_args.args, (target,))
            self.assertEqual(opener.return_value.open.call_args.kwargs, {'timeout': 45})

    def test_workflow_is_manual_and_has_no_build_or_other_writes(self):
        workflow = (ROOT / '.github/workflows/node-lts-alias.yml').read_text()
        self.assertIn('workflow_dispatch:', workflow)
        self.assertIn('group: bundledock-node-publish', workflow)
        self.assertIn('inputs.approved_sha == github.sha', workflow)
        for forbidden in ('workflow_run:', 'schedule:', 'contents: write', 'docker ', 'go build', 'make ', '--write'):
            self.assertNotIn(forbidden, workflow)

    def test_guard_rejects_stale_main_and_nonmanual_context(self):
        env = dict(GITHUB_EVENT_NAME='workflow_dispatch', DEFAULT_BRANCH='main',
                   GITHUB_REF='refs/heads/main', SENDER_TYPE='User', GITHUB_ACTOR='wstein',
                   TRIGGERING_ACTOR='wstein', APPROVED_SHA='a'*40, GITHUB_SHA='a'*40,
                   GITHUB_REPOSITORY='wstein/bundledock')
        with patch.dict(os.environ, env, clear=True), patch.object(alias.tags, 'request', return_value=encoded({'object': {'sha': 'a'*40}})):
            alias.guard()
        for field, value in (('APPROVED_SHA', 'a'*7), ('GITHUB_EVENT_NAME', 'push'),
                             ('GITHUB_REPOSITORY', 'other/repo'), ('GITHUB_SHA', 'b'*40)):
            with self.subTest(field=field), patch.dict(os.environ, dict(env, **{field: value}), clear=True):
                with self.assertRaises((ValueError, subprocess.CalledProcessError)):
                    alias.guard()
        with patch.dict(os.environ, env, clear=True), patch.object(alias.tags, 'request', return_value=encoded({'object': {'sha': 'b'*40}})):
            with self.assertRaises(ValueError):
                alias.guard()


if __name__ == '__main__':
    unittest.main()
