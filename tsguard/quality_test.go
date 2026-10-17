package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProjectLintPoliciesAreRecognized(t *testing.T) {
	for _, name := range []string{"biome.jsonc", "eslint.config.mts", ".eslintrc.json", ".eslintrc.yml", ".oxlintrc.json"} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			if hasLintConfig(root) {
				t.Fatal("unexpected policy")
			}
			if err := os.WriteFile(filepath.Join(root, name), []byte("{}"), 0600); err != nil {
				t.Fatal(err)
			}
			if !hasLintConfig(root) {
				t.Fatal("project policy missed")
			}
		})
	}
}

func TestOwnedCapabilityErrorIsIncomplete(t *testing.T) {
	r := contractRunner(t)
	_, err := r.normalizeOwnedAnalysis(toolSpec{gate: "maintainability"}, strings.NewReader(`{"error":{"category":"tool_missing","message":"missing compiler"}}`), nil)
	if err != nil {
		t.Fatal(err)
	}
	r.result.RecordGate("maintainability", true, false)
	r.result.Finish(1)
	if r.result.Assessment != Incomplete || r.result.Status != StatusError {
		t.Fatalf("%+v", r.result)
	}
}

func TestOwnedPartialCapabilityDoesNotPass(t *testing.T) {
	r := contractRunner(t)
	complete, err := r.normalizeOwnedAnalysis(toolSpec{gate: "maintainability"}, strings.NewReader(`{"findings":[],"measurements":[],"partial":true,"limitation":"local imports are unresolved"}`), nil)
	if err != nil || complete {
		t.Fatalf("complete=%v error=%v", complete, err)
	}
	r.result.RecordGate("maintainability", true, complete)
	r.result.Finish(0)
	if r.result.Assessment != Incomplete {
		t.Fatal(r.result)
	}
}

func TestOwnedFindingStatusCannotBeMaskedByAdvisoryPolicy(t *testing.T) {
	for _, status := range []string{"unknown", "pass", "execution_error"} {
		r := contractRunner(t)
		payload := `{"findings":[{"status":"` + status + `","rule":"example","evidence":"example","location":{"file":"src/a.ts"}}],"measurements":[]}`
		if _, err := r.normalizeOwnedAnalysis(toolSpec{gate: "maintainability", advisory: true}, strings.NewReader(payload), nil); err == nil {
			t.Fatalf("%s status accepted", status)
		}
	}
}
