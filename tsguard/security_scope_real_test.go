package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"testing"
)

// scanWithRealEngine runs the pinned engine with the production scope arguments for root
// and returns the files it reports findings for (slash separated, sorted).
func scanWithRealEngine(t *testing.T, engine, root string) []string {
	t.Helper()
	scan, err := securityScanArgs(root, defaultExcludes)
	if err != nil || scan.empty {
		t.Fatalf("scope: %+v, %v", scan, err)
	}
	args := append([]string{"scan", "--config", filepath.Join("node_modules", ".cache", "oxguard", "rules"), "--json", "--quiet"}, scan.args...)
	cmd := exec.Command(engine, args...) //nolint:gosec
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "OPENGREP_ENABLE_VERSION_CHECK=false")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("engine failed: %v\n%s", err, out)
	}
	var report struct {
		Results []struct {
			Path string `json:"path"`
		} `json:"results"`
		Errors []json.RawMessage `json:"errors"`
	}
	if err := json.Unmarshal(out, &report); err != nil || len(report.Errors) != 0 {
		t.Fatalf("bad engine report: %v %v\n%s", err, report.Errors, out)
	}
	var files []string
	for _, result := range report.Results {
		files = append(files, filepath.ToSlash(result.Path))
	}
	sort.Strings(files)
	return files
}

// realScopeShim is an engine-backed check of the scan-root and Git-root .semgrepignore
// shim and of the default layers, run on every platform that runs the real-engine smoke
// (including Windows): a vulnerable file in docs/ and in tests/ plus a clean src/.
func realScopeShim(t *testing.T, engine string) {
	requireGit(t)
	build := func(scanRootIgnore, gitRootIgnore bool) string {
		top := t.TempDir()
		gitInit(t, top)
		app := filepath.Join(top, "packages", "app")
		write := func(path, content string) {
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		write(filepath.Join(app, "src", "a.ts"), "export const ok = 1;\n")
		write(filepath.Join(app, "docs", "d.ts"), "eval('x');\n")
		write(filepath.Join(app, "tests", "t.ts"), "eval('x');\n")
		write(filepath.Join(app, "vendor", "v.ts"), "eval('x');\n")
		write(filepath.Join(app, "node_modules", ".cache", "oxguard", "rules", "rule.yaml"),
			"rules:\n  - id: eval-use\n    languages: [typescript, javascript]\n    message: eval\n    severity: ERROR\n    pattern: eval($X)\n")
		if scanRootIgnore {
			write(filepath.Join(app, ".semgrepignore"), "docs/\n")
		}
		if gitRootIgnore {
			write(filepath.Join(top, ".semgrepignore"), "docs/\n")
		}
		return app
	}
	cases := []struct {
		name                       string
		scanRootIgnore, gitRootIgn bool
		want                       []string
	}{
		// No file: the 1.23.0 default list applies (tests and vendor skipped), docs is scanned.
		{"defaults without a project file", false, false, []string{"docs/d.ts"}},
		// A scan-root file replaces the test defaults (tests/ scanned), docs/ is hidden by the file, the baseline (vendor) stays excluded.
		{"scan-root file in a Git subproject", true, false, []string{"tests/t.ts"}},
		// A Git-root file does the same for the subproject.
		{"Git-root file", false, true, []string{"tests/t.ts"}},
	}
	for _, c := range cases {
		got := scanWithRealEngine(t, engine, build(c.scanRootIgnore, c.gitRootIgn))
		if len(got) != len(c.want) || (len(got) > 0 && got[0] != c.want[0]) {
			t.Errorf("%s: findings in %v, want %v", c.name, got, c.want)
		}
	}
}
