package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

type toolSpec struct {
	gate, adapter, subject, reportPath, testsPath string
	advisory                                      bool
	threshold                                     float64
}

var tscDiagnostic = regexp.MustCompile(`^(.+)\((\d+),(\d+)\): error (TS\d+): (.*)$`)
var tscGlobalDiagnostic = regexp.MustCompile(`^(?:error )?(TS\d+): (.*)$`)

// FTA 3.0.1 exits before printing JSON on a cap failure. This is its exact
// first-failure stderr record; success uses native JSON, never a rendered table.
var ftaCapDiagnostic = regexp.MustCompile(`(?m)^File (.+) has a score of ([0-9.eE+-]+), which is beyond the score cap of ([0-9]+), exiting\.$`)
var networkDiagnostic = regexp.MustCompile(`(?m)(?:^npm (?:ERR!|error) code |^\s*code: ['"])(EAI_AGAIN|ENOTFOUND|ECONNRESET|ECONNREFUSED|ETIMEDOUT)\b`)
var missingModuleDiagnostic = regexp.MustCompile(`(?m)(?:^Error: Cannot find module |^\s*code: ['"](?:MODULE_NOT_FOUND|ERR_MODULE_NOT_FOUND)['"])`)

func (r *Runner) normalize(spec toolSpec, res Result, stdout io.Reader, refs []string) {
	complete := spec.adapter != ""
	defer func() { r.result.RecordGate(spec.gate, res.ok, complete) }()
	start := len(r.result.Findings)
	status := "blocking"
	if spec.advisory {
		status = "advisory"
	}
	add := func(rule, severity, category, evidence string, location *Location) {
		if severity != "error" && severity != "warning" {
			severity = "info"
		}
		findingStatus := status
		if severity != "error" {
			findingStatus = "advisory"
		}
		if category != "quality" {
			findingStatus = "execution_error"
		}
		r.result.AddFinding(Finding{Gate: spec.gate, Rule: rule, Severity: severity,
			Status: findingStatus, Category: category, Evidence: evidence, Location: location, Diagnostics: refs})
	}
	if res.category != "" {
		add("tsguard.execution."+res.category, "error", res.category, res.message, nil)
		return
	}
	// Recognize explicit transport/package-resolution codes, not English guesses.
	if !res.ok {
		if code := networkDiagnostic.FindStringSubmatch(res.output); code != nil {
			add("tsguard.execution.dependency_failure", "error", "dependency_failure", "Dependency transport failed: "+code[1], nil)
			return
		}
		if missingModuleDiagnostic.MatchString(res.output) {
			add("tsguard.execution.tool_missing", "error", "tool_missing", "An analyzer or required module is missing.", nil)
			return
		}
	}
	var adapterErr error
	switch spec.adapter {
	case "secretlint", "knip", "jscpd", "coverage-summary", "dependency-audit", "audit-ci":
		if r.machine() {
			if err := r.normalizeNative(spec, res, stdout, refs); err != nil {
				adapterErr = err
			}
		}
	case "criticality":
		if res.ok {
			adapterErr = r.normalizeCriticality(stdout, refs)
		}
	case "tsc":
		reader := bufio.NewReader(stdout)
		for {
			line, err := reader.ReadString('\n')
			line = strings.TrimRight(line, "\r\n")
			if m := tscDiagnostic.FindStringSubmatch(line); m != nil {
				row, _ := strconv.Atoi(m[2])
				col, _ := strconv.Atoi(m[3])
				add(m[4], "error", "quality", m[5], &Location{File: relativePath(r.root, m[1]), Line: row, Column: col})
			} else if m := tscGlobalDiagnostic.FindStringSubmatch(line); m != nil {
				add(m[1], "error", "invalid_configuration", m[2], nil)
			}
			if err != nil {
				if err != io.EOF {
					adapterErr = err
				}
				break
			}
		}
	case "fta":
		if !res.ok {
			if m := ftaCapDiagnostic.FindStringSubmatch(res.output); m != nil {
				value, e1 := strconv.ParseFloat(m[2], 64)
				threshold, e2 := strconv.ParseFloat(m[3], 64)
				if e1 == nil && e2 == nil {
					r.addFTA(spec, m[1], value, threshold, refs, status)
				}
			}
		} else {
			var entries []struct {
				File  string  `json:"file_name"`
				Score float64 `json:"fta_score"`
			}
			adapterErr = json.NewDecoder(stdout).Decode(&entries)
			if adapterErr == nil {
				for _, entry := range entries {
					r.addFTA(spec, entry.File, entry.Score, spec.threshold, refs, status)
				}
			}
		}
	case "biome":
		var report struct {
			Diagnostics []struct {
				Severity string `json:"severity"`
				Message  string `json:"message"`
				Category string `json:"category"`
				Location struct {
					Path  string `json:"path"`
					Start *struct {
						Line   int `json:"line"`
						Column int `json:"column"`
					} `json:"start"`
				} `json:"location"`
			} `json:"diagnostics"`
		}
		adapterErr = json.NewDecoder(stdout).Decode(&report)
		if adapterErr == nil && report.Diagnostics == nil {
			adapterErr = fmt.Errorf("missing Biome diagnostics array")
		}
		if adapterErr == nil {
			for _, d := range report.Diagnostics {
				category := "quality"
				if strings.HasPrefix(d.Category, "configuration") {
					category = "invalid_configuration"
				}
				if strings.HasPrefix(d.Category, "internalError") || strings.HasPrefix(d.Category, "internal/") {
					category = "analyzer_failure"
				}
				var location *Location
				if d.Location.Path != "" {
					location = &Location{File: relativePath(r.root, d.Location.Path)}
					if d.Location.Start != nil {
						// Biome JSON uses one-based positions; zero means unknown.
						location.Line = d.Location.Start.Line
						location.Column = d.Location.Start.Column
					}
				}
				rule := d.Category
				if rule == "" {
					rule = "tsguard.lint.failed"
				}
				severity := d.Severity
				if severity == "fatal" {
					severity = "error"
				}
				add(rule, severity, category, d.Message, location)
			}
		}
	case "opengrep":
		var report struct {
			Results []struct {
				Rule  string `json:"check_id"`
				Path  string `json:"path"`
				Start struct {
					Line   int `json:"line"`
					Column int `json:"col"`
				} `json:"start"`
				Extra struct {
					Message  string `json:"message"`
					Severity string `json:"severity"`
				} `json:"extra"`
			} `json:"results"`
			Errors []struct {
				Type    string `json:"type"`
				Message string `json:"message"`
			} `json:"errors"`
		}
		adapterErr = json.NewDecoder(stdout).Decode(&report)
		if adapterErr == nil && (report.Results == nil || report.Errors == nil) {
			adapterErr = fmt.Errorf("missing Opengrep results/errors arrays")
		}
		if adapterErr == nil {
			for _, d := range report.Results {
				// --error makes every match blocking, even a WARNING/INFO rule.
				severity := strings.ToLower(d.Extra.Severity)
				if severity != "warning" && severity != "info" {
					severity = "error"
				}
				r.result.AddFinding(Finding{Gate: spec.gate, Rule: d.Rule, Severity: severity, Status: status, Category: "quality", Evidence: d.Extra.Message,
					Location: &Location{File: relativePath(r.root, d.Path), Line: d.Start.Line, Column: d.Start.Column}, Diagnostics: refs})
			}
			for _, e := range report.Errors {
				category := "analyzer_failure"
				if strings.Contains(e.Type, "Rule") || strings.Contains(e.Type, "Config") {
					category = "invalid_configuration"
				}
				add("tsguard.execution."+category, "error", category, e.Message, nil)
			}
		}
	}
	if adapterErr != nil {
		complete = false
		add("tsguard.execution.adapter_failure", "error", "adapter_failure", "Analyzer structured output could not be decoded; inspect diagnostics.", nil)
	}

	if !res.ok && len(r.result.Findings) == start {
		complete = false
		// Unknown tool details are never confidently labelled source-code defects.
		r.result.AddFinding(Finding{Gate: spec.gate, Rule: "tsguard." + spec.gate + ".failed", Severity: "error", Status: "execution_error",
			Category: "unclassified_failure", Evidence: "Analyzer failed; inspect diagnostics for this gate.", Diagnostics: refs})
	}
}

