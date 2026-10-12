import pathlib
import unittest

ROOT = pathlib.Path(__file__).resolve().parents[2]


class WorkflowTests(unittest.TestCase):
    def test_shared_tag_job_requires_verified_promotion(self):
        workflow = (ROOT/'.github/workflows/bundle-publish.yml').read_text()
        self.assertIn('\n  tag:\n',workflow)
        before, tagging = workflow.split('\n  tag:\n')
        self.assertNotIn('contents: write',before)
        self.assertIn('contents: write',tagging)
        self.assertIn('needs: [preflight, resolve, promote]',tagging)
        self.assertIn("needs.promote.result == 'success'",tagging)
        self.assertIn("needs.resolve.outputs.publish == 'true'",tagging)
        self.assertIn('ref: ${{ needs.preflight.outputs.sha }}',tagging)
        self.assertIn('PUBLICATION_SHA: ${{ needs.preflight.outputs.sha }}',tagging)
        self.assertIn('name: ${{ inputs.bundle }}-publication-${{ github.run_attempt }}',tagging)
        self.assertIn('--publication publication.json --bundle "$BUNDLE" --write',tagging)
        self.assertNotIn('if: always()',tagging)
        self.assertNotIn('go build',tagging)
        for job in ('preflight','resolve','native','promote'):
            start=workflow.index('\n  '+job+':\n')
            # Exact two-space job headers, excluding their indented fields.
            import re
            match=re.search(r'\n  [a-z]+:\n',workflow[start+1:])
            body=workflow[start:] if match is None else workflow[start:start+1+match.start()]
            with self.subTest(job=job):
                self.assertIn('contents: read',body)
                self.assertNotIn('contents: write',body)

    def test_thin_callers_allow_tag_permission_without_build_tokens(self):
        for bundle in ('toolbox','code-server','node','node-current','bun','codex','agy','claude','opentofu'):
            workflow=(ROOT/'.github/workflows'/('bundledock-'+bundle+'.yml')).read_text()
            default, job=workflow.split('\njobs:\n')
            with self.subTest(bundle=bundle):
                self.assertIn('contents: read',default)
                self.assertNotIn('contents: write',default)
                self.assertIn('contents: write',job)
                self.assertIn('uses: ./.github/workflows/bundle-publish.yml',job)
                self.assertNotIn('scripts/bundle-git-tags.py',job)
