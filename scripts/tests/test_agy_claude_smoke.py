"""Source-only adapters: fake Docker records orchestration, never builds images."""
import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[2]


class AgentSmokeTests(unittest.TestCase):
    def test_native_smoke_isolated_readonly_consumers(self):
        for name in ('agy', 'claude'):
            with self.subTest(bundle=name), tempfile.TemporaryDirectory() as directory:
                directory = Path(directory)
                log = directory / 'calls'
                docker = directory / 'docker'
                docker.write_text('''#!/usr/bin/env python3
import json,os,sys
with open(os.environ['CALL_LOG'],'a') as f:f.write(json.dumps(sys.argv[1:])+'\\n')
if sys.argv[1:3]==['image','inspect']:print('sha256:'+64*'a')
if sys.argv[1:3]==['volume','create']:print('fixture-volume')
''')
                docker.chmod(0o755)
                env = dict(os.environ, PATH=str(directory)+':'+os.environ['PATH'], CALL_LOG=str(log))
                subprocess.run(['sh', str(ROOT/'bundles'/name/'scripts'/'smoke.sh')], cwd=directory, env=env, check=True)
                calls = [json.loads(line) for line in log.read_text().splitlines()]
                runs = [call for call in calls if call[0] == 'run']
                self.assertEqual(len(runs), 5)
                for call in runs:
                    self.assertIn('none', call)
                readers = [call for call in runs if '--license' in call]
                self.assertEqual(len(readers), 1)
                self.assertEqual(readers[0][-2:], ['--license', 'licenses/'+name+'/SOURCE'])
                initializers = [call for call in runs if '--user' not in call and '--license' not in call]
                self.assertEqual(len(initializers), 2)
                for call in initializers:
                    self.assertIn('type=volume,src=fixture-volume,dst=/bundle,volume-nocopy', call)
                consumers = [call for call in runs if '--user' in call]
                self.assertEqual(len(consumers), 2)
                for call in consumers:
                    self.assertIn('1000:1000', call)
                    self.assertIn('HOME=/home/bundledock', call)
                    self.assertIn('DISABLE_UPDATES=1', call)
                    self.assertIn('DISABLE_AUTOUPDATER=1', call)
                    self.assertIn('type=volume,src=fixture-volume,dst=/opt/bdock/'+name+',readonly,volume-nocopy', call)
                    source = ROOT/'bundles'/name/'scripts'/'consumer-smoke.sh'
                    self.assertTrue(source.is_file())
                    self.assertIn('type=bind,src='+str(source)+',dst=/'+name+'-smoke.sh,readonly', call)
