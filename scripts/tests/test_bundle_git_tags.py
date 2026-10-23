import importlib.util
import pathlib
import unittest
import shutil

PATH = pathlib.Path(__file__).resolve().parents[1] / 'bundle-git-tags.py'
spec = importlib.util.spec_from_file_location('bundle_tags', PATH)
tags = importlib.util.module_from_spec(spec)
spec.loader.exec_module(tags)

class ProvenanceTests(unittest.TestCase):
    def record(self, **changes):
        result = dict(bundle='node', image='quay.io/bundledock/node', tag='2026.10.12-b1.1',
                      digest='sha256:' + 'a'*64, commit='b'*40,
                      source=tags.SOURCE, run_id='123')
        result.update(changes)
        return result

    def test_canonical_annotation(self):
        record = tags.validate_record(self.record())
        self.assertEqual(tags.tag_name(record), 'node/2026.10.12-b1.1')
        self.assertIn('quay.io/bundledock/node:2026.10.12-b1.1@sha256:', tags.annotation(record))
        self.assertIn(tags.SOURCE + '/actions/runs/123/attempts/1', tags.annotation(record))

    def test_reject_wrong_repository_source_digest_and_version(self):
        for change in [dict(image='quay.io/evil/node'), dict(source='https://github.com/evil/repo'),
                       dict(digest='sha256:bad'), dict(commit='deadbeef'), dict(tag='stable'),
                       dict(tag='2026.10.12-b0.1'), dict(run_id='../123')]:
            with self.subTest(change=change), self.assertRaises(ValueError):
                tags.validate_record(self.record(**change))

    def test_daily_toolbox_attempt_is_independent_of_revision(self):
        record = self.record(bundle='toolbox', image='quay.io/bundledock/toolbox', tag='2026.10.15-b1', run_attempt='7')
        result = tags.validate_record(record)
        self.assertEqual(result['attempt'], 7)
        self.assertIsNone(result['run_number'])
        for change in [dict(run_attempt=None), dict(run_attempt='0'), dict(tag='2026.02.30-b1')]:
            with self.subTest(change=change), self.assertRaises(ValueError):
                tags.validate_record(dict(record, **change))

    def test_legacy_is_explicit(self):
        record = self.record(bundle='toolbox', image='quay.io/bundledock/toolbox',
                             tag='2026.10.10-123-4')
        with self.assertRaises(ValueError):
            tags.validate_record(record)
        self.assertEqual(tags.validate_record(record, legacy=True)['attempt'], 4)

    def test_hashes_fetched_bytes(self):
        with self.assertRaises(ValueError):
            tags.decode_digest(b'{}', 'sha256:'+'a'*64)

    def test_labels_must_match_each_architecture(self):
        record = tags.validate_record(self.record())
        labels = {'org.opencontainers.image.'+key: record[value]
                  for key, value in [('source','source'), ('revision','commit'), ('version','tag')]}
        labels['io.bundledock.github.run-id'] = '123'
        tags.verify_labels(record, labels)
        for key in labels:
            invalid = dict(labels, **{key:'wrong'})
            with self.subTest(key=key), self.assertRaises(ValueError):
                tags.verify_labels(record, invalid)


