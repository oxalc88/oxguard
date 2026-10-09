package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type moduleFact struct {
	ID         string   `json:"id"`
	Location   Location `json:"location"`
	SourceHash string   `json:"source_hash"`
}
type moduleEdge struct {
	ID     string `json:"id"`
	Caller string `json:"caller"`
	Callee string `json:"callee"`
}
type structureFacts struct {
	Out        map[string][]string
	In         map[string][]string
	Depth      map[string]int
	Components [][]string
	Cycles     int
}

// Tarjan SCCs followed by a memoized condensation DAG depth. Cycles have finite
// depth measured between components, never an invented depth inside a cycle.
func analyzeModules(modules []moduleFact, edges []moduleEdge) (structureFacts, error) {
	g := structureFacts{Out: map[string][]string{}, In: map[string][]string{}, Depth: map[string]int{}}
	for _, m := range modules {
		if m.ID == "" || m.Location.File != m.ID {
			return g, fmt.Errorf("invalid module identity")
		}
		if _, ok := g.Out[m.ID]; ok {
			return g, fmt.Errorf("duplicate module identity")
		}
		g.Out[m.ID], g.In[m.ID] = []string{}, []string{}
	}
	seen := map[string]bool{}
	for _, e := range edges {
		if _, ok := g.Out[e.Caller]; !ok {
			return g, fmt.Errorf("unknown importing module")
		}
		if _, ok := g.Out[e.Callee]; !ok {
			return g, fmt.Errorf("unknown imported module")
		}
		key := e.Caller + "\x00" + e.Callee
		if seen[key] {
			continue
		}
		seen[key] = true
		g.Out[e.Caller] = append(g.Out[e.Caller], e.Callee)
		g.In[e.Callee] = append(g.In[e.Callee], e.Caller)
	}
	for id := range g.Out {
		sort.Strings(g.Out[id])
		sort.Strings(g.In[id])
	}
	indexes, low := map[string]int{}, map[string]int{}
	active := map[string]bool{}
	stack := []string{}
	next := 1
	var visit func(string)
	visit = func(id string) {
		indexes[id], low[id] = next, next
		next++
		stack = append(stack, id)
		active[id] = true
		for _, to := range g.Out[id] {
			if indexes[to] == 0 {
				visit(to)
				low[id] = min(low[id], low[to])
			} else if active[to] {
				low[id] = min(low[id], indexes[to])
			}
		}
		if low[id] != indexes[id] {
			return
		}
		component := []string{}
		for {
			last := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			active[last] = false
			component = append(component, last)
			if last == id {
				break
			}
		}
		sort.Strings(component)
		g.Components = append(g.Components, component)
	}
	ids := []string{}
	for id := range g.Out {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		if indexes[id] == 0 {
			visit(id)
		}
	}
	sort.Slice(g.Components, func(i, j int) bool { return g.Components[i][0] < g.Components[j][0] })
	owners := map[string]int{}
	for i, c := range g.Components {
		for _, id := range c {
			owners[id] = i
		}
		cyclic := len(c) > 1
		if len(c) == 1 {
			for _, to := range g.Out[c[0]] {
				if to == c[0] {
					cyclic = true
				}
			}
		}
		if cyclic {
			g.Cycles++
		}
	}
	dag := map[int]map[int]bool{}
	for id, targets := range g.Out {
		for _, to := range targets {
			if owners[id] != owners[to] {
				if dag[owners[id]] == nil {
					dag[owners[id]] = map[int]bool{}
				}
				dag[owners[id]][owners[to]] = true
			}
		}
	}
	depths := map[int]int{}
	var depth func(int) int
	depth = func(id int) int {
		if n, ok := depths[id]; ok {
			return n
		}
		n := 0
		for to := range dag[id] {
			n = max(n, 1+depth(to))
		}
		depths[id] = n
		return n
	}
	for _, id := range ids {
		g.Depth[id] = depth(owners[id])
	}
	return g, nil
}

