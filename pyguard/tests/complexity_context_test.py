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
        facts = function_facts(ast.parse('''async def outer(xs):
    for x in xs:
        if x:
            def inner(y):
                if y:
                    while y:
                        y -= 1
            callback = lambda y: y if y else 0
            return inner(callback(x))
'''), "src/a.py", "a")
        self.assertEqual([f["max_branch_nesting"] for f in facts], [2, 2, 1])
        self.assertEqual([f["location"]["symbol"] for f in facts],
                         ["a.outer", "a.outer.inner", "a.outer.<lambda>@8:24"])
        self.assertEqual([[l["line"] for l in f["branch_locations"]] for f in facts],
                         [[2, 3], [5, 6], [8]])

    def test_headers_execute_in_parent_scope(self):
        facts = function_facts(ast.parse('''def outer(x):
    if x:
        def inner(y=1 if x else 0):
            return y
    return inner
'''), "a.py", "a")
        self.assertEqual([f["max_branch_nesting"] for f in facts], [2, 0])
        self.assertEqual(len(facts[0]["branch_locations"]), 2)

    def test_branch_types(self):
        facts = function_facts(ast.parse('''def work(x):
    try:
        for y in x:
            while y:
                if y:
                    y -= 1
    except Exception:
        match x:
            case 1:
                return 1 if x else 0
'''), "a.py", "a")
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
