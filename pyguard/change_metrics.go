package main

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type changeFunctionMetric struct {
	ID             string   `json:"id"`
	Location       Location `json:"location"`
	Fingerprint    string   `json:"fingerprint"`
	Cognitive      *float64 `json:"cognitive"`
	Cyclomatic     float64  `json:"cyclomatic"`
	CognitiveError string   `json:"cognitive_error,omitempty"`
}

type identityMatch struct {
	Baseline  string `json:"baseline"`
	Candidate string `json:"candidate"`
	Method    string `json:"method"`
}

// Prefer the existing path/symbol, then match only unique exact source/token
// fingerprints. Ambiguous copies stay unmatched; never infer behavioral
// equivalence or match unrelated anonymous functions by position.
func matchIdentities(before, after map[string][2]string) []identityMatch {
	usedBefore, usedAfter := map[string]bool{}, map[string]bool{}
	matches := []identityMatch{}
	for index, method := range []string{"path_or_symbol", "exact_fingerprint"} {
		bucketsB, bucketsA := map[string][]string{}, map[string][]string{}
		for id, keys := range before {
			if keys[index] != "" && !usedBefore[id] {
				bucketsB[keys[index]] = append(bucketsB[keys[index]], id)
			}
		}
		for id, keys := range after {
			if keys[index] != "" && !usedAfter[id] {
				bucketsA[keys[index]] = append(bucketsA[keys[index]], id)
			}
		}
		for key, ids := range bucketsB {
			if len(ids) == 1 && len(bucketsA[key]) == 1 {
				b, a := ids[0], bucketsA[key][0]
				usedBefore[b], usedAfter[a] = true, true
				matches = append(matches, identityMatch{b, a, method})
			}
		}
	}
	sort.Slice(matches, func(i, j int) bool { return matches[i].Baseline < matches[j].Baseline })
	return matches
}

func profileMatches(before, after changeProfile) ([]identityMatch, []identityMatch) {
	filesB, filesA, functionsB, functionsA := map[string][2]string{}, map[string][2]string{}, map[string][2]string{}, map[string][2]string{}
	for _, f := range before.Files {
		filesB[f.File] = [2]string{f.File, f.SourceHash}
	}
	for _, f := range after.Files {
		filesA[f.File] = [2]string{f.File, f.SourceHash}
	}
	functionKey := func(f changeFunctionMetric) string {
		if f.Location.Symbol == "" || strings.Contains(f.Location.Symbol, "<anonymous>") {
			return ""
		}
		return f.Location.File + "\x00" + f.Location.Symbol
	}
	for _, f := range before.Functions {
		functionsB[f.ID] = [2]string{functionKey(f), f.Fingerprint}
	}
	for _, f := range after.Functions {
		functionsA[f.ID] = [2]string{functionKey(f), f.Fingerprint}
	}
	return matchIdentities(filesB, filesA), matchIdentities(functionsB, functionsA)
}

func validChangeNumber(n float64) bool { return !math.IsNaN(n) && !math.IsInf(n, 0) && n >= 0 }

func validateFunctionMetrics(p changeProfile) error {
	if p.Functions == nil {
		return fmt.Errorf("missing function metric array")
	}
	files := map[string]bool{}
	for _, f := range p.Files {
		if f.SourceHash == "" {
			return fmt.Errorf("missing file source hash: %s", f.File)
		}
		files[f.File] = true
	}
	seen := map[string]bool{}
	for _, f := range p.Functions {
		if f.ID == "" || seen[f.ID] || !files[f.Location.File] || f.Location.Line < 1 || f.Location.Symbol == "" || f.Fingerprint == "" || (!validChangeNumber(f.Cyclomatic) || f.Cyclomatic < 1) || f.Cognitive != nil && !validChangeNumber(*f.Cognitive) {
			return fmt.Errorf("invalid function metric: %s", f.ID)
		}
		seen[f.ID] = true
	}
	return nil
}

func (r *Runner) functionChangeMeasure(metric string, loc Location, value float64, unit string) {
	r.result.Measurements = append(r.result.Measurements, Measurement{Metric: metric, Level: "change", Location: loc, Value: value, Unit: unit})
}

