"""Manual-only workflow scope guards."""
from pathlib import Path
import unittest

ROOT = Path(__file__).resolve().parents[2]


class PublishedConsumersWorkflowTests(unittest.TestCase):
    def test_manual_workflow_cannot_publish_or_run_automatically(self):
        workflow = (ROOT/'.github/workflows/published-consumers.yml').read_text()
        self.assertIn('workflow_dispatch:', workflow)
        self.assertIn('approved_sha:', workflow)
        self.assertIn('ubuntu-24.04-arm', workflow)
        self.assertIn('git ls-remote origin refs/heads/main', workflow)
        for forbidden in ('workflow_run:', 'pull_request:', 'push:', 'schedule:', 'contents: write', 'docker build', 'setup-go', 'upload-artifact'):
            self.assertNotIn(forbidden, workflow)
