package contract

// Analysis records execution coverage independently of its parent quality gate.
type Analysis struct {
	Name          string       `json:"name"`
	Gate          string       `json:"gate"`
	Status        Status       `json:"status"`
	Normalization Completeness `json:"normalization"`
	Reason        string       `json:"reason,omitempty"`
	Diagnostics   []string     `json:"diagnostics"`
}

func (r *RunResult) PlanAnalyses(analyses ...Analysis) {
	for _, a := range analyses {
		a.Status, a.Normalization = StatusNotRun, NotRun
		a.Reason = "Analysis has not run."
		a.Diagnostics = []string{}
		r.Analyses = append(r.Analyses, a)
		r.Plan(a.Gate)
	}
}

// CollectAnalysis isolates normalization so an earlier blocker in the same gate
// cannot affect a later metric, while diagnostic IDs stay unique across tools.
func (r *RunResult) CollectAnalysis(index int, run func(*RunResult) int) int {
	a := &r.Analyses[index]
	child := New(r.namespace, r.Command)
	child.Plan(a.Gate)
	child.Diagnostics = append([]Diagnostic(nil), r.Diagnostics...)
	base := len(child.Diagnostics)
	code := run(child)
	err := child.Finish(code)
	r.Diagnostics = child.Diagnostics
	for _, d := range child.Diagnostics[base:] {
		a.Diagnostics = append(a.Diagnostics, d.ID)
	}
	if err != nil {
		a.Status, a.Normalization, a.Reason = StatusError, Partial, "Analysis produced an invalid result: "+err.Error()
		r.Execution(a.Gate, "adapter_failure", a.Reason)
		return 1
	}
	r.Findings = append(r.Findings, child.Findings...)
	r.Measurements = append(r.Measurements, child.Measurements...)
	r.Artifacts = append(r.Artifacts, child.Artifacts...)
	executed := false
	for _, g := range child.Gates {
		if g.Status != StatusNotRun {
			executed = true
			r.RecordGate(g.Name, g.Status != StatusFailed && g.Status != StatusError, g.Normalization == Complete)
		}
	}
	if !executed {
		for _, f := range child.Findings {
			if f.Gate == a.Gate {
				a.Reason = f.Evidence
				break
			}
		}
		return 1
	}
	a.Status = map[Status]Status{StatusPass: StatusPassed, StatusFail: StatusFailed, StatusError: StatusError, StatusAdvisory: StatusAdvisory}[child.Status]
	a.Normalization, a.Reason = Complete, ""
	if child.Assessment != Complete {
		a.Normalization = Partial
		a.Reason = "Analysis did not produce complete normalized results; inspect diagnostics."
		for _, f := range child.Findings {
			if f.Status == StatusExecutionError {
				a.Reason = f.Evidence
				break
			}
		}
	}
	if code != 0 || child.Status == StatusFail || child.Status == StatusError || child.Assessment != Complete {
		return 1
	}
	return 0
}

func (r *RunResult) analysesComplete() bool {
	for _, a := range r.Analyses {
		if a.Status == StatusNotRun || a.Status == StatusError || a.Normalization != Complete {
			return false
		}
	}
	return true
}
