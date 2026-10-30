package main

import (
	"fmt"
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
  "partial": false,
  "limitation": ""
}`

const structureSelfLoopReport = `{
  "schema_version": "1",
  "modules": [{"name": "pkg.a", "file": "src/pkg/a.py", "depth": 0}],
  "imports": [{"importer": "pkg.a", "imported": "pkg.a"}],
  "components": [{"modules": ["pkg.a"], "cyclic": true}],
  "counts": {"modules": 1, "edges": 1, "cycles": 1, "type_imports": 0, "deferred_imports": 0},
  "partial": false,
  "limitation": ""
}`

const structurePartialReport = `{
  "schema_version": "1",
  "modules": [{"name": "pkg.a", "file": "src/pkg/a.py", "depth": 0}],
  "imports": [],
  "components": [{"modules": ["pkg.a"], "cyclic": false}],
  "counts": {"modules": 1, "edges": 0, "cycles": 0, "type_imports": 0, "deferred_imports": 0},
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
			payload := fmt.Sprintf(`{"schema_version":"1","modules":[{"name":"pkg.a","file":%q,"depth":0}],"imports":[{"importer":"pkg.a","imported":"pkg.a"}],"components":[{"modules":["pkg.a"],"cyclic":true}],"counts":{"modules":1,"edges":1,"cycles":1,"type_imports":0,"deferred_imports":0},"partial":false,"limitation":""}`, tc.path)
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
