package main

import (
	"fmt"
	"os"
	"strings"
)

// conventionalTestExclude is the --exclude glob passed to radon when excludeTests is true.
// Matches conventional Python test file naming (test_*.py and *_test.py). conftest.py is
// also excluded — it's test infrastructure that is intentionally verbose.
const conventionalTestExclude = "*/test_*.py,*/*_test.py,*/conftest.py"

// conventionalTestIgnore is the --ignore directory list passed to radon when excludeTests is true.
const conventionalTestIgnore = "tests,test"

// runRadon runs complexity checks and fails if any function has CC > 10 (grade C or
// worse), any file has MI below grade B, or any function exceeds Halstead thresholds.
// Conventional test files are excluded by default (r.excludeTests); add project-specific
// patterns via [tool.pyguard] exclude in pyproject.toml.
func runRadon(r *Runner, dirs []string) int {
	fmt.Println("  radon:")

	// Export test-exclusion env vars for the Python analysis scripts (check_halstead.py,
	// check_type_complexity.py), which use _paths.collect_paths as their single walk point.
	if r.excludeTests {
		os.Setenv("PYGUARD_EXCLUDE_TESTS", "1")
	} else {
		os.Setenv("PYGUARD_EXCLUDE_TESTS", "0")
	}
	os.Setenv("PYGUARD_EXCLUDE_GLOBS", strings.Join(r.exclude, ","))

	// Informational display: CC averages, MI, Halstead raw numbers
	args := append(radonExcludeArgs(r, "cc", "--total-average"), dirs...)
	r.Run("radon cc avg", args...)

	args = append(radonExcludeArgs(r, "mi"), dirs...)
	r.Run("radon mi", args...)

	args = append(radonExcludeArgs(r, "hal"), dirs...)
	r.Run("radon hal", args...)

	if code := runRadonEnforce(r, dirs, "cc", "C", "functions exceeding CC > 10", "all functions CC <= 10"); code != 0 {
		return code
	}
	if code := runRadonEnforce(r, dirs, "mi", "B", "files below maintainability grade B", "all files MI >= grade B"); code != 0 {
		return code
	}

	halArgs := append([]string{"uv", "run", "python", "tools/analysis/check_halstead.py"}, dirs...)
	if code := r.Run("halstead", halArgs...); !code.ok {
		return 1
	}
	return 0
}

// radonExcludeArgs builds the uv+radon base args with --exclude/--ignore when test exclusion is on.
func radonExcludeArgs(r *Runner, subCmd string, extra ...string) []string {
	args := []string{"uv", "run", "radon", subCmd}
	args = append(args, extra...)
	if r.excludeTests {
		args = append(args, "--exclude", conventionalTestExclude)
		args = append(args, "--ignore", conventionalTestIgnore)
	}
	if len(r.exclude) > 0 {
		args = append(args, "--exclude", strings.Join(r.exclude, ","))
	}
	return args
}

// runRadonEnforce runs `radon <subCmd> -n <grade>` and fails if stdout has any output.
// Uses RunSilent so uv's VIRTUAL_ENV stderr warning doesn't cause false-fails.
func runRadonEnforce(r *Runner, dirs []string, subCmd, grade, failMsg, okMsg string) int {
	args := append(radonExcludeArgs(r, subCmd, "-n", grade), dirs...)
	stdout, _, err := RunSilent(r.root, args...)
	trimmed := strings.TrimSpace(stdout)
	if err != nil || trimmed != "" {
		fmt.Printf("  [FAIL] radon — %s:\n", failMsg)
		for _, line := range strings.Split(trimmed, "\n") {
			if line != "" {
				fmt.Printf("         %s\n", line)
			}
		}
		return 1
	}
	fmt.Printf("  [OK]   radon — %s\n", okMsg)
	return 0
}
