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
	if os.Getenv("TSGUARD_CONTRACT_HELPER") == "" {
		return
	}
	switch os.Getenv("TSGUARD_CONTRACT_HELPER") {
	case "types":
		fmt.Print("src/order.ts(41,2): error TS2322: Type 'string' is not assignable to type 'number'.\n")
		fmt.Print("src/service.ts(18,3): error TS2345: Argument has the wrong type.\n")
		// Diagnostics larger than the old memory cap must not erase first findings.
		fmt.Fprint(os.Stderr, strings.Repeat("noise\n", defaultBufCap/3))
		os.Exit(2)
	case "timeout":
		time.Sleep(10 * time.Second)
	case "network":
		fmt.Fprintln(os.Stderr, "npm error code EAI_AGAIN")
		os.Exit(1)
	case "unknown":
		fmt.Fprintln(os.Stderr, "arbitrary tool dialect")
		os.Exit(1)
	}
	os.Exit(0)
}

func contractRunner(t *testing.T) *Runner {
	t.Helper()
	return &Runner{root: t.TempDir(), timeout: 2, outputMode: "json", result: newRunResult("types")}
}

func TestProcessNormalizationIgnoresTailAndKeepsCompleteDiagnostics(t *testing.T) {
	r := contractRunner(t)
	r.tailLines = 1
	t.Setenv("TSGUARD_CONTRACT_HELPER", "types")
	res := r.RunTool(toolSpec{gate: "types", adapter: "tsc"}, "compiler", os.Args[0], "-test.run=^TestAnalyzerProcess$")
	if res.ok {
		t.Fatal("blocking compiler result passed")
	}
	r.result.Finish(1)
	if r.result.Status != "fail" || len(r.result.Findings) != 2 {
		t.Fatalf("result: %+v", r.result)
	}
	rules := map[string]bool{}
	for _, f := range r.result.Findings {
		rules[f.Rule] = true
		if f.Category != "quality" || f.Location == nil || len(f.Diagnostics) != 2 {
			t.Fatalf("finding: %+v", f)
		}
	}
	if !rules["TS2322"] || !rules["TS2345"] {
		t.Fatal(rules)
	}
	data, err := os.ReadFile(filepath.Join(r.root, r.result.Diagnostics[1].Path))
	if err != nil || len(data) <= defaultBufCap {
		t.Fatalf("raw stderr: %d bytes, %v", len(data), err)
	}
	var output bytes.Buffer
	if err := reportResult(&output, "json", r.result); err != nil {
		t.Fatal(err)
	}
	var decoded map[string]json.RawMessage
	if err := json.Unmarshal(output.Bytes(), &decoded); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"schema_version", "status", "command", "exit_code", "findings", "measurements", "artifacts", "diagnostics"} {
		if _, ok := decoded[key]; !ok {
			t.Fatalf("missing %s", key)
		}
	}
	if _, ok := decoded["output"]; ok {
		t.Fatal("prose is not the JSON contract")
	}
}

func TestExecutionCategories(t *testing.T) {
	for _, tc := range []struct{ helper, category, rule string }{
		{"missing", "tool_missing", "tsguard.execution.tool_missing"},
		{"timeout", "timeout", "tsguard.execution.timeout"},
		{"network", "dependency_failure", "tsguard.execution.dependency_failure"},
		{"unknown", "unclassified_failure", "tsguard.coverage.failed"},
	} {
		t.Run(tc.helper, func(t *testing.T) {
			r := contractRunner(t)
			r.timeout = 1
			t.Setenv("TSGUARD_CONTRACT_HELPER", tc.helper)
			args := []string{os.Args[0], "-test.run=^TestAnalyzerProcess$"}
			if tc.helper == "missing" {
				args = []string{filepath.Join(r.root, "absent-tool")}
			}
			res := r.RunTool(toolSpec{gate: "coverage"}, "fixture", args...)
			if res.ok || len(r.result.Findings) != 1 {
				t.Fatalf("result: %+v", r.result)
			}
			f := r.result.Findings[0]
			if f.Category != tc.category || f.Rule != tc.rule {
				t.Fatalf("finding: %+v", f)
			}
		})
	}
}

