"""Compare Opengrep 1.23.0 with the 2.1.0 OxGuard integration; no registry, rule or policy changes.

Baseline: the pre-migration invocation (1.23.0, tsguard exclusions as --exclude, root entries as targets).
Candidate: 2.1.0 with the production argument builder (tsguard/security_scope.go) reached through a compiled Go test helper.

Cases in group "parity" must select the same files and report the same findings, diagnostics and exit code.
Cases in group "matrix" describe projects that use .semgrepignore or other deliberate differences. They are not forced to
match; each records the expected classification (identical / more scanned / less scanned / different) and the script fails
if the pinned engines stop behaving that way, so the documented compatibility matrix is executable evidence.

  python3 scripts/compare-opengrep.py /abs/v1.23.0/opengrep /abs/v2.1.0/opengrep [--markdown] [--large 50000]
"""
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

parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
parser.add_argument("baseline", type=Path, help="verified v1.23.0 Linux x64 asset")
parser.add_argument("candidate", type=Path, help="verified v2.1.0 Linux x64 asset")
parser.add_argument("--markdown", action="store_true", help="print the compatibility matrix as Markdown")
parser.add_argument("--large", type=int, default=0, metavar="FILES", help="also time a generated tree of this many files")
parser.add_argument("--runs", type=int, default=5, help="timed runs per engine and timing case")
parser.add_argument("--tmpdir", type=Path, default=None, help="disk-backed directory for generated projects")
args = parser.parse_args()
root = Path(__file__).resolve().parent.parent
manifest = json.loads((root / "npm/native-tools.json").read_text())
BASELINE_SHA256 = "1f06548af379ab6080698a609612890ffad2d92dc2172f1e97d38d48096d5ef8"
engines = [("1.23.0", args.baseline.resolve(), BASELINE_SHA256),
           (manifest["version"].removeprefix("v"), args.candidate.resolve(), manifest["platforms"]["linux-x64"]["sha256"])]
for _, engine, digest in engines:
    if hashlib.sha256(engine.read_bytes()).hexdigest() != digest:
        raise SystemExit(f"Untrusted engine: {engine}")
corpus = {c["id"]: c for c in json.loads((root / "evals/cases.json").read_text())["cases"]}
DEFAULT_EXCLUDES = ["node_modules", ".git", "dist", ".next", "build", "coverage", ".agents", ".claude", ".opencode", ".kiro", "skills"]
ENV = {**os.environ, "OPENGREP_ENABLE_VERSION_CHECK": "false"}
EVAL = 'eval("x");\n'

# One file set shared by the .semgrepignore matrix. Every file holds one eval() so selected files and findings coincide.
TREE = ["a.ts", "src/a.ts", "src/b.ts", "src/deep/c.ts", "src/a.test.ts", "src/fixtures/f.ts", "src/tests/t.ts", "src/gen/g.ts",
        "tests/a.ts", "test/a.ts", "scripts/s.ts", "docs/d.ts", "cache/private/p.ts", "lib/l.ts", "lib/src/y.ts"]
# Non-source artifacts from the 1.23.0 default list: OxGuard's baseline layer excludes them whatever the project ignore file says.
ARTIFACTS = {"src/x.min.js": EVAL, "vendor/v.ts": EVAL, "build/b.ts": EVAL, "src/vendor/v.ts": EVAL, ".venv/v.ts": EVAL,
             ".tox/t.ts": EVAL, ".env/e.ts": EVAL, "_build/b.ts": EVAL, "node_modules/m/i.ts": EVAL, "dist/d.ts": EVAL}


def tree(extra=None, drop=()):
    files = {name: EVAL for name in TREE if name not in drop}
    files.update(extra or {})
    return files


def case(cid, group, files, expect="identical", note="", **options):
    return {"id": cid, "group": group, "files": files, "expect": expect, "note": note, **options}


