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
	r.println("tsguard check")
	r.println("─────────")

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
			r.printf("\n  FAILED: %s\n", step.name)
			return code
		}
	}

	r.println("\n  All checks passed.")
	return 0
}

// runFix runs auto-formatter: ultracite fix (biome format + lint --fix).
func runFix(r *Runner) int {
	if packagedRuntime() != "" {
		return r.runPackagedBiome(true)
	}
	r.println("tsguard fix")
	res := r.RunTool(toolSpec{gate: "lint"}, "ultracite fix", pkgExec(r.pkgManager, "ultracite", "fix")...)
	if !res.ok {
		return 1
	}
	return 0
}

// runLint runs lint + format check via ultracite check.
// Scopes to the project's source dirs and respects exclude dirs (fixes vendored-dir noise).
func runLint(r *Runner) int {
	if packagedRuntime() != "" {
		return r.runPackagedBiome(false)
	}
	args := pkgExec(r.pkgManager, "ultracite", "check")
	// Pass source dirs so ultracite doesn't lint the entire repo root.
	if len(r.dirs) > 0 {
		args = append(args, r.dirs...)
	}
	res := r.RunTool(toolSpec{gate: "lint"}, "ultracite check", args...)
	if !res.ok {
		return 1
	}
	return 0
}

// runTypes runs strict type checking.
func runTypes(r *Runner) int {
	if packagedRuntime() != "" {
		return r.runPackagedTypes()
	}
	args := pkgExec(r.pkgManager, "tsc", "--noEmit")
	if r.machine() {
		args = append(args, "--pretty", "false")
	}
	res := r.RunTool(toolSpec{gate: "types", adapter: "tsc"}, "tsc --noEmit", args...)
	if !res.ok {
		return 1
	}
	return 0
}

// Compatibility alias: complexity rules are enforced by the lint gate.
func runComplexity(r *Runner, _ []string) int {
	r.println("  complexity: delegated to ultracite check")
	return runLint(r)
}

