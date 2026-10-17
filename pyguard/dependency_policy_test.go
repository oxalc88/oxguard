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
	policy := contract.DependencyPolicy{Inputs: map[string]string{}, Reviews: []contract.DependencyReview{{Ecosystem: "pypi", Package: "tool", Advisory: "GHSA-known", Scope: "development_tool", ProductExposure: "not_exposed", ToolingRisk: "accepted", Reason: "Reviewed build-only dependency; trusted CI inputs and no product inclusion."}}}
	for _, name := range []string{"pyproject.toml", "uv.lock"} {
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
		{"unreviewed", false, false, `{"dependencies":[{"name":"tool","version":"1.0","vulns":[{"id":"GHSA-known","description":"issue","fix_versions":[]}]}]}`, "blocking"},
		{"reviewed", true, false, `{"dependencies":[{"name":"tool","version":"1.0","vulns":[{"id":"GHSA-known","description":"issue","fix_versions":[]}]}]}`, "advisory"},
		{"new advisory", true, false, strings.ReplaceAll(`{"dependencies":[{"name":"tool","version":"1.0","vulns":[{"id":"GHSA-known","description":"issue","fix_versions":[]}]}]}`, "GHSA-known", "GHSA-new"), "blocking"},
		{"stale graph", true, true, `{"dependencies":[{"name":"tool","version":"1.0","vulns":[{"id":"GHSA-known","description":"issue","fix_versions":[]}]}]}`, "execution_error"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			if tc.review {
				dependencyReviewFixture(t, root)
			}
			if tc.stale {
				os.WriteFile(filepath.Join(root, "uv.lock"), []byte("changed"), 0600)
			}
			r := &Runner{root: root, outputMode: "json", result: newRunResult("security")}
			r.normalize(toolSpec{gate: "pip-audit", adapter: "pip-audit"}, Result{ok: false}, strings.NewReader(tc.report), nil)
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
			script := "#!/bin/sh\nprintf '%s\\n' '" + `{"dependencies":[{"name":"tool","version":"1.0","vulns":[{"id":"GHSA-known","description":"issue","fix_versions":[]}]}]}` + "'\nexit 1\n"
			if err := os.WriteFile(shim, []byte(script), 0700); err != nil {
				t.Fatal(err)
			}
			r := &Runner{root: root, timeout: 5, outputMode: mode, result: newRunResult("security")}
			result := r.runDependencyTool(toolSpec{gate: "pip-audit", adapter: "pip-audit"}, "audit fixture", shim)
			if !result.ok || len(r.result.Findings) == 0 {
				t.Fatalf("reviewed finding blocked or hidden: %+v", r.result)
			}
			os.WriteFile(filepath.Join(root, "uv.lock"), []byte("changed"), 0600)
			if r.runDependencyTool(toolSpec{gate: "pip-audit", adapter: "pip-audit"}, "audit fixture", shim).ok {
				t.Fatal("stale review allowed native failure")
			}
		})
	}
}

func TestDependencyFailureDoesNotSkipSecrets(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell audit fixture")
	}
	root := t.TempDir()
	bin := t.TempDir()
	script := `#!/bin/sh
case "$*" in
 *bandit*) printf '%s\n' '{"results":[],"errors":[]}' ;;
 *pip-audit*) printf '%s\n' '{"dependencies":[{"name":"tool","version":"1.0","vulns":[{"id":"GHSA-known","description":"issue","fix_versions":[]}]}]}';exit 1 ;;
 *check_secrets.py*) printf '%s\n' '{"schema_version":"1","findings":[],"measurements":[],"artifacts":[]}' ;;
esac
`
	if err := os.WriteFile(filepath.Join(bin, "uv"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	r := &Runner{root: root, timeout: 5, outputMode: "json", result: newRunResult("security")}
	r.result.Plan("bandit", "pip-audit", "secrets")
	if runSecurity(r, []string{"."}) == 0 {
		t.Fatal("unreviewed advisory accepted")
	}
	for _, gate := range r.result.Gates {
		if gate.Status == StatusNotRun {
			t.Fatalf("security check skipped: %+v", gate)
		}
	}
}
