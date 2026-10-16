from pathlib import Path
import json
import os
import re
import subprocess
import tempfile
import textwrap
import unittest
from types import SimpleNamespace

ROOT = Path(__file__).resolve().parents[2]


class WorkflowTests(unittest.TestCase):
    def test_cleanup_waits_for_publication_and_records_plan_before_apply(self):
        source = (ROOT / '.github/workflows/bundle-publish.yml').read_text()
        job = source.split('\n  cleanup:\n')[1].split('\n  outcome:\n')[0]
        for name in ('promote', 'tag', 'documentation'):
            self.assertIn("needs." + name + ".result == 'success'", job)
        self.assertLess(job.index('name: Save cleanup dry run'), job.index('name: Remove verified staging tags'))
        self.assertIn('--approved-plan staging-plan.json', job)
        self.assertNotIn('contents: write', job)
        self.assertIn('--publication publication.json', job)

    def test_fallback_is_daily_and_shares_repository_lock(self):
        source = (ROOT / '.github/workflows/staging-cleanup.yml').read_text()
        self.assertIn('queue: max', source)
        self.assertIn('queue: max', (ROOT / '.github/workflows/bundle-publish.yml').read_text())
        self.assertIn('schedule:', source)
        self.assertIn('default: false', source)
        self.assertIn('group: bundledock-${{ matrix.repository }}-publish', source)
        self.assertIn('--approved-plan staging-plan.json', source)
        self.assertLess(source.index('name: Save cleanup dry run'), source.index('name: Remove stale staging tags'))
        self.assertNotIn('docker ', source)
        self.assertNotIn('contents: write', source)

    def test_failed_completion_cleanup_uses_trusted_code_saved_plan_and_same_lock(self):
        source = (ROOT / '.github/workflows/staging-cleanup.yml').read_text()
        self.assertIn('workflows: [Publish bundles]', source)
        self.assertIn('types: [completed]', source)
        self.assertIn("github.event.workflow_run.conclusion == 'failure'", source)
        self.assertIn("github.event.workflow_run.conclusion == 'cancelled'", source)
        self.assertIn("github.event.workflow_run.head_branch == 'main'", source)
        self.assertIn('ref: ${{ github.sha }}', source)
        self.assertNotIn('ref: ${{ github.event.workflow_run.head_sha }}', source)
        self.assertIn('github.event.workflow_run.run_attempt', source)
        self.assertEqual(source.count('--failed-run "$FAILED_RUN" --failed-attempt "$FAILED_ATTEMPT"'), 2)
        self.assertIn("github.event_name == 'workflow_run'", source)
        self.assertIn('group: bundledock-${{ matrix.repository }}-publish', source)
        self.assertLess(source.index('name: Save cleanup dry run'), source.index('name: Remove stale staging tags'))
        self.assertNotIn('actions/download-artifact', source)

    def test_completion_gate_rejects_success_foreign_and_non_main_runs(self):
        source = (ROOT / '.github/workflows/staging-cleanup.yml').read_text()
        expression = source.split('    if: >-\n', 1)[1].split('    strategy:', 1)[0]
        expression = expression.replace('&&', ' and ').replace('||', ' or ')
        expression = ' '.join(expression.split())
        run = dict(status='completed', conclusion='failure', head_branch='main',
                   head_repository=SimpleNamespace(full_name='wstein/bundledock'))
        def allowed(**changes):
            candidate = SimpleNamespace(**dict(run, **changes))
            github = SimpleNamespace(repository='wstein/bundledock', ref='refs/heads/main',
                                     event_name='workflow_run', event=SimpleNamespace(workflow_run=candidate))
            return eval(expression, {'__builtins__': {}}, {'github': github})
        for conclusion in ('failure', 'cancelled', 'timed_out'):
            self.assertTrue(allowed(conclusion=conclusion))
        for changes in (dict(conclusion='success'), dict(conclusion='skipped'),
                        dict(status='in_progress'), dict(head_branch='topic'),
                        dict(head_repository=SimpleNamespace(full_name='other/bundledock'))):
            self.assertFalse(allowed(**changes))

    def test_cleanup_reuses_registry_credentials_without_extra_secret(self):
        publisher = (ROOT / '.github/workflows/bundle-publish.yml').read_text()
        fallback = (ROOT / '.github/workflows/staging-cleanup.yml').read_text()
        caller = (ROOT / '.github/workflows/publish-bundles.yml').read_text()
        cleanup_job = publisher.split('\n  cleanup:\n')[1].split('\n  outcome:\n')[0]
        for source in (cleanup_job, fallback):
            self.assertIn('REPOSITORY_USERNAME: ${{ vars.REPOSITORY_USERNAME }}', source)
            self.assertIn('REPOSITORY_TOKEN: ${{ secrets.REPOSITORY_TOKEN }}', source)
            self.assertIn("env.REPOSITORY_USERNAME != '' && env.REPOSITORY_TOKEN != ''", source)
        for source in (publisher, fallback, caller):
            self.assertNotIn('QUAY_CLEANUP_TOKEN', source)

    def test_cleanup_failure_preserves_published_outcome(self):
        source = (ROOT / '.github/workflows/bundle-publish.yml').read_text()
        program = textwrap.dedent(re.search("python3 - <<'PYTHON'\\n(.*?)\\n          PYTHON", source, re.S).group(1))
        with tempfile.TemporaryDirectory() as directory:
            summary = Path(directory) / 'summary'
            needs = {name: {'result': 'success'} for name in ('preflight', 'resolve', 'native', 'promote', 'tag', 'documentation')}
            needs['cleanup'] = {'result': 'failure'}
            env = dict(os.environ, BUNDLE='toolbox', ELIGIBLE='true', STATUS='eligible', PUBLISH='true', RESULTS=json.dumps(needs), GITHUB_STEP_SUMMARY=str(summary))
            subprocess.run(['python3', '-c', program], env=env, check=True)
            self.assertIn('**published**', summary.read_text())
            self.assertIn('Staging cleanup: **failure**', summary.read_text())