CASES = [
    case("fixture.security.clean", "parity", {}, fixture="security.clean", timed=True),
    case("fixture.security.multiple", "parity", {}, fixture="security.multiple", timed=True),
    case("fixture.security.dependency_failure_still_sast", "parity", {}, fixture="security.dependency_failure_still_sast", timed=True),
    # Default scope: no .semgrepignore anywhere. Nothing new may be scanned.
    case("default.scope", "parity", tree({**ARTIFACTS, "testsuite/ts.ts": EVAL, "src/build/b.ts": EVAL}),
         must_skip=["tests/a.ts", "test/a.ts", "testsuite/ts.ts", "src/tests/t.ts", "src/x.min.js", "vendor/v.ts"], must_scan=["a.ts", "src/a.ts"]),
    case("default.git", "parity", tree(ARTIFACTS), git=True, must_skip=["tests/a.ts", "src/x.min.js"]),
    case("default.git-gitignored-untracked", "parity", tree({".gitignore": "src/gen/\nlib/l.ts\n"}), git=True),
    case("default.git-gitignored-tracked", "parity", tree({".gitignore": "src/gen/\n"}), git=True, track=["src/gen/g.ts"]),
    case("default.custom-exclude-any-depth", "parity", tree(), exclude=["cache/private", "generated", "*.snap.ts"]),
    case("default.symlinks", "parity", tree(), symlinks=True),
    # Root entries are explicit targets: 1.23.0 exempted root-level files from every exclusion; OxGuard applies them.
    case("exclude.root-level-files", "matrix", tree({"x.min.js": EVAL}), expect="less scanned", must_skip=["x.min.js"],
         note="a root-level minified file is no longer scanned (1.23.0 scanned explicit root files regardless of exclusions)"),
    case("exclude.anchored-root-file", "matrix", tree(), exclude=["/a.ts"], expect="different", must_skip=["a.ts"], must_scan=["src/a.ts"],
         note="`/a.ts` excludes the root file only; 1.23.0 excluded src/a.ts and still scanned the explicit root file"),
    case("exclude.root-level-files-with-ignore", "matrix", tree({"x.min.js": EVAL, ".semgrepignore": "docs/\n"}), expect="less scanned", must_skip=["x.min.js"]),
    # Layering: a project file replaces the test defaults only; the baseline artifacts stay excluded.
    case("layer.docs-only-ignore", "matrix", tree({".semgrepignore": "docs/\n", **ARTIFACTS}), expect="less scanned",
         must_scan=["tests/a.ts", "test/a.ts", "src/tests/t.ts"], must_skip=["docs/d.ts", "src/x.min.js", "vendor/v.ts", "src/vendor/v.ts", ".venv/v.ts", "node_modules/m/i.ts"],
         note="tests are scanned as in 1.23.0; minified/vendored/build artifacts stay excluded (1.23.0 scanned them once a project file existed)"),
    case("layer.empty-ignore", "matrix", tree({".semgrepignore": "", **ARTIFACTS}), expect="less scanned",
         must_scan=["tests/a.ts", "src/tests/t.ts"], must_skip=["src/x.min.js", "vendor/v.ts"],
         note="an empty project file still releases the test defaults but not the artifacts"),
    case("layer.user-exclude-with-ignore", "matrix", tree({".semgrepignore": "docs/\n"}), exclude=["scripts"], expect="identical",
         must_skip=["scripts/s.ts", "docs/d.ts"], note="tsguard --exclude/config entries apply on top of any project file"),
    case("layer.ignore-cannot-reinclude", "matrix", tree({".semgrepignore": "!vendor/\n!*.min.js\n!/tests/\n", **ARTIFACTS}), expect="less scanned",
         must_skip=["vendor/v.ts", "src/x.min.js"], must_scan=["tests/a.ts"],
         note="negations cannot re-include the baseline layer"),
    # Deliberate differences outside .semgrepignore.
    case("exclude.anchored-root", "matrix", tree(), exclude=["/src"], expect="more scanned", must_skip=["src/a.ts"], must_scan=["lib/src/y.ts"],
         note="`/src` excludes only the scan root's src; 1.23.0 also excluded lib/src"),
    case("exclude.anchored-root-git-subproject", "matrix", tree(), exclude=["/src"], expect="more scanned", git_subdir="packages/app", must_skip=["src/a.ts"], must_scan=["lib/src/y.ts"],
         note="anchored entries are translated to the Git-relative path"),
    case("exclude.unreadable-excluded-dir", "matrix", tree({"secret/inner/s.ts": EVAL}), exclude=["/secret"], unreadable=["secret"],
         expect="different", note="1.23.0 refuses to start on an unreadable root entry; 2.1.0 skips the excluded directory"),
    # a. a root .semgrepignore replaces the defaults (both engines).
    case("ignore.root-replaces-defaults", "matrix", tree({".semgrepignore": "scripts/\n"}), expect="identical", must_scan=["tests/a.ts"],
         note="the test defaults are replaced by the project file in both engines"),
    case("ignore.root-empty", "matrix", tree({".semgrepignore": ""}), expect="identical", note="an empty root file also replaces the defaults"),
    case("ignore.root-comments-only", "matrix", tree({".semgrepignore": "# nothing\n\n   \n"}), expect="identical"),
    case("ignore.root-git", "matrix", tree({".semgrepignore": "scripts/\n"}), git=True, expect="identical"),
    case("ignore.git-subproject-root-file-at-git-root", "matrix", tree({"../../.semgrepignore": "scripts/\n"}), git_subdir="packages/app",
         expect="different", note="2.1.0 reads the default-replacing file at the Git root; 1.23.0 read the scan directory"),
    case("ignore.git-subproject-file-in-scan-dir", "matrix", tree({".semgrepignore": "scripts/\n"}), git_subdir="packages/app",
         expect="less scanned", note="same cause: the scan-directory file no longer replaces the defaults"),
    # b. :include
    case("include.gitignore", "matrix", tree({".semgrepignore": ":include .gitignore\n", ".gitignore": "scripts/\ndocs/\n"}), expect="identical"),
    case("include.gitignore-git", "matrix", tree({".semgrepignore": ":include .gitignore\n", ".gitignore": "scripts/\ndocs/\n"}), git=True, expect="identical"),
    case("include.custom-file", "matrix", tree({".semgrepignore": ":include custom.ignore\n", "custom.ignore": "scripts/\n"}), expect="identical"),
    case("include.directory", "matrix", tree({".semgrepignore": ":include docs\nscripts/\n"}), expect="identical"),
    case("include.unknown-directive", "matrix", tree({".semgrepignore": ":unknown x\nscripts/\n"}), expect="different"),
    case("include.missing-file", "matrix", tree({".semgrepignore": ":include nope.ignore\n"}), expect="identical"),
    # c. typical root ignore files
    case("ignore.scripts-docs", "matrix", tree({".semgrepignore": "scripts/\ndocs/\n"}), expect="identical"),
    case("ignore.scripts-docs-no-slash", "matrix", tree({".semgrepignore": "scripts\ndocs\n"}), expect="identical"),
    case("ignore.test-suffix", "matrix", tree({".semgrepignore": "*.test.ts\n"}), expect="identical"),
    case("ignore.fixtures-globstar", "matrix", tree({".semgrepignore": "**/fixtures/\n"}), expect="identical"),
    case("ignore.anchored-build", "matrix", tree({".semgrepignore": "/build\n"}), expect="identical"),
    case("ignore.anchored-vs-unanchored", "matrix", tree({".semgrepignore": "/b.ts\nc.ts\n"}), expect="identical"),
    case("ignore.inner-slash-anchors", "matrix", tree({".semgrepignore": "src/gen\n"}), expect="identical"),
    case("ignore.comments-and-blank-lines", "matrix", tree({".semgrepignore": "# docs\n\ndocs/\n   \nscripts/\n"}), expect="identical"),
    case("ignore.inline-comment", "matrix", tree({".semgrepignore": "docs/ # trailing comment\nscripts/\n"}), expect="more scanned",
         must_scan=["docs/d.ts"], note="gitignore has no inline comments: 1.23.0 stripped ` # ...`, 2.1.0 reads a literal pattern"),
    case("ignore.star-crosses-slash", "matrix", tree({".semgrepignore": "src/*.ts\n"}), expect="more scanned",
         note="1.23.0 fnmatch `*` crossed `/` (hid src/deep/c.ts); gitignore `*` does not"),
    # d. nested
    case("ignore.nested-src", "matrix", tree({"src/.semgrepignore": "a.ts\n"}), expect="less scanned",
         note="a nested file is now honored; 1.23.0 read only the cwd file"),
    case("ignore.nested-src-with-root", "matrix", tree({".semgrepignore": "docs/\n", "src/.semgrepignore": "a.ts\n"}), expect="less scanned"),
    # e. negation, ranges, anchoring, whitespace
    case("ignore.negation", "matrix", tree({".semgrepignore": "src/*.ts\n!src/b.ts\n"}), expect="more scanned",
         note="negation was silently dropped in 1.23.0 and is honored now"),
    case("ignore.range", "matrix", tree({".semgrepignore": "src/[a-b].ts\n"}), expect="less scanned",
         note="character ranges were silently dropped in 1.23.0"),
    case("ignore.double-star-middle", "matrix", tree({".semgrepignore": "src/**/c.ts\n"}), expect="identical"),
    case("ignore.trailing-spaces", "matrix", tree({".semgrepignore": "src/a.ts   \n"}), expect="identical"),
    case("ignore.crlf", "matrix", tree({".semgrepignore": "src/a.ts\r\nscripts/\r\n"}), expect="identical"),
    case("ignore.escaped-trailing-space", "matrix", tree({"src/e ": EVAL, ".semgrepignore": "src/e\\ \n"}), expect="identical"),
    case("ignore.unicode-name", "matrix", tree({"src/é.ts": EVAL, ".semgrepignore": "src/é.ts\n"}), expect="identical"),
]


