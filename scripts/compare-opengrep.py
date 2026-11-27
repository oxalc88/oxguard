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
with tempfile.TemporaryDirectory(prefix="oxguard-opengrep-comparison-") as temporary:
    for case_id in ["security.clean", "security.multiple", "security.dependency_failure_still_sast"]:
        case = next(c for c in corpus if c["id"] == case_id)
        project = Path(temporary) / case_id
        shutil.copytree(root / "evals/fixtures/security-contract", project)
        for name, content in case.get("files", {}).items():
            target = project / name
            target.parent.mkdir(parents=True, exist_ok=True)
            target.write_text(content)
        rules = project / "node_modules/.cache/oxguard/rules"
        rules.mkdir(parents=True)
        shutil.copyfile(project / "rule.yaml", rules / "rule.yaml")
        for version, engine, _ in engines:
            runs = []
            for iteration in range(6):
                start = time.perf_counter()
                process = subprocess.run([str(engine), "scan", "--config", "node_modules/.cache/oxguard/rules", "--error", "--quiet", "--json",
                                          *["./" + p.name for p in sorted(project.iterdir()) if p.name not in ["node_modules", ".git"]]],
                                         cwd=project, capture_output=True, timeout=60,
                                         env={**os.environ, "OPENGREP_ENABLE_VERSION_CHECK": "false"})
                elapsed = (time.perf_counter() - start) * 1000
                report = json.loads(process.stdout)
                if iteration:
                    runs.append(round(elapsed, 3))
            results.append({"case": case_id, "version": version, "exit": process.returncode, "runs_ms": runs,
                            "median_ms": round(statistics.median(runs), 3), "errors": report["errors"],
                            "findings": [{"rule": f["check_id"], "file": f["path"], "line": f["start"]["line"], "column": f["start"]["col"]} for f in report["results"]]})
print(json.dumps(results, indent=2))
