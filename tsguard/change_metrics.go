package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

//go:embed analysis/change-metrics.cjs
var changeMetricsAnalyzer string

type nativeFileMetric struct {
	File       string  `json:"file"`
	SourceHash string  `json:"source_hash"`
	FTA        float64 `json:"fta_score"`
	Cyclo      float64 `json:"cyclo"`
	Halstead   float64 `json:"halstead_volume"`
	Lines      float64 `json:"lines"`
}
type nativeFunctionMetric struct {
	ID           string   `json:"id"`
	Location     Location `json:"location"`
	Fingerprint  string   `json:"fingerprint"`
	CognitiveMin float64  `json:"cognitive_min"`
	CognitiveMax *float64 `json:"cognitive_max"`
}
type cloneGroup struct {
	Fingerprint string     `json:"fingerprint"`
	Count       int        `json:"count"`
	Locations   []Location `json:"locations"`
}
type nativeCodeProfile struct {
	Files      []nativeFileMetric     `json:"files"`
	Functions  []nativeFunctionMetric `json:"functions"`
	Duplicates struct {
		Clones     int          `json:"clones"`
		Tokens     int          `json:"tokens"`
		Lines      int          `json:"lines"`
		Percentage float64      `json:"percentage"`
		Groups     []cloneGroup `json:"groups"`
	} `json:"duplicates"`
	CognitiveComplete bool `json:"cognitive_complete"`
}
type nativeComparison struct {
	Baseline  nativeCodeProfile `json:"baseline"`
	Candidate nativeCodeProfile `json:"candidate"`
	Policy    map[string]any    `json:"policy"`
}

func finiteNonnegative(n float64) bool { return !math.IsNaN(n) && !math.IsInf(n, 0) && n >= 0 }

