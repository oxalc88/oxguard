"""Rank defined Python functions by distinct callers using pyan3's use edges."""

import ast
import json
import sys
from pathlib import Path
from _paths import collect_paths

# Analyzer scripts may run from a private directory; artifacts belong to the project.
OUTPUT = Path.cwd() / "CRITICALITY.md"


def main() -> int:
    machine = "--json" in sys.argv[1:]
    targets = [arg for arg in sys.argv[1:] if arg != "--json"] or ["functions", "cdk"]
    result = {
        "schema_version": "1",
        "findings": [],
        "measurements": [],
        "artifacts": [],
    }

    def failure(category: str, message: str) -> int:
        if machine:
            result["error"] = {"category": category, "message": message}
            print(json.dumps(result, sort_keys=True))
        else:
            print(f"WARNING: {message}")
        return 1

    try:
        from pyan.analyzer import CallGraphVisitor
    except ImportError:
        return failure(
            "tool_missing", "pyan3 is not installed. Run: uv add --group dev pyan3"
        )
    try:
        files = sorted(
            {str(p.resolve()) for p in collect_paths(targets, "criticality")}
        )
        if not files:
            return failure(
                "invalid_configuration",
                "No Python source files found in the requested scope.",
            )
        visitor = CallGraphVisitor(files)
        functions = {
            n
            for group in visitor.nodes.values()
            for n in group
            if n.defined
            and n.filename in files
            and isinstance(n.ast_node, (ast.FunctionDef, ast.AsyncFunctionDef))
        }
        callers = {n: set() for n in functions}
        for caller, callees in visitor.uses_edges.items():
            if caller in functions:
                for callee in callees:
                    if callee in functions:
                        callers[callee].add(caller)
        ranked = sorted(
            functions,
            key=lambda n: (
                -len(callers[n]),
                n.filename,
                n.ast_node.lineno,
                n.get_name(),
            ),
        )
        lines = [
            "# Criticality Analysis",
            "",
            "Functions ranked by distinct defined function callers. Repeated calls count once; namespace containment and external functions are excluded.",
            "",
            "| Rank | Function | Callers |",
            "|------|----------|---------|",
        ]
        for rank, node in enumerate(ranked, 1):
            count = len(callers[node])
            location = {
                "file": str(Path(node.filename).relative_to(Path.cwd())),
                "line": node.ast_node.lineno,
                "symbol": node.get_name(),
            }
            result["measurements"].append(
                {
                    "metric": "criticality.in_degree",
                    "level": "structure",
                    "location": location,
                    "value": count,
                    "unit": "callers",
                }
            )
            if rank <= 30 and count:
                lines.append(f"| {rank} | `{node.get_label()}` | {count} |")
                result["findings"].append(
                    {
                        "rule": "pyguard.criticality.ranked",
                        "level": "structure",
                        "severity": "info",
                        "location": location,
                        "observed": count,
                        "evidence": f"{node.get_name()} has {count} distinct callers (rank {rank}).",
                    }
                )
        try:
            OUTPUT.write_text("\n".join(lines) + "\n", encoding="utf-8")
        except OSError as error:
            return failure("artifact_failure", f"Cannot write CRITICALITY.md: {error}")
        result["artifacts"].append({"kind": "criticality", "path": str(OUTPUT)})
    except SystemExit as error:
        return int(error.code or 1)
    except Exception as error:
        return failure("adapter_failure", f"Criticality analysis failed: {error}")
    if machine:
        print(json.dumps(result, sort_keys=True))
    else:
        print(f"Criticality analysis written to {OUTPUT}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
