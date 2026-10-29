"""nixbox is in the build, upgrade-proposal and cleanup machinery, never in publication."""
import fnmatch
import importlib.util
from pathlib import Path
import re
import unittest

ROOT = Path(__file__).resolve().parents[2]


def load(name, file):
    spec = importlib.util.spec_from_file_location(name, ROOT / 'scripts' / file)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


config = load('bundle_config', 'bundle-config.py')
publication = load('bundle_publication', 'bundle-publication.py')


class NixboxScopeTests(unittest.TestCase):
    def test_nixbox_is_not_a_publishable_bundle(self):
        self.assertNotIn('nixbox', config.BUNDLES)
        self.assertEqual(config.UPGRADE_ONLY, ('nixbox',))
        with self.assertRaises(ValueError):
            config.select_bundle('nixbox')

    def test_publication_workflows_and_script_do_not_name_nixbox(self):
        for name in ('publish-bundles.yml', 'bundle-publish.yml', 'published-consumers.yml'):
            self.assertNotIn('nixbox', (ROOT / '.github/workflows' / name).read_text(), name)
        self.assertNotIn('nixbox', (ROOT / 'scripts/bundle-publication.py').read_text())
        self.assertNotIn('nixbox', (ROOT / 'scripts/published-consumers.sh').read_text())

    def test_nixbox_updater_topic_is_never_a_publishable_updater(self):
        # Not recognized as any bundle's updater topic, so it can never authorize publication.
        for branch in ('automation/nixbox-upgrade-123-1', 'automation/upgrades-123-1-nixbox'):
            with self.subTest(branch=branch):
                try:
                    found = publication.bundle_for_branch(branch)
                except ValueError:
                    found = None
                self.assertNotIn(found, tuple(config.BUNDLES))

    def test_every_workflow_bundle_option_is_a_bundle_directory(self):
        options = re.search(r'bundle:\n(?:.*\n)*?        options:\n((?:          - .+\n)+)',
                            (ROOT / '.github/workflows/propose-upgrades.yml').read_text()).group(1)
        names = re.findall(r'- (\S+)', options)
        self.assertEqual(names, ['all', *config.BUNDLES, 'nixbox'])
        for name in names[1:]:
            self.assertTrue((ROOT / 'bundles' / name / 'Dockerfile').is_file(), name)

    def test_nixbox_proposal_job_is_separate_least_privilege_and_never_builds(self):
        source = (ROOT / '.github/workflows/propose-upgrades.yml').read_text()
        job = source.split('\n  propose-nixbox:\n')[1]
        self.assertNotIn('\n  propose:\n', job)
        for text in ("contains(fromJSON(needs.plan.outputs.bundles), 'nixbox')", 'contents: write', 'pull-requests: write',
                     'actions: write', 'group: bundledock-nixbox-upgrade-proposal', 'cancel-in-progress: false',
                     'PROPOSAL_GROUP: nixbox', "pattern: 'nixbox-lockrefresh-${{ github.run_attempt }}'",
                     'persist-credentials: false', 'ref: ${{ github.sha }}', 'http.https://github.com/.extraheader',
                     '--unset-all http.https://github.com/.extraheader', 'bash scripts/tool-upgrade.sh publish',
                     "github.event_name == 'schedule' || (github.event_name == 'workflow_dispatch' && !inputs.dry_run)"):
            self.assertIn(text, job)
        for text in ('bundle-build', 'bundle-smoke', 'gh workflow', 'workflow run', 'secrets.', 'docker ', 'publish-bundles',
                     'login'):
            self.assertNotIn(text, job)
        for action in re.findall(r'uses: ([^\n]+)', source):
            if not action.startswith('./'):
                self.assertRegex(action, r'@[0-9a-f]{40} ')

    def test_nixbox_artifacts_never_reach_the_combined_proposal(self):
        helper = (ROOT / '.github/workflows/bundle-upgrade.yml').read_text()
        self.assertIn("inputs.bundle == 'nixbox' && 'nixbox-lockrefresh'", helper)
        self.assertIn("inputs.bundle != 'nixbox'", helper)
        main = (ROOT / '.github/workflows/propose-upgrades.yml').read_text().split('\n  propose:\n')[1].split('\n  propose-nixbox:\n')[0]
        self.assertIn("pattern: '*-upgrade-${{ github.run_attempt }}'", main)
        self.assertFalse(fnmatch.fnmatch('nixbox-lockrefresh-1', '*-upgrade-1'))
        self.assertFalse(fnmatch.fnmatch('nixbox-lockrefresh-1', '*-failed-1'))
        self.assertNotIn('PROPOSAL_GROUP', main)

    def test_resolution_never_dispatches_a_build(self):
        for name in ('propose-upgrades.yml', 'bundle-upgrade.yml'):
            text = (ROOT / '.github/workflows' / name).read_text()
            for forbidden in ('make bundle-build', 'make bundle-smoke', 'nixbox-build', 'gh workflow', 'actions/workflows'):
                self.assertNotIn(forbidden, text, name)

    def test_no_workflow_other_than_the_manual_build_builds_nixbox(self):
        for path in (ROOT / '.github/workflows').glob('*.yml'):
            text = path.read_text()
            if 'BUNDLE=nixbox' in text:
                self.assertEqual(path.name, 'nixbox-build.yml')


if __name__ == '__main__':
    unittest.main()
