package main

import (
	"reflect"
	"testing"
)

func TestSCCDepthAndDistinctCoupling(t *testing.T) {
	modules := []moduleFact{{ID: "a.ts", Location: Location{File: "a.ts"}}, {ID: "b.ts", Location: Location{File: "b.ts"}}, {ID: "c.ts", Location: Location{File: "c.ts"}}}
	edges := []moduleEdge{{Caller: "a.ts", Callee: "b.ts"}, {Caller: "a.ts", Callee: "b.ts"}, {Caller: "b.ts", Callee: "a.ts"}, {Caller: "b.ts", Callee: "c.ts"}}
	g, err := analyzeModules(modules, edges)
	if err != nil {
		t.Fatal(err)
	}
	if g.Cycles != 1 || !reflect.DeepEqual(g.Depth, map[string]int{"a.ts": 1, "b.ts": 1, "c.ts": 0}) || len(g.In["b.ts"]) != 1 {
		t.Fatalf("%+v", g)
	}
}

func TestGraphRejectsUnknownTargetsAndDuplicates(t *testing.T) {
	m := moduleFact{ID: "a.ts", Location: Location{File: "a.ts"}}
	if _, err := analyzeModules([]moduleFact{m, m}, nil); err == nil {
		t.Fatal("duplicate accepted")
	}
	if _, err := analyzeModules([]moduleFact{m}, []moduleEdge{{Caller: "a.ts", Callee: "missing.ts"}}); err == nil {
		t.Fatal("unknown accepted")
	}
}

func TestFragmentationNeedsForwardingEvidence(t *testing.T) {
	r := contractRunner(t)
	s := smellSnapshot("b", "c", "d", "")
	for i := range s.Functions {
		s.Functions[i].Location.File = s.Functions[i].ID + ".ts"
	}
	r.fragmentationFindings("structure", s, nil)
	if len(r.result.Findings) != 1 {
		t.Fatal(r.result.Findings)
	}
	r.result.Findings = nil
	s.Functions = append(s.Functions, functionFact{ID: "business", Location: Location{File: "a.ts", Line: 2}})
	r.fragmentationFindings("structure", s, nil)
	if len(r.result.Findings) != 0 {
		t.Fatal("module with business logic is not wrapper-only")
	}
}
