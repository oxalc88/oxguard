package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestPackagedToolsDoNotInvokeProjectPackageManager(t *testing.T) {
	runtime := filepath.Join(t.TempDir(), "runtime with spaces")
	t.Setenv("TSGUARD_RUNTIME", runtime)
	t.Setenv("TSGUARD_NODE", "/node")
	for _, pm := range []string{"npm", "pnpm", "yarn"} {
		got := pkgExec(pm, "fta", "src with spaces", "--score-cap", "60")
		want := []string{"/node", filepath.Join(runtime, "bin", "tool.cjs"), "fta", "src with spaces", "--score-cap", "60"}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("%s: got %v, want %v", pm, got, want)
		}
	}
}

func TestPackagedSASTFailsWhenEngineIsMissing(t *testing.T) {
	r, _ := newTestRunner(t)
	t.Setenv("TSGUARD_RUNTIME", filepath.Join(r.root, "runtime"))
	t.Setenv("TSGUARD_OPENGREP", filepath.Join(r.root, "missing-opengrep"))
	if got := runOpengrep(r); got != 1 {
		t.Fatalf("missing bundled SAST returned %d", got)
	}
}

func TestStandaloneSASTRetainsMissingEngineBehavior(t *testing.T) {
	r, _ := newTestRunner(t)
	t.Setenv("TSGUARD_RUNTIME", "")
	if got := runOpengrep(r); got != 0 {
		t.Fatalf("standalone missing SAST returned %d", got)
	}
}

func TestPackagedTypesPreservesProjectConfiguration(t *testing.T) {
	r, _ := newTestRunner(t)
	node := filepath.Join(r.root, "bin", "node")
	writeFakeCommand(t, node)
	t.Setenv("TSGUARD_RUNTIME", filepath.Join(r.root, "runtime"))
	t.Setenv("TSGUARD_NODE", node)
	file := filepath.Join(r.root, "tsconfig.json")
	before := []byte(`{"compilerOptions":{"strict":false}}`)
	if err := os.WriteFile(file, before, 0o644); err != nil {
		t.Fatal(err)
	}
	if got := r.runPackagedTypes(); got != 0 {
		t.Fatalf("types returned %d", got)
	}
	after, err := os.ReadFile(file)
	if err != nil || string(after) != string(before) {
		t.Fatalf("project config changed: %q, %v", after, err)
	}
	if _, err := os.Stat(filepath.Join(r.root, opengrepCacheDir, "defaults")); !os.IsNotExist(err) {
		t.Fatalf("unexpected fallback config: %v", err)
	}
}
