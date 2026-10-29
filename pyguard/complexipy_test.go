package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// writeComplexipyReport stores a complexipy JSON report under the runner root
// and returns its path, mirroring the --output file pyguard passes on every run.
func writeComplexipyReport(t *testing.T, r *Runner, data string) string {
	t.Helper()
	path := filepath.Join(r.root, "complexipy.json")
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

const complexipyAboveThreshold = `[
  {"complexity": 0, "file_name": "main.py", "function_name": "flat", "path": "src/main.py", "refactor_plans": []},
  {"complexity": 21, "file_name": "main.py", "function_name": "above", "path": "src/main.py", "refactor_plans": []}
]`

const complexipyAtThreshold = `[
  {"complexity": 20, "file_name": "main.py", "function_name": "boundary", "path": "src/main.py", "refactor_plans": []},
  {"complexity": 0, "file_name": "main.py", "function_name": "flat", "path": "src/main.py", "refactor_plans": []}
]`

const complexipyBanner = "──── 🐙 complexipy ────\nsrc/main.py\n    flat 0  ✅  PASSED\n"

// TestComplexipyProcess is the fake analyzer used by the human-mode runner
// tests. It is the test binary itself, so the timeout/log-file paths exercise
// the production subprocess runner without platform-specific scripts.
func TestComplexipyProcess(t *testing.T) {
	switch mode := os.Getenv("PYGUARD_COMPLEXIPY_HELPER"); mode {
	case "":
		return
	case "timeout":
		fmt.Fprintln(os.Stdout, "complexipy started")
		time.Sleep(10 * time.Second)
	case "above":
		fmt.Fprint(os.Stdout, complexipyBanner)
		fmt.Fprintln(os.Stderr, "complexipy diagnostic")
		path := os.Getenv("PYGUARD_COMPLEXIPY_REPORT")
		if path == "" || os.WriteFile(path, []byte(complexipyAboveThreshold), 0o644) != nil {
			os.Exit(2)
		}
		os.Exit(1) // complexipy's advisory threshold exit
	}
	os.Exit(0)
}

func complexipyHumanHarness(t *testing.T, mode string) (*Runner, string) {
	t.Helper()
	r := &Runner{root: t.TempDir(), timeout: 1, outputMode: "human", result: newRunResult("radon")}
	r.logFile = filepath.Join(t.TempDir(), "pyguard.log")
	report := filepath.Join(t.TempDir(), "complexipy.json")
	t.Setenv("PYGUARD_COMPLEXIPY_HELPER", mode)
	t.Setenv("PYGUARD_COMPLEXIPY_REPORT", report)
	return r, report
}

func complexipyHumanHelperArgs() []string {
	return []string{os.Args[0], "-test.run=^TestComplexipyProcess$"}
}

// Human mode must use the timeout-aware subprocess runner, not RunSilent: a
// hung complexipy fails at --timeout instead of holding the project lock.
func TestComplexipyHumanHonorsTimeout(t *testing.T) {
	r, report := complexipyHumanHarness(t, "timeout")
	start := time.Now()
	code := runComplexipyHumanReport(r, report, complexipyHumanHelperArgs())
	elapsed := time.Since(start)
	if code != 1 {
		t.Fatalf("timeout exit = %d, want 1", code)
	}
	if elapsed > 8*time.Second {
		t.Fatalf("timeout took %s; the runner did not enforce --timeout", elapsed)
	}
	data, err := os.ReadFile(r.logFile)
	if err != nil || !strings.Contains(string(data), "complexipy started") {
		t.Fatalf("timeout output missing from --log-file: %q, %v", data, err)
	}
}

// Human mode must capture complexipy through --log-file and still treat its
// threshold exit 1 as advisory (exit 0), not as an analyzer failure.
func TestComplexipyHumanHonorsLogFileAndAdvisoryExit(t *testing.T) {
	r, report := complexipyHumanHarness(t, "above")
	if code := runComplexipyHumanReport(r, report, complexipyHumanHelperArgs()); code != 0 {
		t.Fatalf("advisory threshold exit changed pyguard exit: %d", code)
	}
	data, err := os.ReadFile(r.logFile)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"flat 0", "complexipy diagnostic"} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("--log-file missing %q: %q", want, data)
		}
	}
}

// complexipy folds nested function bodies into the enclosing function and
// reports a single row under the outer name; there is no inner row to restore.
const complexipyNestedFunction = `[
  {"complexity": 36, "file_name": "nested.py", "function_name": "outer", "path": "src/nested.py", "refactor_plans": []}
]`