// runFTA runs the Fast TypeScript Analyzer for Halstead + cyclomatic + LOC score.
// Fails if any file's FTA score exceeds scoreCap (default 60 = "Needs Improvement" tier).
// Conventional test files are excluded by default (see r.ftaExcludeTests); add project-
// specific patterns via oxguard.toml fta-exclude.
func runFTA(r *Runner, dirs []string, scoreCap int) int {
	scoreCapStr := strconv.Itoa(scoreCap)

	configPath, err := writeFTAConfig(r.root, r.ftaExcludeTests, r.ftaExclude)
	if err != nil {
		r.errorf("tsguard: warning — could not write fta config (%v); running without exclusions\n", err)
		if r.machine() {
			return r.executionFailure("fta", "invalid_configuration", "Could not write FTA exclusion config.")
		}
	}

	for _, dir := range dirs {
		args := pkgExec(r.pkgManager, "fta", "--score-cap", scoreCapStr)
		if r.machine() {
			args = append(args, "--json")
		}
		if configPath != "" {
			args = append(args, "--config-path", configPath)
		}
		args = append(args, dir)
		if !r.RunTool(toolSpec{gate: "fta", adapter: "fta", subject: dir, threshold: float64(scoreCap)}, "fta "+dir, args...).ok {
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
	if packagedRuntime() != "" && runner == "" {
		runner = "vitest"
	}
	switch runner {
	case "vitest":
		if err := checkVitestVersionMatch(r.root); err != nil {
			r.printf("  [FAIL] coverage — %s\n", err)
			return r.executionFailure("coverage", "invalid_configuration", err.Error())
		}
		args := pkgExec(r.pkgManager, "vitest", "run", "--coverage",
			"--coverage.thresholds.lines=80",
			"--coverage.thresholds.functions=80",
			"--coverage.thresholds.branches=80",
			"--coverage.thresholds.statements=80",
		)
		if packagedRuntime() != "" && !hasProjectConfig(r.root, "vitest.config.ts", "vitest.config.js", "vitest.config.mts", "vitest.config.mjs", "vite.config.ts", "vite.config.js", "vite.config.mjs", "vite.config.mts") {
			args = append(args, "--config", filepath.Join(packagedRuntime(), "config", "vitest.config.mjs"))
		}
		res := r.RunTool(toolSpec{gate: "coverage"}, "vitest --coverage", args...)
		if !res.ok {
			return 1
		}
	case "jest":
		const threshold = `--coverageThreshold={"global":{"lines":80,"functions":80,"branches":80,"statements":80}}`
		args := pkgExec(r.pkgManager, "jest", "--coverage", threshold)
		res := r.RunTool(toolSpec{gate: "coverage"}, "jest --coverage", args...)
		if !res.ok {
			return 1
		}
	case "":
		r.println("  [FAIL] coverage — no test runner found in package.json or config files")
		r.println("         Add vitest or jest to devDependencies")
		return r.executionFailure("coverage", "tool_missing", "No test runner found; add vitest or jest.")
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
		res := r.RunTool(toolSpec{gate: "coverage"}, fmt.Sprintf("c8 %s --coverage", runner), args...)
		if !res.ok {
			return 1
		}
	case "nyc":
		args := pkgExec(r.pkgManager, "nyc",
			"--check-coverage",
			"--lines", "80", "--functions", "80", "--branches", "80", "--statements", "80",
			runner,
		)
		res := r.RunTool(toolSpec{gate: "coverage"}, fmt.Sprintf("nyc %s --coverage", runner), args...)
		if !res.ok {
			return 1
		}
	default:
		r.printf("  [FAIL] coverage — %s detected but no coverage wrapper found\n", runner)
		r.println("         Add c8 or nyc to devDependencies: npm install --save-dev c8")
		return r.executionFailure("coverage", "tool_missing", "No coverage wrapper found; add c8 or nyc.")
	}
	return 0
}

// runSecurity runs secretlint → npm audit + audit-ci → opengrep SAST.
// All are hard gates (blocking); opengrep [SKIP]s gracefully if binary is absent.
func runSecurity(r *Runner, initFlag bool) int {
	r.println("  security:")
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
	if packagedRuntime() != "" {
		var err error
		args, err = r.packagedSecretArgs()
		if err != nil {
			r.errorf("tsguard: default secrets config: %v\n", err)
			return r.executionFailure("secrets", "invalid_configuration", err.Error())
		}
	}
	res := r.RunTool(toolSpec{gate: "secrets"}, "secretlint", args...)
	if !res.ok {
		return 1
	}
	return 0
}

// runSecretsInit is the entry point for tsguard secrets --init.
// secretlint requires no persistent baseline — it scans on every run.
// This command runs a scan and reports findings for developer review.
func runSecretsInit(r *Runner) int {
	if packagedRuntime() != "" {
		return runSecretlint(r)
	}
	r.println("  Scanning for secrets (secretlint)...")
	args := pkgExec(r.pkgManager, "secretlint", "--secretlintignore", ".gitignore")
	args = append(args, r.dirs...)
	r.RunTool(toolSpec{gate: "secrets", advisory: true}, "secretlint scan", args...)
	r.println("  [OK]   secretlint scan complete (no baseline needed)")
	return 0
}

// runNpmAudit runs dependency vulnerability scanning. audit-ci is the hard gate:
// it supports allowlists (.auditcirc.json) and custom thresholds. PM-native audit
// runs first as informational output (richer output format) but does not gate.
func runNpmAudit(r *Runner) int {
	// PM-native audit — informational; richer output, does not gate.
	var infoArgs []string
	switch r.pkgManager {
	case "pnpm":
		infoArgs = []string{"pnpm", "audit", "--audit-level", "moderate"}
	case "yarn":
		infoArgs = []string{"yarn", "npm", "audit", "--severity", "moderate"}
	default:
		infoArgs = []string{"npm", "audit", "--audit-level=moderate"}
	}
	r.RunTool(toolSpec{gate: "dependencies", advisory: true}, "dependency audit (info)", infoArgs...)

	// audit-ci — hard gate: threshold enforcement + allowlist via .auditcirc.json.
	res := r.RunTool(toolSpec{gate: "dependencies"}, "audit-ci", pkgExec(r.pkgManager, "audit-ci", "--moderate")...)
	if !res.ok {
		return 1
	}
	return 0
}

// runOpengrep runs the Opengrep SAST engine against the project's scanned dirs.
// Uses the project-local binary (node_modules/.cache/oxguard/opengrep).
// [SKIP]s gracefully if the binary is absent — developer is directed to run setup.
func runOpengrep(r *Runner) int {
	binaryPath := opengrepBinaryPath(r.root)
	if _, err := os.Stat(binaryPath); os.IsNotExist(err) {
		if packagedRuntime() != "" {
			r.errorf("%s\n", "tsguard: bundled Opengrep is missing; reinstall @oxguard/tsguard with optional dependencies enabled")
			return r.executionFailure("security", "tool_missing", "Bundled Opengrep is missing; reinstall with optional dependencies.")
		}
		r.println("  [SKIP] opengrep — binary not found. Run: tsguard setup")
		if r.machine() {
			r.result.execution("security", "tool_missing", "Opengrep is missing; standalone mode skipped SAST.")
		}
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
	if r.machine() {
		args = append(args, "--json")
	}
	for _, dir := range r.excludeDirs {
		args = append(args, "--exclude", dir)
	}
	// Scan the project's source dirs.
	args = append(args, r.dirs...)

	res := r.RunTool(toolSpec{gate: "security", adapter: "opengrep"}, "opengrep SAST", args...)
	if !res.ok {
		return 1
	}
	return 0
}

// runAudit runs informational analysis: criticality + dead-code + duplicates.
// Never fails (exit 0 always) — these are advisory.
func runAudit(r *Runner, dirs []string) int {
	r.println("tsguard audit (informational)")
	runCriticality(r)
	runDeadCode(r)
	runDuplicates(r, dirs)
	return 0
}

// runDeadCode runs knip for dead code and unused dependency detection (informational).
func runDeadCode(r *Runner) int {
	r.RunTool(toolSpec{gate: "dead-code", advisory: true}, "knip", pkgExec(r.pkgManager, "knip")...)
	return 0 // always informational
}

// runDuplicates runs jscpd for copy-paste code detection (informational).
func runDuplicates(r *Runner, dirs []string) int {
	args := append(pkgExec(r.pkgManager, "jscpd"), dirs...)
	r.RunTool(toolSpec{gate: "duplicates", advisory: true}, "jscpd", args...)
	return 0 // always informational
}
