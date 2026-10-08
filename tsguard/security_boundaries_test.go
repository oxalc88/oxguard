package main

import (
 "os"
 "path/filepath"
 "runtime"
 "strings"
 "testing"
)

func TestSecurityScanDirectoriesCannotEscapeProject(t *testing.T) {
 root := t.TempDir()
 if err := os.Mkdir(filepath.Join(root, "src"), 0o755); err != nil { t.Fatal(err) }
 for _, dir := range []string{".", "src", "not-created-yet"} {
  if err := validateScanDirs(root, []string{dir}); err != nil { t.Fatalf("%q: %v", dir, err) }
 }
 for _, dir := range []string{"../sibling", "src/../../sibling", "-o", "--output", filepath.Dir(root), ".."} {
  if err := validateScanDirs(root, []string{dir}); err == nil { t.Errorf("accepted unsafe scope %q", dir) }
 }
 outside := t.TempDir()
 if err := os.Symlink(outside, filepath.Join(root, "external")); err == nil {
  for _, dir := range []string{"external", "external/not-created-yet"} {
   if err := validateScanDirs(root, []string{dir}); err == nil { t.Errorf("accepted linked scope %q", dir) }
  }
 }
}

func TestProjectConfigCannotEscapeProject(t *testing.T) {
 root := t.TempDir()
 for _, dir := range []string{"../sibling", "/tmp", "-write"} {
  conf := "dirs = [" + strconvQuote(dir) + "]\n"
  if err := os.WriteFile(filepath.Join(root, "oxguard.toml"), []byte(conf), 0o644); err != nil { t.Fatal(err) }
  if _, err := buildConfig(config{output: "json"}, root); err == nil { t.Errorf("accepted project config: %s", conf) }
 }
}

func strconvQuote(s string) string { return "\"" + strings.ReplaceAll(s, "\"", "\\\"") + "\"" }

func TestStandaloneSecretlintRejectsProjectExecutableConfig(t *testing.T) {
 t.Setenv("TSGUARD_RUNTIME", "")
 r, _ := newTestRunner(t)
 if err := os.WriteFile(filepath.Join(r.root, ".secretlintrc.js"), []byte("process.exit(19)"), 0o644); err != nil { t.Fatal(err) }
 if got := runSecretlint(r); got != 1 { t.Fatalf("executable configuration accepted: %d", got) }
}

func TestStandaloneSecretlintAlwaysScansRoot(t *testing.T) {
 t.Setenv("TSGUARD_RUNTIME", "")
 r, logPath := newTestRunner(t)
 r.dirs = []string{"empty"}
 if err := runSecretlint(r); err != 0 { t.Fatalf("secretlint exit %d", err) }
 logged := strings.Join(readCommandLog(t, logPath), "\n")
 if !strings.Contains(logged, "secretlint") || !strings.HasSuffix(logged, " .") {
  t.Fatalf("security scope restricted by project config: %s", logged)
 }
}

func TestStandaloneEngineRejectsProjectCacheAndSymlink(t *testing.T) {
 if runtime.GOOS == "windows" { t.Skip("symlink creation may require developer mode") }
 t.Setenv("TSGUARD_RUNTIME", "")
 t.Setenv("XDG_CACHE_HOME", t.TempDir())
 t.Setenv("HOME", t.TempDir())
 cache := opengrepBinaryPath(t.TempDir())
 if cache == "" { t.Fatal("cache directory unavailable") }
 if err := os.MkdirAll(filepath.Dir(cache), 0o700); err != nil { t.Fatal(err) }
 if err := os.WriteFile(cache, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil { t.Fatal(err) }
 if err := verifyOpengrepBinary(cache); err == nil { t.Fatal("untrusted engine accepted") }
 if err := os.Remove(cache); err != nil { t.Fatal(err) }
 if err := os.Symlink(filepath.Join(t.TempDir(), "attacker"), cache); err != nil { t.Skipf("symlink unavailable: %v", err) }
 if err := verifyOpengrepBinary(cache); err == nil { t.Fatal("symlinked engine accepted") }
}
