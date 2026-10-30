package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const structureCycleReport = `{
  "schema_version": "1",
  "modules": [
    {"name": "pkg", "file": "src/pkg/__init__.py", "depth": 0},
    {"name": "pkg.a", "file": "src/pkg/a.py", "depth": 0},
    {"name": "pkg.b", "file": "src/pkg/b.py", "depth": 0},
    {"name": "pkg.c", "file": "src/pkg/c.py", "depth": 2}
  ],
  "imports": [
    {"importer": "pkg.a", "imported": "pkg.b"},
    {"importer": "pkg.b", "imported": "pkg.a"}
  ],
  "components": [
    {"modules": ["pkg"], "cyclic": false},
    {"modules": ["pkg.a", "pkg.b"], "cyclic": true},
    {"modules": ["pkg.c"], "cyclic": false}
  ],
  "counts": {"modules": 4, "edges": 2, "cycles": 1, "type_imports": 2, "deferred_imports": 1},
  "functions": [],
  "calls": [],
  "function_components": [],
  "function_counts": {"functions": 0, "calls": 0, "recursive_components": 0, "unresolved_calls": 0},
  "partial": false,
  "limitation": ""
}`

const structureSelfLoopReport = `{
  "schema_version": "1",
  "modules": [{"name": "pkg.a", "file": "src/pkg/a.py", "depth": 0}],
  "imports": [{"importer": "pkg.a", "imported": "pkg.a"}],
  "components": [{"modules": ["pkg.a"], "cyclic": true}],
  "counts": {"modules": 1, "edges": 1, "cycles": 1, "type_imports": 0, "deferred_imports": 0},
  "functions": [],
  "calls": [],
  "function_components": [],
  "function_counts": {"functions": 0, "calls": 0, "recursive_components": 0, "unresolved_calls": 0},
  "partial": false,
  "limitation": ""
}`

const structurePartialReport = `{
  "schema_version": "1",
  "modules": [{"name": "pkg.a", "file": "src/pkg/a.py", "depth": 0}],
  "imports": [],
  "components": [{"modules": ["pkg.a"], "cyclic": false}],
  "counts": {"modules": 1, "edges": 0, "cycles": 0, "type_imports": 0, "deferred_imports": 0},
  "functions": [],
  "calls": [],
  "function_components": [],
  "function_counts": {"functions": 0, "calls": 0, "recursive_components": 0, "unresolved_calls": 0},
  "partial": true,
  "limitation": "Unresolved local import: pkg.missing (src/pkg/a.py:1)"
}`

func structureSpec() toolSpec {
	return toolSpec{gate: "structure", adapter: "structure", advisory: true}
}

func structureMeasurement(result *RunResult, metric, file string) *Measurement {
	for i := range result.Measurements {
		measurement := &result.Measurements[i]
		if measurement.Metric == metric && measurement.Location.File == file {
			return measurement
		}
	}
	return nil
}

func TestStructureAdapterReportsAdvisoryCycleAndDepth(t *testing.T) {
	r := contractRunner(t)
	r.normalize(structureSpec(), Result{ok: true}, strings.NewReader(structureCycleReport), nil)
	if err := r.result.Finish(0); err != nil {
		t.Fatal(err)
	}
	if r.result.ExitCode != 0 || r.result.Status != StatusAdvisory || r.result.Assessment != Complete {
		t.Fatalf("advisory cycle changed result: exit=%d status=%s assessment=%s", r.result.ExitCode, r.result.Status, r.result.Assessment)
	}
	if len(r.result.Gates) != 1 || r.result.Gates[0].Status != StatusAdvisory || r.result.Gates[0].Normalization != Complete {
		t.Fatalf("gate: %+v", r.result.Gates)
	}
	if len(r.result.Findings) != 1 {
		t.Fatalf("findings: %+v", r.result.Findings)
	}
	f := r.result.Findings[0]
	if f.Rule != "pyguard.structure.CIRCULAR_DEPENDENCY" || f.Level != "structure" || f.Severity != "warning" || f.Status != StatusAdvisory || f.Category != "quality" || f.Gate != "structure" {
		t.Fatalf("finding contract: %+v", f)
	}
	if f.Location == nil || f.Location.File != "src/pkg/a.py" || len(f.Related) != 1 || f.Related[0].File != "src/pkg/b.py" {
		t.Fatalf("finding locations: %+v", f)
	}
	for metric, want := range map[string]float64{
		"module.count": 4, "module.edges": 2, "module.cycles": 1, "module.type_imports": 2, "module.deferred_imports": 1,
	} {
		measurement := structureMeasurement(r.result, metric, ".")
		if measurement == nil || measurement.Value != want || measurement.Level != "structure" {
			t.Fatalf("%s measurement: %+v", metric, measurement)
		}
	}
	for file, want := range map[string]float64{"src/pkg/a.py": 0, "src/pkg/b.py": 0, "src/pkg/c.py": 2, "src/pkg/__init__.py": 0} {
		measurement := structureMeasurement(r.result, "module.dependency_depth", file)
		if measurement == nil || measurement.Value != want || measurement.Unit != "edges" {
			t.Fatalf("depth for %s: %+v", file, measurement)
		}
	}
}

