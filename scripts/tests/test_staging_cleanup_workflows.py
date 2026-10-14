from pathlib import Path
import json
import os
import re
import subprocess
import tempfile
import textwrap
import unittest

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
