package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
	"golang.org/x/term"
)

// requiredUvDevDeps is the manifest of uv dev-group packages pyguard invokes.
// semgrep has been removed: pyguard's baseline SAST is bandit (blocking, local).
// An optional Opengrep deep pass is available via `pyguard security --deep` using
// the same project-local self-contained binary as tsguard (no pip required).
var requiredUvDevDeps = []string{
	"ruff",
	"mypy",
	"pytest",
	"pytest-cov",
	"radon",
	"bandit",
	"pip-audit",
	"detect-secrets",
	"vulture",
	"deptry",
	"pyan3",
	"networkx",
}

// missingUvDevDeps reads pyproject.toml and returns deps from manifest that are
// absent from [dependency-groups].dev (PEP 735) or [tool.uv.dev-dependencies].
func missingUvDevDeps(root string, manifest []string) ([]string, error) {
	data, err := os.ReadFile(filepath.Join(root, "pyproject.toml"))
	if err != nil {
		return nil, nil // no pyproject yet — let uv sync fail naturally
	}

	var raw map[string]interface{}
	if err := toml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parsing pyproject.toml: %w", err)
	}

	declared := map[string]bool{}

	// PEP 735: [dependency-groups].dev (what `uv add --group dev` writes)
	if dg, ok := raw["dependency-groups"].(map[string]interface{}); ok {
		collectDepGroup(dg, "dev", declared)
	}

	// Legacy uv: [tool.uv.dev-dependencies]
	if tool, ok := raw["tool"].(map[string]interface{}); ok {
		if uvSection, ok := tool["uv"].(map[string]interface{}); ok {
			if devList, ok := uvSection["dev-dependencies"].([]interface{}); ok {
				for _, v := range devList {
					if s, ok := v.(string); ok {
						declared[normPkgName(s)] = true
					}
				}
			}
		}
	}

	var missing []string
	for _, dep := range manifest {
		if !declared[normPkgName(dep)] {
			missing = append(missing, dep)
		}
	}
	return missing, nil
}

// collectDepGroup populates declared with package names from a PEP 735 dependency group.
// Entries are either bare strings ("ruff>=0.4") or inline-table group references
// ({include-group = "extra-dev"}), which are recursively followed.
func collectDepGroup(groups map[string]interface{}, name string, declared map[string]bool) {
	list, ok := groups[name].([]interface{})
	if !ok {
		return
	}
	for _, v := range list {
		switch entry := v.(type) {
		case string:
			declared[normPkgName(entry)] = true
		case map[string]interface{}:
			if ref, ok := entry["include-group"].(string); ok {
				collectDepGroup(groups, ref, declared)
			}
		}
	}
}

// normPkgName extracts the bare package name from a PEP 508 specifier,
// lowercases it, and replaces hyphens/underscores for comparison.
// e.g. "ruff>=0.4" → "ruff", "pytest-cov" → "pytest-cov"
func normPkgName(s string) string {
	// strip version specifier (>=, ==, ~=, !=, etc.)
	for _, sep := range []string{">=", "==", "~=", "!=", ">", "<", ";"} {
		if idx := strings.Index(s, sep); idx != -1 {
			s = s[:idx]
		}
	}
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, "_", "-")
	return s
}

// ensureUvDevDeps checks for missing dev-group packages, prompts if any are absent,
// and runs uv add --group dev for the gap. Skips silently if nothing is missing.
func ensureUvDevDeps(root string, cfg config) error {
	missing, err := missingUvDevDeps(root, requiredUvDevDeps)
	if err != nil {
		return err
	}
	if len(missing) == 0 {
		fmt.Println("  [OK]   all required dev-group packages present")
		return nil
	}

	fmt.Println("  Missing dev-group packages:")
	for _, dep := range missing {
		fmt.Printf("    - %s\n", dep)
	}

	if !confirmYesNo("  Add them to pyproject.toml now?", true, cfg.assumeYes) {
		fmt.Println("  [SKIP] dev-group unchanged — run pyguard setup again to add later")
		return nil
	}

	args := append([]string{"add", "--group", "dev"}, missing...)
	if err := RunStreaming(root, append([]string{"uv"}, args...)...); err != nil {
		return fmt.Errorf("uv add --group dev: %w", err)
	}
	fmt.Println("  [OK]   dev-group packages added")
	return nil
}

