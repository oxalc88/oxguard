"""Regression checks for Radon's exact function/method/closure accounting."""
import sys
import tempfile
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "analysis"))
from analyze_change import file_metrics


class ChangeMetricsTests(unittest.TestCase):
    def test_methods_and_closures_are_counted_once(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / "a.py").write_text(
                "class Box:\n"
                "    def method(self, x):\n"
                "        if x:\n"
                "            return 1\n"
                "        return 0\n\n"
                "def outer():\n"
                "    def inner(x):\n"
                "        if x:\n"
                "            return 1\n"
                "        return 0\n"
                "    return inner\n"
            )
            facts = file_metrics({"modules": [{"file": "a.py"}]}, root)
            self.assertEqual(facts[0]["cyclomatic"], 5)
            self.assertEqual(facts[0]["branches"], 2)
            self.assertEqual(facts[0]["max_function_branches"], 1)

    def test_module_decisions_do_not_invent_function_blocks(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / "a.py").write_text("if True:\n    VALUE = 1\n")
            facts = file_metrics({"modules": [{"file": "a.py"}]}, root)
            self.assertEqual(facts[0]["cyclomatic"], 0)
            self.assertEqual(facts[0]["branches"], 0)


if __name__ == "__main__":
    unittest.main()
