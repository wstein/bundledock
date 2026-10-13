import importlib.util
from pathlib import Path
import unittest

SPEC = importlib.util.spec_from_file_location('reconcile', Path(__file__).parents[1] / 'publication-reconcile.py')
reconcile = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(reconcile)


class PublicationReconcileTests(unittest.TestCase):
    def test_success_with_all_artifact_jobs_skipped_is_noop(self):
        jobs = [{'name': name, 'status': 'completed', 'conclusion': 'skipped'}
                for name in reconcile.JOBS]
        self.assertEqual(reconcile.job_state(jobs, 'workflow_run', 'success'), 'not-published')

    def test_missing_jobs_remain_pending(self):
        self.assertEqual(reconcile.job_state([], 'workflow_run', 'success'), 'pending')

import copy
import base64
import hashlib
import io
import json
from unittest import mock
import zipfile
import tempfile

SHA = 'a' * 40
CONTROLLER = 'b' * 40


def encoded(value):
    raw = json.dumps(value).encode()
    return raw, 'sha256:' + hashlib.sha256(raw).hexdigest()


def zipped(receipt, filename='publication.json'):
    buffer = io.BytesIO()
    with zipfile.ZipFile(buffer, 'w') as archive:
        archive.writestr(filename, json.dumps(receipt))
    return buffer.getvalue()


class FakeAPI:
    def __init__(self):
        self.run = dict(id=123, run_attempt=2, run_number=7, head_sha=SHA, head_branch='main',
                        repository={'full_name': reconcile.REPOSITORY},
                        head_repository={'full_name': reconcile.REPOSITORY},
                        path='.github/workflows/publish-bundles.yml',
                        event='workflow_dispatch', status='completed', conclusion='success')
        steps = {reconcile.JOBS[1]: reconcile.SMOKE, reconcile.JOBS[2]: reconcile.SMOKE,
                 reconcile.JOBS[3]: reconcile.PROMOTE, reconcile.JOBS[4]: reconcile.TAG,
                 reconcile.JOBS[5]: reconcile.DOCUMENTATION}
        self.jobs = [dict(id=100 + i, name=name, run_id=123, head_sha=SHA, status='completed', conclusion='success',
                          steps=[dict(name=steps.get(name, 'Resolve'), status='completed', conclusion='success')])
                     for i, name in enumerate(reconcile.JOBS)]
        self.artifacts = [dict(id=55, name='opentofu-publication-2', expired=False,
                               workflow_run={'id': 123, 'head_sha': SHA})]
        self.receipt = dict(bundle='opentofu', image='quay.io/bundledock/opentofu',
                            source=reconcile.tags.SOURCE, commit=SHA, tag='1.13.1-b1', run_id='123',
                            run_attempt='2', created='2026-10-12T01:00:00Z', lock_sha256='c' * 64)
        self.durable = None
        self.registry_data = {}
        platforms = []
        descriptors = []
        for arch in ('amd64', 'arm64'):
            labels = {'org.opencontainers.image.' + k: self.receipt[v] for k, v in
                      [('source', 'source'), ('revision', 'commit'), ('version', 'tag'), ('created', 'created')]}
            labels.update({'io.bundledock.bundle': 'opentofu', 'io.bundledock.github.run-id': '123',
                           'io.bundledock.github.run-attempt': '2',
                           'io.bundledock.tools.lock-sha256': self.receipt['lock_sha256']})
            config, config_digest = encoded(dict(os='linux', architecture=arch, config={'Labels': labels}))
            manifest, manifest_digest = encoded({'config': {'digest': config_digest}})
            self.registry_data['blobs', config_digest] = config
            self.registry_data['manifests', manifest_digest] = manifest
            platforms.append(dict(self.receipt, architecture=arch, smoked=True,
                                  image_id=config_digest, digest=manifest_digest))
            descriptors.append(dict(platform={'os': 'linux', 'architecture': arch}, digest=manifest_digest))
        index, digest = encoded({'manifests': descriptors})
        self.registry_data['manifests', self.receipt['tag']] = index
        self.registry_data['manifests', digest] = index
        self.receipt.update(digest=digest, reference=self.receipt['image'] + '@' + digest, platforms=platforms)
        record = reconcile.tags.validate_record(self.receipt)
        self.ref = {'ref': 'refs/tags/opentofu/1.13.1-b1', 'object': {'type': 'tag', 'sha': 'd' * 40}}
        self.tag = dict(sha='d' * 40, tag='opentofu/1.13.1-b1', message=reconcile.tags.annotation(record),
                        object=dict(type='commit', sha=SHA,
                                    url='https://api.github.com/repos/wstein/bundledock/git/commits/' + SHA))

    def get(self, path):
        if path.startswith('/contents/'):
            if self.durable is None:
                raise reconcile.Pending('No tracked receipt')
            return dict(type='file', path='docs/opentofu-published.json', encoding='base64',
                        content=base64.b64encode(json.dumps(self.durable).encode()).decode())
        if path.startswith('/compare/'):
            return {'merge_base_commit': {'sha': SHA}}
        if path.startswith('/git/ref/tags/'):
            return self.ref
        if path.startswith('/git/tags/'):
            return self.tag
        return self.run

    def listing(self, path, key):
        return self.jobs if key == 'jobs' else self.artifacts

    def raw(self, path):
        assert path == '/actions/artifacts/55/zip'
        return zipped(self.receipt)

    def registry(self, image):
        assert image == 'quay.io/bundledock/opentofu'
        return mock.Mock(get=lambda kind, ref: self.registry_data[kind, ref])

    def check(self):
        return reconcile.reconcile('opentofu', SHA, '123', '2', self, self.registry)


