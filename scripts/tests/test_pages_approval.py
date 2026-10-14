import copy
import importlib.util
import json
import os
import subprocess
import sys
import tempfile
from pathlib import Path
import unittest

SPEC = importlib.util.spec_from_file_location('pages', Path(__file__).resolve().parents[1] / 'pages-approval.py')
pages = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(pages)
SHA = 'a' * 40


class ApprovalTests(unittest.TestCase):
    def setUp(self):
        self.env = dict(GITHUB_EVENT_NAME='workflow_run', GITHUB_REPOSITORY='wstein/bundledock',
                        GITHUB_REF='refs/heads/main', DEFAULT_BRANCH='main', GITHUB_SHA=SHA,
                        PUBLICATION_SHA=SHA, CI_RUN_ID='123', CI_RUN_ATTEMPT='2')
        self.responses = {
            '': dict(full_name='wstein/bundledock', default_branch='main'),
            '/branches/main': dict(commit=dict(sha=SHA)),
            '/actions/workflows/ci.yml': dict(id=42, path='.github/workflows/ci.yml', state='active'),
            '/actions/runs/123': dict(id=123, workflow_id=42, path='.github/workflows/ci.yml',
                head_sha=SHA, head_branch='main', event='push', status='completed', conclusion='success', run_attempt=2,
                repository=dict(full_name='wstein/bundledock'), head_repository=dict(full_name='wstein/bundledock')),
            '/actions/runs/123/attempts/2/jobs?per_page=100&page=1': dict(total_count=1,
                jobs=[dict(id=7, name='check', head_sha=SHA, status='completed', conclusion='success')]),
        }

    def api(self, path):
        return copy.deepcopy(self.responses[path])

    def test_shell_pages_mode_emits_verified_sha_with_read_only_api_calls(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            fixture = root / 'fixture.json'
            fixture.write_text(json.dumps(self.responses))
            gh = root / 'gh'
            gh.write_text('#!' + sys.executable + '\nimport json,sys\n'
                + 'assert sys.argv[1] == "api"\n'
                + 'prefix = "repos/wstein/bundledock"\n'
                + 'assert sys.argv[2].startswith(prefix)\n'
                + 'data = json.load(open(' + repr(str(fixture)) + '))\n'
                + 'print(json.dumps(data[sys.argv[2][len(prefix):]]))\n')
            gh.chmod(0o700)
            output = root / 'outputs'
            env = dict(os.environ, **self.env)
            env.update(PATH=directory + os.pathsep + os.environ['PATH'], GITHUB_OUTPUT=str(output))
            script = Path(__file__).resolve().parents[1] / 'approve-publication.sh'
            result = subprocess.run(['sh', str(script), '--pages'], env=env, capture_output=True, text=True)
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertEqual(output.read_text(), 'sha=' + SHA + '\n')
            output.unlink()
            env['CI_RUN_ID'] = '999'
            result = subprocess.run(['sh', str(script), '--pages'], env=env, capture_output=True, text=True)
            self.assertNotEqual(result.returncode, 0)
            self.assertFalse(output.exists())

    def test_successful_main_push_ci_and_manual_ci_are_eligible(self):
        self.assertEqual(pages.validate(self.env, self.api), SHA)
        self.responses['/actions/runs/123']['event'] = 'workflow_dispatch'
        self.assertEqual(pages.validate(self.env, self.api), SHA)

    def test_wrong_branch_repo_workflow_status_attempt_and_commit_rejected(self):
        key = '/actions/runs/123'
        for field, value in [('head_sha', 'b' * 40), ('head_branch', 'topic'), ('event', 'pull_request'),
                             ('status', 'in_progress'), ('conclusion', 'failure'), ('run_attempt', 3),
                             ('workflow_id', 99), ('path', '.github/workflows/other.yml'),
                             ('repository', dict(full_name='other/repo')),
                             ('head_repository', dict(full_name='fork/bundledock'))]:
            with self.subTest(field=field):
                original = self.responses[key][field]
                self.responses[key][field] = value
                with self.assertRaises(ValueError):
                    pages.validate(self.env, self.api)
                self.responses[key][field] = original

    def test_main_advance_and_missing_evidence_rejected(self):
        self.responses['/branches/main']['commit']['sha'] = 'b' * 40
        with self.assertRaises(ValueError):
            pages.validate(self.env, self.api)
        self.responses['/branches/main']['commit']['sha'] = SHA
        self.responses['/actions/runs/123'] = {}
        with self.assertRaises(ValueError):
            pages.validate(self.env, self.api)

    def test_missing_skipped_failed_and_duplicate_required_check_rejected(self):
        key = '/actions/runs/123/attempts/2/jobs?per_page=100&page=1'
        good = self.responses[key]['jobs'][0]
        for jobs in ([], [dict(good, conclusion='skipped')], [dict(good, conclusion='failure')],
                     [dict(good, head_sha='b' * 40)], [good, dict(good, id=8)]):
            with self.subTest(jobs=jobs), self.assertRaises(ValueError):
                self.responses[key] = dict(total_count=len(jobs), jobs=jobs)
                pages.validate(self.env, self.api)

    def test_jobs_are_paginated_and_truncated_evidence_rejected(self):
        prefix = '/actions/runs/123/attempts/2/jobs?per_page=100&page='
        good = self.responses[prefix + '1']['jobs'][0]
        self.responses[prefix + '1'] = dict(total_count=101, jobs=[dict(good, id=i + 20, name='other') for i in range(100)])
        self.responses[prefix + '2'] = dict(total_count=101, jobs=[good])
        self.assertEqual(pages.validate(self.env, self.api), SHA)
        self.responses[prefix + '2']['jobs'] = []
        with self.assertRaises(ValueError):
            pages.validate(self.env, self.api)

    def test_bad_context_cannot_enable_automatic_publication(self):
        for field, value in [('GITHUB_EVENT_NAME', 'push'), ('GITHUB_EVENT_NAME', 'pull_request'),
                             ('GITHUB_REPOSITORY', 'foreign/repo'), ('DEFAULT_BRANCH', 'develop'),
                             ('GITHUB_REF', 'refs/heads/topic'), ('PUBLICATION_SHA', 'short'),
                             ('GITHUB_SHA', 'b' * 40), ('CI_RUN_ID', '../123'), ('CI_RUN_ATTEMPT', '')]:
            with self.subTest(field=field), self.assertRaises(ValueError):
                pages.validate(dict(self.env, **{field: value}), self.api)

    def test_main_advance_or_ci_rerun_during_validation_rejected(self):
        calls = {}
        for path in ('/branches/main', '/actions/runs/123'):
            calls.clear()
            def changing_api(request):
                calls[request] = calls.get(request, 0) + 1
                data = self.api(request)
                if request == path and calls[request] == 2:
                    if path == '/branches/main':
                        data['commit']['sha'] = 'b' * 40
                    else:
                        data['run_attempt'] = 3
                return data
            with self.subTest(path=path), self.assertRaises(ValueError):
                pages.validate(self.env, changing_api)

    def test_manual_human_approval_still_works_and_rejects_bots_or_wrong_sha(self):
        self.env.update(GITHUB_EVENT_NAME='workflow_dispatch', APPROVED_SHA=SHA,
                        SENDER_TYPE='User', GITHUB_ACTOR='wstein', TRIGGERING_ACTOR='wstein')
        self.assertEqual(pages.validate(self.env, self.api), SHA)
        for field, value in [('SENDER_TYPE', 'Bot'), ('GITHUB_ACTOR', 'github-actions[bot]'),
                             ('TRIGGERING_ACTOR', ''), ('APPROVED_SHA', 'b' * 40), ('GITHUB_SHA', 'b' * 40)]:
            with self.subTest(field=field), self.assertRaises(ValueError):
                pages.validate(dict(self.env, **{field: value}), self.api)
