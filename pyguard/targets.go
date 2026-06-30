package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// runCheck runs the full quality gate: ruff → mypy → radon → coverage → security.
// Sequential, fail-fast: stops at first failure.
func runCheck(r *Runner, dirs []string) int {
	fmt.Println("pyguard check")
	fmt.Println("─────────")

	steps := []struct {
		name string
		fn   func() int
	}{
		{"ruff", func() int { return runRuff(r, dirs) }},
		{"mypy", func() int { return runMypy(r, dirs) }},
		{"radon", func() int { return runRadon(r, dirs) }},
		{"types", func() int { return runTypes(r, dirs) }},
		{"coverage", func() int { return runCoverage(r) }},
		{"security", func() int { return runSecurity(r, dirs) }},
	}

	for _, step := range steps {
		fmt.Printf("  starting %s...\n", step.name)
		if code := step.fn(); code != 0 {
			fmt.Printf("\n  FAILED: %s\n", step.name)
			return code
		}
	}

	fmt.Println("\n  All checks passed.")
	return 0
}

// runFix runs auto-formatters: ruff check --fix then ruff format.
func runFix(r *Runner, dirs []string) int {
	fmt.Println("pyguard fix")
	args1 := append([]string{"uv", "run", "ruff", "check", "--fix"}, dirs...)
	res1 := r.Run("ruff check --fix", args1...)

	args2 := append([]string{"uv", "run", "ruff", "format"}, dirs...)
	res2 := r.Run("ruff format", args2...)

	if !res1.ok || !res2.ok {
		return 1
	}
	return 0
}

// runAudit runs informational analysis: criticality + dead-code + deps.
// Never fails (exit 0 always) — these are advisory.
func runAudit(r *Runner, dirs []string) int {
	fmt.Println("pyguard audit (informational)")
	runCriticality(r)
	runDeadCode(r, dirs)
	runDeps(r)
	return 0
}

// runSecurity runs bandit → pip-audit → secrets, sequentially.
func runSecurity(r *Runner, dirs []string) int {
	fmt.Println("  security:")
	fmt.Println("  starting bandit...")
	if code := runBandit(r, dirs); code != 0 {
		return code
	}
	fmt.Println("  starting pip-audit...")
	if code := runPipAudit(r); code != 0 {
		return code
	}
	fmt.Println("  starting detect-secrets...")
	return runSecrets(r, config{})
}

// runRuff runs lint + format check.
func runRuff(r *Runner, dirs []string) int {
	res1 := r.Run("ruff lint", append([]string{"uv", "run", "ruff", "check"}, dirs...)...)
	if !res1.ok {
		return 1
	}
	res2 := r.Run("ruff format --check", append([]string{"uv", "run", "ruff", "format", "--check"}, dirs...)...)
	if !res2.ok {
		return 1
	}
	return 0
}

// runMypy runs strict type checking.
func runMypy(r *Runner, dirs []string) int {
	res := r.Run("mypy", append([]string{"uv", "run", "mypy"}, dirs...)...)
	if !res.ok {
		return 1
	}
	return 0
}

// runTypes checks type-annotation complexity (depth > 2 or length > 40 chars).
// Exports PYGUARD_EXCLUDE_TESTS/PYGUARD_EXCLUDE_GLOBS so _paths.collect_paths
// applies the same test-file filtering as the radon gate.
func runTypes(r *Runner, dirs []string) int {
	if r.excludeTests {
		os.Setenv("PYGUARD_EXCLUDE_TESTS", "1")
	} else {
		os.Setenv("PYGUARD_EXCLUDE_TESTS", "0")
	}
	os.Setenv("PYGUARD_EXCLUDE_GLOBS", strings.Join(r.exclude, ","))
	args := append([]string{"uv", "run", "python", "tools/analysis/check_type_complexity.py"}, dirs...)
	res := r.Run("types", args...)
	if !res.ok {
		return 1
	}
	return 0
}

// runCoverage detects the project's test runner from pyproject.toml and config files,
// then runs coverage with an 80% floor. pytest is used directly; for unittest projects
// pytest is used as the runner since it discovers unittest tests natively.
func runCoverage(r *Runner) int {
	runner := detectPythonTestRunner(r.root)
	switch runner {
	case "pytest":
		res := r.Run("pytest --cov", "uv", "run", "pytest", "--cov", "--cov-fail-under=80")
		if !res.ok {
			return 1
		}
	case "unittest":
		// pytest discovers and runs unittest tests without requiring rewrites.
		res := r.Run("pytest --cov (unittest)", "uv", "run", "pytest", "--cov", "--cov-fail-under=80")
		if !res.ok {
			return 1
		}
	case "":
		fmt.Println("  [FAIL] coverage — no test runner or test files found")
		fmt.Println("         Add pytest to dev dependencies: uv add --group dev pytest pytest-cov")
		return 1
	}
	return 0
}

// runBandit runs the security scanner.
func runBandit(r *Runner, dirs []string) int {
	res := r.Run("bandit", append([]string{"uv", "run", "bandit", "-r", "-c", "pyproject.toml", "-q"}, dirs...)...)
	if !res.ok {
		return 1
	}
	return 0
}

// runPipAudit runs dependency vulnerability scanning.
func runPipAudit(r *Runner) int {
	res := r.Run("pip-audit", "uv", "run", "pip-audit")
	if !res.ok {
		return 1
	}
	return 0
}

// runSecrets checks for credential leaks. Fails hard if .secrets.baseline is missing.
func runSecrets(r *Runner, cfg config) int {
	baseline := filepath.Join(r.root, ".secrets.baseline")

	if cfg.initFlag {
		// pyguard secrets --init: create baseline explicitly
		fmt.Print("  Creating .secrets.baseline...")
		out, err := RunCapture(r.root, "uv", "run", "detect-secrets", "scan")
		if err != nil {
			fmt.Printf(" failed\n  [FAIL] detect-secrets scan failed\n")
			return 1
		}
		if err := os.WriteFile(baseline, []byte(out), 0o644); err != nil {
			fmt.Printf(" failed\n  [FAIL] could not write .secrets.baseline: %v\n", err)
			return 1
		}
		fmt.Println("\n  [OK]   .secrets.baseline created")
		return 0
	}

	if _, err := os.Stat(baseline); os.IsNotExist(err) {
		fmt.Println("  [FAIL] secrets — .secrets.baseline not found")
		fmt.Println("         Run: pyguard secrets --init")
		return 1
	}

	res := r.Run("detect-secrets", "uv", "run", "python", "tools/analysis/check_secrets.py")
	if !res.ok {
		return 1
	}
	return 0
}

// runCriticality generates the call-graph criticality report (informational).
func runCriticality(r *Runner) int {
	r.Run("criticality", "uv", "run", "python", "tools/analysis/analyze_criticality.py")
	return 0 // always informational
}

// runDeadCode runs vulture for dead code detection (informational).
func runDeadCode(r *Runner, dirs []string) int {
	r.Run("vulture", append([]string{"uv", "run", "vulture"}, dirs...)...)
	return 0
}

// runDeps runs deptry for dependency hygiene (informational).
func runDeps(r *Runner) int {
	r.Run("deptry", "uv", "run", "deptry", "..")
	return 0
}