func (r *Runner) addFTA(spec toolSpec, file string, value, threshold float64, refs []string, status string) {
	location := Location{File: relativePath(r.root, filepath.Join(spec.subject, file))}
	r.result.Measurements = append(r.result.Measurements, Measurement{Metric: "fta.score", Level: "code", Location: location, Value: value, Unit: "score", Threshold: threshold})
	if value > threshold {
		r.result.AddFinding(Finding{Gate: "fta", Rule: "tsguard.fta.score_exceeded", Severity: "error", Status: status, Category: "quality",
			Location: &location, Observed: &value, Threshold: &threshold,
			Evidence: fmt.Sprintf("FTA score %g exceeds %g.", value, threshold), Diagnostics: refs})
	}
}

func (r *Runner) executionFailure(gate, category, message string) int {
	if r.result != nil {
		r.result.Execution(gate, category, message)
	}
	return 1
}

func (r *Runner) machine() bool { return r.outputMode == "agent" || r.outputMode == "json" }

func (r *Runner) printf(format string, args ...any) {
	if !r.machine() {
		fmt.Printf(format, args...)
	}
}

func (r *Runner) println(args ...any) {
	if !r.machine() {
		fmt.Println(args...)
	}
}

func (r *Runner) errorf(format string, args ...any) {
	if !r.machine() {
		fmt.Fprintf(os.Stderr, format, args...)
	}
}
