package hooks

import (
 "os"
 "path/filepath"
 "strings"
 "testing"
)

func TestHookInstallDoesNotChangeParentWorkspace(t *testing.T) {
 workspace := t.TempDir()
 root := filepath.Join(workspace, "nested-project")
 if err := os.MkdirAll(root, 0o700); err != nil { t.Fatal(err) }
 parentConfig := filepath.Join(workspace, "AGENTS.md")
 if err := os.WriteFile(parentConfig, []byte("KEEP PARENT CONTENT"), 0o600); err != nil { t.Fatal(err) }
 if err := GenerateCodexHook(root, []byte("project instructions")); err != nil { t.Fatal(err) }
 if err := GenerateClaudeHook(root); err != nil { t.Fatal(err) }
 if err := GeneratePreCommit(root); err != nil { t.Fatal(err) }
 data, err := os.ReadFile(parentConfig)
 if err != nil || string(data) != "KEEP PARENT CONTENT" { t.Fatalf("parent changed: %s %v", data, err) }
 for _, name := range []string{"AGENTS.md", ".claude/settings.local.json", ".pre-commit-config.yaml"} {
  if _, err := os.Stat(filepath.Join(root, name)); err != nil { t.Fatalf("%s missing in project: %v", name, err) }
 }
}

func TestHookInstallRefusesOverwriteAndSymlink(t *testing.T) {
 root := t.TempDir()
 path := filepath.Join(root, "config.json")
 if err := os.WriteFile(path, []byte("existing"), 0o600); err != nil { t.Fatal(err) }
 if err := writeHookFile(path, "replacement"); err == nil { t.Fatal("overwrote user configuration") }
 data, _ := os.ReadFile(path)
 if string(data) != "existing" { t.Fatal("existing configuration altered") }
 if err := os.Remove(path); err != nil { t.Fatal(err) }
 target := filepath.Join(root, "outside")
 if err := os.WriteFile(target, []byte("outside"), 0o600); err != nil { t.Fatal(err) }
 if err := os.Symlink(target, path); err == nil {
  if err := writeHookFile(path, "replacement"); err == nil { t.Fatal("accepted symlink hook target") }
  data, _ = os.ReadFile(target)
  if string(data) != "outside" { t.Fatal("symlink target altered") }
 }
}

func TestHookCommandIsNotProjectBinary(t *testing.T) {
 bin := pyguardBinary("/untrusted")
 if strings.Contains(bin, "/untrusted") || strings.Contains(bin, "tools/") { t.Fatalf("project-controlled binary: %s", bin) }
}
