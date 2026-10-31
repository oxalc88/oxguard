package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func (r *Runner) comparisonUnavailable(rule, message string) {
	r.result.Plan("change")
	remediation := "Provide an available Git commit with --baseline and rerun change; per-file scores alone do not establish improvement."
	if rule == "INCOMPLETE_INPUT" {
		remediation = "Resolve source-scope, parsing, local-import or analyzer-version errors and rerun the same baseline comparison."
	}
	if rule == "INCOMPLETE_METRICS" {
		remediation = "Inspect analyzer diagnostics, restore Radon or resolve measurement errors, and rerun the same baseline comparison."
	}
	if r.baselineSHA != "" {
		message = "Baseline " + r.baselineSHA + ": " + message
	}
	r.result.AddFinding(Finding{Gate: "change", Level: "change", Rule: "pyguard.change." + rule, Status: StatusAdvisory, Severity: "info", Category: "quality", Evidence: message, Remediation: remediation})
	r.printf("  change: not evaluated — %s\n", message)
}

func changeExitCode(r *Runner, code int) int {
	if code != 0 {
		return code
	}
	if r.result == nil || len(r.result.Gates) == 0 {
		return 1
	}
	for _, g := range r.result.Gates {
		if g.Status == StatusNotRun || g.Status == StatusError || g.Normalization != Complete {
			return 1
		}
	}
	return 0
}

// Check keeps its existing blocking outcome, even if the optional advisory
// comparison cannot execute. Its partial change gate still records that limit.
func runChangeAdvisory(r *Runner) {
	worker := *r
	worker.result = newRunResult("change")
	runChange(&worker)
	complete := changeExitCode(&worker, 0) == 0
	for _, finding := range worker.result.Findings {
		if finding.Status == StatusExecutionError {
			finding.Status, finding.Category, finding.Level = StatusAdvisory, "quality", "change"
			finding.Rule, finding.Severity = "pyguard.change.INCOMPLETE_INPUT", "info"
		}
		r.result.AddFinding(finding)
	}
	r.result.Measurements = append(r.result.Measurements, worker.result.Measurements...)
	r.result.Artifacts = append(r.result.Artifacts, worker.result.Artifacts...)
	// Worker diagnostics are persisted in the candidate checkout. Renumber
	// references before merging to avoid collisions with check diagnostics.
	ids := map[string]string{}
	for _, diagnostic := range worker.result.Diagnostics {
		old := diagnostic.ID
		diagnostic.ID = fmt.Sprintf("change-%s", old)
		ids[old] = diagnostic.ID
		r.result.Diagnostics = append(r.result.Diagnostics, diagnostic)
	}
	for i := range r.result.Findings {
		if r.result.Findings[i].Gate == "change" {
			for j, id := range r.result.Findings[i].Diagnostics {
				if replacement, ok := ids[id]; ok {
					r.result.Findings[i].Diagnostics[j] = replacement
				}
			}
		}
	}
	r.result.RecordGate("change", true, complete)
}

