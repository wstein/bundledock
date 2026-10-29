"""Exercise the guarded CI dispatcher against a credential-free forge fake."""
import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[2]
BASE, HEAD = 'a' * 40, 'b' * 40
REPO = 'wstein/bundledock'
BRANCH = 'automation/agy-upgrade-123-1'
URL = 'https://github.com/' + REPO + '/pull/42'


def evidence(bundles=('agy',), branch=BRANCH):
    files = [{'filename': path, 'status': 'modified'} for name in bundles for path in
             ('bundles/' + name + '/packages.lock.json', 'docs/' + name + '.md')]
    return {
        '': {'full_name': REPO, 'default_branch': 'main'},
        '/pulls/42': {'number': 42, 'state': 'open', 'draft': False, 'merged': False,
                      'user': {'login': 'github-actions[bot]', 'type': 'Bot'},
                      'head': {'sha': HEAD, 'ref': branch, 'repo': {'full_name': REPO}},
                      'base': {'sha': BASE, 'ref': 'main', 'repo': {'full_name': REPO}},
                      'commits': 1, 'changed_files': len(files), 'html_url': URL},
        '/git/ref/heads/main': {'object': {'sha': BASE}},
        '/git/ref/heads/' + branch: {'object': {'sha': HEAD}},
        '/pulls/42/files?per_page=100&page=1': files,
        '/compare/' + BASE + '...' + HEAD: {
            'status': 'ahead', 'ahead_by': 1, 'behind_by': 0, 'total_commits': 1,
            'commits': [{'sha': HEAD, 'parents': [{'sha': BASE, 'url': 'https://api.github.com/repos/' + REPO + '/commits/' + BASE, 'html_url': 'https://github.com/' + REPO + '/commit/' + BASE}]}], 'files': files},
        '/actions/workflows/ci.yml': {'id': 7, 'path': '.github/workflows/ci.yml', 'state': 'active'},
        '/actions/workflows/ci.yml/runs?branch=' + branch + '&event=workflow_dispatch&per_page=100&page=1':
            {'total_count': 0, 'workflow_runs': []},
    }


