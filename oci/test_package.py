import io
from pathlib import Path
import tarfile
import tempfile
import unittest

from package import extract_codex


class CodexPackageTest(unittest.TestCase):
    def test_rejects_escaping_paths_and_links(self):
        for name, kind in [('../escape', tarfile.REGTYPE), ('/escape', tarfile.REGTYPE), ('bin/codex', tarfile.SYMTYPE)]:
            with self.subTest(name=name, kind=kind), tempfile.TemporaryDirectory() as tmp:
                root = Path(tmp)
                with tarfile.open(root / 'package.tar', 'w') as archive:
                    member = tarfile.TarInfo(name)
                    member.type = kind
                    member.linkname = '/outside'
                    archive.addfile(member)
                with tarfile.open(root / 'package.tar') as archive:
                    with self.assertRaises(ValueError):
                        extract_codex(archive, root / 'bundle')

    def test_preserves_daemon_package_and_runtime_assets(self):
        files = {
            'codex-package.json': b'{"entrypoint":"bin/codex","resourcesDir":"codex-resources","pathDir":"codex-path"}',
            'bin/codex': b'cli',
            'bin/codex-code-mode-host': b'code-host',
            'codex-path/rg': b'ripgrep',
            'codex-resources/bwrap': b'bubblewrap',
            'codex-resources/voice/lib/libexample.so': b'library',
        }
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            with tarfile.open(root / 'package.tar', 'w') as archive:
                for name, data in files.items():
                    member = tarfile.TarInfo(name)
                    member.size = len(data)
                    member.mode = 0o644 if name.endswith('.json') else 0o755
                    archive.addfile(member, io.BytesIO(data))
            with tarfile.open(root / 'package.tar') as archive:
                extract_codex(archive, root / 'bundle')
            for name, data in files.items():
                self.assertEqual((root / 'bundle' / name).read_bytes(), data)
            self.assertEqual((root / 'bundle/codex-package.json').stat().st_mode & 0o777, 0o644)
            self.assertTrue((root / 'bundle/bin/codex').stat().st_mode & 0o111)
