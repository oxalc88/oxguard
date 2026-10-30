package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPyprojectHasRuffConfig(t *testing.T) {
	cases := []struct {
		name string
		data string
		want bool
	}{
		{name: "empty document", data: "", want: false},
		{name: "no tool table", data: "[project]\nname = \"demo\"\n", want: false},
		{name: "other tool table", data: "[tool.mypy]\nstrict = true\n", want: false},
		{name: "tool ruff table", data: "[tool.ruff]\nline-length = 88\n", want: true},
		{name: "tool ruff lint subtable", data: "[tool.ruff.lint]\nselect = [\"F\"]\n", want: true},
		{name: "tool ruff nested subtable", data: "[tool.ruff.lint.per-file-ignores]\n\"tests/*\" = [\"S110\"]\n", want: true},
		{name: "inline tool ruff table", data: "tool = { ruff = { line-length = 88 } }\n", want: true},
		{name: "tool is not a table", data: "tool = \"ruff\"\n", want: false},
		// Malformed input is conservative: preserve the project's existing Ruff
		// invocation rather than injecting CLI selects over it.
		{name: "malformed toml", data: "[tool.ruff\nline-length = 88\n", want: true},
		{name: "malformed value", data: "[tool.ruff]\nline-length = \n", want: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := pyprojectHasRuffConfig([]byte(c.data)); got != c.want {
				t.Fatalf("pyprojectHasRuffConfig(%q) = %v, want %v", c.data, got, c.want)
			}
		})
	}
}

func TestHasRuffConfigProjectFiles(t *testing.T) {
	cases := []struct {
		name  string
		files map[string]string
		want  bool
	}{
		{name: "no configuration", files: map[string]string{"pyproject.toml": "[project]\nname = \"demo\"\n"}, want: false},
		{name: "pyproject tool ruff", files: map[string]string{"pyproject.toml": "[tool.ruff]\nline-length = 88\n"}, want: true},
		{name: "pyproject tool ruff lint", files: map[string]string{"pyproject.toml": "[tool.ruff.lint]\nselect = [\"F\"]\n"}, want: true},
		{name: "ruff.toml", files: map[string]string{"pyproject.toml": "[project]\nname = \"demo\"\n", "ruff.toml": "line-length = 88\n"}, want: true},
		{name: ".ruff.toml", files: map[string]string{"pyproject.toml": "[project]\nname = \"demo\"\n", ".ruff.toml": "line-length = 88\n"}, want: true},
		{name: "malformed pyproject", files: map[string]string{"pyproject.toml": "[tool.ruff\n"}, want: true},
		// Ruff searches nested directories for its own config, but pyguard's
		// zero-config decision is about the project root only.
		{name: "nested ruff.toml does not count", files: map[string]string{"pyproject.toml": "[project]\nname = \"demo\"\n", "pkg/ruff.toml": "line-length = 88\n"}, want: false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			root := t.TempDir()
			for name, content := range c.files {
				path := filepath.Join(root, filepath.FromSlash(name))
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if got := hasRuffConfig(root); got != c.want {
				t.Fatalf("hasRuffConfig() = %v, want %v", got, c.want)
			}
		})
	}
}

// An unreadable pyproject.toml (here a directory at that path) is an unknown
// policy, so the baseline must stay off rather than override a project config.
func TestHasRuffConfigUnreadablePyprojectIsConservative(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "pyproject.toml"), 0o755); err != nil {
		t.Fatal(err)
	}
	if !hasRuffConfig(root) {
		t.Fatal("unreadable pyproject.toml must be treated as an existing Ruff config")
	}
}

func TestRuffBaselineArgs(t *testing.T) {
	t.Run("zero config selects baseline and records version", func(t *testing.T) {
		root := t.TempDir()
		if err := os.WriteFile(filepath.Join(root, "pyproject.toml"), []byte("[project]\nname = \"demo\"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		result := newRunResult("ruff")
		runner := &Runner{root: root, result: result}
		args := runner.ruffBaselineArgs()
		want := []string{"--extend-select", "S110,B012"}
		if len(args) != len(want) || args[0] != want[0] || args[1] != want[1] {
			t.Fatalf("ruffBaselineArgs() = %v, want %v", args, want)
		}
		if len(result.Measurements) != 1 {
			t.Fatalf("measurements = %v, want one baseline version", result.Measurements)
		}
		m := result.Measurements[0]
		if m.Metric != "ruff.baseline_version" || m.Level != "code" || m.Location.File != "." || m.Value != ruffBaselineVersion || m.Unit != "version" {
			t.Fatalf("unexpected baseline measurement: %+v", m)
		}
	})

	t.Run("project config is untouched", func(t *testing.T) {
		root := t.TempDir()
		if err := os.WriteFile(filepath.Join(root, "pyproject.toml"), []byte("[tool.ruff.lint]\nignore = [\"S110\"]\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		result := newRunResult("ruff")
		runner := &Runner{root: root, result: result}
		if args := runner.ruffBaselineArgs(); args != nil {
			t.Fatalf("ruffBaselineArgs() = %v, want nil for a configured project", args)
		}
		if len(result.Measurements) != 0 {
			t.Fatalf("measurements = %v, want none for a configured project", result.Measurements)
		}
	})
}
