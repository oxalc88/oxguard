package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNativeAdapters(t *testing.T) {
	cases := []struct {
		adapter, report, rule string
		advisory              bool
	}{
		{"bandit", `{"results":[{"test_id":"B324","issue_text":"weak hash","issue_severity":"HIGH","filename":"src/main.py","line_number":2}],"errors":[]}`, "B324", false},
		{"pip-audit", `{"dependencies":[{"name":"example","version":"1.0","vulns":[{"id":"GHSA-example","description":"vulnerable","fix_versions":["2.0"]}]}]}`, "GHSA-example", false},
		{"deptry", `[{"error":{"code":"DEP002","message":"unused"},"module":"example","location":{"file":"pyproject.toml","line":null}}]`, "DEP002", true},
	}
	for _, c := range cases {
		t.Run(c.adapter, func(t *testing.T) {
			r := &Runner{root: t.TempDir(), outputMode: "json", result: newRunResult(c.adapter)}
			r.normalize(toolSpec{gate: c.adapter, adapter: c.adapter, advisory: c.advisory}, Result{ok: false, exitCode: 1}, strings.NewReader(c.report), nil)
			r.result.Finish(1)
			if len(r.result.Findings) != 1 || r.result.Findings[0].Rule != c.rule || r.result.Assessment != "complete" {
				t.Fatalf("%+v", r.result)
			}
			bad := &Runner{root: r.root, outputMode: "json", result: newRunResult(c.adapter)}
			bad.normalize(toolSpec{gate: c.adapter, adapter: c.adapter}, Result{ok: true}, strings.NewReader(`{}`), nil)
			bad.result.Finish(0)
			if bad.result.Status != "error" || bad.result.Assessment != "incomplete" {
				t.Fatal("malformed report certified pass")
			}
		})
	}
}
func TestReportFileDropsStaleEvidence(t *testing.T) {
	r := &Runner{root: t.TempDir()}
	path, err := r.reportFile("coverage", "coverage.json")
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(path, []byte(`{"totals":{"percent_covered":100}}`), 0600)
	r.reportFile("coverage", "coverage.json")
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("stale evidence survived")
	}
	if filepath.Dir(path) == r.root {
		t.Fatal("report outside owned cache")
	}
}