class EndToEndTests(unittest.TestCase):
    def setUp(self):
        self.api = FakeAPI()

    def test_published_requires_all_layers(self):
        self.assertEqual(self.api.check()['state'], 'published')

    def test_documentation_failure_preserves_proven_publication(self):
        self.api.run['conclusion'] = 'failure'
        self.api.jobs[-1]['conclusion'] = 'failure'
        result = self.api.check()
        self.assertEqual(result['state'], 'published-image-and-tag')
        self.assertEqual(result['documentation'], 'failure')

    def test_tag_failure_preserves_proven_image(self):
        self.api.run['conclusion'] = 'failure'
        self.api.jobs[-2]['conclusion'] = 'failure'
        self.api.jobs[-1]['conclusion'] = 'skipped'
        self.assertEqual(self.api.check()['state'], 'published-image')

    def test_automatic_controller_distinct_from_receipt_source(self):
        self.api.run.update(event='workflow_run', head_sha=CONTROLLER)
        for job in self.api.jobs:
            job['head_sha'] = CONTROLLER
        self.api.artifacts[0]['workflow_run']['head_sha'] = CONTROLLER
        self.assertEqual(self.api.check()['state'], 'published')

    def test_collapsed_skipped_matrix_is_noop(self):
        self.api.run.update(event='workflow_run')
        self.api.jobs = [dict(name='publication / ' + name, status='completed', conclusion='skipped',
                              run_id=123, head_sha=SHA)
                         for name in ('resolve', 'native', 'promote', 'tag', 'documentation')]
        self.api.artifacts = []
        self.assertEqual(self.api.check()['state'], 'not-published')

    def test_skipped_run_with_receipt_is_contradiction(self):
        self.api.run['event'] = 'workflow_run'
        for job in self.api.jobs:
            job['conclusion'] = 'skipped'
        with self.assertRaises(ValueError):
            self.api.check()

    def test_missing_or_running_records_remain_pending(self):
        for mutation in ('run', 'jobs', 'artifact'):
            with self.subTest(mutation=mutation):
                self.api = FakeAPI()
                if mutation == 'run':
                    self.api.run['status'] = 'in_progress'
                    self.assertEqual(self.api.check()['state'], 'pending')
                elif mutation == 'jobs':
                    self.api.jobs = []
                    self.assertEqual(self.api.check()['state'], 'pending')
                else:
                    self.api.artifacts = []
                    with self.assertRaises(reconcile.Pending):
                        self.api.check()

    def test_failed_build_never_claims_publication(self):
        self.api.run['conclusion'] = 'failure'
        self.api.jobs[1]['conclusion'] = 'failure'
        self.assertEqual(self.api.check()['state'], 'incomplete')

    def test_wrong_identity_receipt_registry_and_tag_fail_closed(self):
        mutations = [
            lambda a: a.run.update(head_sha=CONTROLLER),
            lambda a: a.run.update(run_attempt=1),
            lambda a: a.run['repository'].update(full_name='evil/bundledock'),
            lambda a: a.run.update(event='push'),
            lambda a: a.jobs[1].update(head_sha=CONTROLLER),
            lambda a: a.jobs[1]['steps'][0].update(conclusion='skipped'),
            lambda a: a.artifacts[0]['workflow_run'].update(id=999),
            lambda a: a.artifacts.append(copy.deepcopy(a.artifacts[0])),
            lambda a: a.receipt.update(commit=CONTROLLER),
            lambda a: a.receipt.update(run_attempt='1'),
            lambda a: a.receipt.update(run_id='999'),
            lambda a: a.receipt['platforms'][0].update(smoked=False),
            lambda a: a.receipt['platforms'][0].update(digest='sha256:' + 'f' * 64),
            lambda a: a.receipt['platforms'][0].update(image_id='sha256:' + 'f' * 64),
            lambda a: a.receipt['platforms'].pop(),
            lambda a: a.registry_data.update({('manifests', a.receipt['tag']): b'{}'}),
            lambda a: a.ref['object'].update(type='commit'),
            lambda a: a.tag.update(message='incorrect'),
            lambda a: a.tag['object'].update(sha=CONTROLLER),
        ]
        for index, mutate in enumerate(mutations):
            with self.subTest(index=index):
                self.api = FakeAPI()
                mutate(self.api)
                with self.assertRaises(ValueError):
                    self.api.check()

    def test_unknown_conclusion_remains_pending(self):
        self.api.jobs[1]['conclusion'] = 'new-unknown-conclusion'
        self.assertEqual(self.api.check()['state'], 'pending')

    def test_success_cannot_hide_failed_jobs(self):
        self.api.jobs[1]['conclusion'] = 'failure'
        with self.assertRaises(ValueError):
            self.api.check()



