"""Exercise release packaging with a fake compiler; no Go artifacts are built."""
import hashlib
import io
import json
import os
from pathlib import Path
import shutil
import subprocess
import tarfile
import tempfile
import unittest
from unittest import mock

try:
    import tomllib
except ModuleNotFoundError:  # Python < 3.11
    tomllib = None

def _gnu_tar_path():
    """Path of a GNU tar on PATH as `tar` or Homebrew's `gtar`, else None (mirrors scripts/release.sh)."""
    for name in ('tar', 'gtar'):
        path = shutil.which(name)
        if path is None:
            continue
        try:
            out = subprocess.run([path, '--version'], capture_output=True, text=True).stdout
        except OSError:
            continue
        if 'GNU tar' in out:
            return path
    return None


def _has_gnu_tar():
    return _gnu_tar_path() is not None


# scripts/release.sh refuses any tar that is not GNU tar (`tar` or `gtar`), by design.
requires_gnu_tar = unittest.skipUnless(_has_gnu_tar(), 'scripts/release.sh requires GNU tar (tar or gtar), which is not on PATH')

ROOT = Path(__file__).resolve().parents[1]
# Sorted by archive name; darwin/amd64 (end of life) and Windows are not built.
TARGETS = (('darwin', 'arm64'), ('linux', 'amd64'), ('linux', 'arm64'))