class RegistryTests(ProvenanceTests):
    def fixture(self, record):
        import hashlib
        documents = {}
        def add(value, kind):
            raw = tags.json.dumps(value).encode()
            digest = 'sha256:'+hashlib.sha256(raw).hexdigest()
            documents[(kind,digest)] = raw
            return digest
        manifests = []
        for arch in ('amd64','arm64'):
            labels = {'org.opencontainers.image.'+key: record[field]
                      for key,field in [('source','source'),('revision','commit'),('version','tag')]}
            labels['io.bundledock.github.run-id'] = record['run_id']
            config = add({'os':'linux','architecture':arch,'config':{'Labels':labels}}, 'blobs')
            manifest = add({'config':{'digest':config}}, 'manifests')
            manifests.append({'platform':{'os':'linux','architecture':arch}, 'digest':manifest})
        record['digest'] = add({'manifests':manifests}, 'manifests')
        for alias in (record['tag'],*tags._CONFIG.select_bundle(record['bundle'])['aliases'].split()):
            documents[('manifests',alias)] = documents[('manifests',record['digest'])]
        class Registry:
            def get(self, kind, ref):
                return documents[(kind,ref)]
        return Registry(), documents

    def test_verified_two_architecture_index_and_aliases(self):
        record = tags.validate_record(self.record())
        registry, _ = self.fixture(record)
        tags.verify_registry(record, registry, aliases=True)

    def test_tampered_config_and_wrong_alias_fail(self):
        record = tags.validate_record(self.record())
        registry, documents = self.fixture(record)
        documents[('manifests','stable')] = b'{}'
        with self.assertRaises(ValueError):
            tags.verify_registry(record, registry, aliases=True)
        config = next(key for key in documents if key[0]=='blobs')
        documents[config] += b' '
        with self.assertRaises(ValueError):
            tags.verify_registry(record, registry)

