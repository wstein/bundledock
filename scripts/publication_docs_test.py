import importlib.util
import json
from pathlib import Path
import tempfile
import subprocess
import unittest
from unittest.mock import patch

SPEC = importlib.util.spec_from_file_location('publication_docs', Path(__file__).with_name('publication-docs.py'))

class PublicationDocsTest(unittest.TestCase):
    def setUp(self):
        self.module = importlib.util.module_from_spec(SPEC)
        SPEC.loader.exec_module(self.module)
        self.receipt = dict(image='quay.io/bundledock/toolbox', tag='2026.10.12-b5.1', commit='a'*40,
            source='https://github.com/wstein/bundledock', created='2026-10-12T12:00:00Z',
            lock_sha256='b'*64, run_id='123', digest='sha256:'+'c'*64,
            reference='quay.io/bundledock/toolbox@sha256:'+'c'*64, platforms=[])
        for arch in ['amd64', 'arm64']:
            self.receipt['platforms'].append({**{k:self.receipt[k] for k in ['image','tag','commit','source','created','lock_sha256','run_id']},
                'architecture':arch, 'image_id':'sha256:'+'d'*64, 'smoked':True, 'digest':'sha256:'+'e'*64})
    def validate(self):
        return self.module.validate(self.receipt, 'toolbox', 'quay.io/bundledock/toolbox', 'a'*40, '123', '1')
    def test_daily_revision_preserves_explicit_attempt_and_historical_order(self):
        self.receipt.update(tag='2026.10.12-b1', run_attempt='1')
        for platform in self.receipt['platforms']:
            platform.update(tag=self.receipt['tag'], run_attempt='1')
        record = self.validate()
        self.assertEqual(record['attempt'], '1')
        self.assertGreater(self.module.version_key('2026.10.12-b1'), self.module.version_key('2026.10.12-b98.1'))
        self.assertLess(self.module.version_key('2026.10.12-b9'), self.module.version_key('2026.10.13-b1'))
        original = self.receipt['created']
        self.receipt['created'] = '2026-10-13T00:00:00Z'
        for platform in self.receipt['platforms']:
            platform['created'] = self.receipt['created']
        with self.assertRaises(ValueError): self.validate()
        self.receipt['created'] = original
        for platform in self.receipt['platforms']:
            platform['created'] = original
        self.receipt['platforms'][0]['run_attempt'] = '2'
        with self.assertRaises(ValueError): self.validate()

    def test_valid_and_invalid_receipts(self):
        self.validate()
        for key, value in [('image','quay.io/evil/toolbox'), ('source','https://evil.test'), ('commit','a'*39), ('digest','sha256:bad'), ('reference','quay.io/bundledock/toolbox@sha256:'+'f'*64), ('run_id','124'), ('created','bad')]:
            with self.subTest(key=key):
                original=self.receipt[key]; self.receipt[key]=value
                with self.assertRaises(ValueError): self.validate()
                self.receipt[key]=original
        self.receipt['platforms'][0]['smoked']=False
        with self.assertRaises(ValueError): self.validate()
    def test_managed_only_idempotent_and_escape_rejected(self):
        record=self.validate()
        with tempfile.TemporaryDirectory() as directory:
            root=Path(directory); (root/'docs').mkdir()
            path=root/'docs/toolbox.md'
            original='before\n'+self.module.BEGIN+'\n'+self.module.END+'\nafter\n'
            path.write_text(original)
            self.assertTrue(self.module.update(root,'toolbox',record))
            content=path.read_text()
            self.assertTrue(content.startswith('before\n')); self.assertTrue(content.endswith('\nafter\n'))
            self.assertFalse(self.module.update(root,'toolbox',record))
            path.unlink(); path.symlink_to(root/'elsewhere')
            with self.assertRaises(ValueError): self.module.update(root,'toolbox',record)
    def test_all_nine_mappings_and_newer_guard(self):
        self.assertEqual(set(self.module.BUNDLES), {'toolbox','code-server','node','node-current','bun','codex','agy','claude','opentofu'})
        record=self.validate()
        with tempfile.TemporaryDirectory() as directory:
            root=Path(directory); (root/'docs').mkdir()
            path=root/'docs/toolbox.md'; path.write_text(self.module.BEGIN+'\n'+self.module.END)
            self.module.update(root,'toolbox',record)
            older=dict(record,tag='2026.10.10-b4.1')
            with self.assertRaises(ValueError): self.module.update(root,'toolbox',older)

    def test_canonical_guides_accept_publication_without_changing_inventory_or_prose(self):
        repository = Path(__file__).resolve().parents[1]
        for bundle, relative in self.module.BUNDLES.items():
            with self.subTest(bundle=bundle), tempfile.TemporaryDirectory() as directory:
                root = Path(directory)
                path = root / relative
                path.parent.mkdir(parents=True)
                source = (repository / relative).read_bytes()
                path.write_bytes(source)
                receipt = json.loads(json.dumps(self.receipt))
                receipt.update(bundle=bundle, image='quay.io/bundledock/' + self.module._CONFIG.select_bundle(bundle)['repository'],
                               tag='2099.01.01-b1.1' if bundle == 'toolbox' else '999.0.0-b1',
                               run_attempt='1')
                receipt['reference'] = receipt['image'] + '@' + receipt['digest']
                for platform in receipt['platforms']:
                    platform.update({key: receipt[key] for key in self.module.FIELDS})
                    platform['run_attempt'] = '1'
                    platform['bundle'] = bundle
                record = self.module.validate(receipt, bundle, receipt['image'], 'a'*40, '123', '1')
                self.assertTrue(self.module.update(root, bundle, record))
                prefix, managed = source.decode().split(self.module.BEGIN)
                _, suffix = managed.split(self.module.END)
                expected = prefix + self.module.BEGIN + self.module.render(record) + self.module.END + suffix
                self.assertEqual(path.read_bytes(), expected.encode())
                self.assertIn('<!-- BEGIN GENERATED TOOL INVENTORY -->', suffix)
                self.assertFalse(self.module.update(root, bundle, record))

    def test_missing_duplicate_and_reversed_publication_markers_fail_without_write(self):
        begin, end = self.module.BEGIN, self.module.END
        for source in ['prose only', begin, end, begin + end + begin,
                       begin + end + end, end + begin]:
            with self.subTest(source=source), tempfile.TemporaryDirectory() as directory:
                root = Path(directory)
                path = root / self.module.BUNDLES['toolbox']
                path.parent.mkdir(parents=True)
                path.write_text(source)
                with self.assertRaises(ValueError):
                    self.module.update(root, 'toolbox', self.validate())
                self.assertEqual(path.read_text(), source)

    def test_atomic_proposal_races_idempotency_and_prose(self):
        record=self.validate()
        for mode in ['new','moved','identical','conflict','wrong-pr','topic-moved','current']:
            with self.subTest(mode=mode), tempfile.TemporaryDirectory() as directory:
                root=Path(directory); base='1'*40; head='2'*40; tree='3'*40
                branch='publication-docs/toolbox/'+record['tag']
                source='\n\nbefore  \n'+self.module.BEGIN+'\n'+self.module.END+'\nafter  \n\n'
                desired=self.module.replacement(source,record)
                calls=[]; api_calls=[]
                def fake(*args):
                    calls.append(args)
                    if args[:3]==('git','status','--porcelain'): return ''
                    if args[:2]==('gh','api'): return 'main'
                    if args[:4]==('git','remote','get-url','origin'): return self.module.SOURCE+'.git'
                    if args[:2]==('git','fetch'): return ''
                    if args[:2]==('git','ls-tree'): return 'docs/toolbox-published.json' if mode == 'current' else ''
                    if args[:3]==('git','rev-parse','origin/main'): return base
                    if args[:2]==('git','ls-remote'): return ('4'*40 if mode=='moved' else base)+'\trefs/heads/main'
                    if args[:3]==('gh','pr','list'):
                        if mode=='new': return '[]'
                        return json.dumps([dict(number=1,headRefName=branch,headRefOid=head,baseRefName='other' if mode=='wrong-pr' else 'main',isCrossRepository=False)])
                    if args[:3] in [('gh','pr','create'),('gh','workflow','run')]: return 'ok'
                    raise AssertionError(args)
                def fake_api(path,payload=None):
                    api_calls.append((path,payload))
                    if path=='git/commits/'+base: return {'tree':{'sha':'5'*40}}
                    if path=='git/trees':
                        self.assertEqual(payload['tree'],[dict(path='docs/toolbox.md',mode='100644',type='blob',content=desired),
                            dict(path='docs/toolbox-published.json',mode='100644',type='blob',content=self.module.metadata('toolbox',record))])
                        self.assertTrue(payload['tree'][0]['content'].startswith('\n\nbefore  \n'))
                        self.assertTrue(payload['tree'][0]['content'].endswith('\nafter  \n\n'))
                        return {'sha':tree}
                    if path=='git/commits':
                        self.assertEqual(payload['parents'],[base]); self.assertIn('\n\n',payload['message'])
                        return {'sha':head}
                    if path=='git/refs':
                        self.assertEqual(payload,{'ref':'refs/heads/'+branch,'sha':head})
                        if mode in ['identical','conflict','wrong-pr']:
                            raise subprocess.CalledProcessError(1,['gh'],stderr='HTTP 422')
                        return {'object':{'sha':head}}
                    if path=='git/ref/heads/'+branch: return {'object':{'sha':'6'*40 if mode=='topic-moved' else head}}
                    if path=='git/commits/'+head: return {'tree':{'sha':'7'*40 if mode=='conflict' else tree},'parents':[{'sha':base}]}
                    raise AssertionError(path)
                old=Path.cwd()
                try:
                    with patch.object(self.module,'command',side_effect=fake), patch.object(self.module,'api',side_effect=fake_api), patch.object(self.module,'source_at',side_effect=lambda base,path: self.module.metadata('toolbox',record) if path.endswith('.json') else (desired if mode=='current' else source)):
                        if mode in ['moved','conflict','wrong-pr','topic-moved']:
                            with self.assertRaises(ValueError): self.module.propose(root,'toolbox',record)
                        else: self.module.propose(root,'toolbox',record)
                finally: self.module.os.chdir(old)
                self.assertFalse(any(call[:2]==('git','push') for call in calls))
                if mode=='current': self.assertEqual(api_calls,[])
                if mode in ['moved','conflict','wrong-pr','topic-moved']:
                    self.assertFalse(any(call[:3]==('gh','workflow','run') for call in calls))
                else:
                    if mode!='current': self.assertTrue(any('expected_head_sha='+head in call for call in calls))
                if mode=='identical': self.assertFalse(any(call[:3]==('gh','pr','create') for call in calls))

    def test_every_bundle_metadata_retains_canonical_evidence_without_enlarging_markdown(self):
        record = self.validate()
        evidence = {'receipt': self.receipt, 'attempt': {'id': 123, 'run_attempt': 1}}
        for bundle in self.module.BUNDLES:
            with self.subTest(bundle=bundle):
                self.assertEqual(self.module.metadata_path(bundle), 'docs/' + bundle + '-published.json')
                metadata = json.loads(self.module.metadata(bundle, record, evidence=evidence))
                self.assertEqual(metadata['publication_evidence'], evidence)
                self.assertNotIn('platforms', self.module.render(record))

    def test_node_metadata_tracks_receipt_not_source_inventory(self):
        for bundle, tag, upstream in [('node', '24.21.0-b2', '24.21.0'),
                                      ('node-current', '26.11.1-b1', '26.11.1'),
                                      ('node', '2026.10.12-b1.1', 'unknown')]:
            with self.subTest(bundle=bundle, tag=tag), tempfile.TemporaryDirectory() as directory:
                root = Path(directory); (root/'docs').mkdir()
                record = dict(self.validate(), bundle=bundle, tag=tag,
                              image='quay.io/bundledock/node')
                record['reference'] = record['image']+'@'+record['digest']
                guide = root/self.module.BUNDLES[bundle]
                guide.write_text(self.module.BEGIN+'\n'+self.module.END+'\nsource inventory: 999.0.0\n')
                self.assertTrue(self.module.update(root, bundle, record))
                metadata = json.loads((root/('docs/'+bundle+'-published.json')).read_text())
                self.assertEqual(metadata['upstream_version'], upstream)
                for key in ('tag', 'digest', 'commit', 'image'):
                    self.assertEqual(metadata[key], record[key])
                self.assertEqual(metadata['bundle'], bundle)
                self.assertFalse(self.module.update(root, bundle, record))
                # Restore stale metadata while the guide is already current.
                (root/('docs/'+bundle+'-published.json')).write_text('{}\n')
                self.assertTrue(self.module.update(root, bundle, record))
                self.assertEqual(json.loads((root/('docs/'+bundle+'-published.json')).read_text()), metadata)

    def test_legacy_metadata_preserves_only_matching_verified_publication(self):
        record = dict(self.validate(), bundle='node')
        previous = json.loads(self.module.metadata('node', record))
        previous['upstream_version'] = '24.21.0'
        self.assertEqual(json.loads(self.module.metadata('node', record, json.dumps(previous)))['upstream_version'], '24.21.0')
        for key in ('tag', 'digest', 'commit', 'image'):
            with self.subTest(key=key):
                changed = dict(previous); changed[key] = 'different'
                self.assertEqual(json.loads(self.module.metadata('node', record, json.dumps(changed)))['upstream_version'], 'unknown')

    def test_node_proposal_includes_managed_json_even_when_guide_is_current(self):
        record = dict(self.validate(), bundle='node-current', tag='26.11.1-b2',
                      image='quay.io/bundledock/node')
        source = self.module.BEGIN+'\n'+self.module.END
        desired = self.module.replacement(source, record)
        for guide in (source, desired):
            with self.subTest(current=guide == desired), tempfile.TemporaryDirectory() as directory:
                base = '1'*40
                def fake_command(*args):
                    if args[:2] == ('gh', 'api'): return 'main'
                    if args[:3] == ('git', 'remote', 'get-url'): return self.module.SOURCE
                    if args[:2] == ('git', 'rev-parse'): return base
                    return ''
                def fake_api(path, payload=None):
                    if path == 'git/commits/'+base: return {'tree': {'sha': '2'*40}}
                    self.assertEqual(path, 'git/trees')
                    changes = {item['path']: item['content'] for item in payload['tree']}
                    self.assertEqual(json.loads(changes.pop('docs/node-current-published.json'))['upstream_version'], '26.11.1')
                    self.assertEqual(changes, {} if guide == desired else {'docs/node-current.md': desired})
                    raise RuntimeError('checked proposal tree')
                old = Path.cwd()
                try:
                    with patch.object(self.module, 'command', side_effect=fake_command), patch.object(self.module, 'api', side_effect=fake_api), patch.object(self.module, 'source_at', side_effect=lambda base, path: '{}' if path.endswith('.json') else guide):
                        with self.assertRaisesRegex(RuntimeError, 'checked proposal tree'):
                            self.module.propose(Path(directory), 'node-current', record)
                finally:
                    self.module.os.chdir(old)

    def test_linked_metadata_rejected_before_guide_write(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory); (root/'docs').mkdir()
            guide = root/'docs/node.md'
            source = self.module.BEGIN+'\n'+self.module.END
            guide.write_text(source)
            (root/'docs/node-published.json').symlink_to(root/'elsewhere')
            with self.assertRaises(ValueError):
                self.module.update(root, 'node', dict(self.validate(), tag='24.21.0-b1'))
            self.assertEqual(guide.read_text(), source)

    def test_git_source_read_preserves_crlf(self):
        source=b'\r\n before  \r\n<!-- BEGIN PUBLISHED BUNDLE -->\r\n<!-- END PUBLISHED BUNDLE -->\r\nafter  \r\n\r\n'
        with patch.object(self.module.subprocess,'check_output',return_value=source) as call:
            self.assertEqual(self.module.source_at('a'*40,'docs/toolbox.md').encode('utf-8'),source)
            self.assertNotIn('text',call.call_args.kwargs)

    def test_shared_workflow_docs_job_and_caller_permissions(self):
        root=Path(__file__).resolve().parents[1]
        workflow=(root/'.github/workflows/bundle-publish.yml').read_text()
        self.assertIn('\n  documentation:\n',workflow)
        before, docs=workflow.split('\n  documentation:\n')
        self.assertIn('needs: [preflight, resolve, promote, tag]',docs)
        for condition in ["needs.promote.result == 'success'", "needs.tag.result == 'success'", "needs.resolve.outputs.publish == 'true'"]:
            self.assertIn(condition,docs)
        for permission in ['contents: write','pull-requests: write','actions: write']:
            self.assertIn(permission,docs)
        self.assertIn('path: ${{ runner.temp }}/publication-docs-receipt',docs)
        self.assertIn('ref: ${{ needs.preflight.outputs.sha }}',docs)
        self.assertIn('--propose',docs)
        self.assertNotIn('go build',docs)
        self.assertNotIn('docker ',docs)
        self.assertNotIn('hugo',docs)
        for bundle in self.module.BUNDLES:
            caller=(root/'.github/workflows'/'publish-bundles.yml').read_text()
            default, ceiling=caller.split('\njobs:\n')
            self.assertIn('actions: read',default)
            self.assertNotIn('pull-requests: write',default)
            self.assertIn('pull-requests: write',ceiling)
            self.assertIn('actions: write',ceiling)

