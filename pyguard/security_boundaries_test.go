package main

import (
 "os"
 "path/filepath"
 "strings"
 "testing"
)

func TestSecretsScansConfiguredProjectDirectories(t *testing.T) {
 if testing.Short() { t.Skip("shell fixture") }
 root := t.TempDir()
 if err := os.WriteFile(filepath.Join(root, ".secrets.baseline"), []byte("{}"), 0o600); err != nil { t.Fatal(err) }
 binDir := t.TempDir()
 log := filepath.Join(root, "scanned.log")
 shim := "#!/bin/sh\nprintf '%s' \"$*\" > \"$PYGUARD_SCAN_LOG\"\n"
 if err := os.WriteFile(filepath.Join(binDir, "uv"), []byte(shim), 0o700); err != nil { t.Fatal(err) }
 t.Setenv("PYGUARD_SCAN_LOG", log)
 t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
 runner := &Runner{root: root, dirs: []string{"src", "tests"}, timeout: 5}
 if got := runSecrets(runner, config{}); got != 0 { t.Fatalf("secrets returned %d", got) }
 data, err := os.ReadFile(log)
 if err != nil { t.Fatal(err) }
 if !strings.Contains(string(data), "src tests") || strings.Contains(string(data), "functions cdk scripts") {
  t.Fatalf("incorrect secrets scan scope: %s", data)
 }
}

func TestSetupNeverGlobalizesProjectBinary(t *testing.T) {
 root := t.TempDir()
 if _, err := ensureRepoPknBinary(root); err != nil { t.Fatal(err) }
 if enabled, _ := installPkn(root); enabled { t.Fatal("setup installed project-owned binary globally") }
}
