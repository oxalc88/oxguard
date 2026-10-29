package contract

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestBothLanguagesUseSameShapeAndBounds(t *testing.T) {
	for _, language := range []string{"tsguard", "pyguard"} {
		t.Run(language, func(t *testing.T) {
			a, b := New(language, "check"), New(language, "check")
			for i := 0; i < 1000; i++ {
				f := Finding{Gate: "types", Rule: "type.error", Severity: "error", Status: StatusBlocking, Category: "quality", Location: &Location{File: strings.Repeat("file", 200), Line: i + 1}, Evidence: strings.Repeat("é\n\x1b", 1000)}
				a.AddFinding(f)
				f.Evidence = "Changed analyzer prose"
				b.AddFinding(f)
			}
			a.Finish(1)
			b.Finish(1)
			for i := range a.Findings {
				if a.Findings[i].ID != b.Findings[i].ID {
					t.Fatal("unstable ID")
				}
			}
			var agent, jsonOut bytes.Buffer
			if err := Report(&agent, "agent", a); err != nil {
				t.Fatal(err)
			}
			if agent.Len() > 6144 || strings.Count(agent.String(), "\n") > 26 || !strings.Contains(agent.String(), "omitted: 990 (use --output json)") || strings.Contains(agent.String(), "\x1b") {
				t.Fatal("unbounded/unsafe summary")
			}
			Report(&jsonOut, "json", a)
			var decoded RunResult
			if err := json.Unmarshal(jsonOut.Bytes(), &decoded); err != nil || len(decoded.Findings) != 1000 {
				t.Fatal("incomplete JSON", err)
			}
			clean := New(language, "check")
			clean.Execution("invocation", "tool_missing", "missing")
			clean.Finish(0)
			if clean.Status != StatusError || clean.Findings[0].Rule != language+".execution.tool_missing" {
				t.Fatal(clean)
			}
		})
	}
}

func TestStripWindowsExtendedPrefix(t *testing.T) {
	cases := []struct{ name, path, want string }{
		{"slash drive", "//?/C:/Users/runneradmin/AppData/Local/Temp/project/src/complexity.py", "C:/Users/runneradmin/AppData/Local/Temp/project/src/complexity.py"},
		{"backslash drive", `\\?\C:\Users\runneradmin\AppData\Local\Temp\project\src\complexity.py`, `C:\Users\runneradmin\AppData\Local\Temp\project\src\complexity.py`},
		{"slash UNC", "//?/UNC/server/share/project/src/complexity.py", "//server/share/project/src/complexity.py"},
		{"backslash UNC", `\\?\UNC\server\share\project\src\complexity.py`, `\\server\share\project\src\complexity.py`},
		{"plain absolute", "/tmp/project/src/complexity.py", "/tmp/project/src/complexity.py"},
		{"already relative", "src/complexity.py", "src/complexity.py"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := stripWindowsExtendedPrefix(tc.path); got != tc.want {
				t.Fatalf("stripWindowsExtendedPrefix(%q) = %q, want %q", tc.path, got, tc.want)
			}
		})
	}
}

func TestRelativePathNormalizesAbsoluteAndExtendedForms(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "src", "complexity.py")
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte("def f():\n    pass\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cases := []struct{ name, path, want string }{
		{"already relative", filepath.Join("src", "complexity.py"), "src/complexity.py"},
		{"absolute", file, "src/complexity.py"},
	}
	if runtime.GOOS == "windows" {
		cases = append(cases,
			struct{ name, path, want string }{"extended backslash", `\\?\` + file, "src/complexity.py"},
			struct{ name, path, want string }{"extended slash", "//?/" + filepath.ToSlash(file), "src/complexity.py"},
		)
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := RelativePath(root, tc.path); got != tc.want {
				t.Fatalf("RelativePath(%q, %q) = %q, want %q", root, tc.path, got, tc.want)
			}
		})
	}
}

func TestRelativePathDoesNotEscapeProjectRoot(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.py")
	if err := os.WriteFile(outside, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	got := RelativePath(root, outside)
	if strings.HasPrefix(got, "..") {
		t.Fatalf("RelativePath escaped the project root: %q", got)
	}
	if !filepath.IsAbs(filepath.FromSlash(got)) {
		t.Fatalf("outside path did not fall back to an absolute path: %q", got)
	}
}
