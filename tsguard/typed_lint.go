package main

import (
	_ "embed"
	"os"
	"path/filepath"
)

//go:embed analysis/typed-lint.cjs
var typedLintAnalyzer string

func runTypedLint(r *Runner) int {
	timeout := r.timeout
	if timeout <= 0 {
		timeout = 300
	}
	report := filepath.Join(r.root, opengrepCacheDir, "typed-lint", "native.json")
	if err := os.Remove(report); err != nil && !os.IsNotExist(err) {
		return r.executionFailure("typed-lint", "artifact_failure", "Cannot remove stale native typed-lint report.")
	}
	code := r.runOwnedAnalysis("typed-lint", typedLintAnalyzer, false, map[string]any{"timeout": timeout, "excludeTests": r.ftaExcludeTests})
	if _, err := os.Stat(report); err == nil {
		r.result.Artifacts = append(r.result.Artifacts, Artifact{Kind: "typed_lint_native", Path: relativePath(r.root, report)})
	}
	return code
}
