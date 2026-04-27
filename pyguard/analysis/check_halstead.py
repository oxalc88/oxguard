"""check_halstead.py — enforce Halstead complexity thresholds per function.

Mirrors tortastudios/python-ai-guardrails-template scripts/check_halstead.py.
Rationale: cyclomatic complexity (CC) measures control-flow branches; Halstead
measures cognitive vocabulary — the number of distinct operators and operands.
A function can have low CC but high Halstead effort if it chains many distinct
operations. Both dimensions are needed for a complete complexity picture.

Thresholds (from upstream template):
    effort     <= 50,000  cognitive load to write/understand the function
    difficulty <= 30      complexity relative to the operator/operand vocabulary
    bugs       <= 0.4     estimated defect density (volume / 3000)

Usage:
    python tools/analysis/check_halstead.py [path ...]

Paths may be files or directories. Defaults to the current directory.
Exit 0 on success, 1 on any violation or missing path.
"""

from __future__ import annotations

import sys
from dataclasses import dataclass
from pathlib import Path

from radon.metrics import h_visit

from _paths import collect_paths

MAX_EFFORT: float = 50_000.0
MAX_DIFFICULTY: float = 30.0
MAX_BUGS: float = 0.4


@dataclass
class Violation:
    file: Path
    function: str
    effort: float
    difficulty: float
    bugs: float


def check_file(path: Path) -> list[Violation]:
    try:
        source = path.read_text(encoding="utf-8")
    except OSError:
        return []

    try:
        result = h_visit(source)
    except (SyntaxError, IndentationError):
        # ruff/mypy report parse errors; skip here to avoid duplicate noise
        return []

    violations: list[Violation] = []
    for name, report in result.functions:
        if (
            report.effort > MAX_EFFORT
            or report.difficulty > MAX_DIFFICULTY
            or report.bugs > MAX_BUGS
        ):
            violations.append(
                Violation(
                    file=path,
                    function=name,
                    effort=report.effort,
                    difficulty=report.difficulty,
                    bugs=report.bugs,
                )
            )
    return violations


def main() -> None:
    targets = sys.argv[1:] or ["."]
    paths = collect_paths(targets, gate="halstead")

    all_violations: list[Violation] = []
    for path in paths:
        all_violations.extend(check_file(path))

    if not all_violations:
        print(f"  [OK]   halstead — all functions within thresholds ({len(paths)} file(s))")
        sys.exit(0)

    print("  [FAIL] halstead — functions exceeding complexity thresholds:")
    for v in all_violations:
        breaches: list[str] = []
        if v.effort > MAX_EFFORT:
            breaches.append(f"effort={v.effort:.0f} (max {MAX_EFFORT:.0f})")
        if v.difficulty > MAX_DIFFICULTY:
            breaches.append(f"difficulty={v.difficulty:.1f} (max {MAX_DIFFICULTY:.0f})")
        if v.bugs > MAX_BUGS:
            breaches.append(f"bugs={v.bugs:.3f} (max {MAX_BUGS})")
        print(f"         {v.file}  {v.function}")
        for breach in breaches:
            print(f"           {breach}")
    print(f"\n  {len(all_violations)} violation(s) found")
    sys.exit(1)


if __name__ == "__main__":
    main()
