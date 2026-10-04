package contract

// Gate describes requested execution separately from normalized finding detail.
// A failed quality gate can be complete; an execution error cannot complete an assessment.
type Gate struct {
	Name          string `json:"name"`
	Status        string `json:"status"`        // not_run, passed, advisory, failed, error
	Normalization string `json:"normalization"` // not_run, complete, partial
}

func (r *RunResult) Plan(names ...string) {
	for _, name := range names {
		found := false
		for _, g := range r.Gates {
			if g.Name == name {
				found = true
				break
			}
		}
		if !found {
			r.Gates = append(r.Gates, Gate{Name: name, Status: "not_run", Normalization: "not_run"})
		}
	}
}

// RecordGate aggregates multiple tool invocations without erasing an earlier failure.
func (r *RunResult) RecordGate(name string, success, complete bool) {
	r.Plan(name)
	for i := range r.Gates {
		g := &r.Gates[i]
		if g.Name != name {
			continue
		}
		state := "passed"
		blocking := false
		if !success {
			state = "failed"
		}
		for _, f := range r.Findings {
			if f.Gate != name {
				continue
			}
			if f.Status == "execution_error" {
				state = "error"
				break
			}
			if f.Status == "advisory" && state != "error" {
				if !blocking {
					state = "advisory"
				}
			}
			if f.Status == "blocking" {
				state = "failed"
				blocking = true
			}
		}
		priorities := map[string]int{"not_run": 0, "passed": 1, "advisory": 2, "failed": 3, "error": 4}
		if priorities[state] > priorities[g.Status] {
			g.Status = state
		}
		if !complete || g.Normalization == "partial" {
			g.Normalization = "partial"
		} else {
			g.Normalization = "complete"
		}
		return
	}
}
