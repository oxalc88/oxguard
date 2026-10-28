package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAnalyzerProcess(t *testing.T) {
	if os.Getenv("PYGUARD_CONTRACT_HELPER") == "" {
		return
	}
	switch os.Getenv("PYGUARD_CONTRACT_HELPER") {
	case "types":
		fmt.Println(`{"file":"src/main.py","line":1,"column":0,"message":"Wrong type","code":"assignment","severity":"error"}`)
		fmt.Println(`{"file":"src/main.py","line":2,"column":0,"message":"Wrong type","code":"assignment","severity":"error"}`)
		fmt.Fprint(os.Stderr, strings.Repeat("noise\n", defaultBufCap/3))
		os.Exit(1)
	case "timeout":
		time.Sleep(10 * time.Second)
	case "network":
		fmt.Fprintln(os.Stderr, "error: Failed to download dependency")
		os.Exit(1)
	case "spawn":
		fmt.Fprintln(os.Stderr, "error: Failed to spawn: mypy")
		os.Exit(2)
	case "invalid":
		fmt.Fprintln(os.Stderr, "invalid config")
		os.Exit(2)
	case "mypy_exit2":
		// mypy 2.x prints native JSON lines even when it aborts with exit 2.
		// Like type diagnostics, parser syntax columns are zero-based.
		fmt.Println(`{"file":"src/main.py","line":1,"column":7,"end_line":1,"end_column":8,"message":"Expected a parameter","hint":null,"code":"syntax","severity":"error"}`)
		os.Exit(2)
	case "mypy_exit2_prose":
		// Exit 2 with nothing structured on stdout is still a configuration failure.
		fmt.Println("Found 1 error in 1 file")
		os.Exit(2)
	case "unknown":
		fmt.Fprintln(os.Stderr, "tool dialect")
		os.Exit(1)
	case "broken":
		fmt.Print("broken")
	}
	os.Exit(0)
}
func contractRunner(t *testing.T) *Runner {
	t.Helper()
	return &Runner{root: t.TempDir(), timeout: 2, outputMode: "json", result: newRunResult("mypy")}
}
func TestProcessContract(t *testing.T) {
	r := contractRunner(t)
	r.tailLines = 1
	t.Setenv("PYGUARD_CONTRACT_HELPER", "types")
	if r.RunTool(toolSpec{gate: "mypy", adapter: "mypy"}, "mypy", os.Args[0], "-test.run=^TestAnalyzerProcess$").ok {
		t.Fatal("blocking run passed")
	}
	r.result.Finish(1)
	if r.result.Status != StatusFail || len(r.result.Findings) != 2 {
		t.Fatal(r.result)
	}
	for _, f := range r.result.Findings {
		if f.Rule != "assignment" || f.Location.Column != 1 || f.Location.File != "src/main.py" || f.Category != "quality" || len(f.Diagnostics) != 2 {
			t.Fatal(f)
		}
	}
	raw, err := os.ReadFile(filepath.Join(r.root, r.result.Diagnostics[1].Path))
	if err != nil || len(raw) <= defaultBufCap {
		t.Fatal(len(raw), err)
	}
	var out bytes.Buffer
	if err := reportResult(&out, "json", r.result); err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(out.Bytes(), &fields); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"schema_version", "status", "command", "exit_code", "findings", "measurements", "artifacts", "diagnostics"} {
		if _, ok := fields[key]; !ok {
			t.Fatal(key)
		}
	}
	if _, ok := fields["output"]; ok {
		t.Fatal("prose contract")
	}
}
func TestExecutionCategories(t *testing.T) {
	for _, tc := range []struct{ helper, adapter, category string }{{"missing", "mypy", "tool_missing"}, {"timeout", "mypy", "timeout"}, {"network", "mypy", "dependency_failure"}, {"spawn", "mypy", "tool_missing"}, {"invalid", "mypy", "invalid_configuration"}, {"mypy_exit2_prose", "mypy", "invalid_configuration"}, {"broken", "ruff", "adapter_failure"}, {"unknown", "", "unclassified_failure"}} {
		t.Run(tc.helper, func(t *testing.T) {
			r := contractRunner(t)
			r.timeout = 1
			t.Setenv("PYGUARD_CONTRACT_HELPER", tc.helper)
			args := []string{os.Args[0], "-test.run=^TestAnalyzerProcess$"}
			if tc.helper == "missing" {
				args = []string{filepath.Join(r.root, "absent")}
			}
			res := r.RunTool(toolSpec{gate: "mypy", adapter: tc.adapter}, "fixture", args...)
			r.result.Finish(1)
			if res.ok || r.result.Status != StatusError || len(r.result.Findings) != 1 || r.result.Findings[0].Category != tc.category {
				t.Fatal(res, r.result)
			}
		})
	}
}
func TestAdapters(t *testing.T) {
	for _, tc := range []struct {
		adapter, data, rule string
		value               float64
	}{
		{"ruff", `[{"code":"F401","message":"Unused import","filename":"src/main.py","location":{"row":1,"column":1}}]`, "F401", 0},
		{"radon_cc", `{"src/main.py":[{"type":"function","name":"f","lineno":1,"complexity":12}]}`, "pyguard.radon.cc_exceeded", 12},
		{"radon_mi", `{"src/main.py":{"mi":19,"rank":"B"}}`, "pyguard.radon.mi_threshold_failed", 19},
		{"owned", `{"schema_version":"1","findings":[{"rule":"pyguard.types.annotation_complexity","severity":"error","location":{"file":"src/main.py","line":1},"observed":3,"threshold":2,"evidence":"nested"}],"measurements":[],"artifacts":[]}`, "pyguard.types.annotation_complexity", 3},
	} {
		t.Run(tc.adapter, func(t *testing.T) {
			r := contractRunner(t)
			r.normalize(toolSpec{gate: "test", adapter: tc.adapter}, Result{ok: true}, strings.NewReader(tc.data), nil)
			if len(r.result.Findings) != 1 {
				t.Fatal(r.result)
			}
			f := r.result.Findings[0]
			if f.Rule != tc.rule || f.Location.File != "src/main.py" || f.Category != "quality" {
				t.Fatal(f)
			}
			if tc.value != 0 && (f.Observed == nil || *f.Observed != tc.value) {
				t.Fatal(f)
			}
		})
	}
	r := contractRunner(t)
	r.normalize(toolSpec{gate: "radon", adapter: "radon_cc"}, Result{ok: true}, strings.NewReader(`{"src/main.py":[{"name":"boundary","lineno":1,"complexity":10}]}`), nil)
	if len(r.result.Findings) != 0 || len(r.result.Measurements) != 1 {
		t.Fatal(r.result)
	}
	r.normalize(toolSpec{gate: "radon", adapter: "radon_mi"}, Result{ok: true}, strings.NewReader(`{"src/main.py":{"mi":19.5,"rank":"A"}}`), nil)
	if len(r.result.Findings) != 0 {
		t.Fatal("MI boundary changed", r.result)
	}
}

