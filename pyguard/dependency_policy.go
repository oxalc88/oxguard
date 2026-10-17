package main

import "os"

// Keep native full reports in every output mode. Only complete reports whose
// findings are all explicitly reviewed advisories can override native failure.
func (r *Runner) runDependencyTool(spec toolSpec, name string, args ...string) Result {
	worker := *r
	worker.outputMode = "json"
	if worker.result == nil {
		worker.result = newRunResult(spec.gate)
	}
	before := len(worker.result.Findings)
	res := worker.RunTool(spec, name, args...)
	findings := worker.result.Findings[before:]
	for _, finding := range findings {
		if finding.Status == StatusBlocking || finding.Status == StatusExecutionError {
			res.ok = false
		}
	}
	if !res.ok && res.category == "" && len(findings) > 0 {
		accepted := true
		for _, finding := range findings {
			if finding.Status != StatusAdvisory {
				accepted = false
			}
		}
		if accepted {
			res.ok = true
		}
	}
	if !r.machine() {
		projection := newRunResult(spec.gate)
		projection.Findings = findings
		for _, gate := range worker.result.Gates {
			if gate.Name == spec.gate {
				projection.Gates = append(projection.Gates, gate)
			}
		}
		code := 0
		if !res.ok {
			code = 1
		}
		projection.Finish(code)
		reportHumanTool(os.Stdout, name, projection, r.logFile, res.output)
	}
	return res
}