// Every function in the report becomes one complexipy.cognitive measurement at
// level code with unit score and threshold 20; exactly 20 is clean.
func TestComplexipyAdapterMeasuresEveryFunction(t *testing.T) {
	r := contractRunner(t)
	spec := toolSpec{gate: "radon", adapter: "complexipy_cognitive", advisory: true, reportPath: writeComplexipyReport(t, r, complexipyAtThreshold)}
	r.normalize(spec, Result{ok: true}, strings.NewReader(complexipyBanner), nil)
	if len(r.result.Findings) != 0 {
		t.Fatalf("threshold boundary changed: %+v", r.result.Findings)
	}
	if len(r.result.Measurements) != 2 {
		t.Fatalf("one measurement per function required: %+v", r.result.Measurements)
	}
	for _, m := range r.result.Measurements {
		if m.Metric != "complexipy.cognitive" || m.Level != "code" || m.Unit != "score" || m.Threshold != 20 {
			t.Fatalf("measurement contract: %+v", m)
		}
		if m.Location.File != "src/main.py" || m.Location.Symbol == "" {
			t.Fatalf("measurement location: %+v", m.Location)
		}
	}
}

// A function scoring 21 yields exactly one warning finding with observed 21 and
// threshold 20; exactly 20 yields none.
func TestComplexipyAdapterWarnsOnlyAboveThreshold(t *testing.T) {
	r := contractRunner(t)
	spec := toolSpec{gate: "radon", adapter: "complexipy_cognitive", advisory: true, reportPath: writeComplexipyReport(t, r, complexipyAboveThreshold)}
	r.normalize(spec, Result{ok: true}, strings.NewReader(complexipyBanner), nil)
	if len(r.result.Findings) != 1 {
		t.Fatalf("one warning above 20 required: %+v", r.result.Findings)
	}
	f := r.result.Findings[0]
	if f.Rule != "pyguard.complexipy.cognitive_exceeded" || f.Severity != "warning" || f.Category != "quality" || f.Gate != "radon" {
		t.Fatalf("finding contract: %+v", f)
	}
	if f.Observed == nil || *f.Observed != 21 || f.Threshold == nil || *f.Threshold != 20 {
		t.Fatalf("observed/threshold: %+v", f)
	}
	if f.Location == nil || f.Location.File != "src/main.py" || f.Location.Symbol != "above" {
		t.Fatalf("location: %+v", f.Location)
	}
	if len(r.result.Measurements) != 2 {
		t.Fatalf("measurements: %+v", r.result.Measurements)
	}
}

// A report with only the enclosing function is kept as one measurement under
// the outer symbol. complexipy already folded the nested function's complexity
// into that score, so pyguard must not invent an inner measurement or warning.
func TestComplexipyNestedFunctionFoldsIntoOuterMeasurement(t *testing.T) {
	r := contractRunner(t)
	spec := toolSpec{gate: "radon", adapter: "complexipy_cognitive", advisory: true, reportPath: writeComplexipyReport(t, r, complexipyNestedFunction)}
	r.normalize(spec, Result{ok: false, exitCode: 1}, strings.NewReader(complexipyBanner), nil)
	if err := r.result.Finish(0); err != nil {
		t.Fatal(err)
	}
	if len(r.result.Measurements) != 1 || r.result.Measurements[0].Location.Symbol != "outer" || r.result.Measurements[0].Value != 36 {
		t.Fatalf("nested measurement must stay on the outer function: %+v", r.result.Measurements)
	}
	for _, m := range r.result.Measurements {
		if m.Location.Symbol == "inner" {
			t.Fatal("invented a measurement for a folded nested function")
		}
	}
	if len(r.result.Findings) != 1 || r.result.Findings[0].Location == nil || r.result.Findings[0].Location.Symbol != "outer" || r.result.Findings[0].Status != StatusAdvisory {
		t.Fatalf("nested warning must be advisory on the outer function: %+v", r.result.Findings)
	}
	if r.result.ExitCode != 0 || r.result.Status != StatusAdvisory {
		t.Fatalf("folded nested warning changed exit/status: exit=%d status=%s", r.result.ExitCode, r.result.Status)
	}
}