// mypy 2.x --output json adds end_line/end_column keys that mypy 1.x omitted
// (1.18.2 already emitted hint); the adapter keeps decoding the shared fields
// and drops notes.
func TestMypyAdapterReadsMypy2JSONLines(t *testing.T) {
	r := contractRunner(t)
	data := strings.Join([]string{
		`{"file":"src/main.py","line":1,"column":14,"end_line":1,"end_column":21,"message":"Incompatible types in assignment","hint":null,"code":"assignment","severity":"error"}`,
		`{"file":"src/main.py","line":3,"column":0,"end_line":4,"end_column":1,"message":"Inferred type is int","hint":null,"code":"assignment","severity":"note"}`,
		`{"file":"src/main.py","line":5,"column":7,"end_line":5,"end_column":12,"message":"Missing return statement","hint":"Add a return","code":"func-returns-value","severity":"error"}`,
	}, "\n")
	r.normalize(toolSpec{gate: "mypy", adapter: "mypy"}, Result{ok: true}, strings.NewReader(data), nil)
	if len(r.result.Findings) != 2 {
		t.Fatal(r.result.Findings)
	}
	want := []struct {
		rule         string
		line, column int
	}{
		{"assignment", 1, 15},
		{"func-returns-value", 5, 8},
	}
	for i, f := range r.result.Findings {
		if f.Gate != "mypy" || f.Rule != want[i].rule || f.Severity != "error" || f.Status != StatusBlocking || f.Category != "quality" {
			t.Fatal(f)
		}
		if f.Location == nil || f.Location.File != "src/main.py" || f.Location.Line != want[i].line || f.Location.Column != want[i].column {
			t.Fatal(f.Location)
		}
	}
}

