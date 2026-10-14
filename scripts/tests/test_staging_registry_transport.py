import base64
import importlib.util
import io
import json
from pathlib import Path
import unittest
from unittest.mock import patch
import urllib.error
import urllib.parse

SPEC = importlib.util.spec_from_file_location('registry_cleanup', Path(__file__).resolve().parents[1] / 'staging-cleanup.py')
cleanup = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(cleanup)
NAME = 'staging-2026.10.12-b1.1-amd64'


def response(status, document=None):
    result = io.BytesIO(json.dumps(document).encode() if document is not None else b'')
    result.status = status
    return result


class RegistryTransportTests(unittest.TestCase):
    def api(self):
        return cleanup.Quay('bundledock+github-actions', 'fixture-password')

    def test_exchange_robot_credentials_for_repository_scope_and_delete_tag_name(self):
        api = self.api()
        with patch.object(api.opener, 'open', side_effect=[response(200, dict(token='fixture.jwt')), response(202)]) as send:
            api.delete('toolbox', NAME)
        self.assertEqual(send.call_count, 2)
        exchange, deletion = [call.args[0] for call in send.call_args_list]
        parsed = urllib.parse.urlsplit(exchange.full_url)
        self.assertEqual((parsed.scheme, parsed.netloc, parsed.path), ('https', 'quay.io', '/v2/auth'))
        self.assertEqual(urllib.parse.parse_qs(parsed.query), dict(service=['quay.io'], scope=['repository:bundledock/toolbox:pull,push']))
        self.assertEqual(exchange.get_method(), 'GET')
        auth = exchange.get_header('Authorization')
        self.assertEqual(base64.b64decode(auth.removeprefix('Basic ')).decode(), 'bundledock+github-actions:fixture-password')
        self.assertEqual(deletion.full_url, 'https://quay.io/v2/bundledock/toolbox/manifests/' + NAME)
        self.assertEqual(deletion.get_method(), 'DELETE')
        self.assertEqual(deletion.get_header('Authorization'), 'Bearer fixture.jwt')
        self.assertNotIn('fixture-password', deletion.full_url)
        self.assertNotIn('sha256:', deletion.full_url)
        self.assertTrue(any(isinstance(handler, cleanup.tags.NoRedirect) for handler in api.opener.handlers))

    def test_missing_or_partial_credentials_stop_before_network(self):
        for username, password in [('', ''), ('robot', ''), ('', 'password')]:
            api = cleanup.Quay(username, password)
            with patch.object(api.opener, 'open') as send, self.assertRaises(ValueError):
                api.delete('toolbox', NAME)
            send.assert_not_called()

    def test_digest_production_tag_and_foreign_repository_stop_before_network(self):
        api = self.api()
        for repository, name in [('toolbox', 'sha256:' + 'a' * 64), ('toolbox', 'stable'),
                                 ('toolbox', '../stable'), ('toolbox', NAME + '\n'),
                                 ('foreign/toolbox', NAME), ('toolbox', NAME + '/anything')]:
            with self.subTest(name=name), patch.object(api.opener, 'open') as send, self.assertRaises(ValueError):
                api.delete(repository, name)
            send.assert_not_called()

    def test_bad_token_and_exchange_failure_cannot_send_delete(self):
        api = self.api()
        for token in ({}, dict(token=''), dict(token=None), dict(token='bad\r\nheader'), []):
            with self.subTest(token=token), patch.object(api.opener, 'open', return_value=response(200, token)) as send, self.assertRaises(ValueError):
                api.delete('toolbox', NAME)
            self.assertEqual(send.call_count, 1)
        for code in (302, 401, 403, 404, 429, 500):
            error = urllib.error.HTTPError('https://quay.io/v2/auth', code, 'fixture error', {}, None)
            with self.subTest(code=code), patch.object(api.opener, 'open', side_effect=error) as send, self.assertRaises(ValueError):
                api.delete('toolbox', NAME)
            self.assertEqual(send.call_count, 1)

    def test_delete_404_is_idempotent_but_other_errors_stop_without_fallback(self):
        api = self.api()
        for code in (302, 401, 403, 404, 429, 500):
            error = urllib.error.HTTPError('https://quay.io/v2/bundledock/toolbox/manifests/' + NAME, code, 'fixture error', {}, None)
            with self.subTest(code=code), patch.object(api.opener, 'open', side_effect=[response(200, dict(token='fixture.jwt')), error]) as send:
                if code == 404:
                    api.delete('toolbox', NAME)
                else:
                    with self.assertRaises(ValueError):
                        api.delete('toolbox', NAME)
                self.assertEqual(send.call_count, 2)
        with patch.object(api.opener, 'open', side_effect=[response(200, dict(token='fixture.jwt')), response(200)]), self.assertRaises(ValueError):
            api.delete('toolbox', NAME)

    def test_public_inventory_does_not_exchange_or_send_robot_credentials(self):
        api = self.api()
        with patch.object(api.opener, 'open', return_value=response(200, dict(page=1, tags=[], has_additional=False))) as send:
            self.assertEqual(api.list_tags('toolbox'), [])
        self.assertEqual(send.call_count, 1)
        request = send.call_args.args[0]
        self.assertEqual(request.get_method(), 'GET')
        self.assertTrue(request.full_url.startswith('https://quay.io/api/v1/repository/bundledock/toolbox/tag/?'))
        self.assertIsNone(request.get_header('Authorization'))
