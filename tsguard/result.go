package main

import (
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

// RunResult is the versioned analysis contract. It contains no rendered CLI text.
// ExitCode preserves process compatibility; Status describes the semantic outcome.
type RunResult struct {
	SchemaVersion string        `json:"schema_version"`
	Status        string        `json:"status"`
	Command       string        `json:"command"`
	ExitCode      int           `json:"exit_code"`
	Findings      []Finding     `json:"findings"`
	Measurements  []Measurement `json:"measurements"`
	Artifacts     []Artifact    `json:"artifacts"`
	Diagnostics   []Diagnostic  `json:"diagnostics"`
}

type Location struct {
	File   string `json:"file"`
	Line   int    `json:"line,omitempty"` // one-based; omitted when unknown
	Column int    `json:"column,omitempty"`
}

type Finding struct {
	ID          string    `json:"id"`
	Level       string    `json:"level"`
	Gate        string    `json:"gate"`
	Rule        string    `json:"rule"`
	Severity    string    `json:"severity"`
	Status      string    `json:"status"` // blocking, advisory, or execution_error
	Category    string    `json:"category"`
	Location    *Location `json:"location,omitempty"`
	Observed    *float64  `json:"observed,omitempty"`
	Threshold   *float64  `json:"threshold,omitempty"`
	Evidence    string    `json:"evidence"`
	Remediation string    `json:"remediation,omitempty"`
	Diagnostics []string  `json:"diagnostics"`
}

type Measurement struct {
	Metric    string   `json:"metric"`
	Level     string   `json:"level"`
	Location  Location `json:"location"`
	Value     float64  `json:"value"`
	Unit      string   `json:"unit"`
	Threshold float64  `json:"threshold"`
}

type Artifact struct {
	Kind string `json:"kind"`
	Path string `json:"path"`
}

type Diagnostic struct {
	ID      string `json:"id"`
	Gate    string `json:"gate"`
	Channel string `json:"channel"`
	Path    string `json:"path"`
}

func newRunResult(command string) *RunResult {
	return &RunResult{SchemaVersion: "1", Status: "pass", Command: command,
		Findings: []Finding{}, Measurements: []Measurement{}, Artifacts: []Artifact{}, Diagnostics: []Diagnostic{}}
}

func (r *RunResult) addFinding(f Finding) {
	if f.Level == "" {
		f.Level = "code"
	}
	if f.Diagnostics == nil {
		f.Diagnostics = []string{}
	}
	file, line, column := "", 0, 0
	if f.Location != nil {
		file, line, column = f.Location.File, f.Location.Line, f.Location.Column
	}
	// IDs never depend on English messages, analyzer order, cwd, timestamps or PIDs.
	identity := fmt.Sprintf("%s\x00%s\x00%s\x00%d\x00%d", f.Gate, f.Rule, file, line, column)
	f.ID = fmt.Sprintf("%s:%x", f.Rule, sha256.Sum256([]byte(identity)))
	r.Findings = append(r.Findings, f)
}

func (r *RunResult) execution(gate, category, evidence string) {
	r.addFinding(Finding{Gate: gate, Rule: "tsguard.execution." + category,
		Severity: "error", Status: "execution_error", Category: category, Evidence: evidence})
}

func (r *RunResult) finish(code int) {
	r.ExitCode = code
	for _, f := range r.Findings {
		if f.Status == "execution_error" {
			r.Status = "error"
			break
		}
		if f.Status == "blocking" {
			r.Status = "fail"
		} else if r.Status == "pass" {
			r.Status = "advisory"
		}
	}
	sort.SliceStable(r.Findings, func(i, j int) bool {
		a, b := r.Findings[i], r.Findings[j]
		priority := map[string]int{"execution_error": 0, "blocking": 1, "advisory": 2}
		if priority[a.Status] != priority[b.Status] {
			return priority[a.Status] < priority[b.Status]
		}
		if a.Gate != b.Gate {
			return a.Gate < b.Gate
		}
		al, bl := Location{}, Location{}
		if a.Location != nil {
			al = *a.Location
		}
		if b.Location != nil {
			bl = *b.Location
		}
		if al.File != bl.File {
			return al.File < bl.File
		}
		if al.Line != bl.Line {
			return al.Line < bl.Line
		}
		if al.Column != bl.Column {
			return al.Column < bl.Column
		}
		return a.ID < b.ID
	})
	sort.SliceStable(r.Measurements, func(i, j int) bool {
		a, b := r.Measurements[i], r.Measurements[j]
		if a.Location.File != b.Location.File {
			return a.Location.File < b.Location.File
		}
		return a.Metric < b.Metric
	})
}

func relativePath(root, path string) string {
	if filepath.IsAbs(path) {
		if rel, err := filepath.Rel(root, path); err == nil {
			path = rel
		}
	}
	return strings.ReplaceAll(filepath.Clean(path), "\\", "/")
}
