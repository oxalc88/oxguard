package main

import (
	"archive/tar"
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestBaselineArchiveDoesNotWriteEscapesOrLinks(t *testing.T) {
	for _, name := range []string{"../escape.ts", "/escape.ts", `..\escape.ts`} {
		var b bytes.Buffer
		w := tar.NewWriter(&b)
		w.WriteHeader(&tar.Header{Name: name, Mode: 0600, Size: 1})
		w.Write([]byte("x"))
		w.Close()
		if err := exportBaseline(&b, t.TempDir()); err == nil {
			t.Fatal("unsafe archive accepted", name)
		}
	}
	var b bytes.Buffer
	w := tar.NewWriter(&b)
	w.WriteHeader(&tar.Header{Name: "linked.ts", Typeflag: tar.TypeSymlink, Linkname: "/tmp/escape"})
	w.Close()
	root := t.TempDir()
	if err := exportBaseline(&b, root); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(root, "linked.ts")); !os.IsNotExist(err) {
		t.Fatal("symlink written")
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
