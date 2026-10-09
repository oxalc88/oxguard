package main

import (
	_ "embed"
	"fmt"
	"sort"
)

//go:embed analysis/maintainability.cjs
var maintainabilityAnalyzer string

type functionFact struct {
	ID            string   `json:"id"`
	Location      Location `json:"location"`
	ForwardTarget string   `json:"forward_target"`
	Branches      int      `json:"nested_branches"`
}
type handlerFact struct {
	Location    Location `json:"location"`
	Tokens      int      `json:"tokens"`
	Fingerprint string   `json:"fingerprint"`
	Fallback    bool     `json:"fallback"`
}
type maintainabilitySnapshot struct {
	Functions         []functionFact `json:"functions"`
	Calls             []callEdge     `json:"calls"`
	Handlers          []handlerFact  `json:"handlers"`
	UnresolvedCalls   int            `json:"unresolved_calls"`
	Modules           []moduleFact   `json:"modules"`
	Imports           []moduleEdge   `json:"imports"`
	ExternalImports   int            `json:"external_imports"`
	UnresolvedImports int            `json:"unresolved_imports"`
	TypeImports       int            `json:"type_imports"`
}

func runMaintainability(r *Runner, gate string) int {
	return r.runOwnedAnalysis(gate, maintainabilityAnalyzer, true, map[string]any{"excludeTests": r.ftaExcludeTests})
}

func (s *maintainabilitySnapshot) validate() error {
	if s.Functions == nil || s.Calls == nil || s.Handlers == nil || s.UnresolvedCalls < 0 {
		return fmt.Errorf("invalid maintainability snapshot")
	}
	nodes := map[string]bool{}
	for _, f := range s.Functions {
		if f.ID == "" || f.Location.File == "" || f.Location.Line < 1 || nodes[f.ID] || f.Branches < 0 {
			return fmt.Errorf("invalid function fact")
		}
		nodes[f.ID] = true
	}
	for _, f := range s.Functions {
		if f.ForwardTarget != "" && !nodes[f.ForwardTarget] {
			return fmt.Errorf("unknown forwarding target")
		}
	}
	for _, e := range s.Calls {
		if !nodes[e.Caller] || !nodes[e.Callee] {
			return fmt.Errorf("unknown call target")
		}
	}
	for _, h := range s.Handlers {
		if h.Fingerprint == "" || h.Location.File == "" || h.Tokens < 0 {
			return fmt.Errorf("invalid handler fact")
		}
	}
	return nil
}

func (r *Runner) smellFindings(gate string, s *maintainabilitySnapshot, refs []string) {
	nodes := map[string]functionFact{}
	for _, f := range s.Functions {
		nodes[f.ID] = f
	}
	// Memoized suffix lengths; a forwarding cycle has no finite delegation chain.
	lengths, active := map[string]int{}, map[string]bool{}
	var depth func(string) int
	depth = func(id string) int {
		if n, ok := lengths[id]; ok {
			return n
		}
		if active[id] {
			return -1
		}
		f := nodes[id]
		if f.ForwardTarget == "" {
			lengths[id] = 0
			return 0
		}
		active[id] = true
		n := depth(f.ForwardTarget)
		delete(active, id)
		if n >= 0 {
			n++
		}
		lengths[id] = n
		return n
	}
	for _, f := range s.Functions {
		n := depth(f.ID)
		r.measure("delegation.depth", "structure", f.Location, float64(max(0, n)), "layers")
		r.measure("function.branches", "code", f.Location, float64(f.Branches), "branches")
		if n < 3 {
			continue
		}
		locations := []Location{}
		for next := f.ForwardTarget; next != ""; next = nodes[next].ForwardTarget {
			locations = append(locations, nodes[next].Location)
		}
		value, threshold := float64(n), float64(3)
		r.result.AddFinding(Finding{Gate: gate, Level: "structure", Rule: "tsguard.maintainability.LONG_DELEGATION_CHAIN", Severity: "warning", Status: "advisory", Category: "quality", Location: &f.Location, Related: locations, Observed: &value, Threshold: &threshold,
			Evidence: fmt.Sprintf("%d resolved synchronous layers forward the same arguments and return type without transformation.", n), Remediation: "Review whether intermediate layers provide a required boundary; preserve deliberate interfaces and failure policies.", Diagnostics: refs})
	}
	handlers := map[string][]handlerFact{}
	for _, h := range s.Handlers {
		if h.Fallback {
			r.result.AddFinding(Finding{Gate: gate, Rule: "tsguard.maintainability.SILENT_EXCEPTION_FALLBACK", Status: "advisory", Severity: "warning", Category: "quality", Location: &h.Location, Evidence: "The catch body returns only a constant or no value; it does not inspect, propagate or record the caught failure.", Remediation: "Verify this is an intentional best-effort policy; preserve valid recovery and make unexpected failures diagnosable.", Diagnostics: refs})
		}
		if h.Tokens >= 12 {
			handlers[h.Fingerprint] = append(handlers[h.Fingerprint], h)
		}
	}
	keys := []string{}
	for key := range handlers {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		group := handlers[key]
		if len(group) < 3 {
			continue
		}
		locations := []Location{}
		for _, h := range group[1:] {
			locations = append(locations, h.Location)
		}
		value := float64(len(group))
		r.result.AddFinding(Finding{Gate: gate, Rule: "tsguard.maintainability.REPEATED_ERROR_HANDLER", Severity: "warning", Status: "advisory", Category: "quality", Location: &group[0].Location, Related: locations, Observed: &value,
			Evidence: fmt.Sprintf("%d catch blocks contain the same %d tokens, including identifiers and literals.", len(group), group[0].Tokens), Remediation: "Review repeated error policy; consolidate only when recovery and propagation semantics remain equivalent.", Diagnostics: refs})
	}
	r.measure("calls.unresolved", "structure", Location{File: "."}, float64(s.UnresolvedCalls), "calls")
}

func (r *Runner) measure(metric, level string, location Location, value float64, unit string) {
	r.result.Measurements = append(r.result.Measurements, Measurement{Metric: metric, Level: level, Location: location, Value: value, Unit: unit})
}
