package main

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Native report files are recreated for each invocation, so stale output cannot
// certify a new run. Raw logs remain separately available for investigation.
func (r *Runner) reportFile(adapter, name string) (string, error) {
	dir := filepath.Join(r.root, ".pyguard-cache", "reports", adapter)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, name)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return "", err
	}
	return path, nil
}

func (r *Runner) normalizeNative(spec toolSpec, reader io.Reader, refs []string) error {
	if spec.reportPath != "" {
		file, err := os.Open(spec.reportPath)
		if err != nil {
			return err
		}
		defer file.Close()
		reader = file
		r.result.Artifacts = append(r.result.Artifacts, Artifact{Kind: "analyzer-report", Path: relativePath(r.root, spec.reportPath)})
	}
	add := func(rule, severity, message, file string, line int) {
		status := "blocking"
		if spec.advisory {
			status = "advisory"
		}
		var loc *Location
		if file != "" {
			loc = &Location{File: relativePath(r.root, file), Line: line}
		}
		r.result.AddFinding(Finding{Gate: spec.gate, Rule: rule, Severity: severity, Status: status, Category: "quality", Evidence: message, Location: loc, Diagnostics: refs})
	}
	switch spec.adapter {
	case "bandit":
		var data struct {
			Results []struct {
				Rule     string `json:"test_id"`
				Message  string `json:"issue_text"`
				Severity string `json:"issue_severity"`
				File     string `json:"filename"`
				Line     int    `json:"line_number"`
			} `json:"results"`
			Errors []json.RawMessage `json:"errors"`
		}
		if err := json.NewDecoder(reader).Decode(&data); err != nil {
			return err
		}
		if data.Results == nil || data.Errors == nil {
			return fmt.Errorf("missing Bandit arrays")
		}
		for _, row := range data.Results {
			if row.Rule == "" || row.File == "" || row.Line < 1 {
				return fmt.Errorf("invalid Bandit result")
			}
			severity := "warning"
			if row.Severity == "HIGH" {
				severity = "error"
			}
			add(row.Rule, severity, row.Message, row.File, row.Line)
		}
		if len(data.Errors) > 0 {
			r.result.Execution(spec.gate, "analyzer_failure", "Bandit could not evaluate one or more source files.")
		}
	case "pip-audit":
		var data struct {
			Dependencies []struct {
				Name, Version string
				SkipReason    string `json:"skip_reason"`
				Vulns         []struct {
					ID          string   `json:"id"`
					Description string   `json:"description"`
					FixVersions []string `json:"fix_versions"`
				} `json:"vulns"`
			} `json:"dependencies"`
		}
		if err := json.NewDecoder(reader).Decode(&data); err != nil {
			return err
		}
		if data.Dependencies == nil {
			return fmt.Errorf("missing pip-audit dependencies")
		}
		for _, pkg := range data.Dependencies {
			if pkg.Name == "" {
				return fmt.Errorf("invalid audited package")
			}
			if pkg.SkipReason != "" {
				r.result.Execution(spec.gate, "analyzer_failure", "A dependency could not be audited.")
				continue
			}
			if pkg.Version == "" || pkg.Vulns == nil {
				return fmt.Errorf("missing audited version")
			}
			for _, v := range pkg.Vulns {
				if v.ID == "" {
					return fmt.Errorf("missing vulnerability id")
				}
				remediation := ""
				if len(v.FixVersions) > 0 {
					remediation = "Fixed versions: " + strings.Join(v.FixVersions, ", ")
				}
				r.result.AddFinding(Finding{Gate: spec.gate, Rule: v.ID, Severity: "error", Status: "blocking", Category: "quality", Location: &Location{Symbol: pkg.Name}, Evidence: pkg.Name + "@" + pkg.Version + ": " + v.Description, Remediation: remediation, Diagnostics: refs})
			}
		}
	case "deptry":
		var rows []struct {
			Error    struct{ Code, Message string } `json:"error"`
			Module   string                         `json:"module"`
			Location struct {
				File string `json:"file"`
				Line int    `json:"line"`
			} `json:"location"`
		}
		if err := json.NewDecoder(reader).Decode(&rows); err != nil {
			return err
		}
		if rows == nil {
			return fmt.Errorf("missing deptry results")
		}
		for _, row := range rows {
			if row.Error.Code == "" {
				return fmt.Errorf("missing deptry code")
			}
			add(row.Error.Code, "warning", row.Module+": "+row.Error.Message, row.Location.File, row.Location.Line)
		}
	case "pytest":
		var data struct {
			Totals struct {
				Percent *float64 `json:"percent_covered"`
			} `json:"totals"`
			Files map[string]struct {
				Summary struct {
					Percent *float64 `json:"percent_covered"`
				} `json:"summary"`
			} `json:"files"`
		}
		coverageErr := json.NewDecoder(reader).Decode(&data)
		// JUnit is parsed even when coverage is absent (e.g. failing test collection).
		file, err := os.Open(spec.testsPath)
		if err != nil {
			return err
		}
		defer file.Close()
		dec := xml.NewDecoder(file)
		found := false
		for {
			tok, err := dec.Token()
			if err == io.EOF {
				break
			}
			if err != nil {
				return err
			}
			start, ok := tok.(xml.StartElement)
			if !ok || start.Name.Local != "testcase" {
				continue
			}
			found = true
			var test struct {
				Name     string `xml:"name,attr"`
				File     string `xml:"file,attr"`
				Line     int    `xml:"line,attr"`
				Failures []struct {
					Message string `xml:"message,attr"`
				} `xml:"failure"`
				Errors []struct {
					Message string `xml:"message,attr"`
				} `xml:"error"`
			}
			if err := dec.DecodeElement(&test, &start); err != nil {
				return err
			}
			for _, f := range test.Failures {
				add("pyguard.tests.failed", "error", test.Name+": "+f.Message, test.File, test.Line+1)
			}
			for range test.Errors {
				r.result.Execution(spec.gate, "analyzer_failure", "pytest could not collect or execute a test.")
			}
		}
		if !found {
			r.result.Execution(spec.gate, "invalid_configuration", "pytest collected no tests.")
		}
		r.result.Artifacts = append(r.result.Artifacts, Artifact{Kind: "test-report", Path: relativePath(r.root, spec.testsPath)})
		if coverageErr != nil {
			return coverageErr
		}
		if data.Totals.Percent == nil {
			return fmt.Errorf("missing coverage.py total")
		}
		for file, row := range data.Files {
			if row.Summary.Percent == nil || *row.Summary.Percent < 0 || *row.Summary.Percent > 100 {
				return fmt.Errorf("invalid file coverage")
			}
			r.result.Measurements = append(r.result.Measurements, Measurement{Metric: "coverage.lines", Level: "code", Location: Location{File: relativePath(r.root, file)}, Value: *row.Summary.Percent, Unit: "percent"})
		}
		return r.result.Coverage("pyguard", "lines", *data.Totals.Percent, 80, refs)
	default:
		return fmt.Errorf("unsupported adapter: %s", strings.TrimSpace(spec.adapter))
	}
	return nil
}
