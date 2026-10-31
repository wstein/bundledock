"""The nixbox license record covers exactly the locked closure and never guesses."""
import json
from pathlib import Path
import unittest

ROOT = Path(__file__).resolve().parents[2]
NIXBOX = ROOT / 'bundles/nixbox'
UNDETERMINED = {'xgcc-16.2.0-libgcc', 'gmp-with-cxx-6.3.0', 'util-linux-minimal-2.42.4-login'}


class NixboxLicenseTests(unittest.TestCase):
    def setUp(self):
        self.lock = json.loads((NIXBOX / 'packages.lock.json').read_text())
        self.record = json.loads((NIXBOX / 'licenses.json').read_text())
        self.catalog = json.loads((NIXBOX / 'packages.catalog.json').read_text())

    def test_record_covers_every_closure_path_of_the_lock(self):
        # The record keeps the nixpkgs revision it was generated from. A rolling
        # nixpkgs bump that leaves the closure's store paths unchanged (the
        # updater PR changes only the catalog and lock) stays valid, because each
        # path is matched by its output path; a changed path list must fail here
        # until the record is regenerated with the owner's approval.
        self.assertRegex(self.record['nixpkgs_rev'], r'\A[0-9a-f]{40}\Z')
        self.assertEqual(sorted(self.record['platforms']), sorted(self.lock['platforms']))
        for arch, platform in self.lock['platforms'].items():
            self.assertEqual(sorted(self.record['platforms'][arch]), sorted(platform['paths']), arch)

    def test_every_path_has_license_names_or_an_explicit_null(self):
        for arch, paths in self.record['platforms'].items():
            for path, entry in paths.items():
                with self.subTest(arch=arch, path=path):
                    if entry['license'] is None:
                        self.assertIsNone(entry['attr'])
                    else:
                        self.assertTrue(entry['attr'])
                        self.assertTrue(entry['license'] and all(isinstance(name, str) and name for name in entry['license']))

    def test_undetermined_paths_are_named_in_the_guide(self):
        guide = (ROOT / 'docs/nixbox.md').read_text()
        undetermined = {path.split('-', 1)[1] for paths in self.record['platforms'].values()
                        for path, entry in paths.items() if entry['license'] is None}
        self.assertEqual(undetermined, UNDETERMINED)
        for name in undetermined:
            self.assertIn(name, guide)

    def test_glibc_runner_license_is_recorded(self):
        for arch, paths in self.record['platforms'].items():
            glibc = [entry for path, entry in paths.items() if path.split('-', 1)[1].startswith('glibc-')]
            self.assertEqual([entry['license'] for entry in glibc], [['LGPL-2.0-or-later']], arch)


if __name__ == '__main__':
    unittest.main()
