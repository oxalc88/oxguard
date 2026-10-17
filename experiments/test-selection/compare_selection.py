"""Deterministic shadow comparisons. No selection here authorizes skipping CI."""
import argparse
import hashlib
import json
import math
from pathlib import Path
import random
import statistics
import time
from collect import read_report, write_report


def evidence(row):
    # Union preserves observed variation; variation itself forces retention below.
    return set().union(*map(set, row["coverage"])) | {"defect:" + d for d in row.get("defects", [])}


def cost(row):
    return statistics.median(row["samples_ms"])


def validate(rows):
    if not rows or len({r["id"] for r in rows}) != len(rows):
        raise ValueError("Empty or duplicate test IDs")
    for r in rows:
        if not r["samples_ms"] or any(not math.isfinite(x) or x <= 0 for x in r["samples_ms"]):
            raise ValueError("Missing/invalid measured costs")
        if len(r["coverage"]) != len(r["samples_ms"]) or len(r["outcomes"]) != len(r["samples_ms"]):
            raise ValueError("Missing samples")
        if any(x != "pass" for x in r["outcomes"]):
            raise ValueError("Failed/skipped tests cannot justify reduction")


def mandatory(row, protected):
    return ((protected and row.get("protected", True)) or not evidence(row) or
            any(s != row["coverage"][0] for s in row["coverage"]))


def greedy(rows, initial=()):
    chosen = set(initial)
    universe = set().union(*(evidence(r) for r in rows))
    covered = set().union(*(evidence(rows[i]) for i in chosen))
    while covered != universe:
        candidates = [i for i in range(len(rows)) if i not in chosen and evidence(rows[i]) - covered]
        best = min(candidates, key=lambda i: (-len(evidence(rows[i]) - covered) / cost(rows[i]), rows[i]["id"]))
        chosen.add(best)
        covered |= evidence(rows[best])
    return chosen


def clusters(rows, k, fuzzy, seed=0):
    # Binary coverage vectors; duration is used for representatives, not similarity.
    vectors = [evidence(r) for r in rows]
    rng = random.Random(seed)
    centers = [{u: 1.0 for u in sorted(vectors[i])} for i in rng.sample(range(len(rows)), k)]
    membership = None
    for _ in range(40):
        norms = [sum(x * x for x in c.values()) for c in centers]
        distances = [[max(0.0, len(v) + norms[j] - 2 * sum(c.get(u, 0) for u in sorted(v)))
                      for j, c in enumerate(centers)] for v in vectors]
        membership = []
        for d in distances:
            zeros = [i for i, x in enumerate(d) if x == 0]
            if fuzzy:
                membership.append([1 / len(zeros) if j in zeros else 0 for j in range(k)] if zeros else
                                  [(1 / x) / sum(1 / v for v in d) for x in d])
            else:
                nearest = min(range(k), key=lambda j: (d[j], j))
                membership.append([float(j == nearest) for j in range(k)])
        updated = []
        for j in range(k):
            weights = [m[j] ** (2 if fuzzy else 1) for m in membership]
            total = sum(weights)
            center = {}
            if total:
                for v, w in zip(vectors, weights):
                    if w:
                        for u in sorted(v):
                            center[u] = center.get(u, 0) + w / total
            updated.append(center if total else centers[j])
        if updated == centers:
            break
        centers = updated
    groups = [set() for _ in range(k)]
    for i, m in enumerate(membership):
        maximum = max(m)
        # FCM keeps overlapping assignments at least as large as uniform membership.
        for j, value in enumerate(m):
            if value == maximum or (fuzzy and value >= 1 / k):
                groups[j].add(i)
    return [group for group in groups if group]


def select(rows, approach, protected=True, changed=None, k=5, seed=0):
    validate(rows)
    pins = {i for i, r in enumerate(rows) if mandatory(r, protected)}
    if approach == "impact":
        # Coverage is a dynamic dependency LOWER bound. Unmapped changes retain all.
        files = {u.split(":v8:")[0].split(":go:")[0] for r in rows for u in evidence(r)
                 if not u.startswith("defect:")}
        if not changed or not set(changed) <= files:
            return set(range(len(rows)))
        return pins | {i for i, r in enumerate(rows) if r.get("file") in changed or
                       any(u.startswith(f + ":") for f in changed for u in evidence(r))}
    if approach in ["kmeans", "fuzzy"]:
        for group in clusters(rows, min(k, len(rows)), approach == "fuzzy", seed):
            # Fastest measured test, then repair ALL coverage/defect evidence globally.
            pins.add(min(group, key=lambda i: (cost(rows[i]), rows[i]["id"])))
    return greedy(rows, pins)


def metrics(rows, chosen):
    covered = set().union(*(evidence(rows[i]) for i in chosen))
    universe = set().union(*(evidence(r) for r in rows))
    missing = sorted(universe - covered)
    return {"selected": len(chosen), "total": len(rows),
            "estimated_isolated_ms": sum(cost(rows[i]) for i in chosen),
            "coverage_units_retained": len([u for u in covered if not u.startswith("defect:")]),
            "coverage_units_total": len([u for u in universe if not u.startswith("defect:")]),
            "known_defects_missed": [u for u in missing if u.startswith("defect:")],
            "missing_units": missing,
            "test_ids": [rows[i]["id"] for i in sorted(chosen)]}


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("input", type=Path)
    p.add_argument("output", type=Path)
    p.add_argument("--changed", action="append", default=[])
    args = p.parse_args()
    data = read_report(args.input)
    rows = data["tests"]
    validate(rows)
    results = []
    for protected in [False, True]:
        for method in ["greedy", "kmeans", "fuzzy", "impact"]:
            for k in ([5, 10, 15] if method in ["kmeans", "fuzzy"] else [5]):
                start = time.perf_counter()
                chosen = select(rows, method, protected, args.changed, k)
                selection_ms = (time.perf_counter() - start) * 1000
                samples = []
                for sample in range(min(len(r["samples_ms"]) for r in rows)):
                    timed = [dict(r, samples_ms=[r["samples_ms"][sample]] * len(r["samples_ms"])) for r in rows]
                    samples.append(select(timed, method, protected, args.changed, k))
                union = set.union(*samples)
                stability = len(set.intersection(*samples)) / len(union) if union else 1.0
                result = metrics(rows, chosen)
                result["timing_sample_selection_jaccard"] = stability
                result.update({"approach": method, "k": k if method in ["kmeans", "fuzzy"] else None,
                               "protected": protected, "selection_ms": selection_ms,
                               "selection_sha256": hashlib.sha256("\n".join(result["test_ids"]).encode()).hexdigest()})
                results.append(result)
    report = {"mode": "shadow", "safe_to_demote": False, "seed": 0,
              "input_sha256": hashlib.sha256(args.input.read_bytes()).hexdigest(),
              "full": metrics(rows, set(range(len(rows)))), "comparisons": results,
              "warning": "Coverage retention is not defect-detection equivalence. Unmeasured suites remain mandatory."}
    write_report(args.output, report)
    print(json.dumps([{k: r[k] for k in ["approach", "k", "protected", "selected", "estimated_isolated_ms"]} for r in results]))


if __name__ == "__main__":
    main()
