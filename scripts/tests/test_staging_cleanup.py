import copy
from datetime import datetime, timezone, timedelta
import hashlib
import importlib.util
import json
from pathlib import Path
import unittest
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
        outer = self
        class Registry:
            def get(self, kind, ref): return outer.documents[kind, ref]
        class GitHub:
            def run(self, run_id, fresh=False): return copy.deepcopy(outer.run)
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
