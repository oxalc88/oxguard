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

# Recursion failures prove the source is deeper than these thresholds can
# express, so they are reported as violations instead of skipped.
UNMEASURABLE_ANNOTATION = "annotation too deeply nested to measure"
UNPARSEABLE_SOURCE = "source too deeply nested to parse; type complexity not measured"


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
    # False when neither depth nor length could be measured (unparsable file).
    measured: bool = True


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


def source_segment(source: str, node: ast.expr) -> str:
    """Raw source text for node.

    ast.get_source_segment only reads character offsets, so it stays available
    when ast.unparse cannot render the node because it ran out of stack.
    """
    try:
        return ast.get_source_segment(source, node) or ""
    except (AttributeError, TypeError, ValueError):
        return ""


def unmeasurable_annotation(
    node: ast.expr,
    context: str,
    file: Path,
    source: str,
) -> Violation:
    """Report an annotation ast.unparse or annotation_depth could not measure.

    Reaching the recursion limit is itself an over-limit result, so the
    violation is always reported and its measurements are floored above the
    thresholds when the raw source cannot prove the breach.
    """
    segment = source_segment(source, node)
    try:
        depth = annotation_depth(node)
    except RecursionError:
        # The depth walk exhausted the stack: nesting is over the limit.
        depth = MAX_NESTING_DEPTH + 1
    length = len(segment)
    if length <= MAX_TYPE_LENGTH:
        length = MAX_TYPE_LENGTH + 1
    return Violation(
        file=file,
        line=node.lineno,
        column=node.col_offset + 1,
        context=context,
        annotation=UNMEASURABLE_ANNOTATION,
        depth=depth,
        length=length,
        suggestion=suggest_fix(segment or context),
    )


def unparsable_file(path: Path) -> Violation:
    """Report a file ast.parse could not build instead of skipping it."""
    return Violation(
        file=path,
        line=0,
        column=0,
        context="file",
        annotation=UNPARSEABLE_SOURCE,
        depth=0,
        length=0,
        suggestion="Split deeply nested annotations into TypeAliases",
        measured=False,
    )


def variable_context(target: ast.expr) -> str:
    try:
        return f"variable '{ast.unparse(target)}'"
    except (AttributeError, KeyError, NotImplementedError, RecursionError, TypeError, ValueError):
        # The assignment target is malformed or too deep to render; the
        # annotation itself is still checked.
        return "variable"


def check_annotation(
    node: ast.expr,
    context: str,
    file: Path,
    source: str,
) -> Violation | None:
    try:
        text = ast.unparse(node)
        depth = annotation_depth(node)
    except RecursionError:
        # Rendering or walking the annotation exhausted the stack, so it is
        # over any nesting/length limit: report it rather than skipping it.
        return unmeasurable_annotation(node, context, file, source)
    except (AttributeError, KeyError, NotImplementedError, TypeError, ValueError):
        # Malformed annotation nodes are skipped: a malformed annotation must
        # not crash the gate.
        return None
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
    except RecursionError:
        # A file the parser cannot build is not a clean pass: report it as an
        # explicit violation instead of silently skipping it.
        return [unparsable_file(path)]

    violations: list[Violation] = []

    for node in ast.walk(tree):
        if isinstance(node, ast.AnnAssign) and node.annotation:
            v = check_annotation(
                node.annotation,
                variable_context(node.target),
                path,
                source,
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
                        source,
                    )
                    if v:
                        violations.append(v)
            if node.returns:
                v = check_annotation(
                    node.returns,
                    f"return type of {node.name}()",
                    path,
                    source,
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
            location: dict[str, str | int] = {"file": v.file.as_posix()}
            if v.line:
                location["line"] = v.line
            if v.column:
                location["column"] = v.column
            location["symbol"] = v.context
            finding: dict[str, object] = {
                "rule": "pyguard.types.annotation_complexity",
                "severity": "error",
                "location": location,
                "evidence": (
                    f"Annotation depth {v.depth}, length {v.length}: {v.annotation}"
                    if v.measured
                    else v.annotation
                ),
                "remediation": v.suggestion,
            }
            if v.measured:
                observed, threshold = (
                    (v.depth, MAX_NESTING_DEPTH)
                    if v.depth > MAX_NESTING_DEPTH
                    else (v.length, MAX_TYPE_LENGTH)
                )
                finding["observed"] = observed
                finding["threshold"] = threshold
            findings.append(finding)
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
        location = f"{v.file}:{v.line}" if v.line else str(v.file)
        print(f"         {location}  {v.context}")
        print(f"           annotation: {v.annotation}")
        if v.measured:
            print(f"           depth={v.depth}  length={v.length}")
        print(f"           -> {v.suggestion}")
    print(f"\n  {len(all_violations)} violation(s) found")
    sys.exit(1)


if __name__ == "__main__":
    main()
