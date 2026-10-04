package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCriticalityDistinctCallersAndStableRanking(t *testing.T) {
	node := func(id string) callNode {
		return callNode{ID: id, Location: Location{File: id + ".ts", Line: 1, Column: 1, Symbol: id}}
	}
	graph := callGraph{Nodes: []callNode{node("c"), node("b"), node("a")}, Edges: []callEdge{{"a", "b"}, {"a", "b"}, {"c", "b"}, {"b", "a"}, {"a", "a"}}}
	ranked, err := rankCriticality(graph)
	if err != nil {
		t.Fatal(err)
	}
	if ranked[0].ID != "a" || ranked[0].Callers != 2 || ranked[1].ID != "b" || ranked[1].Callers != 2 || ranked[2].Callers != 0 {
		t.Fatalf("unexpected rank: %+v", ranked)
	}
	graph.Edges = append(graph.Edges, callEdge{"unknown", "a"})
	if _, err := rankCriticality(graph); err == nil {
		t.Fatal("accepted unknown caller")
	}
}

func TestCriticalityReportTop30AndCompleteMeasurements(t *testing.T) {
	root := t.TempDir()
	graph := callGraph{Nodes: []callNode{}, Edges: []callEdge{}}
	for i := 0; i < 35; i++ {
		id := fmt.Sprintf("function%02d", i)
		graph.Nodes = append(graph.Nodes, callNode{ID: id, Location: Location{File: "src/main.ts", Line: i + 1, Column: 1, Symbol: id}})
		graph.Edges = append(graph.Edges, callEdge{id, id})
	}
	data, _ := json.Marshal(graph)
	result := newRunResult("criticality")
	r := Runner{root: root, outputMode: "json", result: result}
	if err := r.normalizeCriticality(bytes.NewReader(data), []string{"diagnostic-001"}); err != nil {
		t.Fatal(err)
	}
	result.finish(0)
	if len(result.Findings) != 30 || len(result.Measurements) != 35 || result.Status != "advisory" || result.ExitCode != 0 {
		t.Fatalf("unexpected result: %+v", result)
	}
	report, err := os.ReadFile(filepath.Join(root, "CRITICALITY.md"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(report), "| `") != 30 {
		t.Fatalf("unexpected report: %s", report)
	}
	for _, f := range result.Findings {
		if f.Level != "structure" || f.Rule != "tsguard.criticality.ranked" || f.Status != "advisory" {
			t.Fatalf("unexpected finding: %+v", f)
		}
	}
	var agent bytes.Buffer
	if err := reportResult(&agent, "agent", result); err != nil {
		t.Fatal(err)
	}
	if agent.Len() > 6144 || strings.Count(agent.String(), "\n") > 26 {
		t.Fatal("unbounded agent report")
	}
	firstIDs := make([]string, len(result.Findings))
	for i, f := range result.Findings {
		firstIDs[i] = f.ID
	}
	second := newRunResult("criticality")
	r.result = second
	if err := r.normalizeCriticality(bytes.NewReader(data), nil); err != nil {
		t.Fatal(err)
	}
	second.finish(0)
	for i, f := range second.Findings {
		if firstIDs[i] != f.ID {
			t.Fatal("unstable finding ID")
		}
	}
}

func TestCriticalityExecutionErrorRemainsAdvisoryExit(t *testing.T) {
	r := Runner{root: t.TempDir(), outputMode: "json", result: newRunResult("audit")}
	if err := r.normalizeCriticality(strings.NewReader(`{"error":{"category":"tool_missing","message":"TypeScript missing"}}`), []string{"diagnostic-001"}); err != nil {
		t.Fatal(err)
	}
	r.result.finish(0)
	if r.result.Status != "error" || r.result.ExitCode != 0 || r.result.Findings[0].Category != "tool_missing" || len(r.result.Artifacts) != 0 {
		t.Fatalf("unexpected result: %+v", r.result)
	}
}

func TestRelativePathAcrossSymlinkRoot(t *testing.T) {
	base := t.TempDir()
	physical := filepath.Join(base, "physical")
	if err := os.Mkdir(physical, 0o755); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(base, "alias")
	if err := os.Symlink(physical, alias); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	file := filepath.Join(physical, "project.log")
	if err := os.WriteFile(file, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if got := relativePath(alias, file); got != "project.log" {
		t.Fatalf("physical file / alias root: %s", got)
	}
	if got := relativePath(physical, filepath.Join(alias, "project.log")); got != "project.log" {
		t.Fatalf("alias file / physical root: %s", got)
	}
}
