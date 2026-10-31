"""Known AST nesting facts, independent of radon's native complexity score."""
import ast
import json
import os
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "analysis"))
from complexity_context import function_facts


class ComplexityContextTests(unittest.TestCase):
    def test_boundaries_and_locations(self):
        source = '''async def outer(xs):
    for x in xs:
        if x:
            def inner(y):
                if y:
                    while y:
                        y -= 1
            callback = lambda y: y if y else 0
            return inner(callback(x))
'''
        facts = function_facts(ast.parse(source), "src/a.py", "a", source)
        self.assertEqual([f["max_branch_nesting"] for f in facts], [2, 2, 1])
        self.assertEqual([f["location"]["symbol"] for f in facts],
                         ["a.outer", "a.outer.inner", "a.outer.<lambda>@8:24"])
        self.assertEqual([[l["line"] for l in f["branch_locations"]] for f in facts],
                         [[2, 3], [5, 6], [8]])

    def test_headers_execute_in_parent_scope(self):
        source = '''def outer(x):
    if x:
        def inner(y=1 if x else 0):
            return y
    return inner
'''
        facts = function_facts(ast.parse(source), "a.py", "a", source)
        self.assertEqual([f["max_branch_nesting"] for f in facts], [2, 0])
        self.assertEqual(len(facts[0]["branch_locations"]), 2)

    def test_class_headers_execute_in_parent_scope(self):
        source = """def outer(x, Base, Other, meta, decorate):
    if x:
        @decorate if x else decorate
        class C(Base if x else Other, metaclass=meta if x else meta):
            def method(self):
                if x:
                    return x
        class D((lambda x: x if x else 0)(Base)):
            pass
"""
        facts = function_facts(ast.parse(source), "a.py", "a", source)
        self.assertEqual([f["max_branch_nesting"] for f in facts], [2, 1, 1])
        self.assertEqual([f["location"]["symbol"] for f in facts],
                         ["a.outer", "a.outer.C.method", "a.outer.<lambda>@8:18"])
        self.assertEqual(sorted(l["line"] for l in facts[0]["branch_locations"]), [2, 3, 4, 4])
        self.assertEqual(facts[2]["branch_locations"][0]["line"], 8)

    def test_unicode_columns_and_physical_lines(self):
        source = 'def outer():\n    é = lambda y: y if y else 0; text = "\u2028"; ø = lambda: 1\n'
        facts = function_facts(ast.parse(source), "a.py", "a", source)
        self.assertEqual(facts[1]["location"],
                         {"file": "a.py", "line": 2, "column": 9,
                          "symbol": "a.outer.<lambda>@2:9"})
        self.assertEqual(facts[1]["branch_locations"],
                         [{"file": "a.py", "line": 2, "column": 19}])
        self.assertEqual(facts[2]["location"]["column"], source.split("\n")[1].rindex("lambda") + 1)

    def test_narrowed_nested_scope_preserves_resolved_calls(self):
        helper = Path(__file__).resolve().parents[1] / "analysis" / "complexity_context.py"
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / "pkg/nested").mkdir(parents=True)
            (root / "pkg/nested/calls.py").write_text("""def target():
    return 1

def outer():
    def inner():
        return target()
    return inner()

class Worker:
    def run(self):
        return target()
""")
            for target in ["pkg/nested/calls.py", "pkg/nested", "."]:
                with self.subTest(target=target):
                    subprocess.run([sys.executable, str(helper), str(root / "graph.json"), target],
                                   cwd=root, capture_output=True, text=True, check=True)
                    graph = json.loads((root / "graph.json").read_text())
                    self.assertFalse(graph["partial"], graph["graph_error"])
                    names = {f["id"]: f["location"]["symbol"] for f in graph["functions"]}
                    self.assertEqual({(names[c["caller"]], names[c["callee"]]) for c in graph["calls"]},
                                     {("pkg.nested.calls.outer", "pkg.nested.calls.outer.inner"),
                                      ("pkg.nested.calls.outer.inner", "pkg.nested.calls.target"),
                                      ("pkg.nested.calls.Worker.run", "pkg.nested.calls.target")})

    def test_branch_types(self):
        source = '''def work(x):
    try:
        for y in x:
            while y:
                if y:
                    y -= 1
    except Exception:
        match x:
            case 1:
                return 1 if x else 0
'''
        facts = function_facts(ast.parse(source), "a.py", "a", source)
        self.assertEqual(facts[0]["max_branch_nesting"], 3)
        self.assertEqual([l["line"] for l in facts[0]["branch_locations"]], [3, 4, 5, 7, 8, 10])

    def test_encoded_sources_exclusions_and_partial_context(self):
        helper = Path(__file__).resolve().parents[1] / "analysis" / "complexity_context.py"
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / "src/tests").mkdir(parents=True)
            (root / "src/encoded.py").write_bytes(b'# coding: latin-1\ndef cafe():\n    return "caf\xe9"\n')
            (root / "src/broken.py").write_text("def broken(:\n")
            (root / "src/tests/test_hidden.py").write_text("def excluded(): pass\n")
            (root / "src/skip.py").write_text("def skipped(): pass\n")
            output = subprocess.run([sys.executable, str(helper), str(root / "graph.json"), "src", "src/encoded.py"],
                                    cwd=root, env={**os.environ, "PYGUARD_EXCLUDE_TESTS": "1", "PYGUARD_EXCLUDE_GLOBS": "src/skip.py"},
                                    capture_output=True, text=True, check=True)
            envelope = json.loads(output.stdout)
            self.assertEqual(len(envelope["measurements"]), 1)
            self.assertEqual(envelope["measurements"][0]["value"], 0)
            graph = json.loads((root / "graph.json").read_text())
            self.assertTrue(graph["partial"])
            self.assertEqual(graph["skipped"][0]["file"], "src/broken.py")
            self.assertEqual(graph["functions"][0]["location"]["symbol"], "encoded.cafe")


if __name__ == "__main__":
    unittest.main()
