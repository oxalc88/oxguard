"""Independent source-location checks for owned JSON finding identities."""

import contextlib
import io
import json
import subprocess
import sys
import tempfile
import types
import unittest
from pathlib import Path

ANALYSIS = Path(__file__).resolve().parents[1] / "analysis"
sys.path.insert(0, str(ANALYSIS))
import analyze_structure
import check_halstead
import vulture_report


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
                check=False,
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


class StructureFunctionGraphContract(unittest.TestCase):
    """pyan3 staging and unresolved-call filters keep graph evidence exact."""

    def test_encoded_mirror_preserves_import_root_for_narrow_scope(self):
        with tempfile.TemporaryDirectory() as root:
            project = Path(root)
            package = project / "src" / "pkg"
            package.mkdir(parents=True)
            (package / "__init__.py").write_text("")
            (package / "a.py").write_bytes(
                b"\xef\xbb\xbf"
                + b"from pkg.b import target\n\n\ndef run():\n    return target()\n"
            )
            (package / "b.py").write_bytes(
                b"\xef\xbb\xbf" + b"def target():\n    return 1\n"
            )
            graph, skipped = analyze_structure.build_function_graph(
                [str((package / "a.py").resolve()), str((package / "b.py").resolve())],
                project.resolve(),
            )
            self.assertEqual(skipped, [])
            self.assertEqual(
                [function["symbol"] for function in graph["functions"]],
                ["pkg.a.run", "pkg.b.target"],
            )
            self.assertEqual(graph["calls"], [{"caller": 0, "callee": 1}])
            self.assertEqual(graph["functions"][0]["depth"], 1)
            self.assertEqual(graph["function_counts"]["unresolved_calls"], 0)

    def test_indirect_and_closure_parameter_calls_are_unresolved(self):
        with tempfile.TemporaryDirectory() as root:
            source = Path(root) / "unresolved.py"
            source.write_text(
                "def first(items):\n"
                "    return items[0]()\n\n\n"
                "def plain(items):\n"
                "    return items[0]\n\n\n"
                "def outer(callback):\n"
                "    def inner():\n"
                "        return callback()\n"
                "    return inner()\n"
            )
            graph, skipped = analyze_structure.build_function_graph(
                [str(source.resolve())], Path(root).resolve()
            )
            self.assertEqual(skipped, [])
            self.assertEqual(graph["function_counts"]["unresolved_calls"], 2)
            self.assertEqual(
                [function["symbol"] for function in graph["functions"]],
                [
                    "unresolved.first",
                    "unresolved.plain",
                    "unresolved.outer",
                    "unresolved.outer.inner",
                ],
            )
            self.assertEqual(graph["calls"], [{"caller": 2, "callee": 3}])


class DeepAnnotationContract(unittest.TestCase):
    """Recursion-limit blowups are reported violations, never skipped passes."""

    def run_types_gate(self, source_text, machine=True):
        with tempfile.TemporaryDirectory() as root:
            source = Path(root) / "source.py"
            source.write_text(source_text)
            argv = [sys.executable, str(ANALYSIS / "check_type_complexity.py")]
            if machine:
                argv.append("--json")
            argv.append(str(source))
            run = subprocess.run(
                argv,
                capture_output=True,
                text=True,
                check=False,
            )
            return run, source

    def test_five_hundred_union_operators_report_one_violation(self):
        run, source = self.run_types_gate("value: " + " | ".join(["int"] * 500) + "\n")
        self.assertNotIn("Traceback", run.stderr)
        # Failing-gate convention: any violation exits 1.
        self.assertEqual(run.returncode, 1)
        findings = json.loads(run.stdout)["findings"]
        self.assertEqual(len(findings), 1)
        self.assertEqual(findings[0]["rule"], "pyguard.types.annotation_complexity")
        self.assertEqual(findings[0]["location"]["file"], source.as_posix())
        self.assertIn("annotation too deeply nested to measure", findings[0]["evidence"])
        self.assertGreater(findings[0]["observed"], findings[0]["threshold"])

    def test_ten_thousand_union_operators_report_the_unparsable_file(self):
        source_text = "value: " + " | ".join(["int"] * 10000) + "\n"
        run, source = self.run_types_gate(source_text)
        self.assertNotIn("Traceback", run.stderr)
        self.assertEqual(run.returncode, 1)
        findings = json.loads(run.stdout)["findings"]
        self.assertEqual(len(findings), 1)
        self.assertEqual(findings[0]["location"]["file"], source.as_posix())
        self.assertNotIn("line", findings[0]["location"])
        self.assertIn("too deeply nested to parse", findings[0]["evidence"])

    def test_ten_thousand_union_operators_fail_the_human_gate_output(self):
        run, source = self.run_types_gate(
            "value: " + " | ".join(["int"] * 10000) + "\n", machine=False
        )
        self.assertNotIn("Traceback", run.stderr)
        self.assertEqual(run.returncode, 1)
        self.assertIn("[FAIL] types", run.stdout)
        self.assertIn(str(source), run.stdout)
        self.assertIn("1 violation(s) found", run.stdout)


