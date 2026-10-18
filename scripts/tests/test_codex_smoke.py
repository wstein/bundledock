"""Source-only adapters: fake Docker records orchestration, never builds images."""
import json
import os
import shlex
from pathlib import Path
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[2]


class AgentSmokeTests(unittest.TestCase):
    def test_native_smoke_isolated_readonly_consumers(self):
        for name in ('codex',):
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
                self.assertEqual(len(runs), 4)
                for call in runs:
                    self.assertIn('none', call)
                initializers = [call for call in runs if '--user' not in call]
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

    def test_consumer_prepares_codex_home_before_invocation(self):
        for existing_config in (False, True):
            for has_zsh in (False, True):
                with self.subTest(existing_config=existing_config, has_zsh=has_zsh), tempfile.TemporaryDirectory() as directory:
                    directory = Path(directory)
                    home = directory / 'writable home'
                    home.mkdir()
                    codex_home = home / '.codex'
                    if existing_config:
                        codex_home.mkdir()
                        (codex_home / 'config.toml').write_text('model = "fixture-model"\n')
                    payload = directory / 'readonly payload'
                    files = ['.ready', 'codex-package.json', 'bin/codex-code-mode-host',
                             'codex-path/rg', 'codex-resources/bwrap']
                    if has_zsh:
                        files.append('codex-resources/zsh/bin/zsh')
                    for name in files:
                        target = payload / name
                        target.parent.mkdir(parents=True, exist_ok=True)
                        target.write_text('fixture')
                        target.chmod(0o555)
                    codex = payload / 'bin/codex'
                    codex.write_text('''#!/bin/sh
set -eu
[ "$CODEX_HOME" = "$HOME/.codex" ]
[ -d "$CODEX_HOME" ] || { echo 'CODEX_HOME does not exist' >&2; exit 42; }
printf '%s\\n' "$*" >> "$CODEX_HOME/calls"
''')
                    codex.chmod(0o555)
                    # Model the read-only payload check without mounting a real image.
                    tools = directory / 'tools'
                    tools.mkdir()
                    touch = tools / 'touch'
                    touch.write_text('#!/bin/sh\nexit 1\n')
                    touch.chmod(0o755)
                    source = (ROOT / 'bundles/codex/scripts/consumer-smoke.sh').read_text()
                    self.assertEqual(source.count('root=/opt/bdock/codex\n'), 1)
                    source = source.replace('root=/opt/bdock/codex\n',
                                            'root=' + shlex.quote(str(payload)) + '\n')
                    script = directory / 'consumer-smoke.sh'
                    script.write_text(source)
                    before = {str(p.relative_to(payload)): p.read_bytes()
                              for p in payload.rglob('*') if p.is_file()}
                    env = dict(os.environ, HOME=str(home), PATH=str(tools)+':'+os.environ['PATH'])
                    result = subprocess.run(['sh', str(script)], env=env, capture_output=True, text=True)
                    self.assertEqual(result.returncode, 0, result.stderr)
                    self.assertEqual((codex_home / 'calls').read_text().splitlines(), [
                        '--version', 'app-server daemon start', 'app-server daemon version',
                        'app-server daemon stop', 'app-server daemon stop'])
                    if existing_config:
                        self.assertEqual((codex_home / 'config.toml').read_text(), 'model = "fixture-model"\n')
                    after = {str(p.relative_to(payload)): p.read_bytes()
                             for p in payload.rglob('*') if p.is_file()}
                    self.assertEqual(before, after)
