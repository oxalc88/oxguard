"""Replay shadow sets and measure a small, explicit contract-mutation matrix."""
import argparse
import json
import re
import shutil
import tempfile
from pathlib import Path

from collect import HERE, NODE_FILES, ROOT, read_report, run, write_report
from compare_selection import select

MUTATIONS = [
    ("compare.reject-broken-candidate", "evals/compare.cjs", "candidate_ready: candidate.cases.every", "candidate_ready: true || candidate.cases.every"),
    ("compare.reject-schema", "evals/compare.cjs", "report.schema_version !== '1'", "false"),
    ("compare.reject-duplicate-cases", "evals/compare.cjs", "new Set(candidate.cases.map(c => c.id)).size !== candidate.cases.length", "false"),
    ("detection.count-missed-findings", "evals/run.cjs", "false_negative: expected.filter(f => f.category === 'quality').length - tp", "false_negative: 0"),
]


def mutation_matrix():
    matrix = []
    with tempfile.TemporaryDirectory(prefix="oxguard-mutants-") as temporary:
        root = Path(temporary)
        # Only the harness and its immutable inputs. No source mutation in the checkout.
        for folder in ["evals", "tsguard/skill", "pyguard/skill", "npm/testdata"]:
            shutil.copytree(ROOT / folder, root / folder)
        args = ["node", "--test", "--test-concurrency=1", f"--test-reporter={HERE / 'reporter.cjs'}"] + NODE_FILES
        baseline, _ = run(args, root)
        baseline_events = [json.loads(line) for line in baseline.stdout.splitlines()]
        if baseline.returncode or not baseline_events or any(e["status"] != "pass" for e in baseline_events):
            raise RuntimeError("Isolated mutation control did not pass: " + baseline.stdout + baseline.stderr)
        for identifier, file, before, after in MUTATIONS:
            path = root / file
            original = path.read_text()
            if original.count(before) != 1:
                raise ValueError(f"Mutation no longer matches exactly once: {identifier}")
            try:
                path.write_text(original.replace(before, after))
                p, elapsed = run(args, root)
                events = [json.loads(line) for line in p.stdout.splitlines()]
                if len(events) != len(baseline_events):
                    raise RuntimeError("Mutant changed test inventory; not a valid detection measurement")
                killers = [f"{Path(e['file']).relative_to(root).as_posix()}::{e['name']}"
                           for e in events if e["status"] == "fail"]
                if (p.returncode != 0) != bool(killers):
                    raise RuntimeError("Mutation runner error, not an assertion failure")
                matrix.append({"id": identifier, "file": file, "replacement": [before, after],
                               "detected_by": sorted(killers), "wall_ms": elapsed})
            finally:
                path.write_text(original)
    return matrix


def verify_mutants(rows, selected):
    """Run the actual reduced Node commands on each mutant, not just intersect IDs."""
    measured = []
    with tempfile.TemporaryDirectory(prefix="oxguard-selected-mutants-") as temporary:
        root = Path(temporary)
        for folder in ["evals", "tsguard/skill", "pyguard/skill", "npm/testdata"]:
            shutil.copytree(ROOT / folder, root / folder)
        chosen = [rows[i] for i in sorted(selected) if rows[i]["runner"] == "node"]
        names = sorted({r["name"] for r in chosen})
        files = sorted({r["file"] for r in chosen})
        pattern = "^(" + "|".join(re.escape(n) for n in names) + ")$"
        command = ["node", "--test", "--test-concurrency=1", f"--test-reporter={HERE / 'reporter.cjs'}",
                   "--test-name-pattern=" + pattern] + files
        for identifier, file, before, after in MUTATIONS:
            path = root / file
            original = path.read_text()
            try:
                path.write_text(original.replace(before, after))
                p, elapsed = run(command, root)
                events = [json.loads(line) for line in p.stdout.splitlines()]
                if len(events) != len(chosen):
                    raise RuntimeError("Reduced mutant run changed test inventory")
                failed = [e["name"] for e in events if e["status"] == "fail"]
                if bool(p.returncode) != bool(failed):
                    raise RuntimeError("Reduced mutant runner error")
                measured.append({"id": identifier, "detected": bool(failed), "wall_ms": elapsed})
            finally:
                path.write_text(original)
    return measured


def replay(rows, selected):
    groups = {}
    for i in sorted(selected):
        r = rows[i]
        key = (r["runner"], r.get("file", r.get("module")), r.get("package"))
        groups.setdefault(key, []).append(r["name"])
    total, commands = 0, []
    for (runner, location, package), names in groups.items():
        pattern = "^(" + "|".join(re.escape(n) for n in names) + ")$"
        if runner == "node":
            command = ["node", "--test", "--test-concurrency=1", f"--test-name-pattern={pattern}", location]
            cwd = ROOT
        else:
            command = ["go", "test", "-count=1", "-run=" + pattern, package]
            cwd = ROOT / location
        p, elapsed = run(command, cwd)
        commands.append({"argv": command, "cwd": str(cwd.relative_to(ROOT)),
                         "exit_code": p.returncode, "wall_ms": elapsed})
        if p.returncode:
            raise RuntimeError(p.stdout + p.stderr)
        total += elapsed
    return {"wall_ms": total, "commands": commands, "passed": True}


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("input", type=Path)
    p.add_argument("output", type=Path)
    args = p.parse_args()
    data = read_report(args.input)
    rows = data["tests"]
    matrix = mutation_matrix()
    for row in rows:
        row["defects"] = [m["id"] for m in matrix if row["id"] in m["detected_by"]]
    results, cache = [], {}
    for method in ["greedy", "kmeans", "fuzzy", "impact"]:
        for protected in [False, True]:
            chosen = select(rows, method, protected, ["evals/compare.cjs"])
            ids = {rows[i]["id"] for i in chosen}
            key = tuple(sorted(ids))
            if key not in cache:
                cache[key] = replay(rows, chosen)
            results.append({"method": method, "protected": protected, "selected": len(chosen),
                            "replay": cache[key], "missed_mutants": [m["id"] for m in matrix
                             if m["detected_by"] and not ids.intersection(m["detected_by"])]})
    data["greedy_mutant_replay"] = verify_mutants(rows, select(rows, "greedy", False))
    data["mutation_matrix"] = matrix
    data["shadow_replays"] = results
    write_report(args.output, data)
    print(json.dumps({"mutants": matrix, "replays": [{k: v for k, v in r.items() if k != "replay"} |
                      {"wall_ms": r["replay"]["wall_ms"]} for r in results]}))


if __name__ == "__main__":
    main()
