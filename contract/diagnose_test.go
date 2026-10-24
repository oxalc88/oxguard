package contract

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestDiagnosticCollectsIndependentFindingsAndCoverage(t *testing.T) {
	r := New("pyguard", "diagnose")
	r.PlanAnalyses(Analysis{Name: "radon.cc", Gate: "radon"}, Analysis{Name: "radon.mi", Gate: "radon"})
	for i, rule := range []string{"pyguard.radon.cc_exceeded", "pyguard.radon.mi_threshold_failed"} {
		r.CollectAnalysis(i, func(result *RunResult) int {
			result.AddFinding(Finding{Gate: "radon", Rule: rule, Severity: "error", Status: StatusBlocking, Category: "quality", Evidence: "Policy violation"})
			result.RecordGate("radon", true, true)
			return 0
		})
	}
	if err := r.Finish(1); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	var decoded RunResult
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Status != StatusFail || decoded.Assessment != Complete || len(decoded.Findings) != 2 || len(decoded.Analyses) != 2 {
		t.Fatalf("incomplete inventory: %s", data)
	}
	for _, a := range decoded.Analyses {
		if a.Status != StatusFailed || a.Normalization != Complete {
			t.Fatalf("quality failure is a completed analysis: %+v", a)
		}
	}
}

func TestDiagnosticMissingAnalysisCannotClaimCompletion(t *testing.T) {
	r := New("pyguard", "diagnose")
	r.PlanAnalyses(Analysis{Name: "radon.cc", Gate: "radon"}, Analysis{Name: "radon.mi", Gate: "radon"})
	r.CollectAnalysis(0, func(result *RunResult) int {
		result.RecordGate("radon", true, true)
		return 0
	})
	if err := r.Finish(1); err != nil {
		t.Fatal(err)
	}
	if r.Assessment != Incomplete || r.Analyses[1].Status != StatusNotRun || r.Analyses[1].Reason == "" {
		t.Fatalf("unrun MI hidden: %+v", r)
	}
	r.Assessment = Complete
	if err := r.Validate(); err == nil {
		t.Fatal("forged complete assessment accepted")
	}
}

func TestDiagnosticErrorRetainsFindingsAndLaterAnalysis(t *testing.T) {
	r := New("tsguard", "diagnose")
	r.PlanAnalyses(Analysis{Name: "lint", Gate: "lint"}, Analysis{Name: "fta", Gate: "fta"}, Analysis{Name: "types", Gate: "types"})
	r.CollectAnalysis(0, func(result *RunResult) int {
		result.Execution("lint", "timeout", "Lint exceeded its timeout.")
		return 1
	})
	r.CollectAnalysis(1, func(result *RunResult) int {
		result.AddFinding(Finding{Gate: "fta", Rule: "tsguard.fta.score_exceeded", Severity: "error", Status: StatusBlocking, Category: "quality", Evidence: "High score"})
		result.RecordGate("fta", true, true)
		return 0
	})
	r.CollectAnalysis(2, func(result *RunResult) int {
		result.RecordGate("types", true, true)
		return 0
	})
	if err := r.Finish(1); err != nil {
		t.Fatal(err)
	}
	if r.Status != StatusError || r.Assessment != Incomplete || len(r.Findings) != 2 || r.Analyses[2].Status != StatusPassed || r.Analyses[0].Reason != "Lint exceeded its timeout." {
		t.Fatalf("lost independent evidence: %+v", r)
	}
	var output bytes.Buffer
	if err := Report(&output, "agent", r); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "incomplete_analyses: 1") || !strings.Contains(output.String(), "--output json") {
		t.Fatalf("bounded output hides incomplete analyses: %s", output.String())
	}
}

func TestDiagnosticUnavailableDependencyExplainsNotRun(t *testing.T) {
	r := New("tsguard", "diagnose")
	r.PlanAnalyses(Analysis{Name: "change", Gate: "change"})
	r.CollectAnalysis(0, func(result *RunResult) int {
		result.AddFinding(Finding{Gate: "change", Rule: "tsguard.change.BASELINE_UNAVAILABLE", Severity: "info", Status: StatusAdvisory, Category: "quality", Evidence: "No explicit Git baseline; change quality was not evaluated."})
		return 0
	})
	if err := r.Finish(1); err != nil {
		t.Fatal(err)
	}
	if r.Analyses[0].Status != StatusNotRun || !strings.Contains(r.Analyses[0].Reason, "Git baseline") || r.Assessment != Incomplete {
		t.Fatalf("missing dependency reason lost: %+v", r.Analyses[0])
	}
}