class VultureReportContract(unittest.TestCase):
    """Config/API-shape failures stay invalid_configuration; anything else
    escapes as a crash so pyguard records a gate execution error."""

    MISSING = object()

    def setUp(self):
        self._saved_modules = {
            name: sys.modules.get(name, self.MISSING) for name in ("vulture", "vulture.config")
        }
        self._saved_argv = sys.argv

    def tearDown(self):
        sys.argv = self._saved_argv
        for name, module in self._saved_modules.items():
            if module is self.MISSING:
                sys.modules.pop(name, None)
            else:
                sys.modules[name] = module

    def install_vulture(self, make_config):
        vulture = types.ModuleType("vulture")
        vulture.Vulture = object
        config = types.ModuleType("vulture.config")
        config.make_config = make_config
        vulture.config = config
        sys.modules["vulture"] = vulture
        sys.modules["vulture.config"] = config
        sys.argv = ["vulture_report.py"]

    def run_main(self):
        out = io.StringIO()
        with contextlib.redirect_stdout(out):
            code = vulture_report.main()
        return code, json.loads(out.getvalue())

    def test_configuration_failure_reports_invalid_configuration(self):
        def make_config(argv):
            raise KeyError("paths")

        self.install_vulture(make_config)
        code, data = self.run_main()
        self.assertEqual(code, 1)
        self.assertEqual(data["error"]["category"], "invalid_configuration")
        self.assertEqual(data["findings"], [])

    def test_api_shape_failure_reports_invalid_configuration(self):
        def make_config(argv):
            return {
                "verbose": False,
                "ignore_names": [],
                "ignore_decorators": [],
                "exclude": [],
                "paths": [],
                "min_confidence": 60,
                "sort_by_size": False,
            }

        class BrokenVulture:
            def __init__(self, **kwargs):
                raise TypeError("unexpected Vulture() signature")

        self.install_vulture(make_config)
        sys.modules["vulture"].Vulture = BrokenVulture
        code, data = self.run_main()
        self.assertEqual(code, 1)
        self.assertEqual(data["error"]["category"], "invalid_configuration")

    def test_unexpected_failure_propagates_instead_of_passing(self):
        def make_config(argv):
            raise RuntimeError("analyzer exploded")

        self.install_vulture(make_config)
        out = io.StringIO()
        with contextlib.redirect_stdout(out), self.assertRaises(RuntimeError):
            vulture_report.main()
        self.assertEqual(out.getvalue(), "")

    def test_missing_module_reports_tool_missing(self):
        sys.modules["vulture"] = None
        sys.argv = ["vulture_report.py"]
        code, data = self.run_main()
        self.assertEqual(code, 1)
        self.assertEqual(data["error"]["category"], "tool_missing")


if __name__ == "__main__":
    unittest.main()
