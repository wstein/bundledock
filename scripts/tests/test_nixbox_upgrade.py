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


# Stand-in for gh: `pr create` answers a URL; `api` is a forge fake derived from the fixture repository,
# so the real scripts/updater-source-ci.py runs end to end. A POST (the CI dispatch) is only logged.
GH = '''#!/usr/bin/env python3
import json, os, subprocess, sys
args = sys.argv[1:]
with open(os.environ["CALLS"], "a") as log:
    log.write("gh " + " ".join(args) + "\\n")
if args[0] == "pr":
    print("https://github.com/wstein/bundledock/pull/1")
    sys.exit(0)
if "--method" in args:
    sys.stdin.read()
    sys.exit(0)
def git(*a): return subprocess.check_output(["git", *a], text=True).strip()
repo, endpoint = "wstein/bundledock", args[1].removeprefix("repos/wstein/bundledock")
head, base, branch = git("rev-parse", "HEAD"), git("rev-parse", "main"), git("branch", "--show-current")
files = [{"filename": f, "status": "modified"} for f in git("diff", "--name-only", base, head).split()]
data = {
    "": {"full_name": repo, "default_branch": "main"},
    "/pulls/1": {"number": 1, "state": "open", "draft": False, "merged": False,
                 "user": {"login": "github-actions[bot]", "type": "Bot"},
                 "head": {"sha": head, "ref": branch, "repo": {"full_name": repo}},
                 "base": {"sha": base, "ref": "main", "repo": {"full_name": repo}},
                 "commits": 1, "changed_files": len(files), "html_url": "https://github.com/wstein/bundledock/pull/1"},
    "/git/ref/heads/main": {"object": {"sha": base}},
    "/git/ref/heads/" + branch: {"object": {"sha": head}},
    "/pulls/1/files?per_page=100&page=1": files,
    "/compare/" + base + "..." + head: {"status": "ahead", "ahead_by": 1, "behind_by": 0, "total_commits": 1,
        "commits": [{"sha": head, "parents": [{"sha": base}]}], "files": files},
    "/actions/workflows/ci.yml": {"id": 7, "path": ".github/workflows/ci.yml", "state": "active"},
    "/actions/workflows/ci.yml/runs?branch=" + branch + "&event=workflow_dispatch&per_page=100&page=1":
        {"total_count": 0, "workflow_runs": []},
}
print(json.dumps(data[endpoint]))
'''


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
        for name in ('tool-upgrade.sh', 'bundle-config.py', 'updater-source-ci.py', 'tool-inventory.py'):
            shutil.copy(ROOT / 'scripts' / name, self.repo / 'scripts' / name)
        # The official release binaries (toolbox schema) and their generated inventory are the real ones.
        (self.repo / 'bundles/nixbox/tools').mkdir(parents=True)
        for name in ('packages.catalog.json', 'packages.lock.json'):
            shutil.copy(ROOT / 'bundles/nixbox/tools' / name, self.repo / 'bundles/nixbox/tools' / name)
        (self.repo / 'docs').mkdir()
        shutil.copy(ROOT / 'docs/nixbox.md', self.repo / 'docs/nixbox.md')
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
        # `go run ./cmd/bdock tools upgrade --directory bundles/nixbox/tools` rewrites the tools lock like the real resolver.
        write_exec(self.bin / 'go', '#!/bin/sh\necho "go $*" >> "$CALLS"\n'
                   'case "$*" in *"tools upgrade"*) [ ! -f "$NEW_TOOLS_LOCK" ] || cp "$NEW_TOOLS_LOCK" bundles/nixbox/tools/packages.lock.json;; esac\n')
        write_exec(self.bin / 'gh', GH)
        write_exec(self.bin / 'docker', '#!/bin/sh\necho "docker $*" >> "$CALLS"\nexit 99\n')
        self.result = tmp / 'result'
        self.output = tmp / 'output'
        self.summary = tmp / 'summary'
        self.new_lock = tmp / 'new.lock.json'
        self.new_catalog = tmp / 'new.catalog.json'
        self.new_tools_lock = tmp / 'new.tools.lock.json'

    def git(self, *args):
        return subprocess.run(['git', *args], cwd=self.repo, check=True, capture_output=True, text=True).stdout.strip()

    def run_script(self, phase, new=None, rev=None, **env):
        self.new_catalog.write_text(catalog(rev or OLD_REV))
        if new is not None:
            self.new_lock.write_text(json.dumps(new, indent=2) + '\n')
        full = dict(os.environ, PATH=str(self.bin) + os.pathsep + os.environ['PATH'], CALLS=str(self.calls),
                    UPGRADE_RESULT=str(self.result), GITHUB_OUTPUT=str(self.output), GITHUB_STEP_SUMMARY=str(self.summary),
                    NEW_LOCK=str(self.new_lock), NEW_CATALOG=str(self.new_catalog), NEW_TOOLS_LOCK=str(self.new_tools_lock), GITHUB_REPOSITORY='wstein/bundledock', GITHUB_RUN_ID='7',
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
                         {'nixbox.lock.json', 'nixbox.catalog.json', 'nixbox.tools.lock.json', 'nixbox.md', 'base.sha', 'lock.diff'})
        self.assertIn(NEW_REV, (self.result / 'nixbox.catalog.json').read_text())
        self.assertIn('Proposed nixbox', self.summary.read_text())
        calls = self.call_log()
        self.assertLess(calls.index('bump'), calls.index('refresh'))
        self.assertTrue(any(c.startswith('go run ./cmd/nixbox check') for c in calls))
        # The official release binaries resolve in the same proposal, from the stable releases, never in Docker.
        self.assertIn('go run ./cmd/bdock tools upgrade --directory bundles/nixbox/tools', calls)
        self.assertFalse(any(c.startswith('docker') for c in calls), calls)

    def test_resolve_without_change_creates_nothing(self):
        result = self.resolve(lock('a'))
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn('changed=false', self.output.read_text())
        self.assertFalse((self.result / 'nixbox.lock.json').exists())

    def tools_lock(self, version='v9.8.7'):
        lock = json.loads((ROOT / 'bundles/nixbox/tools/packages.lock.json').read_text())
        for arch in ('amd64', 'arm64'):
            lock['platforms'][arch]['zk']['version'] = version
        return json.dumps(lock, indent=2) + '\n'

    def test_resolve_proposes_a_new_official_binary_with_its_inventory_row(self):
        self.new_tools_lock.write_text(self.tools_lock())
        result = self.resolve(lock('a'))
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn('changed=true', self.output.read_text())
        self.assertIn('9.8.7', (self.result / 'nixbox.tools.lock.json').read_text())
        self.assertIn('| zk (`zk`) | 9.8.7 |', (self.result / 'nixbox.md').read_text())
        self.assertIn('9.8.7', (self.result / 'lock.diff').read_text())

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
        for text in (OLD_REV, NEW_REV, 'dispatched source CI', 'nixbox-build.yml'):
            self.assertIn(text, body)
        calls = self.call_log()
        self.assertTrue(any(c.startswith('gh pr create') and '--head automation/nixbox-upgrade-7-1' in c for c in calls))
        self.assertTrue(any(c.startswith('gh api') and c.endswith('--method POST --input -') and 'workflows/ci.yml/dispatches' in c
                            for c in calls), calls)
        self.assertFalse(any('workflow' in c and 'run' in c.split() for c in calls if c.startswith('gh')), calls)

    def test_publish_opens_one_pull_request_for_an_official_binary_bump(self):
        self.new_tools_lock.write_text(self.tools_lock())
        self.publish_inputs(lock('a'), rev=OLD_REV)
        self.calls.write_text('')
        result = self.run_script('publish', PROPOSAL_GROUP='nixbox')
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(set(self.git('diff', '--name-only', 'main', 'HEAD').split()),
                         {'bundles/nixbox/tools/packages.lock.json', 'docs/nixbox.md'})
        self.assertEqual(self.git('log', '-1', '--format=%s'), 'chore(nixbox): upgrade official tools')
        body = (self.result / 'pr-body.md').read_text()
        self.assertIn('zk:', body)
        self.assertIn('v9.8.7', body)
        self.assertTrue(any(c.startswith('gh pr create') for c in self.call_log()))

    def test_publish_names_both_changes_when_the_pin_and_a_tool_move(self):
        self.new_tools_lock.write_text(self.tools_lock())
        self.publish_inputs(lock('b'))
        result = self.run_script('publish', PROPOSAL_GROUP='nixbox')
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(self.git('log', '-1', '--format=%s'), 'chore(nixbox): bump nixpkgs to bbbbbbbbbbbb and upgrade official tools')

    def test_publish_refuses_an_inventory_that_differs_from_the_generated_one(self):
        self.new_tools_lock.write_text(self.tools_lock())
        self.publish_inputs(lock('b'))
        guide = self.result / 'nixbox.md'
        guide.write_text(guide.read_text().replace('9.8.7', '1.2.3'))
        result = self.run_script('publish', PROPOSAL_GROUP='nixbox')
        self.assertEqual(result.returncode, 1)
        self.assertIn('documentation artifact differs', result.stderr)
        self.assertEqual(self.git('branch', '--show-current'), 'main')

    def test_publish_refuses_a_tools_lock_that_fails_the_check(self):
        self.new_tools_lock.write_text(self.tools_lock())
        self.publish_inputs(lock('b'))
        write_exec(self.bin / 'go', '#!/bin/sh\necho "go $*" >> "$CALLS"\ncase "$*" in *"cmd/nixbox check"*"-tools bundles/nixbox/tools"*) exit 7;; esac\n')
        result = self.run_script('publish', PROPOSAL_GROUP='nixbox')
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(self.git('branch', '--show-current'), 'main')
        self.assertTrue(any('-tools bundles/nixbox/tools' in c for c in self.call_log()))

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
