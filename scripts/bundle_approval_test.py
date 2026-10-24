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
import base64
import json
def b64(value):return base64.b64encode(value if isinstance(value,bytes) else json.dumps(value).encode()).decode()
def plain_lock(version):return {'schema':2,'platforms':{arch:{'tool':{'version':version,'url':'https://example.test/'+version,'sha256':'0'*64,'verification':'publisher-sha256'}} for arch in ('amd64','arm64')}}
OLD,NEW,CHANGED,OTHER=(b64(plain_lock(version)) for version in ('1.0.0','1.0.1','1.0.2','1.0.3'))
PLAIN_CATALOG=b64({'schema':1,'packages':{'tool':{'provider':'github'}}})
PLAIN_PACKAGES=b64(b'tool>=1.0.0\n')
def stage_fixtures(data,bundle):
 for name,content in (('packages.catalog.json',PLAIN_CATALOG),('packages.txt',PLAIN_PACKAGES)):
  for revision in (A,B):data['/contents/'+bundle['directory']+'/'+name+'?ref='+revision]={'encoding':'base64','content':content}

class OwnerMergeTests(unittest.TestCase):
 def setUp(self):
  self.bundle=gate.config.select_bundle('toolbox')
  self.pr={'number':42,'state':'closed','merged':True,'merged_by':{'login':'wstein','type':'User'},'commits':1,'changed_files':2,'merge_commit_sha':A,'head':{'sha':H,'ref':'automation/toolbox-upgrade-123-1','repo':REPO},'base':{'ref':'main','sha':B,'repo':REPO},'user':{'login':'github-actions[bot]','type':'Bot'},'html_url':'https://github.com/wstein/bundledock/pull/42'}
  self.files=[{'filename':self.bundle['lock'],'status':'modified','changes':2},{'filename':self.bundle['inventory'],'status':'modified','changes':2}]
  self.event={'repository':REPO,'workflow_run':{'id':123,'run_attempt':2,'head_sha':A}}
  self.run={'id':123,'run_attempt':2,'workflow_id':7,'path':'.github/workflows/ci.yml','event':'push','head_sha':A,'head_branch':'main','status':'completed','conclusion':'success','repository':REPO,'head_repository':REPO}
  self.data={'':REPO,'/git/ref/heads/main':{'object':{'sha':A}},'/pulls/42':self.pr,'/pulls/42/files?per_page=100&page=1':self.files,'/git/commits/'+H:{'tree':{'sha':T}},'/commits/'+A:{'commit':{'tree':{'sha':T}},'parents':[{'sha':B}],'files':self.files},'/actions/workflows/ci.yml':{'id':7,'path':'.github/workflows/ci.yml'},'/actions/runs/123/attempts/2':self.run,'/commits/'+A+'/pulls?per_page=100&page=1':[{'number':42,'state':'closed','merge_commit_sha':A,'head':{'sha':H}}],'/actions/runs/123/attempts/2/jobs?per_page=100&page=1':{'jobs':[{'name':'check','status':'completed','conclusion':'success','head_sha':A,'check_run_url':'https://api.github.com/repos/wstein/bundledock/check-runs/99'}]},'/check-runs/99':{'app':{'id':15368},'name':'check','head_sha':A,'status':'completed','conclusion':'success'}}
  self.data['/actions/runs/123']=copy.deepcopy(self.run)
  for revision,content in ((A,NEW),(B,OLD)):self.data['/contents/'+self.bundle['lock']+'?ref='+revision]={'encoding':'base64','content':content}
  stage_fixtures(self.data,self.bundle)
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
    if kind=='lock':self.data['/contents/'+self.bundle['lock']+'?ref='+A]['content']=OLD
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
 def advance_main(self,status='ahead',base=A,lock=NEW):
  newer='e'*40
  self.data['/git/ref/heads/main']['object']['sha']=newer
  self.data['/compare/'+A+'...'+newer]={'status':status,'merge_base_commit':{'sha':base}}
  self.data['/contents/'+self.bundle['lock']+'?ref='+newer]={'encoding':'base64','content':lock}
 def test_unrelated_main_advance_keeps_exact_source_authorized(self):
  self.advance_main()
  self.assertEqual(gate.validate(self.event,self.get),(A,self.pr['html_url']))
 def test_main_advance_rejects_non_ancestor_or_changed_bundle_lock(self):
  for kind,arguments in (('diverged',{'status':'diverged'}),('behind',{'status':'behind'}),('identical',{'status':'identical'}),('wrong-base',{'base':B}),('lock',{'lock':OTHER})):
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
    if kind=='lock':self.data['/contents/'+self.bundle['lock']+'?ref='+A]['content']=OLD
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
   self.assertIn('bundles=["toolbox"]',output.read_text())
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
    for revision,content in ((A,NEW),(B,OLD)):self.data['/contents/'+bundle['lock']+'?ref='+revision]={'encoding':'base64','content':content}
    stage_fixtures(self.data,bundle)
    self.assertEqual(gate.validate(self.event,self.get,name)[0],A)
 def test_manual_plan_retains_real_approval_gate_and_current_main(self):
  import tempfile
  import json
  import subprocess
  node=gate.config.select_bundle('node')
  self.data['/contents/'+node['lock']+'?ref='+A]={'encoding':'base64','content':NEW}
  stage_fixtures(self.data,node)
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
 def test_rejection_detail_is_single_line_bounded_output(self):
  import tempfile
  with tempfile.TemporaryDirectory() as directory:
   output=Path(directory)/'output'
   with patch.dict(os.environ,{'GITHUB_OUTPUT':str(output)},clear=True),patch.object(gate,'main',side_effect=ValueError('Source is no longer an ancestor\nof current main '+'x'*400)),self.assertRaises(SystemExit):gate.run()
   lines=[line for line in output.read_text().splitlines() if line.startswith('detail=')]
   self.assertEqual(len(lines),1)
   self.assertTrue(lines[0].startswith('detail=Source is no longer an ancestor of current main'))
   self.assertLessEqual(len(lines[0]),len('detail=')+300)
 def test_rejected_and_pending_execution_emit_explicit_outcomes(self):
  import tempfile
  for error,status in ((gate.OwnerMergeRequired('owner missing'),'blocked_owner_merge'),(ValueError('source mismatch'),'rejected'),(OSError('unavailable'),'pending')):
   with self.subTest(status=status), tempfile.TemporaryDirectory() as directory:
    output=Path(directory)/'output';summary=Path(directory)/'summary'
    with patch.dict(os.environ,{'GITHUB_OUTPUT':str(output),'GITHUB_STEP_SUMMARY':str(summary)},clear=True),patch.object(gate,'main',side_effect=error),self.assertRaises(SystemExit):gate.run()
    self.assertIn('eligible=false',output.read_text())
    self.assertIn('status='+status,output.read_text())
    self.assertIn(status,summary.read_text())