class SemanticPublicationDocsTest(PublicationDocsTest):
    def test_semantic_receipt_and_legacy_migration(self):
        for item in [self.receipt,*self.receipt['platforms']]:
            item.update(bundle='node',image='quay.io/bundledock/node',tag='24.21.0-b1',run_attempt='3')
        self.receipt['reference']='quay.io/bundledock/node@'+self.receipt['digest']
        record=self.module.validate(self.receipt,'node','quay.io/bundledock/node','a'*40,'123','3')
        old=dict(record,tag='2026.10.12-b1.1',attempt='1'); old.pop('run_attempt')
        source=self.module.BEGIN+self.module.render(old)+self.module.END
        updated=self.module.replacement(source,record)
        self.assertIn('24.21.0-b1',updated)
        with self.assertRaises(ValueError): self.module.replacement(updated,old)
        newer=dict(record,tag='24.21.0-b2')
        self.module.replacement(updated,newer)
        self.receipt['platforms'][0]['run_attempt']='2'
        with self.assertRaises(ValueError): self.module.validate(self.receipt,'node','quay.io/bundledock/node','a'*40,'123','3')

class CurrentPublicationDocsTest(PublicationDocsTest):
    def test_shared_current_receipt_preserves_source_and_rejects_cross_channel(self):
        for item in [self.receipt,*self.receipt['platforms']]:
            item.update(bundle='node-current',image='quay.io/bundledock/node',tag='26.11.1-b1',run_attempt='1')
        self.receipt['reference']='quay.io/bundledock/node@'+self.receipt['digest']
        validate=lambda: self.module.validate(self.receipt,'node-current','quay.io/bundledock/node','a'*40,'123','1')
        record=validate()
        self.assertEqual(record['bundle'],'node-current')
        self.assertIn('docker pull quay.io/bundledock/node@',self.module.render(record))
        self.receipt['platforms'][1]['bundle']='node'
        with self.assertRaises(ValueError): validate()
        self.receipt['platforms'][1]['bundle']='node-current'
        del self.receipt['bundle']
        with self.assertRaises(ValueError): validate()

    def test_semantic_lts_receipt_identity_required_but_historical_date_allowed(self):
        for item in [self.receipt,*self.receipt['platforms']]:
            item.update(image='quay.io/bundledock/node',tag='26.11.1-b1',run_attempt='1')
        self.receipt['reference']='quay.io/bundledock/node@'+self.receipt['digest']
        validate=lambda: self.module.validate(self.receipt,'node','quay.io/bundledock/node','a'*40,'123','1')
        with self.assertRaises(ValueError): validate()
        for item in [self.receipt,*self.receipt['platforms']]: item['bundle']='node'
        validate()
        del self.receipt['platforms'][1]['bundle']
        with self.assertRaises(ValueError): validate()
        for item in [self.receipt,*self.receipt['platforms']]:
            item.pop('bundle',None);item['tag']='2026.10.12-b1.1'
        validate()
