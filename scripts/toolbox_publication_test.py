import copy
import sys
sys.dont_write_bytecode = True
import importlib.util
import unittest
from pathlib import Path

spec = importlib.util.spec_from_file_location('gate', Path(__file__).with_name('toolbox-publication.py'))
gate = importlib.util.module_from_spec(spec)
spec.loader.exec_module(gate)
SHA = 'a' * 40
BASE = 'b' * 40
REPO = {'full_name': 'wstein/bundledock', 'default_branch': 'main'}


class ProvenanceTests(unittest.TestCase):
    def setUp(self):
        self.event = {'repository': REPO, 'workflow_run': {'id': 123, 'head_sha': SHA}}
        self.responses = {
            '': REPO,
            '/commits/' + SHA: {'parents': [{'sha': BASE}], 'files': [{'filename': 'bundles/toolbox/tools.lock.json', 'status': 'modified', 'changes': 2}, {'filename': 'docs/tools.md', 'status': 'modified', 'changes': 2}]},
            '/git/ref/heads/main': {'object': {'sha': SHA}},
            '/actions/workflows/ci.yml': {'id': 7, 'path': '.github/workflows/ci.yml'},
            '/actions/runs/123': {'id': 123, 'workflow_id': 7, 'path': '.github/workflows/ci.yml', 'event': 'push', 'status': 'completed', 'conclusion': 'success', 'head_sha': SHA, 'head_branch': 'main', 'repository': REPO, 'head_repository': REPO},
            '/commits/' + SHA + '/pulls?per_page=100&page=1': [{'number': 42}],
            '/pulls/42': {'number': 42, 'merged': True, 'merge_commit_sha': SHA, 'base': {'ref': 'main', 'repo': REPO, 'sha': BASE}, 'head': {'ref': 'automation/toolbox-upgrade-123-1', 'repo': REPO}, 'user': {'login': 'github-actions[bot]', 'type': 'Bot'}, 'changed_files': 2, 'html_url': 'https://github.com/wstein/bundledock/pull/42'},
            '/pulls/42/files?per_page=100&page=1': [{'filename': 'bundles/toolbox/tools.lock.json', 'status': 'modified', 'changes': 2}, {'filename': 'docs/tools.md', 'status': 'modified', 'changes': 2}],
            '/contents/bundles/toolbox/tools.lock.json?ref=' + SHA: {'encoding': 'base64', 'content': 'bmV3'},
            '/contents/bundles/toolbox/tools.lock.json?ref=' + BASE: {'encoding': 'base64', 'content': 'b2xk'},
            '/environments/toolbox-publication': {'can_admins_bypass': False, 'protection_rules': [{'type': 'required_reviewers', 'prevent_self_review': False, 'reviewers': [{'type': 'User', 'reviewer': {'login': 'wstein'}}]}], 'deployment_branch_policy': {'protected_branches': False, 'custom_branch_policies': True}},
            '/environments/toolbox-publication/deployment-branch-policies?per_page=100&page=1': {'total_count': 1, 'branch_policies': [{'name': 'main', 'type': 'branch'}]},
        }

    def check(self):
        return gate.validate(self.event, lambda path: copy.deepcopy(self.responses[path]))

    def test_exact_merged_main(self):
        self.assertEqual(self.check(), (SHA, 'https://github.com/wstein/bundledock/pull/42'))

    def test_rejections(self):
        mutations = {
            'stale before approval or promotion': lambda: self.responses['/git/ref/heads/main']['object'].update(sha=BASE),
            'failed CI': lambda: self.responses['/actions/runs/123'].update(conclusion='failure'),
            'PR CI': lambda: self.responses['/actions/runs/123'].update(event='pull_request'),
            'wrong repository': lambda: self.responses['/actions/runs/123'].update(head_repository={'full_name': 'attacker/bundledock'}),
            'unmerged': lambda: self.responses['/pulls/42'].update(merged=False),
            'wrong merged SHA': lambda: self.responses['/pulls/42'].update(merge_commit_sha=BASE),
            'nonbot': lambda: self.responses['/pulls/42']['user'].update(login='wstein', type='User'),
            'nonupdater': lambda: self.responses['/pulls/42']['head'].update(ref='topic'),
            'unexpected file': lambda: self.responses['/pulls/42/files?per_page=100&page=1'][1].update(filename='scripts/unsafe.sh'),
            'renamed lock': lambda: self.responses['/pulls/42/files?per_page=100&page=1'][0].update(status='renamed'),
            'deleted lock': lambda: self.responses['/pulls/42/files?per_page=100&page=1'][0].update(status='removed'),
            'unexpected merged file': lambda: self.responses['/commits/' + SHA]['files'][1].update(filename='scripts/unsafe.sh'),
            'truncated files': lambda: self.responses['/pulls/42'].update(changed_files=3),
            'no lock changes': lambda: self.responses['/contents/bundles/toolbox/tools.lock.json?ref=' + BASE].update(content='bmV3'),
            'missing environment': lambda: self.responses.pop('/environments/toolbox-publication'),
            'unprotected environment': lambda: self.responses['/environments/toolbox-publication'].update(protection_rules=[]),
            'admin bypass': lambda: self.responses['/environments/toolbox-publication'].update(can_admins_bypass=True),
            'additional reviewer': lambda: self.responses['/environments/toolbox-publication']['protection_rules'][0]['reviewers'].append({'type': 'User', 'reviewer': {'login': 'someone'}}),
            'team reviewer': lambda: self.responses['/environments/toolbox-publication']['protection_rules'][0]['reviewers'][0].update(type='Team'),
            'wrong branch restriction': lambda: self.responses['/environments/toolbox-publication/deployment-branch-policies?per_page=100&page=1']['branch_policies'][0].update(name='*'),
        }
        original = copy.deepcopy(self.responses)
        for name, mutate in mutations.items():
            with self.subTest(name=name):
                self.responses = copy.deepcopy(original)
                mutate()
                with self.assertRaises((ValueError, KeyError)):
                    self.check()

    def test_recheck_rejects_changed_candidate(self):
        self.assertEqual(self.check()[0], SHA)
        self.event['workflow_run']['head_sha'] = BASE
        with self.assertRaises(ValueError):
            self.check()



