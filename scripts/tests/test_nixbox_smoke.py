"""Run smoke.sh against a fake Docker: the matrix, mounts and failure handling."""
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
SMOKE = ROOT / 'bundles/nixbox/scripts/smoke.sh'
CONSUMER = ROOT / 'bundles/nixbox/scripts/consumer-smoke.sh'


@unittest.skipUnless(shutil.which('sh'), 'requires POSIX sh')
class SmokeTests(unittest.TestCase):
    def invoke(self, fail_consumer=None, **env):
        directory = Path(tempfile.mkdtemp())
        self.addCleanup(shutil.rmtree, directory, True)
        docker = directory / 'docker'
        docker.write_text('#!' + sys.executable + '''
import json, os, sys
args = sys.argv[1:]
with open(os.environ['CALLS'], 'a') as stream: stream.write(json.dumps(args) + '\\n')
if args[:2] == ['volume', 'create']:
    n = len(open(os.environ['CALLS']).read().splitlines())
    print('volume-%d' % n)
elif '/bin/cat' in args:
    print('root:x:0:0::/root:/bin/sh')
elif '/nixbox-smoke.sh' in args:
    fail = os.environ.get('FAIL_CONSUMER')
    if fail and any(a.startswith(fail) for a in args):
        print('RESULT 31 32 skipped=0 uid=0'); sys.exit(1)
    print('RESULT 32 32 skipped=0 uid=0')
''')
        docker.chmod(0o755)
        calls = directory / 'calls.jsonl'
        environment = dict(os.environ, PATH=str(directory) + os.pathsep + os.environ['PATH'], CALLS=str(calls))
        environment.pop('NIXBOX_PLATFORM', None)
        if fail_consumer:
            environment['FAIL_CONSUMER'] = fail_consumer
        environment.update(env)
        result = subprocess.run(['sh', str(SMOKE), 'example/nixbox:fixture', 'ignored/consumer'], env=environment, capture_output=True, text=True)
        return result, [json.loads(line) for line in calls.read_text().splitlines()]

    def test_matrix_mounts_and_unprivileged_run(self):
        result, calls = self.invoke(NIXBOX_TOOLBOX_IMAGE='example/toolbox@sha256:' + 'b' * 64)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        runs = [c for c in calls if c[0] == 'run']
        inits = [c for c in runs if '/nixbox-smoke.sh' not in c and '/bin/cat' not in c]
        self.assertEqual([c[-1] for c in inits], ['example/nixbox:fixture'] * 2 + ['example/toolbox@sha256:' + 'b' * 64])
        for call in inits:
            self.assertIn('none', call)
        consumers = [c for c in runs if '/nixbox-smoke.sh' in c]
        self.assertEqual(len(consumers), 5)
        images = [c[c.index('--entrypoint') + 2] for c in consumers]
        self.assertEqual(len(set(images[:4])), 4)
        for image in images:
            self.assertRegex(image, r'^[a-z/]+@sha256:[0-9a-f]{64}$')
        self.assertEqual(sorted(i.split('@')[0] for i in images[:4]), ['alpine', 'debian', 'fedora', 'opensuse/leap'])
        for call in consumers:
            self.assertIn('--read-only', call)
            self.assertTrue(any(re.fullmatch(r'type=volume,src=volume-\d+,dst=/nix,readonly,volume-nocopy', a) for a in call), call)
            self.assertTrue(any(re.fullmatch(r'type=volume,src=volume-\d+,dst=/opt/bdock/toolbox,readonly,volume-nocopy', a) for a in call))
        users = [c[c.index('--user') + 1] for c in consumers]
        self.assertEqual(users, ['0:0'] * 4 + ['1000:1000'])
        self.assertTrue(any('dst=/etc/passwd,readonly' in a for a in consumers[4]))
        self.assertIn('SMOKE native alpine-3.23 uid=0: 32 32', result.stdout)
        removed = [c for c in calls if c[:2] == ['volume', 'rm']]
        self.assertEqual(len(removed), 2)

    def test_alpine_consumer_simulates_gcompat_glibc_loader_stubs(self):
        result, calls = self.invoke()
        self.assertEqual(result.returncode, 0, result.stderr)
        consumers = [c for c in calls if '/nixbox-smoke.sh' in c]
        by_image = {c[c.index('--entrypoint') + 2].split('@')[0]: c for c in consumers[:4]}
        alpine = by_image['alpine']
        for loader in ('/lib/ld-linux-aarch64.so.1', '/lib/ld-linux-x86-64.so.1'):
            self.assertTrue(any(a.startswith('type=bind,src=') and a.endswith(',dst=%s,readonly' % loader) for a in alpine), loader)
        self.assertIn('NIXBOX_GCOMPAT_SIM=1', alpine)
        for image in ('debian', 'fedora', 'opensuse/leap'):
            self.assertFalse(any('ld-linux' in a for a in by_image[image]), image)
            self.assertNotIn('NIXBOX_GCOMPAT_SIM=1', by_image[image])

    def test_consumer_script_checks_the_auto_mode_path_per_libc(self):
        text = CONSUMER.read_text()
        self.assertIn('auto mode runs the toolbox binary directly on a glibc host', text)
        self.assertIn('auto mode uses the Nix loader on musl', text)
        self.assertIn('NIXBOX_GCOMPAT_SIM', text)
        self.assertIn('a user symlink to a shim finds the profile', text)

    def test_platform_and_offline_are_forwarded(self):
        result, calls = self.invoke(NIXBOX_PLATFORM='linux/amd64', NIXBOX_OFFLINE='1')
        self.assertEqual(result.returncode, 0, result.stderr)
        for call in (c for c in calls if c[0] == 'run'):
            self.assertEqual(call[call.index('--platform') + 1], 'linux/amd64')
        consumer = next(c for c in calls if '/nixbox-smoke.sh' in c)
        self.assertEqual(consumer[consumer.index('--network') + 1], 'none')
        self.assertIn('NIXBOX_OFFLINE=1', consumer)
        self.assertIn('SMOKE linux/amd64 ', result.stdout)

    def test_without_a_toolbox_image_no_toolbox_mount_is_made(self):
        result, calls = self.invoke()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertFalse(any('dst=/opt/bdock/toolbox' in a for c in calls for a in c))

    def test_one_failing_consumer_fails_the_run_but_runs_the_rest(self):
        result, calls = self.invoke(fail_consumer='fedora')
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(len([c for c in calls if '/nixbox-smoke.sh' in c]), 5)
        self.assertIn('fedora-44 uid=0: 31 32', result.stdout)

    def test_consumer_script_assumes_only_posix_tools_and_the_profile(self):
        text = CONSUMER.read_text()
        for tool in ('diff ', 'cmp ', 'seq ', 'curl ', 'wget ', 'python'):
            self.assertNotRegex(text.replace('git diff', ''), r'(^|[|&;( ])' + re.escape(tool), tool)
        self.assertEqual(text.splitlines()[0], '#!/bin/sh')
        for tool in ('ast-grep', 'delta', 'difft', 'fastfetch', 'leaf', 'zizmor', 'zk', 'hx'):
            self.assertIn('"%s [$mode]"' % tool, text)
        self.assertIn('auto always', text)


if __name__ == '__main__':
    unittest.main()
