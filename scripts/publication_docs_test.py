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
    def test_all_seven_mappings_and_newer_guard(self):
        self.assertEqual(set(self.module.BUNDLES), {'toolbox','code-server','node','bun','codex','agy','claude'})
        record=self.validate()
        with tempfile.TemporaryDirectory() as directory:
            root=Path(directory); (root/'docs').mkdir()
            path=root/'docs/toolbox.md'; path.write_text(self.module.BEGIN+'\n'+self.module.END)
            self.module.update(root,'toolbox',record)
            older=dict(record,tag='2026.10.10-b4.1')
            with self.assertRaises(ValueError): self.module.update(root,'toolbox',older)

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
                        self.assertEqual(payload['tree'],[dict(path='docs/toolbox.md',mode='100644',type='blob',content=desired)])
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
                    with patch.object(self.module,'command',side_effect=fake), patch.object(self.module,'api',side_effect=fake_api), patch.object(self.module,'source_at',return_value=desired if mode=='current' else source):
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
            caller=(root/'.github/workflows'/('bundledock-'+bundle+'.yml')).read_text()
            default, ceiling=caller.split('\njobs:\n')
            self.assertIn('actions: read',default)
            self.assertNotIn('pull-requests: write',default)
            self.assertIn('pull-requests: write',ceiling)
            self.assertIn('actions: write',ceiling)

class SemanticPublicationDocsTest(PublicationDocsTest):
    def test_semantic_receipt_and_legacy_migration(self):
        for item in [self.receipt,*self.receipt['platforms']]:
            item.update(image='quay.io/bundledock/node',tag='24.21.0-b1',run_attempt='3')
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
