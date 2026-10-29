package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

// complexipyThreshold is the cognitive complexity advisory threshold. A function
// scoring strictly above it yields a warning finding; exactly the threshold is
// clean. complexipy is pinned at 8.3.0 in the evaluation toolchain and is
// required in the dev dependency group like radon/pyan3.
const complexipyThreshold = 20.0

// complexipySpec is the analyzer contract for one complexipy invocation. The
// advisory flag is load-bearing: above-threshold findings must never change the
// exit code or the gate/overall status, in any output mode. The next release is
// expected to make them blocking (drop advisory then); see CHANGELOG.
func complexipySpec(report string) toolSpec {
	return toolSpec{gate: "radon", adapter: "complexipy_cognitive", advisory: true, reportPath: report}
}

// complexipyTestExclude is the --exclude glob list for pyguard's documented
// test policy: test_*.py, *_test.py, conftest.py, and whole tests/ or test/
// directories. complexipy matches these against project-relative paths and has
// no directory --ignore option, so **/ is required for root-level files as well
// as nested paths (verified with complexipy 8.3.0).
const complexipyTestExclude = "**/test_*.py,**/*_test.py,**/conftest.py,**/tests/**,**/test/**"

// complexipyArgs builds the uv+complexipy invocation. complexipy's own exit code
// is policy-driven: with --max-complexity-allowed it exits 1 exactly when a
// function scores above the threshold (verified: 20 exits 0, 21 exits 1), which
// is why the flag is always pinned to the pyguard threshold. report receives the
// structured JSON (machine mode uses the spooled stdout for diagnostics only).
func complexipyArgs(r *Runner, dirs []string, report string) []string {
	args := []string{"uv", "run", "complexipy"}
	args = append(args, "--max-complexity-allowed", strconv.FormatFloat(complexipyThreshold, 'f', -1, 64))
	// complexipy excludes files with one comma-joined --exclude glob list; it has
	// no directory --ignore. Its globs are relative to the scanned root, so **/
	// covers both root-level test files and whole nested tests/ or test/ dirs.
	var globs []string
	if r.excludeTests {
		globs = append(globs, complexipyTestExclude)
	}
	globs = append(globs, r.exclude...)
	if len(globs) > 0 {
		args = append(args, "--exclude", strings.Join(globs, ","))
	}
	if report != "" {
		args = append(args, "--output-format", "json", "--output", report)
	}
	args = append(args, dirs...)
	return args
}

// complexipyRow is one function in complexipy's JSON report. complexipy reports
// no line numbers; the function name stays the symbol identity.
type complexipyRow struct {
	Complexity float64 `json:"complexity"`
	Path       string  `json:"path"`
	Function   string  `json:"function_name"`
}

// parseComplexipyReport reads complexipy's JSON report. A missing, empty or
// malformed report is an analyzer failure, never an empty pass.
func parseComplexipyReport(path string) ([]complexipyRow, error) {
	if path == "" {
		return nil, fmt.Errorf("complexipy report path missing")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var rows []complexipyRow
	if err := json.NewDecoder(bytes.NewReader(data)).Decode(&rows); err != nil {
		return nil, err
	}
	if rows == nil {
		return nil, fmt.Errorf("complexipy report is not an array")
	}
	for _, row := range rows {
		if row.Path == "" || row.Function == "" || row.Complexity < 0 {
			return nil, fmt.Errorf("invalid complexipy result")
		}
	}
	return rows, nil
}

// complexipyFailed reports whether analyzer output carries complexipy's
// per-input failure marker ("error: Failed to process ..."). complexipy prints
// such failures on stdout and still writes a partial report, so the marker is
// the only way to distinguish a dropped file from a threshold exit.
func complexipyFailed(output string) bool {
	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(line, "error:") {
			return true
		}
	}
	return false
}

// runComplexipy measures cognitive complexity for every function. Machine mode
// normalizes one complexipy.cognitive measurement per function and one advisory
// warning per function above the threshold; only execution failures (missing
// tool, unreadable report, dropped input, unexplained exit) return 1.
func runComplexipy(r *Runner, dirs []string) int {
	if !r.machine() {
		return runComplexipyHuman(r, dirs)
	}
	before := len(r.result.Findings)
	report, err := r.reportFile("complexipy", "complexipy.json")
	if err != nil {
		return r.executionFailure("radon", "diagnostics_failure", err.Error())
	}
	r.RunTool(complexipySpec(report), "complexipy cognitive", complexipyArgs(r, dirs, report)...)
	for _, f := range r.result.Findings[before:] {
		if f.Status == StatusExecutionError {
			return 1
		}
	}
	return 0
}

// runCaptured executes through RunTool's machine path so captured human-mode
// analysis still honors --timeout and --log-file. The worker owns a throwaway
// result: callers render their own display and must not leak machine findings
// into the human result.
func (r *Runner) runCaptured(spec toolSpec, name string, args ...string) (Result, []Finding) {
	worker := *r
	worker.outputMode = "json"
	worker.result = newRunResult(spec.gate)
	before := len(worker.result.Findings)
	res := worker.RunTool(spec, name, args...)
	return res, worker.result.Findings[before:]
}

