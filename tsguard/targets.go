package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
)

// runCheck runs the full quality gate: lint (includes Ultracite/Biome complexity) → fta → types → coverage → security.
// Sequential, fail-fast: stops at first failure.
func runCheck(r *Runner, dirs []string, ftaCap int) int {
	fmt.Println("tsguard check")
	fmt.Println("─────────")

	// Remove stale coverage artifacts from previous runs before lint scans the tree.
	// vitest --coverage writes coverage/ at the end of the run; without this cleanup
	// the next lint gate flags generated files it did not produce.
	os.RemoveAll(filepath.Join(r.root, "coverage"))

	steps := []struct {
		name string
		fn   func() int
	}{
		{"lint", func() int { return runLint(r) }},
		{"fta", func() int { return runFTA(r, dirs, ftaCap) }},
		{"types", func() int { return runTypes(r) }},
		{"coverage", func() int { return runCoverage(r) }},
		{"security", func() int { return runSecurity(r, false) }},
	}

	for _, step := range steps {
		if code := step.fn(); code != 0 {
			fmt.Printf("\n  FAILED: %s\n", step.name)
			return code
		}
	}

	fmt.Println("\n  All checks passed.")
	return 0
}

// runFix runs auto-formatter: ultracite fix (biome format + lint --fix).
func runFix(r *Runner) int {
	fmt.Println("tsguard fix")
	res := r.Run("ultracite fix", pkgExec(r.pkgManager, "ultracite", "fix")...)
	if !res.ok {
		return 1
	}
	return 0
}

// runLint runs lint + format check via ultracite check.
// Scopes to the project's source dirs and respects exclude dirs (fixes vendored-dir noise).
func runLint(r *Runner) int {
	args := pkgExec(r.pkgManager, "ultracite", "check")
	// Pass source dirs so ultracite doesn't lint the entire repo root.
	if len(r.dirs) > 0 {
		args = append(args, r.dirs...)
	}
	res := r.Run("ultracite check", args...)
	if !res.ok {
		return 1
	}
	return 0
}

// runTypes runs strict type checking.
func runTypes(r *Runner) int {
	res := r.Run("tsc --noEmit", pkgExec(r.pkgManager, "tsc", "--noEmit")...)
	if !res.ok {
		return 1
	}
	return 0
}

// Compatibility alias: complexity rules are enforced by the lint gate.
func runComplexity(r *Runner, _ []string) int {
	fmt.Println("  complexity: delegated to ultracite check")
	return runLint(r)
}

// runFTA runs the Fast TypeScript Analyzer for Halstead + cyclomatic + LOC score.
// Fails if any file's FTA score exceeds scoreCap (default 60 = "Needs Improvement" tier).
func runFTA(r *Runner, dirs []string, scoreCap int) int {
	scoreCapStr := strconv.Itoa(scoreCap)
	for _, dir := range dirs {
		res := r.Run("fta "+dir, pkgExec(r.pkgManager, "fta", "--score-cap", scoreCapStr, dir)...)
		if !res.ok {
			return 1
		}
	}
	return 0
}

// runCoverage detects the project's test runner and enforces an 80% coverage floor.
// vitest and jest have native threshold flags. For other runners (mocha, ava, jasmine)
// c8 or nyc must be present to wrap the runner — fails hard if neither is found.
func runCoverage(r *Runner) int {
	runner := detectTestRunner(r.root)
	switch runner {
	case "vitest":
		if err := checkVitestVersionMatch(r.root); err != nil {
			fmt.Printf("  [FAIL] coverage — %s\n", err)
			return 1
		}
		args := pkgExec(r.pkgManager, "vitest", "run", "--coverage",
			"--coverage.thresholds.lines=80",
			"--coverage.thresholds.functions=80",
			"--coverage.thresholds.branches=80",
			"--coverage.thresholds.statements=80",
		)
		res := r.Run("vitest --coverage", args...)
		if !res.ok {
			return 1
		}
	case "jest":
		const threshold = `--coverageThreshold={"global":{"lines":80,"functions":80,"branches":80,"statements":80}}`
		args := pkgExec(r.pkgManager, "jest", "--coverage", threshold)
		res := r.Run("jest --coverage", args...)
		if !res.ok {
			return 1
		}
	case "":
		fmt.Println("  [FAIL] coverage — no test runner found in package.json or config files")
		fmt.Println("         Add vitest or jest to devDependencies")
		return 1
	default:
		return runCoverageWithWrapper(r, runner)
	}
	return 0
}

// runCoverageWithWrapper wraps runners that lack native coverage threshold support
// (mocha, ava, jasmine, etc.) with c8 or nyc. Fails if neither wrapper is present.
func runCoverageWithWrapper(r *Runner, runner string) int {
	wrapper := detectCoverageWrapper(r.root)
	switch wrapper {
	case "c8":
		args := pkgExec(r.pkgManager, "c8",
			"--lines", "80", "--functions", "80", "--branches", "80", "--statements", "80",
			runner,
		)
		res := r.Run(fmt.Sprintf("c8 %s --coverage", runner), args...)
		if !res.ok {
			return 1
		}
	case "nyc":
		args := pkgExec(r.pkgManager, "nyc",
			"--check-coverage",
			"--lines", "80", "--functions", "80", "--branches", "80", "--statements", "80",
			runner,
		)
		res := r.Run(fmt.Sprintf("nyc %s --coverage", runner), args...)
		if !res.ok {
			return 1
		}
	default:
		fmt.Printf("  [FAIL] coverage — %s detected but no coverage wrapper found\n", runner)
		fmt.Println("         Add c8 or nyc to devDependencies: npm install --save-dev c8")
		return 1
	}
	return 0
}

