package main

import (
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// ruffBaselineVersion identifies the zero-config exception-handling baseline.
// Bump it only when the selected rules or their blocking policy change.
const ruffBaselineVersion = 1

// ruffBaselineSelect is the complete zero-config baseline. S110 (try-except-pass)
// catches a pass-only bare/Exception/BaseException handler under Ruff's default
// check-typed-exception = false, and B012 (return/continue/break inside finally)
// catches control flow that discards an exception. S112 is intentionally not
// included. Ruff honours inline `# noqa` comments natively.
const ruffBaselineSelect = "S110,B012"

// hasRuffConfig reports whether the project root carries Ruff configuration.
// Only project-level configuration counts: pyproject.toml [tool.ruff] or any
// [tool.ruff.*] table, ruff.toml, or .ruff.toml. Nested configuration files are
// Ruff's own concern; they do not change pyguard's root policy.
//
// Unreadable or malformed input is treated conservatively as "configuration
// exists": pyguard then runs Ruff exactly as before, adds no --extend-select and
// records no baseline measurement. Adding CLI selects over a project policy that
// pyguard cannot read could override the project's `ignore` entries, so refusing
// the baseline is the safe direction. A missing pyproject.toml is not an error
// here; the standalone Ruff config files are still checked.
func hasRuffConfig(root string) bool {
	for _, name := range []string{"ruff.toml", ".ruff.toml"} {
		_, err := os.Stat(filepath.Join(root, name))
		if err == nil {
			return true
		}
		// A stat error other than "not present" means we cannot prove the
		// project is unconfigured; preserve the existing Ruff invocation.
		if !os.IsNotExist(err) {
			return true
		}
	}
	data, err := os.ReadFile(filepath.Join(root, "pyproject.toml"))
	if err != nil {
		return !os.IsNotExist(err)
	}
	return pyprojectHasRuffConfig(data)
}

// pyprojectHasRuffConfig detects [tool.ruff] and every [tool.ruff.*] table in
// pyproject.toml. A malformed document reports true (conservative: keep the
// project's Ruff policy and do not inject CLI overrides).
func pyprojectHasRuffConfig(data []byte) bool {
	var raw map[string]any
	if err := toml.Unmarshal(data, &raw); err != nil {
		return true
	}
	tool, ok := raw["tool"].(map[string]any)
	if !ok {
		return false
	}
	_, ok = tool["ruff"]
	return ok
}

// ruffBaselineArgs returns the CLI arguments for the zero-config baseline and
// records its measurement. It returns nil when the project has a Ruff config, so
// configured projects keep exactly their existing invocation: no extra flags,
// no warning and no measurement.
func (r *Runner) ruffBaselineArgs() []string {
	if hasRuffConfig(r.root) {
		return nil
	}
	if r.result != nil {
		r.result.Measurements = append(r.result.Measurements, Measurement{Metric: "ruff.baseline_version", Level: "code", Location: Location{File: "."}, Value: ruffBaselineVersion, Unit: "version"})
	}
	return []string{"--extend-select", ruffBaselineSelect}
}
