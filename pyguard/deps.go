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
// Python security also runs pip-audit and detect-secrets; --deep is not implemented.
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
	"complexipy==8.3.0",
	"grimp==3.18",
}

// declaredUvDevDeps reads pyproject.toml and returns the declared PEP 508
// specifier for each package in [dependency-groups].dev (PEP 735) or the legacy
// [tool.uv.dev-dependencies], keyed by normalized package name.
func declaredUvDevDeps(root string) (map[string]string, error) {
	data, err := os.ReadFile(filepath.Join(root, "pyproject.toml"))
	if err != nil {
		return nil, nil // no pyproject yet — let uv sync fail naturally
	}

	var raw map[string]interface{}
	if err := toml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parsing pyproject.toml: %w", err)
	}

	declared := map[string]string{}

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
						declared[normPkgName(s)] = s
					}
				}
			}
		}
	}
	return declared, nil
}

// missingUvDevDeps returns deps from manifest absent from the project's dev
// groups. Version specifiers are deliberately not compared here so a range can
// satisfy the package requirement; exact-pin mismatches are reported separately.
func missingUvDevDeps(root string, manifest []string) ([]string, error) {
	declared, err := declaredUvDevDeps(root)
	if err != nil {
		return nil, err
	}
	if declared == nil {
		return nil, nil // no pyproject yet — let uv sync fail naturally
	}

	var missing []string
	for _, dep := range manifest {
		if _, ok := declared[normPkgName(dep)]; !ok {
			missing = append(missing, dep)
		}
	}
	return missing, nil
}

// uvDevDepMismatches reports manifest entries that require an exact version
// while the project declares a different one (or no exact pin). Tools without a
// manifest pin, such as radon, are left to doctor's installed-version report.
func uvDevDepMismatches(root string, manifest []string) ([]string, error) {
	declared, err := declaredUvDevDeps(root)
	if err != nil {
		return nil, err
	}
	var mismatches []string
	for _, dep := range manifest {
		expected, pinned := exactUvDevVersion(dep)
		if !pinned {
			continue
		}
		name := normPkgName(dep)
		declaredSpec, ok := declared[name]
		if !ok {
			continue // missingUvDevDeps already reports absent packages.
		}
		if actual, actualPinned := exactUvDevVersion(declaredSpec); actualPinned && actual == expected {
			continue
		}
		mismatches = append(mismatches, fmt.Sprintf("%s — pyproject.toml declares %q, pyguard expects %q", name, strings.TrimSpace(declaredSpec), strings.TrimSpace(dep)))
	}
	return mismatches, nil
}

// exactUvDevVersion returns the version of an exact == pin. Ranges, markers,
// extras and other constraints are intentionally not treated as exact pins.
func exactUvDevVersion(spec string) (string, bool) {
	idx := strings.Index(spec, "==")
	if idx == -1 {
		return "", false
	}
	version := strings.TrimSpace(spec[idx+2:])
	if version == "" || strings.ContainsAny(version, ",; <>=!~\t") {
		return "", false
	}
	return version, true
}

// collectDepGroup populates declared with package specifiers from a PEP 735 dependency group.
// Entries are either bare strings ("ruff>=0.4") or inline-table group references
// ({include-group = "extra-dev"}), which are recursively followed.
func collectDepGroup(groups map[string]interface{}, name string, declared map[string]string) {
	list, ok := groups[name].([]interface{})
	if !ok {
		return
	}
	for _, v := range list {
		switch entry := v.(type) {
		case string:
			declared[normPkgName(entry)] = entry
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
	mismatches, err := uvDevDepMismatches(root, requiredUvDevDeps)
	if err != nil {
		return err
	}
	for _, mismatch := range mismatches {
		fmt.Printf("  [WARN] %s\n", mismatch)
	}

	missing, err := missingUvDevDeps(root, requiredUvDevDeps)
	if err != nil {
		return err
	}
	if len(missing) == 0 {
		if len(mismatches) == 0 {
			fmt.Println("  [OK]   all required dev-group packages present")
		} else {
			fmt.Println("  [WARN] pinned dev-group versions differ; align them with the versions above")
		}
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
			declared := map[string]string{}
			if dg, ok := raw["dependency-groups"].(map[string]interface{}); ok {
				collectDepGroup(dg, "dev", declared)
			}
			tool, hasTool := raw["tool"].(map[string]interface{})
			if hasTool {
				if uvSec, ok := tool["uv"].(map[string]interface{}); ok {
					if devList, ok := uvSec["dev-dependencies"].([]interface{}); ok {
						for _, v := range devList {
							if s, ok := v.(string); ok {
								declared[normPkgName(s)] = s
							}
						}
					}
				}
			}
			if _, ok := declared["pytest"]; ok {
				return "pytest"
			}
			// pytest 9 reads native TOML configuration from [tool.pytest] in
			// pyproject.toml; the legacy [tool.pytest.ini_options] table stays
			// supported. Either table marks a pytest project.
			if hasTool {
				if _, ok := tool["pytest"]; ok {
					return "pytest"
				}
			}
		}
	}

	// Config file fallback. pytest 9 discovers pytest.toml/.pytest.toml before
	// pytest.ini, so the dedicated config files of both generations count.
	for _, f := range []string{"pytest.ini", "pytest.toml", ".pytest.toml", "conftest.py"} {
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
	Exclude      []string `toml:"exclude"`       // extra globs for radon/complexity gates
}

// loadPyguardConfig reads [tool.pyguard] from pyproject.toml.
// Invalid files/configuration return an explicit invocation error.
func loadPyguardConfig(root string) (pyguardFileConfig, error) {
	data, err := os.ReadFile(filepath.Join(root, "pyproject.toml"))
	if err != nil {
		return pyguardFileConfig{}, err
	}
	var raw struct {
		Tool struct {
			Pyguard pyguardFileConfig `toml:"pyguard"`
		} `toml:"tool"`
	}
	if err := toml.Unmarshal(data, &raw); err != nil {
		return pyguardFileConfig{}, err
	}
	return raw.Tool.Pyguard, nil
}