@unittest.skipUnless(shutil.which("git"), "host git is unavailable")
class GitTests(ProvenanceTests):
    def setUp(self):
        import os
        import tempfile
        from unittest.mock import patch
        import subprocess
        self.temporary = tempfile.TemporaryDirectory()
        # `git commit` spawns a detached `git maintenance run --auto` that can write into
        # `objects` during cleanup (ENOTEMPTY); the fixture repositories never need it.
        patcher = patch.dict(os.environ, {'GIT_CONFIG_COUNT': '2',
                                          'GIT_CONFIG_KEY_0': 'maintenance.auto', 'GIT_CONFIG_VALUE_0': 'false',
                                          'GIT_CONFIG_KEY_1': 'gc.auto', 'GIT_CONFIG_VALUE_1': '0'})
        patcher.start()
        self.addCleanup(patcher.stop)
        self.previous = os.getcwd()
        os.chdir(self.temporary.name)
        subprocess.run(['git','init','--bare','remote.git'],check=True,capture_output=True)
        for key,value in (('maintenance.auto','false'),('gc.auto','0'),('receive.autoGC','false')):
            subprocess.run(['git','-C','remote.git','config',key,value],check=True,capture_output=True)
        subprocess.run(['git','init','work'],check=True,capture_output=True)
        os.chdir('work')
        for args in [('config','user.name','Fixture'),('config','user.email','fixture@example.invalid'),
                     # Throwaway repos: `git commit` and `git-receive-pack` (local transport drops
                     # GIT_CONFIG_*) start detached auto maintenance that races temp cleanup (ENOTEMPTY).
                     ('config','maintenance.auto','false'),('config','gc.auto','0'),
                     ('remote','add','origin','../remote.git'),('commit','--allow-empty','-m','fixture')]:
            subprocess.run(['git',*args],check=True,capture_output=True)
        self.record_data = tags.validate_record(self.record(commit=tags.git('rev-parse','HEAD').stdout.strip()))

    def tearDown(self):
        import os
        os.chdir(self.previous)
        self.temporary.cleanup()

    def test_semantic_tag_preserves_legacy_and_rejects_conflict(self):
        old = self.record_data
        tags.create_tag(old)
        r = tags.validate_record(dict(old, tag='24.21.0-b1', run_attempt='3'))
        tags.create_tag(r)
        self.assertEqual(tags.create_tag(r), 'already present')
        with self.assertRaises(ValueError):
            tags.create_tag(dict(r, digest='sha256:'+'c'*64))
        self.assertTrue(tags.remote_tag(old))
        self.assertTrue(tags.remote_tag(r))

    def test_create_and_idempotency_without_local_tags(self):
        r = self.record_data
        self.assertFalse(tags.remote_tag(r))
        tags.create_tag(r)
        self.assertTrue(tags.remote_tag(r))
        self.assertEqual(tags.create_tag(r), 'already present')
        self.assertEqual(tags.git('tag','--list').stdout, '')
        self.assertEqual(tags.git('for-each-ref','refs/bundledock-verify').stdout,'')

    def test_conflicting_annotation_never_overwritten(self):
        r = self.record_data
        tags.create_tag(r)
        conflict = dict(r, digest='sha256:'+'c'*64)
        before = tags.git('ls-remote','--tags','origin').stdout
        with self.assertRaises(ValueError):
            tags.create_tag(conflict)
        self.assertEqual(tags.git('ls-remote','--tags','origin').stdout,before)

    def test_lightweight_tag_rejected(self):
        r = self.record_data
        tags.git('push','origin',r['commit']+':refs/tags/'+tags.tag_name(r))
        with self.assertRaises(ValueError):
            tags.remote_tag(r)

    def test_concurrent_matching_creation(self):
        from unittest.mock import patch
        r = self.record_data
        original = tags.remote_tag
        calls = 0
        def race(record):
            nonlocal calls
            calls += 1
            if calls == 1:
                tags.create_tag(record)
                return False
            return original(record)
        with patch.object(tags, 'remote_tag', side_effect=race):
            tags.create_tag(r)
        self.assertTrue(tags.remote_tag(r))

    def test_wrong_origin_rejected_before_fetch(self):
        with self.assertRaises(ValueError):
            tags.verify_source(self.record_data)

    def test_push_does_not_follow_unrelated_annotated_tags(self):
        tags.git('tag', '-a', 'unrelated-alias', '-m', 'unrelated')
        tags.git('config', 'push.followTags', 'true')
        tags.create_tag(self.record_data)
        self.assertNotIn('unrelated-alias', tags.git('ls-remote','--tags','origin').stdout)

    def test_multiple_origin_push_urls_rejected_before_fetch(self):
        from unittest.mock import patch
        import subprocess
        def fake(*args, **kwargs):
            if args[:2] == ('remote', 'get-url'):
                value = tags.SOURCE+'\n'
                if '--push' in args and '--all' in args:
                    value += 'https://github.com/evil/repo\n'
                return subprocess.CompletedProcess(args, 0, value, '')
            if args[0] == 'fetch':
                self.fail('untrusted push configuration was not rejected before fetching')
            return subprocess.CompletedProcess(args, 0, '', '')
        with patch.object(tags, 'git', side_effect=fake), self.assertRaises(ValueError):
            tags.verify_source(self.record_data)

    def test_concurrent_conflicting_creation_is_preserved(self):
        from unittest.mock import patch
        r = self.record_data
        original = tags.remote_tag
        calls = 0
        def race(record):
            nonlocal calls
            calls += 1
            if calls == 1:
                tags.create_tag(dict(record, digest='sha256:'+'c'*64))
                return False
            return original(record)
        with patch.object(tags, 'remote_tag', side_effect=race), self.assertRaises(ValueError):
            tags.create_tag(r)
        self.assertTrue(tags.remote_tag(dict(r,digest='sha256:'+'c'*64)))

