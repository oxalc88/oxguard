package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// detectPythonTestRunner must recognize both generations of pytest
// configuration: the pre-9 markers (dev-group deps, pytest.ini, conftest.py,
// setup.cfg) and the pytest 9 forms (pytest.toml/.pytest.toml and the native
// [tool.pytest] table next to the legacy [tool.pytest.ini_options] table).
func TestDetectPythonTestRunner(t *testing.T) {
	cases := []struct {
		name, file, content, want string
		extraDir                  string
	}{
		{name: "empty project", want: ""},
		{name: "dev group declares pytest", file: "pyproject.toml", content: "[dependency-groups]\ndev = [\"pytest==9.1.1\"]\n", want: "pytest"},
		{name: "legacy uv dev dependencies declare pytest", file: "pyproject.toml", content: "[tool.uv]\ndev-dependencies = [\"pytest\"]\n", want: "pytest"},
		{name: "native tool.pytest table", file: "pyproject.toml", content: "[tool.pytest]\naddopts = [\"-q\"]\n", want: "pytest"},
		{name: "legacy tool.pytest.ini_options table", file: "pyproject.toml", content: "[tool.pytest.ini_options]\naddopts = \"-q\"\n", want: "pytest"},
		{name: "tool table without pytest", file: "pyproject.toml", content: "[tool.ruff]\nline-length = 88\n", want: ""},
		{name: "pytest.ini", file: "pytest.ini", content: "[pytest]\n", want: "pytest"},
		{name: "pytest.toml", file: "pytest.toml", content: "[pytest]\n", want: "pytest"},
		{name: ".pytest.toml", file: ".pytest.toml", content: "[pytest]\n", want: "pytest"},
		{name: "conftest.py", file: "conftest.py", content: "", want: "pytest"},
		{name: "setup.cfg tool section", file: "setup.cfg", content: "[tool:pytest]\naddopts = -q\n", want: "pytest"},
		{name: "test files without markers", extraDir: "tests", want: "unittest"},
		{name: "pytest 9 config with unconventional test files", file: "pyproject.toml", content: "[tool.pytest]\n", extraDir: "spec", want: "pytest"},
		{name: "pytest.toml with unconventional test files", file: "pytest.toml", content: "[pytest]\n", extraDir: "spec", want: "pytest"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			root := t.TempDir()
			if c.file != "" {
				if err := os.WriteFile(filepath.Join(root, c.file), []byte(c.content), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if c.extraDir != "" {
				if err := os.MkdirAll(filepath.Join(root, c.extraDir), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(root, c.extraDir, "test_main.py"), []byte("def test_x():\n    pass\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if got := detectPythonTestRunner(root); got != c.want {
				t.Fatalf("detectPythonTestRunner() = %q, want %q", got, c.want)
			}
		})
	}
}

// A different exact complexipy pin is still "present" for the missing check,
// but setup/doctor report it as a warning instead of silently accepting it.
// Unpinned tools such as radon keep the existing package-only behavior.
func TestUvDevDepMismatchesReportsDifferentExactPins(t *testing.T) {
	cases := []struct {
		name, dev, want string
	}{
		{name: "matching pin", dev: `["complexipy==8.3.0"]`},
		{name: "different pin", dev: `["complexipy==8.2.0"]`, want: `complexipy — pyproject.toml declares "complexipy==8.2.0", pyguard expects "complexipy==8.3.0"`},
		{name: "unpinned dependency", dev: `["complexipy"]`, want: `complexipy — pyproject.toml declares "complexipy", pyguard expects "complexipy==8.3.0"`},
		{name: "version range", dev: `["complexipy>=8.0"]`, want: `complexipy — pyproject.toml declares "complexipy>=8.0", pyguard expects "complexipy==8.3.0"`},
		{name: "unpinned radon ignored", dev: `["radon>=6"]`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			root := t.TempDir()
			if err := os.WriteFile(filepath.Join(root, "pyproject.toml"), []byte("[dependency-groups]\ndev = "+c.dev+"\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			warnings, err := uvDevDepMismatches(root, requiredUvDevDeps)
			if err != nil {
				t.Fatal(err)
			}
			if c.want == "" {
				if len(warnings) != 0 {
					t.Fatalf("unexpected warnings: %v", warnings)
				}
				return
			}
			if len(warnings) != 1 || warnings[0] != c.want {
				t.Fatalf("warnings = %v, want [%q]", warnings, c.want)
			}
			missing, err := missingUvDevDeps(root, requiredUvDevDeps)
			if err != nil {
				t.Fatal(err)
			}
			for _, dep := range missing {
				if strings.HasPrefix(dep, "complexipy") {
					t.Fatalf("mismatched pin must stay present for the missing check: %v", missing)
				}
			}
		})
	}
}
