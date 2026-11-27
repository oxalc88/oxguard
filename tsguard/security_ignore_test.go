package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLegacySecurityIgnore(t *testing.T) {
	cases := []struct {
		name, config  string
		ignored, kept []string
	}{
		{"defaults", legacySecurityIgnores, []string{"tests/a.ts", "src/tests/a.ts", "src/a.min.js"}, []string{"src/a.ts", "test.ts"}},
		{"range-negation", "[ab].ts\nsrc/*.ts\n!src/a.ts\n", []string{"src/a.ts", "src/nested/b.ts"}, []string{"a.ts", "b.ts"}},
		{"empty-after-unescape", "\\\n", []string{"src/a.ts", "x.ts"}, nil},
		{"double-slash", "foo//\n", nil, []string{"foo/a.ts", "x/foo/a.ts"}},
		{"anchoring", "/src/a.ts\ncache/\n", []string{"src/a.ts", "x/cache/a.ts"}, []string{"x/src/a.ts", "cache"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			patterns, err := parseLegacyIgnores(c.config, t.TempDir(), map[string]bool{})
			if err != nil {
				t.Fatal(err)
			}
			matches := func(path string) bool {
				for _, p := range patterns {
					if p.MatchString(path) {
						return true
					}
				}
				return false
			}
			for _, path := range c.ignored {
				if !matches(path) {
					t.Errorf("did not exclude %s", path)
				}
			}
			for _, path := range c.kept {
				if matches(path) {
					t.Errorf("excluded %s", path)
				}
			}
		})
	}
}

func TestSecurityIgnoreSnapshot(t *testing.T) {
	root := t.TempDir()
	for name, content := range map[string]string{"src/a.ts": "eval('x')", "src/.semgrepignore": "a.ts", "tests/a.ts": "eval('x')", "a.ts": "eval('x')"} {
		path := filepath.Join(root, name)
		os.MkdirAll(filepath.Dir(path), 0700)
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	args, cleanup, err := securityIgnoreSnapshot(root)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	data, err := os.ReadFile(filepath.Join(root, args[len(args)-1]))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "/tests/a.ts\n") || strings.Contains(string(data), "/src/a.ts") {
		t.Fatalf("incorrect snapshot: %s", data)
	}
	os.WriteFile(filepath.Join(root, ".semgrepignore"), []byte(":include extra.ignore\n[a].ts\n!src/a.ts\n"), 0600)
	os.WriteFile(filepath.Join(root, "extra.ignore"), []byte("src/a.ts\n"), 0600)
	args2, cleanup2, err := securityIgnoreSnapshot(root)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup2()
	data, _ = os.ReadFile(filepath.Join(root, args2[len(args2)-1]))
	if !strings.Contains(string(data), "/src/a.ts\n") || strings.Contains(string(data), "/tests/a.ts") || strings.Contains("\n"+string(data), "\n/a.ts\n") {
		t.Fatalf("incorrect custom snapshot: %s", data)
	}
	os.WriteFile(filepath.Join(root, "extra.ignore"), []byte(":include .semgrepignore\n"), 0600)
	if _, _, err := securityIgnoreSnapshot(root); err == nil {
		t.Fatal("include cycle must fail closed")
	}
}

// The comparison harness uses the production selector, not a duplicate Python port.
func TestSecurityIgnoreComparisonHelper(t *testing.T) {
	root := os.Getenv("OXGUARD_COMPARE_ROOT")
	if root == "" {
		t.Skip("comparison harness only")
	}
	args, _, err := securityIgnoreSnapshot(root)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(args)
	if err := os.WriteFile(os.Getenv("OXGUARD_COMPARE_ARGS"), data, 0600); err != nil {
		t.Fatal(err)
	}
}

func TestSecurityIgnoreSnapshotFailureCleansUp(t *testing.T) {
	if os.PathSeparator != '/' {
		t.Skip("newline filename is a Unix fixture")
	}
	root := t.TempDir()
	os.Mkdir(filepath.Join(root, "tests"), 0700)
	if err := os.WriteFile(filepath.Join(root, "tests", "bad\nname.ts"), []byte("eval('x')"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := securityIgnoreSnapshot(root); err == nil {
		t.Fatal("expected unrepresentable ignore path to fail closed")
	}
	files, _ := filepath.Glob(filepath.Join(root, ".oxguard-security-ignore-*"))
	if len(files) != 0 {
		t.Fatalf("snapshot leak on error: %v", files)
	}
}
