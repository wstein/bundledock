"""Exercise the actual consumer script using stand-ins, never OpenTofu or Docker."""
import json
import os
from pathlib import Path
import shlex
import shutil
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[2]


@unittest.skipUnless(shutil.which('sh'), 'requires POSIX sh')
class OpenTofuSmokeTests(unittest.TestCase):
    def invoke(self, failure='', trace=False):
        with tempfile.TemporaryDirectory() as directory:
            directory = Path(directory)
            home = directory / 'writable home'
            home.mkdir()
            payload = directory / 'readonly payload'
            for name in ('.ready', 'licenses.tar.gz', 'licenses.index.json'):
                target = payload / name
                target.parent.mkdir(parents=True, exist_ok=True)
                target.write_text('fixture')
            tools = directory / 'adapters'
            tools.mkdir()
            mkdir = tools / 'mkdir'
            mkdir.write_text('''#!/usr/bin/env python3
import os,sys
from pathlib import Path
for name in sys.argv[1:]:
 if name!='-p': Path(name).mkdir(parents=True,exist_ok=True)
if os.environ['FAILURE']=='probe' and os.environ.get('TF_DATA_DIR') in sys.argv[1:]:
 (Path(os.environ['TF_DATA_DIR'])/'.bundledock-write-test').mkdir()
''')
            mkdir.chmod(0o755)
            tofu = tools / 'tofu'
            tofu.write_text('''#!/usr/bin/env python3
import json,os,sys
from pathlib import Path
args=sys.argv[1:]
with open(os.environ['CALLS'],'a') as stream: stream.write(json.dumps(args)+'\\n')
# Built-in-only init need not create TF_DATA_DIR: no provider or module downloads.
if args[0]=='apply' and os.environ['FAILURE']!='state': Path('terraform.tfstate').write_text('{}')
if args[0]=='apply' and os.environ['FAILURE']=='data':
 p=Path(os.environ['TF_DATA_DIR'])
 if p.exists(): p.rmdir()
if args[0]=='apply' and os.environ['FAILURE']=='config': Path(os.environ['HOME']+'/.tofurc').write_text('changed')
if args[0]=='output': print('wrong' if os.environ['FAILURE']=='output' else 'bundledock-offline',end='')
''')
            tofu.chmod(0o755)
            touch = tools / 'touch'
            touch.write_text('#!/bin/sh\n[ "$FAILURE" = readonly ]\n')
            touch.chmod(0o755)
            source = (ROOT / 'bundles/opentofu/scripts/consumer-smoke.sh').read_text()
            self.assertEqual(source.count('root=/opt/bdock/opentofu\n'), 1)
            source = source.replace('root=/opt/bdock/opentofu\n',
                                    'root=' + shlex.quote(str(payload)) + '\n')
            script = directory / 'consumer.sh'
            script.write_text(source)
            calls = directory / 'calls'
            env = dict(os.environ, HOME=str(home), PATH=str(tools)+':'+os.environ['PATH'],
                       CALLS=str(calls), FAILURE=failure)
            result = subprocess.run(['sh', *(['-x'] if trace else []), str(script)], env=env, capture_output=True, text=True)
            return result, [json.loads(line) for line in calls.read_text().splitlines()] if calls.exists() else []

    def test_builtin_only_init_does_not_need_to_materialize_data_directory(self):
        result, calls = self.invoke(trace=True)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertIn(['init', '-backend=false', '-input=false'], calls)
        self.assertIn(['apply', '-input=false', 'fixture.plan'], calls)
        self.assertIn(['output', '-raw', 'fixture'], calls)

    def test_carrier_verifies_notices_before_readonly_consumers(self):
        with tempfile.TemporaryDirectory() as directory:
            directory = Path(directory)
            calls = directory / 'docker-calls'
            adapter = directory / 'docker'
            adapter.write_text("#!/usr/bin/env python3\nimport json,os,sys\nargs=sys.argv[1:]\nwith open(os.environ['CALLS'],'a') as stream: stream.write(json.dumps(args)+'\\n')\nif args[:2]==['image','inspect']: print('sha256:fixture')\nif args[:2]==['volume','create']: print('fixture-volume')\nif '--license' in args and os.environ.get('FAIL_NOTICE'): sys.exit(1)\n")
            adapter.chmod(0o755)
            env = dict(os.environ, PATH=str(directory)+':'+os.environ['PATH'], CALLS=str(calls))
            script = ROOT / 'bundles/opentofu/scripts/smoke.sh'
            result = subprocess.run(['sh', str(script), 'local:fixture'], env=env, capture_output=True, text=True)
            self.assertEqual(result.returncode, 0, result.stderr)
            commands = [json.loads(line) for line in calls.read_text().splitlines()]
            members = [args[args.index('--license')+1] for args in commands if '--license' in args]
            self.assertEqual(members, ['licenses/opentofu/'+name for name in ('LICENSE', 'README.md', 'CHANGELOG.md', 'SOURCE')])
            self.assertTrue(all('--network' in args and args[args.index('--network')+1]=='none'
                                for args in commands if '--license' in args))
            env['FAIL_NOTICE'] = '1'
            result = subprocess.run(['sh', str(script), 'local:fixture'], env=env, capture_output=True, text=True)
            self.assertNotEqual(result.returncode, 0)

    def test_post_apply_assertions_fail_with_bounded_stage_diagnostics(self):
        for failure, diagnostic in [('probe','data directory writability'), ('output','fixture output'), ('state','local state'),
                                    ('data','data directory'), ('config','consumer configuration'),
                                    ('readonly','read-only bundle')]:
            with self.subTest(failure=failure):
                result, _ = self.invoke(failure)
                self.assertNotEqual(result.returncode, 0)
                self.assertIn(diagnostic, result.stderr)