func (r *Runner) structureFindings(gate string, s *maintainabilitySnapshot, refs []string) error {
	if s.Modules == nil || s.Imports == nil {
		return fmt.Errorf("missing module graph")
	}
	g, err := analyzeModules(s.Modules, s.Imports)
	if err != nil {
		return err
	}
	for _, m := range s.Modules {
		r.measure("module.fan_in", "structure", m.Location, float64(len(g.In[m.ID])), "modules")
		r.measure("module.fan_out", "structure", m.Location, float64(len(g.Out[m.ID])), "modules")
		r.measure("module.dependency_depth", "structure", m.Location, float64(g.Depth[m.ID]), "edges")
		if len(g.Out[m.ID]) >= 8 && len(g.In[m.ID]) >= 3 {
			r.result.AddFinding(Finding{Gate: gate, Level: "structure", Rule: "tsguard.structure.HIGH_MODULE_COUPLING", Status: "advisory", Severity: "warning", Category: "quality", Location: &m.Location,
				Evidence: fmt.Sprintf("%d importing modules depend on this module, which imports %d in-scope runtime modules.", len(g.In[m.ID]), len(g.Out[m.ID])), Remediation: "Review the change surface and cohesion; these counts alone do not prove excessive coupling.", Diagnostics: refs})
		}
	}
	for _, c := range g.Components {
		cyclic := len(c) > 1
		if len(c) == 1 {
			for _, to := range g.Out[c[0]] {
				if to == c[0] {
					cyclic = true
				}
			}
		}
		if !cyclic {
			continue
		}
		locations := []Location{}
		for _, id := range c {
			locations = append(locations, Location{File: id})
		}
		r.result.AddFinding(Finding{Gate: gate, Level: "structure", Rule: "tsguard.structure.CIRCULAR_DEPENDENCY", Status: "advisory", Severity: "warning", Category: "quality", Location: &locations[0], Related: locations[1:],
			Evidence: "Runtime imports form a strongly connected component: " + strings.Join(c, ", "), Remediation: "Inspect initialization and ownership; remove accidental cycles while preserving deliberate domain relationships.", Diagnostics: refs})
	}
	r.fragmentationFindings(gate, s, refs)
	r.measure("module.cycles", "structure", Location{File: "."}, float64(g.Cycles), "components")
	r.measure("module.count", "structure", Location{File: "."}, float64(len(s.Modules)), "modules")
	r.measure("module.edges", "structure", Location{File: "."}, float64(len(s.Imports)), "edges")
	r.measure("module.external_imports", "structure", Location{File: "."}, float64(s.ExternalImports), "references")
	r.measure("module.unresolved_imports", "structure", Location{File: "."}, float64(s.UnresolvedImports), "references")
	r.measure("module.type_imports", "structure", Location{File: "."}, float64(s.TypeImports), "references")
	file := filepath.Join(r.root, opengrepCacheDir, "maintainability-graph.json")
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Dir(file), 0755); err != nil {
		return err
	}
	if err = os.WriteFile(file, data, 0644); err != nil {
		return err
	}
	r.result.Artifacts = append(r.result.Artifacts, Artifact{Kind: "maintainability_graph", Path: relativePath(r.root, file)})
	return nil
}

func (r *Runner) fragmentationFindings(gate string, s *maintainabilitySnapshot, refs []string) {
	nodes := map[string]functionFact{}
	total, wrappers := map[string]int{}, map[string]int{}
	for _, f := range s.Functions {
		nodes[f.ID] = f
		total[f.Location.File]++
		if f.ForwardTarget != "" {
			wrappers[f.Location.File]++
		}
	}
	for _, start := range s.Functions {
		chain := []Location{}
		seen, files := map[string]bool{}, map[string]bool{}
		for next := start.ID; next != "" && !seen[next]; next = nodes[next].ForwardTarget {
			seen[next] = true
			f := nodes[next]
			if f.ForwardTarget == "" {
				break
			}
			if total[f.Location.File] != wrappers[f.Location.File] {
				chain = nil
				break
			}
			chain = append(chain, f.Location)
			files[f.Location.File] = true
		}
		if len(chain) < 3 || len(files) < 3 {
			continue
		}
		r.result.AddFinding(Finding{Gate: gate, Level: "structure", Rule: "tsguard.structure.FRAGMENTED_DELEGATION", Status: "advisory", Severity: "warning", Category: "quality", Location: &chain[0], Related: chain[1:],
			Evidence: fmt.Sprintf("An unchanged forwarding chain crosses %d modules whose analyzed functions only forward calls.", len(files)), Remediation: "Review whether these modules express separate responsibilities; file count alone is not a violation.", Diagnostics: refs})
	}
}
