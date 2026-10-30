package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
)

// structureModule is one in-scope Python module in the grimp runtime graph.
// File paths are project-relative after normalization by the owned adapter.
type structureModule struct {
	Name  string `json:"name"`
	File  string `json:"file"`
	Depth int    `json:"depth"`
}

type structureImport struct {
	Importer string `json:"importer"`
	Imported string `json:"imported"`
}

type structureComponent struct {
	Modules []string `json:"modules"`
	Cyclic  bool     `json:"cyclic"`
}

type structureCounts struct {
	Modules         int `json:"modules"`
	Edges           int `json:"edges"`
	Cycles          int `json:"cycles"`
	TypeImports     int `json:"type_imports"`
	DeferredImports int `json:"deferred_imports"`
}

// structureReport is the owned analyzer payload. Python builds the runtime
// graph with grimp and networkx; Go validates it and owns findings/policy.
type structureReport struct {
	SchemaVersion string               `json:"schema_version"`
	Modules       []structureModule    `json:"modules"`
	Imports       []structureImport    `json:"imports"`
	Components    []structureComponent `json:"components"`
	Counts        structureCounts      `json:"counts"`
	Partial       bool                 `json:"partial"`
	Limitation    string               `json:"limitation"`
	Error         *struct {
		Category string `json:"category"`
		Message  string `json:"message"`
	} `json:"error,omitempty"`
}

// Joint review thresholds copied from Tsguard: a module is worth reviewing
// only when it is both widely imported and itself imports many modules.
const (
	highModuleCouplingImporters    = 3
	highModuleCouplingDependencies = 8
)

// structureFanCounts counts distinct in-scope runtime importers and
// dependencies per module. A self-import is one runtime edge and one cycle,
// but a module is neither its own importer nor its own dependency.
func structureFanCounts(report structureReport) (map[string]int, map[string]int) {
	fanIn := map[string]int{}
	fanOut := map[string]int{}
	for _, edge := range report.Imports {
		if edge.Importer == edge.Imported {
			continue
		}
		fanOut[edge.Importer]++
		fanIn[edge.Imported]++
	}
	return fanIn, fanOut
}

