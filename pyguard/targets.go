package main

import (
	"os"
	"path/filepath"
)

// runCheck runs the full quality gate: ruff → mypy → radon → coverage → security.
// Sequential, fail-fast: stops at first failure.
func runCheck(r *Runner, dirs []string) int {
	r.println("pyguard check")
	r.println("─────────")

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
		r.printf("  starting %s...\n", step.name)
		if code := step.fn(); code != 0 {
			r.printf("\n  FAILED: %s\n", step.name)
			return code
		}
	}

	r.println("\n  All checks passed.")
	return 0
}

// runFix runs auto-formatters: ruff check --fix then ruff format.
func runFix(r *Runner, dirs []string) int {
	r.println("pyguard fix")
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
	r.println("pyguard audit (informational)")
	runCriticality(r)
	runDeadCode(r, dirs)
	runDeps(r)
	return 0
}

// runSecurity runs bandit → pip-audit → secrets, sequentially.
func runSecurity(r *Runner, dirs []string) int {
	r.println("  security:")
	r.println("  starting bandit...")
	if code := runBandit(r, dirs); code != 0 {
		return code
	}
	r.println("  starting pip-audit...")
	if code := runPipAudit(r); code != 0 {
		return code
	}
	r.println("  starting detect-secrets...")
	return runSecrets(r, config{})
}

// runRuff runs lint + format check.
func runRuff(r *Runner, dirs []string) int {
	args := []string{"uv", "run", "ruff", "check"}
	if r.machine() {
		args = append(args, "--output-format", "json")
	}
	args = append(args, dirs...)
	res1 := r.RunTool(toolSpec{gate: "ruff", adapter: "ruff"}, "ruff lint", args...)
	if !res1.ok {
		return 1
	}
	res2 := r.RunTool(toolSpec{gate: "ruff", adapter: "ruff_format"}, "ruff format --check", append([]string{"uv", "run", "ruff", "format", "--check"}, dirs...)...)
	if !res2.ok {
		return 1
	}
	return 0
}

// runMypy runs strict type checking.
func runMypy(r *Runner, dirs []string) int {
	args := []string{"uv", "run", "mypy"}
	if r.machine() {
		args = append(args, "--output", "json")
	}
	args = append(args, dirs...)
	res := r.RunTool(toolSpec{gate: "mypy", adapter: "mypy"}, "mypy", args...)
	if !res.ok {
		return 1
	}
	return 0
}

// runTypes checks type-annotation complexity (depth > 2 or length > 40 chars).
func runTypes(r *Runner, dirs []string) int {
	r.exportExcludeEnv()
	args := append([]string{"uv", "run", "python", "tools/analysis/check_type_complexity.py"}, dirs...)
	if r.machine() {
		args = append(args, "--json")
	}
	res := r.RunTool(toolSpec{gate: "types", adapter: "owned"}, "types", args...)
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
	case "pytest", "unittest":
		args := []string{"uv", "run", "pytest", "--cov", "--cov-fail-under=80"}
		spec := toolSpec{gate: "coverage"}
		if r.machine() {
			report, err := r.reportFile("coverage", "coverage.json")
			if err != nil {
				return r.executionFailure("coverage", "diagnostics_failure", err.Error())
			}
			tests, err := r.reportFile("coverage", "tests.xml")
			if err != nil {
				return r.executionFailure("coverage", "diagnostics_failure", err.Error())
			}
			spec.adapter, spec.reportPath, spec.testsPath = "pytest", report, tests
			args = append(args, "--cov-report=json:"+report, "--junitxml="+tests, "-o", "junit_family=xunit1")
		}
		if !r.RunTool(spec, "pytest --cov", args...).ok {
			return 1
		}
	case "":
		r.executionFailure("coverage", "tool_missing", "No test runner or test files found.")
		r.println("         Add pytest to dev dependencies: uv add --group dev pytest pytest-cov")
		return 1
	}
	return 0
}

// runBandit runs the security scanner.
func runBandit(r *Runner, dirs []string) int {
	args := []string{"uv", "run", "bandit", "-r", "-c", "pyproject.toml", "-q"}
	if r.machine() {
		args = append(args, "--format", "json")
	}
	res := r.RunTool(toolSpec{gate: "bandit", adapter: "bandit"}, "bandit", append(args, dirs...)...)
	if !res.ok {
		return 1
	}
	return 0
}

// runPipAudit runs dependency vulnerability scanning.
func runPipAudit(r *Runner) int {
	args := []string{"uv", "run", "pip-audit"}
	if r.machine() {
		args = append(args, "--format", "json")
	}
	res := r.RunTool(toolSpec{gate: "pip-audit", adapter: "pip-audit"}, "pip-audit", args...)
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
		r.printf("  Creating .secrets.baseline...")
		out, err := RunCapture(r.root, "uv", "run", "detect-secrets", "scan")
		if err != nil {
			r.printf(" failed\n  [FAIL] detect-secrets scan failed\n")
			return 1
		}
		if err := os.WriteFile(baseline, []byte(out), 0o644); err != nil {
			r.printf(" failed\n  [FAIL] could not write .secrets.baseline: %v\n", err)
			return 1
		}
		r.println("\n  [OK]   .secrets.baseline created")
		return 0
	}

	if _, err := os.Stat(baseline); os.IsNotExist(err) {
		r.executionFailure("secrets", "invalid_configuration", ".secrets.baseline not found.")
		r.println("         Run: pyguard secrets --init")
		return 1
	}

	args := []string{"uv", "run", "python", "tools/analysis/check_secrets.py"}
	if r.machine() {
		args = append(args, "--json")
	}
	res := r.RunTool(toolSpec{gate: "secrets", adapter: "owned"}, "detect-secrets", args...)
	if !res.ok {
		return 1
	}
	return 0
}

// runCriticality generates the call-graph criticality report (informational).
func runCriticality(r *Runner) int {
	r.exportExcludeEnv()
	args := []string{"uv", "run", "python", "tools/analysis/analyze_criticality.py"}
	if r.machine() {
		args = append(args, "--json")
	}
	dirs := r.dirs
	if r.dirsExplicit {
		args = append(args, dirs...)
	}
	r.RunTool(toolSpec{gate: "criticality", adapter: "owned", advisory: true}, "criticality", args...)
	return 0 // always informational
}

// runDeadCode runs vulture for dead code detection (informational).
func runDeadCode(r *Runner, dirs []string) int {
	args := append([]string{"uv", "run", "vulture"}, dirs...)
	spec := toolSpec{gate: "dead-code", advisory: true}
	if r.machine() {
		args = append([]string{"uv", "run", "python", "tools/analysis/vulture_report.py"}, dirs...)
		spec.adapter = "owned"
	}
	r.RunTool(spec, "vulture", args...)
	return 0
}

// runDeps runs deptry for dependency hygiene (informational).
func runDeps(r *Runner) int {
	args := []string{"uv", "run", "deptry", ".."}
	spec := toolSpec{gate: "deps", advisory: true}
	if r.machine() {
		report, err := r.reportFile("deptry", "deptry.json")
		if err != nil {
			r.executionFailure("deps", "diagnostics_failure", err.Error())
			return 0
		}
		args = append(args, "--json-output", report)
		spec.adapter, spec.reportPath = "deptry", report
	}
	r.RunTool(spec, "deptry", args...)
	return 0
}