class RunTests(ProvenanceTests):
    def run_data(self, r):
        return dict(head_sha=r['commit'], head_branch='main', repository={'html_url':tags.SOURCE},
                    path='.github/workflows/publish-bundles.yml', run_attempt=1, id=123,
                    run_number=1, status='completed', conclusion='success', event='workflow_dispatch')

    def test_run_provenance_class_and_backfill_success(self):
        from unittest.mock import patch
        r = tags.validate_record(self.record())
        run = self.run_data(r)
        for field, invalid in [('head_sha','a'*40),('head_branch','topic'),('repository',{'html_url':'https://github.com/evil/repo'}),
                               ('path','.github/workflows/evil.yml'),('run_attempt',2),('id',999),('run_number',2),
                               ('status','in_progress'),('conclusion','failure')]:
            with self.subTest(field=field), patch.object(tags,'request',return_value=tags.json.dumps(dict(run,**{field:invalid})).encode()), self.assertRaises(ValueError):
                tags.verify_run(r, True)
        with patch.object(tags,'request',return_value=tags.json.dumps(run).encode()):
            tags.verify_run(r,True)

    def test_semantic_attempt_does_not_infer_run_number(self):
        from unittest.mock import patch
        r = tags.validate_record(self.record(tag='24.21.0-b2',run_attempt='3'))
        run = dict(self.run_data(r),run_number=999,run_attempt=3)
        with patch.object(tags,'request',return_value=tags.json.dumps(run).encode()):
            tags.verify_run(r,True)
        run['run_attempt']=2
        with patch.object(tags,'request',return_value=tags.json.dumps(run).encode()), self.assertRaises(ValueError):
            tags.verify_run(r,True)

    def test_current_workflow_context(self):
        from unittest.mock import patch
        r = tags.validate_record(self.record())
        run = self.run_data(r)
        run.update(status='in_progress', conclusion=None)
        env = dict(GITHUB_SHA=r['commit'], GITHUB_RUN_ID='123', GITHUB_RUN_ATTEMPT='1')
        with patch.object(tags,'request',return_value=tags.json.dumps(run).encode()):
            with patch.dict(tags.os.environ, env, clear=True):
                tags.verify_run(r,False)
            for key in env:
                with self.subTest(key=key), patch.dict(tags.os.environ,dict(env,**{key:'wrong'}),clear=True), self.assertRaises(ValueError):
                    tags.verify_run(r,False)
        run['event']='push'
        with patch.object(tags,'request',return_value=tags.json.dumps(run).encode()), patch.dict(tags.os.environ,env,clear=True), self.assertRaises(ValueError):
            tags.verify_run(r,False)

    def test_approved_current_bundle_workflow_run_separates_controller_source(self):
        from unittest.mock import patch
        import subprocess
        for bundle in tags.BUNDLES:
            r = tags.validate_record(self.record(bundle=bundle,image='quay.io/bundledock/'+tags._CONFIG.select_bundle(bundle)['repository']))
            run = self.run_data(r)
            run.update(head_sha='a'*40,path='.github/workflows/publish-bundles.yml',
                       event='workflow_run',status='in_progress',conclusion=None)
            env = dict(GITHUB_SHA='a'*40,GITHUB_RUN_ID='123',GITHUB_RUN_ATTEMPT='1',
                       GITHUB_EVENT_NAME='workflow_run',PUBLICATION_SHA=r['commit'])
            with self.subTest(bundle=bundle), patch.object(tags,'request',return_value=tags.json.dumps(run).encode()), patch.dict(tags.os.environ,env,clear=True), patch.object(tags.subprocess,'run',return_value=subprocess.CompletedProcess([],0,'','')) as gate:
                tags.verify_run(r,False)
                self.assertEqual(gate.call_count,1)
                self.assertEqual(gate.call_args.args[0],['python3','scripts/bundle-publication.py','--bundle',bundle])
            with self.subTest(bundle=bundle,case='approval gate failure'), patch.object(tags,'request',return_value=tags.json.dumps(run).encode()), patch.dict(tags.os.environ,env,clear=True), patch.object(tags.subprocess,'run',return_value=subprocess.CompletedProcess([],1,'','')), self.assertRaises(ValueError):
                tags.verify_run(r,False)
            for key in ('PUBLICATION_SHA','GITHUB_SHA','GITHUB_EVENT_NAME'):
                with self.subTest(bundle=bundle,key=key), patch.object(tags,'request',return_value=tags.json.dumps(run).encode()), patch.dict(tags.os.environ,dict(env,**{key:'wrong'}),clear=True), self.assertRaises(ValueError):
                    tags.verify_run(r,False)


