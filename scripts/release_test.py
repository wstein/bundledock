"""Exercise release packaging with a fake compiler; no Go artifacts are built."""
import hashlib
import json
import os
from pathlib import Path
import subprocess
import tarfile
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[1]


class ReleasePackagingTest(unittest.TestCase):
    def test_versioned_archives_and_compiler_contract(self):
        with tempfile.TemporaryDirectory() as directory:
            base = Path(directory)
            compiler = base / 'go'
            compiler.write_text('''#!/usr/bin/env python3
import json, os, pathlib, sys
if sys.argv[1:] == ['version']:
    print('go version fixture linux/arm64')
else:
    with open(os.environ['CALL_LOG'], 'a') as log:
        log.write(json.dumps(dict(args=sys.argv[1:], env={k:os.environ[k] for k in ('CGO_ENABLED','GOOS','GOARCH','GOAMD64','GOARM64','GOFLAGS')}))+'\\n')
    output=pathlib.Path(sys.argv[sys.argv.index('-o')+1])
    output.write_bytes(('fixture-'+os.environ['GOARCH']).encode())
    output.chmod(0o755)
''')
            compiler.chmod(0o755)
            log = base / 'calls'
            env = dict(os.environ, GO=str(compiler), VERSION='0.1.0', CALL_LOG=str(log), GOFLAGS='-race')
            outputs = []
            for attempt in ('first', 'second'):
                destination = base / attempt
                result = subprocess.run(['sh', 'scripts/release.sh'], cwd=ROOT,
                                        env=dict(env, RELEASE_DIR=str(destination)), capture_output=True, text=True)
                self.assertEqual(result.returncode, 0, result.stderr)
                outputs.append(destination)
                names = {'bdock_0.1.0_linux_'+arch+'.tar.gz' for arch in ('amd64', 'arm64')}
                self.assertEqual({p.name for p in destination.iterdir()}, names | {'LICENSE', 'PROVENANCE', 'SHA256SUMS'})
                for arch in ('amd64', 'arm64'):
                    with tarfile.open(destination / ('bdock_0.1.0_linux_'+arch+'.tar.gz')) as archive:
                        self.assertEqual(set(archive.getnames()), {'bdock', 'LICENSE', 'PROVENANCE'})
                        self.assertEqual(archive.extractfile('bdock').read(), ('fixture-'+arch).encode())
                        self.assertTrue(archive.getmember('bdock').mode & 0o111)
                        provenance = archive.extractfile('PROVENANCE').read().decode()
                        self.assertIn('GOARCH='+arch+'\n', provenance)
                        self.assertIn('version=0.1.0\n', provenance)
                        self.assertIn('command=bdock\n', provenance)
                        self.assertEqual(archive.extractfile('LICENSE').read(), (ROOT / 'LICENSE').read_bytes())
                for line in (destination / 'SHA256SUMS').read_text().splitlines():
                    digest, name = line.split('  ')
                    self.assertEqual(hashlib.sha256((destination / name).read_bytes()).hexdigest(), digest)
            for p in outputs[0].iterdir():
                self.assertEqual(p.read_bytes(), (outputs[1] / p.name).read_bytes())
            calls = [json.loads(line) for line in log.read_text().splitlines()]
            self.assertEqual(len(calls), 4)
            for call, arch in zip(calls, ('amd64', 'arm64') * 2):
                self.assertEqual(call['args'][-1], './cmd/bdock')
                self.assertIn('-trimpath', call['args'])
                self.assertIn('-X main.version=0.1.0', call['args'])
                self.assertEqual(call['env'], dict(CGO_ENABLED='0', GOOS='linux', GOARCH=arch, GOAMD64='v1', GOARM64='v8.0', GOFLAGS=''))
            result = subprocess.run(['sh', 'scripts/release.sh'], cwd=ROOT,
                                    env=dict(env, RELEASE_DIR=str(outputs[0])), capture_output=True)
            self.assertNotEqual(result.returncode, 0)
            self.assertEqual(len(log.read_text().splitlines()), 4)
