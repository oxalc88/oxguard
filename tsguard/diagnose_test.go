package main

import (
	"strings"
	"testing"
)

func TestDiagnosticFTAReportsAllFilesAtEffectivePolicyCap(t *testing.T) {
	r := contractRunner(t)
	r.normalize(toolSpec{gate: "fta", adapter: "fta-collection", subject: "src", threshold: 1}, Result{ok: true}, strings.NewReader(`[{"file_name":"b.ts","fta_score":24.240152481108808},{"file_name":"a.ts","fta_score":24.38161685613889}]`), nil)
	if err := r.result.Finish(1); err != nil {
		t.Fatal(err)
	}
	if r.result.Assessment != Complete || len(r.result.Findings) != 2 || len(r.result.Measurements) != 2 {
		t.Fatalf("missing complete measurements: %+v", r.result)
	}
	for i, file := range []string{"src/a.ts", "src/b.ts"} {
		f := r.result.Findings[i]
		if f.Location.File != file || f.Rule != "tsguard.fta.score_exceeded" || *f.Threshold != 1 {
			t.Fatalf("wrong effective policy: %+v", f)
		}
	}
}

func TestDiagnosticFTARejectsIncompleteCollection(t *testing.T) {
	for _, tc := range []struct {
		name, output string
		ok           bool
	}{
		{"cap_exit", "File b.ts has a score of 1000001, which is beyond the score cap of 1000000, exiting.", false},
		{"malformed", "[{", true},
		{"null", "null", true},
		{"missing_score", `[{"file_name":"b.ts"}]`, true},
		{"missing_file", `[{"fta_score":24}]`, true},
		{"trailing_output", `[] []`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := contractRunner(t)
			r.normalize(toolSpec{gate: "fta", adapter: "fta-collection", subject: "src", threshold: 1}, Result{ok: tc.ok, output: tc.output}, strings.NewReader(tc.output), nil)
			if err := r.result.Finish(1); err != nil {
				t.Fatal(err)
			}
			if r.result.Assessment != Incomplete || r.result.Status != StatusError {
				t.Fatalf("incomplete collection claimed success: %+v", r.result)
			}
			for _, f := range r.result.Findings {
				if f.Rule == "tsguard.fta.score_exceeded" {
					t.Fatal("collection ceiling was mistaken for policy")
				}
			}
		})
	}
}
