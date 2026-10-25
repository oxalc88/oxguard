"""Offline, opt-in test-level evidence. Python stdlib + existing Node/Go runners."""
import argparse
import gzip
import hashlib
import json
import os
import platform
import re
import subprocess
import tempfile
import time
from pathlib import Path
from urllib.parse import urlparse
from urllib.request import url2pathname

ROOT = Path(__file__).resolve().parents[2]
HERE = Path(__file__).resolve().parent
NODE_FILES = ["evals/evals.test.cjs", "evals/python/evals.test.cjs",
              "evals/agent-actions.test.cjs", "evals/record-agent-command.test.cjs",
              "evals/maintainability-parity.test.cjs"]


def read_report(path):
    raw = path.read_bytes()
    return json.loads(gzip.decompress(raw) if path.suffix == ".gz" else raw)


def write_report(path, data):
    path.parent.mkdir(parents=True, exist_ok=True)
    raw = (json.dumps(data, indent=2) + "\n").encode("utf-8")
    path.write_bytes(gzip.compress(raw, mtime=0) if path.suffix == ".gz" else raw)


def run(command, cwd=ROOT, env=None, timeout=600):
    start = time.perf_counter()
    p = subprocess.run(command, cwd=cwd, env=env, capture_output=True, text=True,
                       encoding="utf-8", errors="replace", timeout=timeout, check=False)
    return p, (time.perf_counter() - start) * 1000


def node_run(files, pattern=None, coverage=None):
    args = ["node", "--test", "--test-concurrency=1", f"--test-reporter={HERE / 'reporter.cjs'}"]
    if pattern is not None:
        args += [f"--test-name-pattern={pattern}"]
    env = dict(os.environ)
    env.pop("NODE_V8_COVERAGE", None)
    if coverage:
        env["NODE_V8_COVERAGE"] = str(coverage)
    p, elapsed = run(args + files, env=env)
    events = [json.loads(line) for line in p.stdout.splitlines()]
    if p.returncode or not events or any(e["status"] != "pass" for e in events):
        # Filtered-out tests are not emitted on Node 24. Never accept empty or failed runs.
        raise RuntimeError(f"Node collection failed: {p.stderr}\n{p.stdout}")
    return events, elapsed


def v8_units(directory, tracked):
    """Executed V8 ranges, NOT line/branch coverage or behavioral equivalence."""
    units = set()
    for file in directory.glob("*.json"):
        for script in json.loads(file.read_text())["result"]:
            if not script["url"].startswith("file:"):
                continue
            source = Path(url2pathname(urlparse(script["url"]).path))
            try:
                relative = source.resolve().relative_to(ROOT).as_posix()
            except ValueError:
                continue
            if relative not in tracked or relative.endswith(".test.cjs"):
                continue
            for function in script["functions"]:
                for block in function["ranges"]:
                    if block["count"] > 0:
                        units.add(f"{relative}:v8:{block['startOffset']}:{block['endOffset']}")
    return sorted(units)


def collect_node(tracked, repeats, distribution=False):
    files = NODE_FILES + (["npm/distribution.test.cjs"] if distribution else [])
    events, full_ms = node_run(files)
    rows = []
    for event in events:
        if event["nesting"]:
            raise ValueError("Nested tests require a reviewed parent mapping; refusing ambiguous IDs")
        file = Path(event["file"]).resolve().relative_to(ROOT).as_posix()
        row = {"id": f"{file}::{event['name']}", "runner": "node", "file": file,
               "name": event["name"], "samples_ms": [], "coverage": [], "outcomes": [],
               "protected": True, "protection_reason": "existing contract/security/regression suite",
               "defects": [], "historical_failures": None}
        for _ in range(repeats):
            with tempfile.TemporaryDirectory(prefix="oxguard-v8-") as temporary:
                ev, elapsed = node_run([file], "^" + re.escape(event["name"]) + "$", Path(temporary))
                if len(ev) != 1 or ev[0]["name"] != event["name"]:
                    raise ValueError("Test ID did not select exactly one test")
                row["samples_ms"].append(elapsed)
                row["outcomes"].append(ev[0]["status"])
                row["coverage"].append(v8_units(Path(temporary), tracked))
        rows.append(row)
    return rows, {"node_full_wall_ms": full_ms, "node_full_test_count": len(events)}


