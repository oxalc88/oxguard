package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
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
func runLint(r *Runner) int {
	res := r.Run("ultracite check", pkgExec(r.pkgManager, "ultracite", "check")...)
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

// runCoverage runs vitest with coverage, enforcing an 80% floor on all metrics.
// Projects with stricter thresholds in vitest.config.ts will still fail at their own threshold.
func runCoverage(r *Runner) int {
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
	args := []string{"semgrep", "--config=p/javascript", "--config=p/typescript", "--error", "--quiet"}
	for _, dir := range r.excludeDirs {
		args = append(args, "--exclude", dir)
	}
	args = append(args, ".")
	res := r.Run("semgrep", args...)
	if !res.ok {
		return 1
	}
	return 0
}

func runNpmAudit(r *Runner) int {
	var args []string
	switch r.pkgManager {
	case "pnpm":
		args = []string{"pnpm", "audit", "--audit-level", "moderate"}
	case "yarn":
		args = []string{"yarn", "npm", "audit", "--severity", "moderate"}
	default:
		args = []string{"npm", "audit", "--audit-level=moderate"}
	}
	res := r.Run("dependency audit", args...)
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
		out, err := RunCapture(r.root, append([]string{"detect-secrets", "scan"}, detectSecretsExcludeArgs(r.excludeDirs)...)...)
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

	// Read and parse the baseline before running the slow scan so we fail fast on missing/corrupt files.
	baselineData, err := os.ReadFile(baseline)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Println("  [FAIL] secrets — .secrets.baseline not found")
			fmt.Println("         Run: tsguard secrets --init")
		} else {
			fmt.Printf("  [FAIL] secrets — could not read .secrets.baseline: %v\n", err)
		}
		return 1
	}
	var known secretsReport
	if err := json.Unmarshal(baselineData, &known); err != nil {
		fmt.Printf("  [FAIL] secrets — could not parse .secrets.baseline: %v\n", err)
		return 1
	}

	tempBaselinePath, err := seedTempBaseline(baselineData)
	if err != nil {
		fmt.Printf("  [FAIL] detect-secrets scan: could not prepare temp baseline: %v\n", err)
		return 1
	}
	defer os.Remove(tempBaselinePath)

	scanArgs := append([]string{"detect-secrets", "scan", "--baseline", tempBaselinePath}, detectSecretsExcludeArgs(r.excludeDirs)...)
	_, _, scanErr := RunSilent(r.root, scanArgs...)
	if scanErr != nil {
		fmt.Println("  [FAIL] detect-secrets scan failed")
		return 1
	}

	currentData, err := os.ReadFile(tempBaselinePath)
	if err != nil {
		fmt.Printf("  [FAIL] detect-secrets scan: could not read updated baseline: %v\n", err)
		return 1
	}
	var current secretsReport
	if err := json.Unmarshal(currentData, &current); err != nil {
		fmt.Printf("  [FAIL] detect-secrets scan: could not parse updated baseline: %v\n", err)
		return 1
	}
	var newFound []string
	for file, entries := range current.Results {
		if len(entries) == 0 {
			continue
		}
		knownHashes := make(map[string]bool)
		for _, e := range known.Results[file] {
			knownHashes[e.HashedSecret] = true
		}
		for _, e := range entries {
			if !knownHashes[e.HashedSecret] {
				newFound = append(newFound, fmt.Sprintf("  %s:%d [%s]", file, e.LineNumber, e.Type))
			}
		}
	}
	if len(newFound) > 0 {
		fmt.Printf("  [FAIL] detect-secrets — %d new potential secret(s) found:\n", len(newFound))
		for _, s := range newFound {
			fmt.Println(s)
		}
		fmt.Println("         To update baseline: tsguard secrets --init")
		return 1
	}
	fmt.Println("  [OK]   detect-secrets")
	return 0
}

type secretsReport struct {
	Results map[string][]secretEntry `json:"results"`
}

type secretEntry struct {
	HashedSecret string `json:"hashed_secret"`
	Type         string `json:"type"`
	LineNumber   int    `json:"line_number"`
}

// detectSecretsExcludeArgs returns --exclude-files <regex> args for detect-secrets, or nil if dirs is empty.
func detectSecretsExcludeArgs(dirs []string) []string {
	if len(dirs) == 0 {
		return nil
	}
	return []string{"--exclude-files", strings.Join(dirs, "|")}
}

// seedTempBaseline writes data to a fresh temp file and returns its path.
// The caller is responsible for removing the file (e.g. via defer os.Remove).
func seedTempBaseline(data []byte) (string, error) {
	f, err := os.CreateTemp("", "tsguard-secrets-*.baseline")
	if err != nil {
		return "", err
	}
	path := f.Name()
	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(path)
		return "", err
	}
	if err := f.Close(); err != nil {
		os.Remove(path)
		return "", err
	}
	return path, nil
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
