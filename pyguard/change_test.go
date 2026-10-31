package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestBaselinePathsAndBlobLimits(t *testing.T) {
	for _, name := range []string{"../escape.py", "/escape.py", `..\escape.py`, "a/../escape.py", "./escape.py", "a//b.py"} {
		if _, _, err := baselinePath(name); err == nil {
			t.Fatal("unsafe path accepted", name)
		}
	}
	for _, name := range []string{"node_modules/a.py", ".git/a.py", "venv/a.py", ".venv/a.py", "__pycache__/a.py", "image.png"} {
		if _, selected, err := baselinePath(name); err != nil || selected {
			t.Fatal("unwanted path", name, err)
		}
	}
	for _, name := range []string{"src/a.py", "src/a.pyi", "pyproject.toml", "setup.cfg"} {
		if _, selected, err := baselinePath(name); err != nil || !selected {
			t.Fatal("missing path", name, err)
		}
	}
	root := t.TempDir()
	if err := copyBaselineBlobs(bufio.NewReader(strings.NewReader("abc blob 3\nxyz\n")), root, []baselineBlob{{"src/a.py", "abc"}}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, "src/a.py"))
	if err != nil || string(data) != "xyz" {
		t.Fatal(string(data), err)
	}
	for _, payload := range []string{"abc blob 8388609\n", "abc blob -1\n", "wrong blob 2\nxx\n", "abc tree 2\nxx\n", "abc blob 2\nxx!", "abc blob 3\nx"} {
		if err := copyBaselineBlobs(bufio.NewReader(strings.NewReader(payload)), t.TempDir(), []baselineBlob{{"a.py", "abc"}}); err == nil {
			t.Fatal("invalid payload accepted", payload)
		}
	}
	if err := copyBaselineBlobs(bufio.NewReader(strings.NewReader("abc blob 0\n\n")), root, []baselineBlob{{"../escape.py", "abc"}}); err == nil {
		t.Fatal("unsafe direct blob path accepted")
	}
}

