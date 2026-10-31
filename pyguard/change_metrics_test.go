package main

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestChangeIdentityMatching(t *testing.T) {
	before := map[string][2]string{"same": {"a:f", "old"}, "move": {"b:g", "exact"}, "copy1": {"c:h", "copy"}, "copy2": {"d:h", "copy"}}
	after := map[string][2]string{"same2": {"a:f", "exact"}, "move2": {"z:g", "exact"}, "copy3": {"e:h", "copy"}}
	matches := matchIdentities(before, after)
	if len(matches) != 2 || matches[0] != (identityMatch{"move", "move2", "exact_fingerprint"}) || matches[1].Method != "path_or_symbol" {
		t.Fatal(matches)
	}
	// Duplicate symbols cannot use position to break the tie.
	if got := matchIdentities(map[string][2]string{"a": {"f", "x"}, "b": {"f", "x"}}, map[string][2]string{"c": {"f", "x"}}); len(got) != 0 {
		t.Fatal(got)
	}
}

func TestFunctionChangeAndArtifact(t *testing.T) {
	for _, tc := range []struct {
		name          string
		old, current  float64
		missing, move bool
		want          int
	}{
		{"increase", 1, 3, false, false, 1}, {"decrease", 3, 1, false, false, 0}, {"move", 1, 1, false, true, 0}, {"suppressed", 1, 0, true, false, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b, a := changeTestProfile(t), changeTestProfile(t)
			old, current := tc.old, tc.current
			f := changeFunctionMetric{ID: "old", Location: Location{File: "src/a.py", Line: 2, Symbol: "f"}, Fingerprint: "tokens", Cyclomatic: 1, Cognitive: &old}
			b.Functions = []changeFunctionMetric{f}
			b.Files[0].Cyclomatic = 1
			a.Files[0].Cyclomatic = 1
			f.ID = "new"
			f.Cognitive = &current
			if tc.missing {
				f.Cognitive = nil
				f.CognitiveError = "complexipy suppression"
			}
			if tc.move {
				f.Location.File = "src/moved.py"
				a.Files[0].File = f.Location.File
			}
			a.Functions = []changeFunctionMetric{f}
			r := &Runner{root: t.TempDir(), result: newRunResult("change"), baselineSHA: "abc"}
			complete := r.compareFunctionMetrics(b, a)
			r.result.RecordGate("change", true, complete)
			if complete == tc.missing || changeExitCode(r, 0) != map[bool]int{true: 1, false: 0}[tc.missing] {
				t.Fatal(complete, r.result)
			}
			increases := 0
			for _, finding := range r.result.Findings {
				if finding.Rule == "pyguard.change.FUNCTION_COMPLEXITY_INCREASE" {
					increases++
					if finding.Status != StatusAdvisory || finding.Severity != "warning" || finding.Location.Symbol != "f" || len(finding.Related) != 1 || !strings.Contains(finding.Evidence, "abc") || !strings.Contains(finding.Evidence, "path_or_symbol") {
						t.Fatal(finding)
					}
				}
				if tc.missing && (!strings.Contains(finding.Evidence, "f") || !strings.Contains(finding.Evidence, "src/a.py")) {
					t.Fatal(finding)
				}
			}
			if increases != tc.want {
				t.Fatal(r.result.Findings)
			}
			if len(r.result.Artifacts) != 1 || r.result.Artifacts[0].Kind != "maintainability_change" {
				t.Fatal(r.result.Artifacts)
			}
			data, err := os.ReadFile(filepath.Join(r.root, r.result.Artifacts[0].Path))
			if err != nil {
				t.Fatal(err)
			}
			var artifact map[string]json.RawMessage
			if err = json.Unmarshal(data, &artifact); err != nil {
				t.Fatal(err)
			}
			for _, key := range []string{"schema_version", "baseline_sha", "native_metrics", "file_matches", "function_matches", "baseline_structure", "candidate_structure"} {
				if artifact[key] == nil {
					t.Fatal(key)
				}
			}
			var functions []identityMatch
			json.Unmarshal(artifact["function_matches"], &functions)
			if len(functions) != 1 || tc.move && functions[0].Method != "exact_fingerprint" {
				t.Fatal(functions)
			}
			for _, m := range r.result.Measurements {
				if tc.missing && (m.Metric == "change.delta.function.cognitive" || m.Metric == "change.candidate.total_cognitive") {
					t.Fatal(m)
				}
			}
		})
	}
}

func TestFunctionMetricsRejectInvalidEvidence(t *testing.T) {
	score := 1.0
	for _, modify := range []func(*changeProfile){
		func(p *changeProfile) { p.Functions = nil }, func(p *changeProfile) { p.Files[0].SourceHash = "" },
		func(p *changeProfile) { p.Functions[0].Cognitive = func() *float64 { n := math.Inf(1); return &n }() },
		func(p *changeProfile) { p.Functions[0].Cyclomatic = -1 }, func(p *changeProfile) { p.Functions[0].Location.Line = 0 },
		func(p *changeProfile) { p.Functions = append(p.Functions, p.Functions[0]) },
	} {
		p := changeTestProfile(t)
		p.Functions = []changeFunctionMetric{{ID: "f", Location: Location{File: "src/a.py", Line: 1, Symbol: "f"}, Fingerprint: "tokens", Cyclomatic: 1, Cognitive: &score}}
		p.Files[0].Cyclomatic = 1
		modify(&p)
		if validateFunctionMetrics(p) == nil {
			t.Fatal(p)
		}
	}
}

func TestCognitiveComparisonLimitsAndArtifactFailure(t *testing.T) {
	for _, mode := range []string{"baseline suppression", "version mismatch", "unmatched suppression", "artifact failure"} {
		t.Run(mode, func(t *testing.T) {
			b, a := changeTestProfile(t), changeTestProfile(t)
			old, current := 1.0, 3.0
			b.Files[0].Cyclomatic = 1
			a.Files[0].Cyclomatic = 1
			b.Functions = []changeFunctionMetric{{ID: "b", Location: Location{File: "src/a.py", Line: 1, Symbol: "f"}, Fingerprint: "old", Cyclomatic: 1, Cognitive: &old}}
			a.Functions = []changeFunctionMetric{{ID: "a", Location: Location{File: "src/a.py", Line: 1, Symbol: "f"}, Fingerprint: "new", Cyclomatic: 1, Cognitive: &current}}
			root := t.TempDir()
			switch mode {
			case "baseline suppression":
				b.Functions[0].Cognitive = nil
			case "version mismatch":
				a.Versions["complexipy"] = "other"
			case "unmatched suppression":
				b.Functions[0].Cognitive = nil
				a.Functions[0].Location.Symbol = "different"
			case "artifact failure":
				if err := os.WriteFile(filepath.Join(root, ".pyguard-cache"), []byte("file"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			r := &Runner{root: root, result: newRunResult("change"), baselineSHA: "abc"}
			if r.compareFunctionMetrics(b, a) {
				t.Fatal("incomplete comparison passed")
			}
			if mode != "artifact failure" {
				for _, f := range r.result.Findings {
					if f.Rule == "pyguard.change.FUNCTION_COMPLEXITY_INCREASE" {
						t.Fatal(f)
					}
				}
				for _, m := range r.result.Measurements {
					if m.Metric == "change.delta.function.cognitive" {
						t.Fatal(m)
					}
				}
				if len(r.result.Artifacts) != 1 {
					t.Fatal("partial cognitive evidence lost", r.result.Artifacts)
				}
			} else if len(r.result.Artifacts) != 0 {
				t.Fatal("failed artifact registered", r.result.Artifacts)
			}
		})
	}
}
