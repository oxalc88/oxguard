package contract

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
)

// Coverage reports the analyzer's global measurement; file coverage does not
// create new thresholds. The caller supplies the thresholds already enforced.
func (r *RunResult) Coverage(namespace, metric string, value, threshold float64, refs []string) error {
	if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value > 100 {
		return fmt.Errorf("invalid coverage percentage")
	}
	r.Measurements = append(r.Measurements, Measurement{Metric: "coverage." + metric, Level: "code", Value: value, Unit: "percent", Threshold: threshold})
	if value < threshold {
		r.AddFinding(Finding{Gate: "coverage", Rule: namespace + ".coverage." + metric + "_threshold_failed", Severity: "error", Status: "blocking", Category: "quality", Observed: &value, Threshold: &threshold, Evidence: fmt.Sprintf("%s coverage %g%% is below %g%%.", metric, value, threshold), Diagnostics: refs})
	}
	return nil
}

// IstanbulSummary accepts the JSON summary shared by Vitest, Jest, c8 and nyc.
func (r *RunResult) IstanbulSummary(reader io.Reader, root string, refs []string) error {
	var report map[string]map[string]json.RawMessage
	if err := json.NewDecoder(reader).Decode(&report); err != nil {
		return err
	}
	for _, metric := range []string{"lines", "functions", "branches", "statements"} {
		raw, ok := report["total"][metric]
		var row struct {
			Pct *float64 `json:"pct"`
		}
		if !ok || json.Unmarshal(raw, &row) != nil || row.Pct == nil {
			return fmt.Errorf("missing %s coverage", metric)
		}
		if err := r.Coverage("tsguard", metric, *row.Pct, 80, refs); err != nil {
			return err
		}
	}
	for file, rows := range report {
		if file == "total" {
			continue
		}
		for _, metric := range []string{"lines", "functions", "branches", "statements"} {
			var row struct {
				Pct *float64 `json:"pct"`
			}
			if json.Unmarshal(rows[metric], &row) != nil || row.Pct == nil || *row.Pct < 0 || *row.Pct > 100 {
				return fmt.Errorf("invalid file coverage")
			}
			r.Measurements = append(r.Measurements, Measurement{Metric: "coverage." + metric, Level: "code", Location: Location{File: RelativePath(root, file)}, Value: *row.Pct, Unit: "percent"})
		}
	}
	return nil
}