func TestSuccessfulResultAndAdvisoryExit(t *testing.T) {
	r := contractRunner(t)
	t.Setenv("TSGUARD_CONTRACT_HELPER", "success")
	if !r.RunTool(toolSpec{gate: "types", adapter: "tsc"}, "compiler", os.Args[0], "-test.run=^TestAnalyzerProcess$").ok {
		t.Fatal("success failed")
	}
	r.result.Finish(0)
	if r.result.Status != "pass" || len(r.result.Findings) != 0 {
		t.Fatal(r.result)
	}
	t.Setenv("TSGUARD_CONTRACT_HELPER", "unknown")
	r.RunTool(toolSpec{gate: "dead-code", advisory: true}, "knip", os.Args[0], "-test.run=^TestAnalyzerProcess$")
	r.result.Finish(0)
	if r.result.Status != "advisory" || r.result.ExitCode != 0 {
		t.Fatal(r.result)
	}
}

func TestAgentReporterBoundAndStableIDs(t *testing.T) {
	a, b := newRunResult("types"), newRunResult("types")
	for i := 0; i < 1000; i++ {
		f := Finding{Gate: "types", Rule: "TS2322", Severity: "error", Status: "blocking", Category: "quality",
			Location: &Location{File: strings.Repeat("file", 200), Line: i + 1}, Evidence: strings.Repeat("é\n\x1b", 1000)}
		a.AddFinding(f)
		f.Evidence = "Message changed by an analyzer update"
		b.AddFinding(f)
	}
	a.Finish(1)
	b.Finish(1)
	for i := range a.Findings {
		if a.Findings[i].ID != b.Findings[i].ID {
			t.Fatal("ID depends on prose")
		}
	}
	var out bytes.Buffer
	if err := reportResult(&out, "agent", a); err != nil {
		t.Fatal(err)
	}
	if out.Len() > 6*1024 || strings.Count(out.String(), "\n") > 26 {
		t.Fatalf("unbounded agent result: %d bytes", out.Len())
	}
	if !strings.Contains(out.String(), "omitted: 990") || strings.Contains(out.String(), "\x1b") {
		t.Fatal("unsafe or missing reduction")
	}
}

func TestAdapters(t *testing.T) {
	for _, tc := range []struct {
		name, adapter, stdout, raw, rule, category string
		ok                                         bool
	}{
		{"FTA failure", "fta", "", "File order.ts has a score of 77.5, which is beyond the score cap of 60, exiting.\n", "tsguard.fta.score_exceeded", "quality", false},
		{"Biome rule", "biome", `{"diagnostics":[{"category":"lint/suspicious/noExplicitAny","severity":"error","message":"Use a specific type.","location":{"path":"src/order.ts","start":{"line":3,"column":4}}}]}`, "", "lint/suspicious/noExplicitAny", "quality", false},
		{"Opengrep rule", "opengrep", `{"results":[{"check_id":"rules.eval","path":"src/order.ts","start":{"line":3,"col":5},"extra":{"message":"Avoid eval","severity":"WARNING"}}],"errors":[]}`, "", "rules.eval", "quality", false},
		{"Malformed native JSON", "biome", "broken", "", "tsguard.execution.adapter_failure", "adapter_failure", false},
		{"Absent schema", "opengrep", "{}", "", "tsguard.execution.adapter_failure", "adapter_failure", true},
		{"TypeScript missing import is quality", "tsc", "src/order.ts(1,1): error TS2307: Cannot find module 'EAI_AGAIN'.\n", "src/order.ts(1,1): error TS2307: Cannot find module 'EAI_AGAIN'.\n", "TS2307", "quality", false},
		{"TypeScript config", "tsc", "error TS5023: Unknown compiler option.\n", "", "TS5023", "invalid_configuration", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := contractRunner(t)
			r.normalize(toolSpec{gate: "test", adapter: tc.adapter, subject: "src", threshold: 60}, Result{ok: tc.ok, output: tc.raw}, strings.NewReader(tc.stdout), []string{"diagnostic-001"})
			if len(r.result.Findings) != 1 {
				t.Fatalf("findings: %+v", r.result.Findings)
			}
			f := r.result.Findings[0]
			if f.Rule != tc.rule || f.Category != tc.category {
				t.Fatalf("finding: %+v", f)
			}
			if tc.adapter == "fta" && (f.Location.File != "src/order.ts" || *f.Observed != 77.5 || *f.Threshold != 60) {
				t.Fatal(f)
			}
			if tc.adapter == "biome" && tc.category == "quality" && f.Location.Line != 3 {
				t.Fatal("Biome line must be one-based")
			}
			if tc.name == "Opengrep rule" && (f.Severity != "warning" || f.Status != "blocking") {
				t.Fatal("Opengrep severity and gate policy differ", f)
			}
		})
	}
	r := contractRunner(t)
	r.normalize(toolSpec{gate: "fta", adapter: "fta", subject: "src", threshold: 60}, Result{ok: true}, strings.NewReader(`[{"file_name":"order.ts","fta_score":22.5}]`), nil)
	if len(r.result.Measurements) != 1 || len(r.result.Findings) != 0 {
		t.Fatal(r.result)
	}
}

