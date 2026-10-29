"""Static checks of the nixbox Dockerfile: pins, signature policy, nothing compiled."""
from pathlib import Path
import re
import unittest

ROOT = Path(__file__).resolve().parents[2]
BUNDLE = ROOT / 'bundles/nixbox'
KEY = 'cache.nixos.org-1:6NCHdD59X431o0gWypbMrAURkbJ16ZPMQFGspcDShjY='


class NixboxDockerfileTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.text = (BUNDLE / 'Dockerfile').read_text()
        cls.ignore = (BUNDLE / 'Dockerfile.dockerignore').read_text().split('\n')

    def test_every_base_image_is_pinned_by_digest(self):
        stages = re.findall(r'^FROM (?:--platform=\S+ )?(\S+)(?: AS \S+)?$', self.text, re.M)
        self.assertGreaterEqual(len(stages), 3)
        for image in stages:
            if image in ('scratch',) or image in re.findall(r'AS (\S+)', self.text):
                continue
            self.assertRegex(image, r'@sha256:[0-9a-f]{64}$', image)

    def test_nix_copy_checks_signatures_against_the_pinned_key_only(self):
        self.assertIn('nix --extra-experimental-features nix-command copy', self.text)
        self.assertIn('--from https://cache.nixos.org', self.text)
        self.assertIn('--option require-sigs true', self.text)
        self.assertIn("--option trusted-public-keys '" + KEY + "'", self.text)
        self.assertIn("--option substituters ''", self.text)
        self.assertNotIn('no-require-sigs', self.text)
        self.assertNotIn('--no-check-sigs', self.text)
        self.assertNotIn('http://', self.text)

    def test_the_lock_is_checked_before_and_the_store_after_the_copy(self):
        check = self.text.index('nixbox check ')
        copy = self.text.index('nix --extra-experimental-features nix-command copy')
        links = self.text.index('rmdir /image-root/nix/store/.links')
        verify = self.text.index('nixbox verify-store')
        profile = self.text.index('nixbox profile')
        self.assertLess(check, copy)
        self.assertLess(copy, links)
        self.assertLess(links, verify)
        self.assertLess(verify, profile)

    def test_nothing_is_compiled_from_source(self):
        for word in ('nix build', 'nix-build', 'nix-shell', 'nix develop', 'cargo ', 'make ', 'gcc ', 'configure'):
            self.assertNotIn(word, self.text.replace('# ', ''), word)
        # Go compiles only BundleDock's own tool.
        self.assertEqual(re.findall(r'go build [^\n]*?(\./cmd/\S+)', self.text), ['./cmd/nixbox', './cmd/nixbox'])

    def test_final_image_is_a_carrier_that_initializes_the_volume(self):
        final = self.text.split('\nFROM scratch')[-1]
        self.assertIn('COPY --from=', final)
        self.assertIn('/opt/bdock/nixbox', final)
        self.assertRegex(final, r'ENTRYPOINT \["/usr/local/bin/nixbox", "init"')

    def test_build_context_allows_exactly_what_the_build_copies(self):
        copied = set(re.findall(r'^COPY (?:--from=\S+ )?(?!--)(\S+)', self.text, re.M))
        for line in self.ignore:
            self.assertFalse(line.startswith('!') and 'opentofu' in line)
        self.assertIn('**', self.ignore)
        for needed in ('!cmd/nixbox/**', '!internal/nixbox/**', '!go.mod', '!go.sum',
                       '!bundles/nixbox/packages.catalog.json', '!bundles/nixbox/packages.lock.json',
                       '!bundles/nixbox/shims/**'):
            self.assertIn(needed, self.ignore)
        self.assertTrue(copied)

    def test_dockerfile_pins_the_nix_image_the_refresh_script_uses(self):
        pins = set(re.findall(r'nixos/nix@sha256:[0-9a-f]{64}', self.text))
        refresh = set(re.findall(r'nixos/nix@sha256:[0-9a-f]{64}', (BUNDLE / 'scripts/refresh-lock.sh').read_text()))
        self.assertEqual(len(pins), 1)
        self.assertEqual(pins, refresh)


if __name__ == '__main__':
    unittest.main()
