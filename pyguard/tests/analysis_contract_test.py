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

    def test_latin1_source_that_is_also_valid_utf8_keeps_both_functions(self):
        with tempfile.TemporaryDirectory() as root:
            source = Path(root) / "latin1.py"
            # These bytes decode as UTF-8, but the declared Latin-1 encoding
            # makes Python see the identifier Âª (normalized to Âa), which is
            # distinct from a. pyan3's forced UTF-8 read would see ª instead,
            # normalize it to a and collapse the first function into a.
            source.write_bytes(
                b"# coding: latin-1\n\n\n"
                b"def \xc2\xaa():\n"
                b"    return 1\n\n\n"
                b"def a():\n"
                b"    return 2\n\n\n"
                b"def caller():\n"
                b"    return \xc2\xaa() + a()\n"
            )
            self.assertTrue(analyze_structure.needs_utf8_mirror(source.read_bytes()))
            graph, skipped = analyze_structure.build_function_graph(
                [str(source.resolve())], Path(root).resolve()
            )
            self.assertEqual(skipped, [])
            self.assertEqual(
                [function["symbol"] for function in graph["functions"]],
                ["latin1.\u00c2a", "latin1.a", "latin1.caller"],
            )
            self.assertEqual(
                graph["calls"],
                [{"caller": 2, "callee": 0}, {"caller": 2, "callee": 1}],
            )
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

    def test_local_aliases_and_receiver_reads_each_count_one_call_site(self):
        with tempfile.TemporaryDirectory() as root:
            source = Path(root) / "aliases.py"
            source.write_text(
                "def invoke(callback):\n"
                "    saved = callback\n"
                "    return saved()\n\n\n"
                "def indexed(items):\n"
                "    callback = items[0]\n"
                "    return callback()\n\n\n"
                "def receiver(obj):\n"
                "    saved = obj\n"
                "    return obj.run()\n"
            )
            graph, skipped = analyze_structure.build_function_graph(
                [str(source.resolve())], Path(root).resolve()
            )
            self.assertEqual(skipped, [])
            # There is no alias or dataflow tracking: each alias/receiver call
            # is one syntactic call site that does not name an in-scope callee,
            # builtin or import, and a receiver is never a second call site.
            self.assertEqual(graph["function_counts"]["unresolved_calls"], 3)
            self.assertEqual(
                [function["symbol"] for function in graph["functions"]],
                ["aliases.invoke", "aliases.indexed", "aliases.receiver"],
            )
            self.assertEqual(graph["calls"], [])

    def test_nested_definition_bodies_belong_to_their_own_caller(self):
        with tempfile.TemporaryDirectory() as root:
            source = Path(root) / "closure.py"
            source.write_text(
                "def outer(callback):\n"
                "    saved = callback\n"
                "    def inner():\n"
                "        return callback()\n"
                "    return inner()\n"
            )
            graph, skipped = analyze_structure.build_function_graph(
                [str(source.resolve())], Path(root).resolve()
            )
            self.assertEqual(skipped, [])
            # Only inner calls the parameter. outer's own scope contains the
            # resolved inner() call, not inner's body or the saved alias.
            self.assertEqual(graph["function_counts"]["unresolved_calls"], 1)
            self.assertEqual(graph["calls"], [{"caller": 0, "callee": 1}])

    def test_builtin_import_and_resolved_in_scope_calls_are_not_unresolved(self):
        with tempfile.TemporaryDirectory() as root:
            source = Path(root) / "boundaries.py"
            source.write_text(
                "import os\n\n\n"
                "def builtin_calls(value):\n"
                "    print(value)\n"
                "    return len(value)\n\n\n"
                "def external_call():\n"
                "    return os.getcwd()\n\n\n"
                "def resolved_call():\n"
                "    return target()\n\n\n"
                "def target():\n"
                "    return 1\n"
            )
            graph, skipped = analyze_structure.build_function_graph(
                [str(source.resolve())], Path(root).resolve()
            )
            self.assertEqual(skipped, [])
            self.assertEqual(graph["function_counts"]["unresolved_calls"], 0)
            self.assertEqual(graph["calls"], [{"caller": 2, "callee": 3}])

    def test_nested_definition_headers_belong_to_the_enclosing_scope(self):
        with tempfile.TemporaryDirectory() as root:
            source = Path(root) / "headers.py"
            source.write_text(
                "def deco(fn):\n"
                "    return fn\n\n\n"
                "def outer(callback):\n"
                "    @deco()\n"
                "    def inner(value=callback()):\n"
                "        return callback()\n"
                "    return inner()\n"
            )
            graph, skipped = analyze_structure.build_function_graph(
                [str(source.resolve())], Path(root).resolve()
            )
            self.assertEqual(skipped, [])
            # deco() resolves to the defined function; the nested default and
            # inner's own body are the two unresolved parameter call sites.
            self.assertEqual(graph["function_counts"]["unresolved_calls"], 2)


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



