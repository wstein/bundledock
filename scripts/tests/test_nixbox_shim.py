"""Run the real nixbox shim against a fake profile (with its tools) and loader."""
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[2]
SHIM = ROOT / 'bundles/nixbox/shims/nixbox-shim'
TABLE = 'bin\thx\tlib/helix/hx\nenv\thx\tHELIX_RUNTIME\tlib/helix/runtime\nbin\tzk\tbin/zk\n'


@unittest.skipUnless(shutil.which('sh'), 'requires POSIX sh')
class ShimTests(unittest.TestCase):
    def setUp(self):
        self.dir = Path(tempfile.mkdtemp())
        self.addCleanup(shutil.rmtree, self.dir, True)
        # A path with a space proves quoting.
        self.profile = self.dir / 'pro file'
        (self.profile / 'bin').mkdir(parents=True)
        (self.profile / 'libexec').mkdir()
        (self.profile / 'share/nixbox').mkdir(parents=True)
        (self.profile / 'lib').mkdir()
        shutil.copy(SHIM, self.profile / 'libexec/nixbox-shim')
        (self.profile / 'share/nixbox/shims.tsv').write_text(TABLE)
        for name in ('zk', 'hx', 'unlisted'):
            (self.profile / 'bin' / name).symlink_to('../libexec/nixbox-shim')
        (self.profile / 'lib/glibc').mkdir()
        (self.profile / 'lib/gcc').mkdir()
        loader = self.profile / 'lib/ld.so'
        loader.write_text('#!/bin/sh\nprintf "LOADER"; for a in "$@"; do printf " [%s]" "$a"; done; printf "\\n"\n')
        loader.chmod(0o755)
        # The official binaries live inside the profile, below libexec/nixbox.
        self.toolbox = self.profile / 'libexec/nixbox'
        for rel in ('bin/zk', 'lib/helix/hx'):
            path = self.toolbox / rel
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_text('#!/bin/sh\nprintf "DIRECT"; for a in "$@"; do printf " [%s]" "$a"; done; printf " HELIX_RUNTIME=%s\\n" "${HELIX_RUNTIME-unset}"\n')
            path.chmod(0o755)
        self.host_loader = self.dir / 'host-ld'
        self.host_loader.write_text('')

    def run_shim(self, name, *args, **env):
        environment = {k: v for k, v in os.environ.items() if not k.startswith(('NIXBOX_', 'BDOCK_', 'HELIX_'))}
        environment.update(NIXBOX_HOST_LOADERS=str(self.dir / 'absent'),
                           NIXBOX_MUSL_LOADERS=str(self.dir / 'absent'))
        environment.update(env)
        return subprocess.run([str(self.profile / 'bin' / name), *args], env=environment, capture_output=True, text=True)

    def test_without_a_host_loader_the_nix_loader_runs_the_binary(self):
        result = self.run_shim('zk', 'list', 'a b')
        self.assertEqual(result.returncode, 0, result.stderr)
        exe = self.toolbox / 'bin/zk'
        self.assertEqual(result.stdout, 'LOADER [--library-path] [%s/lib/glibc:%s/lib/gcc] [%s] [list] [a b]\n' % (self.profile, self.profile, exe))

    def test_with_a_host_loader_the_binary_runs_directly(self):
        result = self.run_shim('zk', 'list', NIXBOX_HOST_LOADERS='%s %s' % (self.dir / 'absent', self.host_loader))
        self.assertEqual(result.stdout, 'DIRECT [list] HELIX_RUNTIME=unset\n', result.stderr)

    def test_musl_host_uses_the_nix_loader_even_with_a_glibc_looking_loader(self):
        # Alpine with gcompat has /lib/ld-linux-<arch>.so.1 but its binaries still fail.
        result = self.run_shim('zk', 'list', NIXBOX_HOST_LOADERS=str(self.host_loader), NIXBOX_MUSL_LOADERS=str(self.host_loader))
        self.assertTrue(result.stdout.startswith('LOADER'), result.stdout + result.stderr)

    def test_musl_loader_patterns_are_globs(self):
        (self.dir / 'ld-musl-x.so.1').write_text('')
        result = self.run_shim('zk', NIXBOX_HOST_LOADERS=str(self.host_loader), NIXBOX_MUSL_LOADERS=str(self.dir / 'ld-musl-*.so.1'))
        self.assertTrue(result.stdout.startswith('LOADER'), result.stdout)

    def test_glibc_host_without_musl_runs_directly(self):
        result = self.run_shim('zk', NIXBOX_HOST_LOADERS=str(self.host_loader), NIXBOX_MUSL_LOADERS=str(self.dir / 'ld-musl-*.so.1'))
        self.assertTrue(result.stdout.startswith('DIRECT'), result.stdout + result.stderr)

    def test_forced_modes_ignore_musl_detection(self):
        both = dict(NIXBOX_HOST_LOADERS=str(self.host_loader), NIXBOX_MUSL_LOADERS=str(self.host_loader))
        self.assertTrue(self.run_shim('zk', NIXBOX_LOADER='never', **both).stdout.startswith('DIRECT'))
        self.assertTrue(self.run_shim('zk', NIXBOX_LOADER='always', NIXBOX_HOST_LOADERS=str(self.host_loader)).stdout.startswith('LOADER'))

    def test_user_symlink_to_a_shim_resolves_the_profile(self):
        link_dir = self.dir / 'ub'
        link_dir.mkdir()
        (link_dir / 'zk').symlink_to(self.profile / 'bin/zk')
        environment = dict(os.environ, NIXBOX_LOADER='never')
        result = subprocess.run([str(link_dir / 'zk'), 'x'], env=environment, capture_output=True, text=True)
        self.assertEqual((result.returncode, result.stdout), (0, 'DIRECT [x] HELIX_RUNTIME=unset\n'), result.stderr)

    def test_user_symlink_with_another_name_resolves_the_shim_entry(self):
        link_dir = self.dir / 'ub'
        link_dir.mkdir()
        (link_dir / 'helix').symlink_to(self.profile / 'bin/hx')
        (link_dir / 'again').symlink_to('helix')
        environment = dict(os.environ, NIXBOX_LOADER='never')
        environment.pop('HELIX_RUNTIME', None)
        for name in ('helix', 'again'):
            result = subprocess.run([str(link_dir / name), 'x'], env=environment, capture_output=True, text=True)
            self.assertEqual((result.returncode, result.stdout), (0, 'DIRECT [x] HELIX_RUNTIME=%s/lib/helix/runtime\n' % self.toolbox), result.stderr)

    def test_loader_modes_override_detection(self):
        self.assertTrue(self.run_shim('zk', NIXBOX_LOADER='always', NIXBOX_HOST_LOADERS=str(self.host_loader)).stdout.startswith('LOADER'))
        self.assertTrue(self.run_shim('zk', NIXBOX_LOADER='never').stdout.startswith('DIRECT'))
        result = self.run_shim('zk', NIXBOX_LOADER='sometimes')
        self.assertEqual(result.returncode, 127)
        self.assertIn('NIXBOX_LOADER', result.stderr)

    def test_hx_runs_the_inner_binary_with_the_helix_runtime(self):
        result = self.run_shim('hx', '--version', NIXBOX_LOADER='never', HELIX_RUNTIME='/elsewhere')
        self.assertEqual(result.stdout, 'DIRECT [--version] HELIX_RUNTIME=%s/lib/helix/runtime\n' % self.toolbox)
        result = self.run_shim('hx', NIXBOX_LOADER='always')
        self.assertIn('[%s/lib/helix/hx]' % self.toolbox, result.stdout)

    def test_runtime_is_not_set_for_other_tools(self):
        self.assertIn('HELIX_RUNTIME=unset', self.run_shim('zk', NIXBOX_LOADER='never').stdout)

    def test_a_missing_binary_fails_with_a_clear_message(self):
        (self.toolbox / 'bin/zk').unlink()
        result = self.run_shim('zk')
        self.assertEqual(result.returncode, 127)
        self.assertIn('zk is not installed: %s is missing' % (self.toolbox / 'bin/zk'), result.stderr)
        self.assertEqual(result.stdout, '')

    def test_the_toolbox_is_not_used(self):
        # A toolbox copy on the old path is ignored: only the binary inside the profile runs.
        other = self.dir / 'tool box/bin'
        other.mkdir(parents=True)
        (other / 'zk').write_text('#!/bin/sh\necho TOOLBOX\n')
        (other / 'zk').chmod(0o755)
        result = self.run_shim('zk', NIXBOX_LOADER='never', BDOCK_TOOLBOX=str(self.dir / 'tool box'))
        self.assertTrue(result.stdout.startswith('DIRECT'), result.stdout + result.stderr)

    def test_unlisted_name_and_missing_table_fail(self):
        result = self.run_shim('unlisted')
        self.assertEqual((result.returncode, 'not a nixbox shim' in result.stderr), (127, True))
        (self.profile / 'share/nixbox/shims.tsv').unlink()
        result = self.run_shim('zk')
        self.assertEqual((result.returncode, 'missing shim table' in result.stderr), (127, True))

    def test_missing_nix_loader_fails_in_the_loader_path_only(self):
        (self.profile / 'lib/ld.so').unlink()
        self.assertEqual(self.run_shim('zk', NIXBOX_LOADER='always').returncode, 127)
        self.assertEqual(self.run_shim('zk', NIXBOX_LOADER='never').returncode, 0)

    def test_exit_status_and_stdin_pass_through(self):
        (self.toolbox / 'bin/zk').write_text('#!/bin/sh\ncat\nexit 3\n')
        result = subprocess.run([str(self.profile / 'bin/zk')], input='piped', capture_output=True, text=True,
                                env=dict(os.environ, NIXBOX_LOADER='never'))
        self.assertEqual((result.returncode, result.stdout), (3, 'piped'))

    def test_shim_is_posix_sh_and_passes_shellcheck_when_available(self):
        self.assertEqual(SHIM.read_text().splitlines()[0], '#!/bin/sh')
        if shutil.which('shellcheck'):
            self.assertEqual(subprocess.run(['shellcheck', '-s', 'sh', str(SHIM)]).returncode, 0)


if __name__ == '__main__':
    unittest.main()