class ReleasePackagingTest(unittest.TestCase):
    @requires_gnu_tar
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
    output.write_bytes(('fixture-'+os.environ['GOOS']+'-'+os.environ['GOARCH']).encode())
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
                names = {'bdock_0.1.0_%s_%s.tar.gz' % t for t in TARGETS}
                self.assertEqual({p.name for p in destination.iterdir()}, names | {'LICENSE', 'PROVENANCE', 'SHA256SUMS'})
                for goos, arch in TARGETS:
                    with tarfile.open(destination / ('bdock_0.1.0_%s_%s.tar.gz' % (goos, arch))) as archive:
                        self.assertEqual(set(archive.getnames()), {'bdock', 'LICENSE', 'PROVENANCE'})
                        self.assertEqual(archive.extractfile('bdock').read(), ('fixture-%s-%s' % (goos, arch)).encode())
                        self.assertEqual(archive.getmember('bdock').mode, 0o755)
                        for member in archive.getmembers():
                            self.assertEqual((member.uid, member.gid, member.mtime), (0, 0, 0))
                            self.assertEqual(member.mode, 0o755 if member.name == 'bdock' else 0o644)
                        self.assertEqual(archive.getnames(), ['bdock', 'LICENSE', 'PROVENANCE'])
                        provenance = archive.extractfile('PROVENANCE').read().decode()
                        self.assertIn('GOOS='+goos+'\n', provenance)
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
            self.assertEqual(len(calls), 6)
            for call, (goos, arch) in zip(calls, TARGETS * 2):
                self.assertEqual(call['args'][-1], './cmd/bdock')
                self.assertIn('-trimpath', call['args'])
                self.assertIn('-X main.version=0.1.0', call['args'])
                self.assertEqual(call['env'], dict(CGO_ENABLED='0', GOOS=goos, GOARCH=arch, GOAMD64='v1', GOARM64='v8.0', GOFLAGS=''))
            result = subprocess.run(['sh', 'scripts/release.sh'], cwd=ROOT,
                                    env=dict(env, RELEASE_DIR=str(outputs[0])), capture_output=True)
            self.assertNotEqual(result.returncode, 0)
            self.assertEqual(len(log.read_text().splitlines()), 6)

    def fake_go(self, base):
        compiler = base / 'go'
        compiler.write_text("#!/bin/sh\nif [ \"$1\" = version ]; then echo 'go version fixture'; exit 0; fi\n"
                            "while [ \"$1\" != -o ]; do shift; done; printf x > \"$2\"\n")
        compiler.chmod(0o755)
        return compiler

    def run_release(self, base, **extra):
        env = dict(os.environ, GO=str(self.fake_go(base)), VERSION='0.1.0', RELEASE_DIR=str(base / 'out'))
        env.update(extra)
        return subprocess.run(['sh', 'scripts/release.sh'], cwd=ROOT, env=env, capture_output=True, text=True)

    @requires_gnu_tar
    def test_targets_select_one_archive(self):
        with tempfile.TemporaryDirectory() as directory:
            base = Path(directory)
            result = self.run_release(base, TARGETS='darwin/arm64')
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertEqual({p.name for p in (base / 'out').iterdir()},
                             {'bdock_0.1.0_darwin_arm64.tar.gz', 'LICENSE', 'PROVENANCE', 'SHA256SUMS'})

    def stub_tars(self, base, gtar):
        """A PATH-first directory whose `tar` mimics bsdtar; `gtar` is bsdtar-like (None) or the path of a GNU tar that it wraps."""
        stubs = base / 'stubs'
        stubs.mkdir()
        bsd = '#!/bin/sh\nif [ "$1" = --version ]; then echo "bsdtar 3.5.3 - libarchive 3.5.3"; exit 0; fi\n' \
              'echo "bsdtar stub must not build archives" >&2; exit 2\n'
        (stubs / 'tar').write_text(bsd)
        (stubs / 'gtar').write_text(bsd if gtar is None else '#!/bin/sh\ntouch "$GTAR_USED"\nexec "%s" "$@"\n' % gtar)
        for stub in stubs.iterdir():
            stub.chmod(0o755)
        return stubs

    def test_non_gnu_tar_without_gtar_fails_with_install_hint(self):
        with tempfile.TemporaryDirectory() as directory:
            base = Path(directory)
            stubs = self.stub_tars(base, None)
            result = self.run_release(base, PATH=str(stubs) + os.pathsep + os.environ['PATH'])
            self.assertNotEqual(result.returncode, 0)
            self.assertIn('GNU tar is required for repeatable archives', result.stderr)
            self.assertIn('brew install gnu-tar', result.stderr)
            self.assertFalse((base / 'out').exists())

    @requires_gnu_tar
    def test_non_gnu_tar_falls_back_to_gtar_with_gnu_flags(self):
        with tempfile.TemporaryDirectory() as directory:
            base = Path(directory)
            stubs = self.stub_tars(base, _gnu_tar_path())
            used = base / 'gtar-used'
            result = self.run_release(base, PATH=str(stubs) + os.pathsep + os.environ['PATH'], GTAR_USED=str(used),
                                      TARGETS='linux/amd64')
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertTrue(used.exists())
            with tarfile.open(base / 'out/bdock_0.1.0_linux_amd64.tar.gz') as archive:
                self.assertEqual(archive.getnames(), ['bdock', 'LICENSE', 'PROVENANCE'])
                for member in archive.getmembers():
                    self.assertEqual((member.uid, member.gid, member.mtime), (0, 0, 0))

    def test_unsupported_targets_are_refused_before_building(self):
        for target in ('darwin/amd64', 'windows/amd64', 'linux/386', 'linux/amd64 ../x', 'linux/amd64\nlinux/arm64', ''):
            with tempfile.TemporaryDirectory() as directory:
                base = Path(directory)
                result = self.run_release(base, TARGETS=target)
                self.assertNotEqual(result.returncode, 0, repr(target))
                self.assertFalse((base / 'out').exists(), repr(target))