func TestStrictFlags(t *testing.T) {
	for _, args := range [][]string{
		{"--timout", "30"}, {"--timeout"}, {"--timeout", "--tail", "1"}, {"--timeout", "30s"}, {"--timeout", "0"},
		{"--tail", "-1"}, {"--output", "xml"}, {"--output"}, {"--root"}, {"--dirs", "src,"}, {"unexpected"},
	} {
		if _, err := parseFlags(args); err == nil {
			t.Fatalf("accepted %v", args)
		}
	}
	cfg, err := parseFlags([]string{"--output", "json", "--root", "project with spaces", "--tail", "0", "--timeout", "30"})
	if err != nil || cfg.output != "json" || cfg.root != "project with spaces" || cfg.timeout != 30 {
		t.Fatalf("%+v, %v", cfg, err)
	}
}

func TestHumanPipeRefusalIsPreserved(t *testing.T) {
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer read.Close()
	defer write.Close()
	old := os.Stdout
	os.Stdout = write
	defer func() { os.Stdout = old }()
	result := newRunResult("check")
	code := dispatchResult("check", config{output: "human"}, t.TempDir(), result)
	if code != 5 || len(result.Findings) != 1 || result.Findings[0].Category != "pipe_refused" {
		t.Fatal(code, result)
	}
}

func captureCLI(t *testing.T, command string, args []string) (int, *RunResult) {
	t.Helper()
	file, err := os.CreateTemp(t.TempDir(), "stdout")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	old := os.Stdout
	os.Stdout = file
	defer func() { os.Stdout = old }()
	code := runCLI(command, args)
	file.Seek(0, 0)
	data, _ := io.ReadAll(file)
	var result RunResult
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("JSON: %s: %v", data, err)
	}
	return code, &result
}

func TestCLIInvalidInputsAndLock(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "package.json"), []byte("{}"), 0o644)
	for _, args := range [][]string{{"--timout", "30"}, {"--timeout"}, {"--output", "xml"}} {
		args = append(args, "--output", "json", "--root", root)
		code, result := captureCLI(t, "check", args)
		if code != 3 || result.Status != "error" || result.Findings[0].Category != "invalid_configuration" {
			t.Fatal(code, result)
		}
	}
	os.WriteFile(filepath.Join(root, "oxguard.toml"), []byte("dirs = ["), 0o644)
	code, result := captureCLI(t, "types", []string{"--output", "json", "--root", root})
	if code != 3 || result.Findings[0].Category != "invalid_configuration" {
		t.Fatal(code, result)
	}
	os.Remove(filepath.Join(root, "oxguard.toml"))
	release, err := acquireLock(root)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	code, result = captureCLI(t, "types", []string{"--output", "json", "--root", root})
	if code != 4 || result.Status != "error" || result.Findings[0].Category != "lock_contention" {
		t.Fatal(code, result)
	}
}
