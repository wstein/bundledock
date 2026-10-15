"""Exercise repository config tests with synthetic future stable locks, offline."""
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[2]


def next_release(tag, package):
    prefix = package.get('tag_prefix', '')
    if not tag.startswith(prefix):
        prefix = ''
    version = tag[len(prefix):]
    if package['normalization'] == 'tmux-release':
        match = re.fullmatch(r'(\d+)\.(\d+)([a-z]?)', version)
        if not match:
            raise ValueError('unsupported fixture tmux version')
        major, minor, letter = match.groups()
        newer = major + '.' + minor + (chr(ord(letter) + 1) if letter and letter != 'z' else 'a')
        if letter == 'z':
            newer = major + '.' + str(int(minor) + 1)
    else:
        parts = version.split('.')
        if len(parts) != 3 or not all(p.isdigit() for p in parts):
            raise ValueError('unsupported fixture stable version')
        parts[2] = str(int(parts[2]) + 1)
        newer = '.'.join(parts)
    return prefix + newer, version, newer


def copy_repository(root):
    for name in ('bundles', 'scripts', '.config', '.github', 'docs', 'internal/toolconfig',
                 'internal/toolupgrade', 'internal/toolpublish', 'internal/toolbundle/python'):
        shutil.copytree(ROOT / name, root / name, ignore=shutil.ignore_patterns('__pycache__'))
    for name in ('go.mod', 'go.sum'):
        shutil.copyfile(ROOT / name, root / name)


def run_config_tests(root, pattern=None, packages=None):
    env = dict(os.environ, GOPROXY='off', GOSUMDB='off', GOTOOLCHAIN='local')
    command = [os.environ.get('GO', 'go'), 'test', '-count=1']
    command.extend(packages or ['./internal/toolconfig'])
    if pattern:
        command.extend(['-run', pattern])
    return subprocess.run(command, cwd=root, env=env, capture_output=True, text=True, timeout=120)


class RollingLockTests(unittest.TestCase):
    def test_repository_config_tests_accept_future_stable_lock_only_updates(self):
        with tempfile.TemporaryDirectory(prefix='bundledock-future-locks-') as directory:
            root = Path(directory)
            copy_repository(root)
            changed, frozen = [], []
            for catalog_path in sorted((root / 'bundles').glob('*/packages.catalog.json')):
                catalog = json.loads(catalog_path.read_text())
                path = catalog_path.with_name('packages.lock.json')
                lock = json.loads(path.read_text())
                for name, package in catalog['packages'].items():
                    # This deliberately authorized exact source recipe is not a
                    # rolling prebuilt tool. Keep its full immutable pin intact.
                    if package['install'].get('profile') == 'git-trim-source':
                        frozen.append(name)
                        continue
                    old = lock['platforms']['amd64'][name]['version']
                    new, old_version, new_version = next_release(old, package)
                    for arch in ('amd64', 'arm64'):
                        pin = lock['platforms'][arch][name]
                        self.assertEqual(pin['version'], old)
                        url = pin['url'].replace(old, new).replace(old_version, new_version)
                        self.assertNotEqual(url, pin['url'], (name, arch, 'version missing from URL'))
                        pin.update(version=new, url=url)
                        # Synthetic metadata only: no binary downloads or claims
                        # of actual upstream availability or checksum validation.
                        if pin.get('sha512'):
                            pin['sha512'] = hashlib.sha512((name + arch + new).encode()).hexdigest()
                        else:
                            pin['sha256'] = hashlib.sha256((name + arch + new).encode()).hexdigest()
                    changed.append(catalog_path.parent.name + '/' + name)
                path.write_text(json.dumps(lock, indent=2) + '\n')
                # Workflow no-op cases require a consistent baseline inventory,
                # just like a real bot lock-and-inventory updater PR.
                subprocess.run(['python3', 'scripts/tool-inventory.py', '--bundle', catalog_path.parent.name],
                               cwd=root, check=True, capture_output=True, text=True, timeout=30)
            self.assertEqual(frozen, ['git-trim'])
            self.assertGreaterEqual(len(changed), 48)
            self.assertEqual({name.split('/')[0] for name in changed},
                             {'toolbox', 'bun', 'code-server', 'node', 'node-current', 'codex', 'agy', 'claude', 'opentofu'})
            result = run_config_tests(root, packages=['./internal/toolconfig', './internal/toolupgrade'])
            self.assertEqual(result.returncode, 0, 'Future stable lock-and-inventory updates must pass config and workflow tests:\n' + result.stdout + result.stderr)
            # Allocation is a platform-neutral source contract. Native OCI gate
            # tests require Linux and are covered separately by official CI.
            allocations = ('^Test(SingletonVersionAllocation|SingletonAllocationTracksRollingLocks|NodeSharedSourceReservations|'
                           'NodeAllocationSharesAuthenticatedRepository|'
                           'RegistryAllocatesAcrossPagesAndSourceReservations|'
                           'ToolboxDateRevisionAllocation|ToolboxRegistryAndSourceReservations|'
                           'SemanticVersionCollisionStopsPublication|DailyTagCollision)$')
            result = run_config_tests(root, allocations, packages=['./internal/toolpublish'])
            self.assertEqual(result.returncode, 0, 'Future stable locks must pass publisher allocation contracts:\n' + result.stdout + result.stderr)

    def test_integrity_errors_still_block_publication_inputs(self):
        with tempfile.TemporaryDirectory(prefix='bundledock-invalid-locks-') as directory:
            root = Path(directory)
            copy_repository(root)
            path = root / 'bundles/node/packages.lock.json'
            original = path.read_text()
            for kind, reason in [('checksum', 'invalid checksum'),
                                 ('range', 'outside desired range'),
                                 ('provenance', 'invalid official Node pin'),
                                 ('architecture', 'invalid official Node pin')]:
                with self.subTest(kind=kind):
                    lock = json.loads(original)
                    pin = lock['platforms']['amd64']['node']
                    if kind == 'checksum':
                        pin['sha256'] = 'bad'
                    elif kind == 'range':
                        for arch in ('amd64', 'arm64'):
                            p = lock['platforms'][arch]['node']
                            p['url'] = p['url'].replace(p['version'], 'v99.0.0')
                            p['version'] = 'v99.0.0'
                    elif kind == 'provenance':
                        pin['verification'] = 'tls-download-sha256'
                    else:
                        pin['url'] = lock['platforms']['arm64']['node']['url']
                    path.write_text(json.dumps(lock))
                    result = run_config_tests(root, '^TestOptionalNodeBundlePinsBothOfficialPayloads$')
                    self.assertNotEqual(result.returncode, 0)
                    self.assertIn(reason, result.stdout + result.stderr)
            path.write_text(original)
            # License bytes remain protected even when the binary lock rolls.
            license_path = root / 'bundles/toolbox/licenses/leaf/LICENSE'
            license_path.write_text(license_path.read_text() + '\nchanged bytes\n')
            result = run_config_tests(root, '^TestReviewedLicenseSnapshots$')
            self.assertNotEqual(result.returncode, 0)
            self.assertIn('reviewed license bytes changed', result.stdout + result.stderr)