func TestStructureAdapterSelfLoopIsCycle(t *testing.T) {
	r := contractRunner(t)
	r.normalize(structureSpec(), Result{ok: true}, strings.NewReader(structureSelfLoopReport), nil)
	if err := r.result.Finish(0); err != nil {
		t.Fatal(err)
	}
	if len(r.result.Findings) != 1 || r.result.Findings[0].Rule != "pyguard.structure.CIRCULAR_DEPENDENCY" {
		t.Fatalf("self-loop finding: %+v", r.result.Findings)
	}
	if len(r.result.Findings[0].Related) != 0 {
		t.Fatalf("self-loop has related modules: %+v", r.result.Findings[0])
	}
	if r.result.Status != StatusAdvisory || r.result.ExitCode != 0 {
		t.Fatalf("self-loop changed policy: %+v", r.result)
	}
}

func TestStructureAdapterPartialIsAdvisoryNotEvaluated(t *testing.T) {
	r := contractRunner(t)
	r.normalize(structureSpec(), Result{ok: true}, strings.NewReader(structurePartialReport), nil)
	if err := r.result.Finish(0); err != nil {
		t.Fatal(err)
	}
	if r.result.ExitCode != 0 || r.result.Status != StatusAdvisory || r.result.Assessment != Incomplete {
		t.Fatalf("partial result: exit=%d status=%s assessment=%s", r.result.ExitCode, r.result.Status, r.result.Assessment)
	}
	if len(r.result.Gates) != 1 || r.result.Gates[0].Status != StatusAdvisory || r.result.Gates[0].Normalization != Partial {
		t.Fatalf("partial gate: %+v", r.result.Gates)
	}
	if len(r.result.Findings) != 1 {
		t.Fatalf("partial findings: %+v", r.result.Findings)
	}
	f := r.result.Findings[0]
	if f.Rule != "pyguard.structure.not_evaluated" || f.Status != StatusAdvisory || f.Severity != "info" || f.Category != "quality" || !strings.Contains(f.Evidence, "pkg.missing") {
		t.Fatalf("not_evaluated finding: %+v", f)
	}
}

func TestStructureAdapterRejectsMalformedReports(t *testing.T) {
	cases := []string{
		"",
		"{}",
		"not json",
		`{"schema_version":"1","modules":null,"imports":[],"components":[]}`,
		`{"schema_version":"1","modules":[{"name":"a","file":"a.py","depth":0}],"imports":[{"importer":"a","imported":"missing"}],"components":[{"modules":["a"],"cyclic":false}],"counts":{"modules":1,"edges":1,"cycles":0}}`,
		`{"schema_version":"1","modules":[{"name":"a","file":"a.py","depth":0}],"imports":[],"components":[{"modules":["a"],"cyclic":true}],"counts":{"modules":1,"edges":0,"cycles":1}}`,
	}
	for _, data := range cases {
		r := contractRunner(t)
		r.normalize(structureSpec(), Result{ok: true}, strings.NewReader(data), nil)
		if len(r.result.Findings) != 1 || r.result.Findings[0].Status != StatusExecutionError || r.result.Findings[0].Category != "adapter_failure" {
			t.Fatalf("accepted malformed report %q: %+v", data, r.result.Findings)
		}
	}
}