DOCKERFILE='''# syntax=docker/dockerfile:1
FROM --platform=$BUILDPLATFORM golang:1.27.2-alpine@sha256:@@GO@@ AS compiler
ARG TARGETARCH
RUN CGO_ENABLED=0 go build -trimpath -o /out/tools-bundle ./cmd/tools-bundle

FROM --platform=$BUILDPLATFORM alpine:3.23@sha256:@@DIGEST@@ AS downloads
RUN apk add --no-cache ca-certificates
RUN tools-bundle fetch-sources -tool sqlite3 -arch "${TARGETARCH}" -work /build/work/sqlite3

FROM --platform=$TARGETPLATFORM alpine:3.23@sha256:@@DIGEST@@ AS compile-sqlite3
ARG TARGETARCH
RUN --network=none tools-bundle compile -tool sqlite3 -arch "${TARGETARCH}" -work /work -output /out

FROM downloads AS builder
RUN tools-bundle -arch "${TARGETARCH}" --build-outputs /compiled
'''.replace('@@DIGEST@@','a'*64).replace('@@GO@@','c'*64)
# Every file the approval digest reads plus files it must ignore.
INPUT_FILES={'go.mod':b'module bundledock\n','go.sum':b'sum\n'}
for directory in gate.APPROVAL_INPUT_DIRECTORIES:
 INPUT_FILES[directory+'/main.go']=('package x // '+directory+'/main.go\n').encode()
 INPUT_FILES[directory+'/main_test.go']=b'package x // test\n'
INPUT_FILES['internal/toolconfig/config.go']=b'package toolconfig // config\n'
INPUT_FILES.update({'bundles/toolbox/Dockerfile':DOCKERFILE.encode(),'bundles/toolbox/licenses/sqlite3/NOTICE':b'notice\n','bundles/toolbox/licenses/sqlite3/SOURCE':b'source\n',
                    'bundles/toolbox/scripts/sqlite3-smoke.sh':b'smoke\n','.go-version':b'1.27.2\n'})
def names_in(files):
 return lambda directory:[path[len(directory)+1:] for path in files if path.startswith(directory+'/') and '/' not in path[len(directory)+1:]]
def inputs_of(files,package):
 return gate.approval_inputs(files.__getitem__,names_in(files),gate.config.select_bundle('toolbox'),'sqlite3',package)

