package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

//go:embed analysis/criticality.cjs
var criticalityAnalyzer string

type callNode struct {
	ID       string   `json:"id"`
	Location Location `json:"location"`
}
type callEdge struct {
	Caller string `json:"caller"`
	Callee string `json:"callee"`
}
type callGraph struct {
	Nodes []callNode `json:"nodes"`
	Edges []callEdge `json:"edges"`
	Error *struct {
		Category string `json:"category"`
		Message  string `json:"message"`
	} `json:"error,omitempty"`
	UnresolvedCalls int `json:"unresolved_calls"`
}
type rankedFunction struct {
	callNode
	Callers int
}

func rankCriticality(graph callGraph) ([]rankedFunction, error) {
	nodes := make(map[string]callNode)
	for _, node := range graph.Nodes {
		if node.ID == "" || node.Location.File == "" || node.Location.Symbol == "" || node.Location.Line < 1 {
			return nil, fmt.Errorf("invalid call graph node")
		}
		if _, exists := nodes[node.ID]; exists {
			return nil, fmt.Errorf("duplicate call graph node")
		}
		nodes[node.ID] = node
	}
	callers := make(map[string]map[string]bool)
	for _, edge := range graph.Edges {
		if _, ok := nodes[edge.Caller]; !ok {
			return nil, fmt.Errorf("unknown caller")
		}
		if _, ok := nodes[edge.Callee]; !ok {
			return nil, fmt.Errorf("unknown callee")
		}
		if callers[edge.Callee] == nil {
			callers[edge.Callee] = make(map[string]bool)
		}
		callers[edge.Callee][edge.Caller] = true
	}
	ranked := make([]rankedFunction, 0, len(nodes))
	for id, node := range nodes {
		ranked = append(ranked, rankedFunction{node, len(callers[id])})
	}
	sort.Slice(ranked, func(i, j int) bool {
		if ranked[i].Callers != ranked[j].Callers {
			return ranked[i].Callers > ranked[j].Callers
		}
		return ranked[i].ID < ranked[j].ID
	})
	return ranked, nil
}

func runCriticality(r *Runner) int {
	node := os.Getenv("TSGUARD_NODE")
	if node == "" {
		node = "node"
	}
	input, _ := json.Marshal(map[string]any{"runtime": packagedRuntime(), "dirs": r.dirs, "exclude": r.excludeDirs})
	// Advisory like PyGuard: execution problems are explicit in the result, but
	// neither audit nor this command adds a blocking quality threshold.
	analyzer := *r
	analyzer.outputMode = "json" // complete graph decoding also for the human report
	res := analyzer.RunTool(toolSpec{gate: "criticality", adapter: "criticality", advisory: true}, "criticality", node, "-e", compilerProject+"\n"+criticalityAnalyzer, string(input))
	if !r.machine() {
		if res.ok && len(r.result.Artifacts) > 0 && r.result.Artifacts[len(r.result.Artifacts)-1].Kind == "criticality" {
			r.println("  [OK]   criticality (CRITICALITY.md)")
		} else {
			r.println("  [FAIL] criticality (see diagnostics)")
		}
	}
	return 0
}

func (r *Runner) normalizeCriticality(stdout io.Reader, refs []string) error {
	var graph callGraph
	if err := json.NewDecoder(stdout).Decode(&graph); err != nil {
		return err
	}
	if graph.Error != nil {
		category := graph.Error.Category
		if category != "tool_missing" && category != "invalid_configuration" && category != "analyzer_failure" {
			return fmt.Errorf("unknown analyzer error")
		}
		r.result.Execution("criticality", category, graph.Error.Message)
		r.result.Findings[len(r.result.Findings)-1].Diagnostics = refs
		return nil
	}
	if graph.Nodes == nil || graph.Edges == nil || graph.UnresolvedCalls < 0 {
		return fmt.Errorf("invalid graph envelope")
	}
	ranked, err := rankCriticality(graph)
	if err != nil {
		return err
	}
	var report strings.Builder
	report.WriteString("# Criticality Analysis\n\nFunctions ranked by in-degree (distinct callers). High = high risk to change.\n\n")
	fmt.Fprintf(&report, "Static resolved calls within the selected TypeScript scope. Unresolved or external calls: %d. Module-level calls are excluded.\n\n", graph.UnresolvedCalls)
	report.WriteString("| Rank | Function | Callers |\n|------|----------|---------|\n")
	selected := 0
	for _, function := range ranked {
		r.result.Measurements = append(r.result.Measurements, Measurement{Metric: "criticality.in_degree", Level: "structure", Location: function.Location, Value: float64(function.Callers), Unit: "callers"})
		if function.Callers == 0 || selected == 30 {
			continue
		}
		selected++
		label := fmt.Sprintf("%s:%d %s", function.Location.File, function.Location.Line, function.Location.Symbol)
		// Escape table delimiters and control characters in source names.
		label = strings.NewReplacer("|", "\\|", "`", "'", "\n", " ", "\r", " ").Replace(label)
		fmt.Fprintf(&report, "| %d | `%s` | %d |\n", selected, label, function.Callers)
		observed := float64(function.Callers)
		location := function.Location
		r.result.AddFinding(Finding{Level: "structure", Gate: "criticality", Rule: "tsguard.criticality.ranked", Severity: "info", Status: "advisory", Category: "quality", Location: &location, Observed: &observed, Evidence: fmt.Sprintf("%s has %d distinct callers (rank %d).", function.Location.Symbol, function.Callers, selected), Diagnostics: refs})
	}
	if selected == 0 {
		report.WriteString("\nNo functions with resolved callers in this scope.\n")
	}
	if err := os.WriteFile(filepath.Join(r.root, "CRITICALITY.md"), []byte(report.String()), 0o644); err != nil {
		r.result.Execution("criticality", "artifact_failure", "Cannot write CRITICALITY.md.")
		return nil
	}
	r.result.Artifacts = append(r.result.Artifacts, Artifact{Kind: "criticality", Path: "CRITICALITY.md"})
	r.printf("  Criticality analysis written to CRITICALITY.md\n")
	return nil
}