// complexipy exits 1 exactly when a function exceeds the threshold. That exit
// must never become a blocking finding or a failed gate: the run stays advisory
// with exit code 0.
func TestComplexipyWarningNeverBlocksEvenOnNonZeroExit(t *testing.T) {
	r := contractRunner(t)
	spec := toolSpec{gate: "radon", adapter: "complexipy_cognitive", advisory: true, reportPath: writeComplexipyReport(t, r, complexipyAboveThreshold)}
	r.normalize(spec, Result{ok: false, exitCode: 1}, strings.NewReader(complexipyBanner), nil)
	if err := r.result.Finish(0); err != nil {
		t.Fatal(err)
	}
	if r.result.ExitCode != 0 || r.result.Status != StatusAdvisory {
		t.Fatalf("advisory changed exit/status: exit=%d status=%s", r.result.ExitCode, r.result.Status)
	}
	if len(r.result.Gates) != 1 || r.result.Gates[0].Status != StatusAdvisory || r.result.Gates[0].Normalization != Complete {
		t.Fatalf("gates: %+v", r.result.Gates)
	}
	for _, f := range r.result.Findings {
		if f.Status != StatusAdvisory || f.Severity != "warning" {
			t.Fatalf("advisory finding: %+v", f)
		}
	}
}

// The wiring must never let an execution failure pass silently, and the tool
// spec carries the advisory policy that keeps warnings non-blocking.
func TestComplexipySpecIsAdvisoryRadonGate(t *testing.T) {
	spec := complexipySpec("report.json")
	if spec.gate != "radon" || spec.adapter != "complexipy_cognitive" || !spec.advisory || spec.reportPath != "report.json" {
		t.Fatalf("spec: %+v", spec)
	}
}

// A non-zero exit with no above-threshold function and no failure marker is an
// analyzer failure, never a quiet advisory pass.
func TestComplexipyUnexplainedExitIsExecutionError(t *testing.T) {
	r := contractRunner(t)
	spec := toolSpec{gate: "radon", adapter: "complexipy_cognitive", advisory: true, reportPath: writeComplexipyReport(t, r, complexipyAtThreshold)}
	r.normalize(spec, Result{ok: false, exitCode: 1}, strings.NewReader(complexipyBanner), nil)
	if len(r.result.Findings) != 1 {
		t.Fatalf("findings: %+v", r.result.Findings)
	}
	f := r.result.Findings[0]
	if f.Status != StatusExecutionError || f.Category != "analyzer_failure" {
		t.Fatalf("unexplained exit: %+v", f)
	}
}

// complexipy prints per-input parse failures on stdout and still writes a
// partial report; the marker makes the run an explicit analyzer failure instead
// of silently dropping the broken file.
func TestComplexipyDroppedInputIsExecutionError(t *testing.T) {
	r := contractRunner(t)
	spec := toolSpec{gate: "radon", adapter: "complexipy_cognitive", advisory: true, reportPath: writeComplexipyReport(t, r, complexipyAboveThreshold)}
	banner := complexipyBanner + "error: Failed to process /absent/bad.py - Please check file/folder exists or check syntax\n"
	r.normalize(spec, Result{ok: false, exitCode: 1}, strings.NewReader(banner), nil)
	if len(r.result.Findings) != 1 {
		t.Fatalf("findings: %+v", r.result.Findings)
	}
	f := r.result.Findings[0]
	if f.Status != StatusExecutionError || f.Category != "analyzer_failure" {
		t.Fatalf("dropped input: %+v", f)
	}
	if len(r.result.Measurements) != 0 {
		t.Fatalf("partial report certified measurements: %+v", r.result.Measurements)
	}
}

// A missing or malformed report is an adapter failure, never an empty pass.
func TestComplexipyMissingReportIsAdapterFailure(t *testing.T) {
	r := contractRunner(t)
	spec := toolSpec{gate: "radon", adapter: "complexipy_cognitive", advisory: true, reportPath: filepath.Join(r.root, "absent.json")}
	r.normalize(spec, Result{ok: true}, strings.NewReader(complexipyBanner), nil)
	if len(r.result.Findings) != 1 || r.result.Findings[0].Status != StatusExecutionError || r.result.Findings[0].Category != "adapter_failure" {
		t.Fatalf("findings: %+v", r.result.Findings)
	}
}

func TestParseComplexipyReportRejectsInvalidRows(t *testing.T) {
	r := contractRunner(t)
	for _, data := range []string{"", "null", "not json", `[{"complexity":21,"path":"","function_name":"f"}]`, `[{"complexity":-1,"path":"src/main.py","function_name":"f"}]`} {
		path := writeComplexipyReport(t, r, data)
		if _, err := parseComplexipyReport(path); err == nil {
			t.Fatalf("accepted %q", data)
		}
	}
	if _, err := parseComplexipyReport(""); err == nil {
		t.Fatal("empty path accepted")
	}
}

