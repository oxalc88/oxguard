package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func TestRadonBlockingFailureRetainsNestingContext(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX uv shim")
	}
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 unavailable")
	}
	bin := t.TempDir()
	shim := "#!/bin/sh\nshift\nif [ \"$1\" = radon ]; then\n printf '%s\\n' '{\"a.py\":[{\"type\":\"function\",\"name\":\"deep\",\"lineno\":1,\"complexity\":11,\"rank\":\"C\"}]}'\nelse\n shift\n exec " + python + " \"$@\"\nfi\n"
	if err := os.WriteFile(filepath.Join(bin, "uv"), []byte(shim), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	r := &Runner{root: t.TempDir(), timeout: 30, outputMode: "json", result: newRunResult("radon")}
	if err := os.WriteFile(filepath.Join(r.root, "a.py"), []byte("def deep(x):\n    if x:\n        if x > 1:\n            return x\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if code := runRadonMachine(r, []string{"a.py"}); code != 1 {
		t.Fatalf("exit = %d", code)
	}
	if len(r.result.Findings) != 1 || r.result.Findings[0].Rule != "pyguard.radon.cc_exceeded" || r.result.Findings[0].Status != StatusBlocking {
		t.Fatalf("findings changed: %+v", r.result.Findings)
	}
	if len(r.result.Measurements) != 2 || r.result.Measurements[1].Metric != "function.max_branch_nesting" || r.result.Measurements[1].Value != 2 {
		t.Fatalf("measurements: %+v", r.result.Measurements)
	}
	if len(r.result.Artifacts) != 1 || r.result.Artifacts[0].Kind != "maintainability_graph" {
		t.Fatalf("artifacts: %+v", r.result.Artifacts)
	}
	seen := map[string]bool{}
	for _, d := range r.result.Diagnostics {
		if seen[d.ID] {
			t.Fatalf("duplicate diagnostic %s", d.ID)
		}
		seen[d.ID] = true
	}
}

func TestComplexityContextFailureDoesNotChangeDecision(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX uv shim")
	}
	for _, output := range []string{"not JSON", `{"schema_version":"1","findings":[],"measurements":[],"artifacts":[],"error":{"category":"analyzer_failure","message":"failed"}}`} {
		t.Run(output, func(t *testing.T) {
			bin := t.TempDir()
			if err := os.WriteFile(filepath.Join(bin, "uv"), []byte("#!/bin/sh\ncat <<'JSON'\n"+output+"\nJSON\n"), 0755); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
			r := &Runner{root: t.TempDir(), timeout: 30, outputMode: "json", result: newRunResult("radon")}
			r.result.AddFinding(Finding{Gate: "radon", Rule: "pyguard.radon.cc_exceeded", Status: StatusBlocking})
			addComplexityContext(r, []string{"."})
			if len(r.result.Findings) != 1 || len(r.result.Measurements) != 0 || len(r.result.Artifacts) != 0 {
				t.Fatalf("optional failure changed result: %+v", r.result)
			}
		})
	}
}
