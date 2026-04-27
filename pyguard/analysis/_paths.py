"""Shared path collection utility for analysis scripts."""

from __future__ import annotations

import sys
from pathlib import Path


def collect_paths(targets: list[str], gate: str) -> list[Path]:
    paths: list[Path] = []
    for target in targets:
        p = Path(target)
        if not p.exists():
            print(f"  [FAIL] {gate} — path not found: {p}", file=sys.stderr)
            sys.exit(1)
        if p.is_dir():
            paths.extend(
                q for q in sorted(p.rglob("*.py")) if "__pycache__" not in q.parts
            )
        else:
            paths.append(p)
    return paths
