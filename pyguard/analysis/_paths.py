"""Shared path collection utility for analysis scripts."""

from __future__ import annotations

import fnmatch
import os
import sys
from pathlib import Path


def _test_exclude_patterns() -> list[str]:
    """Return file-name glob patterns to exclude, driven by Go-set env vars.

    PYGUARD_EXCLUDE_TESTS=1   → add conventional test-file patterns.
    PYGUARD_EXCLUDE_GLOBS=... → comma-separated extra globs from [tool.pyguard] exclude.
    """
    patterns: list[str] = []
    if os.environ.get("PYGUARD_EXCLUDE_TESTS") == "1":
        # Conventional Python test file conventions — not project-specific.
        patterns += ["test_*.py", "*_test.py", "conftest.py"]
    raw_globs = os.environ.get("PYGUARD_EXCLUDE_GLOBS", "")
    if raw_globs:
        patterns += [g.strip() for g in raw_globs.split(",") if g.strip()]
    return patterns


def _is_excluded(path: Path, patterns: list[str]) -> bool:
    """Return True if path matches any of the exclusion glob patterns."""
    name = path.name
    return any(fnmatch.fnmatch(name, pat) for pat in patterns)


def collect_paths(targets: list[str], gate: str) -> list[Path]:
    exclude_patterns = _test_exclude_patterns()
    paths: list[Path] = []
    for target in targets:
        p = Path(target)
        if not p.exists():
            print(f"  [FAIL] {gate} — path not found: {p}", file=sys.stderr)
            sys.exit(1)
        if p.is_dir():
            paths.extend(
                q
                for q in sorted(p.rglob("*.py"))
                if "__pycache__" not in q.parts
                and not _is_excluded(q, exclude_patterns)
            )
        else:
            if not _is_excluded(p, exclude_patterns):
                paths.append(p)
    return paths