class DurableEvidenceTests(unittest.TestCase):
    def setUp(self):
        self.api = FakeAPI()
        self.api.durable = dict(bundle='opentofu', **{key: self.api.receipt[key]
                                for key in ('tag', 'digest', 'commit', 'image', 'run_id')},
                                publication_evidence=dict(receipt=copy.deepcopy(self.api.receipt),
                                    attempt={**{key: self.api.run[key] for key in ('id', 'run_attempt', 'run_number', 'head_sha', 'event', 'path')},
                                             'jobs': copy.deepcopy(self.api.jobs[1:4])}))

    def test_expired_missing_and_unavailable_artifact_use_tracked_receipt(self):
        for mode in ('expired', 'missing', 'listing-unavailable', 'download-unavailable'):
            with self.subTest(mode=mode):
                if mode == 'expired':
                    self.api.artifacts[0]['expired'] = True
                elif mode == 'missing':
                    self.api.artifacts = []
                elif mode == 'listing-unavailable':
                    original = self.api.listing
                    self.api.listing = lambda path, key: original(path, key) if key == 'jobs' else (_ for _ in ()).throw(reconcile.Pending())
                else:
                    self.setUp()
                    self.api.raw = mock.Mock(side_effect=reconcile.Pending())
                self.assertEqual(self.api.check()['state'], 'published')
                self.setUp()

    def test_capture_during_docs_job_and_roundtrip_metadata(self):
        self.api.run.update(status='in_progress', conclusion=None)
        self.api.jobs[-1].update(status='in_progress', conclusion=None)
        evidence = reconcile.capture_evidence('opentofu', SHA, '123', '2', self.api.receipt,
                                              self.api)
        record = reconcile.docs.validate(self.api.receipt, 'opentofu', self.api.receipt['image'], SHA, '123', '2')
        self.api.durable = json.loads(reconcile.docs.metadata('opentofu', record, evidence=evidence))
        self.api.run.update(status='completed', conclusion='success')
        self.api.jobs[-1].update(status='completed', conclusion='success')
        self.api.artifacts[0]['expired'] = True
        self.assertEqual(self.api.check()['state'], 'published')

    def test_capture_requires_successful_native_promote_and_bound_tag(self):
        for index in (1, 2, 3, 4):
            with self.subTest(index=index):
                self.setUp()
                self.api.jobs[index]['steps'][0]['conclusion'] = 'skipped'
                with self.assertRaises(ValueError):
                    reconcile.capture_evidence('opentofu', SHA, '123', '2', self.api.receipt,
                                               self.api)
        self.setUp()
        self.api.tag['message'] = 'wrong'
        with self.assertRaises(ValueError):
            reconcile.capture_evidence('opentofu', SHA, '123', '2', self.api.receipt, self.api)

    def test_durable_evidence_never_replaces_unavailable_live_jobs(self):
        self.api.listing = mock.Mock(side_effect=reconcile.Pending())
        with self.assertRaises(reconcile.Pending): self.api.check()

    def test_historical_evidence_ref_must_be_full_official_main_ancestor(self):
        self.api.artifacts[0]['expired'] = True
        historical = 'e' * 40
        original = self.api.get
        paths = []
        def get(path):
            paths.append(path)
            if path == '/compare/' + historical + '...main':
                return {'merge_base_commit': {'sha': historical}}
            return original(path)
        self.api.get = get
        result = reconcile.reconcile('opentofu', SHA, '123', '2', self.api, self.api.registry,
                                     evidence_ref=historical)
        self.assertEqual(result['state'], 'published')
        self.assertIn('/contents/docs/opentofu-published.json?ref=' + historical, paths)
        for ref in ('main', 'HEAD', 'e' * 39, CONTROLLER):
            with self.subTest(ref=ref), self.assertRaises(ValueError):
                reconcile.reconcile('opentofu', SHA, '123', '2', self.api, self.api.registry, evidence_ref=ref)
        self.api.durable['run_id'] = '999'
        with self.assertRaises(reconcile.Pending):
            reconcile.reconcile('opentofu', SHA, '123', '2', self.api, self.api.registry, evidence_ref=historical)

    def test_tampered_durable_evidence_rejects(self):
        mutations = [
            lambda e: e['receipt'].update(commit=CONTROLLER),
            lambda e: e['receipt']['platforms'][0].update(smoked=False),
            lambda e: e['receipt']['platforms'][0].update(image_id='sha256:' + 'f' * 64),
            lambda e: e['attempt'].update(event='workflow_run'),
            lambda e: e['attempt'].update(head_sha=CONTROLLER),
            lambda e: e['attempt']['jobs'][0].update(id=999),
            lambda e: e['attempt']['jobs'][0]['steps'][0].update(conclusion='skipped'),
        ]
        for mutate in mutations:
            with self.subTest(mutation=mutate):
                self.setUp()
                self.api.artifacts[0]['expired'] = True
                mutate(self.api.durable['publication_evidence'])
                with self.assertRaises(ValueError):
                    self.api.check()

    def test_durable_proof_cannot_override_live_unknown_skipped_or_failed_jobs(self):
        self.api.artifacts[0]['expired'] = True
        self.api.jobs[1]['conclusion'] = 'unknown'
        self.assertEqual(self.api.check()['state'], 'pending')
        self.api.jobs[1]['conclusion'] = 'failure'
        with self.assertRaises(ValueError): self.api.check()
        self.api.run['event'] = 'workflow_run'
        for job in self.api.jobs: job['conclusion'] = 'skipped'
        with self.assertRaises(ValueError): self.api.check()

    def test_missing_artifact_cannot_hide_durable_receipt_on_skipped_attempt(self):
        self.api.artifacts = []
        self.api.run['event'] = 'workflow_run'
        for job in self.api.jobs: job['conclusion'] = 'skipped'
        with self.assertRaises(ValueError): self.api.check()

    def test_malformed_or_oversized_tracked_metadata_rejects(self):
        self.api.artifacts[0]['expired'] = True
        for value in ([], {'publication_evidence': ['invalid'], 'run_id': '123'},
                      {'publication_evidence': {'attempt': []}, 'run_id': '123'}, 'x' * 65536):
            with self.subTest(value=type(value).__name__):
                self.api.durable = value
                with self.assertRaises(ValueError): self.api.check()

    def test_valid_artifact_contradiction_never_falls_back(self):
        self.api.receipt['commit'] = CONTROLLER
        with self.assertRaises(ValueError): self.api.check()
        self.api.raw = mock.Mock(return_value=zipped(None))
        with self.assertRaises(ValueError): self.api.check()

    def test_unknown_artifact_expiry_cannot_use_durable_fallback(self):
        self.api.artifacts[0]['expired'] = None
        with self.assertRaises(reconcile.Pending): self.api.check()

    def test_stale_metadata_is_pending_and_live_registry_tag_are_still_required(self):
        self.api.artifacts[0]['expired'] = True
        self.api.durable['run_id'] = '999'
        with self.assertRaises(reconcile.Pending): self.api.check()
        self.setUp()
        self.api.artifacts[0]['expired'] = True
        self.api.tag['message'] = 'wrong'
        with self.assertRaises(ValueError): self.api.check()