class UpdaterSourceCITests(unittest.TestCase):
    def run_dispatch(self, data=None, env_changes=None, arguments=None, bundles='agy', branch=BRANCH):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / 'evidence.json').write_text(json.dumps(data or evidence()))
            fake = root / 'gh'
            fake.write_text('''#!/usr/bin/env python3
import json, os, pathlib, sys
root = pathlib.Path(os.environ['FIXTURE'])
args = sys.argv[1:]
with (root / 'calls.jsonl').open('a') as output:
    payload = sys.stdin.read() if '--input' in args else ''
    output.write(json.dumps({'args': args, 'payload': payload}) + '\\n')
endpoint = args[1].removeprefix('repos/wstein/bundledock')
if '--method' in args:
    if os.environ.get('FAIL_DISPATCH'): sys.exit(1)
else:
    data = json.loads((root / 'evidence.json').read_text())
    value = data[endpoint]
    if os.environ.get('MOVE_HEAD') and endpoint.startswith('/git/ref/heads/automation/'):
        value = {'object': {'sha': 'c' * 40}}
    print(json.dumps(value))
''')
            fake.chmod(0o755)
            env = dict(os.environ, PATH=str(root) + os.pathsep + os.environ['PATH'],
                       FIXTURE=str(root), GITHUB_REPOSITORY=REPO, DEFAULT_BRANCH='main',
                       GITHUB_REF='refs/heads/main', GITHUB_SHA=BASE, GITHUB_RUN_ID='123',
                       GITHUB_RUN_ATTEMPT='1')
            env.update(env_changes or {})
            command = ['python3', '-B', str(ROOT / 'scripts/updater-source-ci.py'),
                       '--bundles', bundles, '--branch', branch, '--base', BASE,
                       '--head', HEAD, '--pr-url', URL]
            for key, value in (arguments or {}).items():
                command[command.index('--' + key) + 1] = value
            result = subprocess.run(command, env=env, text=True, capture_output=True)
            calls = [json.loads(line) for line in (root / 'calls.jsonl').read_text().splitlines()] if (root / 'calls.jsonl').exists() else []
            return result, calls

    def test_dispatches_existing_ci_at_exact_fresh_bot_head(self):
        result, calls = self.run_dispatch()
        self.assertEqual(result.returncode, 0, result.stderr)
        writes = [call for call in calls if '--method' in call['args']]
        self.assertEqual(len(writes), 1)
        self.assertEqual(writes[0]['args'], ['api', 'repos/' + REPO + '/actions/workflows/ci.yml/dispatches', '--method', 'POST', '--input', '-'])
        self.assertEqual(json.loads(writes[0]['payload']), {'ref': BRANCH, 'inputs': {'expected_head_sha': HEAD}})

    def test_existing_inventory_only_repair_gets_source_ci(self):
        data = evidence()
        files = [{'filename': 'docs/agy.md', 'status': 'modified'}]
        data['/pulls/42']['changed_files'] = 1
        data['/pulls/42/files?per_page=100&page=1'] = files
        data['/compare/' + BASE + '...' + HEAD]['files'] = files
        result, calls = self.run_dispatch(data)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(sum('--method' in call['args'] for call in calls), 1)

    def test_rejects_untrusted_or_stale_pr_without_dispatch(self):
        cases = []
        for field, value in [('state', 'closed'), ('draft', True), ('commits', 2), ('changed_files', 3)]:
            data = evidence(); data['/pulls/42'][field] = value; cases.append(data)
        for section, field, value in [('user', 'login', 'wstein'), ('user', 'type', 'User'),
                                      ('head', 'sha', 'c' * 40), ('head', 'ref', 'main'),
                                      ('base', 'sha', 'c' * 40), ('base', 'ref', 'other')]:
            data = evidence(); data['/pulls/42'][section][field] = value; cases.append(data)
        data = evidence(); data['/pulls/42']['head']['repo']['full_name'] = 'other/repo'; cases.append(data)
        data = evidence(); data['/git/ref/heads/main']['object']['sha'] = 'c' * 40; cases.append(data)
        data = evidence(); data['/compare/' + BASE + '...' + HEAD]['files'][0]['filename'] = '.github/workflows/ci.yml'; cases.append(data)
        data = evidence(); data['/pulls/42/files?per_page=100&page=1'][0]['status'] = 'added'; cases.append(data)
        data = evidence(); data['/compare/' + BASE + '...' + HEAD]['commits'][0]['parents'] = [{'sha': 'c' * 40}]; cases.append(data)
        data = evidence(); data['/actions/workflows/ci.yml']['path'] = '.github/workflows/publish-bundles.yml'; cases.append(data)
        for data in cases:
            with self.subTest(data=data):
                result, calls = self.run_dispatch(data)
                self.assertNotEqual(result.returncode, 0)
                self.assertFalse(any('--method' in call['args'] for call in calls))

    def test_rechecks_remote_head_and_rejects_other_runtime_scopes(self):
        for change in ({'MOVE_HEAD': '1'}, {'GITHUB_REPOSITORY': 'other/repo'},
                       {'DEFAULT_BRANCH': 'other'}, {'GITHUB_REF': 'refs/heads/other'},
                       {'GITHUB_SHA': 'c' * 40}, {'GITHUB_RUN_ID': '123;bad'},
                       {'GITHUB_RUN_ATTEMPT': '2'}):
            with self.subTest(change=change):
                result, calls = self.run_dispatch(env_changes=change)
                self.assertNotEqual(result.returncode, 0)
                self.assertFalse(any('--method' in call['args'] for call in calls))

    def test_ref_url_and_revision_inputs_cannot_select_other_source(self):
        for arguments in ({'branch': 'main'}, {'branch': 'automation/agy-upgrade-999-1'},
                          {'branch': 'automation/agy-upgrade-123-1;bad'}, {'bundles': 'unknown'},
                          {'pr-url': 'https://github.com/other/repo/pull/42'},
                          {'pr-url': URL + '/extra'}, {'head': 'b' * 7}, {'base': 'a' * 7}):
            with self.subTest(arguments=arguments):
                result, calls = self.run_dispatch(arguments=arguments)
                self.assertNotEqual(result.returncode, 0)
                self.assertFalse(calls)

    def test_skips_exact_queued_running_or_successful_ci_but_retries_failure(self):
        endpoint = '/actions/workflows/ci.yml/runs?branch=' + BRANCH + '&event=workflow_dispatch&per_page=100&page=1'
        for status, conclusion, dispatch in [('queued', None, False), ('in_progress', None, False),
                                             ('completed', 'success', False), ('completed', 'failure', True)]:
            with self.subTest(status=status, conclusion=conclusion):
                data = evidence()
                data[endpoint] = {'total_count': 1, 'workflow_runs': [{'id': 9, 'workflow_id': 7,
                    'path': '.github/workflows/ci.yml', 'head_sha': HEAD, 'head_branch': BRANCH,
                    'event': 'workflow_dispatch', 'status': status, 'conclusion': conclusion,
                    'repository': {'full_name': REPO}, 'head_repository': {'full_name': REPO}}]}
                result, calls = self.run_dispatch(data)
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertEqual(any('--method' in call['args'] for call in calls), dispatch)

    def test_combined_proposal_for_several_bundles_gets_one_dispatch(self):
        branch = 'automation/upgrades-123-1'
        data = evidence(('agy', 'node', 'toolbox'), branch)
        result, calls = self.run_dispatch(data, bundles='agy,node,toolbox', branch=branch)
        self.assertEqual(result.returncode, 0, result.stderr)
        writes = [call for call in calls if '--method' in call['args']]
        self.assertEqual(len(writes), 1)
        self.assertEqual(json.loads(writes[0]['payload']), {'ref': branch, 'inputs': {'expected_head_sha': HEAD}})

    def nixbox_evidence(self, extra=None):
        branch = 'automation/nixbox-upgrade-123-1'
        data = evidence(('nixbox',), branch)
        files = [{'filename': 'bundles/nixbox/packages.lock.json', 'status': 'modified'}] + (extra or [])
        data['/pulls/42/files?per_page=100&page=1'] = files
        data['/pulls/42']['changed_files'] = len(files)
        data['/compare/' + BASE + '...' + HEAD]['files'] = files
        return data, branch

    def test_nixbox_proposal_is_its_own_lock_only_pull_request(self):
        data, branch = self.nixbox_evidence()
        result, calls = self.run_dispatch(data, bundles='nixbox', branch=branch)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(len([call for call in calls if '--method' in call['args']]), 1)
        for extra in ([{'filename': 'docs/nixbox.md', 'status': 'modified'}],
                      [{'filename': 'bundles/nixbox/packages.catalog.json', 'status': 'modified'}]):
            with self.subTest(extra=extra):
                data, branch = self.nixbox_evidence(extra)
                result, calls = self.run_dispatch(data, bundles='nixbox', branch=branch)
                self.assertNotEqual(result.returncode, 0)
                self.assertFalse([call for call in calls if '--method' in call['args']])

    def test_nixbox_never_joins_a_combined_proposal(self):
        branch = 'automation/upgrades-123-1'
        result, calls = self.run_dispatch(evidence(('agy', 'nixbox'), branch), bundles='agy,nixbox', branch=branch)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('nixbox', result.stderr)
        self.assertFalse(calls)

    def test_combined_proposal_may_leave_a_bundle_inventory_untouched(self):
        branch = 'automation/upgrades-123-1'
        data = evidence(('agy', 'node'), branch)
        files = data['/pulls/42/files?per_page=100&page=1'][:3]
        data['/pulls/42/files?per_page=100&page=1'] = files
        data['/pulls/42']['changed_files'] = 3
        data['/compare/' + BASE + '...' + HEAD]['files'] = files
        result, _ = self.run_dispatch(data, bundles='agy,node', branch=branch)
        self.assertEqual(result.returncode, 0, result.stderr)

    def test_branch_name_must_match_the_bundle_count(self):
        # One bundle keeps its own name (publication recognises it); several use the combined name.
        for bundles, branch in [('agy,node', BRANCH), ('agy,node', 'automation/node-upgrade-123-1'),
                                ('agy', 'automation/upgrades-123-1'), ('agy,node', 'automation/upgrades-999-1'),
                                ('agy,node', 'automation/upgrades-123-1;bad')]:
            with self.subTest(bundles=bundles, branch=branch):
                result, calls = self.run_dispatch(evidence(bundles.split(','), branch), bundles=bundles, branch=branch)
                self.assertNotEqual(result.returncode, 0)
                self.assertFalse(calls)

    def test_bundle_list_must_be_known_unique_and_nonempty(self):
        branch = 'automation/upgrades-123-1'
        for bundles in ('', 'agy,agy', 'agy,unknown', 'agy,', ',agy', 'agy node'):
            with self.subTest(bundles=bundles):
                result, calls = self.run_dispatch(evidence(('agy', 'node'), branch), bundles=bundles, branch=branch)
                self.assertNotEqual(result.returncode, 0)
                self.assertFalse(calls)

    def test_file_outside_the_union_of_selected_bundles_is_refused(self):
        branch = 'automation/upgrades-123-1'
        for extra in ('docs/claude.md', 'bundles/claude/packages.lock.json', '.github/workflows/ci.yml'):
            with self.subTest(extra=extra):
                data = evidence(('agy', 'node'), branch)
                item = {'filename': extra, 'status': 'modified'}
                for key in ('/pulls/42/files?per_page=100&page=1',):
                    data[key] = data[key][:3] + [item]
                data['/compare/' + BASE + '...' + HEAD]['files'] = data['/pulls/42/files?per_page=100&page=1']
                result, calls = self.run_dispatch(data, bundles='agy,node', branch=branch)
                self.assertNotEqual(result.returncode, 0)
                self.assertFalse(any('--method' in call['args'] for call in calls))

    def test_combined_proposal_must_stay_one_commit(self):
        branch = 'automation/upgrades-123-1'
        data = evidence(('agy', 'node'), branch)
        data['/pulls/42']['commits'] = 2
        result, calls = self.run_dispatch(data, bundles='agy,node', branch=branch)
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse(any('--method' in call['args'] for call in calls))
        data = evidence(('agy', 'node'), branch)
        data['/compare/' + BASE + '...' + HEAD]['ahead_by'] = 2
        result, calls = self.run_dispatch(data, bundles='agy,node', branch=branch)
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse(any('--method' in call['args'] for call in calls))

    def test_combined_proposal_skips_duplicate_dispatch(self):
        branch = 'automation/upgrades-123-1'
        data = evidence(('agy', 'node'), branch)
        data['/actions/workflows/ci.yml/runs?branch=' + branch + '&event=workflow_dispatch&per_page=100&page=1'] = {
            'total_count': 1, 'workflow_runs': [{'id': 9, 'workflow_id': 7, 'path': '.github/workflows/ci.yml',
            'head_sha': HEAD, 'head_branch': branch, 'event': 'workflow_dispatch', 'status': 'queued', 'conclusion': None,
            'repository': {'full_name': REPO}, 'head_repository': {'full_name': REPO}}]}
        result, calls = self.run_dispatch(data, bundles='agy,node', branch=branch)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertFalse(any('--method' in call['args'] for call in calls))

    def test_dispatch_failure_is_not_success(self):
        result, _ = self.run_dispatch(env_changes={'FAIL_DISPATCH': '1'})
        self.assertNotEqual(result.returncode, 0)

    def test_one_write_job_proposes_all_bundles_with_narrow_permissions(self):
        caller = (ROOT / '.github/workflows/propose-upgrades.yml').read_text()
        helper = (ROOT / '.github/workflows/bundle-upgrade.yml').read_text()
        proposal = caller.split('\n  propose:\n')[1]
        resolvers = caller.split('\n  propose:\n')[0] + helper
        self.assertEqual(caller.count('scripts/tool-upgrade.sh publish'), 1)
        self.assertNotIn('tool-upgrade.sh publish', helper)
        for text in ('actions: write', 'contents: write', 'pull-requests: write'):
            self.assertIn(text, proposal)
            self.assertNotIn(text, resolvers)
        # Pin the exact write scopes: a mutant adding another scope must fail.
        permissions = proposal.split('    permissions:\n')[1].split('    concurrency:')[0]
        self.assertEqual(sorted(line.strip() for line in permissions.splitlines()),
                         ['actions: write', 'contents: write', 'pull-requests: write'])
        self.assertTrue(caller.startswith('name: Propose upgrades') and '\npermissions:\n  contents: read\n' in caller.split('\njobs:\n')[0])
        self.assertNotIn('permissions:', caller.split('\n  proposal:\n')[1].split('\n  propose:\n')[0])
        # Any failed step of a resolve job, not only the resolver, marks the bundle failed.
        marker = helper.split('id: failed\n')[1].split('run:')[0]
        self.assertIn('if: ${{ failure() && (github.event_name', marker)
        self.assertNotIn('steps.resolve.outcome', marker)
        self.assertIn('needs: [plan, proposal]', proposal)
        self.assertIn('!inputs.dry_run', proposal)
        self.assertIn('bundledock-upgrade-proposal', proposal)
        shell = (ROOT / 'scripts/tool-upgrade.sh').read_text()
        self.assertGreater(shell.index('scripts/updater-source-ci.py'), shell.index('gh pr create'))
        self.assertGreater(shell.index('scripts/updater-source-ci.py'), shell.index('if git diff --quiet'))

if __name__ == '__main__':
    unittest.main()
