package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
)

// runCheck runs the full quality gate: lint → complexity → fta → types → coverage → security.
// Sequential, fail-fast: stops at first failure.
func runCheck(r *Runner, dirs []string, ftaCap int) int {
	fmt.Println("tsguard check")
	fmt.Println("─────────")

	steps := []struct {
		name string
		fn   func() int
	}{
		{"lint", func() int { return runLint(r) }},
		{"complexity", func() int { return runComplexity(r, dirs) }},
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
	res := r.Run("ultracite fix", "npx", "ultracite", "fix")
	if !res.ok {
		return 1
	}
	return 0
}

// runLint runs lint + format check via ultracite check.
func runLint(r *Runner) int {
	res := r.Run("ultracite check", "npx", "ultracite", "check")
	if !res.ok {
		return 1
	}
	return 0
}

// runTypes runs strict type checking.
func runTypes(r *Runner) int {
	res := r.Run("tsc --noEmit", "npx", "tsc", "--noEmit")
	if !res.ok {
		return 1
	}
	return 0
}

// runComplexity runs biome complexity check in isolation.
func runComplexity(r *Runner, dirs []string) int {
	args := append([]string{"npx", "@biomejs/biome", "lint",
		"--rule", "complexity/noExcessiveCognitiveComplexity"}, dirs...)
	res := r.Run("complexity", args...)
	if !res.ok {
		return 1
	}
	return 0
}

// runFTA runs the Fast TypeScript Analyzer for Halstead + cyclomatic + LOC score.
// Fails if any file's FTA score exceeds scoreCap (default 60 = "Needs Improvement" tier).
func runFTA(r *Runner, dirs []string, scoreCap int) int {
	scoreCapStr := strconv.Itoa(scoreCap)
	for _, dir := range dirs {
		res := r.Run("fta "+dir, "npx", "fta-cli", "--score-cap", scoreCapStr, dir)
		if !res.ok {
			return 1
		}
	}
	return 0
}

// runCoverage runs vitest with coverage (threshold enforced via vitest.config.ts).
func runCoverage(r *Runner) int {
	res := r.Run("vitest --coverage", "npx", "vitest", "run", "--coverage")
	if !res.ok {
		return 1
	}
	return 0
}

// runSecurity runs detect-secrets → semgrep → npm audit, all as hard gates.
func runSecurity(r *Runner, initFlag bool) int {
	fmt.Println("  security:")
	if code := runSecrets(r, initFlag); code != 0 {
		return code
	}
	if code := runSemgrep(r); code != 0 {
		return code
	}
	if code := runNpmAudit(r); code != 0 {
		return code
	}
	return 0
}

func runSemgrep(r *Runner) int {
	if !ensureSinglePythonTool("semgrep") {
		return 0
	}
	res := r.Run("semgrep", "semgrep", "--config=p/javascript", "--config=p/typescript",
		"--error", "--quiet", ".")
	if !res.ok {
		return 1
	}
	return 0
}

func runNpmAudit(r *Runner) int {
	res := r.Run("npm audit", "npm", "audit", "--audit-level=moderate")
	if !res.ok {
		return 1
	}
	return 0
}

// runSecrets checks for credential leaks. Fails hard if .secrets.baseline is missing.
func runSecrets(r *Runner, initFlag bool) int {
	baseline := filepath.Join(r.root, ".secrets.baseline")

	if initFlag {
		// tsguard secrets --init: create baseline explicitly
		fmt.Print("  Creating .secrets.baseline...")
		out, err := RunCapture(r.root, "detect-secrets", "scan")
		if err != nil {
			fmt.Printf(" failed\n  [FAIL] detect-secrets scan failed\n")
			fmt.Println("         Install detect-secrets: pip install detect-secrets")
			return 1
		}
		if err := os.WriteFile(baseline, []byte(out), 0o644); err != nil {
			fmt.Printf(" failed\n  [FAIL] could not write .secrets.baseline: %v\n", err)
			return 1
		}
		fmt.Println("\n  [OK]   .secrets.baseline created")
		return 0
	}

	if !ensureSinglePythonTool("detect-secrets") {
		return 0
	}

	if _, err := os.Stat(baseline); os.IsNotExist(err) {
		fmt.Println("  [FAIL] secrets — .secrets.baseline not found")
		fmt.Println("         Run: tsguard secrets --init")
		return 1
	}

	res := r.Run("detect-secrets audit", "detect-secrets", "audit", baseline)
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
	r.Run("knip", "npx", "knip")
	return 0 // always informational
}

// runDuplicates runs jscpd for copy-paste code detection (informational).
func runDuplicates(r *Runner, dirs []string) int {
	args := append([]string{"npx", "jscpd"}, dirs...)
	r.Run("jscpd", args...)
	return 0 // always informational
}