class InputBoundaryTests(unittest.TestCase):
    def test_archive_rejects_other_filenames_and_oversized_receipt(self):
        for raw in (zipped({}, '../publication.json'), zipped('x' * 65536), b'x' * (reconcile.LIMIT + 1)):
            with self.subTest(size=len(raw)), self.assertRaises(ValueError):
                reconcile.receipt_from_archive(raw)

    def test_fixed_host_get_only_and_redacted_diagnostics(self):
        original_popen = reconcile.subprocess.Popen
        def launch(args, **kwargs):
            self.assertEqual(args, ['gh', 'api', '--hostname', 'github.com', '--method', 'GET',
                                    'repos/wstein/bundledock/actions/runs/123'])
            self.assertEqual(kwargs['stderr'], reconcile.subprocess.DEVNULL)
            return original_popen([reconcile.sys.executable, '-c',
                                   "import sys; sys.stderr.write('secret-token'); sys.exit(1)"], **kwargs)
        with mock.patch.object(reconcile.subprocess, 'Popen', side_effect=launch):
            with self.assertRaisesRegex(reconcile.Pending, '^GitHub evidence unavailable$'):
                reconcile.GitHub().get('/actions/runs/123')
        for path in ('https://evil.test/', '/../bad', '//evil.test/#bad'):
            with self.assertRaises(ValueError):
                reconcile.GitHub().get(path)

    def test_comparison_path_preserves_git_three_dot_separator(self):
        with mock.patch.object(reconcile.subprocess, 'Popen', side_effect=OSError) as launch:
            with self.assertRaises(reconcile.Pending):
                reconcile.GitHub().get('/compare/' + SHA + '...main')
            self.assertTrue(launch.called)

    def test_response_at_limit_succeeds(self):
        original_popen = reconcile.subprocess.Popen
        def launch(args, **kwargs):
            return original_popen([reconcile.sys.executable, '-c',
                                   "import sys; sys.stdout.buffer.write(b'x' * 8192)"], **kwargs)
        with mock.patch.object(reconcile, 'LIMIT', 8192), \
                mock.patch.object(reconcile.subprocess, 'Popen', side_effect=launch):
            self.assertEqual(reconcile.GitHub().raw('/actions/runs/123'), b'x' * 8192)

    def test_timeout_stops_and_reaps_child(self):
        original_popen = reconcile.subprocess.Popen
        for close_stdout in (False, True):
            with self.subTest(close_stdout=close_stdout):
                children = []
                program = 'import os,time; ' + ('os.close(1); ' if close_stdout else '') + 'time.sleep(5)'
                def launch(args, **kwargs):
                    child = original_popen([reconcile.sys.executable, '-c', program], **kwargs)
                    children.append(child)
                    return child
                with mock.patch.object(reconcile, 'TIMEOUT', 0.1), \
                        mock.patch.object(reconcile.subprocess, 'Popen', side_effect=launch):
                    with self.assertRaisesRegex(reconcile.Pending, '^GitHub evidence unavailable$'):
                        reconcile.GitHub().raw('/actions/runs/123')
                self.assertIsNotNone(children[0].poll())
                self.assertNotEqual(children[0].returncode, 0)

    def test_oversized_response_stops_child_before_completion(self):
        original_popen = reconcile.subprocess.Popen
        children = []
        with tempfile.TemporaryDirectory() as temporary:
            marker = Path(temporary) / 'finished'
            program = ("import sys; from pathlib import Path; "
                       "sys.stdout.buffer.write(b'x' * (16 * 1024 * 1024)); "
                       "sys.stdout.buffer.flush(); Path(sys.argv[1]).touch()")
            def launch(args, **kwargs):
                child = original_popen([reconcile.sys.executable, '-c', program, str(marker)], **kwargs)
                children.append(child)
                return child
            with mock.patch.object(reconcile.subprocess, 'Popen', side_effect=launch):
                with self.assertRaisesRegex(ValueError, 'Oversized GitHub response'):
                    reconcile.GitHub().raw('/actions/runs/123')
            self.assertFalse(marker.exists(), 'Oversized response was allowed to finish writing')
            self.assertEqual(len(children), 1)
            self.assertIsNotNone(children[0].poll(), 'Oversized child must be reaped')
            self.assertNotEqual(children[0].returncode, 0, 'Oversized child must be stopped early')

    def test_listing_is_bounded_and_rejects_truncation(self):
        api = reconcile.GitHub()
        with mock.patch.object(api, 'get', return_value={'total_count': 101, 'jobs': []}):
            with self.assertRaises(ValueError):
                api.listing('/actions/runs/123/jobs', 'jobs')
        with mock.patch.object(api, 'get', return_value={'total_count': 1001, 'jobs': [{}] * 100}):
            with self.assertRaises(ValueError):
                api.listing('/actions/runs/123/jobs', 'jobs')
        with mock.patch.object(api, 'get', return_value={'total_count': 1000, 'jobs': [{}] * 100}) as get:
            self.assertEqual(len(api.listing('/actions/runs/123/jobs', 'jobs')), 1000)
            self.assertEqual(get.call_count, 10)


