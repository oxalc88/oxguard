package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

func (r *Runner) reportFile(adapter, name string) (string, error) {
	dir := filepath.Join(r.root, "node_modules", ".cache", "oxguard", "reports", adapter)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, name)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return "", err
	}
	return path, nil
}

func (r *Runner) normalizeNative(spec toolSpec, res Result, reader io.Reader, refs []string) error {
	if spec.reportPath != "" {
		file, err := os.Open(spec.reportPath)
		if err != nil {
			return err
		}
		defer file.Close()
		reader = file
		r.result.Artifacts = append(r.result.Artifacts, Artifact{Kind: "analyzer-report", Path: relativePath(r.root, spec.reportPath)})
	}
	add := func(rule, message string, loc *Location) {
		status, severity := "blocking", "error"
		if spec.advisory {
			status, severity = "advisory", "warning"
		}
		r.result.AddFinding(Finding{Gate: spec.gate, Rule: rule, Severity: severity, Status: status, Category: "quality", Evidence: message, Location: loc, Diagnostics: refs})
	}
	switch spec.adapter {
	case "dependency-audit", "audit-ci":
		return r.normalizeDependencyAudit(spec, res, reader, refs)
	case "secretlint":
		var rows []struct {
			File     string `json:"filePath"`
			Messages []struct {
				Rule    string `json:"ruleId"`
				Message string `json:"message"`
				Loc     struct {
					Start struct{ Line, Column int } `json:"start"`
				} `json:"loc"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(reader).Decode(&rows); err != nil {
			return err
		}
		if rows == nil {
			return fmt.Errorf("missing secretlint results")
		}
		for _, row := range rows {
			for _, m := range row.Messages {
				if m.Rule == "" || row.File == "" || m.Loc.Start.Line < 1 {
					return fmt.Errorf("invalid secretlint finding")
				}
				add(m.Rule, "Potential secret detected by "+m.Rule+".", &Location{File: relativePath(r.root, row.File), Line: m.Loc.Start.Line, Column: m.Loc.Start.Column + 1})
			}
		}
	case "knip":
		var data struct {
			Issues []map[string]json.RawMessage `json:"issues"`
		}
		if err := json.NewDecoder(reader).Decode(&data); err != nil {
			return err
		}
		if data.Issues == nil {
			return fmt.Errorf("missing Knip issues")
		}
		for _, row := range data.Issues {
			var file string
			if err := json.Unmarshal(row["file"], &file); err != nil || file == "" {
				return fmt.Errorf("missing Knip file")
			}
			for kind, raw := range row {
				if kind == "file" || kind == "owners" {
					continue
				}
				var items []json.RawMessage
				if err := json.Unmarshal(raw, &items); err != nil {
					return err
				}
				for _, item := range items {
					var symbols []json.RawMessage
					if len(item) > 0 && item[0] == '[' {
						if err := json.Unmarshal(item, &symbols); err != nil {
							return err
						}
					} else {
						symbols = []json.RawMessage{item}
					}
					for _, symbol := range symbols {
						var record struct {
							Name   string `json:"name"`
							Line   int    `json:"line"`
							Column int    `json:"col"`
						}
						if err := json.Unmarshal(symbol, &record); err != nil {
							return err
						}
						add("knip."+kind, "Unused or unresolved "+kind+": "+record.Name, &Location{File: relativePath(r.root, file), Line: record.Line, Column: record.Column, Symbol: record.Name})
					}
				}
			}
		}
	case "jscpd":
		var data struct {
			Duplicates []struct {
				Lines     int `json:"lines"`
				FirstFile struct {
					Name  string                     `json:"name"`
					Start struct{ Line, Column int } `json:"startLoc"`
				} `json:"firstFile"`
				SecondFile struct {
					Name  string                     `json:"name"`
					Start struct{ Line, Column int } `json:"startLoc"`
				} `json:"secondFile"`
			} `json:"duplicates"`
			Statistics struct {
				Total struct {
					Percentage *float64 `json:"percentage"`
				} `json:"total"`
			} `json:"statistics"`
		}
		if err := json.NewDecoder(reader).Decode(&data); err != nil {
			return err
		}
		if data.Duplicates == nil || data.Statistics.Total.Percentage == nil {
			return fmt.Errorf("missing jscpd results")
		}
		r.result.Measurements = append(r.result.Measurements, Measurement{Metric: "duplicates.percentage", Level: "code", Value: *data.Statistics.Total.Percentage, Unit: "percent"})
		for _, d := range data.Duplicates {
			if d.FirstFile.Name == "" || d.SecondFile.Name == "" || d.Lines < 1 {
				return fmt.Errorf("invalid duplicate")
			}
			first := Location{File: relativePath(r.root, d.FirstFile.Name), Line: d.FirstFile.Start.Line, Column: d.FirstFile.Start.Column}
			second := Location{File: relativePath(r.root, d.SecondFile.Name), Line: d.SecondFile.Start.Line, Column: d.SecondFile.Start.Column}
			if second.File < first.File || second.File == first.File && (second.Line < first.Line || second.Line == first.Line && second.Column < first.Column) {
				first, second = second, first
			}
			lines := float64(d.Lines)
			r.result.AddFinding(Finding{Gate: spec.gate, Rule: "jscpd.duplicate", Severity: "warning", Status: "advisory", Category: "quality", Observed: &lines, Evidence: fmt.Sprintf("%d duplicated lines.", d.Lines), Location: &first, Related: []Location{second}, Diagnostics: refs})
		}
	case "coverage-summary":
		if err := r.result.IstanbulSummary(reader, r.root, refs); err != nil {
			return err
		}
		if spec.testsPath != "" {
			file, err := os.Open(spec.testsPath)
			if err != nil {
				return err
			}
			defer file.Close()
			var data struct {
				Success       *bool `json:"success"`
				NumTotalTests *int  `json:"numTotalTests"`
				TestResults   []struct {
					Name             string `json:"name"`
					Status           string `json:"status"`
					AssertionResults []struct {
						FullName, Status string
						FailureMessages  []string `json:"failureMessages"`
					}
				} `json:"testResults"`
			}
			if err := json.NewDecoder(file).Decode(&data); err != nil {
				return err
			}
			if data.Success == nil || data.NumTotalTests == nil {
				return fmt.Errorf("missing test results")
			}
			if *data.NumTotalTests == 0 {
				r.result.Execution(spec.gate, "invalid_configuration", "No tests were collected.")
			}
			for _, suite := range data.TestResults {
				for _, test := range suite.AssertionResults {
					if test.Status == "failed" {
						add("tsguard.tests.failed", "Test failed: "+test.FullName, &Location{File: relativePath(r.root, suite.Name), Symbol: test.FullName})
					}
				}
			}
			if !*data.Success && len(r.result.Findings) == 0 {
				r.result.Execution(spec.gate, "analyzer_failure", "Test runner could not complete the configured suite.")
			}
			r.result.Artifacts = append(r.result.Artifacts, Artifact{Kind: "test-report", Path: relativePath(r.root, spec.testsPath)})
		}
	default:
		return fmt.Errorf("unsupported adapter")
	}
	return nil
}

func (r *Runner) coverageReport(runner string, args []string) (toolSpec, []string, error) {
	spec := toolSpec{gate: "coverage"}
	if !r.machine() {
		return spec, args, nil
	}
	report, err := r.reportFile("coverage", "coverage-summary.json")
	if err != nil {
		return spec, args, err
	}
	spec.adapter, spec.reportPath = "coverage-summary", report
	dir := filepath.Dir(report)
	switch runner {
	case "vitest", "jest":
		tests, err := r.reportFile("coverage", "tests.json")
		if err != nil {
			return spec, args, err
		}
		spec.testsPath = tests
		if runner == "vitest" {
			// Preserve coverage evidence when tests fail; this changes reporting,
			// not the test outcome or the existing thresholds.
			args = append(args, "--coverage.reporter=json-summary", "--coverage.reportOnFailure", "--coverage.reportsDirectory="+dir, "--reporter=json", "--outputFile="+tests)
		} else {
			args = append(args, "--coverageReporters=json-summary", "--coverageDirectory="+dir, "--json", "--outputFile="+tests)
		}
	default:
		args = append(args, "--reporter=json-summary", "--report-dir="+dir)
	}
	return spec, args, nil
}
