package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

type toolSpec struct {
	gate, adapter, reportPath, testsPath string
	advisory                             bool
}

func (r *Runner) machine() bool { return r.outputMode == "json" || r.outputMode == "agent" }
func (r *Runner) println(args ...any) {
	if !r.machine() {
		fmt.Println(args...)
	}
}
func (r *Runner) printf(format string, args ...any) {
	if !r.machine() {
		fmt.Printf(format, args...)
	}
}
func (r *Runner) Run(name string, args ...string) Result {
	return r.RunTool(toolSpec{gate: strings.Fields(name)[0]}, name, args...)
}
func (r *Runner) executionFailure(gate, category, message string) int {
	if r.result != nil {
		r.result.Execution(gate, category, message)
	}
	if !r.machine() {
		fmt.Fprintln(os.Stderr, "  [FAIL] "+gate+" — "+message)
	}
	return 1
}
func (r *Runner) normalize(spec toolSpec, res Result, stdout io.Reader, refs []string) {
	if r.result == nil {
		return
	}
	complete := spec.adapter != ""
	defer func() { r.result.RecordGate(spec.gate, res.ok, complete) }()
	status := "blocking"
	if spec.advisory {
		status = "advisory"
	}
	add := func(rule, severity, category, evidence string, location *Location) {
		s := status
		if category != "quality" {
			s = "execution_error"
		}
		r.result.AddFinding(Finding{Gate: spec.gate, Rule: rule, Severity: severity, Status: s, Category: category, Evidence: evidence, Location: location, Diagnostics: refs})
	}
	execution := func(category, message string) {
		r.result.AddFinding(Finding{Gate: spec.gate, Rule: "pyguard.execution." + category, Severity: "error", Status: "execution_error", Category: category, Evidence: message, Diagnostics: refs})
	}
	if res.category != "" {
		execution(res.category, res.message)
		return
	}
	// uv's own transport/spawn diagnostics are separate from analyzer stdout.
	stderr := strings.ToLower(res.stderr)
	if !res.ok && (strings.HasPrefix(stderr, "modulenotfounderror: no module named ") || strings.Contains(stderr, "\nmodulenotfounderror: no module named ")) {
		execution("tool_missing", "Project analyzer module is missing.")
		return
	}
	if !res.ok && strings.Contains(stderr, "error:") {
		if strings.Contains(stderr, "failed to spawn") || strings.Contains(stderr, "no module named") {
			execution("tool_missing", "Project analyzer or module is missing.")
			return
		}
		if strings.Contains(stderr, "failed to download") || strings.Contains(stderr, "failed to fetch") || strings.Contains(stderr, "failed to build") || strings.Contains(stderr, "network") {
			execution("dependency_failure", "Dependency installation or transport failed.")
			return
		}
	}
	if !res.ok && res.exitCode == 2 && (spec.adapter == "ruff" || spec.adapter == "ruff_format" || spec.adapter == "mypy") {
		execution("invalid_configuration", "Analyzer could not evaluate the configured project.")
		return
	}
	before := len(r.result.Findings)
	decodeFailed := false
	switch spec.adapter {
	case "bandit", "pip-audit", "deptry", "pytest":
		if r.machine() {
			if err := r.normalizeNative(spec, stdout, refs); err != nil {
				decodeFailed = true
			}
		}
	case "ruff":
		var rows []struct {
			Code     string                    `json:"code"`
			Message  string                    `json:"message"`
			Filename string                    `json:"filename"`
			Location struct{ Row, Column int } `json:"location"`
		}
		err := json.NewDecoder(stdout).Decode(&rows)
		if err != nil || rows == nil {
			decodeFailed = true
			break
		}
		for _, d := range rows {
			if d.Code == "" || d.Filename == "" || d.Location.Row < 1 {
				decodeFailed = true
				break
			}
			add(d.Code, "error", "quality", d.Message, &Location{File: relativePath(r.root, d.Filename), Line: d.Location.Row, Column: d.Location.Column})
		}
	case "mypy":
		dec := json.NewDecoder(stdout)
		for {
			var d struct {
				File                    string `json:"file"`
				Line, Column            int
				Message, Code, Severity string
			}
			err := dec.Decode(&d)
			if err == io.EOF {
				break
			}
			if err == nil && d.Severity == "note" {
				continue
			}
			if err != nil || d.File == "" || d.Code == "" || d.Line < 1 {
				decodeFailed = true
				break
			}
			add(d.Code, d.Severity, "quality", d.Message, &Location{File: relativePath(r.root, d.File), Line: d.Line, Column: d.Column + 1})
		}
	case "radon_cc":
		var files map[string][]radonBlock
		if err := json.NewDecoder(stdout).Decode(&files); err != nil || files == nil {
			decodeFailed = true
			break
		}
		var visit func(string, radonBlock)
		visit = func(file string, b radonBlock) {
			if b.Line < 1 || b.Name == "" || b.Complexity < 1 {
				decodeFailed = true
				return
			}
			location := Location{File: relativePath(r.root, file), Line: b.Line, Symbol: b.Name}
			value := float64(b.Complexity)
			if b.Line > 0 && b.Complexity > 0 {
				r.result.Measurements = append(r.result.Measurements, Measurement{Metric: "radon.cc", Level: "code", Location: location, Value: value, Unit: "branches", Threshold: 10})
				if b.Complexity > 10 {
					threshold := 10.0
					r.result.AddFinding(Finding{Gate: spec.gate, Rule: "pyguard.radon.cc_exceeded", Severity: "error", Status: status, Category: "quality", Location: &location, Observed: &value, Threshold: &threshold, Evidence: "Cyclomatic complexity exceeds 10.", Diagnostics: refs})
				}
			}
			for _, child := range b.Methods {
				visit(file, child)
			}
			for _, child := range b.Closures {
				visit(file, child)
			}
		}
		for file, blocks := range files {
			for _, b := range blocks {
				visit(file, b)
			}
		}
	case "radon_mi":
		var files map[string]struct {
			MI    *float64 `json:"mi"`
			Error string   `json:"error"`
		}
		if err := json.NewDecoder(stdout).Decode(&files); err != nil || files == nil {
			decodeFailed = true
			break
		}
		for file, row := range files {
			if row.Error != "" {
				execution("invalid_configuration", "Radon could not parse a source file.")
				continue
			}
			if row.MI == nil {
				decodeFailed = true
				break
			}
			location := Location{File: relativePath(r.root, file)}
			r.result.Measurements = append(r.result.Measurements, Measurement{Metric: "radon.mi", Level: "code", Location: location, Value: *row.MI, Unit: "score", Threshold: 19})
			// Match the existing `radon mi -n B` gate: B and C fail (MI <= 19).
			if *row.MI <= 19 {
				value, threshold := *row.MI, 19.0
				r.result.AddFinding(Finding{Gate: spec.gate, Rule: "pyguard.radon.mi_threshold_failed", Severity: "error", Status: status, Category: "quality", Location: &location, Observed: &value, Threshold: &threshold, Evidence: "Maintainability index is at or below 19.", Diagnostics: refs})
			}
		}
	case "ruff_format":
		if !res.ok {
			add("pyguard.ruff.format_required", "error", "quality", "Ruff formatting check failed.", nil)
		}
	case "owned":
		var data ownedResult
		if err := json.NewDecoder(stdout).Decode(&data); err != nil || data.SchemaVersion != "1" || data.Findings == nil || data.Measurements == nil {
			decodeFailed = true
			break
		}
		if data.Error != nil {
			execution(data.Error.Category, data.Error.Message)
			return
		}
		for _, f := range data.Findings {
			if f.Rule == "" {
				decodeFailed = true
				break
			}
			f.Gate = spec.gate
			f.Category = "quality"
			f.Status = status
			f.Diagnostics = refs
			if f.Location != nil {
				f.Location.File = relativePath(r.root, f.Location.File)
			}
			r.result.AddFinding(f)
		}
		for _, m := range data.Measurements {
			m.Location.File = relativePath(r.root, m.Location.File)
			r.result.Measurements = append(r.result.Measurements, m)
		}
		for _, a := range data.Artifacts {
			a.Path = relativePath(r.root, a.Path)
			r.result.Artifacts = append(r.result.Artifacts, a)
		}
	}
	if decodeFailed {
		complete = false
		execution("adapter_failure", "Analyzer structured output is invalid; inspect diagnostics.")
		return
	}

	if !res.ok && len(r.result.Findings) == before {
		complete = false
		add("pyguard."+spec.gate+".failed", "error", "unclassified_failure", "Gate failed; cause is unknown. Inspect referenced diagnostics.", nil)
	}
}

type ownedResult struct {
	SchemaVersion string        `json:"schema_version"`
	Findings      []Finding     `json:"findings"`
	Measurements  []Measurement `json:"measurements"`
	Artifacts     []Artifact    `json:"artifacts"`
	Error         *struct {
		Category string `json:"category"`
		Message  string `json:"message"`
	} `json:"error"`
}

type radonBlock struct {
	Name       string       `json:"name"`
	Type       string       `json:"type"`
	Line       int          `json:"lineno"`
	Complexity int          `json:"complexity"`
	Methods    []radonBlock `json:"methods"`
	Closures   []radonBlock `json:"closures"`
}
