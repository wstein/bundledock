import importlib.util
from pathlib import Path
import unittest

ROOT = Path(__file__).resolve().parents[2]
spec = importlib.util.spec_from_file_location('bundle_config', ROOT / 'scripts/bundle-config.py')
config = importlib.util.module_from_spec(spec)
spec.loader.exec_module(config)


class ConsolidatedWorkflowsTests(unittest.TestCase):
    def test_all_bundles_share_one_current_publication_caller(self):
        for name in config.BUNDLES:
            self.assertEqual(config.select_bundle(name)['workflow'], 'publish-bundles.yml')

    def test_proposals_keep_isolated_bounded_jobs(self):
        source = (ROOT / '.github/workflows/propose-upgrades.yml').read_text()
        for text in ("- cron: '50 6 * * *'", 'workflow_dispatch:', 'bundle:', 'dry_run:',
                     'tool:', 'fail-fast: false', 'max-parallel: 3', 'fromJSON(needs.plan.outputs.bundles)',
                     'uses: ./.github/workflows/bundle-upgrade.yml'):
            self.assertIn(text, source)
        helper = (ROOT / '.github/workflows/bundle-upgrade.yml').read_text()
        self.assertIn('\n  propose-pr:\n', helper)
        self.assertNotIn('\n  publish:\n', helper)
        self.assertIn('bundledock-${{ inputs.bundle }}-upgrade', helper)

    def test_publication_planner_has_read_only_permissions_and_no_compiler(self):
        source = (ROOT / '.github/workflows/publish-bundles.yml').read_text()
        plan = source.split('\n  plan:\n')[1].split('\n  publication:\n')[0]
        for text in ('contents: read', 'actions: read', 'pull-requests: read', '--mode plan'):
            self.assertIn(text, plan)
        for text in ('contents: write', 'setup-go', 'go build', 'docker '):
            self.assertNotIn(text, plan)
        self.assertIn("needs.plan.outputs.eligible == 'true'", source)
        self.assertIn('bundle: ${{ needs.plan.outputs.bundle }}', source)
        for name in config.BUNDLES:
            self.assertFalse((ROOT / '.github/workflows' / ('bundledock-' + name + '.yml')).exists())
            self.assertFalse((ROOT / '.github/workflows' / (name + '-upgrade.yml')).exists())

    def test_selector_emits_all_or_one_and_rejects_unknown(self):
        import json
        import subprocess
        for selection, expected in [('all', list(config.BUNDLES)), ('node-current', ['node-current'])]:
            result = subprocess.run(['python3', str(ROOT / 'scripts/bundle-config.py'), '--upgrade-selection', selection],
                                    text=True, capture_output=True, check=True)
            self.assertEqual(json.loads(result.stdout), expected)
        result = subprocess.run(['python3', str(ROOT / 'scripts/bundle-config.py'), '--upgrade-selection', 'unknown'],
                                capture_output=True)
        self.assertNotEqual(result.returncode, 0)

    def test_publication_outcomes_keep_rejection_noop_and_failure_distinct(self):
        import json
        import os
        import re
        import subprocess
        import tempfile
        import textwrap
        source = (ROOT / '.github/workflows/bundle-publish.yml').read_text()
        program = textwrap.dedent(re.search("python3 - <<'PYTHON'\\n(.*?)\\n          PYTHON", source, re.S).group(1))
        for status, eligible, publish, failed, expected in [
            ('rejected', 'false', '', True, 'rejected'),
            ('pending', 'false', '', True, 'pending'),
            ('blocked_owner_merge', 'false', '', False, 'blocked_owner_merge'),
            ('ineligible', 'false', '', False, 'ineligible'),
            ('eligible', 'true', 'false', False, 'unchanged'),
            ('eligible', 'true', 'true', False, 'published'),
            ('eligible', 'true', 'true', True, 'build-failed'),
        ]:
            with self.subTest(expected=expected), tempfile.TemporaryDirectory() as temporary:
                summary = Path(temporary) / 'summary'
                needs = {name: {'result': 'success'} for name in ('preflight', 'resolve', 'native', 'promote', 'tag', 'documentation')}
                if failed:
                    needs['native']['result'] = 'failure'
                env = dict(os.environ, BUNDLE='node', ELIGIBLE=eligible, STATUS=status, PUBLISH=publish,
                           RESULTS=json.dumps(needs), GITHUB_STEP_SUMMARY=str(summary))
                subprocess.run(['python3', '-c', program], env=env, check=True)
                self.assertEqual(summary.read_text(), 'Bundle node: **' + expected + '**\n')