# Evaluate the actual job conditions against GitHub result/event combinations.
# This keeps always() from admitting a cancelled/skipped automatic approval.
class WorkflowDependencyTests(unittest.TestCase):
    def test_job_result_matrix(self):
        import re
        from types import SimpleNamespace as NS
        workflow = (Path(__file__).parents[1] / '.github/workflows/bundledock-toolbox.yml').read_text()
        for name in ('resolve', 'native', 'promote'):
            body = re.search(r'^  ' + name + r':\n(.*?)(?=^  [a-z]+:|\Z)', workflow, re.M | re.S).group(1)
            expression = re.search(r'^    if: >-\n((?:      .*\n)+)', body, re.M).group(1)
            expression = ' '.join(expression.split()).replace('&&', ' and ').replace('||', ' or ')
            for event in ('workflow_run', 'workflow_dispatch', 'push', 'pull_request', 'unknown'):
                for preflight in ('success', 'failure', 'cancelled', 'skipped'):
                    for approval in ('success', 'failure', 'cancelled', 'skipped'):
                        github = NS(event_name=event, event=NS(repository=NS(default_branch='main'), sender=NS(type='User')), ref='refs/heads/main', sha=SHA)
                        needs = NS(preflight=NS(result=preflight), approval=NS(result=approval), resolve=NS(result='success', outputs=NS(publish='true')), native=NS(result='success'))
                        result = eval(expression, {'__builtins__': {}}, {'github': github, 'needs': needs, 'inputs': NS(approved_sha=SHA), 'always': lambda: True})
                        expected = preflight == 'success' and (event == 'workflow_dispatch' or (event == 'workflow_run' and approval == 'success'))
                        self.assertEqual(result, expected, (name, event, preflight, approval))
            github.event_name = 'workflow_dispatch'
            needs.preflight.result = 'success'
            github.sha = BASE
            self.assertFalse(eval(expression, {'__builtins__': {}}, {'github': github, 'needs': needs, 'inputs': NS(approved_sha=SHA), 'always': lambda: True}))

if __name__ == '__main__':
    unittest.main()
