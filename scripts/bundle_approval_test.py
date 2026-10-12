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
  self.bundle='toolbox'; self.lock='bundles/toolbox/tools.lock.json'; self.inventory='docs/toolbox.md'
  self.pr={'number':42,'node_id':'PR42','state':'open','merged':False,'commits':1,'changed_files':2,'head':{'sha':A,'ref':'automation/toolbox-upgrade-123-1','repo':REPO},'base':{'ref':'main','sha':B,'repo':REPO},'user':{'login':'github-actions[bot]','type':'Bot'},'html_url':'https://github.com/wstein/bundledock/pull/42'}
  self.files=[{'filename':self.lock,'status':'modified','changes':2},{'filename':self.inventory,'status':'modified','changes':2}]
  self.data={'':REPO,'/git/ref/heads/main':{'object':{'sha':B}},'/pulls/42':self.pr,'/pulls/42/files?per_page=100&page=1':self.files,'/pulls/42/reviews?per_page=100&page=1':[{'id':1,'submitted_at':'2026-10-12T10:00:00Z','state':'APPROVED','commit_id':A,'user':{'login':'wstein','type':'User'}}],'/compare/'+B+'...'+A:{'merge_base_commit':{'sha':B},'files':self.files},'/git/commits/'+A:{'tree':{'sha':T}},'/contents/'+self.lock+'?ref='+A:{'encoding':'base64','content':'bmV3'},'/contents/'+self.lock+'?ref='+B:{'encoding':'base64','content':'b2xk'}}
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
  self.data['/pulls/42/reviews?per_page=100&page=1'].append({'id':2,'submitted_at':'2026-10-12T10:01:00Z','state':'CHANGES_REQUESTED','commit_id':A,'user':{'login':'wstein','type':'User'}})
  with self.assertRaises(ValueError):gate.approved_pr(42,self.bundle,self.get)
 def test_submission_chronology_overrides_creation_id(self):
  votes=self.data['/pulls/42/reviews?per_page=100&page=1']
  votes[0]['id']=2
  votes.append(dict(votes[0],id=1,state='CHANGES_REQUESTED',submitted_at='2026-10-12T10:01:00Z'))
  with self.assertRaises(ValueError):gate.approved_pr(42,self.bundle,self.get)
 def test_conflicting_latest_timestamp_rejects(self):
  votes=self.data['/pulls/42/reviews?per_page=100&page=1']
  votes[0]['id']=2
  votes.append(dict(votes[0],id=1,state='CHANGES_REQUESTED'))
  with self.assertRaises(ValueError):gate.approved_pr(42,self.bundle,self.get)
 def test_commented_review_is_nonvoting(self):
  votes=self.data['/pulls/42/reviews?per_page=100&page=1']
  votes.append(dict(votes[0],id=2,state='COMMENTED',submitted_at='2026-10-12T10:01:00Z'))
  self.assertEqual(gate.approved_pr(42,self.bundle,self.get)['head']['sha'],A)
 def test_missing_or_invalid_vote_time_rejects(self):
  vote=self.data['/pulls/42/reviews?per_page=100&page=1'][0]
  for timestamp in (None,'invalid','2026-99-12T10:00:00Z'):
   with self.subTest(timestamp=timestamp):
    vote['submitted_at']=timestamp
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


 def source_fixture(self, workflow='ci.yml', event='workflow_dispatch'):
  self.event={'repository':REPO,'workflow_run':{'id':123,'run_attempt':2,'head_sha':A}}
  run={'id':123,'run_attempt':2,'workflow_id':7,'path':'.github/workflows/'+workflow,'event':event,'head_sha':A,'head_branch':self.pr['head']['ref'],'status':'completed','conclusion':'success','repository':REPO,'head_repository':REPO,'pull_requests':[{'number':42,'head':{'sha':A}}]}
  self.data['/actions/workflows/'+workflow]={'id':7,'path':'.github/workflows/'+workflow}
  self.data['/actions/runs/123/attempts/2']=run
  self.data['/commits/'+A+'/pulls?per_page=100&page=1']=[{'number':42}]
  self.data['/actions/runs/123/attempts/2/jobs?per_page=100&page=1']={'jobs':[{'name':'check','status':'completed','conclusion':'success','head_sha':A,'check_run_url':'https://api.github.com/repos/wstein/bundledock/check-runs/99'}]}
  self.data['/check-runs/99']={'app':{'id':15368},'name':'check','head_sha':A,'status':'completed','conclusion':'success'}
  return run
 def test_exact_source_ci_and_attempt_rejections(self):
  run=self.source_fixture()
  self.assertEqual(gate.source_run(self.event,self.get)[0],'toolbox')
  original=copy.deepcopy(run)
  for change in ({'workflow_id':8},{'path':'.github/workflows/other.yml'},{'run_attempt':3},{'head_sha':B},{'event':'pull_request_review'},{'conclusion':'failure'},{'head_branch':'main'}):
   with self.subTest(change=change):
    run.clear();run.update(original);run.update(change)
    with self.assertRaises((ValueError,KeyError)):gate.source_run(self.event,self.get)
 def test_review_run_synthetic_merge_identity(self):
  run=self.source_fixture('bundle-review-source.yml','pull_request_review')
  self.assertEqual(gate.approval_run(self.event,self.get)[0],'toolbox')
  synthetic='d'*40
  run['head_sha']=synthetic
  self.data['/commits/'+synthetic]={'commit':{'tree':{'sha':T}},'parents':[{'sha':B},{'sha':A}]}
  self.assertEqual(gate.approval_run(self.event,self.get)[0],'toolbox')
  self.data['/commits/'+synthetic]['parents'].reverse()
  with self.assertRaises(ValueError):gate.approval_run(self.event,self.get)
 def test_merged_approved_tree_and_main_ci(self):
  run=self.source_fixture()
  self.data['/git/ref/heads/main']['object']['sha']=A
  run['head_branch']='main'
  self.pr.update(merged=True,state='closed',merge_commit_sha=A)
  self.data['/commits/'+A]={'commit':{'tree':{'sha':T}},'parents':[{'sha':B}],'files':self.files}
  self.assertEqual(gate.validate(self.event,self.get), (A,self.pr['html_url']))
  self.data['/commits/'+A]['commit']['tree']['sha']=B
  with self.assertRaises(ValueError):gate.validate(self.event,self.get)
 def merged_fixture(self):
  self.source_fixture()
  self.data['/git/ref/heads/main']['object']['sha']=A
  self.data['/actions/runs/123/attempts/2']['head_branch']='main'
  self.pr.update(merged=True,state='closed',merge_commit_sha=A)
  self.data['/commits/'+A]={'commit':{'tree':{'sha':T}},'parents':[{'sha':B}],'files':self.files}
 def test_ordinary_main_ci_is_ineligible_for_all_bundles(self):
  self.merged_fixture()
  self.pr['head']['ref']='ci/shared-source-change'
  self.pr['user']={'login':'wstein','type':'User'}
  for name in gate.config.BUNDLES:
   with self.subTest(bundle=name):
    self.assertIsNone(gate.validate(self.event,self.get,name,allow_ineligible=True))
    with self.assertRaises(ValueError):gate.validate(self.event,self.get,name)
 def test_unassociated_main_ci_is_ineligible(self):
  self.merged_fixture()
  self.data['/commits/'+A+'/pulls?per_page=100&page=1']=[]
  self.assertIsNone(gate.validate(self.event,self.get,allow_ineligible=True))
 def test_other_bundle_requires_valid_updater_before_skip(self):
  self.merged_fixture()
  for name in ('node','bun','code-server'):
   with self.subTest(bundle=name):
    self.assertIsNone(gate.validate(self.event,self.get,name,allow_ineligible=True))
  self.data['/pulls/42/reviews?per_page=100&page=1'][0]['state']='DISMISSED'
  for name in gate.config.BUNDLES:
   with self.subTest(bundle=name),self.assertRaises(ValueError):
    gate.validate(self.event,self.get,name,allow_ineligible=True)
 def test_claimed_updater_merge_mismatch_rejects(self):
  for change in ({'merged':False},{'merge_commit_sha':B}):
   with self.subTest(change=change):
    self.setUp();self.merged_fixture();self.pr.update(change)
    with self.assertRaises(ValueError):gate.validate(self.event,self.get,allow_ineligible=True)
 def test_api_ambiguity_is_not_ineligibility(self):
  self.merged_fixture()
  self.data['/commits/'+A+'/pulls?per_page=100&page=1']=[{'number':42}]*100
  with self.assertRaises(ValueError):gate.validate(self.event,self.get,allow_ineligible=True)
 def test_malformed_claim_is_not_benign(self):
  self.merged_fixture()
  self.pr['head']['ref']='automation/toolbox-upgrade-tampered'
  with self.assertRaises(ValueError):gate.validate(self.event,self.get,allow_ineligible=True)
 def test_controller_ordinary_ci_and_review_have_no_writes(self):
  self.source_fixture()
  self.pr['head']['ref']='ci/shared-source-change'
  self.data['/actions/runs/123/attempts/2']['head_branch']=self.pr['head']['ref']
  writes=[]
  def request(path,method='GET',payload=None):
   if method!='GET':writes.append((path,method));raise AssertionError('unexpected write')
   return self.get(path)
  self.assertEqual(gate.merge_approved(self.event,request),{'eligible':'false'})
  self.source_fixture('bundle-review-source.yml','pull_request_review')
  self.assertEqual(gate.dispatch_source(self.event,request),{'eligible':'false'})
  self.assertEqual(writes,[])
 def test_required_check_head_must_match(self):
  self.source_fixture()
  self.data['/actions/runs/123/attempts/2/jobs?per_page=100&page=1']['jobs'][0]['head_sha']=B
  with self.assertRaises(ValueError):gate.source_run(self.event,self.get)
 def test_all_bundle_registry_and_callers(self):
  import os
  from unittest.mock import patch
  for name in ('toolbox','code-server','node','bun'):
   with self.subTest(name=name):
    bundle=gate.config.select_bundle(name)
    workflow={'id':8,'path':'.github/workflows/'+bundle['workflow']}
    run=dict(workflow,id=456,workflow_id=8,run_attempt=2,event='workflow_run',head_sha=A,repository=REPO,head_repository=REPO)
    def get(path):
     return workflow if path.startswith('/actions/workflows/') else run
    with patch.dict(os.environ,{'GITHUB_RUN_ID':'456','GITHUB_RUN_ATTEMPT':'2','GITHUB_SHA':A}):
     gate.validate_caller(get,bundle)
     run['path']='.github/workflows/wrong.yml'
     with self.assertRaises(ValueError):gate.validate_caller(get,bundle)


 def test_substituted_check_app_rejects(self):
  self.source_fixture()
  self.data['/check-runs/99']['app']['id']=42
  with self.assertRaises(ValueError):gate.source_run(self.event,self.get)
 def test_no_write_after_vote_revoked_during_revalidation(self):
  self.source_fixture()
  writes=[]; reads=0
  def request(path, method='GET', payload=None):
   nonlocal reads
   if method!='GET':writes.append((path,method,payload));raise AssertionError('unexpected write')
   if path=='/pulls/42/reviews?per_page=100&page=1':
    reads+=1
    if reads>=2:self.data[path][0]['state']='DISMISSED'
   return self.get(path)
  with self.assertRaises(ValueError):gate.merge_approved(self.event,request)
  self.assertEqual(writes,[])
 def test_postmerge_tree_drift_never_dispatches_publication_ci(self):
  self.source_fixture();writes=[]
  def request(path, method='GET', payload=None):
   if method=='PUT':
    writes.append((path,method,payload))
    self.pr.update(merged=True,state='closed',merge_commit_sha=A)
    self.data['/commits/'+A]={'commit':{'tree':{'sha':B}},'parents':[{'sha':B}],'files':self.files}
    return {'merged':True,'sha':A}
   if method!='GET':writes.append((path,method,payload));return None
   return self.get(path)
  with self.assertRaises(ValueError):gate.merge_approved(self.event,request)
  self.assertEqual(len(writes),1)
  self.assertEqual(writes[0][2],{'sha':A,'merge_method':'rebase'})