class SourceBuildGateTests(unittest.TestCase):
 """Publish-gate content check (design N2): conditions G1-G4 and T1-T4 against the merged commit's parent."""
 setUp=OwnerMergeTests.setUp
 get=OwnerMergeTests.get
 refreshed_updater=OwnerMergeTests.refreshed_updater
 CATALOG={'schema':1,'packages':{'tool':{'provider':'github'},'sqlite3':{'provider':'source-build','version_source':{'kind':'sqlite-product-data','url':'https://www.sqlite.org/download.html'},'asset':'sqlite-amalgamation-{numeric}.zip','base_url':'https://www.sqlite.org','build':{'kind':'c-amalgamation','toolchain':'zig-cc'},'notices':['licenses/sqlite3/NOTICE','licenses/sqlite3/SOURCE']}}}
 def sqlite(self,version='3.54.1',numeric='3540100',year='2026',**changes):
  spec={'version':version,'url':'https://www.sqlite.org/'+year+'/sqlite-amalgamation-'+numeric+'.zip','sha256':'1'*64,'sha3_256':'2'*64,'verification':'publisher-sha3-256','toolchain':'zig-cc'}
  spec.update(changes);return spec
 def approved(self):
  """The owner's approved_digest of the default sqlite3 recipe and pins (the revision that was built and smoked)."""
  package=self.CATALOG['packages']['sqlite3']
  return gate.approval_digest('sqlite3',package,self.lock(self.sqlite(),approvals=False),inputs_of(INPUT_FILES,package))
 def lock(self,sqlite,arm=None,toolchain='0.17.0',extra='a473823',tools=('tool','sqlite3'),approvals=None):
  def platform(spec):
   entries={'tool':{'version':'1.0.0','url':'https://example.test/1.0.0','sha256':'0'*64,'verification':'publisher-sha256'},'sqlite3':spec}
   return {name:entries[name] for name in tools}
  return {'schema':2,'toolchains':{'zig-cc':{'amd64':{'version':toolchain,'sha256':'3'*64},'arm64':{'version':toolchain,'sha256':'4'*64}}},
          'platforms':{'amd64':platform(sqlite),'arm64':platform(arm or sqlite)},
          'extra_sources':{'sqlite3/linenoise':{'commit':extra,'files':{'linenoise.c':{'sha256':'5'*64}}}}} | ({} if approvals is False else {'approvals':{'sqlite3':self.approved() if approvals is None else approvals}})
 def stage(self,before,after,constraint=b'sqlite3>=3.54.0,<4.0.0\ntool>=1.0.0\n'):
  """Parent lock B at the merged commit's parent, merged lock H at A; the PR base is stale and has no data."""
  self.pr['base']['sha']='f'*40
  put=lambda path,revision,content:self.data.__setitem__('/contents/'+path+'?ref='+revision,{'encoding':'base64','content':b64(content)})
  put(self.bundle['lock'],B,before);put(self.bundle['lock'],A,after)
  sources=getattr(self,'sources',None) or INPUT_FILES
  for path,content in sources.items():put(path,A,content)
  for directory in gate.APPROVAL_INPUT_DIRECTORIES:
   self.data['/contents/'+directory+'?ref='+A]=[{'name':name,'type':'file'} for name in names_in(sources)(directory)]
  put(self.bundle['directory']+'/packages.catalog.json',B,self.CATALOG);put(self.bundle['directory']+'/packages.txt',B,constraint)
 def accepted(self):self.assertEqual(gate.validate(self.event,self.get),(A,self.pr['html_url']))
 def bump_accepted(self):
  """A source bump satisfies every content rule (G1-G4, T1-T4), but the owner's approval no longer matches it."""
  gate.content_check(self.get,self.bundle,B,A)
  self.rejected('approved_digest is stale')
 def rejected(self,*needles):
  with self.assertRaises(ValueError) as caught:gate.validate(self.event,self.get)
  for needle in needles:self.assertIn(needle,str(caught.exception))
 def test_source_version_and_checksum_bump_is_accepted(self):
  self.stage(self.lock(self.sqlite()),self.lock(self.sqlite('3.54.2','3540200',sha256='6'*64,sha3_256='7'*64)));self.bump_accepted()
 def test_bump_accepted_under_rebase_merge_with_stale_pull_base(self):
  self.stage(self.lock(self.sqlite()),self.lock(self.sqlite('3.54.2','3540200')));self.assertNotIn('/contents/'+self.bundle['lock']+'?ref='+self.pr['base']['sha'],self.data);self.bump_accepted()
 def test_bump_accepted_under_refresh_merge(self):
  self.refreshed_updater();self.stage(self.lock(self.sqlite()),self.lock(self.sqlite('3.54.2','3540200')));self.bump_accepted()
 def test_merge_commit_with_two_parents_is_rejected(self):
  self.stage(self.lock(self.sqlite()),self.lock(self.sqlite('3.54.2','3540200')));self.data['/commits/'+A]['parents'].append({'sha':H});self.rejected('linear')
 def test_changes_not_touching_source_build_tools_are_accepted(self):
  before=self.lock(self.sqlite());after=self.lock(self.sqlite());after['platforms']['amd64']['tool']['version']='1.0.1';after['platforms']['arm64']['tool']['version']='1.0.1'
  self.stage(before,after);self.accepted()
 def test_lock_without_source_build_tools_is_accepted(self):
  self.accepted()
 def test_g1_changed_toolchain_or_extra_source_is_rejected(self):
  bump=self.sqlite('3.54.2','3540200')
  for kind,after in (('toolchain',self.lock(bump,toolchain='0.18.0')),('extra-commit',self.lock(bump,extra='b'*7))):
   with self.subTest(kind=kind):
    self.setUp();self.stage(self.lock(self.sqlite()),after);self.rejected('(G1)','explicit owner approval')
 def test_g1_changed_extra_source_hash_and_added_section_are_rejected(self):
  bump=self.sqlite('3.54.2','3540200');hashed=self.lock(bump);hashed['extra_sources']['sqlite3/linenoise']['files']['linenoise.c']['sha256']='9'*64
  removed=self.lock(bump);del removed['extra_sources']
  for kind,after in (('hash',hashed),('removed',removed)):
   with self.subTest(kind=kind):
    self.setUp();self.stage(self.lock(self.sqlite()),after);self.rejected('(G1)')
 def test_g2_added_removed_package_and_platform_changes_are_rejected(self):
  bump=self.sqlite('3.54.2','3540200');removed=self.lock(bump,tools=('tool',));platform=self.lock(bump);del platform['platforms']['arm64']
  for kind,before,after in (('removed',self.lock(self.sqlite()),removed),('platform',self.lock(self.sqlite()),platform)):
   with self.subTest(kind=kind):
    self.setUp();self.stage(before,after);self.rejected('(G2)')
 def test_added_non_source_build_package_is_rejected(self):
  before=self.lock(self.sqlite());after=self.lock(self.sqlite())
  for arch in after['platforms']:after['platforms'][arch]['extra']=dict(after['platforms'][arch]['tool'])
  self.stage(before,after);self.rejected('(G2)')
 def test_first_publication_of_source_build_tool_needs_owner_approval(self):
  self.stage(self.lock(self.sqlite(),tools=('tool',)),self.lock(self.sqlite()));self.rejected('first publication','explicit owner approval')
 def test_t1_changes_beyond_the_source_pin_are_rejected(self):
  for field,value in (('verification','publisher-sha256'),('toolchain','other'),('sha512','x')):
   with self.subTest(field=field):
    self.setUp();self.stage(self.lock(self.sqlite()),self.lock(self.sqlite('3.54.2','3540200',**{field:value})));self.rejected('(T1)')
 def test_t2_downgrade_equal_version_and_out_of_constraint_are_rejected(self):
  self.stage(self.lock(self.sqlite()),self.lock(self.sqlite('3.54.0','3540000')));self.rejected('(T2)','did not rise')
  self.setUp();self.stage(self.lock(self.sqlite()),self.lock(self.sqlite(sha256='6'*64)));self.rejected('(T2)','did not rise')
  self.setUp();self.stage(self.lock(self.sqlite()),self.lock(self.sqlite('4.0.0','4000000')));self.rejected('(T2)','outside the desired constraint')
  self.setUp();self.stage(self.lock(self.sqlite()),self.lock(self.sqlite('3.54.2','3540200')),b'sqlite3==3.54.1\n');self.rejected('(T2)','outside the desired constraint')
 def test_t2_missing_constraint_is_rejected(self):
  self.stage(self.lock(self.sqlite()),self.lock(self.sqlite('3.54.2','3540200')),b'tool>=1.0.0\n');self.rejected('(T2)')
 def test_t3_url_not_derived_from_the_version_is_rejected(self):
  for kind,spec in (('numeric',self.sqlite('3.54.2','3540100')),('host',self.sqlite('3.54.2','3540200',url='https://evil.test/2026/sqlite-amalgamation-3540200.zip')),('year',self.sqlite('3.54.2','3540200',year='26')),('suffix',self.sqlite('3.54.2','3540200',url='https://www.sqlite.org/2026/sqlite-amalgamation-3540200.zip.evil'))):
   with self.subTest(kind=kind):
    self.setUp();self.stage(self.lock(self.sqlite()),self.lock(spec));self.rejected('(T3)')
 def test_t3_unknown_resolver_rule_is_rejected(self):
  self.CATALOG=copy.deepcopy(self.CATALOG);self.CATALOG['packages']['sqlite3']['version_source']['kind']='other'
  self.stage(self.lock(self.sqlite()),self.lock(self.sqlite('3.54.2','3540200')));self.rejected('(T3)')
 def test_t4_architecture_disagreement_is_rejected(self):
  bump=self.sqlite('3.54.2','3540200')
  for kind,arm in (('version',self.sqlite('3.54.3','3540300')),('hash',self.sqlite('3.54.2','3540200',sha3_256='8'*64))):
   with self.subTest(kind=kind):
    self.setUp();self.stage(self.lock(self.sqlite()),self.lock(bump,arm));self.rejected('(T4)')
 def test_catalog_source_build_block_change_is_not_a_bump(self):
  # G3: the existing name check forbids catalog and packages.txt changes in an updater PR.
  self.stage(self.lock(self.sqlite()),self.lock(self.sqlite('3.54.2','3540200')))
  self.files[1]['filename']=self.bundle['directory']+'/packages.catalog.json';self.rejected('Only own lock')
 def raw(self,text):return text.encode()
 def test_f1_shadowed_or_duplicate_lock_keys_are_rejected(self):
  good=json.dumps(self.lock(self.sqlite('3.54.2','3540200')))
  hidden=json.dumps(self.lock(self.sqlite('3.54.2','3540200'),toolchain='0.18.0'))
  for kind,text in (('Platforms',good[:-1]+',"Platforms":{}}'),('TOOLCHAINS',good[:-1]+',"TOOLCHAINS":'+json.dumps(json.loads(hidden)['toolchains'])+'}'),
                    ('long-s',good[:-1]+',"toolchain\u017f":{}}'),('duplicate',good[:-1]+',"toolchains":{}}'),('nfkc',good[:-1]+',"\uff54oolchains":{}}'),
                    ('nested-case',good.replace('"sha256"','"SHA256"',1).replace('"version": ','"version": ',1)),
                    ('nested-duplicate',good.replace('"verification": "publisher-sha3-256"','"verification": "publisher-sha3-256", "verification": "x"',1)),
                    ('nested-shadow',good.replace('"sqlite3": {','"sqlite3": {"Version": "9.9.9", ',1)),
                    ('arch-shadow',good[:-1].replace('"platforms": {','"platforms": {"AMD64": {}, ',1)+'}')):
   with self.subTest(kind=kind):
    self.setUp();self.stage(self.lock(self.sqlite()),self.raw(text));self.rejected('Source-build publish gate')
 def test_f1_same_checks_apply_to_the_parent_lock_and_the_catalog(self):
  bump=self.lock(self.sqlite('3.54.2','3540200'))
  self.stage(self.raw(json.dumps(self.lock(self.sqlite()))[:-1]+',"Platforms":{}}'),bump);self.rejected('Source-build publish gate')
  for kind,text in (('provider-case',json.dumps(self.CATALOG).replace('"provider": "source-build"','"Provider": "source-build"')),
                    ('duplicate',json.dumps(self.CATALOG)[:-1]+',"packages":{}}'),('top-shadow',json.dumps(self.CATALOG)[:-1]+',"PACKAGES":{}}'),
                    ('unknown-package-key',json.dumps(self.CATALOG).replace('"provider": "github"','"provider": "github", "Extra": 1',1)),
                    ('package-shadow',json.dumps(self.CATALOG).replace('"tool": {','"tool": {"Provider": "x", ',1))):
   with self.subTest(kind=kind):
    self.setUp();self.stage(self.lock(self.sqlite()),bump);self.data['/contents/'+self.bundle['directory']+'/packages.catalog.json?ref='+B]['content']=b64(self.raw(text));self.rejected('Source-build publish gate')
 def test_f1_unknown_lock_and_spec_keys_are_rejected(self):
  extra=self.lock(self.sqlite('3.54.2','3540200'));extra['hidden']=1
  self.stage(self.lock(self.sqlite()),extra);self.rejected('unknown key')
 def test_f1_shadowing_inside_free_form_sections_is_rejected(self):
  bump=self.lock(self.sqlite('3.54.2','3540200'))
  for kind,old,new in (('nfkc','"commit": ','"commit": "a", "\uff43ommit": '),('case','"commit": ','"commit": "a", "COMMIT": '),('ligature','"files": ','"files": {}, "\ufb01les": ')):
   with self.subTest(kind=kind):
    self.setUp();text=json.dumps(bump).replace(old,new,1);self.stage(self.lock(self.sqlite()),self.raw(text));self.rejected('shadowed')
 def test_f1_unknown_spec_key_is_rejected(self):
  after=self.lock(self.sqlite('3.54.2','3540200'))
  for arch in after['platforms']:after['platforms'][arch]['sqlite3']['bogus']=1
  self.stage(self.lock(self.sqlite()),after);self.rejected('unknown key')
 def test_t4_single_architecture_lock_is_rejected(self):
  def single(spec):
   lock=self.lock(spec);del lock['platforms']['arm64'];return lock
  self.stage(single(self.sqlite()),single(self.sqlite('3.54.2','3540200')));self.rejected('(T4)')
 def test_f2_change_on_one_architecture_only_is_rejected(self):
  after=self.lock(self.sqlite());after['platforms']['amd64']['sqlite3']=self.sqlite('3.54.2','3540200')
  self.stage(self.lock(self.sqlite()),after);self.rejected('(T4)')
 def test_f2_base_url_is_matched_literally(self):
  self.stage(self.lock(self.sqlite()),self.lock(self.sqlite('3.54.2','3540200',url='https://wwwXsqlite.org/2026/sqlite-amalgamation-3540200.zip')));self.rejected('(T3)')
 def test_f2_other_providers_are_not_source_builds(self):
  self.CATALOG=copy.deepcopy(self.CATALOG);self.CATALOG['packages']['tool']['provider']='gitlab'
  before=self.lock(self.sqlite());after=self.lock(self.sqlite())
  for arch in after['platforms']:after['platforms'][arch]['tool']['version']='1.0.1'
  self.stage(before,after);self.accepted()
 def test_f3_packages_txt_grammar_is_the_go_grammar(self):
  bump=(self.lock(self.sqlite()),self.lock(self.sqlite('3.54.2','3540200')))
  for text in (b'sqlite3 >= 3.54.0\n',b'sqlite3\t>=\t3.54.0 , < 4.0.0 # note\n',b'# c\n\ntool>=1.0.0\nsqlite3==3.54.2\n',b'sqlite3>=3.54.0,<3.54.3\n'):
   with self.subTest(text=text):
    self.setUp();self.stage(*bump,text);self.bump_accepted()
  for text in (b'sqlite3<=3.54.2\n',b'sqlite3>3.54.0\n',b'sqlite3>=3.54.0,<=4.0.0\n',b'sqlite3>=3.54.0\nsqlite3>=3.54.0\n',b'sqlite3==3.54.2,<4.0.0\n',b'sqlite3>=3.54\n',b'sqlite3>=3.54.0,<4.0.0,<5.0.0\n',b'tool>=1.0.0\nsqlite3!=3.54.1\n'):
   with self.subTest(text=text):
    self.setUp();self.stage(*bump,text);self.rejected('(T2)')
 def test_unparsable_lock_fails_closed(self):
  self.stage(self.lock(self.sqlite()),b'not json');self.rejected('not valid JSON')

