"""Analyze function criticality via call graph.

Ranks functions by in-degree (how many callers they have).
High in-degree = high criticality = high risk to change.

Output: CRITICALITY.md in the project root.

Usage:
    uv run python tools/analysis/analyze_criticality.py

Dependencies: pyan3, networkx (install via: uv add --group dev pyan3 networkx)
"""

import sys
import tempfile
from pathlib import Path

try:
    import networkx as nx  # type: ignore[import-untyped]
    import pyan  # type: ignore[import-untyped]

    HAS_DEPS = True
except ImportError:
    HAS_DEPS = False

SOURCE_DIRS = ["functions", "cdk"]
OUTPUT = Path(__file__).parent.parent.parent / "CRITICALITY.md"


def find_python_files(dirs: list[str]) -> list[str]:
    files: list[str] = []
    for d in dirs:
        files.extend(str(p) for p in Path(d).rglob("*.py") if "__pycache__" not in str(p))
    return files


def main() -> int:
    if not HAS_DEPS:
        msg = "pyan3/networkx not installed. Run: uv add --group dev pyan3 networkx"
        print(f"WARNING: {msg}")
        OUTPUT.write_text(f"# Criticality Analysis\n\n{msg}\n")
        return 0

    files = find_python_files(SOURCE_DIRS)
    if not files:
        print("No Python files found.")
        return 1

    try:
        callgraph = pyan.create_callgraph(files, format="dot")
    except Exception as e:
        print(f"WARNING: pyan3 failed: {e}")
        OUTPUT.write_text(f"# Criticality Analysis\n\npyan3 failed: {e}\n")
        return 0

    try:
        with tempfile.NamedTemporaryFile(mode="w", suffix=".dot", delete=False) as f:
            f.write(callgraph)
            dot_path = f.name
        graph = nx.DiGraph(nx.drawing.nx_pydot.read_dot(dot_path))
    except Exception as e:
        print(f"WARNING: Could not parse call graph: {e}")
        OUTPUT.write_text(f"# Criticality Analysis\n\nGraph parse failed: {e}\n")
        return 0

    # Rank by in-degree (number of callers)
    ranked = sorted(graph.in_degree(), key=lambda x: x[1], reverse=True)

    lines = [
        "# Criticality Analysis",
        "",
        "Functions ranked by in-degree (number of callers). High = high risk to change.",
        "",
        "| Rank | Function | Callers |",
        "|------|----------|---------|",
    ]
    for i, (node, degree) in enumerate(ranked[:30], 1):
        if degree > 0:
            lines.append(f"| {i} | `{node}` | {degree} |")

    OUTPUT.write_text("\n".join(lines) + "\n")
    print(f"Criticality analysis written to {OUTPUT}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