// A mypy 2.x syntax abort still emits native JSON lines: they stay source
// findings and the aborted check is an execution error, so it never passes.
func TestMypyExit2KeepsSyntaxFindingsAndFailsTheGate(t *testing.T) {
	r := contractRunner(t)
	t.Setenv("PYGUARD_CONTRACT_HELPER", "mypy_exit2")
	if r.RunTool(toolSpec{gate: "mypy", adapter: "mypy"}, "mypy", os.Args[0], "-test.run=^TestAnalyzerProcess$").ok {
		t.Fatal("aborted type check reported success")
	}
	if err := r.result.Finish(1); err != nil {
		t.Fatal(err)
	}
	if r.result.Status != StatusError || r.result.Assessment != Incomplete || len(r.result.Gates) != 1 || r.result.Gates[0].Status != StatusError || r.result.Gates[0].Normalization != Complete {
		t.Fatal(r.result.Status, r.result.Assessment, r.result.Gates)
	}
	if len(r.result.Findings) != 2 {
		t.Fatal(r.result.Findings)
	}
	aborted, syntax := r.result.Findings[0], r.result.Findings[1]
	if aborted.Rule != "pyguard.execution.analyzer_failure" || aborted.Status != StatusExecutionError || aborted.Category != "analyzer_failure" || aborted.Gate != "mypy" {
		t.Fatal(aborted)
	}
	if syntax.Rule != "pyguard.mypy.syntax" || syntax.Status != StatusBlocking || syntax.Category != "quality" || syntax.Severity != "error" || syntax.Gate != "mypy" {
		t.Fatal(syntax)
	}
	if syntax.Location == nil || syntax.Location.File != "src/main.py" || syntax.Location.Line != 1 || syntax.Location.Column != 8 {
		t.Fatal(syntax.Location)
	}
	if syntax.Evidence != "Expected a parameter" || len(syntax.Diagnostics) != 2 {
		t.Fatal(syntax)
	}
}

// Exit 2 without structured stdout keeps the configuration-failure contract.
func TestMypyExit2WithoutStructuredFindingsIsInvalidConfiguration(t *testing.T) {
	for _, stdout := range []string{"", "Found 1 error in 1 file\n", `{"file":"src/main.py"}`} {
		r := contractRunner(t)
		r.normalize(toolSpec{gate: "mypy", adapter: "mypy"}, Result{exitCode: 2}, strings.NewReader(stdout), nil)
		if err := r.result.Finish(1); err != nil {
			t.Fatal(stdout, err)
		}
		if r.result.Status != StatusError || len(r.result.Findings) != 1 || r.result.Findings[0].Category != "invalid_configuration" {
			t.Fatal(stdout, r.result.Findings)
		}
	}
}

// Non-JSON analyzer stdout is malformed structured output, never a silent pass.
func TestMypyAdapterRejectsProseStdout(t *testing.T) {
	r := contractRunner(t)
	prose := "src/main.py:1: error: Invalid syntax  [syntax]\nFound 1 error in 1 file\n"
	r.normalize(toolSpec{gate: "mypy", adapter: "mypy"}, Result{ok: true}, strings.NewReader(prose), nil)
	if len(r.result.Findings) != 1 || r.result.Findings[0].Category != "adapter_failure" || r.result.Findings[0].Status != StatusExecutionError {
		t.Fatal(r.result.Findings)
	}
}

