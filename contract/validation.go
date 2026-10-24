package contract

import (
	"encoding/json"
	"fmt"
)

func validFindingStatus(s Status) bool {
	return s == StatusBlocking || s == StatusAdvisory || s == StatusExecutionError
}
func (f *Finding) UnmarshalJSON(data []byte) error {
	type wire Finding
	var decoded wire
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	if !validFindingStatus(decoded.Status) {
		return fmt.Errorf("invalid finding status %s", decoded.Status)
	}
	*f = Finding(decoded)
	return nil
}
func (r *RunResult) validateStates() error {
	if r == nil {
		return fmt.Errorf("nil result")
	}
	if r.SchemaVersion != "1" {
		return fmt.Errorf("unsupported result schema")
	}
	switch r.Status {
	case StatusPass, StatusFail, StatusError, StatusAdvisory, StatusSkipped:
	default:
		return fmt.Errorf("invalid run status %s", r.Status)
	}
	if r.Assessment != Complete && r.Assessment != Incomplete {
		return fmt.Errorf("invalid assessment %s", r.Assessment)
	}
	names := make(map[string]bool)
	for _, g := range r.Gates {
		if g.Name == "" || names[g.Name] {
			return fmt.Errorf("empty or duplicate gate %q", g.Name)
		}
		names[g.Name] = true
		switch g.Status {
		case StatusNotRun, StatusPassed, StatusAdvisory, StatusFailed, StatusError:
		default:
			return fmt.Errorf("invalid gate status %s", g.Status)
		}
		if g.Normalization != NotRun && g.Normalization != Complete && g.Normalization != Partial {
			return fmt.Errorf("invalid normalization %s", g.Normalization)
		}
		if (g.Status == StatusNotRun) != (g.Normalization == NotRun) {
			return fmt.Errorf("inconsistent gate execution/normalization")
		}
	}
	for _, f := range r.Findings {
		if !validFindingStatus(f.Status) {
			return fmt.Errorf("invalid finding status %s", f.Status)
		}
	}
	analyses := make(map[string]bool)
	for _, a := range r.Analyses {
		if r.Command != "diagnose" || a.Name == "" || analyses[a.Name] || !names[a.Gate] {
			return fmt.Errorf("invalid or duplicate diagnostic analysis %q", a.Name)
		}
		analyses[a.Name] = true
		switch a.Status {
		case StatusNotRun, StatusPassed, StatusAdvisory, StatusFailed, StatusError:
		default:
			return fmt.Errorf("invalid analysis status %s", a.Status)
		}
		if a.Normalization != NotRun && a.Normalization != Complete && a.Normalization != Partial {
			return fmt.Errorf("invalid analysis normalization %s", a.Normalization)
		}
		if (a.Status == StatusNotRun) != (a.Normalization == NotRun) || (a.Status == StatusError && a.Normalization == Complete) {
			return fmt.Errorf("inconsistent analysis execution/normalization")
		}
		if (a.Status == StatusNotRun || a.Normalization != Complete) && a.Reason == "" {
			return fmt.Errorf("incomplete analysis requires a reason")
		}
	}
	return nil
}
func (r *RunResult) semanticStatus(code int) Status {
	if r.Status == StatusSkipped {
		return StatusSkipped
	}
	status := StatusPass
	for _, a := range r.Analyses {
		if a.Status == StatusError {
			return StatusError
		}
		if a.Status == StatusFailed {
			status = StatusFail
		}
		if a.Status == StatusAdvisory && status == StatusPass {
			status = StatusAdvisory
		}
	}
	for _, g := range r.Gates {
		if g.Status == StatusError {
			return StatusError
		}
		if g.Status == StatusFailed {
			status = StatusFail
		}
		if g.Status == StatusAdvisory && status == StatusPass {
			status = StatusAdvisory
		}
	}
	for _, f := range r.Findings {
		if f.Status == StatusExecutionError {
			return StatusError
		}
		if f.Status == StatusBlocking {
			status = StatusFail
		}
		if f.Status == StatusAdvisory && status == StatusPass {
			status = StatusAdvisory
		}
	}
	if code != 0 && status == StatusPass {
		status = StatusFail
	}
	return status
}

// Validate rejects invalid domains and contradictory states, while preserving
// legacy advisory-command exit codes (including execution errors with exit 0).
func (r *RunResult) Validate() error {
	if err := r.validateStates(); err != nil {
		return err
	}
	if r.Status == StatusSkipped {
		if r.ExitCode != 0 || len(r.Gates) != 0 || len(r.Findings) != 0 || r.Assessment != Incomplete {
			return fmt.Errorf("inconsistent skipped result")
		}
		return nil
	}
	if r.Status != r.semanticStatus(r.ExitCode) {
		return fmt.Errorf("run status contradicts gate/finding evidence")
	}
	complete := len(r.Gates) > 0
	for _, g := range r.Gates {
		if g.Status == StatusNotRun || g.Status == StatusError || g.Normalization != Complete {
			complete = false
		}
		for _, f := range r.Findings {
			if f.Gate != g.Name {
				continue
			}
			if f.Status == StatusExecutionError && g.Status != StatusError {
				return fmt.Errorf("execution error hidden by gate status")
			}
			if f.Status == StatusAdvisory && g.Status == StatusPassed {
				return fmt.Errorf("advisory finding hidden by gate status")
			}
			if f.Status == StatusBlocking && g.Status != StatusError && g.Status != StatusFailed && g.Status != StatusNotRun {
				return fmt.Errorf("blocking finding hidden by gate status")
			}
		}
	}
	if (r.Assessment == Complete) != (complete && r.analysesComplete()) {
		return fmt.Errorf("assessment contradicts gate completion")
	}
	return nil
}
func (r RunResult) MarshalJSON() ([]byte, error) {
	if err := r.Validate(); err != nil {
		return nil, err
	}
	type wire RunResult
	return json.Marshal(wire(r))
}
func (r *RunResult) UnmarshalJSON(data []byte) error {
	type wire RunResult
	var decoded wire
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	result := RunResult(decoded)
	if err := result.Validate(); err != nil {
		return err
	}
	*r = result
	return nil
}
