package main

// addComplexityContext uses an isolated result so optional context cannot alter
// radon's findings, assessment, gate status or exit code. Failed captures retain
// diagnostics but never contribute invalid envelope measurements or artifacts.
func addComplexityContext(r *Runner, dirs []string) {
	path, err := r.reportFile("complexity-context", "maintainability-graph.json")
	if err != nil {
		return
	}
	worker := *r
	worker.result = newRunResult("radon")
	before := len(r.result.Diagnostics)
	worker.result.Diagnostics = append(worker.result.Diagnostics, r.result.Diagnostics...)
	args := append([]string{"uv", "run", "python", "tools/analysis/complexity_context.py", path}, dirs...)
	res := worker.RunTool(toolSpec{gate: "radon", adapter: "owned"}, "complexity context", args...)
	r.result.Diagnostics = append(r.result.Diagnostics, worker.result.Diagnostics[before:]...)
	if !res.ok || len(worker.result.Findings) != 0 {
		return
	}
	r.result.Measurements = append(r.result.Measurements, worker.result.Measurements...)
	r.result.Artifacts = append(r.result.Artifacts, worker.result.Artifacts...)
}