func (r *Runner) compareFunctionMetrics(b, a changeProfile) bool {
	for _, p := range []changeProfile{b, a} {
		if err := validateFunctionMetrics(p); err != nil {
			r.comparisonUnavailable("INCOMPLETE_METRICS", err.Error())
			return false
		}
	}
	files, functions := profileMatches(b, a)
	r.changeMeasure("change.matched_files", ".", float64(len(files)), "files")
	r.changeMeasure("change.matched_functions", ".", float64(len(functions)), "functions")
	complete := true
	versionOK := b.Versions["complexipy"] != "" && b.Versions["complexipy"] == a.Versions["complexipy"]
	if !versionOK {
		complete = false
		r.comparisonUnavailable("INCOMPLETE_METRICS", "Complexipy version evidence is missing or differs; cognitive scores cannot be compared.")
	}
	for _, side := range []struct {
		name string
		p    changeProfile
	}{{"baseline", b}, {"candidate", a}} {
		total, sideComplete := 0.0, versionOK && side.p.CognitiveError == ""
		if side.p.CognitiveError != "" {
			complete = false
			r.comparisonUnavailable("INCOMPLETE_METRICS", side.name+": "+side.p.CognitiveError)
		}
		for _, f := range side.p.Functions {
			prefix := "change." + side.name + ".function."
			r.functionChangeMeasure(prefix+"cyclomatic", f.Location, f.Cyclomatic, "paths")
			if f.Cognitive == nil || !versionOK || side.p.CognitiveError != "" {
				sideComplete, complete = false, false
				r.comparisonUnavailable("INCOMPLETE_METRICS", fmt.Sprintf("%s function %s (%s:%d): cognitive score unavailable. %s", side.name, f.Location.Symbol, f.Location.File, f.Location.Line, f.CognitiveError))
			} else {
				r.functionChangeMeasure(prefix+"cognitive", f.Location, *f.Cognitive, "score")
				total += *f.Cognitive
			}
		}
		if sideComplete {
			r.changeMeasure("change."+side.name+".total_cognitive", ".", total, "score")
		}
	}
	before, after := map[string]changeFunctionMetric{}, map[string]changeFunctionMetric{}
	for _, f := range b.Functions {
		before[f.ID] = f
	}
	for _, f := range a.Functions {
		after[f.ID] = f
	}
	for _, match := range functions {
		old, current := before[match.Baseline], after[match.Candidate]
		r.functionChangeMeasure("change.delta.function.cyclomatic", current.Location, current.Cyclomatic-old.Cyclomatic, "paths")
		if !versionOK || b.CognitiveError != "" || a.CognitiveError != "" || old.Cognitive == nil || current.Cognitive == nil {
			continue
		}
		delta := *current.Cognitive - *old.Cognitive
		r.functionChangeMeasure("change.delta.function.cognitive", current.Location, delta, "score")
		if delta > 0 {
			loc := current.Location
			r.result.AddFinding(Finding{Gate: "change", Level: "change", Rule: "pyguard.change.FUNCTION_COMPLEXITY_INCREASE", Status: StatusAdvisory, Severity: "warning", Category: "quality", Location: &loc, Related: []Location{old.Location}, Evidence: fmt.Sprintf("Matched by %s against baseline %s: complexipy cognitive complexity increased from %g to %g.", match.Method, r.baselineSHA, *old.Cognitive, *current.Cognitive), Remediation: "Review the added control flow and required domain behavior; an increase alone is not a blocking violation."})
		}
	}
	nativeProfile := func(p changeProfile) map[string]any {
		complete := versionOK && p.CognitiveError == ""
		for _, f := range p.Functions {
			if f.Cognitive == nil {
				complete = false
			}
		}
		return map[string]any{"files": p.Files, "functions": p.Functions, "versions": p.Versions, "cognitive_complete": complete}
	}
	artifact := map[string]any{
		"schema_version": "1", "baseline_sha": r.baselineSHA,
		"native_metrics": map[string]any{"baseline": nativeProfile(b), "candidate": nativeProfile(a), "policy": map[string]any{"candidate_config_applies_to_both": true, "exclude": r.exclude, "exclude_tests": r.excludeTests, "dirs": r.dirs, "nested_cognitive": "independently measured; parent retains native folding"}},
		"file_matches":   files, "function_matches": functions,
		"baseline_structure": b.Structure, "candidate_structure": a.Structure,
	}
	data, err := json.MarshalIndent(artifact, "", "  ")
	file := filepath.Join(r.root, ".pyguard-cache", "maintainability-change.json")
	if err == nil {
		err = os.MkdirAll(filepath.Dir(file), 0755)
	}
	if err == nil {
		err = os.WriteFile(file, data, 0644)
	}
	if err != nil {
		r.comparisonUnavailable("INCOMPLETE_METRICS", "Cannot write maintainability-change artifact: "+err.Error())
		return false
	}
	r.result.Artifacts = append(r.result.Artifacts, Artifact{Kind: "maintainability_change", Path: relativePath(r.root, file)})
	return complete
}
