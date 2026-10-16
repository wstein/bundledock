import copy
from datetime import datetime, timezone, timedelta
import hashlib
import importlib.util
import json
from pathlib import Path
import unittest
import tempfile
import os
from unittest.mock import patch

PATH = Path(__file__).resolve().parents[1] / 'staging-cleanup.py'
SPEC = importlib.util.spec_from_file_location('cleanup', PATH)
cleanup = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(cleanup)
NOW = datetime(2026, 10, 14, tzinfo=timezone.utc)


class CleanupTests(unittest.TestCase):
    def setUp(self):
        self.documents = {}
        def add(kind, value):
            raw = json.dumps(value).encode()
            digest = 'sha256:' + hashlib.sha256(raw).hexdigest()
            self.documents[kind, digest] = raw
            return digest
        self.labels = {
            'io.bundledock.bundle': 'toolbox',
            'io.bundledock.github.run-id': '123',
            'io.bundledock.github.run-attempt': '1',
            'org.opencontainers.image.source': cleanup.tags.SOURCE,
            'org.opencontainers.image.revision': 'a' * 40,
            'org.opencontainers.image.version': '2026.10.01-b4.1',
        }
        config = add('blobs', {'os': 'linux', 'architecture': 'amd64', 'config': {'Labels': self.labels}})
        digest = add('manifests', {'config': {'digest': config}})
        self.row = dict(name='staging-2026.10.01-b4.1-amd64', manifest_digest=digest,
                        last_modified='Thu, 01 Oct 2026 00:00:00 -0000', is_manifest_list=False)
        self.run = dict(id=123, run_number=4, run_attempt=1, status='completed', conclusion='failure',
                        updated_at='2026-10-01T00:00:00Z', event='workflow_dispatch',
                        head_branch='main', head_sha='a' * 40,
                        repository={'full_name': 'wstein/bundledock'},
                        path='.github/workflows/publish-bundles.yml')
        self.run['head_repository'] = {'full_name': 'wstein/bundledock'}
        self.run['workflow_id'] = 456
        self.attempt_record = copy.deepcopy(self.run)
        self.workflow = dict(id=456, path='.github/workflows/publish-bundles.yml', state='active')
        self.jobs = [dict(id=789, run_id=123, run_attempt=1, status='completed', conclusion='failure')]
        outer = self
        class Registry:
            def get(self, kind, ref): return outer.documents[kind, ref]
        class GitHub:
            def run(self, run_id, fresh=False): return copy.deepcopy(outer.run)
            def attempt(self, run_id, attempt, fresh=False): return copy.deepcopy(outer.attempt_record)
            def workflow(self, fresh=False): return copy.deepcopy(outer.workflow)
            def jobs(self, run_id, attempt, fresh=False): return copy.deepcopy(outer.jobs)
        class Quay:
            def __init__(self): self.deleted = []; self.rows = [outer.row]
            def list_tags(self, repository, name=None):
                return copy.deepcopy([r for r in self.rows if name is None or r['name'] == name])
            def delete(self, repository, name): self.deleted.append((repository, name))
        self.registry, self.github, self.quay = Registry(), GitHub(), Quay()

    def assess(self):
        return cleanup.assess('toolbox', self.row, self.registry, self.github, NOW)

    def test_completed_old_staging_is_eligible_but_dry_run_never_deletes(self):
        row = self.assess()
        self.assertTrue(row['eligible'], row)
        plan = {'schema': 1, 'mode': 'fallback', 'rows': [row]}
        cleanup.apply_plan(plan, lambda repo, row: self.assess(), self.quay, write=False)
        self.assertEqual(self.quay.deleted, [])

    def test_daily_revision_uses_attempt_labels_not_workflow_number(self):
        self.row['name'] = 'staging-2026.10.01-b1-amd64'
        self.labels['org.opencontainers.image.version'] = '2026.10.01-b1'
        raw = json.dumps({'os': 'linux', 'architecture': 'amd64', 'config': {'Labels': self.labels}}).encode()
        digest = 'sha256:' + hashlib.sha256(raw).hexdigest()
        self.documents['blobs', digest] = raw
        raw = json.dumps({'config': {'digest': digest}}).encode()
        self.row['manifest_digest'] = 'sha256:' + hashlib.sha256(raw).hexdigest()
        self.documents['manifests', self.row['manifest_digest']] = raw
        self.run['run_number'] = 98
        self.assertTrue(self.assess()['eligible'])
        # The registry bytes remain bound to attempt 1; a different live attempt
        # is retained until the age and exact provenance checks permit cleanup.
        self.run['run_attempt'] = 2
        self.run['updated_at'] = '2026-10-14T00:00:00Z'
        self.assertFalse(self.assess()['eligible'])

    def test_seven_day_boundary_and_active_or_recent_reruns(self):
        for field, value in [('status', 'in_progress'), ('status', 'queued'),
                             ('updated_at', '2026-10-13T00:00:00Z'), ('run_attempt', 0),
                             ('run_number', 5), ('path', '.github/workflows/ci.yml')]:
            with self.subTest(field=field):
                old = self.run[field]; self.run[field] = value
                self.assertFalse(self.assess()['eligible'])
                self.run[field] = old
        self.row['last_modified'] = 'Wed, 07 Oct 2026 00:00:00 -0000'
        self.assertTrue(self.assess()['eligible'])
        self.row['last_modified'] = 'Wed, 07 Oct 2026 00:00:01 -0000'
        self.assertFalse(self.assess()['eligible'])

    def test_protected_names_wrong_arch_index_and_immutable_are_retained(self):
        for changes in [dict(name='stable'), dict(name='latest'), dict(name='2026.10.01-b4.1'),
                        dict(name='staging-2026.10.01-b4.1-arm64'),
                        dict(is_manifest_list=True), dict(immutable=True),
                        dict(manifest_digest='sha256:' + 'f' * 64)]:
            with self.subTest(changes=changes):
                original = self.row; self.row = dict(original, **changes)
                self.assertFalse(self.assess()['eligible'])
                self.row = original

    def test_missing_api_evidence_fails_closed(self):
        self.run = {}
        self.assertFalse(self.assess()['eligible'])

    def test_apply_rechecks_tag_digest_and_active_status(self):
        approved = {'schema': 1, 'mode': 'fallback', 'rows': [self.assess()]}
        self.run['status'] = 'in_progress'
        with self.assertRaises(ValueError):
            cleanup.apply_plan(approved, lambda repo, row: self.assess(), self.quay, write=True)
        self.assertEqual(self.quay.deleted, [])
        self.run['status'] = 'completed'
        self.quay.rows = [dict(self.row, manifest_digest='sha256:' + 'f' * 64)]
        with self.assertRaises(ValueError):
            cleanup.apply_plan(approved, lambda repo, row: self.assess(), self.quay, write=True)
        self.assertEqual(self.quay.deleted, [])

    def test_apply_deletes_only_exact_tag_and_is_idempotent_when_absent(self):
        approved = {'schema': 1, 'mode': 'fallback', 'rows': [self.assess()]}
        reports = []
        cleanup.apply_plan(approved, lambda repo, row: self.assess(), self.quay, write=True,
                           report=lambda actions: reports.append(copy.deepcopy(actions)))
        self.assertEqual(reports[0][0]['action'], 'deleted')
        self.assertEqual(self.quay.deleted, [('toolbox', self.row['name'])])
        self.quay.rows = []
        cleanup.apply_plan(approved, lambda repo, row: self.assess(), self.quay, write=True)
        self.assertEqual(len(self.quay.deleted), 1)

    def test_post_publication_requires_exact_receipt_platform_digest(self):
        receipt = dict(bundle='toolbox', tag='2026.10.01-b4.1', commit='a' * 40,
                       run_id='123', attempt=1, platforms=[dict(architecture='amd64', digest=self.row['manifest_digest'])])
        self.run['status'] = 'in_progress'
        row = cleanup.assess('toolbox', self.row, self.registry, self.github, NOW, receipt)
        self.assertTrue(row['eligible'], row)
        receipt['platforms'][0]['digest'] = 'sha256:' + 'f' * 64
        self.assertFalse(cleanup.assess('toolbox', self.row, self.registry, self.github, NOW, receipt)['eligible'])

    def test_all_approved_names_validated_before_any_deletion(self):
        good = self.assess()
        for bad in (dict(good, name='stable'), dict(good, repository='foreign'),
                    dict(good, manifest_digest='invalid')):
            with self.subTest(bad=bad), self.assertRaises(ValueError):
                cleanup.apply_plan({'schema': 1, 'mode': 'fallback', 'rows': [good, bad]},
                                   lambda repo, row: self.assess(), self.quay, write=True)
            self.assertEqual(self.quay.deleted, [])

    def failed_assess(self):
        return cleanup.assess('toolbox', self.row, self.registry, self.github, NOW,
                              failed_run=('123', 1), fresh=True)

    def test_failed_run_recent_tags_are_eligible_only_for_exact_completed_attempt(self):
        self.row['last_modified'] = 'Wed, 14 Oct 2026 00:00:00 -0000'
        self.assertFalse(self.assess()['eligible'])
        for conclusion in ('failure', 'cancelled', 'timed_out'):
            with self.subTest(conclusion=conclusion):
                self.run['conclusion'] = self.attempt_record['conclusion'] = conclusion
                self.assertTrue(self.failed_assess()['eligible'])
        approved = dict(schema=1, mode='failed-run', target=dict(run_id='123', attempt=1),
                        rows=[self.failed_assess()])
        cleanup.apply_plan(approved, lambda repo, row: self.failed_assess(), self.quay)
        self.assertEqual(self.quay.deleted, [])
        cleanup.apply_plan(approved, lambda repo, row: self.failed_assess(), self.quay, write=True)
        self.assertEqual(self.quay.deleted, [('toolbox', self.row['name'])])

    def test_failed_run_unavailable_active_successful_foreign_or_rerun_is_retained(self):
        for field, value in [('id', 124), ('run_attempt', 2), ('status', 'in_progress'),
                             ('conclusion', 'success'), ('conclusion', None),
                             ('event', 'pull_request'), ('head_branch', 'topic'),
                             ('path', '.github/workflows/ci.yml'), ('workflow_id', 999),
                             ('repository', {'full_name': 'foreign/repo'}),
                             ('head_repository', {'full_name': 'foreign/repo'})]:
            with self.subTest(field=field):
                original = copy.deepcopy(self.run)
                self.run[field] = value
                self.assertFalse(self.failed_assess()['eligible'])
                self.run = original
        for field, value in [('run_attempt', 2), ('status', 'queued'), ('head_sha', 'b' * 40)]:
            original = copy.deepcopy(self.attempt_record)
            self.attempt_record[field] = value
            self.assertFalse(self.failed_assess()['eligible'])
            self.attempt_record = original
        self.workflow['id'] = 999
        self.assertFalse(self.failed_assess()['eligible'])

    def test_failed_run_all_jobs_must_be_completed_exact_unique_and_nonempty(self):
        original = copy.deepcopy(self.jobs)
        for jobs in ([], [dict(original[0], status='in_progress')],
                     [dict(original[0], conclusion=None)],
                     [dict(original[0], run_attempt=2)], [dict(original[0], run_id=124)],
                     [original[0], original[0]]):
            with self.subTest(jobs=jobs):
                self.jobs = jobs
                self.assertFalse(self.failed_assess()['eligible'])
        self.jobs = original

    def test_failed_run_plan_scope_and_rerun_rechecked_before_deletion(self):
        approved = dict(schema=1, mode='failed-run', target=dict(run_id='123', attempt=1),
                        rows=[self.failed_assess()])
        self.run['run_attempt'] = 2
        with self.assertRaises(ValueError):
            cleanup.apply_plan(approved, lambda repo, row: self.failed_assess(), self.quay, write=True)
        self.assertEqual(self.quay.deleted, [])
        self.run['run_attempt'] = 1
        for bad in (dict(approved, target=dict(run_id='124', attempt=1)),
                    dict(approved, target=dict(run_id='123', attempt=True)),
                    dict(approved, rows=[approved['rows'][0], dict(approved['rows'][0], name='staging-2026.10.01-b5.1-amd64', run_id='124')])):
            with self.subTest(bad=bad), self.assertRaises(ValueError):
                cleanup.apply_plan(bad, lambda repo, row: self.failed_assess(), self.quay, write=True)
            self.assertEqual(self.quay.deleted, [])

    def test_failed_run_cli_saves_targeted_dry_run_and_rejects_other_plan_scope(self):
        with tempfile.TemporaryDirectory() as directory:
            output = Path(directory) / 'plan.json'
            summary = Path(directory) / 'summary.md'
            args = ['cleanup', '--repository', 'toolbox', '--failed-run', '123',
                    '--failed-attempt', '1', '--output', str(output)]
            with patch.object(cleanup.sys, 'argv', args), patch.object(cleanup, 'Quay', return_value=self.quay), \
                 patch.object(cleanup, 'GitHub', return_value=self.github), \
                 patch.object(cleanup.tags, 'Registry', return_value=self.registry), \
                 patch.dict(os.environ, {'GITHUB_STEP_SUMMARY': str(summary)}):
                cleanup.main()
            plan = json.loads(output.read_text())
            self.assertEqual(plan['mode'], 'failed-run')
            self.assertEqual(plan['target'], {'run_id': '123', 'attempt': 1})
            self.assertTrue(plan['rows'][0]['eligible'])
            self.assertEqual(self.quay.deleted, [])
            self.assertIn('failed-run', summary.read_text())
            plan['target']['run_id'] = '124'
            output.write_text(json.dumps(plan))
            with patch.object(cleanup.sys, 'argv', args + ['--apply', '--approved-plan', str(output)]), \
                 patch.object(cleanup, 'Quay', return_value=self.quay), \
                 patch.object(cleanup, 'GitHub', return_value=self.github), \
                 patch.object(cleanup.tags, 'Registry', return_value=self.registry), \
                 patch.dict(os.environ, {'REPOSITORY_USERNAME': 'fixture', 'REPOSITORY_TOKEN': 'fixture'}), \
                 self.assertRaisesRegex(ValueError, 'Plan target'):
                cleanup.main()
            self.assertEqual(self.quay.deleted, [])

    def test_failed_run_proof_rechecks_latest_attempt_after_jobs(self):
        rerun = dict(self.run, run_attempt=2, status='queued', conclusion=None)
        with patch.object(self.github, 'run', side_effect=[self.run, rerun]), self.assertRaisesRegex(ValueError, 'changed while'):
            cleanup.failed_run_proof(self.github, '123', 1, fresh=True)
        self.assertEqual(self.quay.deleted, [])

    def test_failed_run_cli_rejects_incomplete_and_conflicting_selectors(self):
        for selectors in (['--failed-run', '123'], ['--failed-attempt', '1'],
                          ['--failed-run', '123', '--failed-attempt', '0'],
                          ['--failed-run', '../123', '--failed-attempt', '1'],
                          ['--failed-run', '123', '--failed-attempt', '1', '--publication', 'receipt', '--bundle', 'toolbox']):
            with self.subTest(selectors=selectors), patch.object(cleanup.sys, 'argv', ['cleanup', '--output', 'unused'] + selectors), \
                 patch.object(cleanup, 'Quay') as quay, self.assertRaises(ValueError):
                cleanup.main()
            quay.assert_not_called()

    def test_failed_jobs_pagination_requires_every_exact_unique_job(self):
        github = cleanup.GitHub()
        rows = [dict(id=1), dict(id=2)]
        with patch.object(github, '_get', side_effect=[dict(total_count=2, jobs=rows[:1]),
                                                       dict(total_count=2, jobs=rows[1:])]):
            self.assertEqual(github.jobs('123', 1), rows)
        for responses in ([dict(total_count=2, jobs=[])],
                          [dict(total_count=2, jobs=rows[:1]), dict(total_count=2, jobs=rows[:1])],
                          [dict(total_count=2, jobs=rows[:1]), dict(total_count=3, jobs=rows[1:])],
                          [dict(total_count=2001, jobs=[])]):
            with self.subTest(responses=responses), patch.object(github, '_get', side_effect=responses), self.assertRaises(ValueError):
                github.jobs('123', 1)

    def test_no_registry_credentials_cannot_delete(self):
        with self.assertRaisesRegex(ValueError, 'TOKEN required'):
            cleanup.Quay().delete('toolbox', self.row['name'])

    def test_transport_pagination_and_duplicate_detection(self):
        api = cleanup.Quay()
        with patch.object(api, 'request', side_effect=[
            dict(page=1, tags=[self.row], has_additional=True),
            dict(page=2, tags=[dict(self.row, name='another')], has_additional=False),
        ]) as request:
            self.assertEqual(len(api.list_tags('toolbox')), 2)
            self.assertIn('page=2', request.call_args.args[0])
        for data in (dict(page=2, tags=[], has_additional=False),
                     dict(page=1, tags=[], has_additional=True),
                     dict(page=1, tags=[self.row, self.row], has_additional=False)):
            with patch.object(api, 'request', return_value=data), self.assertRaises(ValueError):
                api.list_tags('toolbox')

    def test_publication_proof_requires_index_platforms_and_source_tag(self):
        receipt = dict(bundle='toolbox', image='quay.io/bundledock/toolbox',
                       tag='2026.10.01-b4.1', commit='a' * 40, source=cleanup.tags.SOURCE,
                       run_id='123', digest=self.row['manifest_digest'], platforms=[
                           dict(architecture='amd64', digest=self.row['manifest_digest']),
                           dict(architecture='arm64', digest='sha256:' + 'f' * 64)])
        index = dict(manifests=[dict(platform=dict(architecture=p['architecture']), digest=p['digest']) for p in receipt['platforms']])
        env = dict(GITHUB_RUN_ID='123', GITHUB_RUN_ATTEMPT='1', PUBLICATION_SHA='a' * 40)
        with patch.dict(cleanup.os.environ, env), patch.object(cleanup.Path, 'read_text', return_value=json.dumps(receipt)), \
             patch.object(cleanup.docs, 'validate') as validate, \
             patch.object(cleanup.tags, 'Registry'), patch.object(cleanup.tags, 'verify_source'), \
             patch.object(cleanup.tags, 'verify_registry') as verify, \
             patch.object(cleanup.tags, 'decode_digest', return_value=index), \
             patch.object(cleanup.tags, 'remote_tag', return_value=True) as remote:
            result = cleanup.publication_proof('receipt.json', 'toolbox', self.github)
            self.assertEqual(result['attempt'], 1)
            validate.assert_called_once()
            self.assertTrue(verify.call_args.kwargs['aliases'])
            remote.return_value = False
            with self.assertRaisesRegex(ValueError, 'source tag missing'):
                cleanup.publication_proof('receipt.json', 'toolbox', self.github)
            remote.return_value = True
            index['manifests'][0]['digest'] = 'sha256:' + '0' * 64
            with self.assertRaisesRegex(ValueError, 'platform digests differ'):
                cleanup.publication_proof('receipt.json', 'toolbox', self.github)

    def test_quay_transport_only_allows_fixed_tag_paths(self):
        api = cleanup.Quay('fixture', 'password')
        for repository, name in [('foreign/toolbox', self.row['name']), ('toolbox', 'stable'),
                                 ('toolbox', '../manifest'), ('toolbox', 'staging-x-amd64')]:
            with self.subTest(repository=repository, name=name), self.assertRaises(ValueError):
                api.delete(repository, name)


if __name__ == '__main__': unittest.main()
