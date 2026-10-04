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
	if r.result.Status != "fail" || len(r.result.Findings) != 2 {
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
	for _, tc := range []struct{ helper, adapter, category string }{{"missing", "mypy", "tool_missing"}, {"timeout", "mypy", "timeout"}, {"network", "mypy", "dependency_failure"}, {"spawn", "mypy", "tool_missing"}, {"invalid", "mypy", "invalid_configuration"}, {"broken", "ruff", "adapter_failure"}, {"unknown", "", "unclassified_failure"}} {
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
			if res.ok || r.result.Status != "error" || len(r.result.Findings) != 1 || r.result.Findings[0].Category != tc.category {
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
		if code != 3 || r.Status != "error" || r.Findings[0].Category != "invalid_configuration" {
			t.Fatal(code, r)
		}
	}
	os.WriteFile(filepath.Join(root, "pyproject.toml"), []byte("["), 0644)
	code, r := captureCLI(t, "mypy", []string{"--root", root, "--output", "json"})
	if code != 3 || r.Status != "error" {
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
	if r.result.Status != "error" || r.result.ExitCode != 0 || r.result.Findings[0].Category != "tool_missing" {
		t.Fatal(r.result)
	}
}
