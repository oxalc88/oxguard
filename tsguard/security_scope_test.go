package main

import (
	"encoding/json"
	"errors"
	"fmt"
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
	if scope.restoreNestedTests {
		t.Fatal("outside Git there is no nested-test gap")
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
// baseline layer applies in every case. Only the scan-root-only case needs the
// nested-test restoration.
func TestSecurityScopeSubprojectIgnoreFileReplacesTestDefaults(t *testing.T) {
	requireGit(t)
	cases := []struct {
		name                  string
		gitRootFile, sub      bool
		wantTestDefaults      bool
		wantRestoreNestedTest bool
	}{
		{"no files: defaults apply", false, false, true, false},
		{"scan-root file only", false, true, false, true},
		{"git-root file only", true, false, false, false},
		{"both files", true, true, false, false},
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
		if scope.restoreNestedTests != c.wantRestoreNestedTest {
			t.Errorf("%s: restoreNestedTests = %v, want %v", c.name, scope.restoreNestedTests, c.wantRestoreNestedTest)
		}
	}
}

// When the scan root is the Git root nothing changes: one file, one location,
// and the root file is the project file, so there is no nested-test gap.
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
	if contains(excludesOf(scope.args), "tests/") || !contains(excludesOf(scope.args), "vendor/") || scope.restoreNestedTests {
		t.Fatalf("root file must replace only the test defaults: %q restore=%v", excludesOf(scope.args), scope.restoreNestedTests)
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
	scan, err := securityScanArgs(root, defaultExcludes, "unused-engine")
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
	scan, err := securityScanArgs(root, defaultExcludes, "unused-engine")
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
	scan, err := securityScanArgs(root, defaultExcludes, "unused-engine")
	if err != nil || !scan.empty || len(scan.args) != 0 {
		t.Fatalf("scan = %+v, err = %v; want an explicit empty selection", scan, err)
	}
	// A project with no root entries at all keeps its configuration error.
	if _, err := securityScanArgs(t.TempDir(), defaultExcludes, "unused-engine"); err == nil {
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

func stubNestedEngine(t *testing.T, listed []string, ignored map[string]bool) *[]string {
	t.Helper()
	oldList, oldIgnored := listSelectedFiles, gitIgnoredPaths
	t.Cleanup(func() { listSelectedFiles, gitIgnoredPaths = oldList, oldIgnored })
	query := new([]string)
	listSelectedFiles = func(engine, root string, args []string) ([]string, error) {
		*query = args
		return listed, nil
	}
	gitIgnoredPaths = func(root string, paths []string) (map[string]bool, error) { return ignored, nil }
	return query
}

// makeFiles creates each slash-separated path (file names may hold any byte but / and NUL).
func makeFiles(t *testing.T, root string, paths ...string) {
	t.Helper()
	for _, rel := range paths {
		full := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("eval('x')"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func nestedPath(rel string) string { return "." + string(os.PathSeparator) + filepath.FromSlash(rel) }

// nestedTestTargets: Opengrep lists the selected files, OxGuard derives targets.
func TestNestedTestTargets(t *testing.T) {
	root := t.TempDir()
	listed := []string{
		"a_test.go",   // root-level file: already an explicit target
		"tests/t0.ts", // root entry itself: already a target
		"src/a.ts", "src/tests/t.ts",
		"src/tests/deep/tests/n.ts", // nested inside a nested test directory
		"src/tests/c_test.go", "src/a_test.go", "src/ignored_test.go",
		"lib/pkg/testsuite/s.ts", "lib/pkg/test/u.ts", "src/tests/b/c.ts",
		"src/contest/x.ts",                                           // not a test directory name
		"src/foo_test.go/a.ts", "src/foo_test.go/inner_test.go/b.ts", // directories matching *_test.go
		"src/Tests/c.ts", "src/TEST/d.ts", "src/tests2/e.ts", // other case or name: not built-in test paths
	}
	makeFiles(t, root, listed...)
	query := stubNestedEngine(t, listed, map[string]bool{"src/ignored_test.go": true})
	targets, err := nestedTestTargets("engine", root, []string{"--exclude", "**/dist"}, []string{"./src", "./lib", "./tests"})
	if err != nil {
		t.Fatal(err)
	}
	var want []string
	for _, rel := range []string{"lib/pkg/test", "lib/pkg/testsuite", "src/foo_test.go", "src/foo_test.go/inner_test.go", "src/tests", "src/tests/deep/tests", "src/a_test.go", "src/tests/c_test.go"} {
		want = append(want, nestedPath(rel))
	}
	if !reflect.DeepEqual(targets, want) {
		t.Fatalf("targets = %q, want %q", targets, want)
	}
	seen := map[string]bool{}
	for _, target := range targets {
		if seen[target] {
			t.Fatalf("duplicate target %q", target)
		}
		seen[target] = true
	}
	// The query makes the scan root the project, passes the exclusions and lists only root entries.
	if wantQuery := []string{"--project-root", ".", "--exclude", "**/dist", "./src", "./lib", "./tests"}; !reflect.DeepEqual(*query, wantQuery) {
		t.Fatalf("query = %q, want %q", *query, wantQuery)
	}
}

// The derivation uses exactly the built-in list: directories named like *_test.go
// are included, other case variants and near names are not.
func TestBuiltinTestPathMatching(t *testing.T) {
	cases := []struct {
		name  string
		isDir bool
		want  bool
	}{
		{"test", true, true}, {"tests", true, true}, {"testsuite", true, true}, {"foo_test.go", true, true}, {"_test.go", true, true},
		{"a_test.go", false, true}, {"test", false, false}, {"tests", false, false}, {"testsuite", false, false},
		{"Tests", true, false}, {"TEST", true, false}, {"tests2", true, false}, {"contest", true, false}, {"a_test.go.bak", false, false}, {"a_test.GO", false, false},
		{"foo_test.go", true, true},
	}
	for _, c := range cases {
		if got := hiddenByBuiltins(c.name, c.isDir); got != c.want {
			t.Errorf("hiddenByBuiltins(%q, dir=%v) = %v, want %v", c.name, c.isDir, got, c.want)
		}
	}
}

// Hostile file names: a newline, carriage return, U+0085 or U+2028 inside a name is one path, never two
// (JSON serialization), and each such target is passed intact. Names starting with "-" get a "./" prefix.
func TestNestedTestTargetsAdversarialNames(t *testing.T) {
	if os.PathSeparator != '/' {
		t.Skip("control characters are not valid in Windows file names")
	}
	root := t.TempDir()
	names := []string{
		"src/decoy\nsrc/hidden_test.go", // forged second path inside one name
		"src/cr\r/tests/m.ts", "src/u\u0085y/tests/l.ts", "src/u\u2028x/tests/k.ts",
		"src/-tests/f.ts", "src/--x/tests/g.ts", "src/a b \"q\" 's'/tests/h.ts", "src/a*b[1]/tests/j.ts",
		"src/x\n../outside_test.go", // a name that merely looks like a parent reference
		"src/hidden_test.go",        // the clean, ignored file the forged path imitates is not in the listing
	}
	makeFiles(t, root, names[:len(names)-1]...)
	listed := names[:len(names)-1]
	stubNestedEngine(t, listed, map[string]bool{})
	targets, err := nestedTestTargets("engine", root, nil, []string{"./src"})
	if err != nil {
		t.Fatal(err)
	}
	for _, target := range targets {
		if target == nestedPath("src/hidden_test.go") || strings.HasPrefix(target, ".."+string(os.PathSeparator)) || !strings.HasPrefix(target, "./") {
			t.Fatalf("unsafe or forged target %q", target)
		}
		if _, err := os.Lstat(filepath.Join(root, target)); err != nil {
			t.Fatalf("target %q does not exist: %v", target, err)
		}
	}
	want := map[string]bool{}
	for _, rel := range []string{"src/decoy\nsrc/hidden_test.go", "src/x\n../outside_test.go", "src/cr\r/tests", "src/u\u0085y/tests", "src/u\u2028x/tests",
		"src/--x/tests", "src/a b \"q\" 's'/tests", "src/a*b[1]/tests"} {
		want[nestedPath(rel)] = true
	}
	if len(targets) != len(want) {
		t.Fatalf("targets = %q", targets)
	}
	for _, target := range targets {
		if !want[target] {
			t.Fatalf("unexpected target %q in %q", target, targets)
		}
	}
}

// Containment: a derived target must be a clean relative path to a real file or
// directory below the scan root. Nothing is guessed and the path is quoted in the error.
func TestNestedTargetContainment(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	makeFiles(t, root, "src/tests/t.ts", "src/a_test.go")
	makeFiles(t, outside, "o/tests/x.ts", "z_test.go")
	if os.Symlink(outside, filepath.Join(root, "src", "link")) == nil {
		// A symlinked directory in a listing (Opengrep never emits one) must not be followed out of the root.
		if _, err := nestedTarget(root, "src/link/o/tests", true); err == nil {
			t.Error("symlinked ancestor escaped the scan root")
		}
		if _, err := nestedTarget(root, "src/link", true); err == nil {
			t.Error("symlinked directory accepted as a target")
		}
	}
	if os.Symlink(filepath.Join(outside, "z_test.go"), filepath.Join(root, "src", "evil_test.go")) == nil {
		if _, err := nestedTarget(root, "src/evil_test.go", false); err == nil {
			t.Error("symlinked file accepted as a target")
		}
	}
	for _, rel := range []string{"", "../outside/tests", "src/../../x", "/etc/tests", "src//tests", "src/./tests", "src/nope/tests", "src/tests\x00", "C:/tests"} {
		if _, err := nestedTarget(root, rel, true); err == nil {
			t.Errorf("%q accepted", rel)
		}
	}
	if _, err := nestedTarget(root, "src/a_test.go", true); err == nil {
		t.Error("a file accepted as a directory target")
	}
	if _, err := nestedTarget(root, "src/tests", false); err == nil {
		t.Error("a directory accepted as a file target")
	}
	var tool *scopeToolError
	_, err := nestedTarget(root, "src/bad\n../x", true)
	if !errors.As(err, &tool) || strings.Contains(err.Error(), "\n") {
		t.Errorf("error must be a quoted tool error: %v", err)
	}
	if got, err := nestedTarget(root, "src/tests", true); err != nil || got != nestedPath("src/tests") {
		t.Errorf("valid target = %q, %v", got, err)
	}
}

func TestNestedTestTargetsFailClosed(t *testing.T) {
	root := t.TempDir()
	oldList, oldIgnored := listSelectedFiles, gitIgnoredPaths
	defer func() { listSelectedFiles, gitIgnoredPaths = oldList, oldIgnored }()
	gitIgnoredPaths = func(string, []string) (map[string]bool, error) { return map[string]bool{}, nil }
	listSelectedFiles = func(string, string, []string) ([]string, error) { return nil, errors.New("engine failed") }
	var tool *scopeToolError
	if _, err := nestedTestTargets("engine", root, nil, []string{"./src"}); !errors.As(err, &tool) {
		t.Fatalf("listing failure must be a tool error: %v", err)
	}
	// A listed path that does not exist (or tries to escape) fails the whole scan.
	for _, bad := range []string{"src/tests/missing.ts", "../x/tests/a.ts", "/abs/tests/a.ts"} {
		listSelectedFiles = func(string, string, []string) ([]string, error) { return []string{"src/ok.ts", bad}, nil }
		if _, err := nestedTestTargets("engine", root, nil, []string{"./src"}); err == nil && !strings.Contains(bad, "missing") {
			t.Errorf("%q accepted", bad)
		}
	}
	// A NUL byte in a listed path cannot be a file name.
	listSelectedFiles = func(string, string, []string) ([]string, error) { return []string{"src/tests\x00/a.ts"}, nil }
	if _, err := nestedTestTargets("engine", root, nil, []string{"./src"}); err == nil {
		t.Error("NUL in a listed path accepted")
	}
	// Too many targets for a bounded argv.
	var many []string
	for i := 0; i < 1500; i++ {
		many = append(many, fmt.Sprintf("src/package-%04d/tests/a.ts", i))
	}
	makeFiles(t, root, many...)
	listSelectedFiles = func(string, string, []string) ([]string, error) { return many, nil }
	_, err := nestedTestTargets("engine", root, nil, []string{"./src"})
	var config *scopeConfigError
	if !errors.As(err, &config) || !strings.Contains(err.Error(), "empty .semgrepignore at the Git root") {
		t.Fatalf("an oversized target list must fail closed with the workaround: %v", err)
	}
}

// git check-ignore drops only ignored, untracked files; tracked files stay.
func TestGitIgnoredPathsUsesGit(t *testing.T) {
	requireGit(t)
	root := t.TempDir()
	gitInit(t, root)
	for name, content := range map[string]string{".gitignore": "gen_test.go\n", "gen_test.go": "x", "keep_test.go": "x", "a b_test.go": "x"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	ignored, err := gitIgnoredPaths(root, []string{"gen_test.go", "keep_test.go", "a b_test.go"})
	if err != nil {
		t.Fatal(err)
	}
	if !ignored["gen_test.go"] || ignored["keep_test.go"] || ignored["a b_test.go"] {
		t.Fatalf("ignored = %v", ignored)
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
	scan, err := securityScanArgs(root, excludes, os.Getenv("OXGUARD_COMPARE_ENGINE"))
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
