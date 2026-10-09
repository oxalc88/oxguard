package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestSecurityRootTargetsPreserveProjectScopeWithoutDependencies(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"src", "outside-configured-scope", "node_modules", ".git", "--option"} {
		if err := os.Mkdir(filepath.Join(root, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "root.ts"), []byte("eval('root')"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(t.TempDir(), filepath.Join(root, "external")); err != nil {
		t.Logf("symlink control unavailable: %v", err)
	}
	targets, err := securityRootTargets(root)
	if err != nil {
		t.Fatal(err)
	}
	prefix := "." + string(os.PathSeparator)
	want := []string{prefix + "--option", prefix + "outside-configured-scope", prefix + "root.ts", prefix + "src"}
	if !reflect.DeepEqual(targets, want) {
		t.Fatalf("security targets = %q, want %q", targets, want)
	}
}

func TestSecurityRootTargetsRejectUnavailableRoot(t *testing.T) {
	if _, err := securityRootTargets(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("unavailable root accepted")
	}
}