// confirmYesNo prints a y/n prompt and returns the user's choice.
// Returns defaultYes immediately when assumeYes, CI=true, or stdin is not a TTY.
func confirmYesNo(prompt string, defaultYes bool, assumeYes bool) bool {
	if assumeYes || isCI() || !term.IsTerminal(int(os.Stdin.Fd())) {
		hint := "y"
		if !defaultYes {
			hint = "n"
		}
		fmt.Printf("%s [%s] (auto)\n", prompt, hint)
		return defaultYes
	}

	suffix := "[Y/n]"
	if !defaultYes {
		suffix = "[y/N]"
	}
	fmt.Printf("%s %s ", prompt, suffix)

	scanner := bufio.NewScanner(os.Stdin)
	scanner.Scan()
	input := strings.ToLower(strings.TrimSpace(scanner.Text()))

	switch input {
	case "y", "yes":
		return true
	case "n", "no":
		return false
	default:
		return defaultYes
	}
}

// isCI returns true when running in a known CI environment.
func isCI() bool {
	return os.Getenv("CI") == "true" || os.Getenv("CI") == "1"
}

// detectPythonTestRunner inspects pyproject.toml deps and config files to identify
// the project's test runner. Returns "pytest", "unittest", or "".
// unittest returns "unittest" only when no pytest markers are found but test files exist —
// pyguard will still run pytest (which discovers unittest tests natively).
func detectPythonTestRunner(root string) string {
	data, err := os.ReadFile(filepath.Join(root, "pyproject.toml"))
	if err == nil {
		var raw map[string]interface{}
		if toml.Unmarshal(data, &raw) == nil {
			declared := map[string]bool{}
			if dg, ok := raw["dependency-groups"].(map[string]interface{}); ok {
				collectDepGroup(dg, "dev", declared)
			}
			if tool, ok := raw["tool"].(map[string]interface{}); ok {
				if uvSec, ok := tool["uv"].(map[string]interface{}); ok {
					if devList, ok := uvSec["dev-dependencies"].([]interface{}); ok {
						for _, v := range devList {
							if s, ok := v.(string); ok {
								declared[normPkgName(s)] = true
							}
						}
					}
				}
			}
			if declared["pytest"] {
				return "pytest"
			}
		}
	}

	// Config file fallback.
	for _, f := range []string{"pytest.ini", "conftest.py"} {
		if _, err := os.Stat(filepath.Join(root, f)); err == nil {
			return "pytest"
		}
	}
	if data, err := os.ReadFile(filepath.Join(root, "setup.cfg")); err == nil {
		if strings.Contains(string(data), "[tool:pytest]") {
			return "pytest"
		}
	}

	// No pytest markers found — check for test files (unittest convention).
	for _, dir := range []string{"tests", "test"} {
		if entries, err := os.ReadDir(filepath.Join(root, dir)); err == nil && len(entries) > 0 {
			return "unittest"
		}
	}

	return ""
}

// pyguardFileConfig holds values read from [tool.pyguard] in pyproject.toml.
type pyguardFileConfig struct {
	ExcludeTests *bool    `toml:"exclude-tests"` // nil = use default (true)
	Exclude      []string `toml:"exclude"`        // extra globs for radon/complexity gates
}

// loadPyguardConfig reads [tool.pyguard] from pyproject.toml.
// Missing or unparseable sections return zero values without error.
func loadPyguardConfig(root string) pyguardFileConfig {
	data, err := os.ReadFile(filepath.Join(root, "pyproject.toml"))
	if err != nil {
		return pyguardFileConfig{}
	}
	var raw struct {
		Tool struct {
			Pyguard pyguardFileConfig `toml:"pyguard"`
		} `toml:"tool"`
	}
	if err := toml.Unmarshal(data, &raw); err != nil {
		return pyguardFileConfig{}
	}
	return raw.Tool.Pyguard
}
