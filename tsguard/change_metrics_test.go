package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestIdentityMatchingSurvivesMovesButRejectsAmbiguousCopies(t *testing.T) {
	before := map[string][2]string{"original.ts": {"original.ts", "exact"}, "same.ts": {"same.ts", "old"}, "ambiguous.ts": {"ambiguous.ts", "copy"}}
	after := map[string][2]string{"renamed.ts": {"renamed.ts", "exact"}, "same.ts": {"same.ts", "changed"}, "copy1.ts": {"copy1.ts", "copy"}, "copy2.ts": {"copy2.ts", "copy"}}
	got := matchIdentities(before, after)
	if len(got) != 2 || got[0].Baseline != "original.ts" || got[0].Candidate != "renamed.ts" || got[0].Method != "exact_fingerprint" || got[1].Method != "path_or_symbol" {
		t.Fatal(got)
	}
}

func metricProfile(score float64) nativeCodeProfile {
	upper := score
	p := nativeCodeProfile{Files: []nativeFileMetric{{File: "a.ts", SourceHash: "hash", FTA: 20, Cyclo: 3, Halstead: 30, Lines: 7}}, Functions: []nativeFunctionMetric{{ID: "a:1", Location: Location{File: "a.ts", Line: 1, Symbol: "a"}, Fingerprint: "tokens", CognitiveMin: score, CognitiveMax: &upper}}, CognitiveComplete: true}
	p.Duplicates.Groups = []cloneGroup{}
	return p
}

func TestNativeComparisonEmitsComplexityAndNewCloneEvidence(t *testing.T) {
	r := contractRunner(t)
	b, a := metricProfile(2), metricProfile(5)
	a.Duplicates.Clones, a.Duplicates.Tokens, a.Duplicates.Lines, a.Duplicates.Percentage = 1, 60, 7, 50
	a.Duplicates.Groups = []cloneGroup{{Fingerprint: "clone-fingerprint", Count: 1, Locations: []Location{{File: "a.ts", Line: 1}, {File: "a.ts", Line: 8}}}}
	r.comparison = &nativeComparison{Baseline: b, Candidate: a, Policy: map[string]any{"native": true}}
	s := smellSnapshot()
	if err := r.compareNativeMetrics(s, s, "sha"); err != nil {
		t.Fatal(err)
	}
	if len(r.result.Findings) != 2 {
		t.Fatal(r.result.Findings)
	}
	for _, f := range r.result.Findings {
		if f.Status != "advisory" || f.Level != "change" {
			t.Fatal(f)
		}
	}
	data, err := os.ReadFile(filepath.Join(r.root, opengrepCacheDir, "maintainability-change.json"))
	if err != nil {
		t.Fatal(err)
	}
	var artifact map[string]any
	if err := json.Unmarshal(data, &artifact); err != nil || artifact["baseline_sha"] != "sha" {
		t.Fatal(err, artifact)
	}
	// Same clone group after a file move is retained, not reported as new.
	r.result.Findings = nil
	r.comparison.Baseline = a
	if err := r.compareNativeMetrics(s, s, "sha"); err != nil || len(r.result.Findings) != 0 {
		t.Fatal(err, r.result.Findings)
	}
}

func TestNativeComparisonNeverInventsSuppressedOrMissingMetrics(t *testing.T) {
	c := &nativeComparison{Baseline: metricProfile(2), Candidate: metricProfile(2), Policy: map[string]any{"native": true}}
	c.Candidate.Functions[0].CognitiveMax = nil
	if c.validate() == nil {
		t.Fatal("complete cognitive assessment accepted an unknown bound")
	}
	c.Candidate.CognitiveComplete = false
	if err := c.validate(); err != nil {
		t.Fatal(err)
	}
	r := contractRunner(t)
	r.comparison = c
	if err := r.compareNativeMetrics(smellSnapshot(), smellSnapshot(), "sha"); err != nil {
		t.Fatal(err)
	}
	for _, m := range r.result.Measurements {
		if m.Metric == "change.candidate.total_cognitive_max" {
			t.Fatal("invented upper bound")
		}
	}
	c.Candidate.Duplicates.Clones = 1
	if c.validate() == nil {
		t.Fatal("missing clone identity accepted")
	}
}
