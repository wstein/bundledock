"""tool-upgrade.sh nixbox flow against a throwaway repository with inert adapters."""
import json
import os
from pathlib import Path
import shutil
import stat
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[2]
KEY = 'cache.nixos.org-1:'


def platform(tag):
    names = ['glibc-2.44-57', 'git-minimal-2.56.0']
    paths = {'/nix/store/' + tag * 32 + '-' + n: {'narHash': 'sha256-' + tag, 'narSize': 1, 'references': [],
                                                    'signatures': [KEY + tag * 8]} for n in names}
    return {'system': 'x86_64-linux', 'roots': {n.split('-')[0]: p for n, p in zip(names, paths)}, 'paths': paths}


OLD_REV, NEW_REV = 'a' * 40, 'b' * 40


def catalog(rev):
    return json.dumps({'schema': 1, 'nixpkgs': {'rev': rev, 'url': 'u', 'narHash': 'h'}}, indent=2) + '\n'


def lock(tag, digest='c' * 64):
    return {'schema': 1, 'catalog_digest': digest, 'platforms': {'amd64': platform(tag), 'arm64': platform(tag)}}


def write_exec(path, text):
    path.write_text(text)
    path.chmod(path.stat().st_mode | stat.S_IXUSR)


class NixboxUpgradeTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        tmp = Path(self.tmp.name).resolve()
        self.repo = tmp / 'repo'
        origin = tmp / 'origin.git'
        subprocess.run(['git', 'init', '-q', '--bare', str(origin)], check=True)
        (self.repo / 'scripts').mkdir(parents=True)
        for name in ('tool-upgrade.sh', 'bundle-config.py', 'updater-source-ci.py'):
            shutil.copy(ROOT / 'scripts' / name, self.repo / 'scripts' / name)
        box = self.repo / 'bundles/nixbox'
        (box / 'scripts').mkdir(parents=True)
        (box / 'packages.catalog.json').write_text(catalog(OLD_REV))
        (box / 'packages.lock.json').write_text(json.dumps(lock('a'), indent=2) + '\n')
        # Adapters: the bump writes catalog $NEW_CATALOG, the refresh writes lock $NEW_LOCK; no Docker, no network.
        write_exec(box / 'scripts/bump-rev.sh', '#!/bin/sh\nset -eu\necho bump >> "$CALLS"\n'
                   '[ -z "${NEW_CATALOG:-}" ] || cp "$NEW_CATALOG" bundles/nixbox/packages.catalog.json\n')
        write_exec(box / 'scripts/refresh-lock.sh', '#!/bin/sh\nset -eu\necho refresh >> "$CALLS"\n'
                   'cp "$NEW_LOCK" bundles/nixbox/packages.lock.json\n')
        self.git('init', '-q', '-b', 'main')
        self.git('config', 'user.name', 'Test')
        self.git('config', 'user.email', 'test@example.invalid')
        self.git('config', 'commit.gpgsign', 'false')
        self.git('add', '.')
        self.git('commit', '-q', '-m', 'fixture')
        self.git('remote', 'add', 'origin', str(origin))
        self.bin = tmp / 'bin'
        self.bin.mkdir()
        self.calls = tmp / 'calls'
        write_exec(self.bin / 'go', '#!/bin/sh\necho "go $*" >> "$CALLS"\n')
        write_exec(self.bin / 'gh', '#!/bin/sh\necho "gh $*" >> "$CALLS"\n'
                   'case "$1 $2" in "pr create") echo https://github.com/wstein/bundledock/pull/1;; esac\n')
        write_exec(self.bin / 'docker', '#!/bin/sh\necho "docker $*" >> "$CALLS"\nexit 99\n')
        self.result = tmp / 'result'
        self.output = tmp / 'output'
        self.summary = tmp / 'summary'
        self.new_lock = tmp / 'new.lock.json'
        self.new_catalog = tmp / 'new.catalog.json'
        # An inert updater-source-ci stand-in records the request; the real one is tested elsewhere.
        write_exec(self.repo / 'scripts/updater-source-ci.py', '#!/usr/bin/env python3\nimport os, sys\n'
                   'open(os.environ["CALLS"], "a").write("source-ci " + " ".join(sys.argv[1:]) + "\\n")\n')
        self.git('add', '.')
        self.git('commit', '-q', '-m', 'adapter')

    def git(self, *args):
        return subprocess.run(['git', *args], cwd=self.repo, check=True, capture_output=True, text=True).stdout.strip()

    def run_script(self, phase, new=None, rev=None, **env):
        self.new_catalog.write_text(catalog(rev or OLD_REV))
        if new is not None:
            self.new_lock.write_text(json.dumps(new, indent=2) + '\n')
        full = dict(os.environ, PATH=str(self.bin) + os.pathsep + os.environ['PATH'], CALLS=str(self.calls),
                    UPGRADE_RESULT=str(self.result), GITHUB_OUTPUT=str(self.output), GITHUB_STEP_SUMMARY=str(self.summary),
                    NEW_LOCK=str(self.new_lock), NEW_CATALOG=str(self.new_catalog), GITHUB_REPOSITORY='wstein/bundledock', GITHUB_RUN_ID='7',
                    GITHUB_RUN_ATTEMPT='1', DEFAULT_BRANCH='main', GITHUB_REF='refs/heads/main',
                    GITHUB_SHA=self.git('rev-parse', 'HEAD'))
        full.update(env)
        return subprocess.run(['bash', 'scripts/tool-upgrade.sh', phase], cwd=self.repo, env=full,
                              capture_output=True, text=True)

    def call_log(self):
        return self.calls.read_text().splitlines() if self.calls.exists() else []

    def resolve(self, new, **env):
        return self.run_script('resolve', new, BUNDLE='nixbox', **env)

    def test_resolve_bumps_the_rev_and_reports_the_change(self):
        result = self.resolve(lock('b'), rev=NEW_REV)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn('changed=true', self.output.read_text())
        self.assertEqual({p.name for p in self.result.iterdir()},
                         {'nixbox.lock.json', 'nixbox.catalog.json', 'base.sha', 'lock.diff'})
        self.assertIn(NEW_REV, (self.result / 'nixbox.catalog.json').read_text())
        self.assertIn('Proposed nixbox', self.summary.read_text())
        calls = self.call_log()
        self.assertLess(calls.index('bump'), calls.index('refresh'))
        self.assertTrue(any(c.startswith('go run ./cmd/nixbox check') for c in calls))
        self.assertFalse(any('tools upgrade' in c or c.startswith('docker') for c in calls), calls)

    def test_resolve_without_change_creates_nothing(self):
        result = self.resolve(lock('a'))
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn('changed=false', self.output.read_text())
        self.assertFalse((self.result / 'nixbox.lock.json').exists())

    def test_resolve_refuses_edits_outside_the_lock_and_catalog_and_a_tool_argument(self):
        write_exec(self.repo / 'bundles/nixbox/scripts/refresh-lock.sh',
                   '#!/bin/sh\ncp "$NEW_LOCK" bundles/nixbox/packages.lock.json\necho x > README\n')
        self.git('commit', '-qam', 'bad refresh')
        result = self.resolve(lock('b'))
        self.assertEqual(result.returncode, 1)
        self.assertIn('only the selected existing locks', result.stderr)
        self.git('reset', '-q', '--hard', 'HEAD~1')
        self.git('clean', '-fdq')
        result = self.resolve(lock('b'), TOOL='git')
        self.assertEqual(result.returncode, 1)
        self.assertIn('TOOL is not supported for nixbox', result.stderr)

    def publish_inputs(self, new, rev=NEW_REV):
        self.assertEqual(self.resolve(new, rev=rev).returncode, 0)
        self.git('checkout', '-q', '--', '.')
        # check_result publish requires a clean tree and the artifact files only.
        (self.result / 'lock.diff').unlink()

    def test_publish_opens_one_nixbox_pull_request_naming_the_revs(self):
        self.publish_inputs(lock('b'))
        self.calls.write_text('')
        result = self.run_script('publish', PROPOSAL_GROUP='nixbox')
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(self.git('branch', '--show-current'), 'automation/nixbox-upgrade-7-1')
        self.assertEqual(set(self.git('diff', '--name-only', 'main', 'HEAD').split()),
                         {'bundles/nixbox/packages.lock.json', 'bundles/nixbox/packages.catalog.json'})
        self.assertEqual(self.git('log', '-1', '--format=%s'), 'chore(nixbox): bump nixpkgs to bbbbbbbbbbbb')
        body = (self.result / 'pr-body.md').read_text()
        for text in (OLD_REV, NEW_REV, 'nixbox-build.yml'):
            self.assertIn(text, body)
        calls = self.call_log()
        self.assertTrue(any(c.startswith('gh pr create') and '--head automation/nixbox-upgrade-7-1' in c for c in calls))
        self.assertTrue(any(c.startswith('source-ci --bundles nixbox --branch automation/nixbox-upgrade-7-1') for c in calls))
        self.assertFalse(any('workflow' in c and 'run' in c.split() for c in calls if c.startswith('gh')), calls)

    def test_publish_without_artifact_changes_opens_nothing_and_ignores_the_default_group(self):
        self.publish_inputs(lock('b'))
        # The default publish group never picks the nixbox artifact up.
        shutil.copy(ROOT / 'bundles/nixbox/packages.lock.json', self.new_lock)
        result = self.run_script('publish')
        self.assertEqual(result.returncode, 1)
        self.assertEqual(self.git('branch', '--show-current'), 'main')

    def test_publish_refuses_a_stale_base_and_unknown_group(self):
        self.publish_inputs(lock('b'))
        (self.repo / 'README').write_text('x\n')
        self.git('add', '.')
        self.git('commit', '-q', '-m', 'main moved')
        result = self.run_script('publish', PROPOSAL_GROUP='nixbox')
        self.assertEqual(result.returncode, 1)
        self.assertIn('Refusing stale proposal', result.stderr)
        result = self.run_script('publish', PROPOSAL_GROUP='toolbox')
        self.assertEqual(result.returncode, 1)
        self.assertIn('PROPOSAL_GROUP must be empty or nixbox', result.stderr)


if __name__ == '__main__':
    unittest.main()
