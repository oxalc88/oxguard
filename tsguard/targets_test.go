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

func newTestRunner(t *testing.T) (*Runner, string) {
	t.Helper()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".secrets.baseline"), []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("write baseline: %v", err)
	}

	binDir := filepath.Join(root, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("mkdir bin: %v", err)
	}

	logPath := filepath.Join(root, "commands.log")
	for _, name := range []string{"npx", "npm", "semgrep", "detect-secrets"} {
		writeFakeCommand(t, filepath.Join(binDir, name))
	}

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
