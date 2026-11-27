package main

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is required for this test")
	}
}

func gitInit(t *testing.T, dir string) {
	t.Helper()
	requireGit(t)
	if out, err := exec.Command("git", "-C", dir, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
}

func excludesOf(args []string) []string {
	var patterns []string
	for i := 0; i+1 < len(args); i++ {
		if args[i] == "--exclude" {
			patterns = append(patterns, args[i+1])
		}
	}
	return patterns
}

func contains(list []string, item string) bool {
	for _, value := range list {
		if value == item {
			return true
		}
	}
	return false
}

// The two layers are a fixed, partitioned copy of Opengrep 1.23.0's template;
// together they must reproduce it entry for entry.
func TestSecurityDefaultLayersAreTheV123Template(t *testing.T) {
	template := []string{".git", ".svn", ".hg", "_darcs", "CVS", "build/", "vendor/", "dist/", "*.min.js", ".env/", ".tox/",
		"node_modules/", ".npm/", ".yarn/", ".venv/", "_opam/", "_build/", "_cargo/", "test/", "tests/", "testsuite/", "*_test.go"}
	got := append(append([]string{}, securityBaselineExcludes...), securityReplaceableDefaults...)
	if !reflect.DeepEqual(got, template) {
		t.Fatalf("default layers drifted from the v1.23.0 template: %q", got)
	}
	if !reflect.DeepEqual(securityReplaceableDefaults, []string{"test/", "tests/", "testsuite/", "*_test.go"}) {
		t.Fatalf("only the conventional test paths may be project-replaceable: %q", securityReplaceableDefaults)
	}
}

// The mapping reproduces main's 1.23.0 behavior: leading and trailing slashes
// never anchored anything (scripts/compare-opengrep.py proves it per form).
func TestOpengrepExcludePattern(t *testing.T) {
	cases := map[string]string{
		"node_modules":  "**/node_modules",
		"dist":          "**/dist",
		"cache/private": "**/cache/private",
		"/src":          "**/src",
		"src/":          "**/src",
		"/src/":         "**/src",
		"//src":         "**/src",
		"a/b":           "**/a/b",
		"/a/b":          "**/a/b",
		"/a/b/":         "**/a/b",
		"/a.ts":         "**/a.ts",
		"*.snap":        "**/*.snap",
		"/*.ts":         "**/*.ts",
		"**/generated":  "**/generated",
		"x/**":          "**/x/**",
		"./dist":        "**/./dist",
		"!keep":         "**/!keep",
		"#note":         "**/#note",
		"/a b/ü":        "**/a b/ü",
		"a//b":          "**/a/b",
		"/a//b/":        "**/a/b",
		"a///b":         "**/a/b",
		"a//b//":        "**/a/b",
		"//a//b":        "**/a/b",
		"a/./b":         "**/a/./b",
	}
	for entry, want := range cases {
		got, err := opengrepExcludePattern(entry)
		if err != nil || got != want {
			t.Errorf("%q = %q, %v; want %q", entry, got, err, want)
		}
	}
	// 1.23.0 excluded every file for these; silently disabling the gate is rejected.
	for _, entry := range []string{"", "/", "//"} {
		if got, err := opengrepExcludePattern(entry); err == nil {
			t.Errorf("%q accepted as %q", entry, got)
		}
	}
	if runtime.GOOS == "windows" {
		if got, _ := opengrepExcludePattern(`\src\gen`); got != "**/src/gen" {
			t.Errorf("windows separators not normalized: %q", got)
		}
	}
}

func TestSecurityScopeOutsideGitPinsProjectAndAddsDefaults(t *testing.T) {
	root := t.TempDir()
	scope, err := securityScopeArgs(root, []string{"dist", "/src"})
	if err != nil {
		t.Fatal(err)
	}
	args := scope.args
	if !reflect.DeepEqual(args[:2], []string{"--project-root", "."}) {
		t.Fatalf("non-git scan must pin the project root first: %q", args)
	}
	if contains(args, "--force-exclude") {
		t.Fatalf("--force-exclude would drop root files named like directory entries: %q", args)
	}
	excludes := excludesOf(args)
	for _, want := range []string{"**/dist", "**/src", "tests/", "vendor/", "*.min.js", "build/", "test/", "node_modules/"} {
		if !contains(excludes, want) {
			t.Errorf("missing exclude %q in %q", want, excludes)
		}
	}
}

func scopeOf(t *testing.T, root string, excludes []string) securityScope {
	t.Helper()
	scope, err := securityScopeArgs(root, excludes)
	if err != nil {
		t.Fatal(err)
	}
	return scope
}

// A project-root .semgrepignore replaces only the test-path defaults, as in
// 1.23.0, even when the file is empty. The baseline layer (dependencies,
// vendored code, build output, minified bundles) and OxGuard exclusions always
// apply and cannot be undone by the project file.
func TestSecurityScopeProjectIgnoreFileReplacesOnlyTestDefaults(t *testing.T) {
	for name, content := range map[string]string{"empty": "", "patterns": "scripts/\n"} {
		root := t.TempDir()
		if err := os.WriteFile(filepath.Join(root, ".semgrepignore"), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		got := excludesOf(scopeOf(t, root, []string{"dist"}).args)
		want := append([]string{"**/dist"}, securityBaselineExcludes...)
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s ignore file: excludes = %q, want exclusions plus baseline %q", name, got, want)
		}
		for _, replaced := range securityReplaceableDefaults {
			if contains(got, replaced) {
				t.Errorf("%s ignore file: %q must be project-replaceable", name, replaced)
			}
		}
	}
	// A directory or unreadable entry holds no patterns and does not replace defaults.
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".semgrepignore"), 0o755); err != nil {
		t.Fatal(err)
	}
	if !contains(excludesOf(scopeOf(t, root, nil).args), "tests/") {
		t.Errorf("directory named .semgrepignore replaced defaults")
	}
}