def build_project(base, spec):
    project = base / spec["id"]
    scan = project
    if spec.get("fixture"):
        shutil.copytree(root / "evals/fixtures/security-contract", project)
        overrides = corpus[spec["fixture"]].get("files", {})
    else:
        project.mkdir(parents=True)
        overrides = {}
    if spec.get("git_subdir"):
        scan = project / spec["git_subdir"]
        scan.mkdir(parents=True)
    for name, content in {**spec["files"], **overrides}.items():
        target = scan / name
        target.parent.mkdir(parents=True, exist_ok=True)
        target.write_bytes(content.encode())
    rules = scan / "node_modules/.cache/oxguard/rules"
    rules.mkdir(parents=True)
    rule_source = root / "evals/fixtures/security-contract/rule.yaml"
    shutil.copyfile(rule_source, rules / "rule.yaml")
    if spec.get("git") or spec.get("git_subdir"):
        subprocess.run(["git", "init", "-q"], cwd=project, check=True)
        if spec.get("track"):
            subprocess.run(["git", "add", "-f", *spec["track"]], cwd=scan, check=True)
    if spec.get("symlinks"):
        outside = base / (spec["id"] + "-outside")
        outside.mkdir()
        (outside / "o.ts").write_text(EVAL)
        for link, destination in (("src/escape", outside), ("linkroot", outside), ("src/linkfile.ts", outside / "o.ts")):
            os.symlink(destination, scan / link)
    return project, scan


