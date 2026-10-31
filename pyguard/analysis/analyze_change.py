"""Measure inert sources with the candidate's trusted structure and Radon tools."""
from __future__ import annotations

import importlib.metadata
import json
import os
import sys
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
            "cyclomatic": sum(complexities),
            "maintainability_index": mi_visit(source, multi=True),
            "halstead_volume": h_visit(source).total.volume,
            "branches": sum(branches),
            "max_function_branches": max(branches, default=0),
        })
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
            except Exception as error:
                profile["metrics_error"] = f"{type(error).__name__}: {error}"
        output[side] = profile
    print(json.dumps(output, sort_keys=True, allow_nan=False))


if __name__ == "__main__":
    main()
