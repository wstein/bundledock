import importlib.util
import json
from pathlib import Path
import tempfile
import subprocess
import unittest
from unittest.mock import patch

SPEC = importlib.util.spec_from_file_location('publication_docs', Path(__file__).with_name('publication-docs.py'))

class PublicationDocsTest(unittest.TestCase):
    def setUp(self):
        self.module = importlib.util.module_from_spec(SPEC)
        SPEC.loader.exec_module(self.module)
        self.receipt = dict(image='quay.io/bundledock/toolbox', tag='2026.10.12-b5.1', commit='a'*40,
            source='https://github.com/wstein/bundledock', created='2026-10-12T12:00:00Z',
            lock_sha256='b'*64, run_id='123', digest='sha256:'+'c'*64,
            reference='quay.io/bundledock/toolbox@sha256:'+'c'*64, platforms=[])
        for arch in ['amd64', 'arm64']:
            self.receipt['platforms'].append({**{k:self.receipt[k] for k in ['image','tag','commit','source','created','lock_sha256','run_id']},
                'architecture':arch, 'image_id':'sha256:'+'d'*64, 'smoked':True, 'digest':'sha256:'+'e'*64})
    def validate(self):
        return self.module.validate(self.receipt, 'toolbox', 'quay.io/bundledock/toolbox', 'a'*40, '123', '1')
    def test_daily_revision_preserves_explicit_attempt_and_historical_order(self):
        self.receipt.update(tag='2026.10.12-b1', run_attempt='1')
        for platform in self.receipt['platforms']:
            platform.update(tag=self.receipt['tag'], run_attempt='1')
        record = self.validate()
        self.assertEqual(record['attempt'], '1')
        self.assertGreater(self.module.version_key('2026.10.12-b1'), self.module.version_key('2026.10.12-b98.1'))
        self.assertLess(self.module.version_key('2026.10.12-b9'), self.module.version_key('2026.10.13-b1'))
        original = self.receipt['created']
        self.receipt['created'] = '2026-10-13T00:00:00Z'
        for platform in self.receipt['platforms']:
            platform['created'] = self.receipt['created']
        with self.assertRaises(ValueError): self.validate()
        self.receipt['created'] = original
        for platform in self.receipt['platforms']:
            platform['created'] = original
        self.receipt['platforms'][0]['run_attempt'] = '2'
        with self.assertRaises(ValueError): self.validate()

    def test_valid_and_invalid_receipts(self):
        self.validate()
        for key, value in [('image','quay.io/evil/toolbox'), ('source','https://evil.test'), ('commit','a'*39), ('digest','sha256:bad'), ('reference','quay.io/bundledock/toolbox@sha256:'+'f'*64), ('run_id','124'), ('created','bad')]:
            with self.subTest(key=key):
                original=self.receipt[key]; self.receipt[key]=value
                with self.assertRaises(ValueError): self.validate()
                self.receipt[key]=original
        self.receipt['platforms'][0]['smoked']=False
        with self.assertRaises(ValueError): self.validate()
    def test_managed_only_idempotent_and_escape_rejected(self):
        record=self.validate()
        with tempfile.TemporaryDirectory() as directory:
            root=Path(directory); (root/'docs').mkdir()
            path=root/'docs/toolbox.md'
            original='before\n'+self.module.BEGIN+'\n'+self.module.END+'\nafter\n'
            path.write_text(original)
            self.assertTrue(self.module.update(root,'toolbox',record))
            content=path.read_text()
            self.assertTrue(content.startswith('before\n')); self.assertTrue(content.endswith('\nafter\n'))
            self.assertFalse(self.module.update(root,'toolbox',record))
            path.unlink(); path.symlink_to(root/'elsewhere')
            with self.assertRaises(ValueError): self.module.update(root,'toolbox',record)
    def test_all_ten_mappings_and_newer_guard(self):
        self.assertEqual(set(self.module.BUNDLES), {'toolbox','code-server','node','node-current','bun','codex','agy','claude','opentofu','nixbox'})
        record=self.validate()
        with tempfile.TemporaryDirectory() as directory:
            root=Path(directory); (root/'docs').mkdir()
            path=root/'docs/toolbox.md'; path.write_text(self.module.BEGIN+'\n'+self.module.END)
            self.module.update(root,'toolbox',record)
            older=dict(record,tag='2026.10.10-b4.1')
            with self.assertRaises(ValueError): self.module.update(root,'toolbox',older)

    def test_canonical_guides_accept_publication_without_changing_inventory_or_prose(self):
        repository = Path(__file__).resolve().parents[1]
        for bundle, relative in self.module.BUNDLES.items():
            with self.subTest(bundle=bundle), tempfile.TemporaryDirectory() as directory:
                root = Path(directory)
                path = root / relative
                path.parent.mkdir(parents=True)
                source = (repository / relative).read_bytes()
                path.write_bytes(source)
                receipt = json.loads(json.dumps(self.receipt))
                receipt.update(bundle=bundle, image='quay.io/bundledock/' + self.module._CONFIG.select_bundle(bundle)['repository'],
                               tag='2099.01.01-b1.1' if bundle in ('toolbox', 'nixbox') else '999.0.0-b1',
                               run_attempt='1')
                receipt['reference'] = receipt['image'] + '@' + receipt['digest']
                for platform in receipt['platforms']:
                    platform.update({key: receipt[key] for key in self.module.FIELDS})
                    platform['run_attempt'] = '1'
                    platform['bundle'] = bundle
                record = self.module.validate(receipt, bundle, receipt['image'], 'a'*40, '123', '1')
                self.assertTrue(self.module.update(root, bundle, record))
                prefix, managed = source.decode().split(self.module.BEGIN)
                _, suffix = managed.split(self.module.END)
                expected = prefix + self.module.BEGIN + self.module.render(record) + self.module.END + suffix
                self.assertEqual(path.read_bytes(), expected.encode())
                if bundle != 'nixbox':  # nixbox has a hand-written guide, no generated tool inventory
                    self.assertIn('<!-- BEGIN GENERATED TOOL INVENTORY -->', suffix)
                self.assertFalse(self.module.update(root, bundle, record))

    def test_missing_duplicate_and_reversed_publication_markers_fail_without_write(self):
        begin, end = self.module.BEGIN, self.module.END
        for source in ['prose only', begin, end, begin + end + begin,
                       begin + end + end, end + begin]:
            with self.subTest(source=source), tempfile.TemporaryDirectory() as directory:
                root = Path(directory)
                path = root / self.module.BUNDLES['toolbox']
                path.parent.mkdir(parents=True)
                path.write_text(source)
                with self.assertRaises(ValueError):
                    self.module.update(root, 'toolbox', self.validate())
                self.assertEqual(path.read_text(), source)

    def test_quickstart_markers_and_replacement(self):
        record = self.validate()
        readme = 'pre\n' + self.module.QUICKSTART_BEGIN + '\nold\n' + self.module.QUICKSTART_END + '\npost\n'
        replaced = self.module.replacement_quickstart(readme, record)
        self.assertIn(self.module.QUICKSTART_BEGIN, replaced)
        self.assertIn('TOOLBOX_IMAGE=' + record['reference'], replaced)
        self.assertIn(self.module.QUICKSTART_END, replaced)
        self.assertEqual(self.module.replacement_quickstart(replaced, record), replaced)
        for bad in ['no markers', self.module.QUICKSTART_BEGIN, self.module.QUICKSTART_END]:
            with self.assertRaises(ValueError):
                self.module.replacement_quickstart(bad, record)

    def forge(self, **options):
        """A credential-free fake of git, gh and the Git data API for the rolling topic."""
        module = self.module
        base, head, new_head, tree = '1'*40, '2'*40, '8'*40, '3'*40
        record = self.validate()
        other = dict(record, tag='2026.10.13-b1.1', run_id='124', commit='9'*40, digest='sha256:'+'f'*64,
                     reference='quay.io/bundledock/toolbox@sha256:'+'f'*64, image='quay.io/bundledock/toolbox')
        claude = dict(other, image='quay.io/bundledock/claude', tag='2.1.3-b1', attempt='1', run_attempt='1', bundle='claude',
                      reference='quay.io/bundledock/claude@sha256:'+'f'*64)
        begin = lambda name: '\npre-' + name + '\n' + module.BEGIN + '\n' + module.END + '\npost\n'
        sources = {(base, path): begin(name) for name, path in module.BUNDLES.items()}
        sources[(base, 'README.md')] = '\nhead\n' + module.QUICKSTART_BEGIN + '\n' + module.QUICKSTART_END + '\nfoot\n'
        pending = options.get('pending')
        if pending:
            for name, rec in pending.items():
                sources[(head, module.BUNDLES[name])] = module.replacement(sources[(base, module.BUNDLES[name])], rec)
                sources[(head, module.metadata_path(name))] = module.metadata(name, rec)
        state = dict(calls=[], api=[], branch_sha=head if pending else None)
        def source_at(revision, path):
            if (revision, path) in sources: return sources[(revision, path)]
            if revision == head and (base, path) in sources: return sources[(base, path)]
            raise subprocess.CalledProcessError(128, ['git'])
        def command(*args):
            state['calls'].append(args)
            if args[:3] == ('git', 'status', '--porcelain'): return ''
            if args[:2] == ('gh', 'api'): return 'main'
            if args[:4] == ('git', 'remote', 'get-url', 'origin'): return module.SOURCE + '.git'
            if args[:2] == ('git', 'fetch'): return ''
            if args[:3] == ('git', 'rev-parse', 'origin/main'): return base
            if args[:3] == ('git', 'rev-parse', 'FETCH_HEAD'): return head
            if args[:2] == ('git', 'rev-parse') and args[2].endswith('^'): return base if options.get('parent', base) == base else '5'*40
            if args[:2] == ('git', 'rev-list'): return head + ' ' + base + (' ' + '4'*40 if options.get('merge') else '')
            if args[:2] == ('git', 'diff-tree'): return '\n'.join(options.get('touched', ['docs/toolbox.md']))
            if args[:2] == ('git', 'ls-tree'): return ''
            if args[:2] == ('git', 'ls-remote'): return ('4'*40 if options.get('moved') else base) + '\trefs/heads/main'
            if args[:3] == ('gh', 'pr', 'list'):
                if not pending: return '[]'
                return json.dumps([dict(number=7, headRefName=options.get('ref', module.PENDING), headRefOid=head,
                                        baseRefName='other' if options.get('wrong_pr') else 'main', isCrossRepository=False)])
            if args[:3] in (('gh', 'pr', 'create'), ('gh', 'pr', 'edit'), ('gh', 'workflow', 'run')): return 'ok'
            raise AssertionError(args)
        def api(path, payload=None, method='POST'):
            state['api'].append((path, payload, method))
            if path == 'git/commits/' + base: return {'tree': {'sha': '5'*40}}
            if path == 'git/commits/' + head: return {'tree': {'sha': options.get('head_tree', '6'*40)}}
            if path == 'git/trees':
                state['tree'] = payload['tree']
                return {'sha': options.get('tree', tree)}
            if path == 'git/commits':
                self.assertEqual(payload['parents'], [base])
                return {'sha': new_head}
            if path == 'git/refs':
                if options.get('race'): raise subprocess.CalledProcessError(1, ['gh'], stderr='HTTP 422')
                state['branch_sha'] = new_head
                return {}
            if path == 'git/refs/heads/' + module.PENDING:
                self.assertEqual((payload, method), ({'sha': new_head, 'force': True}, 'PATCH'))
                state['branch_sha'] = new_head
                return {}
            if path == 'git/ref/heads/' + module.PENDING:
                if state['branch_sha'] is None and not options.get('stale'):
                    raise subprocess.CalledProcessError(1, ['gh'], stderr='HTTP 404')
                state['reads'] = state.get('reads', 0) + 1
                late = options.get('lease_late') and state['reads'] >= 2
                return {'object': {'sha': '7'*40 if options.get('lease') or late else (state['branch_sha'] or head)}}
            raise AssertionError(path)
        return module, record, other, claude, state, source_at, command, api

    def propose(self, bundle, record, **options):
        module, base_record, other, claude, state, source_at, command, api = self.forge(**options)
        records = {'toolbox': base_record, 'other': other, 'claude': claude}
        record = records[record] if isinstance(record, str) else record
        old = Path.cwd()
        try:
            with tempfile.TemporaryDirectory() as directory, patch.object(module, 'command', side_effect=command), patch.object(module, 'api', side_effect=api), patch.object(module, 'source_at', side_effect=source_at):
                try: result = module.propose(Path(directory), bundle, record)
                except ValueError as error: result = error
        finally: module.os.chdir(old)
        return result, state

    def forge_pending(self, **options):
        module, base_record, other, claude, *_ = self.forge()
        return {'claude': claude}

    def test_first_publication_opens_the_rolling_topic_and_dispatches_ci_once(self):
        result, state = self.propose('toolbox', 'toolbox')
        self.assertEqual(result, 'ok')
        paths = [change['path'] for change in state['tree']]
        self.assertEqual(paths, ['docs/toolbox.md', 'README.md', 'docs/toolbox-published.json'])
        self.assertIn(('git/refs', {'ref': 'refs/heads/publication-docs/pending', 'sha': '8'*40}, 'POST'), state['api'])
        creates = [call for call in state['calls'] if call[:3] == ('gh', 'pr', 'create')]
        self.assertEqual(len(creates), 1)
        self.assertEqual(creates[0][creates[0].index('--head') + 1], 'publication-docs/pending')
        self.assertTrue(creates[0][creates[0].index('--title') + 1].startswith('docs(toolbox): record publication 2026.10.12-b5.1'))
        runs = [call for call in state['calls'] if call[:3] == ('gh', 'workflow', 'run')]
        self.assertEqual(len(runs), 1)
        self.assertIn('expected_head_sha=' + '8'*40, runs[0])
        self.assertFalse(any(call[:2] == ('git', 'push') for call in state['calls']))

    def test_later_publication_rebuilds_one_commit_with_every_pending_bundle(self):
        module, _, _, claude, *_ = self.forge()
        result, state = self.propose('toolbox', 'toolbox', pending={'claude': claude})
        self.assertEqual(result, 'Updated publication PR')
        paths = sorted(change['path'] for change in state['tree'])
        self.assertEqual(paths, ['README.md', 'docs/claude-published.json', 'docs/claude.md', 'docs/toolbox-published.json', 'docs/toolbox.md'])
        self.assertIn(('git/refs/heads/publication-docs/pending', {'sha': '8'*40, 'force': True}, 'PATCH'), state['api'])
        edits = [call for call in state['calls'] if call[:3] == ('gh', 'pr', 'edit')]
        self.assertEqual(len(edits), 1)
        title, body = edits[0][edits[0].index('--title') + 1], edits[0][edits[0].index('--body') + 1]
        self.assertEqual(title, 'docs(publication): record toolbox 2026.10.12-b5.1, claude 2.1.3-b1')
        self.assertIn('| `claude` | `2.1.3-b1` |', body); self.assertIn('| `toolbox` | `2026.10.12-b5.1` |', body)
        self.assertFalse(any(call[:3] == ('gh', 'pr', 'create') for call in state['calls']))
        runs = [call for call in state['calls'] if call[:3] == ('gh', 'workflow', 'run')]
        self.assertEqual(len(runs), 1); self.assertIn('expected_head_sha=' + '8'*40, runs[0])

    def test_same_publication_again_keeps_the_head_but_still_runs_ci(self):
        module, _, _, claude, *_ = self.forge()
        # The rebuilt tree equals the topic's tree and its parent is current main.
        result, state = self.propose('claude', claude, pending={'claude': claude}, head_tree='3'*40)
        self.assertEqual(result, 'Updated publication PR')
        self.assertFalse(any(call[0] in ('git/commits',) or call[0].startswith('git/refs') for call in state['api']))
        runs = [call for call in state['calls'] if call[:3] == ('gh', 'workflow', 'run')]
        self.assertEqual(len(runs), 1); self.assertIn('expected_head_sha=' + '2'*40, runs[0])

    def test_guards_refuse_without_any_write_or_dispatch(self):
        module, _, _, claude, *_ = self.forge()
        older = dict(claude, tag='2.1.2-b1')
        cases = [('moved main', 'toolbox', 'toolbox', dict(moved=True)),
                 ('wrong pr base', 'toolbox', 'toolbox', dict(pending={'claude': claude}, wrong_pr=True)),
                 ('wrong pr branch', 'toolbox', 'toolbox', dict(pending={'claude': claude}, ref='publication-docs/other')),
                 ('topic moved (lease)', 'toolbox', 'toolbox', dict(pending={'claude': claude}, lease=True)),
                 ('topic moved late (lease)', 'toolbox', 'toolbox', dict(pending={'claude': claude}, lease_late=True)),
                 ('merge commit on topic', 'toolbox', 'toolbox', dict(pending={'claude': claude}, merge=True)),
                 ('topic touches other files', 'toolbox', 'toolbox', dict(pending={'claude': claude}, touched=['docs/toolbox.md', '.github/workflows/ci.yml'])),
                 ('stale topic without PR', 'toolbox', 'toolbox', dict(stale=True)),
                 ('creation race', 'toolbox', 'toolbox', dict(race=True)),
                 ('older than pending', 'claude', older, dict(pending={'claude': claude}))]
        for name, bundle, record, options in cases:
            with self.subTest(name):
                result, state = self.propose(bundle, record, **options)
                self.assertIsInstance(result, ValueError)
                self.assertFalse(any(call[:3] in (('gh', 'workflow', 'run'), ('gh', 'pr', 'create'), ('gh', 'pr', 'edit')) for call in state['calls']))
                self.assertFalse(any(call[2] == 'PATCH' or (call[0] == 'git/refs' and name != 'creation race') for call in state['api']))

    def test_pending_records_are_validated_and_conflicting_same_version_refused(self):
        module, _, _, claude, *_ = self.forge()
        for key, value in (('commit', 'zz'), ('digest', 'sha256:bad'), ('reference', 'quay.io/x@sha256:' + 'f'*64), ('source', 'https://evil.test'), ('run_id', '0'), ('tag', 'garbage')):
            with self.subTest(key=key):
                result, state = self.propose('toolbox', 'toolbox', pending={'claude': dict(claude, **{key: value})})
                self.assertIsInstance(result, ValueError)
                self.assertFalse(any(call[:3] in (('gh', 'workflow', 'run'), ('gh', 'pr', 'edit')) for call in state['calls']))
        with self.assertRaisesRegex(ValueError, 'conflicting'):
            module.merge_entry({'claude': (claude, None)}, 'claude', dict(claude, digest='sha256:' + 'a'*64, reference=claude['image'] + '@sha256:' + 'a'*64), None)
        self.assertIn('claude', module.merge_entry({'claude': (claude, None)}, 'claude', dict(claude), None))

    def test_proposal_paths_are_limited_to_publication_markdown_and_metadata(self):
        allowed = self.module.allowed_paths()
        self.assertIn('docs/toolbox.md', allowed); self.assertIn('README.md', allowed)
        self.assertIn('docs/node-current-published.json', allowed)
        for path in ('.github/workflows/ci.yml', 'bundles/toolbox/packages.lock.json', 'scripts/publication-docs.py'):
            self.assertNotIn(path, allowed)
        module, record, *_ = self.forge()
        with patch.object(module, 'source_at', return_value='x'), patch.object(module, 'allowed_paths', return_value=set()), patch.object(module, 'replacement', return_value='y'), patch.object(module, 'replacement_quickstart', return_value='z'), patch.object(module, 'command', return_value=''):
            with self.assertRaises(ValueError): module.build_changes('1'*40, {'toolbox': (record, None)})

    def test_rolling_job_serialises_runs(self):
        workflow = (Path(__file__).resolve().parents[1] / '.github/workflows/bundle-publish.yml').read_text()
        docs = workflow.split('\n  documentation:\n')[1].split('\n  cleanup:\n')[0]
        self.assertEqual([line.strip() for line in docs.splitlines() if 'group:' in line], ['group: bundledock-publication-docs'])
        self.assertIn('cancel-in-progress: false', docs)
        self.assertIn('queue: max', docs)

    def test_docs_job_finds_this_runs_receipt_after_a_failed_job_rerun(self):
        workflow = (Path(__file__).resolve().parents[1] / '.github/workflows/bundle-publish.yml').read_text()
        docs = workflow.split('\n  documentation:\n')[1].split('\n  cleanup:\n')[0]
        # A rerun of failed jobs has a new attempt, but the receipt keeps the
        # attempt that produced it: download this run's receipt by pattern and
        # hand its own attempt to the script, which checks it against the receipt.
        self.assertNotIn('-publication-${{ github.run_attempt }}', docs)
        self.assertIn('pattern: ${{ inputs.bundle }}-publication-*', docs)
        self.assertNotIn('run-id:', docs)
        self.assertIn('Expected exactly one publication receipt of this run', docs)
        self.assertIn('--attempt "$PUBLICATION_ATTEMPT"', docs)
        self.assertNotIn('--attempt "$GITHUB_RUN_ATTEMPT"', docs)
        self.assertIn('--run-id "$GITHUB_RUN_ID"', docs)

    def test_every_bundle_metadata_retains_canonical_evidence_without_enlarging_markdown(self):
        record = self.validate()
        evidence = {'receipt': self.receipt, 'attempt': {'id': 123, 'run_attempt': 1}}
        for bundle in self.module.BUNDLES:
            with self.subTest(bundle=bundle):
                self.assertEqual(self.module.metadata_path(bundle), 'docs/' + bundle + '-published.json')
                metadata = json.loads(self.module.metadata(bundle, record, evidence=evidence))
                self.assertEqual(metadata['publication_evidence'], evidence)
                self.assertNotIn('platforms', self.module.render(record))

    def test_node_metadata_tracks_receipt_not_source_inventory(self):
        for bundle, tag, upstream in [('node', '24.21.0-b2', '24.21.0'),
                                      ('node-current', '26.11.1-b1', '26.11.1'),
                                      ('node', '2026.10.12-b1.1', 'unknown')]:
            with self.subTest(bundle=bundle, tag=tag), tempfile.TemporaryDirectory() as directory:
                root = Path(directory); (root/'docs').mkdir()
                record = dict(self.validate(), bundle=bundle, tag=tag,
                              image='quay.io/bundledock/node')
                record['reference'] = record['image']+'@'+record['digest']
                guide = root/self.module.BUNDLES[bundle]
                guide.write_text(self.module.BEGIN+'\n'+self.module.END+'\nsource inventory: 999.0.0\n')
                self.assertTrue(self.module.update(root, bundle, record))
                metadata = json.loads((root/('docs/'+bundle+'-published.json')).read_text())
                self.assertEqual(metadata['upstream_version'], upstream)
                for key in ('tag', 'digest', 'commit', 'image'):
                    self.assertEqual(metadata[key], record[key])
                self.assertEqual(metadata['bundle'], bundle)
                self.assertFalse(self.module.update(root, bundle, record))
                # Restore stale metadata while the guide is already current.
                (root/('docs/'+bundle+'-published.json')).write_text('{}\n')
                self.assertTrue(self.module.update(root, bundle, record))
                self.assertEqual(json.loads((root/('docs/'+bundle+'-published.json')).read_text()), metadata)

    def test_legacy_metadata_preserves_only_matching_verified_publication(self):
        record = dict(self.validate(), bundle='node')
        previous = json.loads(self.module.metadata('node', record))
        previous['upstream_version'] = '24.21.0'
        self.assertEqual(json.loads(self.module.metadata('node', record, json.dumps(previous)))['upstream_version'], '24.21.0')
        for key in ('tag', 'digest', 'commit', 'image'):
            with self.subTest(key=key):
                changed = dict(previous); changed[key] = 'different'
                self.assertEqual(json.loads(self.module.metadata('node', record, json.dumps(changed)))['upstream_version'], 'unknown')

    def test_node_proposal_includes_managed_json_even_when_guide_is_current(self):
        record = dict(self.validate(), bundle='node-current', tag='26.11.1-b2',
                      image='quay.io/bundledock/node')
        source = self.module.BEGIN+'\n'+self.module.END
        desired = self.module.replacement(source, record)
        for guide in (source, desired):
            with self.subTest(current=guide == desired), tempfile.TemporaryDirectory() as directory:
                base = '1'*40
                def fake_command(*args):
                    if args[:2] == ('gh', 'api'): return 'main'
                    if args[:3] == ('git', 'remote', 'get-url'): return self.module.SOURCE
                    if args[:2] == ('git', 'rev-parse'): return base
                    if args[:3] == ('gh', 'pr', 'list'): return '[]'
                    if args[:2] == ('git', 'ls-remote'): return base + '\\trefs/heads/main'
                    return ''
                def fake_api(path, payload=None):
                    if path == 'git/commits/'+base: return {'tree': {'sha': '2'*40}}
                    if path.startswith('git/ref/heads/'): raise subprocess.CalledProcessError(1, ['gh'], stderr='HTTP 404')
                    self.assertEqual(path, 'git/trees')
                    changes = {item['path']: item['content'] for item in payload['tree']}
                    self.assertEqual(json.loads(changes.pop('docs/node-current-published.json'))['upstream_version'], '26.11.1')
                    self.assertEqual(changes, {} if guide == desired else {'docs/node-current.md': desired})
                    raise RuntimeError('checked proposal tree')
                old = Path.cwd()
                try:
                    with patch.object(self.module, 'command', side_effect=fake_command), patch.object(self.module, 'api', side_effect=fake_api), patch.object(self.module, 'source_at', side_effect=lambda base, path: '{}' if path.endswith('.json') else guide):
                        with self.assertRaisesRegex(RuntimeError, 'checked proposal tree'):
                            self.module.propose(Path(directory), 'node-current', record)
                finally:
                    self.module.os.chdir(old)

    def test_linked_metadata_rejected_before_guide_write(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory); (root/'docs').mkdir()
            guide = root/'docs/node.md'
            source = self.module.BEGIN+'\n'+self.module.END
            guide.write_text(source)
            (root/'docs/node-published.json').symlink_to(root/'elsewhere')
            with self.assertRaises(ValueError):
                self.module.update(root, 'node', dict(self.validate(), tag='24.21.0-b1'))
            self.assertEqual(guide.read_text(), source)

    def test_git_source_read_preserves_crlf(self):
        source=b'\r\n before  \r\n<!-- BEGIN PUBLISHED BUNDLE -->\r\n<!-- END PUBLISHED BUNDLE -->\r\nafter  \r\n\r\n'
        with patch.object(self.module.subprocess,'check_output',return_value=source) as call:
            self.assertEqual(self.module.source_at('a'*40,'docs/toolbox.md').encode('utf-8'),source)
            self.assertNotIn('text',call.call_args.kwargs)

    def test_shared_workflow_docs_job_and_caller_permissions(self):
        root=Path(__file__).resolve().parents[1]
        workflow=(root/'.github/workflows/bundle-publish.yml').read_text()
        self.assertIn('\n  documentation:\n',workflow)
        before, docs=workflow.split('\n  documentation:\n')
        self.assertIn('needs: [preflight, resolve, promote, tag]',docs)
        for condition in ["needs.promote.result == 'success'", "needs.tag.result == 'success'", "needs.resolve.outputs.publish == 'true'"]:
            self.assertIn(condition,docs)
        for permission in ['contents: write','pull-requests: write','actions: write']:
            self.assertIn(permission,docs)
        self.assertIn('path: ${{ runner.temp }}/publication-docs-receipt',docs)
        self.assertIn('ref: ${{ needs.preflight.outputs.sha }}',docs)
        self.assertIn('--propose',docs)
        self.assertNotIn('go build',docs)
        self.assertNotIn('docker ',docs)
        self.assertNotIn('hugo',docs)
        for bundle in self.module.BUNDLES:
            caller=(root/'.github/workflows'/'publish-bundles.yml').read_text()
            default, ceiling=caller.split('\njobs:\n')
            self.assertIn('actions: read',default)
            self.assertNotIn('pull-requests: write',default)
            self.assertIn('pull-requests: write',ceiling)
            self.assertIn('actions: write',ceiling)

