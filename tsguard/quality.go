package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"os"
)

//go:embed analysis/project.cjs
var compilerProject string

//go:embed analysis/typed-lint.cjs
var typedLintAnalyzer string

const qualityBaselineVersion = 1

// Pin measurable quality policy independently of upstream preset changes.
// Retain Ultracite's existing cognitive cap rather than tightening it silently.
func qualityBaselineRules() map[string]any {
	return map[string]any{
		"complexity":  map[string]any{"noExcessiveCognitiveComplexity": map[string]any{"level": "error", "options": map[string]any{"maxAllowedComplexity": 20}}, "noUselessCatch": "error"},
		"suspicious":  map[string]any{"noEmptyBlockStatements": "error", "noExplicitAny": "error", "noDuplicateElseIf": "error", "noConstantBinaryExpressions": "error"},
		"correctness": map[string]any{"noUnreachable": "error", "noConstantCondition": "error", "noUnsafeFinally": "error", "noUnsafeOptionalChaining": "error"},
		"style":       map[string]any{"noNonNullAssertion": "error"},
	}
}

func hasLintConfig(root string) bool {
	return hasProjectConfig(root, "biome.json", "biome.jsonc", ".biome.json", ".biome.jsonc", "eslint.config.js", "eslint.config.mjs", "eslint.config.cjs", "eslint.config.ts", "eslint.config.mts", "eslint.config.cts", ".eslintrc", ".eslintrc.json", ".eslintrc.js", ".eslintrc.cjs", ".eslintrc.yml", ".eslintrc.yaml", ".oxlintrc.json", "oxlint.config.ts")
}

func runTypedLint(r *Runner) int {
	return r.runOwnedAnalysis("typed-lint", typedLintAnalyzer, false, nil)
}

func (r *Runner) runOwnedAnalysis(gate, source string, advisory bool, extra map[string]any) int {
	if r.result == nil {
		r.result = newRunResult(gate)
	}
	node := os.Getenv("TSGUARD_NODE")
	if node == "" {
		node = "node"
	}
	input := map[string]any{"runtime": packagedRuntime(), "dirs": r.dirs, "exclude": append(append([]string{}, r.excludeDirs...), ".git")}
	for k, v := range extra {
		input[k] = v
	}
	data, err := json.Marshal(input)
	if err != nil {
		return r.executionFailure(gate, "invalid_configuration", err.Error())
	}
	analyzer := *r
	analyzer.outputMode = "json"
	res := analyzer.RunTool(toolSpec{gate: gate, adapter: "owned-analysis", advisory: advisory}, gate, node, "-e", compilerProject+"\n"+source, string(data))
	failed := !res.ok
	for _, f := range r.result.Findings {
		if f.Gate == gate && (f.Status == "execution_error" || f.Status == "blocking") {
			failed = true
		}
	}
	if !r.machine() {
		r.printf("  %s: %d findings (use --output json for complete evidence)\n", gate, len(r.result.Findings))
	}
	if failed && !advisory {
		return 1
	}
	return 0
}

func (r *Runner) normalizeOwnedAnalysis(spec toolSpec, stdout io.Reader, refs []string) (bool, error) {
	var report struct {
		Findings     []Finding     `json:"findings"`
		Measurements []Measurement `json:"measurements"`
		Partial      bool          `json:"partial"`
		Limitation   string        `json:"limitation"`
		Error        *struct {
			Category string `json:"category"`
			Message  string `json:"message"`
		} `json:"error"`
		Snapshot *maintainabilitySnapshot `json:"snapshot"`
	}
	if err := json.NewDecoder(stdout).Decode(&report); err != nil {
		return false, err
	}
	if report.Error != nil {
		category := report.Error.Category
		if category != "tool_missing" && category != "invalid_configuration" && category != "analyzer_failure" {
			return false, fmt.Errorf("unknown analyzer error")
		}
		r.result.Execution(spec.gate, category, report.Error.Message)
		return false, nil
	}
	if report.Findings == nil || report.Measurements == nil {
		return false, fmt.Errorf("missing findings/measurements arrays")
	}
	for _, f := range report.Findings {
		if f.Rule == "" || f.Evidence == "" || f.Location == nil || f.Location.File == "" {
			return false, fmt.Errorf("invalid owned finding")
		}
		f.Gate, f.Diagnostics = spec.gate, refs
		if spec.advisory {
			f.Status = "advisory"
		}
		r.result.AddFinding(f)
	}
	r.result.Measurements = append(r.result.Measurements, report.Measurements...)
	if report.Snapshot != nil {
		if err := report.Snapshot.validate(); err != nil {
			return false, err
		}
		r.smellFindings(spec.gate, report.Snapshot, refs)
	}
	if report.Partial {
		r.result.AddFinding(Finding{Gate: spec.gate, Rule: "tsguard." + spec.gate + ".not_evaluated", Severity: "info", Status: "advisory", Category: "quality", Evidence: report.Limitation, Diagnostics: refs})
	}
	return !report.Partial, nil
}
