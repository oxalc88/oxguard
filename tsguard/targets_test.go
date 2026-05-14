package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunComplexityUsesUltraciteCheck(t *testing.T) {
	r, logPath := newTestRunner(t)

	if code := runComplexity(r, []string{"src"}); code != 0 {
		t.Fatalf("runComplexity returned %d", code)
	}

	entries := readCommandLog(t, logPath)
	if len(entries) != 1 {
		t.Fatalf("expected 1 command, got %d: %v", len(entries), entries)
	}
	if got := entries[0]; got != "npx ultracite check" {
		t.Fatalf("expected ultracite complexity alias, got %q", got)
	}
}

func TestRunCheckDoesNotRunSeparateComplexityStep(t *testing.T) {
	r, logPath := newTestRunner(t)

	if code := runCheck(r, []string{"src", "cdk"}, 60); code != 0 {
		t.Fatalf("runCheck returned %d", code)
	}

	entries := readCommandLog(t, logPath)
	lintRuns := 0
	for _, entry := range entries {
		if entry == "npx ultracite check" {
			lintRuns++
		}
		if strings.Contains(entry, "@biomejs/biome") {
			t.Fatalf("unexpected direct biome invocation: %q", entry)
		}
	}

	if lintRuns != 1 {
		t.Fatalf("expected exactly one ultracite lint pass, got %d: %v", lintRuns, entries)
	}
}

// TestRunCheckNoAuditCall asserts detect-secrets audit is never invoked in check.
func TestRunCheckNoAuditCall(t *testing.T) {
	r, logPath := newTestRunner(t)

	if code := runCheck(r, []string{"src"}, 60); code != 0 {
		t.Fatalf("runCheck returned %d", code)
	}

	for _, entry := range readCommandLog(t, logPath) {
		if strings.Contains(entry, "detect-secrets audit") {
			t.Fatalf("detect-secrets audit must not be called in check, got: %q", entry)
		}
	}
}

// TestSecretsScanDiff_Pass verifies the gate passes when scan matches baseline.
func TestSecretsScanDiff_Pass(t *testing.T) {
	r, logPath := newTestRunner(t)

	// Baseline and scan both empty — no new secrets.
	if err := os.WriteFile(filepath.Join(r.root, ".secrets.baseline"), []byte(`{"results":{}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	if code := runSecrets(r, false); code != 0 {
		t.Fatalf("runSecrets returned %d, want 0", code)
	}

	found := false
	for _, e := range readCommandLog(t, logPath) {
		if strings.HasPrefix(e, "detect-secrets scan") {
			found = true
		}
	}
	if !found {
		t.Fatal("expected detect-secrets scan to be logged")
	}
}

// TestSecretsScanDiff_Fail verifies the gate fails when scan finds a secret not in baseline.
func TestSecretsScanDiff_Fail(t *testing.T) {
	r, _ := newTestRunner(t)

	// Baseline has no entries; scan output reports a new secret.
	if err := os.WriteFile(filepath.Join(r.root, ".secrets.baseline"), []byte(`{"results":{}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DETECT_SECRETS_SCAN_OUTPUT",
		`{"results":{"src/api.ts":[{"hashed_secret":"abc123","type":"Hex High Entropy String","line_number":42}]}}`)

	if code := runSecrets(r, false); code == 0 {
		t.Fatal("runSecrets returned 0, want non-zero (new secret detected)")
	}
}

func newTestRunner(t *testing.T) (*Runner, string) {
	t.Helper()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".secrets.baseline"), []byte(`{"results":{}}`), 0o644); err != nil {
		t.Fatalf("write baseline: %v", err)
	}

	binDir := filepath.Join(root, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("mkdir bin: %v", err)
	}

	logPath := filepath.Join(root, "commands.log")
	for _, name := range []string{"npx", "npm", "semgrep"} {
		writeFakeCommand(t, filepath.Join(binDir, name))
	}
	// detect-secrets needs to emit JSON for scan and respect DETECT_SECRETS_SCAN_OUTPUT.
	writeDetectSecretsCommand(t, filepath.Join(binDir, "detect-secrets"))

	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("TSGUARD_LOG", logPath)

	return &Runner{root: root, timeout: 5}, logPath
}

func writeFakeCommand(t *testing.T, path string) {
	t.Helper()

	script := "#!/bin/sh\n" +
		"printf '%s %s\\n' \"$(basename \"$0\")\" \"$*\" >> \"$TSGUARD_LOG\"\n" +
		"exit 0\n"

	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake command %s: %v", path, err)
	}
}

func writeDetectSecretsCommand(t *testing.T, path string) {
	t.Helper()

	// Logs args like other fakes; for `scan`, emits JSON from DETECT_SECRETS_SCAN_OUTPUT
	// (or an empty-results object by default) so the scan+diff gate can parse it.
	script := "#!/bin/sh\n" +
		"printf '%s %s\\n' \"$(basename \"$0\")\" \"$*\" >> \"$TSGUARD_LOG\"\n" +
		"if [ \"$1\" = \"scan\" ]; then\n" +
		"  if [ -n \"$DETECT_SECRETS_SCAN_OUTPUT\" ]; then\n" +
		"    printf '%s' \"$DETECT_SECRETS_SCAN_OUTPUT\"\n" +
		"  else\n" +
		"    printf '{\"results\":{}}'\n" +
		"  fi\n" +
		"fi\n" +
		"exit 0\n"

	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake detect-secrets: %v", err)
	}
}

func readCommandLog(t *testing.T, path string) []string {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read command log: %v", err)
	}

	content := strings.TrimSpace(string(data))
	if content == "" {
		return nil
	}

	return strings.Split(content, "\n")
}