class ApprovalGateTests(unittest.TestCase):
 """The owner's approved_digest binds the approval of a source build to its exact recipe, pins and toolchain."""
 setUp=OwnerMergeTests.setUp
 get=OwnerMergeTests.get
 CATALOG=SourceBuildGateTests.CATALOG
 sqlite=SourceBuildGateTests.sqlite
 approved=SourceBuildGateTests.approved
 lock=SourceBuildGateTests.lock
 stage=SourceBuildGateTests.stage
 accepted=SourceBuildGateTests.accepted
 rejected=SourceBuildGateTests.rejected
 raw=SourceBuildGateTests.raw
 def check(self,lock,catalog,files=None):
  files=files or INPUT_FILES
  gate.check_approvals(lock,catalog,gate.config.select_bundle('toolbox'),files.__getitem__,names_in(files))
 def digest(self,lock,files=None,package=None):
  package=package or self.CATALOG['packages']['sqlite3']
  return gate.approval_digest('sqlite3',package,lock,inputs_of(files or INPUT_FILES,package))
 def test_reviewer_scenario_unrelated_updater_cannot_publish_an_unapproved_source_build(self):
  # The lock of the catalog change ships without approved_digest; the next updater only bumps another tool.
  before=self.lock(self.sqlite(),approvals=False);after=self.lock(self.sqlite(),approvals=False)
  for arch in after['platforms']:after['platforms'][arch]['tool']['version']='1.0.1'
  self.stage(before,after);self.rejected('sqlite3 has no owner approval','approved_digest')
 def test_matching_approval_lets_an_unrelated_updater_publish(self):
  before=self.lock(self.sqlite());after=self.lock(self.sqlite())
  for arch in after['platforms']:after['platforms'][arch]['tool']['version']='1.0.1'
  self.stage(before,after);self.accepted()
 def test_a_stale_approval_after_a_source_change_is_rejected_for_every_change_kind(self):
  base=self.lock(self.sqlite())
  def changed(kind):
   after=self.lock(self.sqlite())
   if kind=='sha256':
    for arch in after['platforms']:after['platforms'][arch]['sqlite3']['sha256']='6'*64
   elif kind=='toolchain':after['toolchains']['zig-cc']['amd64']['sha256']='9'*64
   elif kind=='extra-source hash':after['extra_sources']['sqlite3/linenoise']['files']['linenoise.c']['sha256']='8'*64
   return after
  # Changes the updater could never make are already refused by G1/T1; the approval check is
  # the second, independent wall, so test it directly on the merged lock.
  for kind in ('sha256','toolchain','extra-source hash'):
   with self.subTest(kind=kind):
    with self.assertRaises(ValueError) as caught:self.check(changed(kind),self.CATALOG)
    self.assertIn('stale',str(caught.exception))
  recipe=copy.deepcopy(self.CATALOG);recipe['packages']['sqlite3']['build']['kind']='other'
  with self.assertRaises(ValueError):self.check(base,recipe)
  self.check(base,self.CATALOG)
 def test_digest_ignores_other_tools_and_the_approvals_themselves(self):
  base=self.lock(self.sqlite());other=self.lock(self.sqlite(),approvals='f'*64)
  other['platforms']['amd64']['tool']['version']='9.9.9'
  digest=self.digest
  self.assertEqual(digest(base),digest(other))
  bumped=self.lock(self.sqlite('3.54.2','3540200'))
  self.assertNotEqual(digest(base),digest(bumped))
 def test_the_updater_bot_cannot_write_or_change_an_approval(self):
  for kind,before,after in (('set',self.lock(self.sqlite(),approvals=False),self.lock(self.sqlite())),
                            ('changed',self.lock(self.sqlite()),self.lock(self.sqlite(),approvals='e'*64)),
                            ('removed',self.lock(self.sqlite()),self.lock(self.sqlite(),approvals=False))):
   with self.subTest(kind=kind):
    self.setUp();self.stage(before,after);self.rejected('approvals changed (G1)')
 def test_approval_for_a_tool_that_is_not_a_source_build_is_rejected(self):
  lock=self.lock(self.sqlite());lock['approvals']['tool']='1'*64
  with self.assertRaises(ValueError) as caught:self.check(lock,self.CATALOG)
  self.assertIn('not a source build',str(caught.exception))
 def test_malformed_or_shadowed_approvals_are_rejected(self):
  good=json.dumps(self.lock(self.sqlite()))
  for kind,text in (('not hex',good.replace(self.approved(),'z'*64)),('upper',good.replace(self.approved(),self.approved().upper())),
                    ('not an object',good.replace('"approvals": {','"approvals": [], "x": {',1)),
                    ('duplicate key',good[:-1]+',"approvals":{}}'),('shadowed key',good[:-1]+',"Approvals":{}}'),
                    ('nested duplicate',good.replace('"approvals": {','"approvals": {"sqlite3": "'+'1'*64+'", ',1))):
   with self.subTest(kind=kind):
    self.setUp();self.stage(self.lock(self.sqlite()),self.raw(text));self.rejected('Source-build publish gate')
 def test_inputs_are_a_closed_documented_list(self):
  self.assertEqual(gate.APPROVAL_INPUT_FILES,('go.mod','go.sum'))
  self.assertEqual(gate.APPROVAL_INPUT_DIRECTORIES,('cmd/tools-bundle','cmd/tools-init','internal/toolbundle','internal/toolconfig','internal/toolvolume'))
  inputs=inputs_of(INPUT_FILES,self.CATALOG['packages']['sqlite3'])
  self.assertEqual(set(inputs),{'go.mod','go.sum','dockerfile compiler stage','dockerfile compile stage','dockerfile fetch instruction','notice licenses/sqlite3/NOTICE','notice licenses/sqlite3/SOURCE'}
                   |{d+'/main.go' for d in gate.APPROVAL_INPUT_DIRECTORIES}|{'internal/toolconfig/config.go'})
  self.assertNotIn('.go-version',inputs)
 def test_docs_name_exactly_the_listed_inputs(self):
  import re
  docs=(Path(__file__).parents[1]/'docs/publication.md').read_text()
  region=docs[docs.index('<!-- BEGIN APPROVAL INPUTS -->'):docs.index('<!-- END APPROVAL INPUTS -->')]
  documented=set(re.findall(r'`([^`]+)`',region))
  listed=set(gate.APPROVAL_INPUT_FILES)|{d+'/*.go' for d in gate.APPROVAL_INPUT_DIRECTORIES}
  self.assertEqual({item for item in documented if re.fullmatch(r'go\.mod|go\.sum|[a-z0-9/_-]+/\*\.go',item)},listed)
 def mutated(self,path,old,new):
  files=dict(INPUT_FILES);files[path]=files[path].replace(old,new);self.assertNotEqual(files[path],INPUT_FILES[path]);return files
 def unrelated_updater(self,files):
  """The reviewer's shape: an updater PR that changes only another tool, on a lock the owner approved."""
  before=self.lock(self.sqlite());after=self.lock(self.sqlite())
  for arch in after['platforms']:after['platforms'][arch]['tool']['version']='1.0.1'
  self.sources=files;self.stage(before,after)
 def test_every_listed_input_changes_the_digest_and_makes_the_approval_stale(self):
  docker='bundles/toolbox/Dockerfile'
  text=INPUT_FILES[docker].decode()
  compile_from=[line for line in text.split('\n') if line.endswith('AS compile-sqlite3')][0].encode()
  cases=[('go.mod',b'module',b'module x'),('go.sum',b'sum',b'sum changed'),
         ('internal/toolconfig/config.go',b'config',b'config changed')]  # the reviewer's examples on real data
  cases+=[(d+'/main.go',b'package',b'package changed') for d in gate.APPROVAL_INPUT_DIRECTORIES]
  cases+=[(docker,b'golang:1.27.2-alpine@sha256:'+b'c'*64,b'golang:1.27.2-alpine@sha256:'+b'd'*64),  # another golang image digest
          (docker,b'go build -trimpath',b'go build'),                                           # -trimpath removed
          (docker,b'CGO_ENABLED=0',b'CGO_ENABLED=1'),
          (docker,b'--network=none tools-bundle compile',b'--network=none tools-bundle compile -x'),
          (docker,compile_from,compile_from.replace(b'a'*64,b'b'*64)),                          # the compile stage's base image digest only
          (docker,b'fetch-sources -tool sqlite3 -arch',b'fetch-sources -tool sqlite3 -x -arch'),
          ('bundles/toolbox/licenses/sqlite3/NOTICE',b'notice',b'notice changed'),('bundles/toolbox/licenses/sqlite3/SOURCE',b'source',b'source changed')]
  for path,old,new in cases:
   with self.subTest(path=path,change=new[-24:]):
    self.setUp();files=self.mutated(path,old,new)
    self.assertNotEqual(self.digest(self.lock(self.sqlite()),files),self.approved())
    self.unrelated_updater(files);self.rejected('approved_digest is stale')
 def test_a_new_go_source_in_a_listed_directory_is_covered_but_a_new_test_is_not(self):
  added=dict(INPUT_FILES);added['internal/toolbundle/extra.go']=b'package x\n'
  self.assertNotEqual(self.digest(self.lock(self.sqlite()),added),self.approved())
  tested=dict(INPUT_FILES);tested['internal/toolbundle/extra_test.go']=b'package x\n'
  self.assertEqual(self.digest(self.lock(self.sqlite()),tested),self.approved())
  emptied={path:content for path,content in INPUT_FILES.items() if not path.startswith('internal/toolvolume/')}
  with self.assertRaises(ValueError):self.digest(self.lock(self.sqlite()),emptied)
 def test_files_the_artifact_does_not_depend_on_leave_the_approval_valid(self):
  docker='bundles/toolbox/Dockerfile'
  for path,old,new in (('bundles/toolbox/scripts/sqlite3-smoke.sh',b'smoke',b'smoke changed'),('internal/toolbundle/main_test.go',b'test',b'test changed'),('.go-version',b'1.27.2',b'1.28.0'),
                       (docker,b'# syntax=docker/dockerfile:1',b'# syntax=docker/dockerfile:1.99'),
                       (docker,b'RUN apk add --no-cache ca-certificates',b'RUN apk add --no-cache ca-certificates curl'),(docker,b'tools-bundle -arch "${TARGETARCH}" --build-outputs',b'tools-bundle -arch "${TARGETARCH}" -x --build-outputs'),
                       (docker,b'AS downloads',b'AS downloads')):
   if path==docker and old==b'AS downloads':old,new=b'alpine:3.23@sha256:'+b'a'*64+b' AS downloads',b'alpine:3.23@sha256:'+b'e'*64+b' AS downloads'
   with self.subTest(path=path,change=new[-24:]):
    self.setUp();files=self.mutated(path,old,new)
    self.assertEqual(self.digest(self.lock(self.sqlite()),files),self.approved())
    self.unrelated_updater(files);self.accepted()
 def test_dockerfile_parsing_is_case_insensitive_and_reads_continuation_lines(self):
  docker='bundles/toolbox/Dockerfile'
  lower=self.mutated(docker,b'FROM --platform=$TARGETPLATFORM',b'from --platform=$TARGETPLATFORM')
  lower=dict(lower);lower[docker]=lower[docker].replace(b'AS compile-sqlite3',b'as compile-sqlite3')
  self.assertEqual(self.digest(self.lock(self.sqlite()),lower),self.digest(self.lock(self.sqlite()),lower))
  lowered=gate.dockerfile_inputs(lower[docker].decode(),'sqlite3')
  self.assertIn('from --platform',lowered[1])
  split=self.mutated(docker,b'fetch-sources -tool sqlite3 -arch "${TARGETARCH}" -work /build/work/sqlite3',b'fetch-sources -tool sqlite3 \\\n    -arch "${TARGETARCH}" -work /build/work/sqlite3')
  base=self.digest(self.lock(self.sqlite()),split)
  changed=dict(split);changed[docker]=split[docker].replace(b'-work /build/work/sqlite3',b'-work /elsewhere')
  self.assertNotEqual(self.digest(self.lock(self.sqlite()),changed),base)
 def test_missing_or_ambiguous_dockerfile_stage_is_rejected(self):
  docker='bundles/toolbox/Dockerfile'
  for kind,old,new in (('no compile stage',b'AS compile-sqlite3',b'AS compile-other'),('no compiler stage',b'AS compiler',b'AS builder-go'),('no fetch line',b'RUN tools-bundle fetch-sources -tool sqlite3',b'RUN true sqlite3'),
                       ('two fetch lines',b'RUN apk add --no-cache ca-certificates',b'RUN tools-bundle fetch-sources -tool sqlite3 -arch x')):
   with self.subTest(kind=kind):
    self.setUp();files=self.mutated(docker,old,new)
    with self.assertRaises(ValueError):self.digest(self.lock(self.sqlite()),files)
 def test_digest_covers_both_architectures_and_the_publisher_sha3(self):
  base=self.digest(self.lock(self.sqlite()))
  only_arm64=self.lock(self.sqlite(),arm=self.sqlite(sha256='9'*64))
  self.assertNotEqual(self.digest(only_arm64),base)
  only_amd64=self.lock(self.sqlite(sha256='9'*64),arm=self.sqlite())
  self.assertNotEqual(self.digest(only_amd64),base)
  self.assertNotEqual(self.digest(self.lock(self.sqlite(sha3_256='8'*64))),base)
 def test_manual_publication_needs_the_approval_too(self):
  put=lambda path,content:self.data.__setitem__('/contents/'+path+'?ref='+A,{'encoding':'base64','content':b64(content)})
  put(self.bundle['directory']+'/packages.catalog.json',self.CATALOG)
  put(self.bundle['lock'],self.lock(self.sqlite(),approvals=False))
  for path,content in INPUT_FILES.items():put(path,content)
  for directory in gate.APPROVAL_INPUT_DIRECTORIES:self.data['/contents/'+directory+'?ref='+A]=[{'name':name,'type':'file'} for name in names_in(INPUT_FILES)(directory)]
  with self.assertRaises(ValueError) as caught:gate.require_approvals(self.get,self.bundle,A)
  self.assertIn('no owner approval',str(caught.exception))
  put(self.bundle['lock'],self.lock(self.sqlite()))
  gate.require_approvals(self.get,self.bundle,A)
 def test_manual_dispatch_plan_calls_the_approval_check(self):
  import tempfile
  with tempfile.TemporaryDirectory() as directory:
   event=Path(directory)/'event';event.write_text(json.dumps({'repository':REPO}));output=Path(directory)/'output'
   env={'GITHUB_REPOSITORY':REPO['full_name'],'GITHUB_EVENT_NAME':'workflow_dispatch','GITHUB_EVENT_PATH':str(event),'GITHUB_TOKEN':'fixture','GITHUB_OUTPUT':str(output),'BUNDLE':'node','APPROVED_SHA':A,'GITHUB_SHA':A,'DEFAULT_BRANCH':'main','GITHUB_REF':'refs/heads/main','SENDER_TYPE':'User','GITHUB_ACTOR':'wstein','TRIGGERING_ACTOR':'wstein'}
   with patch.dict(os.environ,env,clear=True),patch.object(sys,'argv',['gate','--mode','plan']),patch.object(gate,'GitHub') as api,patch.object(gate,'validate_caller'),patch.object(gate.subprocess,'check_output',return_value=A),patch.object(gate,'require_approvals',side_effect=ValueError('no owner approval')) as check:
    api.return_value.request.side_effect=self.get
    with self.assertRaises(SystemExit):gate.run()
    check.assert_called_once()
    self.assertEqual(check.call_args.args[2],A)
    self.assertNotIn('eligible=true',output.read_text())