func (c *nativeComparison) validate() error {
	if c.Policy == nil {
		return fmt.Errorf("missing native comparison policy")
	}
	for _, p := range []nativeCodeProfile{c.Baseline, c.Candidate} {
		if p.Files == nil || p.Functions == nil || p.Duplicates.Groups == nil {
			return fmt.Errorf("missing native metric arrays")
		}
		seen := map[string]bool{}
		for _, f := range p.Files {
			if f.File == "" || f.SourceHash == "" || seen[f.File] || !finiteNonnegative(f.FTA) || !finiteNonnegative(f.Cyclo) || !finiteNonnegative(f.Halstead) || !finiteNonnegative(f.Lines) {
				return fmt.Errorf("invalid native file metric")
			}
			seen[f.File] = true
		}
		functionIDs := map[string]bool{}
		for _, f := range p.Functions {
			if f.ID == "" || functionIDs[f.ID] || !seen[f.Location.File] || f.Location.Line < 1 || !finiteNonnegative(f.CognitiveMin) || f.CognitiveMax != nil && (!finiteNonnegative(*f.CognitiveMax) || *f.CognitiveMax < f.CognitiveMin) || p.CognitiveComplete && f.CognitiveMax == nil {
				return fmt.Errorf("invalid cognitive metric")
			}
			functionIDs[f.ID] = true
		}
		d := p.Duplicates
		if d.Clones < 0 || d.Tokens < 0 || d.Lines < 0 || !finiteNonnegative(d.Percentage) || d.Percentage > 100 {
			return fmt.Errorf("invalid native duplication metric")
		}
		groups, count := map[string]bool{}, 0
		for _, g := range d.Groups {
			if g.Fingerprint == "" || groups[g.Fingerprint] || g.Count < 1 || len(g.Locations) != 2*g.Count {
				return fmt.Errorf("invalid native clone group")
			}
			groups[g.Fingerprint] = true
			count += g.Count
			for _, loc := range g.Locations {
				if !seen[loc.File] || loc.Line < 1 {
					return fmt.Errorf("invalid native clone location")
				}
			}
		}
		if count != d.Clones {
			return fmt.Errorf("incomplete clone identity evidence")
		}
	}
	return nil
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

func profileMatches(before, after nativeCodeProfile) ([]identityMatch, []identityMatch) {
	filesB, filesA, functionsB, functionsA := map[string][2]string{}, map[string][2]string{}, map[string][2]string{}, map[string][2]string{}
	for _, f := range before.Files {
		filesB[f.File] = [2]string{f.File, f.SourceHash}
	}
	for _, f := range after.Files {
		filesA[f.File] = [2]string{f.File, f.SourceHash}
	}
	functionKey := func(f nativeFunctionMetric) string {
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

func (r *Runner) compareNativeMetrics(before, after *maintainabilitySnapshot, sha string) error {
	c := r.comparison
	if err := c.validate(); err != nil {
		return err
	}
	files, functions := profileMatches(c.Baseline, c.Candidate)
	fileB, fileA := map[string]nativeFileMetric{}, map[string]nativeFileMetric{}
	for _, f := range c.Baseline.Files {
		fileB[f.File] = f
	}
	for _, f := range c.Candidate.Files {
		fileA[f.File] = f
	}
	for _, match := range files {
		b, a := fileB[match.Baseline], fileA[match.Candidate]
		r.measure("change.delta.fta.score", "change", Location{File: a.File}, a.FTA-b.FTA, "score")
		r.measure("change.delta.file.cyclomatic", "change", Location{File: a.File}, a.Cyclo-b.Cyclo, "paths")
	}
	for _, side := range []struct {
		name    string
		profile nativeCodeProfile
	}{{"baseline", c.Baseline}, {"candidate", c.Candidate}} {
		prefix := "change." + side.name + "."
		scores := []float64{}
		var cyclo, volume, cognitiveMin, cognitiveMax float64
		complete := true
		for _, f := range side.profile.Files {
			loc := Location{File: f.File}
			r.measure(prefix+"fta.score", "change", loc, f.FTA, "score")
			r.measure(prefix+"file.cyclomatic", "change", loc, f.Cyclo, "paths")
			r.measure(prefix+"file.halstead_volume", "change", loc, f.Halstead, "volume")
			scores = append(scores, f.FTA)
			cyclo += f.Cyclo
			volume += f.Halstead
		}
		sort.Float64s(scores)
		for _, percentile := range []struct {
			name     string
			fraction float64
		}{{"max_fta_score", 1}, {"p50_fta_score", 0.5}, {"p90_fta_score", 0.9}} {
			value := float64(0)
			if len(scores) > 0 {
				value = scores[max(0, int(math.Ceil(float64(len(scores))*percentile.fraction))-1)]
			}
			r.measure(prefix+percentile.name, "change", Location{File: "."}, value, "score")
		}
		for _, f := range side.profile.Functions {
			r.measure(prefix+"function.cognitive_min", "change", f.Location, f.CognitiveMin, "complexity")
			cognitiveMin += f.CognitiveMin
			if f.CognitiveMax != nil {
				r.measure(prefix+"function.cognitive_max", "change", f.Location, *f.CognitiveMax, "complexity")
				cognitiveMax += *f.CognitiveMax
			} else {
				complete = false
			}
		}
		r.measure(prefix+"total_cyclomatic", "change", Location{File: "."}, cyclo, "paths")
		r.measure(prefix+"total_halstead_volume", "change", Location{File: "."}, volume, "volume")
		r.measure(prefix+"total_cognitive_min", "change", Location{File: "."}, cognitiveMin, "complexity")
		if complete {
			r.measure(prefix+"total_cognitive_max", "change", Location{File: "."}, cognitiveMax, "complexity")
		}
		d := side.profile.Duplicates
		r.measure(prefix+"duplicate_clones", "change", Location{File: "."}, float64(d.Clones), "clones")
		r.measure(prefix+"duplicated_tokens", "change", Location{File: "."}, float64(d.Tokens), "tokens")
		r.measure(prefix+"duplicated_lines", "change", Location{File: "."}, float64(d.Lines), "lines")
		r.measure(prefix+"duplication_percentage", "change", Location{File: "."}, d.Percentage, "percent")
	}
	functionB, functionA := map[string]nativeFunctionMetric{}, map[string]nativeFunctionMetric{}
	for _, f := range c.Baseline.Functions {
		functionB[f.ID] = f
	}
	for _, f := range c.Candidate.Functions {
		functionA[f.ID] = f
	}
	for _, match := range functions {
		b, a := functionB[match.Baseline], functionA[match.Candidate]
		if b.CognitiveMax != nil && a.CognitiveMin > *b.CognitiveMax {
			loc := a.Location
			r.result.AddFinding(Finding{Gate: "change", Level: "change", Rule: "tsguard.change.FUNCTION_COMPLEXITY_INCREASE", Status: "advisory", Severity: "warning", Category: "quality", Location: &loc, Related: []Location{b.Location}, Evidence: fmt.Sprintf("Matched by %s against %s: native Biome cognitive complexity increased beyond baseline upper bound %g to at least %g.", match.Method, sha, *b.CognitiveMax, a.CognitiveMin), Remediation: "Review the added paths and required domain behavior; an increase alone is not a blocking violation."})
		}
	}
	baseClones := map[string]int{}
	for _, g := range c.Baseline.Duplicates.Groups {
		baseClones[g.Fingerprint] = g.Count
	}
	for _, g := range c.Candidate.Duplicates.Groups {
		if g.Count > baseClones[g.Fingerprint] {
			loc := g.Locations[0]
			r.result.AddFinding(Finding{Gate: "change", Level: "change", Rule: "tsguard.change.NEW_DUPLICATION", Status: "advisory", Severity: "warning", Category: "quality", Location: &loc, Related: g.Locations[1:], Evidence: fmt.Sprintf("Native jscpd clone group %s has %d additional matches against %s; exact token identity survives file moves.", g.Fingerprint[:min(12, len(g.Fingerprint))], g.Count-baseClones[g.Fingerprint], sha), Remediation: "Review duplicated logic and independent failure policies; share it only when semantics and ownership permit."})
		}
	}
	r.measure("change.matched_files", "change", Location{File: "."}, float64(len(files)), "files")
	r.measure("change.matched_functions", "change", Location{File: "."}, float64(len(functions)), "functions")
	artifact := struct {
		SchemaVersion      string                   `json:"schema_version"`
		BaselineSHA        string                   `json:"baseline_sha"`
		Comparison         *nativeComparison        `json:"native_metrics"`
		FileMatches        []identityMatch          `json:"file_matches"`
		FunctionMatches    []identityMatch          `json:"function_matches"`
		BaselineStructure  *maintainabilitySnapshot `json:"baseline_structure"`
		CandidateStructure *maintainabilitySnapshot `json:"candidate_structure"`
	}{"1", sha, c, files, functions, before, after}
	data, err := json.MarshalIndent(artifact, "", "  ")
	if err != nil {
		return err
	}
	file := filepath.Join(r.root, opengrepCacheDir, "maintainability-change.json")
	if err := os.MkdirAll(filepath.Dir(file), 0755); err != nil {
		return err
	}
	if err := os.WriteFile(file, data, 0644); err != nil {
		return err
	}
	r.result.Artifacts = append(r.result.Artifacts, Artifact{Kind: "maintainability_change", Path: relativePath(r.root, file)})
	return nil
}
