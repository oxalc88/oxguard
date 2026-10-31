"""AST branch evidence for radon; never a new complexity gate or score."""
from __future__ import annotations

import ast
import json
import os
import sys
from pathlib import Path

from analyze_structure import (
    build_function_graph, collect_candidates, excluded, exclusion_patterns, module_name,
    nearest_import_root, project_import_roots, read_tree, relative_path,
)

BRANCHES = (ast.If, ast.IfExp, ast.For, ast.AsyncFor, ast.While,
            ast.Match, ast.ExceptHandler)
FUNCTIONS = (ast.FunctionDef, ast.AsyncFunctionDef, ast.Lambda)


def function_facts(tree: ast.AST, file: str, module: str) -> list[dict]:
    facts = []

    def visit(node, scope, owner=None, depth=0):
        if isinstance(node, FUNCTIONS):
            name = (f"<lambda>@{node.lineno}:{node.col_offset + 1}"
                    if isinstance(node, ast.Lambda) else node.name)
            symbol = ".".join(filter(None, [scope, name]))
            # Headers execute in the enclosing scope, not the new function.
            for field, value in ast.iter_fields(node):
                if field != "body":
                    for child in value if isinstance(value, list) else [value]:
                        if isinstance(child, ast.AST):
                            visit(child, scope, owner, depth)
            fact = {"location": {"file": file, "line": node.lineno,
                                 "column": node.col_offset + 1, "symbol": symbol},
                    "max_branch_nesting": 0, "branch_locations": []}
            fact["id"] = json.dumps([file, node.lineno, symbol], separators=(",", ":"))
            facts.append(fact)
            body = node.body if isinstance(node.body, list) else [node.body]
            for child in body:
                visit(child, symbol, fact, 0)
            return
        if isinstance(node, ast.ClassDef):
            # Class bodies are another scope; methods begin at depth zero.
            for child in node.body:
                visit(child, ".".join(filter(None, [scope, node.name])))
            return
        branch = isinstance(node, BRANCHES)
        if owner is not None and branch:
            owner["max_branch_nesting"] = max(owner["max_branch_nesting"], depth + 1)
            owner["branch_locations"].append(
                {"file": file, "line": node.lineno, "column": node.col_offset + 1})
        for child in ast.iter_child_nodes(node):
            visit(child, scope, owner, depth + int(branch))

    visit(tree, module)
    return facts


def build_context(targets: list[str]) -> dict:
    root = Path.cwd().resolve()
    roots = project_import_roots(root)
    patterns = exclusion_patterns()
    functions, skipped, parsed = [], [], []
    for path in sorted(collect_candidates(targets)):
        if excluded(path, root, patterns, os.environ.get("PYGUARD_EXCLUDE_TESTS") == "1"):
            continue
        file = relative_path(path, root)
        try:
            tree = read_tree(path)
        except Exception as error:
            skipped.append({"file": file, "reason": str(error)})
            continue
        parsed.append(str(path))
        functions.extend(function_facts(
            tree, file, module_name(path, nearest_import_root(path, roots))))
    # Reuse the existing pyan3 function graph; AST facts add lambdas as
    # independent evidence boundaries without changing call-resolution policy.
    index = {(f["location"]["file"], f["location"]["line"],
              f["location"]["symbol"]): f["id"] for f in functions}
    calls, graph_error = [], ""
    try:
        graph, dropped = build_function_graph(parsed, root)
        identifiers = [index.get((f["file"], f["line"], f["symbol"]))
                       for f in graph["functions"]]
        for call in graph["calls"]:
            caller, callee = identifiers[call["caller"]], identifiers[call["callee"]]
            if caller is not None and callee is not None:
                calls.append({"caller": caller, "callee": callee})
            else:
                graph_error = "A resolved call endpoint has no AST nesting fact."
        skipped.extend({"file": relative_path(Path(path), root),
                        "reason": "Not parsed by pyan3"} for path in dropped)
    except Exception as error:
        graph_error = str(error)
    return {"schema_version": "1", "functions": functions, "calls": calls,
            "partial": bool(skipped or graph_error), "skipped": skipped,
            "limitation": "Branch nesting is AST evidence, not a new complexity score. "
                          "Call edges retain the existing pyan3 static-resolution limits.",
            "graph_error": graph_error}



def main():
    artifact = Path(sys.argv[1])
    graph = build_context(sys.argv[2:])
    artifact.write_text(json.dumps(graph, indent=2) + "\n", encoding="utf-8")
    print(json.dumps({"schema_version": "1", "findings": [],
                      "measurements": [{"metric": "function.max_branch_nesting",
                                        "level": "code", "location": f["location"],
                                        "value": f["max_branch_nesting"], "unit": "levels"}
                                       for f in graph["functions"]],
                      "artifacts": [{"kind": "maintainability_graph", "path": str(artifact)}]}))


if __name__ == "__main__":
    main()