class CombinedUpdaterTests(unittest.TestCase):
 """One owner merge of automation/upgrades-<run>-<attempt> plans every bundle it changed."""
 setUp=OwnerMergeTests.setUp
 get=OwnerMergeTests.get
 def combine(self,names=('agy','node')):
  self.pr['head']['ref']='automation/upgrades-123-1'
  self.files[:]=[]
  for name in names:
   bundle=gate.config.select_bundle(name)
   self.files.extend([{'filename':bundle['lock'],'status':'modified','changes':2},{'filename':bundle['inventory'],'status':'modified','changes':2}])
   for revision,content in ((A,NEW),(B,OLD)):self.data['/contents/'+bundle['lock']+'?ref='+revision]={'encoding':'base64','content':content}
   stage_fixtures(self.data,bundle)
  self.pr['changed_files']=len(self.files)
 def test_plan_lists_every_changed_bundle_in_canonical_order(self):
  self.combine(('agy','node','toolbox'))
  self.assertEqual(gate.validate(self.event,self.get,None),(['toolbox','node','agy'],A,self.pr['html_url']))
 def test_each_bundle_leg_authorizes_only_members(self):
  self.combine()
  for name in ('agy','node'):self.assertEqual(gate.validate(self.event,self.get,name),(A,self.pr['html_url']))
  with self.assertRaises(ValueError):gate.validate(self.event,self.get,'claude')
  self.assertIsNone(gate.validate(self.event,self.get,'claude',allow_ineligible=True))
 def test_plan_command_emits_json_bundle_list(self):
  import tempfile
  import json
  self.combine()
  with tempfile.TemporaryDirectory() as directory:
   event=Path(directory)/'event';event.write_text(json.dumps(self.event));output=Path(directory)/'output'
   env={'GITHUB_REPOSITORY':REPO['full_name'],'GITHUB_EVENT_NAME':'workflow_run','GITHUB_EVENT_PATH':str(event),'GITHUB_TOKEN':'fixture','GITHUB_OUTPUT':str(output)}
   with patch.dict(os.environ,env,clear=True),patch.object(sys,'argv',['gate','--mode','plan']),patch.object(gate,'GitHub') as api,patch.object(gate,'validate_caller'),patch.object(gate.subprocess,'check_output',return_value=A):
    api.return_value.request.side_effect=self.get
    gate.main()
   self.assertIn('bundles=["node", "agy"]',output.read_text())
 def test_combined_scope_lock_and_branch_guards(self):
  for kind in ('outside-file','unchanged-lock','bad-branch','no-lock','count','single-topic','workflow','renamed'):
   with self.subTest(kind=kind):
    self.setUp();self.combine()
    if kind=='outside-file':
     self.files.append({'filename':gate.config.select_bundle('claude')['inventory'],'status':'modified','changes':2});self.pr['changed_files']=5
    if kind=='unchanged-lock':self.data['/contents/'+gate.config.select_bundle('agy')['lock']+'?ref='+A]['content']=OLD
    if kind=='bad-branch':self.pr['head']['ref']='automation/upgrades-tampered'
    if kind=='no-lock':
     self.files[:]=[{'filename':gate.config.select_bundle('agy')['inventory'],'status':'modified','changes':2}];self.pr['changed_files']=1
    if kind=='count':self.pr['changed_files']=2
    if kind=='single-topic':self.pr['head']['ref']='automation/agy-upgrade-123-1'
    if kind=='workflow':self.files[1]['filename']='.github/workflows/ci.yml'
    if kind=='renamed':self.files[0]['status']='renamed'
    for name in (None,'agy'):
     with self.assertRaises(ValueError):gate.validate(self.event,self.get,name)
 def test_malformed_combined_topic_is_rejected_not_ignored(self):
  self.combine()
  for branch in ('automation/upgrades-tampered','automation/upgrades-1-1-1','automation/upgrades-0-1'):
   self.pr['head']['ref']=branch
   with self.assertRaises(ValueError):gate.validate(self.event,self.get,None,allow_ineligible=True)
 def test_main_advance_checks_own_lock_per_leg(self):
  self.combine()
  newer='e'*40
  self.data['/git/ref/heads/main']['object']['sha']=newer
  self.data['/compare/'+A+'...'+newer]={'status':'ahead','merge_base_commit':{'sha':A}}
  self.data['/contents/'+gate.config.select_bundle('agy')['lock']+'?ref='+newer]={'encoding':'base64','content':NEW}
  self.data['/contents/'+gate.config.select_bundle('node')['lock']+'?ref='+newer]={'encoding':'base64','content':CHANGED}
  self.assertEqual(gate.validate(self.event,self.get,'agy'),(A,self.pr['html_url']))
  with self.assertRaises(ValueError):gate.validate(self.event,self.get,'node')
  with self.assertRaises(ValueError):gate.validate(self.event,self.get,None)

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

class ApprovalDigestToolTests(unittest.TestCase):
 def test_owner_tool_prints_the_digest_the_gate_checks(self):
  import subprocess,tempfile
  fixture=SourceBuildGateTests();bundle=gate.config.select_bundle('toolbox')
  lock=fixture.lock(fixture.sqlite(),approvals=False)
  with tempfile.TemporaryDirectory() as directory:
   root=Path(directory)
   for path,content in ((bundle['lock'],lock),(bundle['directory']+'/packages.catalog.json',fixture.CATALOG)):
    (root/path).parent.mkdir(parents=True,exist_ok=True);(root/path).write_text(json.dumps(content))
   for path,content in INPUT_FILES.items():
    (root/path).parent.mkdir(parents=True,exist_ok=True);(root/path).write_bytes(content)
   script=str(Path(__file__).with_name('approval-digest.py'))
   run=lambda *args:subprocess.run([sys.executable,'-B',script,'--root',str(root),*args],capture_output=True,text=True)
   result=run('sqlite3')
   self.assertEqual((result.returncode,result.stdout.strip()),(0,fixture.approved()))
   for tool in ('tool','nonesuch'):
    self.assertEqual(run(tool).returncode,1)

if __name__=='__main__':unittest.main()