func runChange(r *Runner) int {
	if r.result == nil {
		r.result = newRunResult("change")
	}
	r.result.Plan("change")
	if r.baseline == "" {
		r.comparisonUnavailable("BASELINE_UNAVAILABLE", "No explicit Git baseline; change quality was not evaluated.")
		return 0
	}
	if _, err := exec.LookPath("git"); err != nil {
		return r.executionFailure("change", "tool_missing", "Git is required for --baseline.")
	}
	timeout := r.timeout
	if timeout <= 0 {
		timeout = 300
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--verify", "--end-of-options", r.baseline+"^{commit}")
	cmd.Dir = r.root
	out, err := cmd.Output()
	if err != nil {
		r.comparisonUnavailable("BASELINE_UNAVAILABLE", "Requested Git baseline could not be resolved to a commit.")
		return 0
	}
	r.baselineSHA = strings.TrimSpace(string(out))
	root, err := baselineRoot(ctx, r.root, r.baselineSHA)
	if err != nil {
		r.comparisonUnavailable("INCOMPLETE_INPUT", err.Error())
		return 0
	}
	defer os.RemoveAll(root)
	dirs := []string{}
	for _, dir := range r.dirs {
		if filepath.IsAbs(dir) {
			dir, err = filepath.Rel(r.root, dir)
			if err != nil {
				r.comparisonUnavailable("INCOMPLETE_INPUT", "Source scope is outside the project.")
				return 0
			}
		}
		clean := filepath.Clean(dir)
		if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			r.comparisonUnavailable("INCOMPLETE_INPUT", "Source scope is outside the project.")
			return 0
		}
		dirs = append(dirs, clean)
	}
	if len(dirs) == 0 {
		dirs = []string{"."}
	}
	payload, _ := json.Marshal(map[string]any{"baseline": root, "candidate": r.root, "dirs": dirs, "exclude": r.exclude, "exclude_tests": r.excludeTests})
	helper, cleanup, err := materializeAnalysisHelper("tools/analysis/analyze_change.py")
	if err != nil {
		return r.executionFailure("change", "analyzer_failure", err.Error())
	}
	defer cleanup()
	// Resolve Python and dependencies in the candidate installation once. uv
	// never runs from the baseline and --no-sync prevents build/install hooks.
	worker := *r
	worker.outputMode = "json"
	start := len(r.result.Findings)
	worker.RunTool(toolSpec{gate: "change", adapter: "change", advisory: true}, "change", "uv", "run", "--no-sync", "python", "-E", helper, string(payload))
	for i := start; i < len(r.result.Findings); i++ {
		finding := &r.result.Findings[i]
		if finding.Status == StatusExecutionError {
			finding.Evidence = "Baseline " + r.baselineSHA + ": " + finding.Evidence
		}
		r.printf("  [ADVISORY] %s — %s\n", finding.Rule, finding.Evidence)
	}
	return 0
}

type changeFileMetric struct {
	File                string  `json:"file"`
	SourceHash          string  `json:"source_hash"`
	Cyclomatic          float64 `json:"cyclomatic"`
	MI                  float64 `json:"maintainability_index"`
	Halstead            float64 `json:"halstead_volume"`
	Branches            int     `json:"branches"`
	MaxFunctionBranches int     `json:"max_function_branches"`
}
type changeProfile struct {
	Functions      []changeFunctionMetric `json:"functions"`
	CognitiveError string                 `json:"cognitive_error,omitempty"`
	Structure      json.RawMessage        `json:"structure"`
	Versions       map[string]string      `json:"versions"`
	Files          []changeFileMetric     `json:"files"`
	InputError     string                 `json:"input_error"`
	MetricsError   string                 `json:"metrics_error"`
}
type changeMetrics struct {
	Modules, Edges, Cycles, Depth, Wrappers, Branches, MaxFileBranches, MaxFunctionBranches int
	MaxFanIn, MaxFanOut, CouplingHotspots, FunctionDepth, FunctionCycles                    int
	Cyclomatic, Halstead, MinMI, MaxCC                                                      float64
}

func profileMetrics(s structureReport, files []changeFileMetric) (changeMetrics, error) {
	m := changeMetrics{Modules: len(s.Modules), Edges: len(s.Imports), Cycles: s.Counts.Cycles, FunctionCycles: s.FunctionCounts.RecursiveComponents, MinMI: 100}
	in, out := structureFanCounts(s)
	expected := map[string]bool{}
	for _, module := range s.Modules {
		expected[module.File] = true
		m.Depth = max(m.Depth, module.Depth)
		m.MaxFanIn = max(m.MaxFanIn, in[module.Name])
		m.MaxFanOut = max(m.MaxFanOut, out[module.Name])
		if in[module.Name] >= 3 && out[module.Name] >= 8 {
			m.CouplingHotspots++
		}
	}
	for _, function := range s.Functions {
		m.FunctionDepth = max(m.FunctionDepth, function.Depth)
		if function.ForwardTarget != "" {
			m.Wrappers++
		}
	}
	for _, f := range files {
		if !expected[f.File] {
			return m, fmt.Errorf("missing or duplicate file identity")
		}
		delete(expected, f.File)
		for _, n := range []float64{f.Cyclomatic, f.MI, f.Halstead} {
			if math.IsNaN(n) || math.IsInf(n, 0) || n < 0 {
				return m, fmt.Errorf("invalid Radon measurement")
			}
		}
		if f.MI > 100 || f.Branches < 0 || f.MaxFunctionBranches < 0 || f.MaxFunctionBranches > f.Branches {
			return m, fmt.Errorf("invalid Radon branch measurement")
		}
		m.Branches += f.Branches
		m.MaxFileBranches = max(m.MaxFileBranches, f.Branches)
		m.MaxFunctionBranches = max(m.MaxFunctionBranches, f.MaxFunctionBranches)
		m.Cyclomatic += f.Cyclomatic
		m.Halstead += f.Halstead
		m.MinMI = min(m.MinMI, f.MI)
		m.MaxCC = max(m.MaxCC, f.Cyclomatic)
	}
	if len(expected) > 0 || files == nil {
		return m, fmt.Errorf("Radon file coverage is incomplete")
	}
	return m, nil
}