def baseline_targets(scan):
    # The pre-migration invocation: every root entry except node_modules/.git/symlinks.
    return ["./" + p.name for p in sorted(scan.iterdir()) if p.name not in ("node_modules", ".git") and not p.is_symlink()]


def run_engine(engine, scan, extra, targets):
    started = time.perf_counter()
    process = subprocess.run([str(engine), "scan", "--config", "node_modules/.cache/oxguard/rules", "--error", "--quiet", "--json", *extra, *targets],
                             cwd=scan, capture_output=True, timeout=1800, env=ENV)
    return process, (time.perf_counter() - started) * 1000


def summarize(process):
    try:
        report = json.loads(process.stdout)
    except ValueError:
        return {"exit": process.returncode, "scanned": [], "findings": [], "errors": ["unparseable output: " + process.stderr.decode(errors="replace").strip()[:120]]}
    return {"exit": process.returncode, "scanned": sorted(report["paths"]["scanned"]),
            "findings": [[f["check_id"], f["path"], f["start"]["line"], f["start"]["col"]] for f in report["results"]],
            "errors": sorted((e.get("type") or e.get("message", ""))[:80] for e in report["errors"])}


def classify(old, new):
    if old == new:
        return "identical"
    a, b = set(old["scanned"]), set(new["scanned"])
    if old["exit"] != new["exit"] and old["exit"] == 2:
        return "different"
    if a == b and old["findings"] == new["findings"]:
        return "different"  # same files; only diagnostics/exit differ
    if a < b:
        return "more scanned"
    if b < a:
        return "less scanned"
    return "different"


