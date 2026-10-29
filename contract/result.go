package contract

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
	namespace     string
	SchemaVersion string        `json:"schema_version"`
	Status        Status        `json:"status"`
	Assessment    Completeness  `json:"assessment"`
	Gates         []Gate        `json:"gates"`
	Command       string        `json:"command"`
	ExitCode      int           `json:"exit_code"`
	Findings      []Finding     `json:"findings"`
	Measurements  []Measurement `json:"measurements"`
	Artifacts     []Artifact    `json:"artifacts"`
	Diagnostics   []Diagnostic  `json:"diagnostics"`
	Analyses      []Analysis    `json:"analyses,omitempty"`
}

type Location struct {
	File   string `json:"file"`
	Line   int    `json:"line,omitempty"` // one-based; omitted when unknown
	Column int    `json:"column,omitempty"`
	Symbol string `json:"symbol,omitempty"`
}

type Finding struct {
	ID          string     `json:"id"`
	Level       string     `json:"level"`
	Gate        string     `json:"gate"`
	Rule        string     `json:"rule"`
	Severity    string     `json:"severity"`
	Status      Status     `json:"status"` // blocking, advisory, or execution_error
	Category    string     `json:"category"`
	Location    *Location  `json:"location,omitempty"`
	Related     []Location `json:"related,omitempty"`
	Observed    *float64   `json:"observed,omitempty"`
	Threshold   *float64   `json:"threshold,omitempty"`
	Evidence    string     `json:"evidence"`
	Remediation string     `json:"remediation,omitempty"`
	Diagnostics []string   `json:"diagnostics"`
}

type Measurement struct {
	Metric    string   `json:"metric"`
	Level     string   `json:"level"`
	Location  Location `json:"location"`
	Value     float64  `json:"value"`
	Unit      string   `json:"unit"`
	Threshold float64  `json:"threshold,omitempty"`
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

func New(namespace, command string) *RunResult {
	return &RunResult{namespace: namespace, SchemaVersion: "1", Status: StatusPass, Command: command,
		Gates: []Gate{}, Assessment: Incomplete, Findings: []Finding{}, Measurements: []Measurement{}, Artifacts: []Artifact{}, Diagnostics: []Diagnostic{}}
}

func (r *RunResult) AddFinding(f Finding) {
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
	if f.Location != nil && f.Location.Symbol != "" {
		identity += "\x00symbol:" + f.Location.Symbol
	}
	for _, loc := range f.Related {
		identity += fmt.Sprintf("\x00related:%s:%d:%d:%s", loc.File, loc.Line, loc.Column, loc.Symbol)
	}
	f.ID = fmt.Sprintf("%s:%x", f.Rule, sha256.Sum256([]byte(identity)))
	r.Findings = append(r.Findings, f)
}

func (r *RunResult) Execution(gate, category, evidence string) {
	r.AddFinding(Finding{Gate: gate, Rule: r.namespace + ".execution." + category,
		Severity: "error", Status: StatusExecutionError, Category: category, Evidence: evidence})
	r.RecordGate(gate, false, true)
}

func (r *RunResult) Finish(code int) error {
	if err := r.validateStates(); err != nil {
		return err
	}
	r.Status = r.semanticStatus(code)
	r.ExitCode = code
	r.Assessment = Complete
	if len(r.Gates) == 0 {
		r.Assessment = Incomplete
	}
	for _, g := range r.Gates {
		if g.Status == StatusNotRun || g.Status == StatusError || g.Normalization != Complete {
			r.Assessment = Incomplete
		}
	}
	if !r.analysesComplete() {
		r.Assessment = Incomplete
	}

	sort.SliceStable(r.Findings, func(i, j int) bool {
		a, b := r.Findings[i], r.Findings[j]
		priority := map[Status]int{StatusExecutionError: 0, StatusBlocking: 1, StatusAdvisory: 2}
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
		if a.Metric != b.Metric {
			return a.Metric < b.Metric
		}
		if a.Location.Line != b.Location.Line {
			return a.Location.Line < b.Location.Line
		}
		return a.Location.Symbol < b.Location.Symbol
	})
	return r.Validate()
}

func RelativePath(root, path string) string {
	root = stripWindowsExtendedPrefix(root)
	path = stripWindowsExtendedPrefix(path)
	if filepath.IsAbs(path) {
		// macOS exposes temporary roots through /var -> /private/var. Compare
		// physical paths so explicit roots and invocation-cwd logs agree.
		// Windows 8.3 short names are another alias for the same directory.
		if physical, err := filepath.EvalSymlinks(root); err == nil {
			root = stripWindowsExtendedPrefix(physical)
		}
		if physical, err := filepath.EvalSymlinks(path); err == nil {
			path = stripWindowsExtendedPrefix(physical)
		}
		if rel, err := filepath.Rel(root, path); err == nil && filepath.IsLocal(rel) {
			path = rel
		}
	}
	return strings.ReplaceAll(filepath.Clean(path), "\\", "/")
}

// stripWindowsExtendedPrefix removes the extended-length prefixes Windows
// analyzers can emit (\\?\C:\..., //?/C:/..., and their UNC forms). Go's
// filepath.Rel treats an extended path as a different volume from a normal
// project root, so without this a path under the project looks like an
// unrelated absolute path. The slash form is used by complexipy 8.3.0.
func stripWindowsExtendedPrefix(path string) string {
	prefixes := []struct {
		prefix      string
		replacement string
	}{
		{`\\?\UNC\`, `\\`},
		{`//?/UNC/`, `//`},
		{`\\?\`, ``},
		{`//?/`, ``},
	}
	for _, candidate := range prefixes {
		if len(path) >= len(candidate.prefix) && strings.EqualFold(path[:len(candidate.prefix)], candidate.prefix) {
			return candidate.replacement + path[len(candidate.prefix):]
		}
	}
	return path
}