class StructureDelegationContract(unittest.TestCase):
    """Forwarding facts stay exact for the Python-specific delegation rules."""

    def graph(self, root, files):
        package = Path(root) / "pkg"
        package.mkdir()
        (package / "__init__.py").write_text("")
        for name, text in files.items():
            (package / name).write_text(text)
        graph, skipped = analyze_structure.build_function_graph(
            sorted(str(path.resolve()) for path in package.glob("*.py")),
            Path(root).resolve(),
        )
        self.assertEqual(skipped, [])
        return {function["symbol"]: function for function in graph["functions"]}

    def test_module_level_chain_records_modes_and_targets(self):
        files = {
            "real.py": "def real(x: int) -> int:\n    return x * 2\n",
            "c.py": "from .real import real\n\n\ndef c(x: int) -> int:\n    return real(x)\n",
            "b.py": "from .c import c\n\n\ndef b(x: int) -> int:\n    return c(x)\n",
            "a.py": "from .b import b\n\n\ndef a(x: int) -> int:\n    return b(x)\n",
        }
        with tempfile.TemporaryDirectory() as root:
            functions = self.graph(root, files)
        self.assertEqual(functions["pkg.a.a"]["forward_target"], "pkg.b.b")
        self.assertEqual(functions["pkg.a.a"]["forwarding_mode"], "sync")
        self.assertEqual(functions["pkg.c.c"]["forward_target"], "pkg.real.real")
        self.assertEqual(functions["pkg.real.real"]["forward_target"], "")
        self.assertTrue(functions["pkg.a.a"]["module_level"])

    def test_async_forwarding_requires_await_and_an_async_target(self):
        files = {
            "chain.py": (
                "async def work(x: int) -> int:\n"
                "    return x\n\n\n"
                "async def c(x: int) -> int:\n"
                "    return await work(x)\n\n\n"
                "async def boundary(x: int) -> int:\n"
                "    try:\n"
                "        return await work(x)\n"
                "    except Exception as error:\n"
                "        raise RuntimeError('boundary') from error\n"
            )
        }
        with tempfile.TemporaryDirectory() as root:
            functions = self.graph(root, files)
        self.assertEqual(functions["pkg.chain.c"]["forwarding_mode"], "async-return-await")
        self.assertEqual(functions["pkg.chain.c"]["forward_target"], "pkg.chain.work")
        self.assertEqual(functions["pkg.chain.boundary"]["forward_target"], "")
        self.assertEqual(functions["pkg.chain.boundary"]["forwarding_mode"], "")

    def test_annotation_factory_edges_do_not_change_returned_call_resolution(self):
        files = {
            "cases.py": (
                "def schema():\n"
                "    return int\n\n\n"
                "def recursive(x: schema()):\n"
                "    return recursive(x)\n\n\n"
                "def real(x):\n"
                "    return x\n\n\n"
                "def wrapper(x: schema()):\n"
                "    return real(x)\n"
            )
        }
        with tempfile.TemporaryDirectory() as root:
            functions = self.graph(root, files)
        self.assertEqual(functions["pkg.cases.recursive"]["forward_target"], "")
        self.assertEqual(functions["pkg.cases.recursive"]["forwarding_mode"], "")
        self.assertEqual(functions["pkg.cases.wrapper"]["forward_target"], "pkg.cases.real")
        self.assertEqual(functions["pkg.cases.wrapper"]["forwarding_mode"], "sync")

    def test_aliased_import_and_same_named_annotation_use_return_binding(self):
        files = {
            "worker.py": "def real(x):\n    return x\n",
            "schema.py": "def target():\n    return int\n",
            "wrapper.py": (
                "from .worker import real as target\n"
                "from . import schema\n\n\n"
                "def wrapper(x: schema.target()):\n"
                "    return target(x)\n"
            ),
        }
        with tempfile.TemporaryDirectory() as root:
            functions = self.graph(root, files)
        self.assertEqual(functions["pkg.wrapper.wrapper"]["forward_target"], "pkg.worker.real")
        self.assertEqual(functions["pkg.wrapper.wrapper"]["forwarding_mode"], "sync")

    def test_parameter_shadowing_does_not_resolve_to_module_function(self):
        files = {
            "cases.py": (
                "def target(x):\n"
                "    return x\n\n\n"
                "def wrapper(target):\n"
                "    return target(target)\n"
            )
        }
        with tempfile.TemporaryDirectory() as root:
            functions = self.graph(root, files)
        self.assertEqual(functions["pkg.cases.wrapper"]["forward_target"], "")
        self.assertEqual(functions["pkg.cases.wrapper"]["forwarding_mode"], "")

    def test_non_forwarding_shapes_are_excluded(self):
        files = {
            "controls.py": (
                "def real(x: int, y: int = 0) -> int:\n"
                "    return x + y\n\n\n"
                "def defaults(x: int) -> int:\n"
                "    return real(x, 0)\n\n\n"
                "def varargs(*args: int) -> int:\n"
                "    return real(*args)\n\n\n"
                "def kwonly(x: int, *, y: int) -> int:\n"
                "    return real(x, y)\n\n\n"
                "def reordered(x: int, y: int) -> int:\n"
                "    return real(y, x)\n\n\n"
                "def mismatched(x: int) -> str:\n"
                "    return real(x, 0)\n\n\n"
                "def marker(function):\n"
                "    return function\n\n\n"
                "@marker\n"
                "def decorated(x: int) -> int:\n"
                "    return real(x, 0)\n\n\n"
                "class Service:\n"
                "    def load(self, x: int) -> int:\n"
                "        return real(x, 0)\n"
            )
        }
        with tempfile.TemporaryDirectory() as root:
            functions = self.graph(root, files)
        for symbol in (
            "pkg.controls.real",
            "pkg.controls.defaults",
            "pkg.controls.varargs",
            "pkg.controls.kwonly",
            "pkg.controls.reordered",
            "pkg.controls.mismatched",
            "pkg.controls.decorated",
            "pkg.controls.Service.load",
        ):
            self.assertEqual(functions[symbol]["forward_target"], "")
            self.assertEqual(functions[symbol]["forwarding_mode"], "")
        self.assertFalse(functions["pkg.controls.Service.load"]["module_level"])

    def test_forwarding_cycle_keeps_facts_without_a_finite_depth(self):
        files = {
            "a.py": "from .b import b\n\n\ndef a(x: int) -> int:\n    return b(x)\n",
            "b.py": "from .c import c\n\n\ndef b(x: int) -> int:\n    return c(x)\n",
            "c.py": "from .a import a\n\n\ndef c(x: int) -> int:\n    return a(x)\n",
        }
        with tempfile.TemporaryDirectory() as root:
            functions = self.graph(root, files)
        for symbol in ("pkg.a.a", "pkg.b.b", "pkg.c.c"):
            self.assertEqual(functions[symbol]["forwarding_mode"], "sync")
            self.assertNotEqual(functions[symbol]["forward_target"], "")

    def test_failure_handler_policy_and_fingerprint_facts(self):
        source_text = (
            "def run():\n"
            "    try:\n        work()\n    except Exception:\n        return ()\n"
            "    try:\n        work()\n    except KeyError:\n        return None\n"
            "    try:\n        work()\n    except Exception as error:\n        return error\n"
            "    try:\n        work()\n    except Exception as error:\n        return other\n"
            "    try:\n        work()\n    except:\n        pass\n"
            "    try:\n        work()\n    except Exception:\n        return set()\n"
        )
        with tempfile.TemporaryDirectory() as root:
            source = Path(root) / "handlers.py"
            source.write_text(source_text)
            facts = analyze_structure.failure_handlers(source, analyze_structure.read_tree(source))
        self.assertEqual([fact["line"] for fact in facts], [4, 8, 12, 16, 20, 24])
        self.assertEqual([fact["fallback"] for fact in facts], [True, False, False, True, False, False])
        self.assertTrue(all(fact["exit"] is False for fact in facts))
        self.assertEqual(facts[0]["token_count"], 3)
        self.assertEqual(len(facts[0]["fingerprint"]), 64)

    def test_failure_handler_fingerprint_ignores_comments_and_newlines(self):
        bodies = ["    record(error)\n    return None\n", "    record( error )  # same body\n    return None\n"]
        fingerprints = []
        for body in bodies:
            with tempfile.TemporaryDirectory() as root:
                source = Path(root) / "handlers.py"
                source.write_text("def run():\n    try:\n        work()\n    except Exception as error:\n" + "".join("    " + line for line in body.splitlines(keepends=True)))
                facts = analyze_structure.failure_handlers(source, analyze_structure.read_tree(source))
                fingerprints.append((facts[0]["fingerprint"], facts[0]["token_count"]))
        self.assertEqual(fingerprints[0], fingerprints[1])

    def test_failure_handler_body_fingerprint_excludes_inline_except_header(self):
        sources = [
            "def run():\n    try:\n        work()\n    except Exception as error: return fallback\n",
            "def run():\n    try:\n        work()\n    except KeyError as error: return fallback\n",
        ]
        facts = []
        for source_text in sources:
            with tempfile.TemporaryDirectory() as root:
                source = Path(root) / "handlers.py"
                source.write_text(source_text)
                facts.append(analyze_structure.failure_handlers(source, analyze_structure.read_tree(source))[0])
        self.assertEqual(facts[0]["fingerprint"], facts[1]["fingerprint"])
        self.assertEqual(facts[0]["token_count"], 2)

    def test_failure_handler_fingerprint_skips_lambda_colon_in_header(self):
        body = """        record(error)
        retry(error)
        report(error)
        recover(error)
        annotate(error)
        retain(error)
        update(error)
        notify(error)
        measure(error)
        finish(error)
        return None
"""
        headers = (
            "except ValueError if enabled else lambda: Exception:",
            "except ValueError:",
            "except Exception:",
        )
        facts = []
        for header in headers:
            source_text = "def run():\n    try:\n        work()\n    " + header + "\n" + body
            with tempfile.TemporaryDirectory() as root:
                source = Path(root) / "handlers.py"
                source.write_text(source_text)
                facts.extend(analyze_structure.failure_handlers(source, analyze_structure.read_tree(source)))
        self.assertEqual([fact["token_count"] for fact in facts], [42, 42, 42])
        self.assertEqual(len({fact["fingerprint"] for fact in facts}), 1)

    def test_failure_handler_fingerprint_includes_nested_decorators(self):
        source_text = """def run():
    try:
        work()
    except Exception:
        @alpha
        def nested():
            return 1
    try:
        work()
    except Exception:
        @beta
        def nested():
            return 1
    try:
        work()
    except Exception:
        @gamma
        def nested():
            return 1
"""
        with tempfile.TemporaryDirectory() as root:
            source = Path(root) / "handlers.py"
            source.write_text(source_text)
            facts = analyze_structure.failure_handlers(source, analyze_structure.read_tree(source))
        self.assertEqual(len(facts), 3)
        self.assertEqual(len({fact["fingerprint"] for fact in facts}), 3)

    def test_failure_handler_fingerprint_uses_physical_lines(self):
        body = """        record(error)
        retry(error)
        report(error)
        recover(error)
        annotate(error)
        retain(error)
        update(error)
        notify(error)
        measure(error)
        finish(error)
        return None
"""
        prefix = 'label = "before\u2028after"\n'
        handler = "    try:\n        work()\n    except Exception as error:\n" + body
        source_text = prefix + "def run():\n" + handler * 3
        for newline in ("\n", "\r\n"):
            with self.subTest(newline=repr(newline)), tempfile.TemporaryDirectory() as root:
                source = Path(root) / "handlers.py"
                source.write_bytes(source_text.replace("\n", newline).encode("utf-8"))
                facts = analyze_structure.failure_handlers(source, analyze_structure.read_tree(source))
                self.assertEqual(len(facts), 3)
                self.assertEqual([fact["token_count"] for fact in facts], [42, 42, 42])
                self.assertEqual(len({fact["fingerprint"] for fact in facts}), 1)

    def test_failure_handler_broad_attribute_is_only_builtin_qualified(self):
        source_text = (
            "def run():\n"
            "    try:\n        work()\n    except custom.Exception:\n        return None\n"
            "    try:\n        work()\n    except builtins.Exception:\n        return None\n"
            "    try:\n        work()\n    except (KeyError, Exception):\n        return None\n"
        )
        with tempfile.TemporaryDirectory() as root:
            source = Path(root) / "handlers.py"
            source.write_text(source_text)
            facts = analyze_structure.failure_handlers(source, analyze_structure.read_tree(source))
        self.assertEqual([fact["fallback"] for fact in facts], [False, True, True])


if __name__ == "__main__":
    unittest.main()
