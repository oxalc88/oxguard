package contract

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestGateAssessment(t *testing.T) {
	r := New("test", "check")
	r.Plan("lint", "types", "coverage")
	r.AddFinding(Finding{Gate: "types", Rule: "TS2322", Status: "blocking", Category: "quality"})
	r.RecordGate("lint", true, true)
	r.RecordGate("types", false, true)
	r.Finish(1)
	if r.Assessment != "incomplete" || r.Gates[2].Status != "not_run" || r.Gates[1].Normalization != "complete" {
		t.Fatalf("bad fail-fast assessment: %+v", r)
	}
	r.RecordGate("coverage", true, false)
	r.Finish(1)
	if r.Assessment != "incomplete" {
		t.Fatal("partial result certified complete")
	}
	r = New("test", "audit")
	r.Plan("unused")
	r.AddFinding(Finding{Gate: "unused", Rule: "unused.function", Status: "advisory", Category: "quality"})
	r.RecordGate("unused", false, true)
	r.RecordGate("unused", true, true)
	r.Finish(0)
	if r.Status != "advisory" || r.Assessment != "complete" || r.Gates[0].Status != "advisory" {
		t.Fatalf("advisory confused with blocking: %+v", r)
	}
	r.Execution("unused", "timeout", "Timed out")
	r.Finish(0)
	if r.Status != "error" || r.Assessment != "incomplete" || r.ExitCode != 0 {
		t.Fatal("execution failure hidden by advisory exit")
	}
}
func TestCoverageNativeSummary(t *testing.T) {
	r := New("tsguard", "coverage")
	err := r.IstanbulSummary(strings.NewReader(`{"total":{"lines":{"pct":79.9},"functions":{"pct":80},"branches":{"pct":100},"statements":{"pct":100},"branchesTrue":{"pct":"Unknown"}}}`), "", nil)
	if err != nil || len(r.Measurements) != 4 || len(r.Findings) != 1 || r.Findings[0].Rule != "tsguard.coverage.lines_threshold_failed" {
		t.Fatalf("native summary: %v %+v", err, r)
	}
	for _, report := range []string{`{}`, `{"total":{"lines":{"pct":"Unknown"}}}`, `{"total":{"lines":{"pct":101}}}`} {
		if New("tsguard", "coverage").IstanbulSummary(strings.NewReader(report), "", nil) == nil {
			t.Fatal("invalid report accepted")
		}
	}
}
func TestGateReporterBounds(t *testing.T) {
	r := New("test", "check")
	for i := 0; i < 300; i++ {
		name := strings.Repeat("a", 300)
		r.Plan(name)
		r.AddFinding(Finding{Gate: name, Rule: name, Status: "blocking", Evidence: strings.Repeat("x\n", 1000)})
	}
	r.Finish(1)
	var b bytes.Buffer
	if err := Report(&b, "agent", r); err != nil {
		t.Fatal(err)
	}
	if b.Len() > 6144 || strings.Count(b.String(), "\n") > 26 || !strings.Contains(b.String(), "omitted: 290") {
		t.Fatal("agent bound/omissions violated")
	}
	b.Reset()
	Report(&b, "json", r)
	var decoded RunResult
	if json.Unmarshal(b.Bytes(), &decoded) != nil || len(decoded.Findings) != 300 || len(decoded.Gates) != 1 {
		t.Fatal("JSON dropped normalized records")
	}
}
func TestFindingIdentityIncludesSymbolAndRelated(t *testing.T) {
	r := New("test", "audit")
	for _, name := range []string{"a", "b"} {
		r.AddFinding(Finding{Gate: "deps", Rule: "GHSA-example", Location: &Location{File: "package.json", Symbol: name}})
	}
	if r.Findings[0].ID == r.Findings[1].ID {
		t.Fatal("packages collide")
	}
	for _, line := range []int{2, 3} {
		r.AddFinding(Finding{Gate: "duplicates", Rule: "jscpd.duplicate", Location: &Location{File: "a.ts", Line: 1}, Related: []Location{{File: "b.ts", Line: line}}})
	}
	if r.Findings[2].ID == r.Findings[3].ID {
		t.Fatal("duplicate pairs collide")
	}
}