class WorkflowResultTests(unittest.TestCase):
 def test_shared_job_conditions_fail_closed(self):
  import re
  from types import SimpleNamespace as NS
  workflow=(Path(__file__).parents[1]/'.github/workflows/bundle-publish.yml').read_text()
  for name in ('resolve','native','promote'):
   body=re.search(r'^  '+name+r':\n(.*?)(?=^  [a-z]+:|\Z)',workflow,re.M|re.S).group(1)
   expression=re.search(r'^    if: >-\n((?:      .*\n)+)',body,re.M).group(1)
   expression=' '.join(expression.split()).replace('&&',' and ').replace('||',' or ')
   for event in ('workflow_run','workflow_dispatch','push','unknown'):
    for result in ('success','failure','cancelled','skipped'):
     github=NS(event_name=event,event=NS(repository=NS(default_branch='main'),sender=NS(type='User')),ref='refs/heads/main',sha=A)
     needs=NS(preflight=NS(result=result,outputs=NS(eligible='true')),resolve=NS(result='success',outputs=NS(publish='true')),native=NS(result='success'))
     inputs=NS(approved_sha=A)
     actual=eval(expression,{'__builtins__':{}},{'github':github,'needs':needs,'inputs':inputs})
     self.assertEqual(actual,result=='success' and event in ('workflow_run','workflow_dispatch'))
   github.event_name='workflow_run';needs.preflight.result='success'
   for eligibility in ('false','',None):
    needs.preflight.outputs.eligible=eligibility
    self.assertFalse(eval(expression,{'__builtins__':{}},{'github':github,'needs':needs,'inputs':inputs}))
   needs.preflight.outputs.eligible='true'
   github.event_name='workflow_dispatch';needs.preflight.result='success';inputs.approved_sha=B
   self.assertFalse(eval(expression,{'__builtins__':{}},{'github':github,'needs':needs,'inputs':inputs}))

if __name__=='__main__':unittest.main()
