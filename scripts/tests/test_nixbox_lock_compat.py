"""The nixbox updater may only propose a lock with the same package list."""
import copy
import importlib.util
import json
from pathlib import Path
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[2]
SCRIPT = ROOT / 'scripts/nixbox-lock-compat.py'
spec = importlib.util.spec_from_file_location('compat', SCRIPT)
compat = importlib.util.module_from_spec(spec)
spec.loader.exec_module(compat)
KEY = 'cache.nixos.org-1:'


def store(hash_, name):
    return '/nix/store/' + hash_ * 32 + '-' + name


def platform(hash_, glibc='glibc-2.44-57', git='git-minimal-2.56.0'):
    paths = {store(hash_, glibc): {}, store(hash_, git): {}, store(hash_, 'zstd-1.5.7-bin'): {}}
    for path in paths:
        paths[path] = {'narHash': 'sha256-' + hash_ * 8, 'narSize': 5, 'references': [path],
                       'signatures': [KEY + hash_ * 16]}
    return {'system': 'x86_64-linux',
            'roots': {'glibc': store(hash_, glibc), 'git': store(hash_, git), 'zstd': store(hash_, 'zstd-1.5.7-bin')},
            'paths': paths}


def lock(hash_='a', **kw):
    return {'schema': 1, 'catalog_digest': 'c' * 64,
            'platforms': {'amd64': platform(hash_, **kw), 'arm64': platform(hash_, **kw)}}


class CompatTests(unittest.TestCase):
    def check(self, old, new):
        return compat.compare(old, new)

    def test_identical_and_rolled_forward_locks_are_compatible(self):
        self.assertEqual(self.check(lock(), lock()), [])
        # New nixpkgs revision: other hashes, narHashes, signatures and versions.
        new = lock('b', glibc='glibc-2.45-1', git='git-minimal-2.57.0')
        self.assertEqual(self.check(lock(), new), [])

    def rejects(self, new, text, old=None):
        problems = self.check(old or lock(), new)
        self.assertTrue(any(text in p for p in problems), problems)

    def test_package_list_changes_are_rejected(self):
        added = lock()
        added['platforms']['amd64']['paths'][store('d', 'extra-1.0')] = {
            'narHash': 'x', 'narSize': 1, 'references': [], 'signatures': [KEY + 'x']}
        self.rejects(added, 'package list')
        removed = lock()
        del removed['platforms']['arm64']['paths'][store('a', 'zstd-1.5.7-bin')]
        self.rejects(removed, 'package list')
        renamed = lock()
        renamed['platforms']['amd64']['roots']['zstd'] = store('a', 'zstd-1.5.7-bin')
        renamed['platforms']['amd64']['roots']['less'] = store('a', 'less-710')
        self.rejects(renamed, 'package list')

    def test_catalog_schema_platform_and_key_changes_are_rejected(self):
        for field, value, text in (('catalog_digest', 'd' * 64, 'catalog'), ('schema', 2, 'schema')):
            new = lock(); new[field] = value
            self.rejects(new, text)
        new = lock(); new['platforms']['amd64']['system'] = 'aarch64-linux'; self.rejects(new, 'platform')
        new = lock(); del new['platforms']['arm64']; self.rejects(new, 'platform')
        new = lock()
        for info in new['platforms']['amd64']['paths'].values():
            info['signatures'] = ['other-key-1:' + 'z' * 16]
        self.rejects(new, 'key')
        new = lock()
        next(iter(new['platforms']['amd64']['paths'].values()))['signatures'] = []
        self.rejects(new, 'signature')

    def run_cli(self, old, new):
        with tempfile.TemporaryDirectory() as directory:
            paths = []
            for name, value in (('old', old), ('new', new)):
                path = Path(directory) / (name + '.json')
                path.write_text(json.dumps(value))
                paths.append(str(path))
            return subprocess.run(['python3', '-B', str(SCRIPT), *paths], capture_output=True, text=True)

    def test_cli_exit_status_and_clear_message(self):
        ok = self.run_cli(lock(), lock('b'))
        self.assertEqual(ok.returncode, 0, ok.stderr)
        bad = lock(); bad['catalog_digest'] = 'd' * 64
        result = self.run_cli(lock(), bad)
        self.assertEqual(result.returncode, 1)
        self.assertIn('Refusing nixbox proposal:', result.stderr)
        self.assertIn('explicit owner approval', result.stderr)

    def test_the_committed_lock_is_compatible_with_itself(self):
        real = json.loads((ROOT / 'bundles/nixbox/packages.lock.json').read_text())
        self.assertEqual(self.check(real, copy.deepcopy(real)), [])


if __name__ == '__main__':
    unittest.main()
