package contract

import (
	"encoding/json"
	"fmt"
)

// Status is numeric internally; zero is uninitialized. Schema-1 strings remain unchanged.
// Each field validates its own allowed domain, preventing cross-domain states.
type Status uint8

const (
	StatusUnknown Status = iota
	StatusPass
	StatusFail
	StatusError
	StatusAdvisory
	StatusSkipped
	StatusBlocking
	StatusExecutionError
	StatusNotRun
	StatusPassed
	StatusFailed
)

var statusNames = [...]string{"", "pass", "fail", "error", "advisory", "skipped", "blocking", "execution_error", "not_run", "passed", "failed"}

func (s Status) String() string {
	if s == StatusUnknown || int(s) >= len(statusNames) {
		return "unknown"
	}
	return statusNames[s]
}
func (s Status) MarshalJSON() ([]byte, error) {
	if s == StatusUnknown || int(s) >= len(statusNames) {
		return nil, fmt.Errorf("unknown status %d", s)
	}
	return json.Marshal(s.String())
}
func (s *Status) UnmarshalJSON(data []byte) error {
	var name string
	if err := json.Unmarshal(data, &name); err != nil {
		return fmt.Errorf("status must be a schema-1 string: %w", err)
	}
	for i := 1; i < len(statusNames); i++ {
		if statusNames[i] == name {
			*s = Status(i)
			return nil
		}
	}
	return fmt.Errorf("unknown status %q", name)
}

// Completeness separates assessment and per-gate normalization domains.
type Completeness uint8

const (
	CompletenessUnknown Completeness = iota
	NotRun
	Complete
	Partial
	Incomplete
)

var completenessNames = [...]string{"", "not_run", "complete", "partial", "incomplete"}

func (s Completeness) String() string {
	if s == CompletenessUnknown || int(s) >= len(completenessNames) {
		return "unknown"
	}
	return completenessNames[s]
}
func (s Completeness) MarshalJSON() ([]byte, error) {
	if s == CompletenessUnknown || int(s) >= len(completenessNames) {
		return nil, fmt.Errorf("unknown completeness %d", s)
	}
	return json.Marshal(s.String())
}
func (s *Completeness) UnmarshalJSON(data []byte) error {
	var name string
	if err := json.Unmarshal(data, &name); err != nil {
		return fmt.Errorf("completeness must be a schema-1 string: %w", err)
	}
	for i := 1; i < len(completenessNames); i++ {
		if completenessNames[i] == name {
			*s = Completeness(i)
			return nil
		}
	}
	return fmt.Errorf("unknown completeness %q", name)
}
