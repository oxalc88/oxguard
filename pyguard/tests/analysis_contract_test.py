"""Independent source-location checks for owned JSON finding identities."""

import json
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

ANALYSIS = Path(__file__).resolve().parents[1] / "analysis"
sys.path.insert(0, str(ANALYSIS))
import check_halstead  # noqa: E402


class ContractLocations(unittest.TestCase):
    def test_two_annotations_on_one_line_have_distinct_columns(self):
        with tempfile.TemporaryDirectory() as root:
            source = Path(root) / "source.py"
            source.write_text(
                "def f(a: list[list[list[int]]], b: list[list[list[int]]]) -> None:\n    pass\n"
            )
            run = subprocess.run(
                [
                    sys.executable,
                    str(ANALYSIS / "check_type_complexity.py"),
                    "--json",
                    str(source),
                ],
                capture_output=True,
                text=True,
            )
            self.assertEqual(run.returncode, 1)
            findings = json.loads(run.stdout)["findings"]
            self.assertEqual(len(findings), 2)
            self.assertEqual([f["location"]["line"] for f in findings], [1, 1])
            self.assertEqual([f["location"]["column"] for f in findings], [10, 36])

    def test_same_named_methods_keep_their_own_source_lines(self):
        with tempfile.TemporaryDirectory() as root:
            source = Path(root) / "source.py"
            source.write_text(
                "class First:\n    def work(self, value):\n        return value + 1\nclass Second:\n    def work(self, value):\n        return value + 2\n"
            )
            old_threshold = check_halstead.MAX_EFFORT
            try:
                check_halstead.MAX_EFFORT = 0
                violations = check_halstead.check_file(source)
            finally:
                check_halstead.MAX_EFFORT = old_threshold
            self.assertEqual(
                [(v.function, v.line) for v in violations], [("work", 2), ("work", 5)]
            )


if __name__ == "__main__":
    unittest.main()
