package contract

// Gate describes requested execution separately from normalized finding detail.
// A failed quality gate can be complete; an execution error cannot complete an assessment.
type Gate struct {
	Name          string       `json:"name"`
	Status        Status       `json:"status"`        // not_run, passed, advisory, failed, error
	Normalization Completeness `json:"normalization"` // not_run, complete, partial
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
			r.Gates = append(r.Gates, Gate{Name: name, Status: StatusNotRun, Normalization: NotRun})
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
		state := StatusPassed
		blocking := false
		if !success {
			state = StatusFailed
		}
		for _, f := range r.Findings {
			if f.Gate != name {
				continue
			}
			if f.Status == StatusExecutionError {
				state = StatusError
				break
			}
			if f.Status == StatusAdvisory && state != StatusError {
				if !blocking {
					state = StatusAdvisory
				}
			}
			if f.Status == StatusBlocking {
				state = StatusFailed
				blocking = true
			}
		}
		priorities := map[Status]int{StatusNotRun: 0, StatusPassed: 1, StatusAdvisory: 2, StatusFailed: 3, StatusError: 4}
		if priorities[state] > priorities[g.Status] {
			g.Status = state
		}
		if !complete || g.Normalization == Partial {
			g.Normalization = Partial
		} else {
			g.Normalization = Complete
		}
		return
	}
}
