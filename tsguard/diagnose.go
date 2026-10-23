package main

import (
	"strconv"

	"github.com/oxalc88/oxguard/contract"
)

func runDiagnose(r *Runner, dirs []string, cap int) int {
	steps := []struct {
		name, gate string
		run        func(*Runner) int
	}{
		{"lint", "lint", runLint},
		{"fta", "fta", func(r *Runner) int { return runFTADiagnostic(r, dirs, cap) }},
		{"types", "types", runTypes},
		{"typed-lint", "typed-lint", runTypedLint},
		{"coverage", "coverage", runCoverage},
		{"secrets", "secrets", runSecretlint},
		{"dependencies", "dependencies", runNpmAudit},
		{"security", "security", runOpengrep},
		{"maintainability", "maintainability", func(r *Runner) int { return runMaintainability(r, "maintainability") }},
		{"duplicates", "duplicates", func(r *Runner) int { return runDuplicates(r, dirs) }},
		{"change", "change", runChange},
	}
	for _, step := range steps {
		r.result.PlanAnalyses(contract.Analysis{Name: step.name, Gate: step.gate})
	}
	code := 0
	for i, step := range steps {
		if r.result.CollectAnalysis(i, func(result *RunResult) int {
			analysis := *r
			analysis.result, analysis.outputMode = result, "json"
			status := step.run(&analysis)
			r.snapshot, r.comparison = analysis.snapshot, analysis.comparison
			return status
		}) != 0 {
			code = 1
		}
	}
	return code
}

func runFTADiagnostic(r *Runner, dirs []string, cap int) int {
	config, err := writeFTAConfig(r.root, r.ftaExcludeTests, r.ftaExclude)
	if err != nil {
		return r.executionFailure("fta", "invalid_configuration", "Could not write FTA exclusion config.")
	}
	// Collection uses a separate ceiling; only the effective project cap is policy.
	const collectionCap = 1000000
	code := 0
	for _, dir := range dirs {
		r.result.Measurements = append(r.result.Measurements, Measurement{Metric: "fta.collection_score_cap", Level: "code", Location: Location{File: relativePath(r.root, dir)}, Value: collectionCap, Unit: "score"})
		args := pkgExec(r.pkgManager, "fta", "--score-cap", strconv.Itoa(collectionCap), "--json")
		if config != "" {
			args = append(args, "--config-path", config)
		}
		args = append(args, dir)
		if !r.RunTool(toolSpec{gate: "fta", adapter: "fta-collection", subject: dir, threshold: float64(cap)}, "fta "+dir, args...).ok {
			code = 1
		}
	}
	return code
}