class RedirectTests(unittest.TestCase):
    def redirect(self, target):
        return tags.urllib.error.HTTPError('https://quay.io/blob',302,'redirect',{'Location':target},None)

    def test_authenticated_requests_refuse_redirects(self):
        from unittest.mock import patch, Mock
        opener = Mock()
        opener.open.side_effect = self.redirect('https://storage.example/blob')
        with patch.object(tags.urllib.request,'build_opener',return_value=opener), self.assertRaises(tags.urllib.error.HTTPError):
            tags.request('https://quay.io/manifest',{'Authorization':'Bearer secret'})
        self.assertEqual(opener.open.call_count,1)

    def test_signed_blob_strips_auth_and_refuses_second_redirect(self):
        from unittest.mock import patch, Mock, MagicMock
        opener = Mock()
        response = MagicMock()
        response.__enter__.return_value=response
        response.read.return_value=b'{}'
        opener.open.side_effect=[self.redirect('https://storage.example/blob?signature=test'),response]
        with patch.object(tags.urllib.request,'build_opener',return_value=opener):
            self.assertEqual(tags.request('https://quay.io/blob',{'Authorization':'Bearer secret'},True),b'{}')
        self.assertEqual(opener.open.call_args_list[1].args,('https://storage.example/blob?signature=test',))
        opener.open.side_effect=[self.redirect('https://storage.example/blob'),self.redirect('https://other.example/blob')]
        with patch.object(tags.urllib.request,'build_opener',return_value=opener), self.assertRaises(tags.urllib.error.HTTPError):
            tags.request('https://quay.io/blob',{'Authorization':'Bearer secret'},True)

    def test_unsafe_signed_blob_targets_rejected(self):
        from unittest.mock import patch, Mock
        for target in ('http://storage.example/blob','https://user:pass@storage.example/blob','https://storage.example:443/blob'):
            opener=Mock()
            opener.open.side_effect=self.redirect(target)
            with self.subTest(target=target), patch.object(tags.urllib.request,'build_opener',return_value=opener), self.assertRaises(ValueError):
                tags.request('https://quay.io/blob',{'Authorization':'Bearer secret'},True)
            self.assertEqual(opener.open.call_count,1)

class SemanticVersionTests(ProvenanceTests):
    def test_semantic_source_tag_binds_explicit_attempt(self):
        r = tags.validate_record(self.record(tag='24.21.0-b2', run_attempt='7'))
        self.assertEqual(tags.tag_name(r), 'node/24.21.0-b2')
        self.assertIsNone(r['run_number'])
        self.assertEqual(r['attempt'], 7)
        self.assertIn('/attempts/7', tags.annotation(r))
        for attempt in (None, '', '0', '01', 'bad'):
            with self.subTest(attempt=attempt), self.assertRaises(ValueError):
                tags.validate_record(self.record(tag='24.21.0-b2', run_attempt=attempt))
        with self.assertRaises(ValueError):
            tags.validate_record(self.record(bundle='toolbox',image='quay.io/bundledock/toolbox', tag='24.21.0-b2',run_attempt='7'))

    def test_semantic_labels_bind_attempt(self):
        r = tags.validate_record(self.record(tag='24.21.0-b2',run_attempt='7'))
        labels = {'org.opencontainers.image.'+key:r[field] for key,field in [('source','source'),('revision','commit'),('version','tag')]}
        labels.update({'io.bundledock.bundle':'node','io.bundledock.github.run-id':'123','io.bundledock.github.run-attempt':'7'})
        tags.verify_labels(r,labels)
        labels['io.bundledock.github.run-attempt']='8'
        with self.assertRaises(ValueError): tags.verify_labels(r,labels)

class CallerMigrationTests(RunTests):
    def test_legacy_caller_is_historical_evidence_only(self):
        from unittest.mock import patch
        r = tags.validate_record(self.record())
        run = self.run_data(r)
        run['path'] = '.github/workflows/bundledock-node.yml'
        env = dict(GITHUB_SHA=r['commit'], GITHUB_RUN_ID='123', GITHUB_RUN_ATTEMPT='1')
        with patch.object(tags, 'request', return_value=tags.json.dumps(run).encode()):
            tags.verify_run(r, True)
            with patch.dict(tags.os.environ, env, clear=True), self.assertRaises(ValueError):
                tags.verify_run(r, False)
