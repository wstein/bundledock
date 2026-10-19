import copy
import io
import importlib.util
import os
import sys
import unittest
from pathlib import Path
from unittest.mock import patch
sys.dont_write_bytecode = True
spec = importlib.util.spec_from_file_location('gate', Path(__file__).with_name('bundle-publication.py'))
gate = importlib.util.module_from_spec(spec)
spec.loader.exec_module(gate)
A, B, H, T = 'a'*40, 'b'*40, 'd'*40, 'c'*40
REPO = {'full_name':'wstein/bundledock','default_branch':'main'}

class OwnerMergeTests(unittest.TestCase):
 def setUp(self):
  self.bundle=gate.config.select_bundle('toolbox')
  self.pr={'number':42,'state':'closed','merged':True,'merged_by':{'login':'wstein','type':'User'},'commits':1,'changed_files':2,'merge_commit_sha':A,'head':{'sha':H,'ref':'automation/toolbox-upgrade-123-1','repo':REPO},'base':{'ref':'main','sha':B,'repo':REPO},'user':{'login':'github-actions[bot]','type':'Bot'},'html_url':'https://github.com/wstein/bundledock/pull/42'}
  self.files=[{'filename':self.bundle['lock'],'status':'modified','changes':2},{'filename':self.bundle['inventory'],'status':'modified','changes':2}]
  self.event={'repository':REPO,'workflow_run':{'id':123,'run_attempt':2,'head_sha':A}}
  self.run={'id':123,'run_attempt':2,'workflow_id':7,'path':'.github/workflows/ci.yml','event':'push','head_sha':A,'head_branch':'main','status':'completed','conclusion':'success','repository':REPO,'head_repository':REPO}
  self.data={'':REPO,'/git/ref/heads/main':{'object':{'sha':A}},'/pulls/42':self.pr,'/pulls/42/files?per_page=100&page=1':self.files,'/git/commits/'+H:{'tree':{'sha':T}},'/commits/'+A:{'commit':{'tree':{'sha':T}},'parents':[{'sha':B}],'files':self.files},'/actions/workflows/ci.yml':{'id':7,'path':'.github/workflows/ci.yml'},'/actions/runs/123/attempts/2':self.run,'/commits/'+A+'/pulls?per_page=100&page=1':[{'number':42,'state':'closed','merge_commit_sha':A,'head':{'sha':H}}],'/actions/runs/123/attempts/2/jobs?per_page=100&page=1':{'jobs':[{'name':'check','status':'completed','conclusion':'success','head_sha':A,'check_run_url':'https://api.github.com/repos/wstein/bundledock/check-runs/99'}]},'/check-runs/99':{'app':{'id':15368},'name':'check','head_sha':A,'status':'completed','conclusion':'success'}}
  self.data['/actions/runs/123']=copy.deepcopy(self.run)
  for revision,content in ((A,'bmV3'),(B,'b2xk')):self.data['/contents/'+self.bundle['lock']+'?ref='+revision]={'encoding':'base64','content':content}
 def get(self,path):return copy.deepcopy(self.data[path])
 def test_transport_pins_api_contract_with_exact_merge_sha(self):
  import json
  api=gate.GitHub('fixture-token')
  requests=[]
  def open_request(request,timeout):
   requests.append(request)
   result=self.get(request.full_url.removeprefix('https://api.github.com/repos/'+REPO['full_name']))
   # GitHub 2026-03-10 removes merge_commit_sha from every PR response.
   if request.get_header('X-github-api-version')!='2022-11-28':
    if isinstance(result,dict):result.pop('merge_commit_sha',None)
    if isinstance(result,list):
     for item in result:item.pop('merge_commit_sha',None)
   return io.BytesIO(json.dumps(result).encode())
  with patch.object(api.opener,'open',side_effect=open_request),patch('time.sleep'):
   self.assertEqual(gate.validate(self.event,api.request)[0],A)
  self.assertTrue(requests)
  self.assertTrue(all(request.get_header('X-github-api-version')=='2022-11-28' for request in requests))
 def test_known_wrong_associated_merge_rejects_even_when_full_pr_matches(self):
  self.data['/commits/'+A+'/pulls?per_page=100&page=1'][0]['merge_commit_sha']=B
  with self.assertRaises(ValueError):gate.validate(self.event,self.get)
 def test_owner_merge_without_reviews_authorizes_exact_frozen_source(self):
  self.assertEqual(gate.validate(self.event,self.get),(A,self.pr['html_url']))
 def refreshed_updater(self):
  original='e'*40
  self.pr['commits']=2
  commits=[{'sha':original,'parents':[{'sha':B}]},{'sha':H,'parents':[{'sha':original},{'sha':B}]}]
  self.data['/pulls/42/commits?per_page=100&page=1']=commits
  return commits
 def test_main_refresh_merge_retains_exact_lock_only_authority(self):
  self.refreshed_updater()
  self.assertEqual(gate.validate(self.event,self.get),(A,self.pr['html_url']))
 def test_refresh_from_older_main_ancestor_is_verified(self):
  commits=self.refreshed_updater()
  commits[0]['parents'][0]['sha']=T
  commits[1]['parents'][1]['sha']=T
  self.data['/compare/'+T+'...'+B]={'status':'ahead','merge_base_commit':{'sha':T}}
  self.assertEqual(gate.validate(self.event,self.get)[0],A)
 def test_refresh_history_rejects_extra_source_commits_and_foreign_merges(self):
  for kind in ('extra-source','foreign-parent','wrong-merge-base','wrong-first-parent','wrong-head','missing','count','duplicate','octopus','scope'):
   with self.subTest(kind=kind):
    self.setUp();commits=self.refreshed_updater()
    if kind=='extra-source':commits[1]['parents'].pop()
    if kind=='foreign-parent':
     commits[1]['parents'][1]['sha']=T
     self.data['/compare/'+T+'...'+B]={'status':'diverged','merge_base_commit':{'sha':'f'*40}}
    if kind=='wrong-merge-base':
     commits[1]['parents'][1]['sha']=T
     self.data['/compare/'+T+'...'+B]={'status':'ahead','merge_base_commit':{'sha':'f'*40}}
    if kind=='wrong-first-parent':commits[1]['parents'][0]['sha']=T
    if kind=='wrong-head':commits[1]['sha']=T
    if kind=='missing':commits.pop()
    if kind=='count':self.pr['commits']=100
    if kind=='duplicate':commits[1]['sha']=commits[0]['sha']
    if kind=='octopus':commits[1]['parents'].append({'sha':T})
    if kind=='scope':self.files[1]['filename']='cmd/tools-publish/main.go'
    with self.assertRaises(ValueError):gate.validate(self.event,self.get)
 def test_missing_refresh_history_api_evidence_fails_closed(self):
  self.refreshed_updater()
  del self.data['/pulls/42/commits?per_page=100&page=1']
  with self.assertRaises(KeyError):gate.validate(self.event,self.get)
 def test_only_actual_human_owner_merge_authorizes(self):
  for change in ({'merged_by':None},{'merged_by':{'login':'other','type':'User'}},{'merged_by':{'login':'wstein','type':'Bot'}},{'merged':False},{'state':'open'}):
   with self.subTest(change=change):
    self.setUp();self.pr.update(change)
    with self.assertRaises(ValueError):gate.validate(self.event,self.get)
 def test_bot_branch_repository_files_and_tree_remain_required(self):
  for kind in ('author','branch','repository','files','tree','parents','lock','commits'):
   with self.subTest(kind=kind):
    self.setUp()
    if kind=='author':self.pr['user']['login']='other'
    if kind=='branch':self.pr['head']['ref']='automation/toolbox-upgrade-tampered'
    if kind=='repository':self.pr['head']['repo']={'full_name':'other/fork'}
    if kind=='files':self.files[1]['filename']='.github/workflows/ci.yml'
    if kind=='tree':self.data['/commits/'+A]['commit']['tree']['sha']=B
    if kind=='parents':self.data['/commits/'+A]['parents'].append({'sha':H})
    if kind=='lock':self.data['/contents/'+self.bundle['lock']+'?ref='+A]['content']='b2xk'
    if kind=='commits':self.pr['commits']=0
    with self.assertRaises(ValueError):gate.validate(self.event,self.get)
 def test_exact_main_ci_and_check_identity(self):
  for kind in ('attempt','workflow','event','head','main','app','check','job'):
   with self.subTest(kind=kind):
    self.setUp()
    if kind=='attempt':self.run['run_attempt']=3
    if kind=='workflow':self.run['workflow_id']=8
    if kind=='event':self.run['event']='pull_request'
    if kind=='head':self.run['head_sha']=B
    if kind=='main':self.data['/git/ref/heads/main']['object']['sha']=B;self.data['/compare/'+A+'...'+B]={'status':'diverged','merge_base_commit':{'sha':T}}
    if kind=='app':self.data['/check-runs/99']['app']['id']=42
    if kind=='check':self.data['/check-runs/99']['head_sha']=B
    if kind=='job':self.data['/actions/runs/123/attempts/2/jobs?per_page=100&page=1']['jobs'][0]['conclusion']='failure'
    with self.assertRaises(ValueError):gate.validate(self.event,self.get)
 def advance_main(self,status='ahead',base=A,lock='bmV3'):
  newer='e'*40
  self.data['/git/ref/heads/main']['object']['sha']=newer
  self.data['/compare/'+A+'...'+newer]={'status':status,'merge_base_commit':{'sha':base}}
  self.data['/contents/'+self.bundle['lock']+'?ref='+newer]={'encoding':'base64','content':lock}
 def test_unrelated_main_advance_keeps_exact_source_authorized(self):
  self.advance_main()
  self.assertEqual(gate.validate(self.event,self.get),(A,self.pr['html_url']))
 def test_main_advance_rejects_non_ancestor_or_changed_bundle_lock(self):
  for kind,arguments in (('diverged',{'status':'diverged'}),('behind',{'status':'behind'}),('identical',{'status':'identical'}),('wrong-base',{'base':B}),('lock',{'lock':'b3RoZXI='})):
   with self.subTest(kind=kind):
    self.setUp();self.advance_main(**arguments)
    with self.assertRaises(ValueError):gate.validate(self.event,self.get)
 def test_main_advance_with_unavailable_evidence_fails_closed(self):
  self.advance_main()
  del self.data['/compare/'+A+'...'+'e'*40]
  with self.assertRaises(KeyError):gate.validate(self.event,self.get)
 def test_old_success_cannot_authorize_after_newer_ci_attempt(self):
  for change in ({'run_attempt':3,'status':'in_progress','conclusion':None},{'run_attempt':3,'status':'completed','conclusion':'failure'},{'run_attempt':3,'status':'completed','conclusion':'success'},{'head_sha':B},{'status':'in_progress','conclusion':None}):
   with self.subTest(change=change):
    self.setUp();self.data['/actions/runs/123'].update(change)
    with self.assertRaises(ValueError):gate.validate(self.event,self.get)
 def test_missing_current_ci_attempt_is_pending_and_fail_closed(self):
  del self.data['/actions/runs/123']
  with self.assertRaises(KeyError):gate.validate(self.event,self.get)
 def test_associated_summary_is_not_full_pr_merge_proof(self):
  self.data['/commits/'+A+'/pulls?per_page=100&page=1']=[{'number':42}]
  self.assertEqual(gate.validate(self.event,self.get)[0],A)
 def test_missing_full_merge_with_exact_association_gets_delayed_retry(self):
  get,reads=self.missing_merge_get(recover_at=2)
  self.data['/commits/'+A+'/pulls?per_page=100&page=1'][0]['merge_commit_sha']=A
  with patch('time.sleep') as sleep:
   self.assertEqual(gate.validate(self.event,get)[0],A)
  self.assertEqual(len(reads),2)
  sleep.assert_called_once_with(2)
 def test_known_wrong_merge_never_retries_even_if_association_matches(self):
  reads=[]
  def get(path):
   result=self.get(path)
   if path=='/pulls/42':
    reads.append(path)
    if len(reads)==1:result['merge_commit_sha']=B
   return result
  with patch('time.sleep') as sleep,self.assertRaises(ValueError):gate.validate(self.event,get)
  self.assertEqual(len(reads),1)
  sleep.assert_not_called()
 def test_missing_merge_recovery_ignores_unrelated_profile_metadata(self):
  def change(result):
   result['merged_by']['avatar_url']='https://example.invalid/new-avatar'
   result['head']['repo']['description']='Updated repository description'
  get,reads=self.missing_merge_get(recover_at=2,changed=change)
  with patch('time.sleep'):
   self.assertEqual(gate.validate(self.event,get)[0],A)
 def missing_merge_get(self, recover_at=None, changed=None, absent=False):
  reads=[]
  self.data['/commits/'+A+'/pulls?per_page=100&page=1'][0]['merge_commit_sha']=None
  def get(path):
   result=self.get(path)
   if path=='/pulls/42':
    reads.append(path)
    if recover_at is None or len(reads)<recover_at:
     result.pop('merge_commit_sha',None)
     if not absent:result['merge_commit_sha']=None
    elif changed:changed(result)
   return result
  return get,reads
 def test_both_missing_merge_fields_recover_only_with_exact_full_pr(self):
  for absent in (False,True):
   with self.subTest(absent=absent):
    self.setUp()
    self.pr['merged_by']['id']=123
    self.pr['user']['id']=456
    get,reads=self.missing_merge_get(recover_at=3,absent=absent)
    with patch('time.sleep') as sleep,patch.object(sys,'stderr',new_callable=io.StringIO) as diagnostic:
     self.assertEqual(gate.validate(self.event,get)[0],A)
    self.assertEqual(len(reads),3)
    self.assertEqual(sleep.call_args_list,[((2,),),((2,),)])
    self.assertIn('attempt=2/3 reason=missing_merge_metadata expected_source='+A,diagnostic.getvalue())
    self.assertIn('observed_merge=None',diagnostic.getvalue())
 def test_persistently_missing_merge_metadata_is_bounded_and_pending(self):
  get,reads=self.missing_merge_get()
  with patch('time.sleep') as sleep,self.assertRaisesRegex(OSError,'Merge metadata unavailable.*expected_source='+A+'.*observed_merge=None'):
   gate.validate(self.event,get,allow_ineligible=True)
  self.assertEqual(len(reads),3)
  self.assertEqual(sleep.call_count,2)
 def test_missing_merge_retry_rejects_changed_or_contradictory_full_pr(self):
  for kind in ('number','head','branch','owner','author','repository','base','merged','state','merge'):
   with self.subTest(kind=kind):
    self.setUp()
    def change(result):
     if kind=='number':result['number']=43
     if kind=='head':result['head']['sha']=B
     if kind=='branch':result['head']['ref']='automation/toolbox-upgrade-456-1'
     if kind=='owner':result['merged_by']['login']='other'
     if kind=='author':result['user']['login']='other'
     if kind=='repository':result['head']['repo']['full_name']='other/fork'
     if kind=='base':result['base']['ref']='other'
     if kind=='merged':result['merged']=False
     if kind=='state':result['state']='open'
     if kind=='merge':result['merge_commit_sha']=B
    get,reads=self.missing_merge_get(recover_at=2,changed=change)
    with patch('time.sleep'),self.assertRaises(ValueError):gate.validate(self.event,get)
    self.assertEqual(len(reads),2)
 def test_missing_merge_retry_requires_trusted_closed_matching_association(self):
  for kind in ('owner','author','repository','association_head','association_state','association_merge','merged','state'):
   with self.subTest(kind=kind):
    self.setUp();get,reads=self.missing_merge_get()
    summary=self.data['/commits/'+A+'/pulls?per_page=100&page=1'][0]
    if kind=='owner':self.pr['merged_by']=None
    if kind=='author':self.pr['user']['login']='other'
    if kind=='repository':self.pr['head']['repo']={'full_name':'other/fork'}
    if kind=='association_head':summary['head']['sha']=B
    if kind=='association_state':summary['state']='open'
    if kind=='association_merge':summary['merge_commit_sha']=B
    if kind=='merged':self.pr['merged']=False
    if kind=='state':self.pr['state']='open'
    with patch('time.sleep') as sleep,self.assertRaises(ValueError):gate.validate(self.event,get)
    self.assertEqual(len(reads),1)
    sleep.assert_not_called()
 def test_missing_merge_recovery_preserves_all_downstream_gates(self):
  for kind in ('commits','files','tree','lock','main','ci'):
   with self.subTest(kind=kind):
    self.setUp();get,reads=self.missing_merge_get(recover_at=2)
    if kind=='commits':self.pr['commits']=0
    if kind=='files':self.files[1]['filename']='.github/workflows/ci.yml'
    if kind=='tree':self.data['/commits/'+A]['commit']['tree']['sha']=B
    if kind=='lock':self.data['/contents/'+self.bundle['lock']+'?ref='+A]['content']='b2xk'
    if kind=='main':self.data['/git/ref/heads/main']['object']['sha']=B;self.data['/compare/'+A+'...'+B]={'status':'diverged','merge_base_commit':{'sha':T}}
    if kind=='ci':self.data['/check-runs/99']['app']['id']=42
    with patch('time.sleep'),self.assertRaises(ValueError):gate.validate(self.event,get)
 def test_missing_merge_recovery_rechecks_current_ci_after_delayed_reads(self):
  recover,reads=self.missing_merge_get(recover_at=2)
  def get(path):
   if path=='/actions/runs/123' and len(reads)>=2:
    return dict(self.run,run_attempt=3,status='in_progress',conclusion=None)
   return recover(path)
  with patch('time.sleep'),self.assertRaisesRegex(ValueError,'Source CI attempt is no longer current'):
   gate.validate(self.event,get)
 def test_missing_merge_execution_emits_pending_without_authorization(self):
  import tempfile
  get,reads=self.missing_merge_get()
  with tempfile.TemporaryDirectory() as directory:
   output=Path(directory)/'output'
   with patch.dict(os.environ,{'GITHUB_OUTPUT':str(output)},clear=True),patch('time.sleep'),patch.object(gate,'main',side_effect=lambda:gate.validate(self.event,get)),self.assertRaisesRegex(SystemExit,'Bundle publication pending:.*expected_source='+A):gate.run()
   self.assertIn('eligible=false',output.read_text())
   self.assertIn('status=pending',output.read_text())
 def test_persistent_mismatch_captures_safe_expected_observed_values(self):
  self.pr['merge_commit_sha']=B
  with self.assertRaisesRegex(ValueError,'expected_source='+A+'.*pr=42.*observed_merge='+B):gate.validate(self.event,self.get)
 def test_unproven_mismatch_does_not_retry(self):
  self.data['/commits/'+A+'/pulls?per_page=100&page=1']=[{'number':42}]
  self.pr['merge_commit_sha']=B;reads=[]
  def get(path):
   if path=='/pulls/42':reads.append(path)
   return self.get(path)
  with self.assertRaises(ValueError):gate.validate(self.event,get)
  self.assertEqual(len(reads),1)
 def test_api_failure_and_ambiguity_fail_closed(self):
  for associated in ([{'number':42}]*100,[{'number':42},{'number':42}]):
   self.data['/commits/'+A+'/pulls?per_page=100&page=1']=associated
   with self.assertRaises(ValueError):gate.validate(self.event,self.get,allow_ineligible=True)
  def unavailable(path):raise OSError('API unavailable')
  with self.assertRaises(OSError):gate.validate(self.event,unavailable,allow_ineligible=True)
 def test_ordinary_source_and_other_bundle_are_ineligible(self):
  self.assertIsNone(gate.validate(self.event,self.get,'node',allow_ineligible=True))
  self.pr['head']['ref']='ci/ordinary-change'
  self.assertIsNone(gate.validate(self.event,self.get,allow_ineligible=True))
 def test_no_review_or_merge_controller_remains(self):
  for controller in ('bundle-review-source.yml','bundle-approved-merge.yml'):
   self.assertFalse((Path(__file__).parents[1]/'.github/workflows'/controller).exists())
  for writer in ('dispatch_source','merge_approved','approval_run','source_run'):
   self.assertFalse(hasattr(gate,writer))
 def test_all_bundles_and_caller_identity(self):
  for name in gate.config.BUNDLES:
   bundle=gate.config.select_bundle(name);workflow={'id':8,'path':'.github/workflows/'+bundle['workflow']}
   run=dict(workflow,id=456,workflow_id=8,run_attempt=2,event='workflow_run',head_sha=A,repository=REPO,head_repository=REPO)
   def get(path):return workflow if path.startswith('/actions/workflows/') else run
   with patch.dict(os.environ,{'GITHUB_RUN_ID':'456','GITHUB_RUN_ATTEMPT':'2','GITHUB_SHA':A},clear=True):
    gate.validate_caller(get,bundle)
    run['path']='.github/workflows/wrong.yml'
    with self.assertRaises(ValueError):gate.validate_caller(get,bundle)

 def test_auto_plan_emits_bundle_contract(self):
  import tempfile
  import json
  with tempfile.TemporaryDirectory() as directory:
   event=Path(directory)/'event';event.write_text(json.dumps(self.event));output=Path(directory)/'output'
   env={'GITHUB_REPOSITORY':REPO['full_name'],'GITHUB_EVENT_NAME':'workflow_run','GITHUB_EVENT_PATH':str(event),'GITHUB_TOKEN':'fixture','GITHUB_OUTPUT':str(output)}
   with patch.dict(os.environ,env,clear=True),patch.object(sys,'argv',['gate','--mode','plan']),patch.object(gate,'GitHub') as api,patch.object(gate,'validate_caller'),patch.object(gate.subprocess,'check_output',return_value=A):
    api.return_value.request.side_effect=self.get
    gate.main()
    os.environ['PUBLICATION_SHA']=A
    gate.main()
    accepted=output.read_text()
    os.environ['PUBLICATION_SHA']=B
    with self.assertRaisesRegex(ValueError,'Authorized source changed'):gate.main()
    self.assertEqual(output.read_text(),accepted)
   self.assertIn('bundle=toolbox',output.read_text())
   self.assertIn('eligible=true',output.read_text())
 def test_publication_fixtures_ignore_ambient_environment(self):
  import tempfile
  with tempfile.TemporaryDirectory() as directory:
   summary=Path(directory)/'summary';summary.write_text('Publisher summary remains unchanged\n')
   ambient={'PUBLICATION_SHA':B,'APPROVED_SHA':B,'BUNDLE':'opentofu','GITHUB_SHA':B,'GITHUB_EVENT_NAME':'workflow_dispatch','GITHUB_REPOSITORY':'other/repository','GITHUB_TOKEN':'ambient-token','GITHUB_OUTPUT':str(Path(directory)/'output'),'GITHUB_STEP_SUMMARY':str(summary),'GITHUB_RUN_ID':'999','GITHUB_RUN_ATTEMPT':'9','GITHUB_ACTOR':'github-actions[bot]','TRIGGERING_ACTOR':'github-actions[bot]','SENDER_TYPE':'Bot','GITHUB_REF':'refs/heads/other','DEFAULT_BRANCH':'other'}
   with patch.dict(os.environ,ambient):
    expected=dict(os.environ)
    for name in ('test_auto_plan_emits_bundle_contract','test_manual_plan_retains_real_approval_gate_and_current_main','test_all_bundles_and_caller_identity','test_missing_merge_execution_emits_pending_without_authorization','test_rejected_and_pending_execution_emit_explicit_outcomes'):
     with self.subTest(fixture=name):
      summary.write_text('Publisher summary remains unchanged\n')
      result=unittest.TestResult()
      OwnerMergeTests(name).run(result)
      self.assertTrue(result.wasSuccessful(),result.errors+result.failures)
      self.assertEqual(dict(os.environ),expected)
      self.assertEqual(summary.read_text(),'Publisher summary remains unchanged\n')
      self.assertFalse((Path(directory)/'output').exists())
 def test_all_bundle_owner_merges_use_own_paths(self):
  for name in gate.config.BUNDLES:
   with self.subTest(name=name):
    self.setUp();bundle=gate.config.select_bundle(name)
    self.pr['head']['ref']='automation/'+name+'-upgrade-123-1'
    self.files[0]['filename']=bundle['lock'];self.files[1]['filename']=bundle['inventory']
    for revision,content in ((A,'bmV3'),(B,'b2xk')):self.data['/contents/'+bundle['lock']+'?ref='+revision]={'encoding':'base64','content':content}
    self.assertEqual(gate.validate(self.event,self.get,name)[0],A)
 def test_manual_plan_retains_real_approval_gate_and_current_main(self):
  import tempfile
  import json
  import subprocess
  with tempfile.TemporaryDirectory() as directory:
   event=Path(directory)/'event';event.write_text(json.dumps({'repository':REPO}));output=Path(directory)/'output'
   env={'GITHUB_REPOSITORY':REPO['full_name'],'GITHUB_EVENT_NAME':'workflow_dispatch','GITHUB_EVENT_PATH':str(event),'GITHUB_TOKEN':'fixture','GITHUB_OUTPUT':str(output),'BUNDLE':'node','APPROVED_SHA':A,'GITHUB_SHA':A,'DEFAULT_BRANCH':'main','GITHUB_REF':'refs/heads/main','SENDER_TYPE':'User','GITHUB_ACTOR':'wstein','TRIGGERING_ACTOR':'wstein'}
   with patch.dict(os.environ,env,clear=True),patch.object(sys,'argv',['gate','--mode','plan']),patch.object(gate,'GitHub') as api,patch.object(gate,'validate_caller'),patch.object(gate.subprocess,'check_output',return_value=A):
    api.return_value.request.side_effect=self.get
    gate.main()
    self.assertIn('bundle=node',output.read_text())
    os.environ['APPROVED_SHA']=B
    with self.assertRaises(subprocess.CalledProcessError):gate.main()
    os.environ['APPROVED_SHA']=A;self.data['/git/ref/heads/main']['object']['sha']=B
    with self.assertRaises(ValueError):gate.main()
 def test_rejected_and_pending_execution_emit_explicit_outcomes(self):
  import tempfile
  for error,status in ((gate.OwnerMergeRequired('owner missing'),'blocked_owner_merge'),(ValueError('source mismatch'),'rejected'),(OSError('unavailable'),'pending')):
   with self.subTest(status=status), tempfile.TemporaryDirectory() as directory:
    output=Path(directory)/'output';summary=Path(directory)/'summary'
    with patch.dict(os.environ,{'GITHUB_OUTPUT':str(output),'GITHUB_STEP_SUMMARY':str(summary)},clear=True),patch.object(gate,'main',side_effect=error),self.assertRaises(SystemExit):gate.run()
    self.assertIn('eligible=false',output.read_text())
    self.assertIn('status='+status,output.read_text())
    self.assertIn(status,summary.read_text())

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