func TestStrictFlags(t *testing.T) {
	for _, args := range [][]string{{"--timout", "30"}, {"--timeout"}, {"--timeout", "--tail", "1"}, {"--timeout", "30s"}, {"--timeout", "0"}, {"--tail", "-1"}, {"--output", "xml"}, {"--output"}, {"--root"}, {"--dirs", "src,"}, {"unexpected"}} {
		if _, err := parseFlags(args); err == nil {
			t.Fatal(args)
		}
	}
	cfg, err := parseFlags([]string{"--output", "json", "--root", "project with spaces", "--timeout", "30"})
	if err != nil || cfg.root != "project with spaces" || cfg.output != "json" || cfg.timeout != 30 {
		t.Fatal(cfg, err)
	}
}
func captureCLI(t *testing.T, command string, args []string) (int, *RunResult) {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "stdout")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	old := os.Stdout
	os.Stdout = f
	defer func() { os.Stdout = old }()
	code := runCLI(command, args)
	f.Seek(0, 0)
	data, _ := io.ReadAll(f)
	var result RunResult
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("%s: %v", data, err)
	}
	return code, &result
}
func TestCLIInputsAndLock(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "pyproject.toml"), []byte("[project]\nname='fixture'\nversion='0.0.0'\n"), 0644)
	for _, args := range [][]string{{"--timout", "30"}, {"--timeout"}, {"--output", "xml"}} {
		code, r := captureCLI(t, "check", append(args, "--output", "json", "--root", root))
		if code != 3 || r.Status != StatusError || r.Findings[0].Category != "invalid_configuration" {
			t.Fatal(code, r)
		}
	}
	os.WriteFile(filepath.Join(root, "pyproject.toml"), []byte("["), 0644)
	code, r := captureCLI(t, "mypy", []string{"--root", root, "--output", "json"})
	if code != 3 || r.Status != StatusError {
		t.Fatal(code, r)
	}
	os.WriteFile(filepath.Join(root, "pyproject.toml"), []byte("[project]\nname='fixture'\nversion='0.0.0'\n"), 0644)
	release, err := acquireLock(root)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	code, r = captureCLI(t, "mypy", []string{"--root", root, "--output", "json"})
	if code != 4 || r.Findings[0].Category != "lock_contention" {
		t.Fatal(code, r)
	}
}
func TestAdvisoryAndMissingHelper(t *testing.T) {
	r := contractRunner(t)
	r.RunTool(toolSpec{gate: "criticality", adapter: "owned", advisory: true}, "criticality", "uv", "run", "python", "tools/analysis/analyze_criticality.py", "--json")
	r.result.Finish(0)
	if r.result.Status != StatusError || r.result.ExitCode != 0 || r.result.Findings[0].Category != "tool_missing" {
		t.Fatal(r.result)
	}
}

// pyan3 2.x drops files it cannot compile instead of failing the run, and
// analyze_criticality reports that drop as an owned error payload. The
// advisory criticality gate must surface it as an explicit execution error,
// with an incomplete assessment and no registered CRITICALITY.md artifact —
// a partial call graph is never a quiet pass.
func TestCriticalitySkippedSourceFailsExplicitly(t *testing.T) {
	r := contractRunner(t)
	payload := `{"schema_version":"1","findings":[],"measurements":[],"artifacts":[],"error":{"category":"adapter_failure","message":"Criticality analysis failed: unparsable source skipped by pyan3: src/main.py"}}`
	r.normalize(toolSpec{gate: "criticality", adapter: "owned", advisory: true}, Result{ok: false, exitCode: 1}, strings.NewReader(payload), nil)
	if err := r.result.Finish(0); err != nil {
		t.Fatal(err)
	}
	if r.result.Status != StatusError || r.result.ExitCode != 0 || r.result.Assessment != Incomplete {
		t.Fatal(r.result.Status, r.result.ExitCode, r.result.Assessment)
	}
	if len(r.result.Gates) != 1 || r.result.Gates[0].Status != StatusError || r.result.Gates[0].Normalization != Complete {
		t.Fatal(r.result.Gates)
	}
	if len(r.result.Artifacts) != 0 {
		t.Fatal("analysis failure must not register a criticality artifact", r.result.Artifacts)
	}
	if len(r.result.Findings) != 1 {
		t.Fatal(r.result.Findings)
	}
	f := r.result.Findings[0]
	if f.Gate != "criticality" || f.Rule != "pyguard.execution.adapter_failure" || f.Category != "adapter_failure" || f.Status != StatusExecutionError {
		t.Fatal(f)
	}
}

func TestOwnedFindingStatusCannotBeMaskedByAdvisoryPolicy(t *testing.T) {
	for _, status := range []string{"unknown", "pass", "execution_error"} {
		r := contractRunner(t)
		payload := `{"schema_version":"1","findings":[{"status":"` + status + `","rule":"example"}],"measurements":[],"artifacts":[]}`
		r.normalize(toolSpec{gate: "test", adapter: "owned", advisory: true}, Result{ok: true}, strings.NewReader(payload), nil)
		if len(r.result.Findings) != 1 || r.result.Findings[0].Status != StatusExecutionError {
			t.Fatalf("%s status hidden", status)
		}
	}
}
