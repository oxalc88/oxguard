import copy
import unittest
from compare_selection import select, metrics


def row(name, coverage, duration=1, defects=(), protected=False):
    return {"id": name, "samples_ms": [duration], "coverage": [coverage], "outcomes": ["pass"],
            "defects": list(defects), "protected": protected}


class SelectionSafety(unittest.TestCase):
    def test_identical_coverage_does_not_hide_distinct_defect_detection(self):
        rows = [row("fast", ["src:x"], defects=["security"]),
                row("slow", ["src:x"], 10, ["contract"]), row("duplicate", ["src:x"], 20)]
        for method in ["greedy", "kmeans", "fuzzy"]:
            chosen = select(rows, method, k=1)
            self.assertTrue({0, 1} <= chosen)
            self.assertEqual(metrics(rows, chosen)["known_defects_missed"], [])

    def test_protected_unmeasured_and_variable_tests_are_retained(self):
        rows = [row("fast", ["a"]), row("protected", ["a"], 10, protected=True),
                row("unmeasured", [], 20), row("variable", ["a"], 30)]
        rows[-1].update(samples_ms=[30, 31], coverage=[["a"], ["b"]], outcomes=["pass", "pass"])
        for method in ["greedy", "kmeans", "fuzzy"]:
            self.assertTrue({1, 2, 3} <= select(rows, method))

    def test_missing_failed_and_duplicate_inputs_are_rejected(self):
        for rows in [[], [row("a", ["x"]), row("a", ["x"])]]:
            with self.assertRaises(ValueError):
                select(rows, "greedy")
        for change in [{"samples_ms": [0]}, {"outcomes": ["fail"]}, {"coverage": []}]:
            r = row("a", ["x"])
            r.update(change)
            with self.assertRaises(ValueError):
                select([r], "greedy")

    def test_unknown_impact_falls_back_to_full_suite(self):
        rows = [row("a", ["src/a.cjs:v8:0:1"]), row("b", ["src/b.cjs:v8:0:1"])]
        self.assertEqual(select(rows, "impact", changed=["package-lock.json"]), {0, 1})
        self.assertEqual(select(rows, "impact", changed=["src/a.cjs"]), {0})

    def test_seed_is_reproducible_and_input_is_unchanged(self):
        rows = [row(str(i), [str(i % 3), str(i % 2)], i + 1) for i in range(9)]
        before = copy.deepcopy(rows)
        for method in ["greedy", "kmeans", "fuzzy"]:
            self.assertEqual(select(rows, method), select(rows, method))
        self.assertEqual(rows, before)


if __name__ == "__main__":
    unittest.main()
