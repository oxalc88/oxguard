package main

import "testing"

func smellSnapshot(targets ...string) *maintainabilitySnapshot {
	s := &maintainabilitySnapshot{Functions: []functionFact{}, Calls: []callEdge{}, Handlers: []handlerFact{}}
	for i, target := range targets {
		s.Functions = append(s.Functions, functionFact{ID: string(rune('a' + i)), ForwardTarget: target, Location: Location{File: "src.ts", Line: i + 1, Symbol: string(rune('a' + i))}})
	}
	return s
}

func TestDelegationEvidenceAndFalsePositiveControls(t *testing.T) {
	for _, tc := range []struct {
		name     string
		targets  []string
		findings int
	}{
		{"chain", []string{"b", "c", "d", ""}, 1},
		{"one boundary", []string{"b", ""}, 0},
		{"cycle", []string{"b", "a"}, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := contractRunner(t)
			s := smellSnapshot(tc.targets...)
			if err := s.validate(); err != nil {
				t.Fatal(err)
			}
			r.smellFindings("smells", s, nil)
			if len(r.result.Findings) != tc.findings {
				t.Fatalf("%+v", r.result.Findings)
			}
			for _, f := range r.result.Findings {
				if f.Status != "advisory" || len(f.Related) < 3 {
					t.Fatal(f)
				}
			}
		})
	}
}

func TestRepeatedHandlersRequireSameFailurePolicy(t *testing.T) {
	r := contractRunner(t)
	s := smellSnapshot()
	for i, fingerprint := range []string{"throw", "return", "throw", "return"} {
		s.Handlers = append(s.Handlers, handlerFact{Location: Location{File: "source.ts", Line: i + 1}, Tokens: 20, Fingerprint: fingerprint})
	}
	r.smellFindings("smells", s, nil)
	if len(r.result.Findings) != 0 {
		t.Fatal(r.result.Findings)
	}
	s.Handlers = append(s.Handlers, s.Handlers[0])
	r.smellFindings("smells", s, nil)
	if len(r.result.Findings) != 1 || len(r.result.Findings[0].Related) != 2 {
		t.Fatal(r.result.Findings)
	}
}
