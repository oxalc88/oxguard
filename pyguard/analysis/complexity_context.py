"""AST branch evidence for radon; never a new complexity gate or score."""
from __future__ import annotations

import ast
import json
import os
import sys
import tokenize
from pathlib import Path

from analyze_structure import (
    build_function_graph, collect_candidates, excluded, exclusion_patterns, module_name,
    nearest_import_root, project_import_roots, read_tree, relative_path,
)

BRANCHES = (ast.If, ast.IfExp, ast.For, ast.AsyncFor, ast.While,
            ast.Match, ast.ExceptHandler)
FUNCTIONS = (ast.FunctionDef, ast.AsyncFunctionDef, ast.Lambda)


def function_facts(tree: ast.AST, file: str, module: str, source: str) -> list[dict]:
    facts = []
    lines = source.replace("\r\n", "\n").replace("\r", "\n").split("\n")

    def column(node):
        line = lines[node.lineno - 1]
        return len(line.encode("utf-8")[:node.col_offset].decode("utf-8")) + 1

    def visit(node, scope, owner=None, depth=0):
        if isinstance(node, FUNCTIONS):
            name = (f"<lambda>@{node.lineno}:{column(node)}"
                    if isinstance(node, ast.Lambda) else node.name)
            symbol = ".".join(filter(None, [scope, name]))
            # Headers execute in the enclosing scope, not the new function.
            for field, value in ast.iter_fields(node):
                if field != "body":
                    for child in value if isinstance(value, list) else [value]:
                        if isinstance(child, ast.AST):
                            visit(child, scope, owner, depth)
            fact = {"location": {"file": file, "line": node.lineno,
                                 "column": column(node), "symbol": symbol},
                    "max_branch_nesting": 0, "branch_locations": []}
            fact["id"] = json.dumps([file, node.lineno, symbol], separators=(",", ":"))
            facts.append(fact)
            body = node.body if isinstance(node.body, list) else [node.body]
            for child in body:
                visit(child, symbol, fact, 0)
            return
        if isinstance(node, ast.ClassDef):
            # Bases, keywords and decorators execute in the enclosing scope.
            for field, value in ast.iter_fields(node):
                if field != "body":
                    for child in value if isinstance(value, list) else [value]:
                        if isinstance(child, ast.AST):
                            visit(child, scope, owner, depth)
            # Class bodies are another scope; methods begin at depth zero.
            for child in node.body:
                visit(child, ".".join(filter(None, [scope, node.name])))
            return
        branch = isinstance(node, BRANCHES)
        if owner is not None and branch:
            owner["max_branch_nesting"] = max(owner["max_branch_nesting"], depth + 1)
            owner["branch_locations"].append(
                {"file": file, "line": node.lineno, "column": column(node)})
        for child in ast.iter_child_nodes(node):
            visit(child, scope, owner, depth + int(branch))

    visit(tree, module)
    return facts


def build_context(targets: list[str]) -> dict:
    root = Path.cwd().resolve()
    roots = project_import_roots(root)
    patterns = exclusion_patterns()
    functions, skipped, parsed = [], [], []
    local_symbols = {}
    for path in sorted(collect_candidates(targets)):
        if excluded(path, root, patterns, os.environ.get("PYGUARD_EXCLUDE_TESTS") == "1"):
            continue
        file = relative_path(path, root)
        try:
            tree = read_tree(path)
            with tokenize.open(path) as handle:
                source = handle.read()
        except Exception as error:
            skipped.append({"file": file, "reason": str(error)})
            continue
        parsed.append(str(path))
        module = module_name(path, nearest_import_root(path, roots))
        facts = function_facts(tree, file, module, source)
        functions.extend(facts)
        for fact in facts:
            location = fact["location"]
            local_symbols.setdefault((file, location["line"]), []).append(
                (location["symbol"].removeprefix(module + "."), fact["id"]))
    # Reuse the existing pyan3 function graph; AST facts add lambdas as
    # independent evidence boundaries without changing call-resolution policy.
    def ast_identifier(function):
        # File and definition line anchor the identity. Match the complete
        # local qualified name, allowing pyan to choose a different module root.
        matches = [identifier for symbol, identifier in local_symbols.get(
            (function["file"], function["line"]), [])
            if function["symbol"] == symbol or function["symbol"].endswith("." + symbol)]
        return matches[0] if len(matches) == 1 else None
    calls, graph_error = [], ""
    try:
        graph, dropped = build_function_graph(parsed, root)
        identifiers = [ast_identifier(f) for f in graph["functions"]]
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
