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

// structureFunction is one defined in-scope Python function. Depth is the
// longest path between SCCs in the function-call condensation. Forwarding is
// only meaningful for module-level definitions; the target is the
// module-qualified symbol of the single resolved in-scope function.
type structureFunction struct {
	File           string `json:"file"`
	Line           int    `json:"line"`
	Symbol         string `json:"symbol"`
	Depth          int    `json:"depth"`
	ModuleLevel    bool   `json:"module_level"`
	ForwardingMode string `json:"forwarding_mode"`
	ForwardTarget  string `json:"forward_target"`
}

type structureCall struct {
	Caller int `json:"caller"`
	Callee int `json:"callee"`
}

type structureFunctionComponent struct {
	Functions []int `json:"functions"`
	Cyclic    bool  `json:"cyclic"`
}

type structureFunctionCounts struct {
	Functions           int `json:"functions"`
	Calls               int `json:"calls"`
	RecursiveComponents int `json:"recursive_components"`
	UnresolvedCalls     int `json:"unresolved_calls"`
}

type structureFailureHandler struct {
	File        string `json:"file"`
	Line        int    `json:"line"`
	Fallback    bool   `json:"fallback"`
	Exit        bool   `json:"exit"`
	TokenCount  int    `json:"token_count"`
	Fingerprint string `json:"fingerprint"`
}

// structureReport is the owned analyzer payload. Python builds the runtime
// module graph with grimp and the function graph with pyan3/networkx; Go
// validates both and owns measurements, findings and policy.
type structureReport struct {
	SchemaVersion      string                       `json:"schema_version"`
	Modules            []structureModule            `json:"modules"`
	Imports            []structureImport            `json:"imports"`
	Components         []structureComponent         `json:"components"`
	Counts             structureCounts              `json:"counts"`
	Functions          []structureFunction          `json:"functions"`
	Calls              []structureCall              `json:"calls"`
	FunctionComponents []structureFunctionComponent `json:"function_components"`
	FunctionCounts     structureFunctionCounts      `json:"function_counts"`
	FailureHandlers    []structureFailureHandler    `json:"failure_handlers"`
	Partial            bool                         `json:"partial"`
	Limitation         string                       `json:"limitation"`
	Error              *struct {
		Category string `json:"category"`
		Message  string `json:"message"`
	} `json:"error,omitempty"`
}

func (r *Runner) structureFailurePolicyFindings(gate string, handlers []structureFailureHandler, status Status, refs []string) {
	ordered := append([]structureFailureHandler(nil), handlers...)
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].File == ordered[j].File {
			return ordered[i].Line < ordered[j].Line
		}
		return ordered[i].File < ordered[j].File
	})
	for _, handler := range ordered {
		for _, policy := range []struct {
			match          bool
			rule, evidence string
		}{{handler.Fallback, "SILENT_EXCEPTION_FALLBACK", "A broad exception handler returns a silent fallback value."}, {handler.Exit, "SILENT_EXCEPTION_EXIT", "A broad exception handler silently exits its loop with continue or break."}} {
			if !policy.match {
				continue
			}
			location := Location{File: relativePath(r.root, handler.File), Line: handler.Line}
			r.result.AddFinding(Finding{Gate: gate, Level: "structure", Rule: "pyguard.structure." + policy.rule, Status: status, Severity: "warning", Category: "quality", Location: &location, Evidence: policy.evidence, Remediation: "Review whether this failure policy is intentional and whether callers need a recorded or propagated failure.", Diagnostics: refs})
		}
	}
	groups := map[string][]structureFailureHandler{}
	for _, handler := range ordered {
		if handler.TokenCount >= 12 && handler.Fingerprint != "" {
			groups[handler.Fingerprint] = append(groups[handler.Fingerprint], handler)
		}
	}
	keys := make([]string, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		group := groups[key]
		if len(group) < 3 {
			continue
		}
		locations := make([]Location, len(group))
		for i, h := range group {
			locations[i] = Location{File: relativePath(r.root, h.File), Line: h.Line}
		}
		value := float64(len(group))
		r.result.AddFinding(Finding{Gate: gate, Level: "structure", Rule: "pyguard.structure.REPEATED_ERROR_HANDLER", Status: status, Severity: "warning", Category: "quality", Location: &locations[0], Related: locations[1:], Observed: &value, Evidence: fmt.Sprintf("%d handlers have the same body token sequence (%d tokens).", len(group), group[0].TokenCount), Remediation: "Review whether repeated recovery logic should share a helper or represent an explicit common policy.", Diagnostics: refs})
	}
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

// structureFunctionFanCounts counts distinct in-scope defined callers and
// callees per function. A self-call remains one call edge and one recursive
// component, but a function is neither its own caller nor its own callee.
func structureFunctionFanCounts(report structureReport) (map[int]int, map[int]int) {
	fanIn := map[int]int{}
	fanOut := map[int]int{}
	for _, call := range report.Calls {
		if call.Caller == call.Callee {
			continue
		}
		fanOut[call.Caller]++
		fanIn[call.Callee]++
	}
	return fanIn, fanOut
}

