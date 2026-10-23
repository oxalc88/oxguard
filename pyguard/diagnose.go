package main

import "github.com/oxalc88/oxguard/contract"

func runDiagnose(r *Runner, dirs []string) int {
	steps := []struct {
		name, gate string
		run        func(*Runner) int
	}{
		{"ruff.lint", "ruff", func(r *Runner) int {
			args := append([]string{"uv", "run", "ruff", "check", "--output-format", "json"}, dirs...)
			return diagnosticTool(r, toolSpec{gate: "ruff", adapter: "ruff"}, "ruff check", args)
		}},
		{"ruff.format", "ruff", func(r *Runner) int {
			args := append([]string{"uv", "run", "ruff", "format", "--check"}, dirs...)
			return diagnosticTool(r, toolSpec{gate: "ruff", adapter: "ruff_format"}, "ruff format", args)
		}},
		{"mypy", "mypy", func(r *Runner) int { return runMypy(r, dirs) }},
		{"radon.cc", "radon", func(r *Runner) int {
			r.exportExcludeEnv()
			return diagnosticTool(r, toolSpec{gate: "radon", adapter: "radon_cc"}, "radon cc", append(radonExcludeArgs(r, "cc", "--json"), dirs...))
		}},
		{"radon.mi", "radon", func(r *Runner) int {
			return diagnosticTool(r, toolSpec{gate: "radon", adapter: "radon_mi"}, "radon mi", append(radonExcludeArgs(r, "mi", "--json"), dirs...))
		}},
		{"radon.halstead", "radon", func(r *Runner) int {
			args := append([]string{"uv", "run", "python", "tools/analysis/check_halstead.py", "--json"}, dirs...)
			return diagnosticTool(r, toolSpec{gate: "radon", adapter: "owned"}, "halstead", args)
		}},
		{"types", "types", func(r *Runner) int { return runTypes(r, dirs) }},
		{"coverage", "coverage", runCoverage},
		{"bandit", "bandit", func(r *Runner) int { return runBandit(r, dirs) }},
		{"pip-audit", "pip-audit", runPipAudit},
		{"secrets", "secrets", func(r *Runner) int { return runSecrets(r, config{}) }},
	}
	for _, step := range steps {
		r.result.PlanAnalyses(contract.Analysis{Name: step.name, Gate: step.gate})
	}
	code := 0
	for i, step := range steps {
		if r.result.CollectAnalysis(i, func(result *RunResult) int {
			analysis := *r
			analysis.result, analysis.outputMode = result, "json"
			return step.run(&analysis)
		}) != 0 {
			code = 1
		}
	}
	return code
}

func diagnosticTool(r *Runner, spec toolSpec, name string, args []string) int {
	if !r.RunTool(spec, name, args...).ok {
		return 1
	}
	return 0
}
