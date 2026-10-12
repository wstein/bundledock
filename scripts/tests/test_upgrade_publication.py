import importlib.util
import contextlib
import io
import runpy
import json
import hashlib
import urllib.error
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest
from unittest.mock import patch

ROOT = Path(__file__).resolve().parents[2]


def load_helper():
    spec = importlib.util.spec_from_file_location('upgrade_publication', ROOT/'scripts/upgrade-publication.py')
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


class ResolveTests(unittest.TestCase):
    @unittest.skipUnless(shutil.which("git") and shutil.which("bash"), "requires git and bash")
    def test_actual_no_change_resolver_reports_verified_unpublished(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            (root/'scripts').mkdir()
            shutil.copy(ROOT/'scripts/tool-upgrade.sh', root/'scripts')
            (root/'scripts/tool-inventory.py').write_text('')
            # Fake only the registry-read helper, never the resolver under test.
            (root/'scripts/upgrade-publication.py').write_text(
                'print("Bundle unpublished—initial publication required")\n')
            (root/'bundles/codex').mkdir(parents=True)
            (root/'bundles/codex/tools.lock.json').write_text('{}')
            (root/'docs').mkdir()
            (root/'docs/codex.md').write_text('inventory')
            (root/'bin').mkdir()
            (root/'bin/go').write_text('#!/bin/sh\nexit 0\n')
            (root/'bin/go').chmod(0o755)
            for args in (['init','-q'], ['add','.'], ['-c','user.name=Test','-c','user.email=test@example.com','commit','-qm','fixture']):
                subprocess.run(['git',*args],cwd=root,check=True,capture_output=True)
            env = dict(os.environ, PATH=str(root/'bin')+':'+os.environ['PATH'], BUNDLE='codex',
                       UPGRADE_RESULT=str(root/'result'), GITHUB_OUTPUT=str(root/'output'),
                       GITHUB_STEP_SUMMARY=str(root/'summary'), REPOSITORY_URL='quay.io/bundledock')
            result = subprocess.run(['bash','scripts/tool-upgrade.sh','resolve'],cwd=root,env=env,capture_output=True,text=True)
            self.assertEqual(result.returncode,0,result.stderr)
            self.assertIn('changed=false', (root/'output').read_text())
            self.assertIn('Bundle unpublished—initial publication required',(root/'summary').read_text())


class PublicationTests(unittest.TestCase):
    def setUp(self):
        self.helper = load_helper()
        self.sha = 'a'*40

    @unittest.skipUnless(shutil.which("git"), "requires git")
    def test_cli_reads_actual_git_head_with_fake_http(self):
        class Response(io.BytesIO):
            pass
        class Opener:
            def open(self, request, timeout):
                self_url = request.full_url
                if not self_url.startswith('https://quay.io/api/v1/repository/'):
                    raise AssertionError(self_url)
                if request.get_header('Authorization') is not None:
                    raise AssertionError('public listing sent credentials')
                return Response(b'{"tags":[],"has_additional":false}')
        head = subprocess.run(['git','rev-parse','--verify','HEAD^{commit}'], check=True,
                              capture_output=True, text=True).stdout.strip()
        output = io.StringIO()
        with patch.dict(os.environ, BUNDLE='codex', REPOSITORY_URL='quay.io/bundledock', GITHUB_SHA='spoofed'), \
                patch('urllib.request.build_opener', return_value=Opener()) as opener, \
                contextlib.redirect_stdout(output):
            runpy.run_path(str(ROOT/'scripts/upgrade-publication.py'), run_name='__main__')
        self.assertIn('`approved_sha` set to `'+head+'`',output.getvalue())
        self.assertNotIn('spoofed',output.getvalue())
        self.assertEqual(opener.call_args.args[0].__name__, 'NoRedirect')

    def test_empty_complete_public_listing_for_every_bundle(self):
        for bundle in self.helper.config.BUNDLES:
            with self.subTest(bundle=bundle), patch.object(self.helper.provenance, 'request', return_value=b'{"tags":[],"has_additional":false}') as request:
                result = self.helper.summary(bundle, 'quay.io/another-namespace/', self.sha)
                self.assertIn('Bundle unpublished—initial publication required', result)
                self.assertIn('bundledock-'+bundle+'.yml', result)
                self.assertIn('`approved_sha` set to `'+self.sha+'`', result)
                self.assertIn('/repository/another-namespace/'+bundle+'/tag/', request.call_args.args[0])
                self.assertEqual(request.call_count, 1)

    def test_ambiguous_or_partial_listing_never_means_unpublished(self):
        for body in (b'{}', b'{"tags":[]}', b'{"tags":[],"has_additional":true}',
                     b'{"tags":{},"has_additional":false}', b'not json',
                     b'{"tags":[{"name":"staging-123"}],"has_additional":false}'):
            with self.subTest(body=body), patch.object(self.helper.provenance, 'request', return_value=body):
                self.assertEqual(self.helper.publication_status('codex','quay.io/bundledock'), 'unknown')

    def test_auth_absence_redirect_and_network_fail_closed_and_redact(self):
        errors = [urllib.error.HTTPError('https://quay.io', code, 'secret-token', {}, None)
                  for code in (301,302,401,403,404,429,500)]
        errors += [urllib.error.URLError('signed-url-secret')]
        for error in errors:
            with self.subTest(error=type(error).__name__), patch.object(self.helper.provenance, 'request', side_effect=error):
                result = self.helper.summary('agy', 'quay.io/bundledock', self.sha)
                self.assertIn('status unknown', result)
                self.assertNotIn('secret', result)
                self.assertNotIn('initial publication required', result)

    def test_configuration_and_head_validation(self):
        for prefix in ('https://quay.io/bundledock', 'evil.example/bundledock', 'quay.io/x/y',
                       'quay.io/x//', 'quay.io/x?token=secret', 'quay.io/user@host', ''):
            with self.subTest(prefix=prefix), patch.object(self.helper.provenance, 'request') as request:
                self.assertEqual(self.helper.publication_status('claude', prefix), 'unknown')
                request.assert_not_called()
        with self.assertRaises(ValueError):
            self.helper.summary('codex','quay.io/bundledock','short-sha')
        with self.assertRaises(ValueError):
            self.helper.publication_status('../escape','quay.io/bundledock')

    def test_published_requires_both_platforms_and_matching_provenance(self):
        for broken in ('', 'missing-platform', 'wrong-source', 'bad-digest', 'invalid-date', 'legacy-run-mismatch'):
            with self.subTest(broken=broken):
                objects = {}
                def store(value):
                    raw = json.dumps(value).encode()
                    digest = 'sha256:'+hashlib.sha256(raw).hexdigest()
                    objects[digest] = raw
                    return digest
                version = {'invalid-date':'2026.99.99-b1.1', 'legacy-run-mismatch':'2026.10.12-999-1'}.get(broken, '2026.10.12-b1.1')
                descriptors = []
                for arch in ('amd64','arm64'):
                    labels = {'org.opencontainers.image.source': self.helper.provenance.SOURCE,
                              'org.opencontainers.image.revision':self.sha,
                              'org.opencontainers.image.version':version,
                              'io.bundledock.github.run-id':'123'}
                    if broken == 'wrong-source' and arch == 'arm64':
                        labels['org.opencontainers.image.source'] = 'wrong'
                    config = store({'os':'linux','architecture':arch,'config':{'Labels':labels}})
                    manifest = store({'config':{'digest':config}})
                    descriptors.append({'platform':{'os':'linux','architecture':arch}, 'digest':manifest})
                if broken == 'missing-platform':
                    descriptors.pop()
                digest = store({'manifests':descriptors})
                if broken == 'bad-digest':
                    digest = 'sha256:'+'0'*64
                listing = json.dumps({'tags':[{'name':version,'manifest_digest':digest}], 'has_additional':False}).encode()
                calls = []
                def http(url, headers=None, signed_blob=False):
                    calls.append((url, headers))
                    if '/api/v1/' in url:
                        return listing
                    if '/v2/auth?' in url:
                        self.assertIsNone(headers)
                        return b'{"token":"anonymous-token"}'
                    ref = url.rsplit('/',1)[1]
                    return objects[digest] if ref == version else objects[ref]
                with patch.object(self.helper.provenance, 'request', side_effect=http):
                    self.assertEqual(self.helper.publication_status('codex','quay.io/bundledock'),
                                     'published' if not broken else 'unknown')
                self.assertLessEqual(len(calls), 12)
                self.assertTrue(all(url.startswith('https://quay.io/') for url,_ in calls))

    def test_shared_workflow_keeps_all_seven_read_only_resolvers(self):
        workflow = (ROOT/'.github/workflows/bundle-upgrade.yml').read_text()
        resolve = workflow.split('\n  publish:\n')[0]
        self.assertIn('REPOSITORY_URL: ${{ vars.REPOSITORY_URL }}', resolve)
        self.assertNotIn('secrets.',resolve)
        for bundle in self.helper.config.BUNDLES:
            caller = (ROOT/'.github/workflows'/f'{bundle}-upgrade.yml').read_text()
            self.assertIn('uses: ./.github/workflows/bundle-upgrade.yml',caller)
            self.assertIn("cron: '50 6 * * *'",caller)


if __name__ == '__main__':
    unittest.main()
