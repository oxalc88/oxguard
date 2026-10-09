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
	_, err := r.normalizeOwnedAnalysis(toolSpec{gate: "typed-lint"}, strings.NewReader(`{"error":{"category":"tool_missing","message":"missing parser"}}`), nil)
	if err != nil {
		t.Fatal(err)
	}
	r.result.RecordGate("typed-lint", true, false)
	r.result.Finish(1)
	if r.result.Assessment != "incomplete" || r.result.Status != "error" {
		t.Fatalf("%+v", r.result)
	}
}

func TestOwnedPartialCapabilityDoesNotPass(t *testing.T) {
	r := contractRunner(t)
	complete, err := r.normalizeOwnedAnalysis(toolSpec{gate: "typed-lint"}, strings.NewReader(`{"findings":[],"measurements":[],"partial":true,"limitation":"strictNullChecks is disabled"}`), nil)
	if err != nil || complete {
		t.Fatalf("complete=%v error=%v", complete, err)
	}
	r.result.RecordGate("typed-lint", true, complete)
	r.result.Finish(0)
	if r.result.Assessment != "incomplete" {
		t.Fatal(r.result)
	}
}