// runSecurity runs secretlint → npm audit + audit-ci → opengrep SAST.
// All are hard gates (blocking); opengrep [SKIP]s gracefully if binary is absent.
func runSecurity(r *Runner, initFlag bool) int {
	fmt.Println("  security:")
	if initFlag {
		return runSecretsInit(r)
	}
	if code := runSecretlint(r); code != 0 {
		return code
	}
	if code := runNpmAudit(r); code != 0 {
		return code
	}
	if code := runOpengrep(r); code != 0 {
		return code
	}
	return 0
}

// runSecretlint scans for hardcoded credentials using secretlint (npm-native, no Python).
// Scopes to r.dirs so it only scans project source; .gitignore handles exclusions.
// NOTE: Go's exec.Command does not invoke a shell — globs like **/* would be passed
// as literals. Pass source dirs as positional arguments instead.
func runSecretlint(r *Runner) int {
	args := pkgExec(r.pkgManager, "secretlint", "--secretlintignore", ".gitignore")
	args = append(args, r.dirs...)
	res := r.Run("secretlint", args...)
	if !res.ok {
		return 1
	}
	return 0
}

// runSecretsInit is the entry point for tsguard secrets --init.
// secretlint requires no persistent baseline — it scans on every run.
// This command runs a scan and reports findings for developer review.
func runSecretsInit(r *Runner) int {
	fmt.Println("  Scanning for secrets (secretlint)...")
	args := pkgExec(r.pkgManager, "secretlint", "--secretlintignore", ".gitignore")
	args = append(args, r.dirs...)
	r.Run("secretlint scan", args...)
	fmt.Println("  [OK]   secretlint scan complete (no baseline needed)")
	return 0
}

// runNpmAudit runs the PM-native dependency vulnerability scan (hard gate for moderate+).
// Also runs audit-ci for threshold enforcement and allowlist support.
func runNpmAudit(r *Runner) int {
	// PM-native audit.
	var auditArgs []string
	switch r.pkgManager {
	case "pnpm":
		auditArgs = []string{"pnpm", "audit", "--audit-level", "moderate"}
	case "yarn":
		auditArgs = []string{"yarn", "npm", "audit", "--severity", "moderate"}
	default:
		auditArgs = []string{"npm", "audit", "--audit-level=moderate"}
	}
	res := r.Run("dependency audit", auditArgs...)
	if !res.ok {
		return 1
	}

	// audit-ci: threshold enforcement with allowlist support (.auditcirc.json if present).
	auditCIArgs := pkgExec(r.pkgManager, "audit-ci", "--moderate")
	r.Run("audit-ci", auditCIArgs...) // advisory — don't fail the gate (audit already gated above)
	return 0
}

// runOpengrep runs the Opengrep SAST engine against the project's scanned dirs.
// Uses the project-local binary (node_modules/.cache/oxguard/opengrep).
// [SKIP]s gracefully if the binary is absent — developer is directed to run setup.
func runOpengrep(r *Runner) int {
	binaryPath := opengrepBinaryPath(r.root)
	if _, err := os.Stat(binaryPath); os.IsNotExist(err) {
		fmt.Println("  [SKIP] opengrep — binary not found. Run: tsguard setup")
		return 0
	}

	// Vendor rules from the project store; fall back to p/javascript + p/typescript.
	rulesDir := filepath.Join(r.root, "node_modules/.cache/oxguard/rules")
	var configArgs []string
	if _, err := os.Stat(rulesDir); err == nil {
		configArgs = []string{"--config", rulesDir}
	} else {
		configArgs = []string{"--config", "p/javascript", "--config", "p/typescript"}
	}

	args := []string{binaryPath, "scan"}
	args = append(args, configArgs...)
	args = append(args, "--error", "--quiet")
	for _, dir := range r.excludeDirs {
		args = append(args, "--exclude", dir)
	}
	// Scan the project's source dirs.
	args = append(args, r.dirs...)

	res := r.Run("opengrep SAST", args...)
	if !res.ok {
		return 1
	}
	return 0
}

// runAudit runs informational analysis: dead-code + duplicates.
// Never fails (exit 0 always) — these are advisory.
func runAudit(r *Runner, dirs []string) int {
	fmt.Println("tsguard audit (informational)")
	runDeadCode(r)
	runDuplicates(r, dirs)
	return 0
}

// runDeadCode runs knip for dead code and unused dependency detection (informational).
func runDeadCode(r *Runner) int {
	r.Run("knip", pkgExec(r.pkgManager, "knip")...)
	return 0 // always informational
}

// runDuplicates runs jscpd for copy-paste code detection (informational).
func runDuplicates(r *Runner, dirs []string) int {
	args := append(pkgExec(r.pkgManager, "jscpd"), dirs...)
	r.Run("jscpd", args...)
	return 0 // always informational
}