def collect_go(repeats):
    rows, baselines = [], {}
    for module in ["contract", "tsguard", "pyguard"]:
        cwd = ROOT / module
        p, full_ms = run(["go", "test", "-count=1", "./..."], cwd)
        if p.returncode:
            raise RuntimeError(p.stdout + p.stderr)
        baselines[f"{module}_full_wall_ms"] = full_ms
        packages, _ = run(["go", "list", "./..."], cwd)
        if packages.returncode:
            raise RuntimeError(packages.stderr)
        for package in packages.stdout.splitlines():
            with tempfile.TemporaryDirectory(prefix="oxguard-go-") as temporary:
                binary = Path(temporary) / "suite.test.exe"
                p, _ = run(["go", "test", "-c", "-coverpkg=./...", "-o", str(binary), package], cwd)
                if p.returncode:
                    raise RuntimeError(p.stderr)
                if not binary.exists():  # package without tests
                    continue
                listed, _ = run([str(binary), "-test.list=^Test"], cwd)
                if listed.returncode:
                    raise RuntimeError(listed.stderr)
                for name in listed.stdout.splitlines():
                    if not re.fullmatch(r"Test\w+", name):
                        continue
                    row = {"id": f"{module}:{package}::{name}", "runner": "go", "module": module,
                           "package": package, "name": name, "samples_ms": [], "coverage": [],
                           "outcomes": [], "protected": True,
                           "protection_reason": "existing contract/security/regression suite",
                           "defects": [], "historical_failures": None}
                    for _ in range(repeats):
                        profile = Path(temporary) / "cover.out"
                        p, elapsed = run([str(binary), f"-test.run=^{name}$", "-test.count=1", "-test.v",
                                          f"-test.coverprofile={profile}"], cwd)
                        if p.returncode or f"--- PASS: {name} " not in p.stdout or "--- SKIP:" in p.stdout:
                            raise RuntimeError("Failed/skipped test cannot justify selection: " + p.stdout + p.stderr)
                        units = []
                        for line in profile.read_text().splitlines()[1:]:
                            location, statements, count = line.split()
                            if int(count) > 0 and int(statements) > 0:
                                # Module import path -> repository file, including replaced contract.
                                source, span = location.rsplit(":", 1)
                                for owner in ["oxalc88", "oxDevelop"]:
                                    source = source.removeprefix(f"github.com/{owner}/oxguard/")
                                units.append(f"{source}:go:{span}")
                        row["samples_ms"].append(elapsed)
                        row["outcomes"].append("pass")
                        row["coverage"].append(sorted(set(units)))
                    rows.append(row)
    return rows, baselines


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("--go", action="store_true", help="also collect all three Go modules")
    parser.add_argument("--distribution", action="store_true", help="also run expensive installed npm tests")
    parser.add_argument("--repeats", type=int, default=2)
    args = parser.parse_args()
    if args.repeats < 1:
        parser.error("repeats must be positive")
    p, _ = run(["git", "ls-files", "-z"])
    tracked = p.stdout.rstrip("\0").split("\0")
    digest = hashlib.sha256()
    for name in sorted(tracked):
        digest.update(name.encode() + b"\0" + (ROOT / name).read_bytes())
    sha, _ = run(["git", "rev-parse", "HEAD"])
    rows, baseline = collect_node(set(tracked), args.repeats, args.distribution)
    if args.go:
        go_rows, go_baseline = collect_go(args.repeats)
        rows += go_rows
        baseline.update(go_baseline)
    report = {"schema_version": 1, "mode": "shadow", "revision": sha.stdout.strip(),
              "tracked_inputs_sha256": digest.hexdigest(),
              "environment": {"platform": platform.platform(), "python": platform.python_version(),
                              "node": run(["node", "--version"])[0].stdout.strip(),
                              "go": run(["go", "version"])[0].stdout.strip() if args.go else None},
              "baseline": baseline, "tests": sorted(rows, key=lambda r: r["id"]),
              "unavailable": ["per-test historical failures/flakiness", "exhaustive mutation matrix",
                              "branch coverage", "Python helper subprocess coverage"],
              "unmeasured_mandatory": ["Python installed behavior/parity scenarios", "Python helper tests",
                                      "identity hooks", "other OS/package-manager variants"] +
                                     ([] if args.distribution else ["npm distribution tests"])}
    write_report(args.output, report)
    print(json.dumps({"tests": len(rows), "baseline": baseline, "output": str(args.output)}))


if __name__ == "__main__":
    main()
