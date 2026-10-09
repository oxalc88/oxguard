package main

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBaselinePathsAndBlobLimits(t *testing.T) {
	for _, name := range []string{"../escape.ts", "/escape.ts", `..\escape.ts`} {
		if _, _, err := baselinePath(name); err == nil {
			t.Fatal("unsafe path accepted", name)
		}
	}
	for _, name := range []string{"node_modules/dependency.ts", ".git/config", "data.png"} {
		if _, selected, err := baselinePath(name); err != nil || selected {
			t.Fatal("unwanted input selected", name)
		}
	}
	root := t.TempDir()
	reader := bufio.NewReader(strings.NewReader("abc blob 3\nxyz\n"))
	if err := copyBaselineBlobs(reader, root, []baselineBlob{{"source.ts", "abc"}}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, "source.ts"))
	if err != nil || string(data) != "xyz" {
		t.Fatal(string(data), err)
	}
	reader = bufio.NewReader(strings.NewReader("abc blob 8388609\n"))
	if err := copyBaselineBlobs(reader, root, []baselineBlob{{"large.ts", "abc"}}); err == nil {
		t.Fatal("large blob accepted")
	}
}

func changeSnapshot(branches []int, edges []moduleEdge) *maintainabilitySnapshot {
	s := smellSnapshot()
	s.Modules = []moduleFact{}
	s.Imports = edges
	for i, n := range branches {
		id := string(rune('a'+i)) + ".ts"
		s.Modules = append(s.Modules, moduleFact{ID: id, Location: Location{File: id}})
		s.Functions = append(s.Functions, functionFact{ID: id, Location: Location{File: id, Line: 1}, Branches: n})
	}
	return s
}

func TestDisplacementRequiresUnreducedBranchesAndStructuralIncrease(t *testing.T) {
	before := changeSnapshot([]int{8}, []moduleEdge{})
	for _, tc := range []struct {
		name     string
		branches []int
		want     int
	}{
		{"artificial split", []int{4, 4}, 1},
		{"real branch reduction", []int{2, 2}, 0},
		{"move without lower file count", []int{8, 0}, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := contractRunner(t)
			after := changeSnapshot(tc.branches, []moduleEdge{{Caller: "a.ts", Callee: "b.ts"}})
			if err := r.compareSnapshots(before, after, "baseline-sha"); err != nil {
				t.Fatal(err)
			}
			if len(r.result.Findings) != tc.want {
				t.Fatal(r.result.Findings)
			}
		})
	}
}

func TestAbsentBaselineIsNotEvaluatedAndNonBlocking(t *testing.T) {
	r := contractRunner(t)
	if code := runChange(r); code != 0 {
		t.Fatal(code)
	}
	r.result.Finish(0)
	if r.result.Assessment != "incomplete" || r.result.Gates[0].Status != "not_run" || r.result.Findings[0].Status != "advisory" {
		t.Fatal(r.result)
	}
}
