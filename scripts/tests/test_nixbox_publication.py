"""nixbox is selectable for publication by the owner's dispatch only; no automatic path reaches it."""
import fnmatch
import importlib.util
import json
import os
from pathlib import Path
import re
import tempfile
import unittest
from unittest import mock

ROOT = Path(__file__).resolve().parents[2]


def load(name, file):
    spec = importlib.util.spec_from_file_location(name, ROOT / 'scripts' / file)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


config = load('bundle_config', 'bundle-config.py')
publication = load('bundle_publication', 'bundle-publication.py')
tags = load('bundle_git_tags', 'bundle-git-tags.py')
docs = load('publication_docs', 'publication-docs.py')
workflows = ROOT / '.github/workflows'


def workflow_options(name):
    text = (workflows / name).read_text()
    return re.findall(r'- (\S+)', re.search(r'bundle:\n(?:.*\n)*?        options:\n((?:          - .+\n)+)', text).group(1))


class NixboxPublicationTests(unittest.TestCase):
    def test_nixbox_is_a_publishable_bundle_of_its_own_repository(self):
        selected = config.select_bundle('nixbox')
        self.assertEqual(config.BUNDLES['nixbox'], selected)
        self.assertEqual((selected['repository'], selected['workflow'], selected['aliases'], selected['artifact']),
                         ('nixbox', 'publish-bundles.yml', 'stable latest', 'resolved-nixbox'))
        self.assertEqual((selected['lock'], selected['inventory'], selected['proposal_lock']),
                         ('bundles/nixbox/packages.lock.json', 'docs/nixbox.md', 'nixbox.lock.json'))
        self.assertEqual(config.MANUAL_ONLY, ('nixbox',))
        self.assertEqual(config.AUTOMATIC, tuple(name for name in config.BUNDLES if name != 'nixbox'))

    def test_dispatch_dropdown_lists_exactly_the_configured_bundles(self):
        self.assertEqual(workflow_options('publish-bundles.yml'), list(config.BUNDLES))
        self.assertEqual(workflow_options('publish-bundles.yml')[-1], 'nixbox')
        self.assertEqual(workflow_options('propose-upgrades.yml'), ['all', *config.BUNDLES])

    def test_selection_helpers_know_nixbox(self):
        self.assertEqual(tags.BUNDLES, tuple(config.BUNDLES))
        self.assertEqual(docs.BUNDLES['nixbox'], 'docs/nixbox.md')
        self.assertEqual(docs.metadata_path('nixbox'), 'docs/nixbox-published.json')

    def test_no_automatic_path_reaches_nixbox(self):
        for branch in ('automation/nixbox-upgrade-123-1', 'automation/upgrades-123-1-nixbox'):
            with self.subTest(branch=branch):
                try:
                    found = publication.bundle_for_branch(branch)
                except ValueError:
                    found = None
                self.assertNotIn(found, tuple(config.BUNDLES))
        # A combined updater never claims nixbox, even if its lock were among the changed files.
        files = [{'filename': config.select_bundle(name)['lock']} for name in ('agy', 'nixbox')]
        pr = {'number': 7, 'changed_files': 2}
        self.assertEqual(publication.updater_members(pr, publication.COMBINED, lambda path: files), ['agy'])

    def test_a_merged_ci_run_can_never_select_nixbox(self):
        with tempfile.TemporaryDirectory() as directory:
            event = Path(directory) / 'event.json'
            event.write_text('{}')
            env = {'GITHUB_REPOSITORY': 'wstein/bundledock', 'GITHUB_EVENT_NAME': 'workflow_run',
                   'GITHUB_EVENT_PATH': str(event), 'GITHUB_TOKEN': 'x'}
            with mock.patch.dict(os.environ, env), mock.patch.object(publication.sys, 'argv', ['p', '--bundle', 'nixbox', '--mode', 'preflight']):
                with self.assertRaisesRegex(SystemExit, 'manual dispatch'):
                    publication.run()

    def test_nixbox_publication_records_use_the_daily_tag(self):
        record = dict(bundle='nixbox', image='quay.io/bundledock/nixbox', source=tags.SOURCE, commit='a' * 40,
                      digest='sha256:' + 'b' * 64, run_id='123', tag='2026.10.30-b1', run_attempt='1')
        self.assertEqual(tags.validate_record(record)['attempt'], 1)
        self.assertEqual(tags.tag_name(record), 'nixbox/2026.10.30-b1')
        with self.assertRaises(ValueError):
            tags.validate_record(dict(record, tag='1.2.3-b1'))

    def test_publication_docs_record_nixbox_in_its_guide(self):
        record = dict(image='quay.io/bundledock/nixbox', tag='2026.10.30-b1', commit='a' * 40, source=docs.SOURCE,
                      created='2026-10-30T10:00:00Z', lock_sha256='c' * 64, run_id='123', run_attempt='1', bundle='nixbox',
                      digest='sha256:' + 'b' * 64, reference='quay.io/bundledock/nixbox@sha256:' + 'b' * 64)
        record['platforms'] = [dict(record, architecture=arch, smoked=True, image_id='sha256:' + 'd' * 64, digest='sha256:' + 'e' * 64)
                               for arch in ('amd64', 'arm64')]
        verified = docs.validate(record, 'nixbox', 'quay.io/bundledock/nixbox', 'a' * 40, '123', '1')
        self.assertEqual(verified['tag'], '2026.10.30-b1')
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / 'docs').mkdir()
            guide_source = (ROOT / 'docs/nixbox.md').read_text()
            (root / 'docs/nixbox.md').write_text(guide_source)
            self.assertTrue(docs.update(root, 'nixbox', verified))
            guide = (root / 'docs/nixbox.md').read_text()
            self.assertIn('docker pull quay.io/bundledock/nixbox@sha256:' + 'b' * 64, guide)
            self.assertEqual(json.loads((root / 'docs/nixbox-published.json').read_text())['bundle'], 'nixbox')

    def test_publication_workflows_smoke_nixbox_with_the_published_toolbox_record(self):
        shared = (workflows / 'bundle-publish.yml').read_text()
        step = shared.split('- name: Build once and smoke on the native architecture')[1].split('- name:')[0]
        self.assertIn('NIXBOX_PLATFORM: linux/${{ matrix.architecture }}', step)
        self.assertNotIn('NIXBOX_TOOLBOX_IMAGE', step)
        self.assertIn('docs/toolbox-published.json', (ROOT / 'bundles/nixbox/scripts/smoke.sh').read_text())

    def test_consumer_pins_wait_for_a_published_digest(self):
        # published-consumers pins digests of published images; nixbox has none until its first publication.
        self.assertNotIn('nixbox', (workflows / 'published-consumers.yml').read_text())
        self.assertNotIn('nixbox', (ROOT / 'scripts/published-consumers.sh').read_text())

    def test_every_workflow_bundle_option_is_a_bundle_directory(self):
        options = re.search(r'bundle:\n(?:.*\n)*?        options:\n((?:          - .+\n)+)',
                            (ROOT / '.github/workflows/propose-upgrades.yml').read_text()).group(1)
        names = re.findall(r'- (\S+)', options)
        self.assertEqual(names, ['all', *config.BUNDLES])
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