func TestSecurityScopeNestedIgnoreFileDoesNotReplaceDefaults(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "src", ".semgrepignore"), []byte("a.ts\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !contains(excludesOf(scopeOf(t, root, nil).args), "tests/") {
		t.Fatal("nested ignore file replaced the defaults")
	}
}

func TestSecurityScopeInGitKeepsNativeProject(t *testing.T) {
	root := t.TempDir()
	gitInit(t, root)
	scope := scopeOf(t, root, []string{"/src"})
	if contains(scope.args, "--project-root") {
		t.Fatalf("Git projects keep Opengrep's native Git root and .gitignore handling: %q", scope.args)
	}
	// The same mapping everywhere: no anchoring and no Git-relative translation.
	if !contains(excludesOf(scope.args), "**/src") || !contains(excludesOf(scope.args), "tests/") {
		t.Fatalf("unexpected excludes %q", excludesOf(scope.args))
	}
	sub := filepath.Join(root, "packages", "app")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	subScope := scopeOf(t, sub, []string{"/src"})
	if !reflect.DeepEqual(excludesOf(subScope.args), excludesOf(scope.args)) || contains(subScope.args, "--project-root") {
		t.Fatalf("subproject scope = %q", subScope.args)
	}
}

// Monorepo subproject: a .semgrepignore at the scan root replaces the test
// defaults, as in 1.23.0, and so does one at the Git root, as in 2.1.0. The
// baseline layer applies in every case. Nested test directories of the
// scan-root-only case are a documented limit (deferred), not restored here.
func TestSecurityScopeSubprojectIgnoreFileReplacesTestDefaults(t *testing.T) {
	requireGit(t)
	cases := []struct {
		name             string
		gitRootFile, sub bool
		wantTestDefaults bool
	}{
		{"no files: defaults apply", false, false, true},
		{"scan-root file only", false, true, false},
		{"git-root file only", true, false, false},
		{"both files", true, true, false},
	}
	for _, c := range cases {
		top := t.TempDir()
		gitInit(t, top)
		sub := filepath.Join(top, "packages", "app")
		if err := os.MkdirAll(sub, 0o755); err != nil {
			t.Fatal(err)
		}
		for path, present := range map[string]bool{filepath.Join(top, ".semgrepignore"): c.gitRootFile, filepath.Join(sub, ".semgrepignore"): c.sub} {
			if present {
				if err := os.WriteFile(path, []byte("docs/\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
		}
		scope := scopeOf(t, sub, nil)
		excludes := excludesOf(scope.args)
		if got := contains(excludes, "tests/"); got != c.wantTestDefaults {
			t.Errorf("%s: test defaults present = %v, want %v (%q)", c.name, got, c.wantTestDefaults, excludes)
		}
		if !contains(excludes, "vendor/") || !contains(excludes, "*.min.js") {
			t.Errorf("%s: baseline layer missing: %q", c.name, excludes)
		}
		if contains(scope.args, "--project-root") {
			t.Errorf("%s: subproject scans must keep the Git root for .gitignore: %q", c.name, scope.args)
		}
	}
}

// When the scan root is the Git root nothing changes: one file, one location,
// and the root file is the project file.
func TestSecurityScopeScanRootEqualToGitRoot(t *testing.T) {
	top := t.TempDir()
	gitInit(t, top)
	if !contains(excludesOf(scopeOf(t, top, nil).args), "tests/") {
		t.Fatal("defaults missing at the Git root")
	}
	if err := os.WriteFile(filepath.Join(top, ".semgrepignore"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	scope := scopeOf(t, top, nil)
	if contains(excludesOf(scope.args), "tests/") || !contains(excludesOf(scope.args), "vendor/") {
		t.Fatalf("root file must replace only the test defaults: %q", excludesOf(scope.args))
	}
}

func TestSecurityScopeRejectsInvalidExclusions(t *testing.T) {
	for _, entry := range []string{"", "/"} {
		_, err := securityScopeArgs(t.TempDir(), []string{entry})
		var config *scopeConfigError
		if !errors.As(err, &config) {
			t.Fatalf("%q: want a configuration error, got %v", entry, err)
		}
	}
}

// Target selection is read-only: it neither creates files nor needs a writable
// scan root, unlike the removed snapshot, and leaves no temporary files behind.
func TestSecurityScanArgsWriteNothingIntoProject(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"src", "tests"} {
		if err := os.Mkdir(filepath.Join(root, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	list := func() []string {
		var names []string
		_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			names = append(names, path)
			return nil
		})
		return names
	}
	before := list()
	if os.Geteuid() != 0 {
		if err := os.Chmod(root, 0o555); err != nil {
			t.Fatal(err)
		}
		defer os.Chmod(root, 0o755)
	}
	scan, err := securityScanArgs(root, defaultExcludes)
	if err != nil {
		t.Fatal(err)
	}
	if after := list(); !reflect.DeepEqual(before, after) {
		t.Fatalf("scope selection changed the project: %v -> %v", before, after)
	}
	if !contains(scan.args, "."+string(os.PathSeparator)+"src") {
		t.Fatalf("root targets missing: %q", scan.args)
	}
}

func TestSecurityScanArgsKeepRootTargetsAndNeverEnumerateDependencies(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"src", "node_modules", ".git", "--option"} {
		if err := os.Mkdir(filepath.Join(root, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	scan, err := securityScanArgs(root, defaultExcludes)
	if err != nil {
		t.Fatal(err)
	}
	args := scan.args
	prefix := "." + string(os.PathSeparator)
	if tail := args[len(args)-2:]; !reflect.DeepEqual(tail, []string{prefix + "--option", prefix + "src"}) {
		t.Fatalf("targets = %q", tail)
	}
	joined := strings.Join(args, "\x00")
	if strings.Contains(joined, "\x00"+prefix+"node_modules") || strings.Contains(joined, "\x00"+prefix+".git") {
		t.Fatalf("dependency or VCS directory used as target: %q", args)
	}
}

// Only minified root files: nothing to scan, and Opengrep must not be started.
func TestSecurityScanArgsEmptySelectionIsExplicit(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "x.min.js"), []byte("eval('x')"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "node_modules"), 0o755); err != nil {
		t.Fatal(err)
	}
	scan, err := securityScanArgs(root, defaultExcludes)
	if err != nil || !scan.empty || len(scan.args) != 0 {
		t.Fatalf("scan = %+v, err = %v; want an explicit empty selection", scan, err)
	}
	// A project with no root entries at all keeps its configuration error.
	if _, err := securityScanArgs(t.TempDir(), defaultExcludes); err == nil {
		t.Fatal("a root with no files must still be reported")
	}
}

func TestRecordEmptySecurityScanIsCleanAndComplete(t *testing.T) {
	result := newRunResult("check")
	r := &Runner{outputMode: "json", result: result}
	if code := r.recordEmptySecurityScan(); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if err := result.Finish(0); err != nil {
		t.Fatal(err)
	}
	if len(result.Findings) != 0 || result.Status != StatusPass || result.Assessment != Complete {
		t.Fatalf("empty scan result = status %q assessment %q findings %d", result.Status, result.Assessment, len(result.Findings))
	}
	if len(result.Gates) != 1 || result.Gates[0].Name != "security" || result.Gates[0].Status != StatusPassed {
		t.Fatalf("gates = %+v", result.Gates)
	}
}

// Used by scripts/compare-opengrep.py so the engine comparison runs the
// production argument construction.
func TestSecurityScopeComparisonHelper(t *testing.T) {
	root := os.Getenv("OXGUARD_COMPARE_ROOT")
	if root == "" {
		t.Skip("comparison harness only")
	}
	excludes := append([]string{}, defaultExcludes...)
	if extra := os.Getenv("OXGUARD_COMPARE_EXCLUDE"); extra != "" {
		var more []string
		if err := json.Unmarshal([]byte(extra), &more); err != nil {
			t.Fatal(err)
		}
		excludes = append(excludes, more...)
	}
	scan, err := securityScanArgs(root, excludes)
	result := map[string]any{"args": scan.args, "empty": scan.empty,
		"layers": map[string]any{"baseline": securityBaselineExcludes, "replaceable": securityReplaceableDefaults}}
	if err != nil {
		// A rejected configuration or a fail-closed scope is part of the measured behavior, not a test failure.
		result["error"] = err.Error()
	}
	data, _ := json.Marshal(result)
	if err := os.WriteFile(os.Getenv("OXGUARD_COMPARE_ARGS"), data, 0o600); err != nil {
		t.Fatal(err)
	}
}
