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
	// runLint now appends dirs to the ultracite check command.
	if got := entries[0]; !strings.HasPrefix(got, "npx ultracite check") {
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
		// runLint now appends dirs to the command (e.g. "npx ultracite check src cdk").
		if strings.HasPrefix(entry, "npx ultracite check") {
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

// TestRunCheckNoDetectSecretsCall asserts detect-secrets (old Python tool) is never invoked.
func TestRunCheckNoDetectSecretsCall(t *testing.T) {
	r, logPath := newTestRunner(t)

	if code := runCheck(r, []string{"src"}, 60); code != 0 {
		t.Fatalf("runCheck returned %d", code)
	}

	for _, entry := range readCommandLog(t, logPath) {
		if strings.Contains(entry, "detect-secrets") {
			t.Fatalf("detect-secrets must not be called in check, got: %q", entry)
		}
	}
}

// TestRunCheckNoPythonToolCalls asserts no Python tools (semgrep, detect-secrets, pipx, uv) are invoked.
func TestRunCheckNoPythonToolCalls(t *testing.T) {
	r, logPath := newTestRunner(t)

	if code := runCheck(r, []string{"src"}, 60); code != 0 {
		t.Fatalf("runCheck returned %d", code)
	}

	pythonTools := []string{"semgrep", "detect-secrets", "pipx", "uv tool"}
	for _, entry := range readCommandLog(t, logPath) {
		for _, tool := range pythonTools {
			if strings.Contains(entry, tool) {
				t.Fatalf("Python tool %q must not be called in tsguard check, got: %q", tool, entry)
			}
		}
	}
}

// TestRunSecretlintInvokesSecretlint verifies runSecretlint delegates to secretlint via pkgExec.
func TestRunSecretlintInvokesSecretlint(t *testing.T) {
	r, logPath := newTestRunner(t)

	if code := runSecretlint(r); code != 0 {
		t.Fatalf("runSecretlint returned %d", code)
	}

	for _, entry := range readCommandLog(t, logPath) {
		if strings.Contains(entry, "secretlint") {
			return
		}
	}
	t.Fatalf("expected secretlint in command log, got: %v", readCommandLog(t, logPath))
}

// TestDetectPackageManager checks lockfile and packageManager-field resolution.
func TestDetectPackageManager(t *testing.T) {
	cases := []struct {
		name   string
		files  map[string]string // filename → content
		wantPM string
	}{
		{
			name:   "pnpm lockfile",
			files:  map[string]string{"pnpm-lock.yaml": ""},
			wantPM: "pnpm",
		},
		{
			name:   "yarn lockfile",
			files:  map[string]string{"yarn.lock": ""},
			wantPM: "yarn",
		},
		{
			name:   "npm lockfile",
			files:  map[string]string{"package-lock.json": "{}"},
			wantPM: "npm",
		},
		{
			name:   "packageManager field pnpm wins over no lockfile",
			files:  map[string]string{"package.json": `{"packageManager":"pnpm@9.0.0"}`},
			wantPM: "pnpm",
		},
		{
			name:   "packageManager field yarn",
			files:  map[string]string{"package.json": `{"packageManager":"yarn@4.0.0"}`},
			wantPM: "yarn",
		},
		{
			name: "packageManager field wins over lockfile",
			files: map[string]string{
				"package.json":      `{"packageManager":"pnpm@9.0.0"}`,
				"package-lock.json": "{}",
			},
			wantPM: "pnpm",
		},
		{
			name:   "no lockfile defaults to npm",
			files:  map[string]string{"package.json": `{}`},
			wantPM: "npm",
		},
		{
			name:   "empty root defaults to npm",
			files:  map[string]string{},
			wantPM: "npm",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			for name, content := range tc.files {
				if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			got := detectPackageManager(dir)
			if got != tc.wantPM {
				t.Fatalf("detectPackageManager = %q, want %q", got, tc.wantPM)
			}
		})
	}
}

// TestDetectPackageManagerWorkspace verifies lockfile detection walks up to the workspace root.
// In pnpm/yarn workspaces the lockfile lives at the workspace root, not in each subpackage.
func TestDetectPackageManagerWorkspace(t *testing.T) {
	cases := []struct {
		name     string
		lockfile string
		wantPM   string
	}{
		{"pnpm workspace lockfile at grandparent", "pnpm-lock.yaml", "pnpm"},
		{"yarn workspace lockfile at grandparent", "yarn.lock", "yarn"},
		{"npm workspace lockfile at grandparent", "package-lock.json", "npm"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			workspaceRoot := t.TempDir()
			subpkg := filepath.Join(workspaceRoot, "packages", "foundation")
			if err := os.MkdirAll(subpkg, 0o755); err != nil {
				t.Fatal(err)
			}
			// Lockfile at workspace root only — not in the subpackage.
			if err := os.WriteFile(filepath.Join(workspaceRoot, tc.lockfile), []byte(""), 0o644); err != nil {
				t.Fatal(err)
			}
			got := detectPackageManager(subpkg)
			if got != tc.wantPM {
				t.Fatalf("detectPackageManager = %q, want %q", got, tc.wantPM)
			}
		})
	}
}

// TestPkgExecSelectsCorrectRunner checks pkgExec returns the right command prefix.
func TestPkgExecSelectsCorrectRunner(t *testing.T) {
	cases := []struct {
		pm   string
		tool string
		want string // first two words of joined result
	}{
		{"npm", "tsc", "npx tsc"},
		{"", "tsc", "npx tsc"}, // empty defaults to npm
		{"pnpm", "tsc", "pnpm exec tsc"},
		{"yarn", "tsc", "yarn exec tsc"},
	}
	for _, tc := range cases {
		got := strings.Join(pkgExec(tc.pm, tc.tool), " ")
		if got != tc.want {
			t.Errorf("pkgExec(%q, %q) = %q, want %q", tc.pm, tc.tool, got, tc.want)
		}
	}
}

// TestRunNpmAuditUsesPnpm verifies pnpm audit is invoked in a pnpm project.
func TestRunNpmAuditUsesPnpm(t *testing.T) {
	r, logPath := newTestRunner(t)
	r.pkgManager = "pnpm"

	if code := runNpmAudit(r); code != 0 {
		t.Fatalf("runNpmAudit returned %d", code)
	}

	for _, entry := range readCommandLog(t, logPath) {
		if strings.HasPrefix(entry, "pnpm audit") {
			return
		}
	}
	t.Fatalf("expected pnpm audit in log, got: %v", readCommandLog(t, logPath))
}

// TestRunNpmAuditDefaultsToNpm verifies npm audit is used when no PM is set.
func TestRunNpmAuditDefaultsToNpm(t *testing.T) {
	r, logPath := newTestRunner(t)

	if code := runNpmAudit(r); code != 0 {
		t.Fatalf("runNpmAudit returned %d", code)
	}

	for _, entry := range readCommandLog(t, logPath) {
		if strings.HasPrefix(entry, "npm audit") {
			return
		}
	}
	t.Fatalf("expected npm audit in log, got: %v", readCommandLog(t, logPath))
}

// TestPkgExecUsedForLintInPnpmProject verifies lint uses pnpm exec in a pnpm project.
func TestPkgExecUsedForLintInPnpmProject(t *testing.T) {
	r, logPath := newTestRunner(t)
	r.pkgManager = "pnpm"

	if code := runLint(r); code != 0 {
		t.Fatalf("runLint returned %d", code)
	}

	entries := readCommandLog(t, logPath)
	for _, entry := range entries {
		// runLint now appends dirs, e.g. "pnpm exec ultracite check src".
		if strings.HasPrefix(entry, "pnpm exec ultracite check") {
			return
		}
	}
	t.Fatalf("expected 'pnpm exec ultracite check ...' in log, got: %v", entries)
}

// TestRunFTAUsesFtaBinaryInPnpmProject verifies pnpm projects call the
// executable exported by the fta-cli package, which is `fta`.
func TestRunFTAUsesFtaBinaryInPnpmProject(t *testing.T) {
	r, logPath := newTestRunner(t)
	r.pkgManager = "pnpm"

	if code := runFTA(r, []string{"src"}, 60); code != 0 {
		t.Fatalf("runFTA returned %d", code)
	}

	for _, entry := range readCommandLog(t, logPath) {
		if entry == "pnpm exec fta --score-cap 60 src" {
			return
		}
	}
	t.Fatalf("expected 'pnpm exec fta --score-cap 60 src' in log, got: %v", readCommandLog(t, logPath))
}

// TestRunFTAUsesFtaBinaryByDefault verifies npm/npx also uses the `fta`
// executable instead of the package name `fta-cli`.
func TestRunFTAUsesFtaBinaryByDefault(t *testing.T) {
	r, logPath := newTestRunner(t)

	if code := runFTA(r, []string{"src"}, 60); code != 0 {
		t.Fatalf("runFTA returned %d", code)
	}

	for _, entry := range readCommandLog(t, logPath) {
		if entry == "npx fta --score-cap 60 src" {
			return
		}
	}
	t.Fatalf("expected 'npx fta --score-cap 60 src' in log, got: %v", readCommandLog(t, logPath))
}

// TestNoNpxInPnpmCheckRun verifies no bare npx calls appear in a pnpm check run.
func TestNoNpxInPnpmCheckRun(t *testing.T) {
	r, logPath := newTestRunner(t)
	r.pkgManager = "pnpm"

	if code := runCheck(r, []string{"src"}, 60); code != 0 {
		t.Fatalf("runCheck returned %d", code)
	}

	for _, entry := range readCommandLog(t, logPath) {
		if strings.HasPrefix(entry, "npx ") {
			t.Fatalf("unexpected bare npx call in pnpm project: %q", entry)
		}
	}
}

func newTestRunner(t *testing.T) (*Runner, string) {
	t.Helper()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte(`{"devDependencies":{"vitest":"*"}}`), 0o644); err != nil {
		t.Fatalf("write package.json: %v", err)
	}

	binDir := filepath.Join(root, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("mkdir bin: %v", err)
	}

	logPath := filepath.Join(root, "commands.log")
	for _, name := range []string{"npx", "npm", "pnpm", "yarn"} {
		writeFakeCommand(t, filepath.Join(binDir, name))
	}

	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("TSGUARD_LOG", logPath)

	return &Runner{root: root, timeout: 5, dirs: []string{"src"}}, logPath
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
