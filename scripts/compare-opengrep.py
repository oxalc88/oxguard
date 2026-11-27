"""Compare the pinned engines on unchanged SAST fixtures; no registry or policy changes."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import shutil
import statistics
import subprocess
import tempfile
import time

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument("baseline", type=Path, help="verified v1.23.0 Linux x64 asset")
parser.add_argument("candidate", type=Path, help="verified v2.1.0 Linux x64 asset")
args = parser.parse_args()
root = Path(__file__).resolve().parent.parent
manifest = json.loads((root / "npm/native-tools.json").read_text())
engines = [("1.23.0", args.baseline.resolve(), "1f06548af379ab6080698a609612890ffad2d92dc2172f1e97d38d48096d5ef8"),
           (manifest["version"].removeprefix("v"), args.candidate.resolve(), manifest["platforms"]["linux-x64"]["sha256"])]
for _, engine, digest in engines:
    if hashlib.sha256(engine.read_bytes()).hexdigest() != digest:
        raise SystemExit(f"Untrusted engine: {engine}")
corpus = json.loads((root / "evals/cases.json").read_text())["cases"]
results = []
selection_cases = [
    {"id": "selection.defaults", "files": {"src/index.ts": "const x = 1;", "tests/a.ts": 'eval("x");'}},
    {"id": "selection.nested", "files": {"src/index.ts": 'eval("x");', "src/.semgrepignore": "index.ts"}},
    {"id": "selection.range", "files": {"src/index.ts": 'eval("x");', ".semgrepignore": "src/[a-z]*.ts"}},
    {"id": "selection.negation", "files": {"src/index.ts": 'eval("x");', ".semgrepignore": "src/*.ts\n!src/index.ts"}},
    {"id": "selection.fnmatch", "files": {"src/index.ts": 'eval("x");', "src/deep/a.ts": 'eval("x");', ".semgrepignore": "src/*.ts"}},
    {"id": "selection.empty", "files": {"src/index.ts": 'eval("x");', "tests/a.ts": 'eval("x");', ".semgrepignore": ""}},
    {"id": "selection.include", "files": {"src/index.ts": 'eval("x");', ".semgrepignore": ":include custom.ignore", "custom.ignore": "src/index.ts"}},
    {"id": "selection.git-untracked", "git": True, "files": {"src/index.ts": 'eval("x");', ".gitignore": "src/index.ts"}},
    {"id": "selection.git-tracked", "git": True, "tracked": True, "files": {"src/index.ts": 'eval("x");', ".gitignore": "src/index.ts"}},
    {"id": "selection.custom-dir", "exclude": ["cache/private"], "files": {"src/index.ts": "const x = 1;", "src/cache/private/a.ts": 'eval("x");'}},
    {"id": "selection.root-explicit", "files": {"src/index.ts": "const x = 1;", "a.ts": 'eval("x");', ".semgrepignore": "*.ts"}},
    {"id": "selection.literal", "files": {"src/index.ts": 'eval("x");', "src/a b.ts": 'eval("x");', ".semgrepignore": "src/a b.ts"}},
]

with tempfile.TemporaryDirectory(prefix="oxguard-opengrep-comparison-") as temporary:
    helper = Path(temporary) / "selector.test"
    subprocess.run(["go", "test", "-c", "-o", str(helper)], cwd=root / "tsguard", check=True)
    for case in [next(c for c in corpus if c["id"] == i) for i in ["security.clean", "security.multiple", "security.dependency_failure_still_sast"]] + selection_cases:
        case_id = case["id"]
        project = Path(temporary) / case_id
        shutil.copytree(root / "evals/fixtures/security-contract", project)
        for name, content in case.get("files", {}).items():
            target = project / name
            target.parent.mkdir(parents=True, exist_ok=True)
            target.write_text(content)
        if case.get("git"):
            subprocess.run(["git", "init", "-q"], cwd=project, check=True)
            if case.get("tracked"):
                subprocess.run(["git", "add", "-f", "src/index.ts"], cwd=project, check=True)
        rules = project / "node_modules/.cache/oxguard/rules"
        rules.mkdir(parents=True)
        shutil.copyfile(project / "rule.yaml", rules / "rule.yaml")
        targets = ["./" + p.name for p in sorted(project.iterdir()) if p.name not in ["node_modules", ".git"]]
        args_file = Path(temporary) / "args.json"
        subprocess.run([str(helper), "-test.run=^TestSecurityIgnoreComparisonHelper$"], check=True, capture_output=True,
                       env={**os.environ, "OXGUARD_COMPARE_ROOT": str(project), "OXGUARD_COMPARE_ARGS": str(args_file)})
        compatibility = json.loads(args_file.read_text())
        case_results = []
        for version, engine, _ in engines:
            runs = []
            for iteration in range(6 if case_id.startswith("security.") else 1):
                start = time.perf_counter()
                process = subprocess.run([str(engine), "scan", "--config", "node_modules/.cache/oxguard/rules", "--error", "--quiet", "--json",
                                          *[item for d in ["node_modules", "dist", ".next", "build", "coverage"] + case.get("exclude", []) for item in ["--exclude", ("**/" + d if version != "1.23.0" else d)]],
                                          *(compatibility if version != "1.23.0" else []), *targets],
                                         cwd=project, capture_output=True, timeout=60,
                                         env={**os.environ, "OPENGREP_ENABLE_VERSION_CHECK": "false"})
                elapsed = (time.perf_counter() - start) * 1000
                report = json.loads(process.stdout)
                if iteration or not case_id.startswith("security."):
                    runs.append(round(elapsed, 3))
            case_results.append({"case": case_id, "version": version, "exit": process.returncode, "runs_ms": runs,
                            "median_ms": round(statistics.median(runs), 3), "errors": report["errors"],
                            "scanned": sorted(report["paths"]["scanned"]),
                            "findings": [{"rule": f["check_id"], "file": f["path"], "line": f["start"]["line"], "column": f["start"]["col"]} for f in report["results"]]})
        results.extend(case_results)
        for key in ["exit", "findings", "scanned"]:
            if case_results[0][key] != case_results[1][key]:
                raise SystemExit(f"Parity failure: {case_id} {key}: {case_results}")
print(json.dumps(results, indent=2))