func (r *Runner) normalizeChange(stdout io.Reader, refs []string) bool {
	start := len(r.result.Findings)
	defer func() {
		for i := start; i < len(r.result.Findings); i++ {
			r.result.Findings[i].Diagnostics = append([]string(nil), refs...)
		}
	}()
	var report struct {
		Baseline  changeProfile `json:"baseline"`
		Candidate changeProfile `json:"candidate"`
	}
	decoder := json.NewDecoder(stdout)
	if err := decoder.Decode(&report); err != nil {
		r.comparisonUnavailable("INCOMPLETE_INPUT", "Comparison analyzer output is missing or invalid.")
		return false
	}
	var trailing any
	if decoder.Decode(&trailing) != io.EOF {
		r.comparisonUnavailable("INCOMPLETE_INPUT", "Comparison analyzer output contains trailing data.")
		return false
	}
	b, a := report.Baseline, report.Candidate
	base, eb := parseStructureReport(b.Structure)
	candidate, ea := parseStructureReport(a.Structure)
	if eb != nil || ea != nil || base.Partial || candidate.Partial || b.InputError != "" || a.InputError != "" {
		r.comparisonUnavailable("INCOMPLETE_INPUT", fmt.Sprintf("Baseline or candidate inputs are incomplete; comparison was not evaluated. %s %s %s %s", b.InputError, a.InputError, base.Limitation, candidate.Limitation))
		return false
	}
	if len(b.Versions) < 4 || len(a.Versions) < 4 {
		r.comparisonUnavailable("INCOMPLETE_INPUT", "Analyzer version evidence is missing.")
		return false
	}
	for _, name := range []string{"python", "grimp", "networkx", "pyan3"} {
		version := b.Versions[name]
		if version == "" || a.Versions[name] != version {
			r.comparisonUnavailable("INCOMPLETE_INPUT", "Baseline and candidate analyzer versions differ.")
			return false
		}
	}
	bm, eb := profileMetrics(base, b.Files)
	am, ea := profileMetrics(candidate, a.Files)
	if b.MetricsError != "" || a.MetricsError != "" || eb != nil || ea != nil {
		r.comparisonUnavailable("INCOMPLETE_METRICS", fmt.Sprintf("Radon metrics are incomplete. %s %s", b.MetricsError, a.MetricsError))
		return false
	}
	if b.Versions["radon"] == "" || b.Versions["radon"] != a.Versions["radon"] {
		r.comparisonUnavailable("INCOMPLETE_INPUT", "Baseline and candidate Radon versions differ.")
		return false
	}
	r.compareChange(bm, am, b.Files, a.Files)
	return r.compareFunctionMetrics(b, a)
}

func displacedComplexity(b, a changeMetrics) bool {
	reduced := a.MaxFileBranches < b.MaxFileBranches || (a.MaxCC < b.MaxCC || a.MinMI > b.MinMI) && a.Cyclomatic >= b.Cyclomatic
	return reduced && a.Branches >= b.Branches && a.Edges > b.Edges && (a.Depth > b.Depth || a.Wrappers > b.Wrappers || a.Cycles > b.Cycles)
}