func TestBaselineRootUsesNestedPrefixAndSkipsLinks(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip(err)
	}
	root := t.TempDir()
	git := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	git("init")
	if err := os.MkdirAll(filepath.Join(root, "nested/src"), 0755); err != nil {
		t.Fatal(err)
	}
	for name, text := range map[string]string{"outside.py": "outside", "nested/src/a.py": "inside", "nested/pyproject.toml": "[project]"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(text), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink("a.py", filepath.Join(root, "nested/src/link.py")); err != nil {
		t.Skip(err)
	}
	git("add", ".")
	git("-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "-c", "core.hooksPath=/dev/null", "commit", "-m", "fixture")
	baseline, err := baselineRoot(context.Background(), filepath.Join(root, "nested"), git("rev-parse", "HEAD"))
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(baseline)
	if data, err := os.ReadFile(filepath.Join(baseline, "src/a.py")); err != nil || string(data) != "inside" {
		t.Fatal(string(data), err)
	}
	for _, name := range []string{"outside.py", "src/link.py"} {
		if _, err := os.Lstat(filepath.Join(baseline, name)); !os.IsNotExist(err) {
			t.Fatal("unexpected baseline file", name, err)
		}
	}
}

func TestChangeExitCodeAndAbsentBaseline(t *testing.T) {
	r := &Runner{result: newRunResult("change"), outputMode: "json"}
	if code := runChange(r); code != 0 || changeExitCode(r, code) != 1 {
		t.Fatal("absent baseline was complete")
	}
	if r.result.Findings[0].Rule != "pyguard.change.BASELINE_UNAVAILABLE" || r.result.Findings[0].Status != StatusAdvisory {
		t.Fatal(r.result.Findings)
	}
	for _, tc := range []struct {
		status   Status
		complete bool
		want     int
	}{{StatusPassed, true, 0}, {StatusAdvisory, true, 0}, {StatusNotRun, true, 1}, {StatusError, true, 1}, {StatusPassed, false, 1}} {
		r.result = newRunResult("change")
		r.result.RecordGate("change", true, tc.complete)
		r.result.Gates[0].Status = tc.status
		if got := changeExitCode(r, 0); got != tc.want {
			t.Fatal(tc, got)
		}
		if got := changeExitCode(r, 7); got != 7 {
			t.Fatal(got)
		}
	}
	if changeExitCode(&Runner{}, 0) != 1 {
		t.Fatal("nil result complete")
	}
}

func TestDisplacementRequiresUnreducedWorkAndStructure(t *testing.T) {
	b := changeMetrics{Branches: 8, MaxFileBranches: 8, Cyclomatic: 10, MaxCC: 10, MinMI: 30}
	a := changeMetrics{Branches: 8, MaxFileBranches: 4, Cyclomatic: 12, MaxCC: 6, MinMI: 40, Edges: 1, Depth: 1}
	if !displacedComplexity(b, a) {
		t.Fatal("split not detected")
	}
	for _, adjust := range []func(*changeMetrics){func(a *changeMetrics) { a.Branches = 4 }, func(a *changeMetrics) { a.Edges = 0 }, func(a *changeMetrics) { a.Depth = 0 }, func(a *changeMetrics) { a.MaxFileBranches = 8; a.MaxCC = 10; a.MinMI = 30 }} {
		control := a
		adjust(&control)
		if displacedComplexity(b, control) {
			t.Fatal("false positive", control)
		}
	}
	a.MaxFileBranches = 8
	a.MaxCC = 10
	if !displacedComplexity(b, a) {
		t.Fatal("MI evidence ignored")
	}
	a.Cyclomatic = 8
	if displacedComplexity(b, a) {
		t.Fatal("native reduction judged displacement despite lower total CC")
	}
}

func TestChangeFileDeltasRequirePathOrFingerprintEvidence(t *testing.T) {
	r := &Runner{result: newRunResult("change"), baselineSHA: "abc"}
	r.compareChange(changeMetrics{}, changeMetrics{Cycles: 1, CouplingHotspots: 1}, []changeFileMetric{{File: "same.py", Cyclomatic: 3, MI: 40}, {File: "old.py", Cyclomatic: 9}}, []changeFileMetric{{File: "same.py", Cyclomatic: 2, MI: 50}, {File: "new.py", Cyclomatic: 9}})
	deltas := 0
	for _, m := range r.result.Measurements {
		if strings.HasPrefix(m.Metric, "change.delta.") {
			deltas++
			if m.Location.File != "same.py" {
				t.Fatal(m)
			}
		}
		if m.Level != "change" {
			t.Fatal(m)
		}
	}
	if deltas != 2 {
		t.Fatal(deltas)
	}
	if len(r.result.Findings) != 2 {
		t.Fatal(r.result.Findings)
	}
	for _, f := range r.result.Findings {
		if f.Status != StatusAdvisory || !strings.Contains(f.Evidence, "abc") {
			t.Fatal(f)
		}
	}
}

func TestBaselineListingAndAggregateBudgets(t *testing.T) {
	if _, err := parseBaselineTree(make([]byte, baselineListingLimit+1), ""); err == nil {
		t.Fatal("oversize listing accepted")
	}
	var listing strings.Builder
	for i := 0; i < baselineFileLimit; i++ {
		fmt.Fprintf(&listing, "100644 blob abc\tsrc/f%d.py\x00", i)
	}
	if blobs, err := parseBaselineTree([]byte(listing.String()), ""); err != nil || len(blobs) != baselineFileLimit {
		t.Fatal(len(blobs), err)
	}
	listing.WriteString("100644 blob abc\tsrc/overflow.py\x00")
	if _, err := parseBaselineTree([]byte(listing.String()), ""); err == nil {
		t.Fatal("too many selected files accepted")
	}
	budget := baselineBudget{}
	for i := 0; i < baselineTotalLimit/baselineBlobLimit; i++ {
		if err := budget.add(baselineBlobLimit); err != nil {
			t.Fatal(err)
		}
	}
	if budget.total != baselineTotalLimit {
		t.Fatal(budget)
	}
	if err := budget.add(1); err == nil {
		t.Fatal("total byte budget exceeded")
	}
	if err := (&baselineBudget{}).add(baselineBlobLimit + 1); err == nil {
		t.Fatal("per-file budget exceeded")
	}
	if err := (&baselineBudget{}).add(-1); err == nil {
		t.Fatal("negative blob size accepted")
	}
}

func changeTestProfile(t *testing.T) changeProfile {
	t.Helper()
	report := structureReport{SchemaVersion: "1", Modules: []structureModule{{Name: "a", File: "src/a.py"}}, Imports: []structureImport{}, Components: []structureComponent{{Modules: []string{"a"}}}, Counts: structureCounts{Modules: 1}, Functions: []structureFunction{}, Calls: []structureCall{}, FunctionComponents: []structureFunctionComponent{}}
	data, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	return changeProfile{Structure: data, Versions: map[string]string{"python": "3.12", "grimp": "3.18", "networkx": "3.7", "pyan3": "2.9.0", "radon": "6.0.1", "complexipy": "8.3.0"}, Functions: []changeFunctionMetric{}, Files: []changeFileMetric{{File: "src/a.py", SourceHash: "hash", MI: 100}}}
}

func TestChangeNormalizationRejectsIncompleteEvidence(t *testing.T) {
	for _, tc := range []struct {
		name, rule string
		modify     func(*changeProfile)
	}{
		{"complete", "", func(p *changeProfile) {}},
		{"unparsable", "INCOMPLETE_INPUT", func(p *changeProfile) {
			var s structureReport
			json.Unmarshal(p.Structure, &s)
			s.Partial = true
			s.Limitation = "Unparsable source: src/a.py"
			p.Structure, _ = json.Marshal(s)
		}},
		{"unresolved local import", "INCOMPLETE_INPUT", func(p *changeProfile) {
			var s structureReport
			json.Unmarshal(p.Structure, &s)
			s.Partial = true
			s.Limitation = "Unresolved local import: pkg.missing"
			p.Structure, _ = json.Marshal(s)
		}},
		{"tool version mismatch", "INCOMPLETE_INPUT", func(p *changeProfile) { p.Versions["grimp"] = "other" }},
		{"missing version evidence", "INCOMPLETE_INPUT", func(p *changeProfile) { delete(p.Versions, "pyan3") }},
		{"radon version mismatch", "INCOMPLETE_INPUT", func(p *changeProfile) { p.Versions["radon"] = "other" }},
		{"radon failure", "INCOMPLETE_METRICS", func(p *changeProfile) { p.Files = nil; p.MetricsError = "radon missing"; delete(p.Versions, "radon") }},
		{"missing file", "INCOMPLETE_METRICS", func(p *changeProfile) { p.Files = []changeFileMetric{} }},
		{"duplicate file", "INCOMPLETE_METRICS", func(p *changeProfile) { p.Files = append(p.Files, p.Files[0]) }},
		{"bad metric", "INCOMPLETE_METRICS", func(p *changeProfile) { p.Files[0].MI = 101 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b, a := changeTestProfile(t), changeTestProfile(t)
			tc.modify(&b)
			payload, err := json.Marshal(map[string]any{"baseline": b, "candidate": a})
			if err != nil {
				t.Fatal(err)
			}
			r := &Runner{root: t.TempDir(), result: newRunResult("change"), outputMode: "json", baselineSHA: "abc"}
			complete := r.normalizeChange(bytes.NewReader(payload), []string{"diagnostic-001"})
			if complete != (tc.rule == "") {
				t.Fatal(complete, r.result.Findings)
			}
			if tc.rule != "" {
				if len(r.result.Measurements) != 0 || len(r.result.Findings) != 1 || r.result.Findings[0].Rule != "pyguard.change."+tc.rule || !strings.Contains(r.result.Findings[0].Evidence, "abc") {
					t.Fatal(r.result)
				}
			}
		})
	}
}

func TestOptionalCheckComparisonCannotBecomeBlocking(t *testing.T) {
	t.Setenv("PATH", t.TempDir()) // git is unavailable: focused change execution error.
	for _, blocking := range []bool{false, true} {
		r := &Runner{root: t.TempDir(), baseline: "HEAD", result: newRunResult("check"), outputMode: "json"}
		r.result.RecordGate("ruff", true, true)
		code := 0
		if blocking {
			r.result.AddFinding(Finding{Gate: "ruff", Rule: "ruff.blocked", Level: "code", Status: StatusBlocking, Severity: "error", Category: "quality", Evidence: "existing failure"})
			r.result.RecordGate("ruff", false, true)
			code = 1
		}
		runChangeAdvisory(r)
		if err := r.result.Finish(code); err != nil {
			t.Fatal(err)
		}
		want := StatusAdvisory
		if blocking {
			want = StatusFail
		}
		if r.result.Status != want || r.result.ExitCode != code || r.result.Assessment != Incomplete {
			t.Fatal(r.result)
		}
		for _, f := range r.result.Findings {
			if f.Gate == "change" && (f.Status != StatusAdvisory || f.Category != "quality") {
				t.Fatal(f)
			}
		}
	}
}

func TestChangeMalformedStubsMakeEitherSideIncomplete(t *testing.T) {
	for _, side := range []string{"baseline", "candidate"} {
		t.Run(side, func(t *testing.T) {
			profiles := map[string]changeProfile{"baseline": changeTestProfile(t), "candidate": changeTestProfile(t)}
			profile := profiles[side]
			profile.InputError = "StructureError: Unparsable source: src/broken.pyi: invalid syntax"
			profiles[side] = profile
			payload, err := json.Marshal(profiles)
			if err != nil {
				t.Fatal(err)
			}
			r := &Runner{root: t.TempDir(), result: newRunResult("change"), outputMode: "json", baselineSHA: "abc"}
			if r.normalizeChange(bytes.NewReader(payload), nil) {
				t.Fatal("malformed stub accepted as complete")
			}
			if len(r.result.Measurements) != 0 || len(r.result.Findings) != 1 || r.result.Findings[0].Rule != "pyguard.change.INCOMPLETE_INPUT" || !strings.Contains(r.result.Findings[0].Evidence, "broken.pyi") {
				t.Fatal(r.result)
			}
		})
	}
}
