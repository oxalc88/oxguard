package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// captureOutput runs fn with os.Stdout redirected and returns its output.
func captureOutput(t *testing.T, fn func() int) (int, string) {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "stdout")
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = f
	defer func() { os.Stdout = old }()
	code := fn()
	os.Stdout = old
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(f.Name())
	if err != nil {
		t.Fatal(err)
	}
	return code, string(data)
}

// writeShim drops a POSIX shell command named name into a fresh PATH entry.
func writeShim(t *testing.T, name, script string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake python/uv shims are POSIX shell scripts")
	}
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, name), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// Setup must announce the Python 3.12 requirement and stop when the
// interpreter is older than the supported floor.
func TestSetupPythonStepReportsSupportedRequirement(t *testing.T) {
	writeShim(t, "python3", "#!/bin/sh\nprintf 'Python 3.11.9\\n'\n")

	code, out := captureOutput(t, func() int {
		return runSetup(t.TempDir(), config{})
	})
	if code != 2 {
		t.Fatalf("runSetup returned %d, want 2; output:\n%s", code, out)
	}
	for _, want := range []string{
		"[1/8] Python 3.12...",
		"[FAIL] Python — found Python 3.11.9 but need >= 3.12 (not 3.14.1)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("setup output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "3.13") {
		t.Errorf("setup output still references 3.13:\n%s", out)
	}
}

// Setup continues past a supported Python into the uv steps when the
// interpreter satisfies >= 3.12 (excluding 3.14.1).
func TestSetupContinuesWithSupportedPython(t *testing.T) {
	writeShim(t, "python3", "#!/bin/sh\nprintf 'Python 3.12.0\\n'\n")
	writeShim(t, "uv", "#!/bin/sh\n"+
		"if [ \"$1\" = \"--version\" ]; then printf 'uv 0.12.19\\n'; exit 0; fi\n"+
		"exit 1\n")

	code, out := captureOutput(t, func() int {
		return runSetup(t.TempDir(), config{})
	})
	if code != 1 {
		t.Fatalf("runSetup returned %d, want 1; output:\n%s", code, out)
	}
	for _, want := range []string{
		"[1/8] Python 3.12...",
		"[OK]   Python 3.12.0",
		"[2/8] uv... found (uv 0.12.19)",
		"[4/8] uv sync...",
		"[FAIL] uv sync failed",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("setup output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "3.13") {
		t.Errorf("setup output still references 3.13:\n%s", out)
	}
}

// The doctor Python step must fail for a rejected version with the
// same requirement wording as setup.
func TestCheckPythonRejectsOldVersion(t *testing.T) {
	writeShim(t, "python3", "#!/bin/sh\nprintf 'Python 3.11.9\\n'\n")

	code, out := captureOutput(t, func() int {
		if checkPython() {
			return 0
		}
		return 1
	})
	if code != 1 {
		t.Fatalf("checkPython accepted unsupported version; output:\n%s", out)
	}
	if !strings.Contains(out, "need >= 3.12 (not 3.14.1)") {
		t.Errorf("doctor output missing requirement wording:\n%s", out)
	}
	if strings.Contains(out, "3.13") {
		t.Errorf("doctor output still references 3.13:\n%s", out)
	}
}