class SourceStewardAdapterTest(unittest.TestCase):
    def fixture(self, base):
        root = base / 'repo'
        root.mkdir()
        for name in ('scripts/release.sh', 'scripts/release-stw.sh',
                     '.github/scripts/release-verify.sh', 'LICENSE'):
            source = ROOT / name
            if source.exists():
                destination = root / name
                destination.parent.mkdir(parents=True, exist_ok=True)
                shutil.copyfile(source, destination)
        (root / '.gitignore').write_text('/dist/\n')
        # `git commit` starts a detached `git maintenance run --auto`. Under git's geometric
        # strategy it repacks into objects/pack when the commit hash shares the 17 prefix of the
        # fixed .gitignore blob, racing TemporaryDirectory cleanup (ENOTEMPTY). Repository-local
        # config applies to every git process that touches the fixture.
        for args in (('init', '-q'), ('config', 'maintenance.auto', 'false'),
                     ('config', 'gc.auto', '0'), ('add', '.'),
                     ('-c', 'user.name=fixture', '-c', 'user.email=fixture@example.invalid',
                      'commit', '-q', '-m', 'fixture')):
            subprocess.run(['git', *args], cwd=root, check=True, capture_output=True)
        compiler = base / 'go'
        compiler.write_text('''#!/usr/bin/env python3
import json, os, pathlib, sys
base = pathlib.Path(__file__).parent
keys = ('CGO_ENABLED', 'GOOS', 'GOARCH', 'GOAMD64', 'GOARM64', 'GOFLAGS',
        'GOTOOLCHAIN', 'GOPROXY', 'GOSUMDB', 'GOCACHE', 'GOMODCACHE', 'HOME')
with (base / 'calls').open('a') as log:
    log.write(json.dumps(dict(args=sys.argv[1:], env={k:os.environ.get(k) for k in keys}))+'\\n')
if (base / 'fail').exists() and sys.argv[1:] != ['version']:
    sys.exit(23)
if sys.argv[1:] == ['version']:
    print('go version fixture linux/amd64')
else:
    pathlib.Path(sys.argv[sys.argv.index('-o')+1]).write_bytes(b'fake-compiler-output')
''')
        compiler.chmod(0o755)
        (base / 'tmp').mkdir()
        env = dict(os.environ, GO=str(compiler), TMPDIR=str(base / 'tmp'), GOFLAGS='-race')
        return root, env

    def test_fixture_commit_starts_no_background_maintenance(self):
        with tempfile.TemporaryDirectory() as directory:
            base = Path(directory) / 'fx'
            base.mkdir()
            events = Path(directory) / 'trace2.json'
            with mock.patch.dict(os.environ, GIT_TRACE2_EVENT=str(events)):
                self.fixture(base)
            records = [json.loads(line) for line in events.read_text().splitlines()]
            commands = [r['name'] for r in records if r['event'] == 'cmd_name']
            self.assertIn('commit', commands)  # the fixture's own commit was traced
            children = [r['argv'] for r in records if r['event'] == 'child_start']
            self.assertEqual([argv for argv in children if 'maintenance' in argv], [])

    def invoke(self, root, env, os_name='linux', arch='amd64', version='0.1.0', output=None):
        if output is None:
            output = f'dist/release/bdock_{version}_{os_name}_{arch}.tar.gz'
        return subprocess.run(['sh', 'scripts/release-stw.sh', os_name, arch, version, output],
                              cwd=root, env=env, capture_output=True, text=True)

    @unittest.skipIf(tomllib is None, 'tomllib needs Python 3.11+; host python3 is older')
    def test_contract_preserves_targets_and_binds_offline_child_environment(self):
        contract = tomllib.loads((ROOT / '.stw/release.toml').read_text())
        self.assertEqual(contract['version'], dict(scheme='semver', tag_prefix='bdock/', source='input'))
        self.assertEqual(contract['allowlist']['paths'], [])
        self.assertEqual(contract['reproducibility'], dict(policy='required', source_date_epoch='commit-time'))
        self.assertEqual(contract['ci'], dict(workflows=['ci.yml'],
                                             release_workflow='.github/workflows/release-build.yml'))
        self.assertEqual(contract['outputs']['checksums'], 'SHA256SUMS')
        self.assertEqual(len(contract['build']), 1)
        build = contract['build'][0]
        self.assertEqual(build['scope'], 'per-target')
        self.assertEqual(build.get('network', 'none'), 'none')
        settings = dict(item.split('=', 1) for item in build['env_set'])
        for key, value in dict(GOTOOLCHAIN='local', GOPROXY='off', GOSUMDB='off').items():
            self.assertEqual(settings[key], value)
        expected = {('linux', 'amd64'): 'ubuntu-24.04',
                    ('linux', 'arm64'): 'ubuntu-24.04-arm', ('darwin', 'arm64'): 'macos-15'}
        self.assertEqual({(t['os'], t['arch']): t['runner'] for t in contract['target']}, expected)
        self.assertEqual(len(contract['target']), 3)
        with tempfile.TemporaryDirectory() as directory:
            base = Path(directory)
            root, env = self.fixture(base)
            # The declared cache paths must survive the build's environment filter;
            # a later workflow must seed these same paths before this offline build.
            env.update(HOME=str(base / 'home'), GOCACHE=str(base / 'gocache'),
                       GOMODCACHE=str(base / 'gomodcache'))
            for cache in ('GOCACHE', 'GOMODCACHE'):
                Path(env[cache]).mkdir()
                (Path(env[cache]) / 'seed').write_text('verified setup fixture')
            env['PATH'] = str(base) + os.pathsep + env['PATH']
            filtered = {key: env[key] for key in build['env_inherit'] if key in env}
            substitutions = dict(os='linux', arch='amd64', version='v0.1.0', version_number='0.1.0',
                                 output='dist/release/bdock_0.1.0_linux_amd64.tar.gz')
            filtered.update({key: value.format(**substitutions) for key, value in settings.items()})
            argv = [part.format(**substitutions) for part in build['argv']]
            result = subprocess.run(argv, cwd=root, env=filtered, capture_output=True, text=True)
            self.assertEqual(result.returncode, 0, result.stderr)
            calls = [json.loads(line) for line in (base / 'calls').read_text().splitlines()]
            child = next(call for call in calls if call['args'][0] == 'build')
            for key in ('HOME', 'GOCACHE', 'GOMODCACHE'):
                self.assertEqual(child['env'][key], env[key])
            for key in ('GOTOOLCHAIN', 'GOPROXY', 'GOSUMDB'):
                self.assertEqual(child['env'][key], settings[key])
            self.assertEqual(child['env']['GOFLAGS'], '')
            for cache in ('GOCACHE', 'GOMODCACHE'):
                self.assertEqual((Path(env[cache]) / 'seed').read_text(), 'verified setup fixture')
        for target in contract['target']:
            self.assertEqual(target['kind'], 'file')
            self.assertEqual(target['build'], build['id'])
            self.assertEqual(target['output'],
                             'dist/release/bdock_{version_number}_' + target['os'] + '_' + target['arch'] + '.tar.gz')
            self.assertEqual(target['smoke_argv'], ['sh', '.github/scripts/release-smoke.sh',
                             '{version_number}', '{commit}', '{os}', '{arch}', '{artifact}'])

    @requires_gnu_tar
    def test_three_targets_use_fresh_staging_and_preserve_archive_contract(self):
        with tempfile.TemporaryDirectory() as directory:
            base = Path(directory)
            root, env = self.fixture(base)
            revision = subprocess.check_output(['git', 'rev-parse', 'HEAD'], cwd=root, text=True).strip()
            names = set()
            for os_name, arch in TARGETS:
                result = self.invoke(root, env, os_name, arch)
                self.assertEqual(result.returncode, 0, result.stderr)
                name = f'bdock_0.1.0_{os_name}_{arch}.tar.gz'
                names.add(name)
                with tarfile.open(root / 'dist/release' / name) as archive:
                    self.assertEqual(archive.getnames(), ['bdock', 'LICENSE', 'PROVENANCE'])
                    self.assertEqual(archive.extractfile('bdock').read(), b'fake-compiler-output')
                    self.assertEqual(archive.extractfile('LICENSE').read(), (ROOT / 'LICENSE').read_bytes())
                    provenance = archive.extractfile('PROVENANCE').read().decode()
                    for line in ('source_revision=' + revision, 'source_modified=false', 'version=0.1.0',
                                 'GOOS=' + os_name, 'GOARCH=' + arch, 'CGO_ENABLED=0'):
                        self.assertIn(line, provenance.splitlines())
                self.assertEqual(list((base / 'tmp').iterdir()), [])
            self.assertEqual({p.name for p in (root / 'dist/release').iterdir()}, names)
            calls = [json.loads(line) for line in (base / 'calls').read_text().splitlines()]
            builds = [call for call in calls if call['args'][0] == 'build']
            self.assertEqual(len(builds), 3)
            for call, (os_name, arch) in zip(builds, TARGETS):
                self.assertEqual(call['args'][-1], './cmd/bdock')
                self.assertIn('-trimpath', call['args'])
                self.assertIn('-X main.version=0.1.0', call['args'])
                self.assertEqual(call['env']['GOOS'], os_name)
                self.assertEqual(call['env']['GOARCH'], arch)
                self.assertEqual(call['env']['GOFLAGS'], '')
                self.assertEqual(call['env']['GOAMD64'], 'v1')
                self.assertEqual(call['env']['GOARM64'], 'v8.0')

    def test_invalid_arguments_and_existing_paths_do_not_invoke_compiler(self):
        cases = [dict(os_name='windows'), dict(arch='386'), dict(version=''), dict(version='v0.1.0'),
                 dict(version='01.2.3'), dict(version='1.2.3-rc1'), dict(version='1.2.3\n'),
                 dict(output='../outside'), dict(output='/tmp/out'),
                 dict(output='dist/release/../outside'), dict(output='dist/release/wrong.tar.gz')]
        for case in cases:
            with self.subTest(case=case), tempfile.TemporaryDirectory() as directory:
                base = Path(directory)
                root, env = self.fixture(base)
                result = self.invoke(root, env, **case)
                self.assertNotEqual(result.returncode, 0)
                self.assertFalse((base / 'calls').exists())
                self.assertEqual(list((base / 'tmp').iterdir()), [])
        for existing in ('file', 'directory', 'symlink', 'dangling', 'parent-symlink', 'dist-symlink'):
            with self.subTest(existing=existing), tempfile.TemporaryDirectory() as directory:
                base = Path(directory)
                root, env = self.fixture(base)
                output = root / 'dist/release/bdock_0.1.0_linux_amd64.tar.gz'
                output.parent.mkdir(parents=True)
                if existing == 'file':
                    output.write_text('preserve')
                elif existing == 'directory':
                    output.mkdir()
                elif existing in ('symlink', 'dangling'):
                    outside = base / 'outside'
                    if existing == 'symlink':
                        outside.write_text('preserve')
                    output.symlink_to(outside)
                elif existing == 'parent-symlink':
                    output.parent.rmdir()
                    (base / 'outside').mkdir()
                    output.parent.symlink_to(base / 'outside')
                else:
                    output.parent.rmdir()
                    output.parent.parent.rmdir()
                    (base / 'outside').mkdir()
                    output.parent.parent.symlink_to(base / 'outside')
                result = self.invoke(root, env)
                self.assertNotEqual(result.returncode, 0)
                self.assertFalse((base / 'calls').exists())
                self.assertEqual(list((base / 'tmp').iterdir()), [])
                if existing == 'file':
                    self.assertEqual(output.read_text(), 'preserve')
                if existing == 'symlink':
                    self.assertEqual((base / 'outside').read_text(), 'preserve')

    @requires_gnu_tar
    def test_compiler_failure_cleans_staging_without_publishing_output(self):
        with tempfile.TemporaryDirectory() as directory:
            base = Path(directory)
            root, env = self.fixture(base)
            (base / 'fail').touch()
            result = self.invoke(root, env)
            self.assertNotEqual(result.returncode, 0)
            self.assertTrue((base / 'calls').exists())
            self.assertFalse((root / 'dist/release/bdock_0.1.0_linux_amd64.tar.gz').exists())
            self.assertEqual(list((root / 'dist/release').iterdir()), [])
            self.assertEqual(list((base / 'tmp').iterdir()), [])


