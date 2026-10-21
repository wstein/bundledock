import importlib.util
from pathlib import Path
import unittest

ROOT = Path(__file__).resolve().parents[2]
spec = importlib.util.spec_from_file_location('bundle_config', ROOT / 'scripts/bundle-config.py')
config = importlib.util.module_from_spec(spec)
spec.loader.exec_module(config)


class ConsolidatedWorkflowsTests(unittest.TestCase):
    def test_publication_planner_skips_unrelated_ci_completions(self):
        import re
        from types import SimpleNamespace as NS
        source = (ROOT / '.github/workflows/publish-bundles.yml').read_text()
        plan = source.split('\n  plan:\n')[1].split('\n  publication:\n')[0]
        match = re.search(r'^    if: >-\n((?:      .*\n)+)', plan, re.M)
        self.assertIsNotNone(match, 'Planner needs a CI completion filter')
        expression = ' '.join(match.group(1).split()).replace('&&', ' and ').replace('||', ' or ')
        for event in ('push', 'pull_request', 'workflow_dispatch', 'schedule'):
            for branch in ('main', 'automation/toolbox-upgrade-123-1'):
                for conclusion in ('success', 'failure', 'cancelled', 'skipped', None):
                    with self.subTest(event=event, branch=branch, conclusion=conclusion):
                        run = NS(event=event, head_branch=branch, conclusion=conclusion)
                        github = NS(event_name='workflow_run', event=NS(workflow_run=run))
                        actual = eval(expression, {'__builtins__': {}}, {'github': github})
                        self.assertEqual(actual, event in ('push', 'workflow_dispatch') and branch == 'main' and conclusion == 'success')
        # Explicit manual approval must still reach the planner without a CI payload.
        github = NS(event_name='workflow_dispatch', event=NS())
        self.assertTrue(eval(expression, {'__builtins__': {}}, {'github': github}))
        github.event_name = 'pull_request'
        self.assertFalse(eval(expression, {'__builtins__': {}}, {'github': github}))

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
        self.assertIn('\n  propose:\n', source)
        self.assertNotIn('\n  propose-pr:\n', helper)
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
        self.assertIn('bundle: ${{ matrix.bundle }}', source)
        self.assertIn('fromJSON(needs.plan.outputs.bundles)', source)
        self.assertIn('fail-fast: false', source.split('\n  publication:\n')[1])
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

    def run_outcome(self, needs, **extra):
        import json
        import os
        import re
        import subprocess
        import tempfile
        import textwrap
        source = (ROOT / '.github/workflows/bundle-publish.yml').read_text()
        program = textwrap.dedent(re.search("python3 - <<'PYTHON'\\n(.*?)\\n          PYTHON", source, re.S).group(1))
        with tempfile.TemporaryDirectory() as temporary:
            summary = Path(temporary) / 'summary'
            env = dict(os.environ, BUNDLE='claude', ELIGIBLE='true', STATUS='eligible', PUBLISH='true',
                       RESULTS=json.dumps(needs), GITHUB_STEP_SUMMARY=str(summary), **extra)
            subprocess.run(['python3', '-c', program], env=env, check=True)
            return summary.read_text()

    def needs_after_promote_gate(self, status):
        needs = {name: {'result': 'success'} for name in ('preflight', 'resolve', 'native', 'tag', 'documentation')}
        needs['promote'] = {'result': 'failure', 'outputs': {'gate_status': status, 'gate_detail': 'Source is no longer an ancestor of current main'}}
        return needs

    def test_promote_gate_rejection_is_reported_as_source_rejected_with_recovery(self):
        sha = 'a' * 40
        for status in ('rejected', 'pending', 'blocked_owner_merge'):
            with self.subTest(status=status):
                text = self.run_outcome(self.needs_after_promote_gate(status), SOURCE_SHA=sha)
                self.assertIn('Bundle claude: **source-rejected**', text)
                self.assertNotIn('build-failed', text)
                self.assertIn('Source is no longer an ancestor of current main', text)
                self.assertIn('manual dispatch', text)
                self.assertIn('bundle=claude', text)
                self.assertIn('current main', text)

    def test_promote_failure_without_gate_rejection_stays_build_failed(self):
        needs = self.needs_after_promote_gate('')
        needs['promote']['outputs'] = {'gate_status': '', 'gate_detail': ''}
        self.assertIn('Bundle claude: **build-failed**', self.run_outcome(needs, SOURCE_SHA='a' * 40))
        del needs['promote']['outputs']
        self.assertIn('Bundle claude: **build-failed**', self.run_outcome(needs, SOURCE_SHA='a' * 40))

    def test_promote_gate_outputs_are_exported_and_consumed(self):
        source = (ROOT / '.github/workflows/bundle-publish.yml').read_text()
        self.assertIn('gate_status: ${{ steps.final_gate.outputs.status || steps.early_gate.outputs.status }}', source)
        self.assertIn('id: final_gate', source)
        self.assertIn('id: early_gate', source)
        self.assertIn('gate_detail: ${{ steps.final_gate.outputs.detail || steps.early_gate.outputs.detail }}', source)
        self.assertIn('SOURCE_SHA: ${{ needs.preflight.outputs.sha }}', source)

