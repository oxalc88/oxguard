package main

import (
	"encoding/json"
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

func TestOpengrepExcludePattern(t *testing.T) {
	cases := []struct {
		entry, prefix, want string
		skip, fail          bool
	}{
		{entry: "node_modules", want: "**/node_modules"},
		{entry: "dist", want: "**/dist"},
		{entry: "cache/private", want: "**/cache/private"},
		{entry: "./dist", want: "**/dist"},
		{entry: "**/generated", want: "**/generated"},
		{entry: "src/", want: "**/src/"},
		{entry: "*.snap", want: "**/*.snap"},
		{entry: "/src", want: "/src"},
		{entry: "/src/generated", want: "/src/generated"},
		{entry: "//src", want: "/src"},
		{entry: "/src", prefix: "packages/app/", want: "/packages/app/src"},
		{entry: "/a b/ü", want: "/a b/ü"},
		{entry: "", skip: true},
		{entry: "!keep", fail: true},
		{entry: "#note", fail: true},
		{entry: "/", fail: true},
		{entry: ".", fail: true},
	}
	for _, c := range cases {
		got, ok, err := opengrepExcludePattern(c.entry, c.prefix)
		if c.fail {
			if err == nil {
				t.Errorf("%q accepted as %q", c.entry, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("%q: %v", c.entry, err)
			continue
		}
		if c.skip == ok || got != c.want {
			t.Errorf("%q (prefix %q) = %q,%v; want %q", c.entry, c.prefix, got, ok, c.want)
		}
	}
	if runtime.GOOS == "windows" {
		if got, _, _ := opengrepExcludePattern(`\src\gen`, ""); got != "/src/gen" {
			t.Errorf("windows separators not normalized: %q", got)
		}
	}
}

func TestSecurityScopeOutsideGitPinsProjectAndAddsDefaults(t *testing.T) {
	root := t.TempDir()
	args, err := securityScopeArgs(root, []string{"dist", "/src"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(args[:2], []string{"--project-root", "."}) {
		t.Fatalf("non-git scan must pin the project root first: %q", args)
	}
	if contains(args, "--force-exclude") {
		t.Fatalf("--force-exclude would drop root files named like directory entries: %q", args)
	}
	excludes := excludesOf(args)
	for _, want := range []string{"**/dist", "/src", "tests/", "vendor/", "*.min.js", "build/", "test/", "node_modules/"} {
		if !contains(excludes, want) {
			t.Errorf("missing exclude %q in %q", want, excludes)
		}
	}
	if contains(excludes, "**//src") {
		t.Fatalf("anchored exclusion was prefixed: %q", excludes)
	}
}

// A project-root .semgrepignore replaces only the test-path defaults, as it did
// in 1.23.0, even when the file is empty. The baseline layer (dependencies,
// vendored code, build output, minified bundles) and OxGuard exclusions always
// apply and cannot be undone by the project file.
func TestSecurityScopeProjectIgnoreFileReplacesOnlyTestDefaults(t *testing.T) {
	for name, content := range map[string]string{"empty": "", "patterns": "scripts/\n"} {
		root := t.TempDir()
		if err := os.WriteFile(filepath.Join(root, ".semgrepignore"), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		args, err := securityScopeArgs(root, []string{"dist"})
		if err != nil {
			t.Fatal(err)
		}
		want := append([]string{"**/dist"}, securityBaselineExcludes...)
		if got := excludesOf(args); !reflect.DeepEqual(got, want) {
			t.Errorf("%s ignore file: excludes = %q, want exclusions plus baseline %q", name, got, want)
		}
		for _, replaced := range securityReplaceableDefaults {
			if contains(excludesOf(args), replaced) {
				t.Errorf("%s ignore file: %q must be project-replaceable", name, replaced)
			}
		}
	}
	// A directory or unreadable entry holds no patterns and does not replace defaults.
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".semgrepignore"), 0o755); err != nil {
		t.Fatal(err)
	}
	args, _ := securityScopeArgs(root, nil)
	if !contains(excludesOf(args), "tests/") {
		t.Errorf("directory named .semgrepignore replaced defaults: %q", excludesOf(args))
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
	args, _ := securityScopeArgs(root, nil)
	if !contains(excludesOf(args), "tests/") {
		t.Fatalf("nested ignore file replaced the defaults: %q", excludesOf(args))
	}
}

func TestSecurityScopeInGitUsesNativeProjectAndAnchorsAtGitRoot(t *testing.T) {
	root := t.TempDir()
	gitInit(t, root)
	args, err := securityScopeArgs(root, []string{"/src"})
	if err != nil {
		t.Fatal(err)
	}
	if contains(args, "--project-root") {
		t.Fatalf("Git projects keep Opengrep's native Git root and .gitignore handling: %q", args)
	}
	if !contains(excludesOf(args), "/src") || !contains(excludesOf(args), "tests/") {
		t.Fatalf("unexpected excludes %q", excludesOf(args))
	}
	// A scan root below the Git root anchors at the Git root.
	sub := filepath.Join(root, "packages", "app")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	args, err = securityScopeArgs(sub, []string{"/src"})
	if err != nil {
		t.Fatal(err)
	}
	if !contains(excludesOf(args), "/packages/app/src") || contains(args, "--project-root") {
		t.Fatalf("subproject scope = %q", args)
	}
}

// Monorepo subproject: a .semgrepignore at the scan root replaces the test
// defaults, as in 1.23.0, and so does one at the Git root, as in 2.1.0. The
// baseline layer applies in every case.
func TestSecurityScopeSubprojectIgnoreFileReplacesTestDefaults(t *testing.T) {
	requireGit(t)
	write := func(path string) {
		if err := os.WriteFile(path, []byte("docs/\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
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
		if c.gitRootFile {
			write(filepath.Join(top, ".semgrepignore"))
		}
		if c.sub {
			write(filepath.Join(sub, ".semgrepignore"))
		}
		args, err := securityScopeArgs(sub, nil)
		if err != nil {
			t.Fatal(err)
		}
		excludes := excludesOf(args)
		if got := contains(excludes, "tests/"); got != c.wantTestDefaults {
			t.Errorf("%s: test defaults present = %v, want %v (%q)", c.name, got, c.wantTestDefaults, excludes)
		}
		if !contains(excludes, "vendor/") || !contains(excludes, "*.min.js") {
			t.Errorf("%s: baseline layer missing: %q", c.name, excludes)
		}
		if contains(args, "--project-root") {
			t.Errorf("%s: subproject scans must keep the Git root for .gitignore: %q", c.name, args)
		}
	}
}

// When the scan root is the Git root nothing changes: one file, one location.
func TestSecurityScopeScanRootEqualToGitRoot(t *testing.T) {
	top := t.TempDir()
	gitInit(t, top)
	args, _ := securityScopeArgs(top, nil)
	if !contains(excludesOf(args), "tests/") {
		t.Fatalf("defaults missing at the Git root: %q", excludesOf(args))
	}
	if err := os.WriteFile(filepath.Join(top, ".semgrepignore"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	args, _ = securityScopeArgs(top, nil)
	if contains(excludesOf(args), "tests/") || !contains(excludesOf(args), "vendor/") {
		t.Fatalf("root file must replace only the test defaults: %q", excludesOf(args))
	}
}

func TestSecurityScopeRejectsInvalidExclusions(t *testing.T) {
	if _, err := securityScopeArgs(t.TempDir(), []string{"!tests"}); err == nil {
		t.Fatal("negated exclusion accepted")
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
	args, err := securityScanArgs(root, defaultExcludes)
	if err != nil {
		t.Fatal(err)
	}
	if after := list(); !reflect.DeepEqual(before, after) {
		t.Fatalf("scope selection changed the project: %v -> %v", before, after)
	}
	if !contains(args, "."+string(os.PathSeparator)+"src") {
		t.Fatalf("root targets missing: %q", args)
	}
}

func TestSecurityScanArgsKeepRootTargetsAndNeverEnumerateDependencies(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"src", "node_modules", ".git", "--option"} {
		if err := os.Mkdir(filepath.Join(root, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	args, err := securityScanArgs(root, defaultExcludes)
	if err != nil {
		t.Fatal(err)
	}
	prefix := "." + string(os.PathSeparator)
	tail := args[len(args)-2:]
	if !reflect.DeepEqual(tail, []string{prefix + "--option", prefix + "src"}) {
		t.Fatalf("targets = %q", tail)
	}
	joined := strings.Join(args, "\x00")
	if strings.Contains(joined, "\x00"+prefix+"node_modules") || strings.Contains(joined, "\x00"+prefix+".git") {
		t.Fatalf("dependency or VCS directory used as target: %q", args)
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
	args, err := securityScanArgs(root, excludes)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(args)
	if err := os.WriteFile(os.Getenv("OXGUARD_COMPARE_ARGS"), data, 0o600); err != nil {
		t.Fatal(err)
	}
}
