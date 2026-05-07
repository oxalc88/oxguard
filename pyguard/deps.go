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
	"semgrep",
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
		if devList, ok := dg["dev"].([]interface{}); ok {
			for _, v := range devList {
				if s, ok := v.(string); ok {
					declared[normPkgName(s)] = true
				}
			}
		}
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
	case "", "y", "yes":
		return defaultYes || input == "y" || input == "yes"
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
