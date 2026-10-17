package contract

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestDependencyReviewRequiresBoundEvidence(t *testing.T) {
	root := t.TempDir()
	policy := DependencyPolicy{Inputs: map[string]string{}, Reviews: []DependencyReview{{Ecosystem: "npm", Package: "tool", Advisory: "GHSA-known", Scope: "development_tool", ProductExposure: "not_exposed", ToolingRisk: "accepted", Reason: "Build tool only; affected input is excluded and CI uses trusted inputs."}}}
	for _, name := range []string{"package.json", "package-lock.json"} {
		os.WriteFile(filepath.Join(root, name), []byte("{}"), 0600)
		policy.Inputs[name] = fmt.Sprintf("%x", sha256.Sum256([]byte("{}")))
	}
	write := func() {
		t.Helper()
		data, err := json.Marshal(policy)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, ".oxguard-dependency-policy.json"), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	write()
	loaded, err := LoadDependencyPolicy(root, "npm")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Evidence("npm", "tool", "GHSA-known") == "" {
		t.Fatal("review missing")
	}
	for _, identity := range [][3]string{{"npm", "other", "GHSA-known"}, {"npm", "tool", "GHSA-other"}, {"pypi", "tool", "GHSA-known"}} {
		if loaded.Evidence(identity[0], identity[1], identity[2]) != "" {
			t.Fatal("review escaped identity")
		}
	}
	for _, invalid := range []DependencyReview{
		{Ecosystem: "npm", Package: "tool", Advisory: "GHSA-known", Scope: "runtime", ProductExposure: "not_exposed", ToolingRisk: "accepted", Reason: "reason"},
		{Ecosystem: "npm", Package: "tool", Advisory: "GHSA-known", Scope: "ci", ProductExposure: "unknown", ToolingRisk: "accepted", Reason: "reason"},
		{Ecosystem: "npm", Package: "tool", Advisory: "GHSA-known", Scope: "ci", ProductExposure: "not_exposed", ToolingRisk: "unknown", Reason: "reason"},
		{Ecosystem: "npm", Package: "tool", Advisory: "GHSA-known", Scope: "ci", ProductExposure: "not_exposed", ToolingRisk: "accepted"},
	} {
		policy.Reviews = []DependencyReview{invalid}
		write()
		if _, err := LoadDependencyPolicy(root, "npm"); err == nil {
			t.Fatal("invalid risk decision accepted")
		}
	}
	policy.Reviews = nil
	write()
	if err := os.WriteFile(filepath.Join(root, "pnpm-lock.yaml"), []byte("graph"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadDependencyPolicy(root, "npm"); err == nil {
		t.Fatal("unbound second package manager lock accepted")
	}
	if err := os.Remove(filepath.Join(root, "pnpm-lock.yaml")); err != nil {
		t.Fatal(err)
	}
	write()
	os.WriteFile(filepath.Join(root, "package-lock.json"), []byte("changed"), 0600)
	if _, err := LoadDependencyPolicy(root, "npm"); err == nil {
		t.Fatal("stale review accepted")
	}
}

func TestMissingDependencyPolicyHasNoExceptions(t *testing.T) {
	policy, err := LoadDependencyPolicy(t.TempDir(), "pypi")
	if err != nil || policy.Evidence("pypi", "tool", "GHSA-known") != "" {
		t.Fatal("missing policy created exception")
	}
}
