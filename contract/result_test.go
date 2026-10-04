package contract

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestBothLanguagesUseSameShapeAndBounds(t *testing.T) {
	for _, language := range []string{"tsguard", "pyguard"} {
		t.Run(language, func(t *testing.T) {
			a, b := New(language, "check"), New(language, "check")
			for i := 0; i < 1000; i++ {
				f := Finding{Gate: "types", Rule: "type.error", Severity: "error", Status: "blocking", Category: "quality", Location: &Location{File: strings.Repeat("file", 200), Line: i + 1}, Evidence: strings.Repeat("é\n\x1b", 1000)}
				a.AddFinding(f)
				f.Evidence = "Changed analyzer prose"
				b.AddFinding(f)
			}
			a.Finish(1)
			b.Finish(1)
			for i := range a.Findings {
				if a.Findings[i].ID != b.Findings[i].ID {
					t.Fatal("unstable ID")
				}
			}
			var agent, jsonOut bytes.Buffer
			if err := Report(&agent, "agent", a); err != nil {
				t.Fatal(err)
			}
			if agent.Len() > 6144 || strings.Count(agent.String(), "\n") > 26 || !strings.Contains(agent.String(), "omitted: 990 (use --output json)") || strings.Contains(agent.String(), "\x1b") {
				t.Fatal("unbounded/unsafe summary")
			}
			Report(&jsonOut, "json", a)
			var decoded RunResult
			if err := json.Unmarshal(jsonOut.Bytes(), &decoded); err != nil || len(decoded.Findings) != 1000 {
				t.Fatal("incomplete JSON", err)
			}
			clean := New(language, "check")
			clean.Execution("invocation", "tool_missing", "missing")
			clean.Finish(0)
			if clean.Status != "error" || clean.Findings[0].Rule != language+".execution.tool_missing" {
				t.Fatal(clean)
			}
		})
	}
}