class ReleaseSmokeChecksumsTest(unittest.TestCase):
    def fixture(self, base):
        archive = base / 'bdock_0.1.0_linux_amd64.tar.gz'
        executable = b'''#!/bin/sh
case "$*" in
    --version) echo 'bdock version 0.1.0' ;;
    --help) echo tools ;;
    'tools --help') echo 'volume upgrade' ;;
    'completion bash') echo completion ;;
    *) exit 1 ;;
esac
'''
        with tarfile.open(archive, 'w:gz') as output:
            contents = {'bdock': executable, 'LICENSE': (ROOT / 'LICENSE').read_bytes(),
                        'PROVENANCE': b'version=0.1.0\ncommand=bdock\nsource_revision=fixture\n'
                                      b'source_modified=false\nGOOS=linux\nGOARCH=amd64\nCGO_ENABLED=0\n'}
            for name, content in contents.items():
                member = tarfile.TarInfo(name)
                member.size = len(content)
                member.mode = 0o755 if name == 'bdock' else 0o644
                output.addfile(member, io.BytesIO(content))
        line = hashlib.sha256(archive.read_bytes()).hexdigest() + '  ' + archive.name + '\n'
        return archive, line

    def run_smoke(self, archive):
        return subprocess.run(['sh', '.github/scripts/release-smoke.sh', '0.1.0', 'fixture',
                               'linux', 'amd64', str(archive)], cwd=ROOT, capture_output=True, text=True)

    def test_global_checksum_selects_only_archive_without_artifact_writes(self):
        with tempfile.TemporaryDirectory() as directory:
            base = Path(directory)
            archive, line = self.fixture(base)
            (base / 'SHA256SUMS').write_text('0' * 64 + '  build-manifest.json\n' + line
                                            + '0' * 64 + '  bdock_0.1.0_linux_arm64.tar.gz\n')
            before = {p.name: p.read_bytes() for p in base.iterdir()}
            result = self.run_smoke(archive)
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertEqual({p.name: p.read_bytes() for p in base.iterdir()}, before)

    def test_legacy_sidecar_is_preserved_and_takes_precedence(self):
        for valid in (True, False):
            with self.subTest(valid=valid), tempfile.TemporaryDirectory() as directory:
                base = Path(directory)
                archive, line = self.fixture(base)
                (base / 'SHA256SUMS').write_text(line)
                sidecar = line if valid else '0' * 64 + '  ' + archive.name + '\n'
                Path(str(archive) + '.sha256').write_text(sidecar)
                result = self.run_smoke(archive)
                self.assertEqual(result.returncode == 0, valid, result.stderr)
                if valid:
                    (base / 'SHA256SUMS').unlink()
                    self.assertEqual(self.run_smoke(archive).returncode, 0)

    def test_invalid_global_checksums_are_refused(self):
        for kind in ('missing', 'duplicate', 'binary-duplicate', 'wrong-digest', 'malformed', 'wrong-name', 'path-name'):
            with self.subTest(kind=kind), tempfile.TemporaryDirectory() as directory:
                base = Path(directory)
                archive, line = self.fixture(base)
                values = {'duplicate': line * 2, 'binary-duplicate': line + line.replace('  ', ' *'),
                          'wrong-digest': '0' * 64 + '  ' + archive.name + '\n',
                          'malformed': 'invalid  ' + archive.name + '\n',
                          'wrong-name': line.replace(archive.name, 'other.tar.gz'),
                          'path-name': line.replace(archive.name, '../' + archive.name)}
                if kind != 'missing':
                    (base / 'SHA256SUMS').write_text(values[kind])
                self.assertNotEqual(self.run_smoke(archive).returncode, 0)
