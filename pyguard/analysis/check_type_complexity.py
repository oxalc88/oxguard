"""check_type_complexity.py — flag overly nested or long type annotations.

Mirrors tortastudios/python-ai-guardrails-template scripts/check_type_complexity.py.
Rationale: mypy checks correctness; this checks legibility. An annotation like
dict[str, list[dict[str, str]]] type-checks cleanly but obscures meaning — it
should be extracted into a TypedDict, @dataclass, or TypeAlias.

Usage:
    python tools/analysis/check_type_complexity.py [path ...]

Paths may be files or directories. Defaults to the current directory.
Exit 0 on success, 1 on any violation or missing path.
"""

from __future__ import annotations

import ast
import json
import sys
from dataclasses import dataclass
from pathlib import Path

from _paths import collect_paths

MAX_NESTING_DEPTH: int = 2
MAX_TYPE_LENGTH: int = 40


@dataclass
class Violation:
    file: Path
    line: int
    column: int
    context: str
    annotation: str
    depth: int
    length: int
    suggestion: str


def annotation_depth(node: ast.expr) -> int:
    if isinstance(node, ast.Subscript):
        return 1 + annotation_depth(node.slice)
    if isinstance(node, ast.Tuple):
        return max((annotation_depth(e) for e in node.elts), default=0)
    return 0


def suggest_fix(text: str) -> str:
    lower = text.lower()
    if "dict" in lower:
        return "Extract into a TypedDict or @dataclass"
    if "list" in lower or "tuple" in lower:
        return "Extract into a TypeAlias or @dataclass"
    return "Extract into a TypeAlias"


def check_annotation(
    node: ast.expr,
    context: str,
    file: Path,
) -> Violation | None:
    try:
        text = ast.unparse(node)
    except Exception:
        return None
    depth = annotation_depth(node)
    length = len(text)
    if depth > MAX_NESTING_DEPTH or length > MAX_TYPE_LENGTH:
        return Violation(
            file=file,
            line=node.lineno,
            column=node.col_offset + 1,
            context=context,
            annotation=text,
            depth=depth,
            length=length,
            suggestion=suggest_fix(text),
        )
    return None


def check_file(path: Path) -> list[Violation]:
    try:
        source = path.read_text(encoding="utf-8")
    except OSError:
        return []

    try:
        tree = ast.parse(source, filename=str(path))
    except SyntaxError:
        return []

    violations: list[Violation] = []

    for node in ast.walk(tree):
        if isinstance(node, ast.AnnAssign) and node.annotation:
            v = check_annotation(
                node.annotation,
                f"variable '{ast.unparse(node.target)}'",
                path,
            )
            if v:
                violations.append(v)

        if isinstance(node, ast.FunctionDef | ast.AsyncFunctionDef):
            for arg in node.args.args + node.args.kwonlyargs:
                if arg.annotation:
                    v = check_annotation(
                        arg.annotation,
                        f"argument '{arg.arg}' in {node.name}()",
                        path,
                    )
                    if v:
                        violations.append(v)
            if node.returns:
                v = check_annotation(
                    node.returns,
                    f"return type of {node.name}()",
                    path,
                )
                if v:
                    violations.append(v)

    return violations


def main() -> None:
    machine = "--json" in sys.argv[1:]
    targets = [arg for arg in sys.argv[1:] if arg != "--json"] or ["."]
    paths = collect_paths(targets, gate="types")

    all_violations: list[Violation] = []
    for path in paths:
        all_violations.extend(check_file(path))

    if machine:
        findings = []
        for v in all_violations:
            observed, threshold = (
                (v.depth, MAX_NESTING_DEPTH)
                if v.depth > MAX_NESTING_DEPTH
                else (v.length, MAX_TYPE_LENGTH)
            )
            findings.append(
                {
                    "rule": "pyguard.types.annotation_complexity",
                    "severity": "error",
                    "location": {
                        "file": v.file.as_posix(),
                        "line": v.line,
                        "column": v.column,
                        "symbol": v.context,
                    },
                    "observed": observed,
                    "threshold": threshold,
                    "evidence": f"Annotation depth {v.depth}, length {v.length}: {v.annotation}",
                    "remediation": v.suggestion,
                }
            )
        print(
            json.dumps(
                {
                    "schema_version": "1",
                    "findings": findings,
                    "measurements": [],
                    "artifacts": [],
                },
                sort_keys=True,
            )
        )
        sys.exit(1 if findings else 0)

    if not all_violations:
        print(f"  [OK]   types — annotations within thresholds ({len(paths)} file(s))")
        sys.exit(0)

    print("  [FAIL] types — annotation complexity violations:")
    for v in all_violations:
        print(f"         {v.file}:{v.line}  {v.context}")
        print(f"           annotation: {v.annotation}")
        print(f"           depth={v.depth}  length={v.length}")
        print(f"           -> {v.suggestion}")
    print(f"\n  {len(all_violations)} violation(s) found")
    sys.exit(1)


if __name__ == "__main__":
    main()
