"""Regression checks for change input completeness and Radon accounting."""
import json
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "analysis"))
from analyze_change import file_metrics

ANALYZER = Path(__file__).resolve().parents[1] / "analysis" / "analyze_change.py"


class ChangeStubTests(unittest.TestCase):
    def analyze(self, baseline=None, candidate=None, *, dirs=None, exclude=None,
                exclude_tests=True):
        with tempfile.TemporaryDirectory() as directory:
            roots = {}
            for side, files in (("baseline", baseline), ("candidate", candidate)):
                root = Path(directory) / side
                root.mkdir()
                for name, source in {"a.py": "VALUE = 1\n", **(files or {})}.items():
                    path = root / name
                    path.parent.mkdir(parents=True, exist_ok=True)
                    path.write_bytes(source if isinstance(source, bytes) else source.encode())
                roots[side] = str(root)
            request = {**roots, "dirs": dirs or ["."], "exclude": exclude or [],
                       "exclude_tests": exclude_tests}
            completed = subprocess.run(
                [sys.executable, str(ANALYZER), json.dumps(request)],
                capture_output=True, text=True, check=True,
            )
            return json.loads(completed.stdout)

    def assert_complete(self, output):
        for side in ("baseline", "candidate"):
            self.assertNotIn("input_error", output[side], output)
            self.assertNotIn("metrics_error", output[side], output)
            self.assertFalse(output[side]["structure"]["partial"], output)

    def test_malformed_stub_fails_each_side_in_real_main(self):
        for side in ("baseline", "candidate"):
            with self.subTest(side=side):
                output = self.analyze(**{side: {"broken.pyi": "def broken(\n"}})
                self.assertIn("broken.pyi", output[side]["input_error"])
                self.assertNotIn("structure", output[side])
                other = "candidate" if side == "baseline" else "baseline"
                self.assertNotIn("input_error", output[other])

    def test_valid_stubs_are_parsed_without_graph_nodes_or_metrics(self):
        files = {"a.pyi": "def declared(value: int) -> str: ...\n",
                 "latin.pyi": b"# coding: latin-1\nLABEL: str = 'caf\xe9'\n",
                 "bom.pyi": b"\xef\xbb\xbfVALUE: int\n"}
        output = self.analyze(files, files)
        self.assert_complete(output)
        for profile in output.values():
            self.assertEqual([item["file"] for item in profile["files"]], ["a.py"])
            self.assertEqual([item["file"] for item in profile["structure"]["modules"]], ["a.py"])

    def test_invalid_stub_encoding_is_input_error(self):
        output = self.analyze(candidate={"bad.pyi": b"VALUE: str = '\xff'\n"})
        self.assertIn("bad.pyi", output["candidate"]["input_error"])

    def test_requested_scope_ignores_other_stubs(self):
        files = {"src/scoped.py": "VALUE = 1\n", "elsewhere/bad.pyi": "def broken(\n"}
        self.assert_complete(self.analyze(files, files, dirs=["src"]))

    def test_explicit_stub_file_scope_is_validated(self):
        output = self.analyze(candidate={"bad.pyi": "def broken(\n"}, dirs=["bad.pyi"])
        self.assertIn("bad.pyi", output["candidate"]["input_error"])

    def test_configured_exclusions_apply_to_both_sides(self):
        files = {"excluded/bad.pyi": "def broken(\n", "ignored.pyi": "def broken(\n"}
        self.assert_complete(self.analyze(files, files, exclude=["excluded", "ignored.pyi"]))

    def test_py_only_config_exclusion_does_not_hide_stubs(self):
        output = self.analyze(candidate={"bad.pyi": "def broken(\n"}, exclude=["*.py"])
        self.assertIn("bad.pyi", output["candidate"]["input_error"])

    def test_default_test_exclusions_cover_stub_names_and_directories(self):
        for name in ("test_example.pyi", "example_test.pyi", "conftest.pyi",
                     "tests/example.pyi", "test/example.pyi"):
            with self.subTest(name=name):
                files = {name: "def broken(\n"}
                self.assert_complete(self.analyze(files, files))
                output = self.analyze(candidate=files, exclude_tests=False)
                self.assertIn(name, output["candidate"]["input_error"])

    def test_discovery_prunes_same_directories_as_python_sources(self):
        files = {f"{name}/bad.pyi": "def broken(\n" for name in (
            ".hidden", ".venv", "venv", "__pycache__", "node_modules", "build", "dist",
        )}
        self.assert_complete(self.analyze(files, files))


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
