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

// TestSecretsScanUsesBaseline verifies the scan command passes the baseline
// path back to detect-secrets so baseline filters and exclusions are applied.
func TestSecretsScanUsesBaseline(t *testing.T) {
	r, logPath := newTestRunner(t)
	baselinePath := filepath.Join(r.root, ".secrets.baseline")

	if err := os.WriteFile(baselinePath, []byte(`{"results":{}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	if code := runSecrets(r, false); code != 0 {
		t.Fatalf("runSecrets returned %d, want 0", code)
	}

	want := "detect-secrets scan --baseline " + baselinePath
	for _, entry := range readCommandLog(t, logPath) {
		if entry == want {
			return
		}
	}
	t.Fatalf("expected %q in log, got: %v", want, readCommandLog(t, logPath))
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
		if entry == "pnpm exec ultracite check" {
			return
		}
	}
	t.Fatalf("expected 'pnpm exec ultracite check' in log, got: %v", entries)
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
	if err := os.WriteFile(filepath.Join(root, ".secrets.baseline"), []byte(`{"results":{}}`), 0o644); err != nil {
		t.Fatalf("write baseline: %v", err)
	}

	binDir := filepath.Join(root, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatalf("mkdir bin: %v", err)
	}

	logPath := filepath.Join(root, "commands.log")
	for _, name := range []string{"npx", "npm", "pnpm", "yarn", "semgrep"} {
		writeFakeCommand(t, filepath.Join(binDir, name))
	}
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
