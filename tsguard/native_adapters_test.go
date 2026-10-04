package main

import (
	"fmt"
	"strings"
	"testing"
)

func TestDuplicateIdentityIgnoresPairDirection(t *testing.T) {
	first := `{"name":"src/a.ts","startLoc":{"line":1,"column":1}}`
	second := `{"name":"src/b.ts","startLoc":{"line":2,"column":1}}`
	var ids []string
	for _, pair := range [][2]string{{first, second}, {second, first}} {
		r := &Runner{root: t.TempDir(), outputMode: "json", result: newRunResult("duplicates")}
		report := fmt.Sprintf(`{"statistics":{"total":{"percentage":25}},"duplicates":[{"lines":10,"firstFile":%s,"secondFile":%s}]}`, pair[0], pair[1])
		r.normalize(toolSpec{gate: "duplicates", adapter: "jscpd", advisory: true}, Result{ok: false}, strings.NewReader(report), nil)
		if len(r.result.Findings) != 1 || r.result.Findings[0].Location.File != "src/a.ts" || r.result.Findings[0].Related[0].File != "src/b.ts" {
			t.Fatalf("noncanonical duplicate pair: %+v", r.result)
		}
		ids = append(ids, r.result.Findings[0].ID)
	}
	if ids[0] != ids[1] {
		t.Fatal("duplicate identity depends on analyzer pair direction")
	}
}

func TestNativeAdapters(t *testing.T) {
	cases := []struct {
		adapter, report, rule string
		advisory              bool
	}{
		{"secretlint", `[{"filePath":"src/a.ts","messages":[{"ruleId":"@secretlint/token","message":"must not expose secret","line":2,"column":3}]}]`, "@secretlint/token", false},
		{"knip", `{"issues":[{"file":"src/a.ts","exports":[{"name":"unused","line":1,"col":1}]}]}`, "knip.exports", true},
		{"jscpd", `{"statistics":{"total":{"percentage":25}},"duplicates":[{"lines":10,"firstFile":{"name":"src/a.ts","startLoc":{"line":1,"column":1}},"secondFile":{"name":"src/b.ts","startLoc":{"line":2,"column":1}}}]}`, "jscpd.duplicate", true},
	}
	for _, c := range cases {
		t.Run(c.adapter, func(t *testing.T) {
			r := &Runner{root: t.TempDir(), outputMode: "json", result: newRunResult(c.adapter)}
			r.normalize(toolSpec{gate: c.adapter, adapter: c.adapter, advisory: c.advisory}, Result{ok: false}, strings.NewReader(c.report), nil)
			r.result.Finish(1)
			if len(r.result.Findings) != 1 || r.result.Findings[0].Rule != c.rule || r.result.Assessment != "complete" {
				t.Fatalf("%+v", r.result)
			}
			if c.adapter == "secretlint" && strings.Contains(r.result.Findings[0].Evidence, "must not expose secret") {
				t.Fatal("secret evidence exposed")
			}
		})
	}
}
func TestDependencyPolicyPreserved(t *testing.T) {
	report := `{"auditReportVersion":2,"vulnerabilities":{"example":{"via":[{"name":"example","severity":"high","title":"issue","url":"https://github.com/advisories/GHSA-example"}]}},"metadata":{}}`
	for _, ok := range []bool{false, true} {
		r := &Runner{root: t.TempDir(), outputMode: "json", result: newRunResult("npm-audit")}
		r.normalize(toolSpec{gate: "dependencies", adapter: "audit-ci"}, Result{ok: ok}, strings.NewReader(report), nil)
		r.result.Finish(0)
		want := 0
		if !ok {
			want = 1
		}
		if len(r.result.Findings) != want || r.result.Assessment != "complete" {
			t.Fatal("native allowlist decision not preserved")
		}
	}
	r := &Runner{root: t.TempDir(), outputMode: "json", result: newRunResult("npm-audit")}
	r.normalize(toolSpec{gate: "dependencies", adapter: "dependency-audit", advisory: true}, Result{ok: false}, strings.NewReader(report), nil)
	r.result.Finish(0)
	if r.result.Status != "advisory" || r.result.Findings[0].Rule != "GHSA-example" || r.result.Gates[0].Status != "advisory" {
		t.Fatal("informational audit treated as blocking")
	}
}