// structureDelegationDepths memoizes the suffix length of each forwarding
// chain. A forwarding cycle has no finite delegation chain, so its depth is
// reported as zero instead of an invented value.
func structureDelegationDepths(report structureReport) []int {
	index := make(map[string]int, len(report.Functions))
	for position, function := range report.Functions {
		index[function.Symbol] = position
	}
	lengths := map[int]int{}
	active := map[int]bool{}
	var depth func(int) int
	depth = func(position int) int {
		if length, ok := lengths[position]; ok {
			return length
		}
		if active[position] {
			return -1
		}
		function := report.Functions[position]
		if function.ForwardTarget == "" {
			lengths[position] = 0
			return 0
		}
		target, ok := index[function.ForwardTarget]
		if !ok {
			lengths[position] = 0
			return 0
		}
		active[position] = true
		length := depth(target)
		delete(active, position)
		if length >= 0 {
			length++
		}
		lengths[position] = length
		return length
	}
	depths := make([]int, len(report.Functions))
	for position := range report.Functions {
		depths[position] = max(0, depth(position))
	}
	return depths
}

func structureFunctionLocation(root string, function structureFunction) Location {
	return Location{File: relativePath(root, function.File), Line: function.Line, Symbol: function.Symbol}
}

// structureDelegationFindings mirrors Tsguard's fixed-threshold delegation
// review cues. A chain shorter than three layers stays silent, and a chain in
// which any forwarding module also has a non-forwarding module-level function
// is not fragmented.
func (r *Runner) structureDelegationFindings(gate string, report structureReport, depths []int, status Status, refs []string) {
	if len(report.Functions) == 0 {
		return
	}
	index := make(map[string]int, len(report.Functions))
	for position, function := range report.Functions {
		index[function.Symbol] = position
	}
	for position, function := range report.Functions {
		depth := depths[position]
		if depth < 3 {
			continue
		}
		locations := []Location{}
		seen := map[string]bool{}
		for next := function.ForwardTarget; next != ""; {
			target, ok := index[next]
			if !ok || seen[next] {
				break
			}
			seen[next] = true
			locations = append(locations, structureFunctionLocation(r.root, report.Functions[target]))
			next = report.Functions[target].ForwardTarget
		}
		location := structureFunctionLocation(r.root, function)
		value, threshold := float64(depth), float64(3)
		r.result.AddFinding(Finding{Gate: gate, Level: "structure", Rule: "pyguard.structure.LONG_DELEGATION_CHAIN", Status: status, Severity: "warning", Category: "quality", Location: &location, Related: locations, Observed: &value, Threshold: &threshold,
			Evidence:    fmt.Sprintf("%d resolved single-return layers forward unchanged positional arguments and an identical textual return annotation; async modes are recorded in the graph and may preserve intentional timing or stack boundaries.", depth),
			Remediation: "Review whether intermediate layers provide a required boundary; preserve deliberate interfaces and failure policies.",
			Diagnostics: refs})
	}

	total, wrappers := map[string]int{}, map[string]int{}
	for _, function := range report.Functions {
		if !function.ModuleLevel {
			continue
		}
		total[function.File]++
		if function.ForwardTarget != "" {
			wrappers[function.File]++
		}
	}
	for _, start := range report.Functions {
		chain := []Location{}
		seen, files := map[string]bool{}, map[string]bool{}
		cyclic := false
		for next := start.Symbol; next != ""; {
			if seen[next] {
				cyclic = true
				break
			}
			seen[next] = true
			position, ok := index[next]
			if !ok {
				break
			}
			function := report.Functions[position]
			if function.ForwardTarget == "" {
				break
			}
			if total[function.File] == 0 || total[function.File] != wrappers[function.File] {
				chain = nil
				break
			}
			chain = append(chain, structureFunctionLocation(r.root, function))
			files[function.File] = true
			next = function.ForwardTarget
		}
		if cyclic || len(chain) < 3 || len(files) < 3 {
			continue
		}
		location := chain[0]
		r.result.AddFinding(Finding{Gate: gate, Level: "structure", Rule: "pyguard.structure.FRAGMENTED_DELEGATION", Status: status, Severity: "warning", Category: "quality", Location: &location, Related: chain[1:],
			Evidence:    fmt.Sprintf("An unchanged forwarding chain crosses %d modules whose analyzed module-level functions only forward calls.", len(files)),
			Remediation: "Review whether these modules express separate responsibilities; file count alone is not a violation.",
			Diagnostics: refs})
	}
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
	if report.Functions == nil || report.Calls == nil || report.FunctionComponents == nil {
		return report, fmt.Errorf("structure function graph arrays are missing")
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
	if report.FunctionCounts.Functions != len(report.Functions) || report.FunctionCounts.Calls != len(report.Calls) || report.FunctionCounts.UnresolvedCalls < 0 {
		return report, fmt.Errorf("function counts disagree with graph facts")
	}
	seenFunctionFacts := map[struct {
		file   string
		line   int
		symbol string
	}]bool{}
	functionSymbols := map[string]int{}
	ambiguousSymbols := map[string]bool{}
	for index, function := range report.Functions {
		if function.File == "" || function.Line < 1 || function.Symbol == "" || function.Depth < 0 {
			return report, fmt.Errorf("invalid function fact")
		}
		key := struct {
			file   string
			line   int
			symbol string
		}{function.File, function.Line, function.Symbol}
		if seenFunctionFacts[key] {
			return report, fmt.Errorf("duplicate function fact")
		}
		seenFunctionFacts[key] = true
		if _, exists := functionSymbols[function.Symbol]; exists {
			ambiguousSymbols[function.Symbol] = true
		} else {
			functionSymbols[function.Symbol] = index
		}
		if function.ForwardTarget == "" {
			if function.ForwardingMode != "" {
				return report, fmt.Errorf("forwarding mode without target")
			}
			continue
		}
		if function.ForwardingMode != "sync" && function.ForwardingMode != "async-return-await" {
			return report, fmt.Errorf("invalid forwarding mode")
		}
		if !function.ModuleLevel {
			return report, fmt.Errorf("non-module forwarding function")
		}
	}
	for index, function := range report.Functions {
		if function.ForwardTarget == "" {
			continue
		}
		target, exists := functionSymbols[function.ForwardTarget]
		if !exists {
			return report, fmt.Errorf("unknown forwarding target %q", function.ForwardTarget)
		}
		if ambiguousSymbols[function.ForwardTarget] {
			return report, fmt.Errorf("ambiguous forwarding target %q", function.ForwardTarget)
		}
		if target == index {
			return report, fmt.Errorf("self forwarding function")
		}
	}
	seenCalls := map[[2]int]bool{}
	selfCalls := map[int]bool{}
	for _, call := range report.Calls {
		if call.Caller < 0 || call.Caller >= len(report.Functions) || call.Callee < 0 || call.Callee >= len(report.Functions) {
			return report, fmt.Errorf("unknown function call endpoint")
		}
		key := [2]int{call.Caller, call.Callee}
		if seenCalls[key] {
			return report, fmt.Errorf("duplicate function call")
		}
		seenCalls[key] = true
		if call.Caller == call.Callee {
			selfCalls[call.Caller] = true
		}
	}
	seenFunctions := map[int]bool{}
	functionCycles := 0
	for _, component := range report.FunctionComponents {
		if len(component.Functions) == 0 {
			return report, fmt.Errorf("empty function component")
		}
		cyclic := len(component.Functions) > 1 || selfCalls[component.Functions[0]]
		if component.Cyclic != cyclic {
			return report, fmt.Errorf("function component cycle flag disagrees with graph facts")
		}
		if cyclic {
			functionCycles++
		}
		for _, index := range component.Functions {
			if index < 0 || index >= len(report.Functions) {
				return report, fmt.Errorf("unknown function component member")
			}
			if seenFunctions[index] {
				return report, fmt.Errorf("function appears in multiple components")
			}
			seenFunctions[index] = true
		}
	}
	if len(seenFunctions) != len(report.Functions) || report.FunctionCounts.RecursiveComponents != functionCycles {
		return report, fmt.Errorf("function component coverage disagrees with graph facts")
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
	functionFanIn, functionFanOut := structureFunctionFanCounts(report)
	delegationDepths := structureDelegationDepths(report)
	status := StatusAdvisory
	if !spec.advisory {
		status = StatusBlocking
	}
	r.structureFailurePolicyFindings(spec.gate, report.FailureHandlers, status, refs)
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
	for index, function := range report.Functions {
		location := Location{File: relativePath(r.root, function.File), Line: function.Line, Symbol: function.Symbol}
		r.result.Measurements = append(r.result.Measurements,
			Measurement{Metric: "function.fan_in", Level: "structure", Location: location, Value: float64(functionFanIn[index]), Unit: "callers"},
			Measurement{Metric: "function.fan_out", Level: "structure", Location: location, Value: float64(functionFanOut[index]), Unit: "callees"},
			Measurement{Metric: "function.call_depth", Level: "structure", Location: location, Value: float64(function.Depth), Unit: "edges"},
			Measurement{Metric: "delegation.depth", Level: "structure", Location: location, Value: float64(delegationDepths[index]), Unit: "layers"},
		)
		if function.ForwardTarget != "" {
			r.result.Measurements = append(r.result.Measurements, Measurement{Metric: "delegation.forwarding", Level: "structure", Location: location, Value: 1, Unit: "calls", ForwardingMode: function.ForwardingMode, ForwardTarget: function.ForwardTarget})
		}
	}
	r.structureDelegationFindings(spec.gate, report, delegationDepths, status, refs)
	r.result.Measurements = append(r.result.Measurements,
		Measurement{Metric: "function.recursive_components", Level: "structure", Location: Location{File: "."}, Value: float64(report.FunctionCounts.RecursiveComponents), Unit: "components"},
		Measurement{Metric: "calls.unresolved", Level: "structure", Location: Location{File: "."}, Value: float64(report.FunctionCounts.UnresolvedCalls), Unit: "calls"},
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