// runComplexipyHuman renders the same analysis as a score list. Threshold
// exceedance (complexipy's own exit 1 with a clean report) prints an advisory
// line and keeps exit 0; analyzer failures return 1.
func runComplexipyHuman(r *Runner, dirs []string) int {
	report, err := os.CreateTemp("", "pyguard-complexipy-*.json")
	if err != nil {
		return r.executionFailure("radon", "diagnostics_failure", err.Error())
	}
	path := report.Name()
	report.Close()
	defer os.Remove(path)

	return runComplexipyHumanReport(r, path, complexipyArgs(r, dirs, path))
}

// runComplexipyHumanReport runs complexipy and renders the score list. It keeps
// the advisory policy: complexipy's threshold exit is not an error, while a
// timeout, dropped input, unreadable report or unexplained exit still fails.
func runComplexipyHumanReport(r *Runner, reportPath string, args []string) int {
	res, findings := r.runCaptured(complexipySpec(reportPath), "complexipy cognitive", args...)
	if code := complexipyHumanFailure(r, res, findings); code != 0 {
		return code
	}
	rows, err := parseComplexipyReport(reportPath)
	if err != nil {
		r.printf("  [FAIL] complexipy — readable report missing: %v\n", err)
		return 1
	}
	above := 0
	r.println("  complexipy (cognitive complexity, advisory above 20):")
	for _, row := range rows {
		marker := ""
		if row.Complexity > complexipyThreshold {
			above++
			marker = " (advisory)"
		}
		r.printf("    %s %s %g%s\n", row.Path, row.Function, row.Complexity, marker)
	}
	if !res.ok && above == 0 {
		r.println("  [FAIL] complexipy — exited without reporting a function above the threshold")
		return 1
	}
	if above > 0 {
		r.printf("  [ADVISORY] complexipy — %d function(s) above %g; warnings never change the exit code\n", above, complexipyThreshold)
	} else {
		r.println("  [OK]   complexipy — all functions at or below the threshold")
	}
	return 0
}

// complexipyHumanFailure distinguishes advisory threshold exits from execution
// failures. It reports the runner's timeout/category, complexipy's per-input
// failure marker, and normalized execution findings from the captured run.
func complexipyHumanFailure(r *Runner, res Result, findings []Finding) int {
	marker := complexipyFailed(res.output) || complexipyFailed(res.stderr)
	var failure *Finding
	for i := range findings {
		if findings[i].Status == StatusExecutionError {
			failure = &findings[i]
			break
		}
	}
	if res.category == "" && !marker && failure == nil {
		return 0
	}
	switch {
	case res.category != "":
		r.printf("  [FAIL] complexipy — %s\n", res.message)
	case marker:
		r.println("  [FAIL] complexipy — the analyzer could not evaluate every input:")
	case failure != nil:
		r.printf("  [FAIL] complexipy — %s\n", failure.Evidence)
	}
	printComplexipyErrors(r, res.output, res.stderr)
	return 1
}

func printComplexipyErrors(r *Runner, stdout, stderr string) {
	for _, output := range []string{stdout, stderr} {
		for _, line := range strings.Split(output, "\n") {
			if strings.HasPrefix(line, "error:") {
				r.println("         " + strings.TrimSpace(line))
			}
		}
	}
}

// normalizeComplexipy decodes the report file into the contract: one
// measurement per function, advisory warnings above the threshold. The reader
// is the spooled stdout, which complexipy uses for its human banner and
// per-input failure marker only.
func (r *Runner) normalizeComplexipy(spec toolSpec, res Result, stdout io.Reader, refs []string) {
	banner, err := io.ReadAll(stdout)
	if err == nil && complexipyFailed(string(banner)) {
		r.result.AddFinding(Finding{Gate: spec.gate, Rule: "pyguard.execution.analyzer_failure", Severity: "error", Status: StatusExecutionError, Category: "analyzer_failure", Evidence: "Complexipy could not evaluate every source input.", Diagnostics: refs})
		return
	}
	rows, parseErr := parseComplexipyReport(spec.reportPath)
	if parseErr != nil {
		r.result.AddFinding(Finding{Gate: spec.gate, Rule: "pyguard.execution.adapter_failure", Severity: "error", Status: StatusExecutionError, Category: "adapter_failure", Evidence: "Complexipy structured output is missing or invalid; inspect diagnostics.", Diagnostics: refs})
		return
	}
	above := false
	for _, row := range rows {
		location := Location{File: relativePath(r.root, row.Path), Symbol: row.Function}
		value := row.Complexity
		r.result.Measurements = append(r.result.Measurements, Measurement{Metric: "complexipy.cognitive", Level: "code", Location: location, Value: value, Unit: "score", Threshold: complexipyThreshold})
		if value > complexipyThreshold {
			above = true
			threshold := complexipyThreshold
			r.result.AddFinding(Finding{Gate: spec.gate, Rule: "pyguard.complexipy.cognitive_exceeded", Severity: "warning", Status: StatusAdvisory, Category: "quality", Location: &location, Observed: &value, Threshold: &threshold, Evidence: "Cognitive complexity exceeds 20.", Diagnostics: refs})
		}
	}
	// complexipy exits 1 exactly for threshold exceedance or a failed input.
	// The marker above covers failed inputs; any other non-zero exit without a
	// violation is an analyzer failure, never a silent pass.
	if !res.ok && !above {
		r.result.AddFinding(Finding{Gate: spec.gate, Rule: "pyguard.execution.analyzer_failure", Severity: "error", Status: StatusExecutionError, Category: "analyzer_failure", Evidence: "Complexipy exited without reporting a function above the threshold.", Diagnostics: refs})
	}
}
