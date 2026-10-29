"""Check the nixbox lock refresh script with a fake Docker; never builds or fetches."""
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import sys
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[2]
SCRIPTS = ROOT / 'bundles/nixbox/scripts'
PINNED = re.compile(r'^[a-z0-9./_-]+(:[A-Za-z0-9._-]+)?@sha256:[0-9a-f]{64}$')


@unittest.skipUnless(shutil.which('sh'), 'requires POSIX sh')
class RefreshLockTests(unittest.TestCase):
    def run_script(self):
        with tempfile.TemporaryDirectory() as directory:
            directory = Path(directory)
            repo = directory / 'repo'
            shutil.copytree(ROOT / 'bundles/nixbox', repo / 'bundles/nixbox', ignore=shutil.ignore_patterns('packages.lock.json'))
            adapters = directory / 'bin'
            adapters.mkdir()
            calls = directory / 'calls.jsonl'
            docker = adapters / 'docker'
            docker.write_text('#!' + sys.executable + '''
import json, os, re, sys
args = sys.argv[1:]
with open(os.environ['CALLS'], 'a') as stream: stream.write(json.dumps(args) + '\\n')
for arg in args:
    m = re.match(r'type=bind,src=(.*),dst=/(work|out)$', arg)
    if m and m.group(2) == 'work':
        open(os.path.join(m.group(1), 'packages.lock.json'), 'w').write('{"fixture": true}\\n')
''')
            docker.chmod(0o755)
            result = subprocess.run(['sh', str(repo / 'bundles/nixbox/scripts/refresh-lock.sh')],
                                    env=dict(os.environ, PATH=str(adapters) + os.pathsep + os.environ['PATH'], CALLS=str(calls)),
                                    capture_output=True, text=True)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            lines = [json.loads(line) for line in calls.read_text().splitlines()]
            lock = (repo / 'bundles/nixbox/packages.lock.json').read_text()
            return lines, lock

    def test_images_are_digest_pinned_and_the_source_is_read_only(self):
        calls, lock = self.run_script()
        self.assertEqual(len(calls), 2)
        for call in calls:
            self.assertEqual(call[0], 'run')
            images = [a for a in call if '@sha256:' in a and not a.startswith('type=')]
            self.assertEqual(len(images), 1, call)
            self.assertRegex(images[0], PINNED)
            self.assertTrue(any(a.startswith('type=bind,src=') and a.endswith('/src,readonly') for a in call), call)
        self.assertIn('nixos/nix@', ' '.join(calls[1]))
        self.assertIn('--network', calls[0])
        self.assertEqual(calls[0][calls[0].index('--network') + 1], 'none')
        self.assertEqual(lock, '{"fixture": true}\n')

    def test_inner_script_trusts_no_signature_and_builds_nothing(self):
        text = (SCRIPTS / 'refresh-lock-inner.sh').read_text()
        self.assertIn('--store https://cache.nixos.org', text)
        self.assertNotIn('nix build', text)
        self.assertNotIn('nix-build', text)
        self.assertNotIn('--no-require-sigs', text)
        self.assertIn('nixbox" lock', text)

    def test_scripts_pin_the_same_nix_image_as_the_dockerfile_when_present(self):
        dockerfile = ROOT / 'bundles/nixbox/Dockerfile'
        if not dockerfile.exists():
            self.skipTest('Dockerfile not written yet')
        pins = set(re.findall(r'nixos/nix@sha256:[0-9a-f]{64}', dockerfile.read_text()))
        refresh = set(re.findall(r'nixos/nix@sha256:[0-9a-f]{64}', (SCRIPTS / 'refresh-lock.sh').read_text()))
        self.assertEqual(pins, refresh)


if __name__ == '__main__':
    unittest.main()
