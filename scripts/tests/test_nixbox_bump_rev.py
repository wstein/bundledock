"""nixbox-bump-rev.py with a stubbed channel; no network."""
import importlib.util
import json
from pathlib import Path
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[2]
SCRIPT = ROOT / 'scripts/nixbox-bump-rev.py'
spec = importlib.util.spec_from_file_location('bump', SCRIPT)
bump = importlib.util.module_from_spec(spec)
spec.loader.exec_module(bump)

OLD, NEW = 'e' * 40, '1' * 40
RELEASE = 'https://releases.nixos.org/nixpkgs/nixpkgs-26.11pre2.' + NEW[:12]
CATALOG = '''{
  "schema": 1,
  "nixpkgs": {
    "rev": "%s",
    "url": "https://releases.nixos.org/nixpkgs/nixpkgs-26.11pre1.%s/nixexprs.tar.zst",
    "narHash": "sha256-AAAA"
  },
  "packages": {"git": {"provider": "nix", "attr": "gitMinimal"}}
}
''' % (OLD, OLD[:12])


def channel(rev):
    def fetch(url):
        if url == bump.CHANNEL:
            return RELEASE
        if url == RELEASE + '/git-revision':
            return rev + '\n'
        raise AssertionError(url)
    return fetch


class BumpTests(unittest.TestCase):
    def test_latest_resolves_the_channel_head(self):
        self.assertEqual(bump.latest(channel(NEW)), (NEW, RELEASE + '/nixexprs.tar.zst'))

    def test_latest_rejects_a_malformed_revision(self):
        with self.assertRaises(SystemExit):
            bump.latest(channel('not-a-rev'))

    def test_set_pin_writes_only_rev_url_and_narhash(self):
        out = bump.set_pin(CATALOG, NEW, RELEASE + '/nixexprs.tar.zst', 'sha256-BBBB')
        before, after = json.loads(CATALOG), json.loads(out)
        self.assertEqual(after['nixpkgs'], {'rev': NEW, 'url': RELEASE + '/nixexprs.tar.zst', 'narHash': 'sha256-BBBB'})
        after['nixpkgs'] = before['nixpkgs'] = None
        self.assertEqual(after, before)
        self.assertEqual(out.count('\n'), CATALOG.count('\n'))

    def test_changed_reports_new_rev_and_unchanged_reports_nothing(self):
        self.assertEqual(bump.pending(CATALOG, channel(NEW)), (NEW, RELEASE + '/nixexprs.tar.zst'))
        self.assertIsNone(bump.pending(CATALOG, channel(OLD)))

    def test_cli_set_rewrites_the_catalog(self):
        with tempfile.TemporaryDirectory() as d:
            path = Path(d) / 'c.json'
            path.write_text(CATALOG)
            subprocess.run(['python3', '-B', str(SCRIPT), 'set', str(path), NEW, RELEASE + '/nixexprs.tar.zst', 'sha256-BBBB'], check=True)
            self.assertEqual(json.loads(path.read_text())['nixpkgs']['rev'], NEW)


if __name__ == '__main__':
    unittest.main()
