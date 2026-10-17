package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/oxalc88/oxguard/contract"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func dependencyReviewFixture(t *testing.T, root string) {
	t.Helper()
	policy := contract.DependencyPolicy{Inputs: map[string]string{}, Reviews: []contract.DependencyReview{{Ecosystem: "npm", Package: "tool", Advisory: "GHSA-known", Scope: "development_tool", ProductExposure: "not_exposed", ToolingRisk: "accepted", Reason: "Reviewed build-only dependency; trusted CI inputs and no product inclusion."}}}
	for _, name := range []string{"package.json", "package-lock.json"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("{}"), 0600); err != nil {
			t.Fatal(err)
		}
		policy.Inputs[name] = fmt.Sprintf("%x", sha256.Sum256([]byte("{}")))
	}
	data, _ := json.Marshal(policy)
	if err := os.WriteFile(filepath.Join(root, ".oxguard-dependency-policy.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
}

func TestReviewedDependencyRemainsVisible(t *testing.T) {
	for _, tc := range []struct {
		name   string
		review bool
		stale  bool
		report string
		want   string
	}{
		{"unreviewed", false, false, `{"auditReportVersion":2,"vulnerabilities":{"tool":{"via":[{"name":"tool","severity":"high","title":"issue","url":"https://github.com/advisories/GHSA-known"}]}},"metadata":{}}`, "blocking"},
		{"reviewed", true, false, `{"auditReportVersion":2,"vulnerabilities":{"tool":{"via":[{"name":"tool","severity":"high","title":"issue","url":"https://github.com/advisories/GHSA-known"}]}},"metadata":{}}`, "advisory"},
		{"new advisory", true, false, strings.ReplaceAll(`{"auditReportVersion":2,"vulnerabilities":{"tool":{"via":[{"name":"tool","severity":"high","title":"issue","url":"https://github.com/advisories/GHSA-known"}]}},"metadata":{}}`, "GHSA-known", "GHSA-new"), "blocking"},
		{"stale graph", true, true, `{"auditReportVersion":2,"vulnerabilities":{"tool":{"via":[{"name":"tool","severity":"high","title":"issue","url":"https://github.com/advisories/GHSA-known"}]}},"metadata":{}}`, "execution_error"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			if tc.review {
				dependencyReviewFixture(t, root)
			}
			if tc.stale {
				os.WriteFile(filepath.Join(root, "package-lock.json"), []byte("changed"), 0600)
			}
			r := &Runner{root: root, outputMode: "json", result: newRunResult("security")}
			r.normalize(toolSpec{gate: "dependencies", adapter: "audit-ci"}, Result{ok: false}, strings.NewReader(tc.report), nil)
			found := false
			for _, finding := range r.result.Findings {
				if finding.Status.String() == tc.want {
					found = true
				}
			}
			if !found {
				t.Fatalf("findings: %+v", r.result.Findings)
			}
		})
	}
}

func TestReviewedAuditUsesSameDecisionInHumanAndMachineModes(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell audit fixture")
	}
	for _, mode := range []string{"human", "json", "agent"} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			dependencyReviewFixture(t, root)
			shim := filepath.Join(root, "audit-fixture")
			script := "#!/bin/sh\nprintf '%s\\n' '" + `{"auditReportVersion":2,"vulnerabilities":{"tool":{"via":[{"name":"tool","severity":"high","title":"issue","url":"https://github.com/advisories/GHSA-known"}]}},"metadata":{}}` + "'\nexit 1\n"
			if err := os.WriteFile(shim, []byte(script), 0700); err != nil {
				t.Fatal(err)
			}
			r := &Runner{root: root, timeout: 5, outputMode: mode, result: newRunResult("security")}
			result := r.runDependencyTool(toolSpec{gate: "dependencies", adapter: "audit-ci"}, "audit fixture", shim)
			if !result.ok || len(r.result.Findings) == 0 {
				t.Fatalf("reviewed finding blocked or hidden: %+v", r.result)
			}
			os.WriteFile(filepath.Join(root, "package-lock.json"), []byte("changed"), 0600)
			if r.runDependencyTool(toolSpec{gate: "dependencies", adapter: "audit-ci"}, "audit fixture", shim).ok {
				t.Fatal("stale review allowed native failure")
			}
		})
	}
}

func TestSecurityCompletesIndependentChecksAfterFailures(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell audit fixture")
	}
	r, _ := newTestRunner(t)
	r.outputMode = "json"
	r.result = newRunResult("security")
	r.result.Plan("secrets", "dependencies", "security")
	// Empty secretlint output fails normalization. Missing Opengrep must still be
	// reported after dependencies execute, instead of remaining not_run.
	if runSecurity(r, false) == 0 {
		t.Fatal("failed security scan passed")
	}
	for _, gate := range r.result.Gates {
		if gate.Status == StatusNotRun {
			t.Fatalf("security check skipped: %+v", gate)
		}
	}
}
