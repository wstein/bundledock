"""Source-only checks of the manual nixbox build and smoke workflow."""
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
TOOLBOX = "quay.io/bundledock/toolbox@sha256:d3a83465d5223e7d7e2e870b48bbd996dba37cbe92d7230bdcf3ef22f46f5f75"


class NixboxBuildWorkflowTests(unittest.TestCase):
    def workflow(self):
        return (ROOT / ".github/workflows/nixbox-build.yml").read_text()

    def commands(self):
        return re.findall(r"^        run: (.+)$", self.workflow(), re.M)

    def approve(self, changes=None):
        """Run only the approval step against inert git adapters."""
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            log = root / "calls"
            adapter = r"""#!/usr/bin/env python3
import json, os, pathlib, sys
name = pathlib.Path(sys.argv[0]).name
args = sys.argv[1:]
with open(os.environ['CALLS'], 'a') as calls:
    calls.write(json.dumps([name] + args) + '\n')
if name == 'git' and args == ['rev-parse', 'HEAD']:
    print(os.environ['CHECKOUT_SHA'])
elif name == 'git' and args == ['ls-remote', 'origin', 'refs/heads/main']:
    print(os.environ['MAIN_SHA'] + '\trefs/heads/main')
else:
    sys.exit('unexpected external operation')
"""
            for name in ("git", "make", "docker", "gh", "curl", "sh_unused"):
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
            result = subprocess.run(["sh", "-ec", self.commands()[0]], cwd=ROOT, env=env,
                                    capture_output=True, text=True)
            calls = [json.loads(line) for line in log.read_text().splitlines()] if log.exists() else []
            return result, calls

    def test_approved_source_passes_the_guard_and_touches_nothing_else(self):
        result, calls = self.approve()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(calls, [["git", "rev-parse", "HEAD"], ["git", "ls-remote", "origin", "refs/heads/main"]])

    def test_rejections_stop_at_the_guard(self):
        cases = [{"GITHUB_EVENT_NAME": event} for event in ("push", "schedule", "workflow_run", "pull_request")]
        cases += [{"DEFAULT_BRANCH": "develop"}, {"GITHUB_REF": "refs/heads/topic"},
                  {"SENDER_TYPE": "Bot"}, {"APPROVED_SHA": ""}, {"APPROVED_SHA": OTHER},
                  {"GITHUB_ACTOR": "github-actions[bot]"}, {"TRIGGERING_ACTOR": "other[bot]"},
                  {"CHECKOUT_SHA": OTHER}, {"MAIN_SHA": OTHER}]
        for changes in cases:
            with self.subTest(changes=changes):
                result, calls = self.approve(changes)
                self.assertNotEqual(result.returncode, 0)
                self.assertIn("Build requires explicit user approval of this exact current main SHA.", result.stderr)
                self.assertTrue(all(call[0] == "git" for call in calls), calls)

    def test_workflow_is_manual_read_only_native_build_and_smoke_only(self):
        workflow = self.workflow()
        self.assertTrue(workflow.startswith("name: Build and smoke nixbox\n"))
        self.assertRegex(workflow, r"(?m)^on:\n  workflow_dispatch:\n    inputs:\n      approved_sha:")
        self.assertIn("permissions:\n  contents: read", workflow)
        self.assertIn("ref: ${{ github.sha }}", workflow)
        self.assertIn("persist-credentials: false", workflow)
        self.assertIn("architecture: amd64\n            runner: ubuntu-24.04\n            platform: linux/amd64", workflow)
        self.assertIn("architecture: arm64\n            runner: ubuntu-24.04-arm\n            platform: linux/arm64", workflow)
        self.assertEqual(self.commands(), [
            "sh scripts/approve-build.sh",
            "make bundle-build BUNDLE=nixbox IMAGE=bundledock-nixbox:build-only",
            "sh bundles/nixbox/scripts/smoke.sh bundledock-nixbox:build-only",
        ])
        for setting in ("APPROVED_SHA: ${{ inputs.approved_sha }}",
                        "DEFAULT_BRANCH: ${{ github.event.repository.default_branch }}",
                        "SENDER_TYPE: ${{ github.event.sender.type }}",
                        "TRIGGERING_ACTOR: ${{ github.triggering_actor }}",
                        "NIXBOX_PLATFORM: ${{ matrix.platform }}",
                        "NIXBOX_TOOLBOX_IMAGE: " + TOOLBOX):
            self.assertIn(setting, workflow)
        for action in re.findall(r"uses: ([^\n]+)", workflow):
            self.assertRegex(action, r"@(?:[0-9a-f]{40})(?: |$)")
        for forbidden in ("workflow_run:", "workflow_call:", "schedule:", "push:", "pull_request:",
                          "secrets.", "vars.", "contents: write", "login", "upload-artifact",
                          "docker push", "bundle-volume", "tools-publish", "bundle-publish",
                          "setup-go", "gh ", "curl ", "id-token"):
            self.assertNotIn(forbidden, workflow)

    def test_toolbox_pin_matches_the_published_record(self):
        record = json.loads((ROOT / "docs/toolbox-published.json").read_text())
        self.assertEqual(TOOLBOX, "quay.io/bundledock/toolbox@" + record["digest"])


if __name__ == "__main__":
    unittest.main()
