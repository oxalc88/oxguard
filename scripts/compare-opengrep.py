"""Compare Opengrep 1.23.0 with the 2.1.0 OxGuard integration; no registry, rule or policy changes.

Baseline: the pre-migration invocation (1.23.0, tsguard exclusions as --exclude, root entries as targets).
Candidate: 2.1.0 with the production argument builder (tsguard/security_scope.go) reached through a compiled Go test helper.

Cases in group "parity" must select the same files and report the same findings, diagnostics and exit code.
Cases in group "matrix" describe projects that use .semgrepignore or other deliberate differences. They are not forced to
match; each records the expected classification (identical / more scanned / less scanned / different / approved-reduction) and
the script fails if the pinned engines stop behaving that way, so the documented compatibility matrix is executable evidence.
"approved-reduction" is the owner-approved exception (minified files, root included, are never scanned): the candidate must equal
the baseline minus the listed paths, and nothing else.

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
parser.add_argument("baseline", type=Path, nargs="?", help="verified v1.23.0 Linux x64 asset")
parser.add_argument("candidate", type=Path, nargs="?", help="verified v2.1.0 Linux x64 asset")
parser.add_argument("--self-test", action="store_true", help="only run the classifier self-test (no engines needed)")
parser.add_argument("--update-doc", type=Path, metavar="MARKDOWN", help="rewrite the generated matrix block between the matrix markers of this file")
parser.add_argument("--markdown", action="store_true", help="print the compatibility matrix as Markdown")
parser.add_argument("--large", type=int, default=0, metavar="FILES", help="also time a generated tree of this many files")
parser.add_argument("--runs", type=int, default=5, help="timed runs per engine and timing case")
parser.add_argument("--tmpdir", type=Path, default=None, help="disk-backed directory for generated projects")
args = parser.parse_args()
root = Path(__file__).resolve().parent.parent
if not args.self_test and not (args.baseline and args.candidate):
    parser.error("the baseline and candidate engines are required unless --self-test is given")
manifest = json.loads((root / "npm/native-tools.json").read_text())
BASELINE_SHA256 = "1f06548af379ab6080698a609612890ffad2d92dc2172f1e97d38d48096d5ef8"
engines = []
if not args.self_test:
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

EXCLUDE_TREE = {name: EVAL for name in ["a.ts", "x.ts", "src/a.ts", "src/x.ts", "lib/src/y.ts", "lib/a.ts", "x/f.ts", "src/x/f.ts", "a/b/c.ts", "src/a/b/c.ts",
                                        "lib/a/b/c.ts", "xx/f.ts", "src/xx/f.ts", "deep/er/x/g.ts", "x2/x/h.ts", "keep/s.snap.ts", "src/a/b/b.ts", "a/c/b.ts", "b/a.ts"]}

# Scan root of a monorepo subproject whose only ignore file is its own: nested test paths must be restored.
NESTED = {name: EVAL for name in ["src/a.ts", "tests/t0.ts", "src/tests/t.ts", "src/tests/deep/tests/n.ts", "src/test/u.ts", "src/testsuite/s.ts", "lib/pkg/testsuite/s.ts",
                                  "lib/pkg/test/u.ts", "src/a_test.go", "src/tests/c_test.go", "lib/pkg/b_test.go", "root_test.go", "docs/d.ts", "src/contest/x.ts"]}
NESTED[".semgrepignore"] = "docs/\n"
NESTED_CHECKS = {"must_scan": ["src/tests/t.ts", "src/tests/deep/tests/n.ts", "src/test/u.ts", "src/testsuite/s.ts", "lib/pkg/testsuite/s.ts", "src/a_test.go", "src/tests/c_test.go",
                               "lib/pkg/b_test.go", "root_test.go", "src/contest/x.ts"], "must_skip": ["docs/d.ts"]}


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
    # Root-level files are explicit targets: Opengrep exempts them from exclusions and ignore files in both engines, so OxGuard does not
    # pass --force-exclude (it would also drop root files named build, skills, .claude, CVS, ...). The one exception is APPROVED BY THE
    # OWNER (round-3 review P1): minified files are never scanned, root included, so root *.min.js files are not targets. The label
    # "approved-reduction" is distinct from "less scanned" and drift in either direction fails the run.
    case("approved.root-minified", "matrix", tree({"x.min.js": EVAL}), expect="approved-reduction", approved_paths=["x.min.js"], must_skip=["x.min.js"],
         note="owner-approved: 1.23.0 scanned an explicit root x.min.js; minified files are never scanned"),
    case("approved.root-minified-only-finding", "matrix", {"x.min.js": EVAL, "src/ok.ts": "export const a = 1;\n"}, expect="approved-reduction",
         approved_paths=["x.min.js"], must_skip=["x.min.js"], note="1.23.0 exits 1 on the minified file alone; the candidate exits 0"),
    case("approved.root-minified-with-ignore", "matrix", tree({"x.min.js": EVAL, ".semgrepignore": "docs/\n"}), expect="approved-reduction",
         approved_paths=["x.min.js", "src/x.min.js", "vendor/v.ts", ".venv/v.ts"], must_skip=["x.min.js"]),
    # No .semgrepignore: every root-level entry type, enumerated with a language-agnostic rule so extension-less files are analyzed.
    case("default.root-level-entries", "matrix", {name: EVAL for name in ["build", "dist", "vendor", "CVS", ".env", "skills", "testsuite", "test", "tests", "coverage", ".next",
                                                                          "foo_test.go", "x.min.js", "a.min.js", "README.md", "Makefile", "a.ts", ".hg", ".svn", "_darcs", ".venv",
                                                                          ".tox", ".npm", ".yarn", "_opam", "_build", "_cargo", ".agents", ".claude", ".opencode", ".kiro"]},
         rule="generic", expect="approved-reduction", approved_paths=["x.min.js", "a.min.js"], must_scan=["build", "dist", "skills", "CVS", ".claude", "foo_test.go", "a.ts"],
         must_skip=["x.min.js", "a.min.js"],
         note="root files named like directory entries stay scanned exactly as in 1.23.0; only minified files differ"),
    case("default.root-level-entries-user-exclude", "matrix", {"a.ts": EVAL, "generated.ts": EVAL, "src/b.ts": EVAL}, exclude=["generated.ts", "/a.ts"], expect="identical",
         must_scan=["a.ts", "generated.ts"], note="explicit root files ignore exclusions in both engines (root files cannot be excluded)"),
    case("approved.root-minified-alone", "matrix", {"x.min.js": EVAL}, expect="approved-reduction", approved_paths=["x.min.js"],
         note="only a root minified file: no Opengrep run, a clean complete result (1.23.0 exits 1)"),
    # tsguard exclusions: the effective file set of main + 1.23.0 for every form. 1.23.0 turned each entry into "**/E"; leading and
    # trailing slashes never anchored anything and root-level files were never excludable.
    *[case("exclude.map." + name, "parity", dict(EXCLUDE_TREE), exclude=[entry], note=f"`{entry}` excludes the same files in both engines")
      for name, entry in [("x", "x"), ("slash-x", "/x"), ("x-slash", "x/"), ("slash-x-slash", "/x/"), ("a-b", "a/b"), ("slash-a-b", "/a/b"), ("slash-a-b-slash", "/a/b/"),
                          ("slash-src", "/src"), ("src-a-ts", "src/a.ts"), ("slash-a-ts", "/a.ts"), ("slash-lib-src", "/lib/src"), ("glob", "*.snap.ts"),
                          ("slash-glob", "/*.ts"), ("bang", "!a.ts"), ("dot-slash", "./x"), ("double-star", "**/x"), ("children", "x/*"),
                          ("double-slash", "a//b"), ("slash-double-slash", "/a//b/"), ("triple-slash", "a///b"), ("dot-a", "./a"), ("a-dot-b", "a/./b"),
                          ("trailing-double", "a//b//"), ("leading-double", "//a//b")]],
    *[case("approved.empty-exclusion." + name, "matrix", dict(EXCLUDE_TREE), exclude=[entry], expect="approved-rejection", label_check="approved-rejection",
          note=f"owner-approved hardening: `{entry}` in oxguard.toml excluded every file below the root entries under 1.23.0 (explicit root files stayed scanned); it is rejected with an actionable error")
      for name, entry in [("empty", ""), ("slash", "/"), ("double-slash", "//")]],
    case("exclude.map.git-subproject-slash-src", "parity", dict(EXCLUDE_TREE), exclude=["/src", "/x"], git_subdir="packages/app",
         note="the same mapping below a Git root: no Git-relative translation"),
    case("exclude.map.root-file-not-excludable", "parity", dict(EXCLUDE_TREE), exclude=["/a.ts", "a.ts"], must_scan=["a.ts"],
         note="root-level explicit files were never excludable in 1.23.0 and still are not"),
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
    # OWNER-APPROVED difference: an unreadable EXCLUDED directory. 1.23.0 refuses to start (exit 2) because the root entry cannot be read, even
    # when it is excluded; 2.1.0 skips it. An unreadable INCLUDED directory must keep failing in both engines (rows below).
    case("approved.unreadable-excluded-dir", "matrix", tree({"secret/inner/s.ts": EVAL}), exclude=["/secret"], unreadable=["secret"], label_check="approved-tolerance",
         expect="approved-tolerance", must_skip=["secret/inner/s.ts"], note="owner-approved: 1.23.0 refuses to start on an unreadable root entry even when excluded (exit 2); 2.1.0 skips the excluded directory and scans the rest"),
    case("unreadable.included-root-entry", "matrix", tree({"secret/inner/s.ts": EVAL}), unreadable=["secret"], label_check="both-fail", expect="both-fail",
         note="an unreadable root entry that is NOT excluded keeps failing: 1.23.0 exits 2, 2.1.0 reports a scan error (OxGuard: analyzer_failure)"),
    case("unreadable.included-nested-dir", "matrix", tree({"src/locked/l.ts": EVAL}), unreadable=["src/locked"], expect="identical", must_skip=["src/locked/l.ts"],
         note="unchanged: an unreadable directory below a root entry has never failed either engine (it is skipped silently in both); only an unreadable root entry fails"),
    case("unreadable.excluded-nested-dir", "matrix", tree({"src/locked/l.ts": EVAL}), exclude=["locked"], unreadable=["src/locked"], expect="identical",
         must_skip=["src/locked/l.ts"], note="an unreadable directory below a root entry that IS excluded does not fail either engine"),
    # a. a root .semgrepignore replaces the defaults (both engines).
    case("ignore.root-replaces-defaults", "matrix", tree({".semgrepignore": "scripts/\n"}), expect="identical", must_scan=["tests/a.ts"],
         note="the test defaults are replaced by the project file in both engines"),
    case("ignore.root-empty", "matrix", tree({".semgrepignore": ""}), expect="identical", note="an empty root file also replaces the defaults"),
    case("ignore.root-comments-only", "matrix", tree({".semgrepignore": "# nothing\n\n   \n"}), expect="identical"),
    case("ignore.root-git", "matrix", tree({".semgrepignore": "scripts/\n"}), git=True, expect="identical"),
    # Monorepo: scan root packages/app below the Git root. .gitignore is honored from the Git root and from the scan directory in both engines.
    case("monorepo.no-ignore-files", "matrix", tree({".gitignore": "lib/\n", "../../.gitignore": "gen/\n"}), git_subdir="packages/app", expect="identical",
         must_skip=["tests/a.ts", "src/gen/g.ts", "lib/l.ts"], note="defaults apply; Git-root and scan-directory .gitignore honored"),
    case("monorepo.scan-root-file", "matrix", tree({".semgrepignore": "docs/\n", ".gitignore": "lib/\n", "../../.gitignore": "gen/\n"}), git_subdir="packages/app",
         expect="identical", must_scan=["tests/a.ts", "test/a.ts", "src/tests/t.ts"], must_skip=["docs/d.ts", "src/gen/g.ts", "lib/l.ts"],
         note="OxGuard restores the test paths Opengrep's built-ins would hide: root entries and nested directories (src/tests)"),
    case("monorepo.scan-root-file-and-empty-git-root-file", "matrix", tree({".semgrepignore": "docs/\n", "../../.semgrepignore": "", ".gitignore": "lib/\n", "../../.gitignore": "gen/\n"}),
         git_subdir="packages/app", expect="identical", must_scan=["tests/a.ts", "src/tests/t.ts"], must_skip=["docs/d.ts", "src/gen/g.ts", "lib/l.ts"],
         note="an empty .semgrepignore at the Git root: native replacement, no restoration needed"),
    case("monorepo.git-root-file", "matrix", tree({"../../.semgrepignore": "scripts/\n"}), git_subdir="packages/app", expect="different", must_scan=["tests/a.ts"],
         must_skip=["scripts/s.ts"], note="the Git-root file is honored (1.23.0 ignored it) and replaces the test defaults"),
    case("monorepo.both-files", "matrix", tree({".semgrepignore": "docs/\n", "../../.semgrepignore": "scripts/\n"}), git_subdir="packages/app", expect="less scanned",
         must_scan=["tests/a.ts", "src/tests/t.ts"], must_skip=["docs/d.ts", "scripts/s.ts"], note="patterns of both files apply (1.23.0 read only the scan-root file); both replace the defaults"),
    case("monorepo.git-root-equals-scan-root", "matrix", tree({".semgrepignore": "docs/\n", ".gitignore": "lib/\n"}), git=True, expect="identical", must_scan=["tests/a.ts"],
         must_skip=["docs/d.ts", "lib/l.ts"], note="unchanged when the scan root is the Git root"),
    # Nested test paths with a scan-root .semgrepignore only (language-agnostic rule so *_test.go files are analyzed).
    case("nested-tests.basic", "matrix", NESTED, git_subdir="packages/app", rule="generic", expect="identical", **NESTED_CHECKS,
         note="nested test, tests, testsuite directories and nested *_test.go files are scanned as in 1.23.0"),
    case("nested-tests.project-ignored-stays-excluded", "matrix", {**NESTED, ".semgrepignore": "docs/\nig/\n", "src/ig/tests/i.ts": EVAL, "src/ig/z_test.go": EVAL},
         git_subdir="packages/app", rule="generic", expect="identical", must_skip=["src/ig/tests/i.ts", "src/ig/z_test.go"], must_scan=["src/tests/t.ts"],
         note="a nested test directory and a *_test.go file the project .semgrepignore excludes stay excluded"),
    case("nested-tests.gitignored-stays-excluded", "matrix", {**NESTED, "../../.gitignore": "gi/\ngi_file_test.go\n", ".gitignore": "ni_test.go\nni/\n", "src/gi/tests/g.ts": EVAL,
                                                               "src/gi_file_test.go": EVAL, "src/ni/tests/n.ts": EVAL, "src/ni_test.go": EVAL},
         git_subdir="packages/app", rule="generic", expect="identical", must_skip=["src/gi/tests/g.ts", "src/gi_file_test.go", "src/ni/tests/n.ts", "src/ni_test.go"],
         must_scan=["src/tests/t.ts"], note="nested test paths ignored by the Git-root or the scan-directory .gitignore stay excluded"),
    case("nested-tests.deep", "matrix", {**NESTED, "src/a/b/c/d/tests/e/f/tests/g.ts": EVAL, "src/a/b/c/d/tests/e/f/h_test.go": EVAL, "lib/x/y/z/testsuite/q.ts": EVAL},
         git_subdir="packages/app", rule="generic", expect="identical", must_scan=["src/a/b/c/d/tests/e/f/tests/g.ts", "src/a/b/c/d/tests/e/f/h_test.go", "lib/x/y/z/testsuite/q.ts"],
         note="deeply nested directories, test directories inside test directories, nested *_test.go inside them"),
    case("nested-tests.baseline-inside-stays-excluded", "matrix", {**NESTED, "src/tests/vendor/v.ts": EVAL, "src/tests/x.min.js": EVAL, "src/tests/dist/d.ts": EVAL,
                                                                    "src/tests/node_modules/n.ts": EVAL, "src/tests/build/b.ts": EVAL},
         git_subdir="packages/app", rule="generic", expect="approved-reduction", approved_paths=["src/tests/vendor/v.ts", "src/tests/x.min.js", "src/tests/build/b.ts"],
         must_skip=["src/tests/vendor/v.ts", "src/tests/x.min.js", "src/tests/dist/d.ts", "src/tests/node_modules/n.ts", "src/tests/build/b.ts"], must_scan=["src/tests/t.ts"],
         note="the baseline layer applies inside nested tests (1.23.0 scanned vendor, build and minified files once a project file existed)"),
    case("nested-tests.no-duplicate-reports", "matrix", {**NESTED, "src/tests/tests/inner.ts": EVAL, "src/tests/tests/inner_test.go": EVAL}, git_subdir="packages/app", rule="generic",
         expect="identical", must_scan=["src/tests/tests/inner.ts", "src/tests/tests/inner_test.go"], unique_findings=True,
         note="overlapping candidates are reported exactly once: Opengrep skips test directories nested in a target"),
    # Adversarial names and hostile layouts for the explicit-target restoration (scan-root .semgrepignore only, Git subproject).
    case("nested-tests.forged-path-newline", "matrix", {**NESTED, ".semgrepignore": "docs/\n/src/hidden_test.go\n", "src/decoy\nsrc/hidden_test.go": EVAL, "src/hidden_test.go": "export const ok = 1;\n"},
         git_subdir="packages/app", rule="generic", expect="identical", must_scan=["src/decoy\nsrc/hidden_test.go"], must_skip=["src/hidden_test.go"],
         note="a file named `src/decoy<LF>src/hidden_test.go` is one path: the ignored clean file it imitates stays excluded and the vulnerable file is scanned"),
    case("nested-tests.parent-reference-in-name", "matrix", {**NESTED, "src/x\n../outside_test.go": EVAL}, git_subdir="packages/app", rule="generic", expect="identical",
         must_scan=["src/x\n../outside_test.go"], note="a name containing `<LF>../` is a single file name inside the scan root; nothing outside is added"),
    case("nested-tests.control-characters", "matrix", {**NESTED, "src/u\u0085y/tests/l.ts": EVAL, "src/u\u2028x/tests/k.ts": EVAL, "src/u\u2029p/tests/k2.ts": EVAL,
                                                       "src/tab\tz/tests/t.ts": EVAL, "src/esc\x1bz/tests/e.ts": EVAL, "src/nl\nz/tests/n.ts": EVAL, "src/vt\x0bz/tests/v.ts": EVAL},
         git_subdir="packages/app", rule="generic", expect="identical",
         must_scan=["src/u\u0085y/tests/l.ts", "src/u\u2028x/tests/k.ts", "src/u\u2029p/tests/k2.ts", "src/tab\tz/tests/t.ts", "src/esc\x1bz/tests/e.ts", "src/nl\nz/tests/n.ts", "src/vt\x0bz/tests/v.ts"],
         note="TAB, ESC, VT, LF, U+0085, U+2028 and U+2029 in directory names are one path each and are scanned"),
    case("nested-tests.carriage-return-name", "matrix", {**NESTED, "src/cr\r/tests/m.ts": EVAL}, git_subdir="packages/app", rule="generic", expect="more scanned",
         must_scan=["src/cr\r/tests/m.ts"], note="1.23.0 drops any path containing a carriage return from the scan (a quirk); the candidate scans it, the safe direction"),
    case("nested-tests.quotes-spaces-globs-dashes", "matrix", {**NESTED, "src/a b \"q\" 's'/tests/h.ts": EVAL, "src/a*b[1]?/tests/j.ts": EVAL, "src/{x,y}/tests/k.ts": EVAL, "src/--x/tests/g.ts": EVAL,
                                                                "src/-t/tests/f.ts": EVAL, "src/$(id)/tests/d.ts": EVAL, "src/`x`/tests/b.ts": EVAL, "src/!neg/tests/n.ts": EVAL, "src/#hash/tests/h2.ts": EVAL},
         git_subdir="packages/app", rule="generic", expect="identical",
         must_scan=["src/a b \"q\" 's'/tests/h.ts", "src/a*b[1]?/tests/j.ts", "src/{x,y}/tests/k.ts", "src/--x/tests/g.ts", "src/-t/tests/f.ts", "src/$(id)/tests/d.ts", "src/`x`/tests/b.ts", "src/!neg/tests/n.ts", "src/#hash/tests/h2.ts"],
         note="quotes, spaces, glob and shell metacharacters and leading dashes are passed as ./-prefixed literal targets"),
    case("nested-tests.directory-named-test-go", "matrix", {"src/foo_test.go/a.ts": EVAL, "src/ok.ts": "export const ok = 1;\n", ".semgrepignore": ""}, git_subdir="packages/app", expect="identical",
         must_scan=["src/foo_test.go/a.ts"], note="a directory named like the *_test.go built-in is hidden natively and is restored (empty subproject .semgrepignore)"),
    case("nested-tests.directory-named-test-go-deep", "matrix", {**NESTED, "src/foo_test.go/inner_test.go/a.ts": EVAL, "src/foo_test.go/x_test.go": EVAL, "lib/b_test.go/tests/c.ts": EVAL},
         git_subdir="packages/app", rule="generic", expect="identical", unique_findings=True,
         must_scan=["src/foo_test.go/inner_test.go/a.ts", "src/foo_test.go/x_test.go", "lib/b_test.go/tests/c.ts"],
         note="directories matching *_test.go, nested inside each other and holding test files and test directories, are scanned once"),
    case("nested-tests.case-variants", "matrix", {**NESTED, "src/Tests/a.ts": EVAL, "src/TEST/b.ts": EVAL, "src/TestSuite/c.ts": EVAL, "src/a_TEST.go": EVAL, "src/b_test.GO": EVAL},
         git_subdir="packages/app", rule="generic", expect="identical", must_scan=["src/Tests/a.ts", "src/TEST/b.ts", "src/TestSuite/c.ts", "src/a_TEST.go", "src/b_test.GO"],
         note="Opengrep's built-ins are case-sensitive: other spellings are never hidden and never restored"),
    case("nested-tests.symlinked-test-directory", "matrix", {**NESTED}, git_subdir="packages/app", rule="generic", expect="identical", outside_links=["src/tests/linked", "src/test2/tests"],
         must_skip=["src/tests/linked/o.ts", "src/test2/tests/o.ts"], note="a symlinked directory is never followed out of the scan root, and a symlink named tests is not a target"),
    case("nested-tests.ignore-depths", "matrix", {**NESTED, ".semgrepignore": "docs/\nig/\n/src/b/tests/private/\n", "src/a/tests/x.ts": EVAL, "src/gi/tests/g.ts": EVAL, "src/ig/b/tests/i.ts": EVAL,
                                                   "src/b/tests/private/p.ts": EVAL, "src/b/tests/public/q.ts": EVAL, "src/c/d/e/tests/ok.ts": EVAL, "src/c/d/gi/tests/g2.ts": EVAL,
                                                   "../../.gitignore": "gi/\n"},
         git_subdir="packages/app", rule="generic", expect="identical", must_scan=["src/a/tests/x.ts", "src/b/tests/public/q.ts", "src/c/d/e/tests/ok.ts"],
         must_skip=["src/gi/tests/g.ts", "src/ig/b/tests/i.ts", "src/b/tests/private/p.ts", "src/c/d/gi/tests/g2.ts"],
         note="test directories ignored by .gitignore or the project .semgrepignore at different depths, inside or around a test directory, stay excluded"),
    case("nested-tests.baseline-at-depths", "matrix", {**NESTED, "src/vendor/tests/v.ts": EVAL, "src/a/node_modules/tests/n.ts": EVAL, "src/b/dist/tests/d.ts": EVAL, "src/c/tests/vendor/tests/w.ts": EVAL,
                                                       "src/d/x.min.js": EVAL},
         git_subdir="packages/app", rule="generic", expect="approved-reduction", approved_paths=["src/vendor/tests/v.ts", "src/c/tests/vendor/tests/w.ts", "src/d/x.min.js"],
         must_skip=["src/vendor/tests/v.ts", "src/a/node_modules/tests/n.ts", "src/b/dist/tests/d.ts", "src/c/tests/vendor/tests/w.ts", "src/d/x.min.js"],
         note="baseline entries (vendor, node_modules, dist, minified files) exclude test directories at any depth around or inside them"),
    case("nested-tests.long-path", "matrix", {**NESTED, "src/" + "/".join(["segment-with-a-long-name-%02d" % i for i in range(1, 12)]) + "/tests/deep.ts": EVAL}, git_subdir="packages/app", rule="generic",
         expect="identical", must_scan=["src/" + "/".join(["segment-with-a-long-name-%02d" % i for i in range(1, 12)]) + "/tests/deep.ts"], note="a path of about 350 characters is restored"),
    case("nested-tests.outside-git", "matrix", {**NESTED}, rule="generic", expect="identical", must_scan=["src/tests/t.ts"],
         note="outside Git --project-root already replaces the defaults: no restoration needed"),
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
    if spec.get("rule") == "generic":
        (rules / "rule.yaml").write_text('rules:\n  - id: generic-eval\n    languages: [generic]\n    message: eval\n    severity: ERROR\n    pattern: eval("x")\n')
    else:
        shutil.copyfile(rule_source, rules / "rule.yaml")
    if spec.get("git") or spec.get("git_subdir"):
        subprocess.run(["git", "init", "-q"], cwd=project, check=True)
        if spec.get("track"):
            subprocess.run(["git", "add", "-f", *spec["track"]], cwd=scan, check=True)
    for link in spec.get("outside_links", []):
        outside = base / (spec["id"] + "-outside-" + link.replace("/", "_"))
        outside.mkdir()
        (outside / "o.ts").write_text(EVAL)
        (scan / link).parent.mkdir(parents=True, exist_ok=True)
        os.symlink(outside, scan / link)
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
    """Every field of the result contract OxGuard consumes: rule, path, line/column, message, severity, diagnostics and exit."""
    try:
        report = json.loads(process.stdout)
    except ValueError:
        return {"exit": process.returncode, "scanned": [], "findings": [],
                "errors": [["unparseable", "", "output: " + process.stderr.decode(errors="replace").strip()[:120], ""]]}
    return {"exit": process.returncode, "scanned": sorted(report["paths"]["scanned"]),
            "findings": [[f["check_id"], f["path"], f["start"]["line"], f["start"]["col"], f["extra"]["message"], f["extra"]["severity"]] for f in report["results"]],
            "errors": sorted([e.get("type", ""), e.get("level", ""), e.get("message", ""), str(e.get("path", ""))] for e in report["errors"])}


def without(report, paths):
    """The baseline report as if the approved paths were never selected: scanned files, findings and diagnostics about
    those paths are removed and the exit is recomputed from what remains. Everything else must still match exactly."""
    kept = {**report, "scanned": [p for p in report["scanned"] if p not in paths], "findings": [f for f in report["findings"] if f[1] not in paths],
            "errors": [e for e in report["errors"] if e[3] not in paths]}
    if report["exit"] in (0, 1):
        kept["exit"] = 1 if kept["findings"] else 0
    return kept


def classify(old, new, approved=()):
    if old == new:  # compares every field above, including messages, severities and diagnostics
        return "identical"
    if approved and without(old, set(approved)) == new:
        return "approved-reduction"
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


def check_tolerance(old, new, spec):
    return old["exit"] == 2 and new["exit"] in (0, 1) and not new["errors"] and not any(p in new["scanned"] for p in spec.get("must_skip", []))


def check_rejection(old, new, spec):
    return (new["exit"] == "rejected" and "Unsupported exclusion" in new["errors"][0][2] and old["exit"] == 1
            and bool(old["scanned"]) and all("/" not in p for p in old["scanned"]))


def check_both_fail(old, new, spec):
    return old["exit"] != 0 and new["exit"] not in (0, "rejected") and bool(new["errors"])


LABEL_CHECKS = {"approved-tolerance": check_tolerance, "approved-rejection": check_rejection, "both-fail": check_both_fail}


def self_test():
    """The approved-reduction check must fail on drift in any contract field, not only on the selected files."""
    rule = "rules.eval"
    old = {"exit": 1, "scanned": ["a.ts", "x.min.js"], "errors": [["ParseError", "warn", "cannot parse", "a.ts"], ["ParseError", "warn", "cannot parse", "x.min.js"]],
           "findings": [[rule, "a.ts", 1, 1, "Avoid eval", "ERROR"], [rule, "x.min.js", 1, 1, "Avoid eval", "ERROR"]]}
    good = {"exit": 1, "scanned": ["a.ts"], "errors": [["ParseError", "warn", "cannot parse", "a.ts"]], "findings": [[rule, "a.ts", 1, 1, "Avoid eval", "ERROR"]]}
    assert classify(old, good, ["x.min.js"]) == "approved-reduction", "the approved reduction itself must be recognized"
    assert classify(old, old, ["x.min.js"]) == "identical"
    drift = {
        "diagnostic message": {**good, "errors": [["ParseError", "warn", "changed message", "a.ts"]]},
        "diagnostic path": {**good, "errors": [["ParseError", "warn", "cannot parse", "other.ts"]]},
        "diagnostic type": {**good, "errors": [["SemgrepError", "warn", "cannot parse", "a.ts"]]},
        "diagnostic level": {**good, "errors": [["ParseError", "error", "cannot parse", "a.ts"]]},
        "extra diagnostic": {**good, "errors": good["errors"] + [["ParseError", "warn", "again", "a.ts"]]},
        "missing diagnostic": {**good, "errors": []},
        "finding message": {**good, "findings": [[rule, "a.ts", 1, 1, "Changed", "ERROR"]]},
        "finding severity": {**good, "findings": [[rule, "a.ts", 1, 1, "Avoid eval", "WARNING"]]},
        "finding rule": {**good, "findings": [["rules.other", "a.ts", 1, 1, "Avoid eval", "ERROR"]]},
        "finding line": {**good, "findings": [[rule, "a.ts", 2, 1, "Avoid eval", "ERROR"]]},
        "finding column": {**good, "findings": [[rule, "a.ts", 1, 2, "Avoid eval", "ERROR"]]},
        "finding path": {**good, "findings": [[rule, "b.ts", 1, 1, "Avoid eval", "ERROR"]]},
        "duplicate finding": {**good, "findings": good["findings"] * 2},
        "exit code": {**good, "exit": 2},
        "extra scanned file": {**good, "scanned": ["a.ts", "b.ts"]},
    }
    for name, candidate in drift.items():
        label = classify(old, candidate, ["x.min.js"])
        assert label not in ("approved-reduction", "identical"), f"self-test: drift in {name} was classified {label}"
    # Label checks fail when the specific behavior they approve is absent.
    base_report = {"exit": 1, "scanned": ["a.ts"], "findings": [], "errors": []}
    assert not check_tolerance({**base_report, "exit": 1}, {**base_report, "exit": 1}, {}), "tolerance needs the baseline to refuse to start"
    assert not check_tolerance({**base_report, "exit": 2}, {**base_report, "exit": 1, "errors": [["SemgrepError", "error", "m", ""]]}, {}), "tolerance must not hide a scan error"
    assert not check_rejection({**base_report, "scanned": ["a.ts", "src/a.ts"]}, {**base_report, "exit": "rejected", "errors": [["rejected", "error", "Unsupported exclusion", ""]]}, {}), "rejection needs the old all-excluded behavior"
    assert not check_both_fail({**base_report, "exit": 2}, {**base_report, "exit": 0}, {}), "both-fail needs the candidate to fail"
    assert not check_both_fail({**base_report, "exit": 2}, {**base_report, "exit": 1, "errors": []}, {}), "both-fail needs a reported error"
    # Without an approved path nothing is forgiven.
    assert classify(old, good) != "approved-reduction"


self_test()
if args.self_test:
    print("self-test passed")
    raise SystemExit(0)


def candidate_args(helper, scan, spec, temporary):
    out = temporary / "args.json"
    env = {**os.environ, "OXGUARD_COMPARE_ROOT": str(scan), "OXGUARD_COMPARE_ARGS": str(out),
           "OXGUARD_COMPARE_EXCLUDE": json.dumps(spec.get("exclude", [])), "OXGUARD_COMPARE_ENGINE": str(engines[1][1])}
    started = time.perf_counter()
    subprocess.run([str(helper), "-test.run=^TestSecurityScopeComparisonHelper$"], check=True, capture_output=True, env=env)
    elapsed = (time.perf_counter() - started) * 1000
    return json.loads(out.read_text()), elapsed


def baseline_args(spec):
    return [item for d in DEFAULT_EXCLUDES + spec.get("exclude", []) for item in ("--exclude", d)]


def embedded_template(data, start):
    """The default ignore list embedded in an engine binary: the text from `start` to the last built-in entry."""
    begin = data.find(start)
    if begin < 0:
        return None
    end = data.index(b"*_test.go\n", begin) + len(b"*_test.go\n")
    return [line.strip() for line in data[begin:end].decode().splitlines() if line.strip() and not line.startswith("#")]


LAYER_CHECK = {"done": False}


def check_layers(layers):
    """The code's two layers must reproduce the default list embedded in each pinned engine, entry for entry (2.1.0: Semgrepignore.ml
    default_semgrepignore_for_semgrep_scan; 1.23.0: templates/.semgrepignore). Run once; fails the whole comparison on drift."""
    if LAYER_CHECK["done"]:
        return
    LAYER_CHECK["done"] = True
    ours = layers["baseline"] + layers["replaceable"]
    checked = 0
    for (version, engine, _), marker in zip(engines, (b"# Git administrative folder or file", b"# Administrative folder or file used by popular version control systems")):
        listed = embedded_template(engine.read_bytes(), marker)
        if listed is None:
            continue  # the baseline engine may not embed a readable copy
        checked += 1
        if listed != ours:
            raise SystemExit(f"The built-in default list of Opengrep {version} differs from the OxGuard layers:\n  engine {listed}\n  OxGuard {ours}")
    if not checked:
        raise SystemExit("Could not read an embedded default ignore list from either engine")


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
            plan, enumeration_ms = candidate_args(helper, scan, spec, temporary)
            new_args = plan["args"]
            check_layers(plan["layers"])
            old_args = baseline_args(spec) + baseline_targets(scan)
            old_process, _ = run_engine(engines[0][1], scan, [], old_args)
            if plan.get("error"):
                # The production scope refused the configuration: Opengrep is not started and the gate reports invalid_configuration.
                new = {"exit": "rejected", "scanned": [], "findings": [], "errors": [["rejected", "error", plan["error"], ""]]}
            elif plan["empty"]:
                # Production code does not start Opengrep when nothing is selected: a clean, complete result with exit 0.
                new = {"exit": 0, "scanned": [], "findings": [], "errors": []}
            else:
                new_process, _ = run_engine(engines[1][1], scan, new_args, [])
                new = summarize(new_process)
            old = summarize(old_process)
            measured = classify(old, new, spec.get("approved_paths", ()))
            if spec.get("label_check") and LABEL_CHECKS[spec["label_check"]](old, new, spec):
                measured = spec["label_check"]
            row = {"case": spec["id"], "group": spec["group"], "baseline": old, "candidate": new, "classification": measured,
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
            if spec.get("unique_findings") and len({tuple(f[:4]) for f in new["findings"]}) != len(new["findings"]):
                failures.append(f"{spec['id']}: duplicate findings in the 2.1.0 report")
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
            plan, helper_ms = candidate_args(helper, base, spec, temporary)
            new_args = plan["args"]
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

def matrix_markdown(rows):
    """The compatibility matrix exactly as measured; docs/opengrep-2.1-migration.md embeds it between the matrix markers."""
    counts = {}
    for row in rows:
        counts[row["classification"]] = counts.get(row["classification"], 0) + 1
    summary = ", ".join(f"{n} {label}" for label, n in sorted(counts.items()))
    lines = [f"{len(rows)} cases measured with both engines: {summary}.", "",
             "| Case | Scenario | 1.23.0 files | 2.1.0 files | Exit 1.23.0 / 2.1.0 | Result |", "|---|---|---:|---:|---|---|"]
    for row in rows:
        old, new = row["baseline"], row["candidate"]
        only_old, only_new = sorted(set(old["scanned"]) - set(new["scanned"])), sorted(set(new["scanned"]) - set(old["scanned"]))
        delta = []
        if only_old:
            delta.append("only 1.23.0: " + ", ".join(f"`{p}`" for p in only_old))
        if only_new:
            delta.append("only 2.1.0: " + ", ".join(f"`{p}`" for p in only_new))
        scenario = (row["note"] + (" " if row["note"] and delta else "") + ("(" + "; ".join(delta) + ")" if delta else "")).replace("|", "\\|")
        lines.append(f"| `{row['case']}` | {scenario} | {len(old['scanned'])} | {len(new['scanned'])} | {old['exit']} / {new['exit']} | {row['classification']} |")
    return "\n".join(lines)


if args.update_doc:
    START, END = "<!-- matrix:start (generated by scripts/compare-opengrep.py --update-doc; do not edit by hand) -->", "<!-- matrix:end -->"
    text = args.update_doc.read_text()
    begin, finish = text.index("<!-- matrix:start"), text.index(END)
    args.update_doc.write_text(text[:begin] + START + "\n" + matrix_markdown(results) + "\n" + text[finish:])

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