// parseStructureReport rejects missing, malformed or internally inconsistent
// graph payloads. A partial report is valid evidence, but never a complete one.
func parseStructureReport(data []byte) (structureReport, error) {
	var report structureReport
	if len(bytes.TrimSpace(data)) == 0 {
		return report, fmt.Errorf("structure report is empty")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(&report); err != nil {
		return report, err
	}
	if report.Error != nil || report.SchemaVersion != "1" {
		return report, fmt.Errorf("invalid structure report")
	}
	if report.Modules == nil || report.Imports == nil || report.Components == nil {
		return report, fmt.Errorf("structure report arrays are missing")
	}
	if report.Counts.TypeImports < 0 || report.Counts.DeferredImports < 0 {
		return report, fmt.Errorf("negative import count")
	}
	byName := map[string]structureModule{}
	for _, module := range report.Modules {
		if module.Name == "" || module.File == "" || module.Depth < 0 {
			return report, fmt.Errorf("invalid module fact")
		}
		if _, exists := byName[module.Name]; exists {
			return report, fmt.Errorf("duplicate module %q", module.Name)
		}
		byName[module.Name] = module
	}
	if report.Counts.Modules != len(report.Modules) || report.Counts.Edges != len(report.Imports) {
		return report, fmt.Errorf("module/edge counts disagree with graph facts")
	}
	seenEdges := map[string]bool{}
	selfLoops := map[string]bool{}
	for _, edge := range report.Imports {
		if _, exists := byName[edge.Importer]; !exists {
			return report, fmt.Errorf("unknown importing module %q", edge.Importer)
		}
		if _, exists := byName[edge.Imported]; !exists {
			return report, fmt.Errorf("unknown imported module %q", edge.Imported)
		}
		key := edge.Importer + "\x00" + edge.Imported
		if seenEdges[key] {
			return report, fmt.Errorf("duplicate module edge")
		}
		seenEdges[key] = true
		if edge.Importer == edge.Imported {
			selfLoops[edge.Importer] = true
		}
	}
	seenModules := map[string]bool{}
	cycles := 0
	for _, component := range report.Components {
		if len(component.Modules) == 0 {
			return report, fmt.Errorf("empty component")
		}
		cyclic := len(component.Modules) > 1 || selfLoops[component.Modules[0]]
		if component.Cyclic != cyclic {
			return report, fmt.Errorf("component cycle flag disagrees with graph facts")
		}
		if cyclic {
			cycles++
		}
		for _, name := range component.Modules {
			if _, exists := byName[name]; !exists {
				return report, fmt.Errorf("unknown component module %q", name)
			}
			if seenModules[name] {
				return report, fmt.Errorf("module appears in multiple components")
			}
			seenModules[name] = true
		}
	}
	if len(seenModules) != len(report.Modules) || report.Counts.Cycles != cycles {
		return report, fmt.Errorf("component coverage disagrees with graph facts")
	}
	return report, nil
}

// normalizeStructure validates the runtime graph, records measurements and
// emits one advisory finding per cyclic strongly connected component. A
// partial report adds a not_evaluated advisory and leaves normalization
// incomplete; it never turns an advisory cycle into a blocking failure.
func (r *Runner) normalizeStructure(spec toolSpec, res Result, stdout io.Reader, refs []string) bool {
	data, err := io.ReadAll(stdout)
	if err != nil {
		r.result.AddFinding(Finding{Gate: spec.gate, Rule: "pyguard.execution.adapter_failure", Severity: "error", Status: StatusExecutionError, Category: "adapter_failure", Evidence: "Structure analyzer output cannot be read.", Diagnostics: refs})
		return false
	}
	report, err := parseStructureReport(data)
	if err != nil {
		var failed structureReport
		if json.Unmarshal(data, &failed) == nil && failed.Error != nil {
			category := failed.Error.Category
			if category == "" {
				category = "adapter_failure"
			}
			r.result.Execution(spec.gate, category, failed.Error.Message)
			if len(r.result.Findings) > 0 {
				r.result.Findings[len(r.result.Findings)-1].Diagnostics = refs
			}
			return false
		}
		r.result.AddFinding(Finding{Gate: spec.gate, Rule: "pyguard.execution.adapter_failure", Severity: "error", Status: StatusExecutionError, Category: "adapter_failure", Evidence: "Structure structured output is missing or invalid; inspect diagnostics.", Diagnostics: refs})
		return false
	}

	complete := true
	if !res.ok {
		r.result.AddFinding(Finding{Gate: spec.gate, Rule: "pyguard.execution.analyzer_failure", Severity: "error", Status: StatusExecutionError, Category: "analyzer_failure", Evidence: "Structure analysis exited without a successful report.", Diagnostics: refs})
		complete = false
	}

	fanIn, fanOut := structureFanCounts(report)
	status := StatusAdvisory
	if !spec.advisory {
		status = StatusBlocking
	}
	for _, module := range report.Modules {
		location := Location{File: relativePath(r.root, module.File)}
		r.result.Measurements = append(r.result.Measurements,
			Measurement{Metric: "module.fan_in", Level: "structure", Location: location, Value: float64(fanIn[module.Name]), Unit: "modules"},
			Measurement{Metric: "module.fan_out", Level: "structure", Location: location, Value: float64(fanOut[module.Name]), Unit: "modules"},
			Measurement{Metric: "module.dependency_depth", Level: "structure", Location: location, Value: float64(module.Depth), Unit: "edges"},
		)
		if fanIn[module.Name] < highModuleCouplingImporters || fanOut[module.Name] < highModuleCouplingDependencies {
			continue
		}
		importers, dependencies := fanIn[module.Name], fanOut[module.Name]
		r.result.AddFinding(Finding{Gate: spec.gate, Level: "structure", Rule: "pyguard.structure.HIGH_MODULE_COUPLING", Status: status, Severity: "warning", Category: "quality", Location: &location,
			Evidence:    fmt.Sprintf("%d importing modules depend on this module, which imports %d in-scope runtime modules.", importers, dependencies),
			Remediation: "Review the change surface and cohesion; these counts alone do not prove excessive coupling.",
			Diagnostics: refs})
	}
	for _, component := range report.Components {
		if !component.Cyclic {
			continue
		}
		locations := make([]Location, 0, len(component.Modules))
		for _, name := range component.Modules {
			for _, module := range report.Modules {
				if module.Name == name {
					locations = append(locations, Location{File: relativePath(r.root, module.File)})
					break
				}
			}
		}
		sort.Slice(locations, func(i, j int) bool { return locations[i].File < locations[j].File })
		if len(locations) == 0 {
			continue
		}
		files := make([]string, len(locations))
		for i, location := range locations {
			files[i] = location.File
		}
		r.result.AddFinding(Finding{Gate: spec.gate, Level: "structure", Rule: "pyguard.structure.CIRCULAR_DEPENDENCY", Status: status, Severity: "warning", Category: "quality", Location: &locations[0], Related: locations[1:],
			Evidence:    "Runtime imports form a strongly connected component: " + strings.Join(files, ", "),
			Remediation: "Inspect initialization and ownership; remove accidental cycles while preserving deliberate domain relationships.",
			Diagnostics: refs})
	}
	r.result.Measurements = append(r.result.Measurements,
		Measurement{Metric: "module.count", Level: "structure", Location: Location{File: "."}, Value: float64(report.Counts.Modules), Unit: "modules"},
		Measurement{Metric: "module.edges", Level: "structure", Location: Location{File: "."}, Value: float64(report.Counts.Edges), Unit: "edges"},
		Measurement{Metric: "module.cycles", Level: "structure", Location: Location{File: "."}, Value: float64(report.Counts.Cycles), Unit: "components"},
		Measurement{Metric: "module.type_imports", Level: "structure", Location: Location{File: "."}, Value: float64(report.Counts.TypeImports), Unit: "references"},
		Measurement{Metric: "module.deferred_imports", Level: "structure", Location: Location{File: "."}, Value: float64(report.Counts.DeferredImports), Unit: "references"},
	)
	if report.Partial {
		limitation := strings.TrimSpace(report.Limitation)
		if limitation == "" {
			limitation = "Structure analysis is incomplete."
		}
		r.result.AddFinding(Finding{Gate: spec.gate, Level: "structure", Rule: "pyguard.structure.not_evaluated", Severity: "info", Status: StatusAdvisory, Category: "quality", Evidence: limitation, Diagnostics: refs})
		complete = false
	}
	return complete
}

// structureArgs builds the analyzer invocation. The structured payload is the
// single source of truth in every mode; human output is rendered from the same
// normalized findings.
func structureArgs(r *Runner) []string {
	args := []string{"uv", "run", "python", "tools/analysis/analyze_structure.py", "--json"}
	return append(args, r.dirs...)
}

// runStructure measures the runtime module graph. Cyclic-component and coupling
// findings are advisory in every mode; only analyzer execution failures return 1.
func runStructure(r *Runner) int {
	r.exportExcludeEnv()
	if !r.machine() {
		return runStructureHuman(r)
	}
	before := len(r.result.Findings)
	res := r.RunTool(toolSpec{gate: "structure", adapter: "structure", advisory: true}, "structure", structureArgs(r)...)
	for _, finding := range r.result.Findings[before:] {
		if finding.Status == StatusExecutionError {
			return 1
		}
	}
	if !res.ok {
		return 1
	}
	return 0
}

// runStructureHuman runs the same structured analysis through the adapter and
// renders its advisory findings. It never re-derives cycles or coupling from
// analyzer prose, so human and machine modes share one policy.
func runStructureHuman(r *Runner) int {
	return runStructureHumanReport(r, structureArgs(r)...)
}

func runStructureHumanReport(r *Runner, args ...string) int {
	res, findings := r.runCaptured(toolSpec{gate: "structure", adapter: "structure", advisory: true}, "structure", args...)
	for _, finding := range findings {
		if finding.Status == StatusExecutionError {
			r.printf("  [FAIL] structure — %s\n", finding.Evidence)
			return 1
		}
	}
	if !res.ok {
		r.println("  [FAIL] structure — analyzer exited without a successful report")
		return 1
	}
	reportHumanStructure(os.Stdout, findings)
	return 0
}

// reportHumanStructure preserves the historical success line and adds one
// advisory line per normalized finding, including coupling and cycles.
func reportHumanStructure(w io.Writer, findings []Finding) {
	fmt.Fprintln(w, "  [OK]   structure")
	for _, finding := range findings {
		if finding.Status != StatusAdvisory {
			continue
		}
		if finding.Location != nil && finding.Location.File != "" {
			fmt.Fprintf(w, "  [ADVISORY] %s %s — %s\n", finding.Rule, finding.Location.File, finding.Evidence)
			continue
		}
		fmt.Fprintf(w, "  [ADVISORY] %s — %s\n", finding.Rule, finding.Evidence)
	}
}