def candidate_args(helper, scan, spec, temporary):
    out = temporary / "args.json"
    env = {**os.environ, "OXGUARD_COMPARE_ROOT": str(scan), "OXGUARD_COMPARE_ARGS": str(out),
           "OXGUARD_COMPARE_EXCLUDE": json.dumps(spec.get("exclude", []))}
    started = time.perf_counter()
    subprocess.run([str(helper), "-test.run=^TestSecurityScopeComparisonHelper$"], check=True, capture_output=True, env=env)
    elapsed = (time.perf_counter() - started) * 1000
    return json.loads(out.read_text()), elapsed


def baseline_args(spec):
    return [item for d in DEFAULT_EXCLUDES + spec.get("exclude", []) for item in ("--exclude", d)]


results, failures = [], []
temporary_root = args.tmpdir
if temporary_root:
    temporary_root.mkdir(parents=True, exist_ok=True)
with tempfile.TemporaryDirectory(prefix="oxguard-opengrep-comparison-", dir=temporary_root) as name:
    temporary = Path(name)
    helper = temporary / "scope.test"
    subprocess.run(["go", "test", "-c", "-o", str(helper)], cwd=root / "tsguard", check=True)
    for spec in CASES:
        project, scan = build_project(temporary, spec)
        locked = [scan / p for p in spec.get("unreadable", [])]
        for path in locked:
            path.chmod(0)
        try:
            new_args, enumeration_ms = candidate_args(helper, scan, spec, temporary)
            old_args = baseline_args(spec) + baseline_targets(scan)
            old_process, _ = run_engine(engines[0][1], scan, [], old_args)
            new_process, _ = run_engine(engines[1][1], scan, new_args, [])
            old, new = summarize(old_process), summarize(new_process)
            row = {"case": spec["id"], "group": spec["group"], "baseline": old, "candidate": new, "classification": classify(old, new),
                   "expected": spec["expect"], "note": spec["note"]}
            if spec.get("timed"):
                runs = {name_: [] for name_, *_ in engines}
                for iteration in range(args.runs + 1):
                    _, old_ms = run_engine(engines[0][1], scan, [], old_args)
                    _, helper_ms = candidate_args(helper, scan, spec, temporary)
                    _, new_ms = run_engine(engines[1][1], scan, new_args, [])
                    if iteration:  # the first run warms caches and is discarded
                        runs[engines[0][0]].append(round(old_ms, 1))
                        runs[engines[1][0]].append(round(new_ms + helper_ms, 1))
                row["runs_ms"] = runs
                row["median_ms"] = {k: round(statistics.median(v), 1) for k, v in runs.items()}
            results.append(row)
            for path in spec.get("must_scan", []):
                if path not in new["scanned"]:
                    failures.append(f"{spec['id']}: {path} must be scanned by 2.1.0 but was not")
            for path in spec.get("must_skip", []):
                if path in new["scanned"]:
                    failures.append(f"{spec['id']}: {path} must be skipped by 2.1.0 but was scanned")
            if row["classification"] != spec["expect"]:
                failures.append(f"{spec['id']}: expected {spec['expect']}, measured {row['classification']}\n  1.23.0 {old}\n  2.1.0  {new}")
            if spec["group"] == "parity" and old != new:
                failures.append(f"{spec['id']}: parity failure\n  1.23.0 {old}\n  2.1.0  {new}")
        finally:
            for path in locked:
                path.chmod(0o755)
    large = None
    if args.large:
        # A generated tree: nested source directories plus excluded dependency and test directories.
        base = temporary / "large"
        per_dir = 100
        for d in range(args.large // per_dir):
            folder = base / "src" / f"m{d // 20}" / f"d{d}"
            folder.mkdir(parents=True)
            for f in range(per_dir):
                (folder / f"f{f}.ts").write_text(f"export const v{f} = {f};\n" + (EVAL if f == 0 and d % 50 == 0 else ""))
        (base / "tests").mkdir()
        (base / "tests/t.ts").write_text(EVAL)
        (base / "node_modules").mkdir()
        (base / "node_modules/.cache/oxguard/rules").mkdir(parents=True)
        shutil.copyfile(root / "evals/fixtures/security-contract/rule.yaml", base / "node_modules/.cache/oxguard/rules/rule.yaml")
        spec = {"id": "large", "exclude": []}
        runs = {name_: [] for name_, *_ in engines}
        summaries = {}
        for iteration in range(max(3, args.runs // 2) + 1):
            new_args, helper_ms = candidate_args(helper, base, spec, temporary)
            old_process, old_ms = run_engine(engines[0][1], base, [], baseline_args(spec) + baseline_targets(base))
            new_process, new_ms = run_engine(engines[1][1], base, new_args, [])
            summaries = {engines[0][0]: summarize(old_process), engines[1][0]: summarize(new_process)}
            if iteration:
                runs[engines[0][0]].append(round(old_ms, 1))
                runs[engines[1][0]].append(round(new_ms + helper_ms, 1))
        old, new = summaries[engines[0][0]], summaries[engines[1][0]]
        large = {"files": args.large, "runs_ms": runs, "median_ms": {k: round(statistics.median(v), 1) for k, v in runs.items()},
                 "enumeration_ms_last": round(helper_ms, 1), "scanned": {"1.23.0": len(old["scanned"]), "2.1.0": len(new["scanned"])},
                 "findings": {"1.23.0": len(old["findings"]), "2.1.0": len(new["findings"])},
                 "parity": old["scanned"] == new["scanned"] and old["findings"] == new["findings"] and old["exit"] == new["exit"]}
        if not large["parity"]:
            failures.append(f"large tree differs: {large}")

if args.markdown:
    print("| Case | 1.23.0 selected | 2.1.0 selected | Exit 1.23.0 / 2.1.0 | Result |")
    print("|---|---|---|---|---|")
    for row in results:
        old, new = row["baseline"], row["candidate"]
        only_old, only_new = sorted(set(old["scanned"]) - set(new["scanned"])), sorted(set(new["scanned"]) - set(old["scanned"]))
        delta = "" if not (only_old or only_new) else " (" + "; ".join(filter(None, [("only 1.23.0: " + ", ".join(only_old)) if only_old else "", ("only 2.1.0: " + ", ".join(only_new)) if only_new else ""])) + ")"
        print(f"| {row['case']} | {len(old['scanned'])} | {len(new['scanned'])}{delta} | {old['exit']} / {new['exit']} | {row['classification']} |")
else:
    print(json.dumps({"results": results, "large": large}, indent=2))
if failures:
    raise SystemExit("Comparison failures:\n" + "\n".join(failures))
