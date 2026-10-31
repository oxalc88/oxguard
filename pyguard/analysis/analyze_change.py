"""Measure inert sources with the candidate's trusted structure, Radon and complexipy tools."""
from __future__ import annotations

import ast
import hashlib
import importlib.metadata
import io
import json
import os
import sys
import tokenize
from pathlib import Path

from analyze_structure import (
    TEST_FILE_PATTERNS,
    build_report,
    excluded,
    exclusion_patterns,
    read_tree,
    skipped_directory,
)


def validate_stubs(targets: list[str]) -> None:
    """Parse scoped stubs as input evidence, without adding graph/metric files."""
    root = Path.cwd()
    exclude_tests = os.environ.get("PYGUARD_EXCLUDE_TESTS") == "1"
    patterns = exclusion_patterns()
    if exclude_tests:
        patterns.extend(pattern + "i" for pattern in TEST_FILE_PATTERNS)
    candidates: set[Path] = set()
    for target in targets or ["."]:
        path = Path(target)
        if path.is_file():
            if path.suffix == ".pyi" and "__pycache__" not in path.resolve().parts:
                candidates.add(path.resolve())
            continue
        for directory, names, filenames in os.walk(path):
            names[:] = sorted(name for name in names if not skipped_directory(Path(name)))
            candidates.update(
                (Path(directory) / filename).resolve()
                for filename in filenames if filename.endswith(".pyi")
            )
    for path in sorted(candidates):
        if not excluded(path, root, patterns, exclude_tests):
            read_tree(path)


def file_metrics(report: dict[str, object], root: Path) -> list[dict[str, object]]:
    from radon.complexity import cc_visit
    from radon.metrics import h_visit, mi_visit
    import tokenize

    result = []
    for module in report["modules"]:
        with tokenize.open(root / module["file"]) as handle:
            source = handle.read()
        blocks = cc_visit(source)
        # Classes aggregate their methods; count functions/methods once, and
        # recurse into closures once. Module-level statements have no CC block.
        seen = set()

        def functions(items):
            for block in items:
                if id(block) in seen:
                    continue
                seen.add(id(block))
                if hasattr(block, "methods"):
                    yield from functions(block.methods)
                    yield from functions(block.inner_classes)
                else:
                    yield block
                    yield from functions(block.closures)

        complexities = [block.complexity for block in functions(blocks)]
        branches = [max(0, complexity - 1) for complexity in complexities]
        result.append({
            "file": module["file"],
            "source_hash": hashlib.sha256((root / module["file"]).read_bytes()).hexdigest(),
            "cyclomatic": sum(complexities),
            "maintainability_index": mi_visit(source, multi=True),
            "halstead_volume": h_visit(source).total.volume,
            "branches": sum(branches),
            "max_function_branches": max(branches, default=0),
        })
    return result


def function_metrics(report: dict[str, object], root: Path) -> list[dict[str, object]]:
    """Measure each AST definition, including closures, without executing source.

    complexipy folds closures into their parent. Analyze each definition as a
    standalone fragment so closures also have their own score. Parent scores
    retain native folding; totals therefore overlap for nested definitions.
    """
    from radon.complexity import cc_visit

    result = []
    for module in report["modules"]:
        with tokenize.open(root / module["file"]) as handle:
            source = handle.read()
        lines = io.StringIO(source).readlines()
        tree = ast.parse(source)

        def visit(node, parents=(), *, lines, file):
            for child in ast.iter_child_nodes(node):
                scope = parents
                if isinstance(child, (ast.FunctionDef, ast.AsyncFunctionDef, ast.ClassDef)):
                    scope = (*parents, child.name)
                if isinstance(child, (ast.FunctionDef, ast.AsyncFunctionDef)):
                    # Include decorators and directly preceding comments: native
                    # complexipy recognizes ignore comments above a definition.
                    start = min([child.lineno, *(d.lineno for d in child.decorator_list)]) - 1
                    while (start > 0 and lines[start - 1].lstrip().startswith("#")
                           and len(lines[start - 1]) - len(lines[start - 1].lstrip()) >= child.col_offset):
                        start -= 1
                    fragment = "".join(lines[start:child.end_lineno])
                    # A synthetic, inert suite lets the native parsers read an
                    # indented definition without altering multiline literals.
                    measured = "if True:\n" + fragment if child.col_offset else fragment
                    tokens = [t.string for t in tokenize.generate_tokens(io.StringIO(fragment).readline)
                              if t.type not in {tokenize.COMMENT, tokenize.NL, tokenize.NEWLINE,
                                                tokenize.INDENT, tokenize.DEDENT, tokenize.ENDMARKER}]
                    qualname = ".".join(scope)
                    metric = {
                        "id": f"{file}:{child.lineno}:{qualname}",
                        "location": {"file": file, "line": child.lineno, "symbol": qualname},
                        "fingerprint": hashlib.sha256("\0".join(tokens).encode("utf-8")).hexdigest(),
                        "cyclomatic": cc_visit(measured)[0].complexity,
                        "cognitive": None,
                    }
                    try:
                        from complexipy import code_complexity
                        rows = code_complexity(measured, with_plans=False).functions
                        if len(rows) == 1:
                            metric["cognitive"] = rows[0].complexity
                        else:
                            metric["cognitive_error"] = "complexipy omitted function (suppression or unsupported definition)"
                    except Exception as error:
                        metric["cognitive_error"] = f"{type(error).__name__}: {error}"
                    result.append(metric)
                visit(child, scope, lines=lines, file=file)

        visit(tree, lines=lines, file=module["file"])
    return result


def main() -> None:
    request = json.loads(sys.argv[1])
    os.environ["PYGUARD_EXCLUDE_TESTS"] = "1" if request["exclude_tests"] else "0"
    os.environ["PYGUARD_EXCLUDE_GLOBS"] = ",".join(request["exclude"] or [])
    output = {}
    for side in ("baseline", "candidate"):
        root = Path(request[side]).resolve()
        os.chdir(root)
        try:
            validate_stubs(request["dirs"])
            report = build_report(request["dirs"])
            # Version evidence is required independently of Radon measurement
            # success, so a missing metric tool reports INCOMPLETE_METRICS.
            tool_versions = {"python": sys.version, **{
                name: importlib.metadata.version(name)
                for name in ("grimp", "networkx", "pyan3")
            }}
            profile = {"structure": report, "versions": tool_versions}
        except Exception as error:
            output[side] = {"input_error": f"{type(error).__name__}: {error}"}
            continue
        if not report["partial"]:
            try:
                profile["files"] = file_metrics(report, root)
                profile["versions"]["radon"] = importlib.metadata.version("radon")
                profile["functions"] = function_metrics(report, root)
                try:
                    profile["versions"]["complexipy"] = importlib.metadata.version("complexipy")
                except importlib.metadata.PackageNotFoundError:
                    profile["cognitive_error"] = "complexipy is unavailable"
            except Exception as error:
                profile["metrics_error"] = f"{type(error).__name__}: {error}"
        output[side] = profile
    print(json.dumps(output, sort_keys=True, allow_nan=False))


if __name__ == "__main__":
    main()