class SemanticPublicationDocsTest(PublicationDocsTest):
    def test_semantic_receipt_and_legacy_migration(self):
        for item in [self.receipt,*self.receipt['platforms']]:
            item.update(bundle='node',image='quay.io/bundledock/node',tag='24.21.0-b1',run_attempt='3')
        self.receipt['reference']='quay.io/bundledock/node@'+self.receipt['digest']
        record=self.module.validate(self.receipt,'node','quay.io/bundledock/node','a'*40,'123','3')
        old=dict(record,tag='2026.10.12-b1.1',attempt='1'); old.pop('run_attempt')
        source=self.module.BEGIN+self.module.render(old)+self.module.END
        updated=self.module.replacement(source,record)
        self.assertIn('24.21.0-b1',updated)
        with self.assertRaises(ValueError): self.module.replacement(updated,old)
        newer=dict(record,tag='24.21.0-b2')
        self.module.replacement(updated,newer)
        self.receipt['platforms'][0]['run_attempt']='2'
        with self.assertRaises(ValueError): self.module.validate(self.receipt,'node','quay.io/bundledock/node','a'*40,'123','3')

class CurrentPublicationDocsTest(PublicationDocsTest):
    def test_shared_current_receipt_preserves_source_and_rejects_cross_channel(self):
        for item in [self.receipt,*self.receipt['platforms']]:
            item.update(bundle='node-current',image='quay.io/bundledock/node',tag='26.11.1-b1',run_attempt='1')
        self.receipt['reference']='quay.io/bundledock/node@'+self.receipt['digest']
        validate=lambda: self.module.validate(self.receipt,'node-current','quay.io/bundledock/node','a'*40,'123','1')
        record=validate()
        self.assertEqual(record['bundle'],'node-current')
        self.assertIn('docker pull quay.io/bundledock/node@',self.module.render(record))
        self.receipt['platforms'][1]['bundle']='node'
        with self.assertRaises(ValueError): validate()
        self.receipt['platforms'][1]['bundle']='node-current'
        del self.receipt['bundle']
        with self.assertRaises(ValueError): validate()

    def test_semantic_lts_receipt_identity_required_but_historical_date_allowed(self):
        for item in [self.receipt,*self.receipt['platforms']]:
            item.update(image='quay.io/bundledock/node',tag='26.11.1-b1',run_attempt='1')
        self.receipt['reference']='quay.io/bundledock/node@'+self.receipt['digest']
        validate=lambda: self.module.validate(self.receipt,'node','quay.io/bundledock/node','a'*40,'123','1')
        with self.assertRaises(ValueError): validate()
        for item in [self.receipt,*self.receipt['platforms']]: item['bundle']='node'
        validate()
        del self.receipt['platforms'][1]['bundle']
        with self.assertRaises(ValueError): validate()
        for item in [self.receipt,*self.receipt['platforms']]:
            item.pop('bundle',None);item['tag']='2026.10.12-b1.1'
        validate()


class ReadmePublishedBundleRowsTest(unittest.TestCase):
    def test_published_bundle_is_not_described_as_pending_in_readme(self):
        root = Path(__file__).resolve().parent.parent
        rows = [line for line in (root / 'README.md').read_text().splitlines() if line.startswith('| ')]
        receipts = sorted((root / 'docs').glob('*-published.json'))
        self.assertTrue(receipts)
        for receipt in receipts:
            bundle = receipt.name.removesuffix('-published.json')
            with self.subTest(bundle=bundle):
                matching = [row for row in rows if f'docs/{bundle}.md)' in row]
                self.assertEqual(len(matching), 1)
                self.assertNotRegex(matching[0].lower(), r'publication pending|source prepared')
