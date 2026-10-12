"""Source-only fixtures: workflow shell runs against inert command adapters."""
import json
import os
from pathlib import Path
import re
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[2]
SHA = "a" * 40
OTHER = "b" * 40


class ToolboxBuildWorkflowTests(unittest.TestCase):
    def workflow(self):
        return (ROOT / ".github/workflows/toolbox-build.yml").read_text()

    def commands(self):
        return re.findall(r"^        run: (.+)$", self.workflow(), re.M)

    def invoke(self, changes=None):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            log = root / "calls"
            adapter = r"""#!/usr/bin/env python3
import json, os, pathlib, sys
name = pathlib.Path(sys.argv[0]).name
args = sys.argv[1:]
with open(os.environ['CALLS'], 'a') as calls:
    calls.write(json.dumps([name] + args) + '\n')
if name == 'git':
    if args == ['rev-parse', 'HEAD']:
        print(os.environ['CHECKOUT_SHA'])
    elif args == ['ls-remote', 'origin', 'refs/heads/main']:
        if os.environ.get('REMOTE_FAILURE') == '1':
            sys.exit(1)
        print(os.environ['MAIN_SHA'] + '\trefs/heads/main')
    else:
        sys.exit('unexpected git call')
elif name != 'make':
    sys.exit('unexpected external operation')
"""
            for name in ("git", "make", "docker", "gh", "curl"):
                path = root / name
                path.write_text(adapter)
                path.chmod(0o700)
            env = dict(os.environ, PATH=str(root) + os.pathsep + os.environ["PATH"],
                       CALLS=str(log), GITHUB_EVENT_NAME="workflow_dispatch",
                       DEFAULT_BRANCH="main", GITHUB_REF="refs/heads/main",
                       SENDER_TYPE="User", GITHUB_ACTOR="maintainer",
                       TRIGGERING_ACTOR="maintainer", GITHUB_SHA=SHA,
                       APPROVED_SHA=SHA, CHECKOUT_SHA=SHA, MAIN_SHA=SHA)
            env.update(changes or {})
            result = subprocess.run(["sh", "-ec", "\n".join(self.commands())],
                                    cwd=ROOT, env=env, capture_output=True, text=True)
            calls = [json.loads(line) for line in log.read_text().splitlines()] if log.exists() else []
            return result, calls

    def test_approved_source_builds_and_smokes_local_image(self):
        result, calls = self.invoke()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(calls[:2], [["git", "rev-parse", "HEAD"],
                                     ["git", "ls-remote", "origin", "refs/heads/main"]])
        self.assertEqual(calls[2:], [
            ["make", "bundle-build", "BUNDLE=toolbox", "IMAGE=bundledock-toolbox:build-only"],
            ["make", "bundle-smoke", "BUNDLE=toolbox", "IMAGE=bundledock-toolbox:build-only",
             "CONSUMER=mcr.microsoft.com/devcontainers/go:2-1.27-trixie@sha256:8f1c83128e4bce9d745b1dc0f28f8c027e303d3886e2434966ff135296c41770"],
        ])

    def test_rejections_never_build_or_write(self):
        cases = [
            {"GITHUB_EVENT_NAME": event} for event in ("push", "schedule", "workflow_run", "pull_request")
        ] + [
            {"DEFAULT_BRANCH": "develop"}, {"GITHUB_REF": "refs/heads/topic"},
            {"SENDER_TYPE": "Bot"}, {"SENDER_TYPE": ""},
        ]
        cases += [{"APPROVED_SHA": value} for value in ("", "a" * 7, "A" * 40, "g" * 40, OTHER)]
        cases += [{key: value} for key in ("GITHUB_ACTOR", "TRIGGERING_ACTOR")
                  for value in ("", "github-actions", "github-actions[bot]", "other[bot]")]
        cases += [{"GITHUB_SHA": OTHER}, {"CHECKOUT_SHA": OTHER}, {"MAIN_SHA": OTHER},
                  {"MAIN_SHA": ""}, {"REMOTE_FAILURE": "1"}]
        for changes in cases:
            with self.subTest(changes=changes):
                result, calls = self.invoke(changes)
                self.assertNotEqual(result.returncode, 0)
                self.assertIn("Build requires explicit user approval of this exact current main SHA.", result.stderr)
                self.assertTrue(all(call[0] == "git" for call in calls), calls)

    def test_workflow_has_only_manual_read_only_native_builds(self):
        workflow = self.workflow()
        self.assertRegex(workflow, r"(?m)^on:\n  workflow_dispatch:\n    inputs:\n      approved_sha:")
        self.assertRegex(workflow, r"(?m)^        required: true\n        type: string")
        self.assertIn("permissions:\n  contents: read", workflow)
        self.assertIn("ref: ${{ github.sha }}", workflow)
        self.assertIn("persist-credentials: false", workflow)
        self.assertIn("runner: ubuntu-24.04\n", workflow)
        self.assertIn("runner: ubuntu-24.04-arm\n", workflow)
        self.assertEqual(self.commands()[0], "sh scripts/approve-build.sh")
        self.assertEqual(len(self.commands()), 3)
        for setting in ("APPROVED_SHA: ${{ inputs.approved_sha }}",
                        "DEFAULT_BRANCH: ${{ github.event.repository.default_branch }}",
                        "SENDER_TYPE: ${{ github.event.sender.type }}",
                        "TRIGGERING_ACTOR: ${{ github.triggering_actor }}"):
            self.assertIn(setting, workflow)
        for action in re.findall(r"uses: ([^\n]+)", workflow):
            self.assertRegex(action, r"@(?:[0-9a-f]{40})(?: |$)")
        for forbidden in ("workflow_run:", "workflow_call:", "schedule:", "push:",
                          "pull_request:", "secrets.", "contents: write", "login", "upload-artifact",
                          "tools-publish", "bundle-publish", "setup-go", "gh ", "curl "):
            self.assertNotIn(forbidden, workflow)


if __name__ == "__main__":
    unittest.main()
