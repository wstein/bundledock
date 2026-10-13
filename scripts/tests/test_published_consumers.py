"""Source-only orchestration checks; the fixture never runs Docker."""
import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[2]
DIGESTS = {
    'toolbox': '40bae37ef7fd7b8e7131803850801ed41cfb23a6ef774bc2e40f95ee81e59a1c',
    'node': '295cce35e9f580db41dfdc97bd30243d9f0cf388556a8a8d87a6c59ee51e6a8c',
    'codex': 'eb5ec6514b7476c55d1d2c0c1def871b982026f0a75144ddab33e9af4a3f40ec',
    'agy': 'a96463845e3948487029f72ef1a242996b986c4bc50e42d68707308652284496',
    'claude': '4b02222a4954f174a46af2a7ef732c2f0ff7b7bb7b462875491dd80aabf1940f',
}
CONSUMER = 'mcr.microsoft.com/devcontainers/go@sha256:8f1c83128e4bce9d745b1dc0f28f8c027e303d3886e2434966ff135296c41770'


class PublishedConsumersTests(unittest.TestCase):
    def invoke(self, args=(), failure='', archive=False, corrupt=False):
        with tempfile.TemporaryDirectory() as temporary:
            directory = Path(temporary)
            log = directory / 'calls'
            docker = directory / 'docker'
            docker.write_text('''#!/usr/bin/env python3
import json, os, sys
args = sys.argv[1:]
with open(os.environ['CALL_LOG'], 'a') as f: f.write(json.dumps(args)+'\\n')
if args[0] == 'pull' and os.environ.get('FAIL_PULL') in args[1] and os.environ.get('FAIL_PULL'): sys.exit(1)
if '--help' in args and os.environ.get('ARCHIVE')=='1': print('  -list-licenses')
if '--license' in args and os.environ.get('CORRUPT')=='1': sys.exit(1)
if args[:2] == ['image', 'inspect']: print('sha256:' + 64 * 'a')
if args[:2] == ['volume', 'create']:
    print(args[2] if len(args) > 2 else 'fixture-' + str(sum(1 for line in open(os.environ['CALL_LOG']) if json.loads(line)[:2] == ['volume', 'create'])))
''')
            docker.chmod(0o755)
            env = dict(os.environ, PATH=str(directory)+':'+os.environ['PATH'], CALL_LOG=str(log), FAIL_PULL=failure, ARCHIVE=str(int(archive)), CORRUPT=str(int(corrupt)))
            result = subprocess.run(['sh', str(ROOT/'scripts/published-consumers.sh'), *args], env=env, capture_output=True, text=True)
            calls = [json.loads(line) for line in log.read_text().splitlines()] if log.exists() else []
            return result, calls

    def test_all_pulls_precede_real_helper_initializers_and_shared_consumer(self):
        result, calls = self.invoke()
        self.assertEqual(result.returncode, 0, result.stderr)
        images = ['quay.io/bundledock/'+name+'@sha256:'+digest for name, digest in DIGESTS.items()]
        self.assertEqual(calls[:6], [['pull', image] for image in [*images, CONSUMER]])
        runs = [call for call in calls if call[0] == 'run' and '--help' not in call]
        self.assertEqual(len(runs), 8)
        for index, (name, call) in enumerate(zip(DIGESTS, runs[:5])):
            volume = 'fixture-' + str(2*index+1)
            self.assertIn('type=volume,src='+volume+',dst=/bundle,volume-nocopy', call)
            self.assertIn('sha256:' + 64 * 'a', call)
            self.assertNotIn('--source', call)
            self.assertNotIn('--entrypoint', call)
            self.assertIn('none', call)
        consumer = runs[-1]
        self.assertEqual(runs[-2][runs[-2].index('--user')+1], '1000:1000')
        self.assertIn('VERIFY_PASS=1', runs[-2])
        self.assertIn('VERIFY_PASS=2', consumer)
        self.assertIn('type=volume,src=fixture-11,dst=/home/vscode,volume-nocopy', runs[-2])
        self.assertIn(CONSUMER, consumer)
        self.assertIn('1000:1000', consumer)
        self.assertIn('type=volume,src=fixture-11,dst=/home/vscode,volume-nocopy', consumer)
        self.assertIn('0:0', runs[-3])
        self.assertIn('chown 1000:1000 /home/vscode', runs[-3])
        self.assertIn('HOME=/home/vscode', consumer)
        for index, name in enumerate(DIGESTS):
            volume = 'fixture-' + str(2*index+1)
            self.assertIn('type=volume,src='+volume+',dst=/opt/bdock/'+name+',readonly,volume-nocopy', consumer)
        self.assertEqual(consumer[consumer.index('--network')+1], 'none')
        self.assertEqual(len([call for call in calls if call[:2] == ['volume', 'rm']]), 6)

    def test_actual_notice_checks_accept_legacy_and_require_both_archive_files(self):
        source = (ROOT/'scripts/published-consumer-check.sh').read_text()
        start = source.index('for name in agy claude; do')
        end = source.index('for name in node npm npx; do', start)
        for layout, succeeds in [('legacy', True), ('archive', True), ('archive-only', False), ('index-only', False)]:
            with self.subTest(layout=layout), tempfile.TemporaryDirectory() as temporary:
                directory = Path(temporary)
                for name in ('agy', 'claude'):
                    bundle = directory/name
                    notice = bundle/'licenses'/name/'SOURCE'
                    notice.parent.mkdir(parents=True)
                    notice.write_text('historical original SOURCE')
                    if layout in ('archive', 'archive-only'):
                        (bundle/'licenses.tar.gz').write_bytes(b'fixture')
                    if layout in ('archive', 'index-only'):
                        (bundle/'licenses.index.json').write_text('{}')
                    if layout == 'archive':
                        notice.unlink()
                checks = source[start:end].replace('/opt/bdock/', str(directory)+'/')
                result = subprocess.run(['sh', '-ec', checks], capture_output=True, text=True)
                self.assertEqual(result.returncode == 0, succeeds, result.stderr)

    def test_new_carrier_reads_source_and_corruption_stops_consumers(self):
        result, calls = self.invoke(archive=True)
        self.assertEqual(result.returncode, 0, result.stderr)
        readers = [call for call in calls if '--license' in call]
        self.assertEqual([call[-1] for call in readers], ['licenses/agy/SOURCE', 'licenses/claude/SOURCE'])
        self.assertTrue(all(call[call.index('--network')+1]=='none' for call in readers))
        result, calls = self.invoke(archive=True, corrupt=True)
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse(any('--user' in call for call in calls))

    def test_no_missing_or_wrong_image_overrides(self):
        for args in [('quay.io/bundledock/codex:latest',), ('quay.io/bundledock/codex@sha256:'+'a'*64,), ('',)]:
            with self.subTest(args=args):
                result, calls = self.invoke(args)
                self.assertNotEqual(result.returncode, 0)
                self.assertEqual(calls, [])

    def test_failed_pull_prevents_any_initialization(self):
        result, calls = self.invoke(failure='/claude@')
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse(any(call[0] != 'pull' for call in calls))



if __name__ == '__main__':
    unittest.main()