class MissingEvidenceTests(unittest.TestCase):
    def test_missing_smoke_steps_or_expired_receipt_are_pending(self):
        for missing in ('steps', 'expired', 'wrong-name'):
            with self.subTest(missing=missing):
                api = FakeAPI()
                if missing == 'steps':
                    api.jobs[1]['steps'] = []
                elif missing == 'expired':
                    api.artifacts[0]['expired'] = True
                else:
                    api.artifacts[0]['name'] = 'opentofu-publication-1'
                with self.assertRaises(reconcile.Pending):
                    api.check()

    def test_missing_tag_does_not_turn_into_no_publication(self):
        api = FakeAPI()
        original = api.get
        def get(path):
            if path.startswith('/git/ref/tags/'):
                raise reconcile.Pending('Missing tag')
            return original(path)
        api.get = get
        with self.assertRaises(reconcile.Pending):
            api.check()

    def test_skipped_smoke_or_docs_step_rejects_successful_job(self):
        for index in (1, 2, 3, 4, 5):
            with self.subTest(index=index):
                api = FakeAPI()
                api.jobs[index]['steps'][0]['conclusion'] = 'skipped'
                with self.assertRaises(ValueError):
                    api.check()

    def test_both_configs_are_digest_checked(self):
        for arch in ('amd64', 'arm64'):
            with self.subTest(arch=arch):
                api = FakeAPI()
                platform = next(p for p in api.receipt['platforms'] if p['architecture'] == arch)
                api.registry_data['blobs', platform['image_id']] = b'{}'
                with self.assertRaises(ValueError):
                    api.check()

    def test_cli_never_prints_exception_secrets(self):
        argv = ['publication-reconcile.py', '--bundle', 'opentofu', '--sha', SHA,
                '--run-id', '123', '--attempt', '2']
        for error, status in ((ValueError('secret-token'), 1), (reconcile.Pending('secret-token'), 2)):
            with self.subTest(error=type(error).__name__), mock.patch.object(reconcile.sys, 'argv', argv), \
                    mock.patch.object(reconcile, 'reconcile', side_effect=error), \
                    mock.patch('sys.stdout', new_callable=io.StringIO) as output:
                self.assertEqual(reconcile.main(), status)
                self.assertNotIn('secret-token', output.getvalue())


if __name__ == '__main__':
    unittest.main()

class ConsolidatedIdentityTests(unittest.TestCase):
    def test_current_and_historical_callers_reconcile_read_only(self):
        for path in ('publish-bundles.yml', 'bundledock-opentofu.yml'):
            with self.subTest(path=path):
                api = FakeAPI()
                api.run['path'] = '.github/workflows/' + path
                self.assertEqual(api.check()['state'], 'published')

    def test_other_bundle_historical_caller_rejected(self):
        api = FakeAPI()
        api.run['path'] = '.github/workflows/bundledock-node.yml'
        with self.assertRaises(ValueError):
            api.check()

    def test_new_evidence_capture_rejects_historical_caller(self):
        api = FakeAPI()
        api.run['path'] = '.github/workflows/bundledock-opentofu.yml'
        with self.assertRaises(ValueError):
            reconcile.capture_evidence('opentofu', SHA, '123', '2', api.receipt, api)