func TestStructureAdapterNonZeroExitIsExecutionFailure(t *testing.T) {
	r := contractRunner(t)
	r.normalize(structureSpec(), Result{ok: false, exitCode: 1}, strings.NewReader(structureCycleReport), nil)
	if err := r.result.Finish(1); err != nil {
		t.Fatal(err)
	}
	if r.result.Status != StatusError || r.result.Assessment != Incomplete {
		t.Fatalf("failed run: %+v", r.result)
	}
	found := false
	for _, finding := range r.result.Findings {
		if finding.Status == StatusExecutionError && finding.Category == "analyzer_failure" {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing analyzer failure: %+v", r.result.Findings)
	}
}

func TestStructureAdapterReportsMissingToolCategory(t *testing.T) {
	r := contractRunner(t)
	payload := `{"schema_version":"1","modules":[],"imports":[],"components":[],"counts":{},"partial":false,"limitation":"","error":{"category":"tool_missing","message":"grimp is not installed."}}`
	r.normalize(structureSpec(), Result{ok: false, exitCode: 1}, strings.NewReader(payload), nil)
	if err := r.result.Finish(1); err != nil {
		t.Fatal(err)
	}
	if len(r.result.Findings) != 1 || r.result.Findings[0].Status != StatusExecutionError || r.result.Findings[0].Category != "tool_missing" {
		t.Fatalf("tool_missing: %+v", r.result.Findings)
	}
}

func TestStructureAdapterNormalizesModulePaths(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "src", "pkg", "a.py")
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte("import pkg.a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cases := []struct{ name, path string }{{"absolute", file}}
	if runtime.GOOS == "windows" {
		cases = append(cases,
			struct{ name, path string }{"extended backslash", `\\?\` + file},
			struct{ name, path string }{"extended slash", "//?/" + filepath.ToSlash(file)},
		)
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := &Runner{root: root, timeout: 2, outputMode: "json", result: newRunResult("structure")}
			payload := fmt.Sprintf(`{"schema_version":"1","modules":[{"name":"pkg.a","file":%q,"depth":0}],"imports":[{"importer":"pkg.a","imported":"pkg.a"}],"components":[{"modules":["pkg.a"],"cyclic":true}],"counts":{"modules":1,"edges":1,"cycles":1,"type_imports":0,"deferred_imports":0},"functions":[],"calls":[],"function_components":[],"function_counts":{"functions":0,"calls":0,"recursive_components":0,"unresolved_calls":0},"partial":false,"limitation":""}`, tc.path)
			r.normalize(structureSpec(), Result{ok: true}, strings.NewReader(payload), nil)
			measurement := structureMeasurement(r.result, "module.dependency_depth", "src/pkg/a.py")
			if measurement == nil {
				t.Fatalf("measurement path not normalized: %+v", r.result.Measurements)
			}
			if len(r.result.Findings) != 1 || r.result.Findings[0].Location == nil || r.result.Findings[0].Location.File != "src/pkg/a.py" {
				t.Fatalf("finding path not normalized: %+v", r.result.Findings)
			}
		})
	}
}

// structureCouplingReport builds a valid acyclic graph with a hub that is
// imported by callers and imports dependencies. A self-import is optional so
// its effect on fan counts can be isolated from the cycle it still creates.
func structureCouplingReport(t *testing.T, importers, dependencies int, selfImport bool) string {
	t.Helper()
	modules := []structureModule{{Name: "hub", File: "hub.py"}}
	imports := []structureImport{}
	for i := 0; i < importers; i++ {
		name := fmt.Sprintf("caller%d", i)
		modules = append(modules, structureModule{Name: name, File: name + ".py"})
		imports = append(imports, structureImport{Importer: name, Imported: "hub"})
	}
	for i := 0; i < dependencies; i++ {
		name := fmt.Sprintf("dep%d", i)
		modules = append(modules, structureModule{Name: name, File: name + ".py"})
		imports = append(imports, structureImport{Importer: "hub", Imported: name})
	}
	if selfImport {
		imports = append(imports, structureImport{Importer: "hub", Imported: "hub"})
	}
	components := make([]structureComponent, len(modules))
	for i, module := range modules {
		components[i] = structureComponent{Modules: []string{module.Name}, Cyclic: selfImport && module.Name == "hub"}
	}
	report := structureReport{
		SchemaVersion:      "1",
		Modules:            modules,
		Imports:            imports,
		Components:         components,
		Counts:             structureCounts{Modules: len(modules), Edges: len(imports)},
		Functions:          []structureFunction{},
		Calls:              []structureCall{},
		FunctionComponents: []structureFunctionComponent{},
		FunctionCounts:     structureFunctionCounts{},
	}
	if selfImport {
		report.Counts.Cycles = 1
	}
	data, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func couplingFindings(result *RunResult) []Finding {
	findings := []Finding{}
	for _, finding := range result.Findings {
		if finding.Rule == "pyguard.structure.HIGH_MODULE_COUPLING" {
			findings = append(findings, finding)
		}
	}
	return findings
}

func TestStructureAdapterCouplingThresholds(t *testing.T) {
	cases := []struct {
		name       string
		importers  int
		deps       int
		high       bool
		wantStatus Status
		wantGate   Status
	}{
		{"at threshold", 3, 8, true, StatusAdvisory, StatusAdvisory},
		{"too few importers", 2, 8, false, StatusPass, StatusPassed},
		{"too few dependencies", 3, 7, false, StatusPass, StatusPassed},
		{"below both", 2, 7, false, StatusPass, StatusPassed},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := contractRunner(t)
			r.normalize(structureSpec(), Result{ok: true}, strings.NewReader(structureCouplingReport(t, tc.importers, tc.deps, false)), nil)
			if err := r.result.Finish(0); err != nil {
				t.Fatal(err)
			}
			if r.result.ExitCode != 0 || r.result.Status != tc.wantStatus || r.result.Assessment != Complete {
				t.Fatalf("coupling changed policy: exit=%d status=%s assessment=%s", r.result.ExitCode, r.result.Status, r.result.Assessment)
			}
			if len(r.result.Gates) != 1 || r.result.Gates[0].Status != tc.wantGate || r.result.Gates[0].Normalization != Complete {
				t.Fatalf("gate: %+v", r.result.Gates)
			}
			fanIn := structureMeasurement(r.result, "module.fan_in", "hub.py")
			fanOut := structureMeasurement(r.result, "module.fan_out", "hub.py")
			if fanIn == nil || fanIn.Value != float64(tc.importers) || fanIn.Unit != "modules" || fanIn.Level != "structure" {
				t.Fatalf("hub fan_in: %+v", fanIn)
			}
			if fanOut == nil || fanOut.Value != float64(tc.deps) || fanOut.Unit != "modules" || fanOut.Level != "structure" {
				t.Fatalf("hub fan_out: %+v", fanOut)
			}
			if caller := structureMeasurement(r.result, "module.fan_out", "caller0.py"); caller == nil || caller.Value != 1 {
				t.Fatalf("caller fan_out: %+v", caller)
			}
			if dep := structureMeasurement(r.result, "module.fan_in", "dep0.py"); dep == nil || dep.Value != 1 {
				t.Fatalf("dependency fan_in: %+v", dep)
			}
			findings := couplingFindings(r.result)
			if !tc.high {
				if len(findings) != 0 || len(r.result.Findings) != 0 {
					t.Fatalf("unexpected findings: %+v", r.result.Findings)
				}
				return
			}
			if len(findings) != 1 {
				t.Fatalf("coupling findings: %+v", r.result.Findings)
			}
			f := findings[0]
			if f.Level != "structure" || f.Gate != "structure" || f.Severity != "warning" || f.Status != StatusAdvisory || f.Category != "quality" {
				t.Fatalf("coupling finding contract: %+v", f)
			}
			if f.Location == nil || f.Location.File != "hub.py" {
				t.Fatalf("coupling location: %+v", f.Location)
			}
			if f.Evidence != "3 importing modules depend on this module, which imports 8 in-scope runtime modules." {
				t.Fatalf("coupling evidence: %q", f.Evidence)
			}
			if !strings.Contains(f.Remediation, "do not prove excessive coupling") {
				t.Fatalf("coupling remediation: %q", f.Remediation)
			}
		})
	}
}

func TestStructureAdapterSelfImportIsNotItsOwnCoupling(t *testing.T) {
	r := contractRunner(t)
	// Two importers and seven dependencies stay below the joint threshold
	// unless the self-import is counted as both an importer and a dependency.
	r.normalize(structureSpec(), Result{ok: true}, strings.NewReader(structureCouplingReport(t, 2, 7, true)), nil)
	if err := r.result.Finish(0); err != nil {
		t.Fatal(err)
	}
	if fanIn := structureMeasurement(r.result, "module.fan_in", "hub.py"); fanIn == nil || fanIn.Value != 2 {
		t.Fatalf("self-import counted as importer: %+v", fanIn)
	}
	if fanOut := structureMeasurement(r.result, "module.fan_out", "hub.py"); fanOut == nil || fanOut.Value != 7 {
		t.Fatalf("self-import counted as dependency: %+v", fanOut)
	}
	if len(couplingFindings(r.result)) != 0 {
		t.Fatalf("self-import created coupling: %+v", r.result.Findings)
	}
	cycles := 0
	for _, finding := range r.result.Findings {
		if finding.Rule == "pyguard.structure.CIRCULAR_DEPENDENCY" {
			cycles++
		}
	}
	if cycles != 1 {
		t.Fatalf("self-import cycle finding: %+v", r.result.Findings)
	}
	if r.result.ExitCode != 0 || r.result.Status != StatusAdvisory || r.result.Assessment != Complete {
		t.Fatalf("self-import changed policy: %+v", r.result)
	}
}

// structureHumanReport combines a coupling hub with a runtime cycle so the
// human renderer must show both advisory finding kinds.
func structureHumanReport(t *testing.T) string {
	t.Helper()
	var report structureReport
	if err := json.Unmarshal([]byte(structureCouplingReport(t, 3, 8, false)), &report); err != nil {
		t.Fatal(err)
	}
	report.Modules = append(report.Modules,
		structureModule{Name: "pkg.a", File: "src/pkg/a.py"},
		structureModule{Name: "pkg.b", File: "src/pkg/b.py"},
	)
	report.Imports = append(report.Imports,
		structureImport{Importer: "pkg.a", Imported: "pkg.b"},
		structureImport{Importer: "pkg.b", Imported: "pkg.a"},
	)
	report.Components = append(report.Components, structureComponent{Modules: []string{"pkg.a", "pkg.b"}, Cyclic: true})
	report.Counts.Modules = len(report.Modules)
	report.Counts.Edges = len(report.Imports)
	report.Counts.Cycles = 1
	data, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// TestStructureHumanProcess is the fake analyzer used by the human-mode
// regression test. It emits the same structured payload the owned adapter
// consumes, so the test exercises the production subprocess runner.
func TestStructureHumanProcess(t *testing.T) {
	if os.Getenv("PYGUARD_STRUCTURE_HUMAN_HELPER") == "" {
		return
	}
	fmt.Fprint(os.Stdout, structureHumanReport(t))
	os.Exit(0)
}

func captureStdoutText(t *testing.T, fn func() int) (int, string) {
	t.Helper()
	file, err := os.CreateTemp(t.TempDir(), "stdout")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	old := os.Stdout
	os.Stdout = file
	defer func() { os.Stdout = old }()
	code := fn()
	if _, err := file.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(file)
	if err != nil {
		t.Fatal(err)
	}
	return code, string(data)
}

func TestStructureHumanRendersCouplingAndCycles(t *testing.T) {
	r := &Runner{root: t.TempDir(), timeout: 2, outputMode: "human", result: newRunResult("structure")}
	t.Setenv("PYGUARD_STRUCTURE_HUMAN_HELPER", "1")
	code, output := captureStdoutText(t, func() int {
		return runStructureHumanReport(r, os.Args[0], "-test.run=^TestStructureHumanProcess$")
	})
	if code != 0 {
		t.Fatalf("human structure exit = %d, want 0; output: %s", code, output)
	}
	for _, want := range []string{
		"[OK]   structure",
		"[ADVISORY] pyguard.structure.HIGH_MODULE_COUPLING hub.py",
		"[ADVISORY] pyguard.structure.CIRCULAR_DEPENDENCY src/pkg/a.py",
		"src/pkg/a.py, src/pkg/b.py",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("human structure output missing %q: %s", want, output)
		}
	}
	if count := strings.Count(output, "HIGH_MODULE_COUPLING"); count != 1 {
		t.Fatalf("human structure coupling count = %d, want 1: %s", count, output)
	}
}

const structureFunctionReport = `{
  "schema_version": "1",
  "modules": [{"name": "source", "file": "src/source.py", "depth": 0}],
  "imports": [],
  "components": [{"modules": ["source"], "cyclic": false}],
  "counts": {"modules": 1, "edges": 0, "cycles": 0, "type_imports": 0, "deferred_imports": 0},
  "functions": [
    {"file": "src/source.py", "line": 1, "symbol": "source.target", "depth": 0},
    {"file": "src/source.py", "line": 4, "symbol": "source.a", "depth": 1},
    {"file": "src/source.py", "line": 8, "symbol": "source.b", "depth": 1},
    {"file": "src/source.py", "line": 11, "symbol": "source.recursive", "depth": 0}
  ],
  "calls": [
    {"caller": 1, "callee": 0},
    {"caller": 2, "callee": 0},
    {"caller": 3, "callee": 3}
  ],
  "function_components": [
    {"functions": [0], "cyclic": false},
    {"functions": [1], "cyclic": false},
    {"functions": [2], "cyclic": false},
    {"functions": [3], "cyclic": true}
  ],
  "function_counts": {"functions": 4, "calls": 3, "recursive_components": 1, "unresolved_calls": 0},
  "partial": false,
  "limitation": ""
}`

func functionMeasurement(result *RunResult, metric, file, symbol string) *Measurement {
	for i := range result.Measurements {
		measurement := &result.Measurements[i]
		if measurement.Metric == metric && measurement.Location.File == file && measurement.Location.Symbol == symbol {
			return measurement
		}
	}
	return nil
}

func TestStructureAdapterReportsFunctionGraphWithoutFindings(t *testing.T) {
	r := contractRunner(t)
	r.normalize(structureSpec(), Result{ok: true}, strings.NewReader(structureFunctionReport), nil)
	if err := r.result.Finish(0); err != nil {
		t.Fatal(err)
	}
	if r.result.ExitCode != 0 || r.result.Status != StatusPass || r.result.Assessment != Complete {
		t.Fatalf("function graph changed policy: exit=%d status=%s assessment=%s", r.result.ExitCode, r.result.Status, r.result.Assessment)
	}
	if len(r.result.Findings) != 0 {
		t.Fatalf("function graph emitted findings: %+v", r.result.Findings)
	}
	for _, tc := range []struct {
		metric, file, symbol string
		want                 float64
		unit                 string
	}{
		{"function.fan_in", "src/source.py", "source.target", 2, "callers"},
		{"function.fan_out", "src/source.py", "source.target", 0, "callees"},
		{"function.call_depth", "src/source.py", "source.target", 0, "edges"},
		{"function.fan_in", "src/source.py", "source.a", 0, "callers"},
		{"function.fan_out", "src/source.py", "source.a", 1, "callees"},
		{"function.call_depth", "src/source.py", "source.a", 1, "edges"},
		{"function.fan_in", "src/source.py", "source.recursive", 0, "callers"},
		{"function.fan_out", "src/source.py", "source.recursive", 0, "callees"},
		{"function.call_depth", "src/source.py", "source.recursive", 0, "edges"},
	} {
		measurement := functionMeasurement(r.result, tc.metric, tc.file, tc.symbol)
		if measurement == nil || measurement.Value != tc.want || measurement.Unit != tc.unit || measurement.Level != "structure" {
			t.Fatalf("%s for %s: %+v", tc.metric, tc.symbol, measurement)
		}
	}
	for metric, want := range map[string]float64{
		"function.recursive_components": 1,
		"calls.unresolved":              0,
	} {
		measurement := structureMeasurement(r.result, metric, ".")
		if measurement == nil || measurement.Value != want || measurement.Level != "structure" {
			t.Fatalf("%s measurement: %+v", metric, measurement)
		}
		if metric == "function.recursive_components" && measurement.Unit != "components" {
			t.Fatalf("%s unit: %+v", metric, measurement)
		}
		if metric == "calls.unresolved" && measurement.Unit != "calls" {
			t.Fatalf("%s unit: %+v", metric, measurement)
		}
	}
}

func TestStructureAdapterReportsUnresolvedCalls(t *testing.T) {
	r := contractRunner(t)
	payload := strings.Replace(structureFunctionReport,
		`"unresolved_calls": 0`,
		`"unresolved_calls": 2`, 1)
	r.normalize(structureSpec(), Result{ok: true}, strings.NewReader(payload), nil)
	if err := r.result.Finish(0); err != nil {
		t.Fatal(err)
	}
	measurement := structureMeasurement(r.result, "calls.unresolved", ".")
	if measurement == nil || measurement.Value != 2 || measurement.Unit != "calls" {
		t.Fatalf("unresolved calls measurement: %+v", measurement)
	}
	if len(r.result.Findings) != 0 || r.result.Assessment != Complete || r.result.ExitCode != 0 {
		t.Fatalf("unresolved measurement changed policy: %+v", r.result)
	}
}

func TestStructureAdapterRejectsMalformedFunctionGraphs(t *testing.T) {
	cases := []string{
		`{"schema_version":"1","modules":[],"imports":[],"components":[],"counts":{"modules":0,"edges":0,"cycles":0},"partial":false,"limitation":""}`,
		strings.Replace(structureFunctionReport, `"function_counts": {"functions": 4`, `"function_counts": {"functions": 5`, 1),
		strings.Replace(structureFunctionReport, `{"caller": 3, "callee": 3}`, `{"caller": 9, "callee": 3}`, 1),
		strings.Replace(structureFunctionReport, `{"functions": [3], "cyclic": true}`, `{"functions": [3], "cyclic": false}`, 1),
		strings.Replace(structureFunctionReport, `{"file": "src/source.py", "line": 4, "symbol": "source.a", "depth": 1}`, `{"file": "src/source.py", "line": 1, "symbol": "source.target", "depth": 1}`, 1),
		strings.Replace(structureFunctionReport, `"unresolved_calls": 0`, `"unresolved_calls": -1`, 1),
	}
	for _, data := range cases {
		r := contractRunner(t)
		r.normalize(structureSpec(), Result{ok: true}, strings.NewReader(data), nil)
		if len(r.result.Findings) != 1 || r.result.Findings[0].Status != StatusExecutionError || r.result.Findings[0].Category != "adapter_failure" {
			t.Fatalf("accepted malformed function graph %q: %+v", data, r.result.Findings)
		}
	}
}

// structureDelegationReport builds a valid module/function payload from the
// supplied facts. Calls are derived from forwarding targets when includeCalls
// is set; the forwarding graph itself is always acyclic in those cases.
func structureDelegationReport(t *testing.T, functions []structureFunction, includeCalls bool) string {
	t.Helper()
	modules := []structureModule{}
	components := []structureComponent{}
	files := map[string]bool{}
	for _, function := range functions {
		if files[function.File] {
			continue
		}
		files[function.File] = true
		modules = append(modules, structureModule{Name: function.Symbol, File: function.File})
		components = append(components, structureComponent{Modules: []string{function.Symbol}, Cyclic: false})
	}
	calls := []structureCall{}
	if includeCalls {
		index := map[string]int{}
		for position, function := range functions {
			index[function.Symbol] = position
		}
		for position, function := range functions {
			if target, ok := index[function.ForwardTarget]; ok && function.ForwardTarget != "" {
				calls = append(calls, structureCall{Caller: position, Callee: target})
			}
		}
	}
	functionComponents := make([]structureFunctionComponent, len(functions))
	for position := range functions {
		functionComponents[position] = structureFunctionComponent{Functions: []int{position}, Cyclic: false}
	}
	report := structureReport{
		SchemaVersion:      "1",
		Modules:            modules,
		Imports:            []structureImport{},
		Components:         components,
		Counts:             structureCounts{Modules: len(modules)},
		Functions:          functions,
		Calls:              calls,
		FunctionComponents: functionComponents,
		FunctionCounts:     structureFunctionCounts{Functions: len(functions), Calls: len(calls)},
	}
	data, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func delegationFindings(result *RunResult, rule string) []Finding {
	findings := []Finding{}
	for _, finding := range result.Findings {
		if finding.Rule == "pyguard.structure."+rule {
			findings = append(findings, finding)
		}
	}
	return findings
}

func TestStructureAdapterReportsDelegationDepthAndFindings(t *testing.T) {
	functions := []structureFunction{
		{File: "src/pkg/a.py", Line: 1, Symbol: "pkg.a.a", ModuleLevel: true, ForwardingMode: "sync", ForwardTarget: "pkg.b.b"},
		{File: "src/pkg/b.py", Line: 1, Symbol: "pkg.b.b", ModuleLevel: true, ForwardingMode: "sync", ForwardTarget: "pkg.c.c"},
		{File: "src/pkg/c.py", Line: 1, Symbol: "pkg.c.c", ModuleLevel: true, ForwardingMode: "sync", ForwardTarget: "pkg.real.real"},
		{File: "src/pkg/real.py", Line: 1, Symbol: "pkg.real.real", ModuleLevel: true},
	}
	r := contractRunner(t)
	r.normalize(structureSpec(), Result{ok: true}, strings.NewReader(structureDelegationReport(t, functions, true)), nil)
	if err := r.result.Finish(0); err != nil {
		t.Fatal(err)
	}
	if r.result.ExitCode != 0 || r.result.Status != StatusAdvisory || r.result.Assessment != Complete {
		t.Fatalf("delegation changed policy: exit=%d status=%s assessment=%s", r.result.ExitCode, r.result.Status, r.result.Assessment)
	}
	if len(r.result.Gates) != 1 || r.result.Gates[0].Status != StatusAdvisory || r.result.Gates[0].Normalization != Complete {
		t.Fatalf("gate: %+v", r.result.Gates)
	}
	for symbol, want := range map[string]float64{"pkg.a.a": 3, "pkg.b.b": 2, "pkg.c.c": 1, "pkg.real.real": 0} {
		file := "src/pkg/" + map[string]string{"pkg.a.a": "a", "pkg.b.b": "b", "pkg.c.c": "c", "pkg.real.real": "real"}[symbol] + ".py"
		measurement := functionMeasurement(r.result, "delegation.depth", file, symbol)
		if measurement == nil || measurement.Value != want || measurement.Unit != "layers" || measurement.Level != "structure" {
			t.Fatalf("delegation.depth for %s: %+v", symbol, measurement)
		}
	}
	forwarding := functionMeasurement(r.result, "delegation.forwarding", "src/pkg/a.py", "pkg.a.a")
	if forwarding == nil || forwarding.Value != 1 || forwarding.Unit != "calls" || forwarding.ForwardingMode != "sync" || forwarding.ForwardTarget != "pkg.b.b" {
		t.Fatalf("delegation.forwarding for pkg.a.a: %+v", forwarding)
	}
	if functionMeasurement(r.result, "delegation.forwarding", "src/pkg/real.py", "pkg.real.real") != nil {
		t.Fatalf("non-forwarding function has forwarding measurement: %+v", r.result.Measurements)
	}
	long := delegationFindings(r.result, "LONG_DELEGATION_CHAIN")
	if len(long) != 1 {
		t.Fatalf("long delegation findings: %+v", r.result.Findings)
	}
	finding := long[0]
	if finding.Level != "structure" || finding.Gate != "structure" || finding.Severity != "warning" || finding.Status != StatusAdvisory || finding.Category != "quality" {
		t.Fatalf("long delegation contract: %+v", finding)
	}
	if finding.Location == nil || finding.Location.File != "src/pkg/a.py" || finding.Location.Symbol != "pkg.a.a" {
		t.Fatalf("long delegation location: %+v", finding.Location)
	}
	if finding.Observed == nil || *finding.Observed != 3 || finding.Threshold == nil || *finding.Threshold != 3 {
		t.Fatalf("long delegation threshold: %+v", finding)
	}
	if len(finding.Related) != 3 || finding.Related[0].Symbol != "pkg.b.b" || finding.Related[2].Symbol != "pkg.real.real" {
		t.Fatalf("long delegation related: %+v", finding.Related)
	}
	fragmented := delegationFindings(r.result, "FRAGMENTED_DELEGATION")
	if len(fragmented) != 1 {
		t.Fatalf("fragmented delegation findings: %+v", r.result.Findings)
	}
	if fragmented[0].Location == nil || fragmented[0].Location.File != "src/pkg/a.py" || len(fragmented[0].Related) != 2 {
		t.Fatalf("fragmented delegation shape: %+v", fragmented[0])
	}
	for _, finding := range r.result.Findings {
		if finding.Status != StatusAdvisory {
			t.Fatalf("delegation finding is not advisory: %+v", finding)
		}
	}
}

func TestStructureAdapterDelegationBoundaries(t *testing.T) {
	t.Run("depth two is not long", func(t *testing.T) {
		functions := []structureFunction{
			{File: "src/pkg/a.py", Line: 1, Symbol: "pkg.a.a", ModuleLevel: true, ForwardingMode: "sync", ForwardTarget: "pkg.b.b"},
			{File: "src/pkg/b.py", Line: 1, Symbol: "pkg.b.b", ModuleLevel: true, ForwardingMode: "sync", ForwardTarget: "pkg.real.real"},
			{File: "src/pkg/real.py", Line: 1, Symbol: "pkg.real.real", ModuleLevel: true},
		}
		r := contractRunner(t)
		r.normalize(structureSpec(), Result{ok: true}, strings.NewReader(structureDelegationReport(t, functions, true)), nil)
		if err := r.result.Finish(0); err != nil {
			t.Fatal(err)
		}
		if len(r.result.Findings) != 0 {
			t.Fatalf("depth two emitted findings: %+v", r.result.Findings)
		}
		if measurement := functionMeasurement(r.result, "delegation.depth", "src/pkg/a.py", "pkg.a.a"); measurement == nil || measurement.Value != 2 {
			t.Fatalf("depth two measurement: %+v", measurement)
		}
	})
	t.Run("module with a real function is not fragmented", func(t *testing.T) {
		functions := []structureFunction{
			{File: "src/pkg/a.py", Line: 1, Symbol: "pkg.a.a", ModuleLevel: true, ForwardingMode: "sync", ForwardTarget: "pkg.b.b"},
			{File: "src/pkg/b.py", Line: 1, Symbol: "pkg.b.b", ModuleLevel: true, ForwardingMode: "sync", ForwardTarget: "pkg.c.c"},
			{File: "src/pkg/b.py", Line: 8, Symbol: "pkg.b.helper", ModuleLevel: true},
			{File: "src/pkg/c.py", Line: 1, Symbol: "pkg.c.c", ModuleLevel: true, ForwardingMode: "sync", ForwardTarget: "pkg.real.real"},
			{File: "src/pkg/real.py", Line: 1, Symbol: "pkg.real.real", ModuleLevel: true},
		}
		r := contractRunner(t)
		r.normalize(structureSpec(), Result{ok: true}, strings.NewReader(structureDelegationReport(t, functions, true)), nil)
		if err := r.result.Finish(0); err != nil {
			t.Fatal(err)
		}
		if len(delegationFindings(r.result, "LONG_DELEGATION_CHAIN")) != 1 {
			t.Fatalf("missing long delegation: %+v", r.result.Findings)
		}
		if len(delegationFindings(r.result, "FRAGMENTED_DELEGATION")) != 0 {
			t.Fatalf("non-forwarding module counted as fragmented: %+v", r.result.Findings)
		}
	})
	t.Run("forwarding cycle has depth zero and no finding", func(t *testing.T) {
		functions := []structureFunction{
			{File: "src/pkg/a.py", Line: 1, Symbol: "pkg.a.a", ModuleLevel: true, ForwardingMode: "sync", ForwardTarget: "pkg.b.b"},
			{File: "src/pkg/b.py", Line: 1, Symbol: "pkg.b.b", ModuleLevel: true, ForwardingMode: "sync", ForwardTarget: "pkg.c.c"},
			{File: "src/pkg/c.py", Line: 1, Symbol: "pkg.c.c", ModuleLevel: true, ForwardingMode: "sync", ForwardTarget: "pkg.a.a"},
		}
		r := contractRunner(t)
		r.normalize(structureSpec(), Result{ok: true}, strings.NewReader(structureDelegationReport(t, functions, false)), nil)
		if err := r.result.Finish(0); err != nil {
			t.Fatal(err)
		}
		if len(r.result.Findings) != 0 {
			t.Fatalf("forwarding cycle emitted findings: %+v", r.result.Findings)
		}
		for _, function := range functions {
			if measurement := functionMeasurement(r.result, "delegation.depth", function.File, function.Symbol); measurement == nil || measurement.Value != 0 {
				t.Fatalf("cycle depth for %s: %+v", function.Symbol, measurement)
			}
		}
	})
	t.Run("async return await chain", func(t *testing.T) {
		functions := []structureFunction{
			{File: "src/pkg/a.py", Line: 1, Symbol: "pkg.a.a", ModuleLevel: true, ForwardingMode: "async-return-await", ForwardTarget: "pkg.b.b"},
			{File: "src/pkg/b.py", Line: 1, Symbol: "pkg.b.b", ModuleLevel: true, ForwardingMode: "async-return-await", ForwardTarget: "pkg.c.c"},
			{File: "src/pkg/c.py", Line: 1, Symbol: "pkg.c.c", ModuleLevel: true, ForwardingMode: "async-return-await", ForwardTarget: "pkg.work.work"},
			{File: "src/pkg/work.py", Line: 1, Symbol: "pkg.work.work", ModuleLevel: true},
		}
		r := contractRunner(t)
		r.normalize(structureSpec(), Result{ok: true}, strings.NewReader(structureDelegationReport(t, functions, true)), nil)
		if err := r.result.Finish(0); err != nil {
			t.Fatal(err)
		}
		if len(delegationFindings(r.result, "LONG_DELEGATION_CHAIN")) != 1 {
			t.Fatalf("missing async long delegation: %+v", r.result.Findings)
		}
		measurement := functionMeasurement(r.result, "delegation.forwarding", "src/pkg/c.py", "pkg.c.c")
		if measurement == nil || measurement.ForwardingMode != "async-return-await" || measurement.ForwardTarget != "pkg.work.work" {
			t.Fatalf("async forwarding measurement: %+v", measurement)
		}
	})
}

func TestStructureAdapterRejectsMalformedForwarding(t *testing.T) {
	valid := []structureFunction{
		{File: "src/pkg/a.py", Line: 1, Symbol: "pkg.a.a", ModuleLevel: true, ForwardingMode: "sync", ForwardTarget: "pkg.b.b"},
		{File: "src/pkg/b.py", Line: 1, Symbol: "pkg.b.b", ModuleLevel: true},
	}
	cases := []string{
		strings.Replace(structureDelegationReport(t, valid, false), `"forward_target":"pkg.b.b"`, `"forward_target":"pkg.missing.missing"`, 1),
		strings.Replace(structureDelegationReport(t, valid, false), `"forwarding_mode":"sync"`, `"forwarding_mode":""`, 1),
		strings.Replace(structureDelegationReport(t, valid, false), `"forwarding_mode":"sync"`, `"forwarding_mode":"async-return"`, 1),
		strings.Replace(structureDelegationReport(t, valid, false), `"module_level":true`, `"module_level":false`, 1),
		strings.Replace(structureDelegationReport(t, valid, false), `"forward_target":"pkg.b.b"`, `"forward_target":"pkg.a.a"`, 1),
	}
	for _, data := range cases {
		r := contractRunner(t)
		r.normalize(structureSpec(), Result{ok: true}, strings.NewReader(data), nil)
		if len(r.result.Findings) != 1 || r.result.Findings[0].Status != StatusExecutionError || r.result.Findings[0].Category != "adapter_failure" {
			t.Fatalf("accepted malformed forwarding %q: %+v", data, r.result.Findings)
		}
	}
}
