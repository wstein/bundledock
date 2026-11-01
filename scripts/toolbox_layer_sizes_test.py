import importlib.util
import json
from pathlib import Path
import unittest

SPEC = importlib.util.spec_from_file_location("sizes", Path(__file__).with_name("toolbox-layer-sizes.py"))


class LayerSizesTest(unittest.TestCase):
    def setUp(self):
        self.module = importlib.util.module_from_spec(SPEC)
        SPEC.loader.exec_module(self.module)

    def test_group_totals_follow_plan_order(self):
        plan = {"layers": [{"name": "low", "tools": ["a", "b"]}, {"name": "high", "tools": ["c"]}]}
        self.assertEqual(self.module.group_totals({"a": 5, "b": 7, "c": 3}, plan), [("low", 2, 12), ("high", 1, 3)])

    def test_group_totals_reject_a_tool_missing_from_either_side(self):
        plan = {"layers": [{"name": "low", "tools": ["a"]}]}
        with self.assertRaises(ValueError):
            self.module.group_totals({"a": 1, "b": 2}, plan)
        with self.assertRaises(ValueError):
            self.module.group_totals({}, plan)

    def test_committed_plan_matches_the_lock(self):
        root = Path(__file__).resolve().parent.parent / "bundles" / "toolbox"
        plan = json.loads((root / "layers.json").read_text())
        lock = json.loads((root / "packages.lock.json").read_text())
        for arch, pins in lock["platforms"].items():
            totals = self.module.group_totals({t: 1 for t in pins}, plan)
            self.assertEqual(sum(n for _, n, _ in totals), len(pins), arch)


if __name__ == "__main__":
    unittest.main()
