package main

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// Native audit advisories are informational. Only audit-ci's native decision
// blocks, preserving its allowlists and configuration without reimplementing policy.
func (r *Runner) normalizeDependencyAudit(spec toolSpec, res Result, reader io.Reader, refs []string) error {
	decoder := json.NewDecoder(reader)
	valid := false
	count := 0
	add := func(id, name, severity, title string) {
		if id == "" {
			id = "tsguard.dependencies.vulnerable"
		}
		if severity != "error" && severity != "warning" {
			severity = "warning"
		}
		if spec.adapter == "dependency-audit" {
			r.result.AddFinding(Finding{Gate: spec.gate, Rule: id, Severity: severity, Status: "advisory", Category: "quality", Location: &Location{File: "package.json", Symbol: name}, Evidence: name + ": " + title, Diagnostics: refs})
		}
		count++
	}
	for {
		var report struct {
			Error           json.RawMessage `json:"error"`
			Version         int             `json:"auditReportVersion"`
			Metadata        json.RawMessage `json:"metadata"`
			Vulnerabilities map[string]struct {
				Name, Severity string
				Via            []json.RawMessage `json:"via"`
			} `json:"vulnerabilities"`
			Advisories map[string]struct {
				Name                 string `json:"module_name"`
				Title, Severity, URL string
			} `json:"advisories"`
			Type string          `json:"type"`
			Data json.RawMessage `json:"data"`
		}
		err := decoder.Decode(&report)
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if len(report.Error) > 0 && string(report.Error) != "null" {
			r.result.Execution(spec.gate, "dependency_failure", "Dependency registry audit failed; inspect diagnostics.")
			return nil
		}
		if report.Type == "auditSummary" {
			valid = true
			continue
		}
		if report.Type == "auditAdvisory" {
			var event struct {
				Advisory struct {
					Name                 string `json:"module_name"`
					Title, Severity, URL string
				}
			}
			if err := json.Unmarshal(report.Data, &event); err != nil {
				return err
			}
			v := event.Advisory
			add(advisoryID(v.URL), v.Name, v.Severity, v.Title)
			valid = true
			continue
		}
		if report.Version == 2 && report.Vulnerabilities != nil || report.Advisories != nil && len(report.Metadata) > 0 || len(report.Metadata) > 0 && report.Vulnerabilities != nil {
			valid = true
		} else {
			return fmt.Errorf("unsupported dependency audit report")
		}
		for name, v := range report.Vulnerabilities {
			for _, via := range v.Via {
				var row struct{ Name, Severity, Title, URL string }
				if json.Unmarshal(via, &row) != nil {
					continue
				}
				add(advisoryID(row.URL), name, row.Severity, row.Title)
			}
		}
		for _, v := range report.Advisories {
			add(advisoryID(v.URL), v.Name, v.Severity, v.Title)
		}
	}
	if !valid {
		return fmt.Errorf("missing dependency audit report")
	}
	if spec.adapter == "audit-ci" && !res.ok {
		if count == 0 {
			r.result.Execution(spec.gate, "analyzer_failure", "audit-ci failed without a reported vulnerability; inspect diagnostics.")
		} else {
			r.result.AddFinding(Finding{Gate: spec.gate, Rule: "tsguard.dependencies.policy_failed", Severity: "error", Status: "blocking", Category: "quality", Evidence: "audit-ci rejected the native dependency report under the configured severity and allowlist policy.", Diagnostics: refs})
		}
	}
	return nil
}
func advisoryID(url string) string {
	parts := strings.Split(strings.TrimRight(url, "/"), "/")
	id := parts[len(parts)-1]
	if strings.HasPrefix(id, "GHSA-") {
		return id
	}
	return "tsguard.dependencies.vulnerable"
}
