import pathlib
import subprocess
import sys
import tempfile
import unittest

SCRIPT = pathlib.Path(__file__).resolve().parents[1] / 'select-receipt.py'


def select(present, current='2', bundle='nixbox'):
    with tempfile.TemporaryDirectory() as directory:
        for attempt in present:
            folder = pathlib.Path(directory) / (bundle + '-publication-' + attempt)
            folder.mkdir()
            (folder / 'publication.json').write_text('{}')
        # A foreign bundle's receipt must never match.
        (pathlib.Path(directory) / 'node-current-publication-9').mkdir()
        result = subprocess.run([sys.executable, '-I', '-B', str(SCRIPT), directory, bundle, current], capture_output=True, text=True)
        return result.returncode, result.stdout.strip()


class SelectReceiptTest(unittest.TestCase):
    def test_current_attempt_receipt_wins(self):
        self.assertEqual(select(['1', '2']), (0, '2'))
        self.assertEqual(select(['2']), (0, '2'))

    def test_failed_jobs_rerun_uses_the_single_older_receipt(self):
        self.assertEqual(select(['1']), (0, '1'))

    def test_no_receipt_fails(self):
        self.assertEqual(select([])[0], 1)

    def test_ambiguous_older_receipts_fail(self):
        self.assertEqual(select(['1', '2'], current='3')[0], 1)

    def test_other_bundle_prefix_does_not_match(self):
        self.assertEqual(select(['1'], bundle='node')[1], '1')

    def test_receipt_file_is_required(self):
        with tempfile.TemporaryDirectory() as directory:
            (pathlib.Path(directory) / 'nixbox-publication-1').mkdir()
            result = subprocess.run([sys.executable, '-I', '-B', str(SCRIPT), directory, 'nixbox', '2'], capture_output=True, text=True)
            self.assertEqual(result.returncode, 1)


if __name__ == '__main__':
    unittest.main()
