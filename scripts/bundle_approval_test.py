import copy
import importlib.util
import sys
import unittest
from pathlib import Path
sys.dont_write_bytecode = True
spec = importlib.util.spec_from_file_location('gate', Path(__file__).with_name('bundle-publication.py'))
gate = importlib.util.module_from_spec(spec)
spec.loader.exec_module(gate)
A, B, T = 'a'*40, 'b'*40, 'c'*40
REPO = {'full_name':'wstein/bundledock','default_branch':'main'}

class ApprovalTests(unittest.TestCase):
 def setUp(self):
  self.bundle='toolbox'; self.lock='bundles/toolbox/tools.lock.json'; self.inventory='docs/tools.md'
  self.pr={'number':42,'node_id':'PR42','state':'open','merged':False,'commits':1,'changed_files':2,'head':{'sha':A,'ref':'automation/toolbox-upgrade-123-1','repo':REPO},'base':{'ref':'main','sha':B,'repo':REPO},'user':{'login':'github-actions[bot]','type':'Bot'},'html_url':'https://github.com/wstein/bundledock/pull/42'}
  self.files=[{'filename':self.lock,'status':'modified','changes':2},{'filename':self.inventory,'status':'modified','changes':2}]
  self.data={'':REPO,'/git/ref/heads/main':{'object':{'sha':B}},'/pulls/42':self.pr,'/pulls/42/files?per_page=100&page=1':self.files,'/pulls/42/reviews?per_page=100&page=1':[{'id':1,'state':'APPROVED','commit_id':A,'user':{'login':'wstein','type':'User'}}],'/compare/'+B+'...'+A:{'merge_base_commit':{'sha':B},'files':self.files},'/git/commits/'+A:{'tree':{'sha':T}},'/contents/'+self.lock+'?ref='+A:{'encoding':'base64','content':'bmV3'},'/contents/'+self.lock+'?ref='+B:{'encoding':'base64','content':'b2xk'}}
 def get(self,path):return copy.deepcopy(self.data[path])
 def test_one_exact_approval(self):self.assertEqual(gate.approved_pr(42,self.bundle,self.get)['head']['sha'],A)
 def test_reject_wrong_or_stale_approval(self):
  for change in ({'state':'DISMISSED'},{'state':'CHANGES_REQUESTED'},{'commit_id':B},{'user':{'login':'other','type':'User'}}):
   with self.subTest(change=change):
    original=copy.deepcopy(self.data['/pulls/42/reviews?per_page=100&page=1'])
    self.data['/pulls/42/reviews?per_page=100&page=1'][0].update(change)
    with self.assertRaises(ValueError):gate.approved_pr(42,self.bundle,self.get)
    self.data['/pulls/42/reviews?per_page=100&page=1']=original
 def test_latest_vote_and_head_changes_reject(self):
  self.data['/pulls/42/reviews?per_page=100&page=1'].append({'id':2,'state':'CHANGES_REQUESTED','commit_id':A,'user':{'login':'wstein','type':'User'}})
  with self.assertRaises(ValueError):gate.approved_pr(42,self.bundle,self.get)
 def test_wrong_bundle_rejects(self):
  for bundle in ('node','bun','code-server','arbitrary'):
   with self.subTest(bundle=bundle),self.assertRaises(ValueError):gate.approved_pr(42,bundle,self.get)
 def test_unreviewed_main_drift_rejects(self):
  self.data['/git/ref/heads/main']['object']['sha']=T
  with self.assertRaises((ValueError,KeyError)):gate.approved_pr(42,self.bundle,self.get)
 def test_paths_and_tree_reject(self):
  self.files[1]['filename']='.github/workflows/ci.yml'
  with self.assertRaises(ValueError):gate.approved_pr(42,self.bundle,self.get)

if __name__=='__main__':unittest.main()
