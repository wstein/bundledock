import base64
import importlib.util
import json
from pathlib import Path
import unittest

SPEC = importlib.util.spec_from_file_location('pending', Path(__file__).parents[1] / 'publication-pending.py')
pending = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(pending)

HEAD, OLD, NEW, TAGGED = 'a' * 40, 'b' * 40, 'c' * 40, 'd' * 40
LOCK = pending.approval.config.select_bundle('claude')['lock']


def b64(raw):
    return base64.b64encode(raw).decode()


class FakeApi:
    def __init__(self, bundle='claude', head_lock=b'new', published_lock=b'old', metadata=OLD, tags=()):
        self.bundle = bundle
        lock = pending.approval.config.select_bundle(bundle)['lock']
        record = {'bundle': bundle, 'commit': metadata}
        self.data = {
            '/git/ref/heads/main': {'object': {'sha': HEAD}},
            '/contents/' + lock + '?ref=' + HEAD: {'encoding': 'base64', 'content': b64(head_lock)},
            '/contents/' + lock + '?ref=' + OLD: {'encoding': 'base64', 'content': b64(published_lock)},
            '/contents/docs?ref=main': [{'name': bundle + '-published.json'}] if metadata else [],
            '/contents/docs/' + bundle + '-published.json?ref=main': {
                'type': 'file', 'encoding': 'base64', 'content': b64(json.dumps(record).encode())},
            '/git/matching-refs/tags/' + bundle + '/': list(tags),
            '/commits?path=' + lock + '&sha=' + HEAD + '&per_page=1': [{'sha': NEW}],
            '/commits/' + NEW + '/pulls?per_page=100&page=1': [{'number': 165}],
        }
        if bundle == 'nixbox':  # the lock of its official release binaries, same at every revision by default
            for revision in (HEAD, OLD, TAGGED):
                self.data['/contents/bundles/nixbox/tools/packages.lock.json?ref=' + revision] = {'encoding': 'base64', 'content': b64(b'tools')}
        self.reads = []

    def get(self, path):
        self.reads.append(path)
        return self.data[path]


class PublicationPendingTests(unittest.TestCase):
    def nixbox_api(self, head_tools, published_tools):
        api = FakeApi('nixbox', head_lock=b'nix', published_lock=b'nix')
        tools = 'bundles/nixbox/tools/packages.lock.json'
        api.data['/contents/' + tools + '?ref=' + HEAD] = {'encoding': 'base64', 'content': b64(head_tools)}
        api.data['/contents/' + tools + '?ref=' + OLD] = {'encoding': 'base64', 'content': b64(published_tools)}
        return api

    def test_nixbox_tools_lock_change_alone_is_unpublished(self):
        self.assertEqual(pending.inspect('nixbox', self.nixbox_api(b'same', b'same'))['state'], 'published')
        result = pending.inspect('nixbox', self.nixbox_api(b'new tools', b'old tools'))
        self.assertEqual(result['state'], 'unpublished')

    def test_lock_matching_recorded_publication_is_published(self):
        api = FakeApi(head_lock=b'same', published_lock=b'same')
        self.assertEqual(pending.inspect('claude', api)['state'], 'published')

    def test_lock_ahead_of_publication_lists_commit_and_pull_request(self):
        result = pending.inspect('claude', FakeApi())
        self.assertEqual(result, {'bundle': 'claude', 'main': HEAD, 'state': 'unpublished',
                                  'lock_commit': NEW, 'pull_requests': [165]})

    def test_annotated_source_tag_proves_publication_before_docs_pr_merges(self):
        api = FakeApi(tags=[{'ref': 'refs/tags/claude/2.1.303-b1', 'object': {'type': 'tag', 'sha': 'e' * 40}}])
        api.data['/git/tags/' + 'e' * 40] = {'object': {'type': 'commit', 'sha': TAGGED}}
        api.data['/contents/' + LOCK + '?ref=' + TAGGED] = {'encoding': 'base64', 'content': b64(b'new')}
        result = pending.inspect('claude', api)
        self.assertEqual((result['state'], result['source_sha']), ('published', TAGGED))

    def test_historical_tag_commit_without_lock_is_skipped_not_pending(self):
        class Api(FakeApi):
            def get(self, path):
                if path.endswith('?ref=' + TAGGED):
                    raise pending.Pending('missing lock path at historical commit')
                return super().get(path)
        api = Api(tags=[{'ref': 'refs/tags/claude/old', 'object': {'type': 'commit', 'sha': TAGGED}}])
        self.assertEqual(pending.inspect('claude', api)['state'], 'unpublished')

    def test_no_metadata_and_no_tags_is_no_baseline(self):
        self.assertEqual(pending.inspect('claude', FakeApi(metadata=None))['state'], 'no-baseline')

    def test_unavailable_evidence_is_pending_and_nonzero(self):
        class Down:
            def get(self, path):
                raise pending.Pending('unavailable')
        results = pending.survey(['claude'], Down())
        self.assertEqual(results, [{'bundle': 'claude', 'state': 'pending'}])
        self.assertEqual(pending.exit_code(results), 2)

    def test_exit_code_zero_only_when_all_published(self):
        self.assertEqual(pending.exit_code([{'state': 'published'}]), 0)
        for state in ('unpublished', 'no-baseline', 'pending'):
            self.assertEqual(pending.exit_code([{'state': 'published'}, {'state': state}]), 2)

    def test_foreign_metadata_is_rejected(self):
        api = FakeApi()
        api.data['/contents/docs/claude-published.json?ref=main']['content'] = b64(json.dumps({'bundle': 'codex', 'commit': OLD}).encode())
        with self.assertRaises(ValueError):
            pending.inspect('claude', api)

    def test_helper_only_reads(self):
        api = FakeApi()
        pending.inspect('claude', api)
        self.assertTrue(all(path.startswith('/') for path in api.reads))
        self.assertFalse(hasattr(api, 'post'))

    def test_all_bundles_have_a_valid_lock_path(self):
        for name in pending.approval.config.BUNDLES:
            api = FakeApi(bundle=name, head_lock=b'same', published_lock=b'same')
            self.assertEqual(pending.inspect(name, api)['state'], 'published')


if __name__ == '__main__':
    unittest.main()
