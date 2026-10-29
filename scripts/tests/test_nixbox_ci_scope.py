"""nixbox is in the build, upgrade-proposal and cleanup machinery, never in publication."""
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

    def test_no_workflow_other_than_the_manual_build_builds_nixbox(self):
        for path in (ROOT / '.github/workflows').glob('*.yml'):
            text = path.read_text()
            if 'BUNDLE=nixbox' in text:
                self.assertEqual(path.name, 'nixbox-build.yml')


if __name__ == '__main__':
    unittest.main()
