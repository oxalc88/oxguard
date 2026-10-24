package main

import "testing"

func TestRunToolWithNoArgs(t *testing.T) {
	for _, command := range []string{"types", "diagnose"} {
		t.Run(command, func(t *testing.T) {
			r := &Runner{result: newRunResult(command)}
			res := r.RunTool(toolSpec{gate: "types"}, "compiler")
			if res.ok || res.name != "compiler" || res.category != "startup_failure" || res.message == "" {
				t.Fatalf("unexpected result: %+v", res)
			}
			if len(r.result.Findings) != 1 || r.result.Findings[0].Category != "startup_failure" {
				t.Fatalf("execution failure was not recorded: %+v", r.result.Findings)
			}
		})
	}

	r := &Runner{}
	if res := r.RunTool(toolSpec{}, "compiler"); res.ok || res.message == "" {
		t.Fatalf("unexpected result without run result: %+v", res)
	}
}