func (r *Runner) changeMeasure(metric, file string, value float64, unit string) {
	r.result.Measurements = append(r.result.Measurements, Measurement{Metric: metric, Level: "change", Location: Location{File: file}, Value: value, Unit: unit})
}
func (r *Runner) compareChange(b, a changeMetrics, bfiles, afiles []changeFileMetric) {
	for _, metric := range []struct {
		name string
		b, a int
	}{
		{"module_count", b.Modules, a.Modules}, {"module_edges", b.Edges, a.Edges}, {"module_cycles", b.Cycles, a.Cycles}, {"dependency_depth", b.Depth, a.Depth}, {"max_module_fan_in", b.MaxFanIn, a.MaxFanIn}, {"max_module_fan_out", b.MaxFanOut, a.MaxFanOut}, {"coupling_hotspots", b.CouplingHotspots, a.CouplingHotspots}, {"function_call_depth", b.FunctionDepth, a.FunctionDepth}, {"function_recursive_components", b.FunctionCycles, a.FunctionCycles}, {"wrappers", b.Wrappers, a.Wrappers}, {"total_branches", b.Branches, a.Branches}, {"max_file_branches", b.MaxFileBranches, a.MaxFileBranches}, {"max_function_branches", b.MaxFunctionBranches, a.MaxFunctionBranches},
	} {
		r.changeMeasure("change.baseline."+metric.name, ".", float64(metric.b), "count")
		r.changeMeasure("change.candidate."+metric.name, ".", float64(metric.a), "count")
	}
	for _, side := range []struct {
		name  string
		m     changeMetrics
		files []changeFileMetric
	}{{"baseline", b, bfiles}, {"candidate", a, afiles}} {
		prefix := "change." + side.name + "."
		for _, f := range side.files {
			r.changeMeasure(prefix+"file.cyclomatic", f.File, f.Cyclomatic, "paths")
			r.changeMeasure(prefix+"file.maintainability_index", f.File, f.MI, "score")
			r.changeMeasure(prefix+"file.halstead_volume", f.File, f.Halstead, "volume")
		}
		r.changeMeasure(prefix+"total_cyclomatic", ".", side.m.Cyclomatic, "paths")
		r.changeMeasure(prefix+"total_halstead_volume", ".", side.m.Halstead, "volume")
		r.changeMeasure(prefix+"min_maintainability_index", ".", side.m.MinMI, "score")
	}
	before, after := map[string]changeFileMetric{}, map[string]changeFileMetric{}
	for _, f := range bfiles {
		before[f.File] = f
	}
	for _, f := range afiles {
		after[f.File] = f
	}
	matches, _ := profileMatches(changeProfile{Files: bfiles}, changeProfile{Files: afiles})
	for _, match := range matches {
		old, f := before[match.Baseline], after[match.Candidate]
		r.changeMeasure("change.delta.file.cyclomatic", f.File, f.Cyclomatic-old.Cyclomatic, "paths")
		r.changeMeasure("change.delta.file.maintainability_index", f.File, f.MI-old.MI, "score")
	}
	add := func(rule, evidence, remediation string) {
		r.result.AddFinding(Finding{Gate: "change", Level: "change", Rule: "pyguard.change." + rule, Status: StatusAdvisory, Severity: "warning", Category: "quality", Location: &Location{File: "."}, Evidence: "Baseline " + r.baselineSHA + ": " + evidence, Remediation: remediation})
	}
	if displacedComplexity(b, a) {
		add("POSSIBLE_COMPLEXITY_DISPLACEMENT", fmt.Sprintf("Maximum per-file branches %d→%d, total branches %d→%d, runtime edges %d→%d, dependency depth %d→%d, forwarding functions %d→%d, cycles %d→%d; maximum file CC %g→%g, total CC %g→%g, minimum MI %g→%g.", b.MaxFileBranches, a.MaxFileBranches, b.Branches, a.Branches, b.Edges, a.Edges, b.Depth, a.Depth, b.Wrappers, a.Wrappers, b.Cycles, a.Cycles, b.MaxCC, a.MaxCC, b.Cyclomatic, a.Cyclomatic, b.MinMI, a.MinMI), "Review the end-to-end workflow and behavior before claiming simplification; smaller file scores alone do not show reduced maintainability cost.")
	}
	if a.Cycles > b.Cycles {
		add("NEW_DEPENDENCY_CYCLES", fmt.Sprintf("Runtime dependency cycles increased from %d to %d.", b.Cycles, a.Cycles), "Inspect initialization and ownership; preserve deliberate domain relationships.")
	}
	if a.CouplingHotspots > b.CouplingHotspots {
		add("INCREASED_COUPLING", fmt.Sprintf("Modules with at least 3 importers and 8 runtime dependencies increased from %d to %d.", b.CouplingHotspots, a.CouplingHotspots), "Review the changed dependency surface and cohesion; these counts do not prescribe an architecture.")
	}
}