// complexipy's failure marker only counts at the start of a line; indented
// function output such as "error: 5" stays data.
func TestComplexipyFailedMarkerNeedsLinePrefix(t *testing.T) {
	if complexipyFailed("    error: 3\n") {
		t.Fatal("indented text treated as failure")
	}
	if !complexipyFailed("banner\nerror: Failed to process x.py\n") {
		t.Fatal("failure marker missed")
	}
}

// The invocation mirrors radon's exclusion policy and pins the threshold flag,
// so complexipy's own exit code agrees with the pyguard advisory boundary.
func TestComplexipyArgs(t *testing.T) {
	r := &Runner{root: t.TempDir(), excludeTests: true, exclude: []string{"vendor/*"}}
	args := complexipyArgs(r, []string{"src"}, "report.json")
	joined := strings.Join(args, " ")
	for _, want := range []string{
		"uv run complexipy",
		"--max-complexity-allowed 20",
		"--exclude **/test_*.py,**/*_test.py,**/conftest.py,**/tests/**,**/test/**,vendor/*",
		"--output-format json --output report.json",
		"src",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("args %q missing %q", joined, want)
		}
	}
	if args[len(args)-1] != "src" || len(args[0]) == 0 {
		t.Fatalf("paths must be trailing: %v", args)
	}
	// Without test exclusion only the project globs remain.
	r = &Runner{root: t.TempDir()}
	joined = strings.Join(complexipyArgs(r, []string{"."}, ""), " ")
	if strings.Contains(joined, "--exclude") || strings.Contains(joined, "--output") {
		t.Fatalf("unexpected flags: %q", joined)
	}
}

// complexipy has no directory --ignore, so the exclusion globs must cover
// root-level test files/conftest.py and whole tests/ or test/ directories at
// any depth, not just files below a slash.
func TestComplexipyTestExcludeCoversRootAndNestedPaths(t *testing.T) {
	for _, want := range []string{"**/test_*.py", "**/*_test.py", "**/conftest.py", "**/tests/**", "**/test/**"} {
		found := false
		for _, glob := range strings.Split(complexipyTestExclude, ",") {
			if glob == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("complexipy test exclusion %q missing %q", complexipyTestExclude, want)
		}
	}
}

// complexipy on Windows can report extended-length absolute paths. Both the
// diagnostic location and the measurement must stay project-relative with
// forward slashes so JSON consumers see the same path on every platform.
func TestComplexipyNormalizesAnalyzerPaths(t *testing.T) {
	const windowsFile = `C:\Users\runneradmin\AppData\Local\Temp\project\src\complexity.py`
	root := t.TempDir()
	file := filepath.Join(root, "src", "complexity.py")
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte("def above(x):\n    return x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cases := []struct{ name, input, want string }{
		{"already relative", "src/complexity.py", "src/complexity.py"},
		{"absolute", file, "src/complexity.py"},
	}
	if runtime.GOOS == "windows" {
		cases = append(cases,
			struct{ name, input, want string }{"extended backslash", `\\?\` + file, "src/complexity.py"},
			struct{ name, input, want string }{"extended slash", "//?/" + filepath.ToSlash(file), "src/complexity.py"},
		)
	} else {
		cases = append(cases,
			struct{ name, input, want string }{"extended slash", "//?/C:/Users/runneradmin/AppData/Local/Temp/project/src/complexity.py", "C:/Users/runneradmin/AppData/Local/Temp/project/src/complexity.py"},
			struct{ name, input, want string }{"extended backslash", `\\?\` + windowsFile, strings.ReplaceAll(windowsFile, `\`, "/")},
		)
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := &Runner{root: root, timeout: 2, outputMode: "json", result: newRunResult("radon")}
			data := fmt.Sprintf(`[{"complexity":21,"function_name":"above","path":%q}]`, tc.input)
			spec := toolSpec{gate: "radon", adapter: "complexipy_cognitive", advisory: true, reportPath: writeComplexipyReport(t, r, data)}
			r.normalizeComplexipy(spec, Result{ok: false, exitCode: 1}, strings.NewReader(""), nil)
			if len(r.result.Measurements) != 1 {
				t.Fatalf("measurements: %+v", r.result.Measurements)
			}
			if got := r.result.Measurements[0].Location.File; got != tc.want {
				t.Fatalf("measurement path = %q, want %q", got, tc.want)
			}
			if len(r.result.Findings) != 1 || r.result.Findings[0].Location == nil {
				t.Fatalf("findings: %+v", r.result.Findings)
			}
			if got := r.result.Findings[0].Location.File; got != tc.want {
				t.Fatalf("finding path = %q, want %q", got, tc.want)
			}
		})
	}
}
