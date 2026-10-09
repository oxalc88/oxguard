package main

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Read exact Git blobs, not git archive: export-ignore/export-subst attributes
// must not hide or transform baseline source. No checkout, hooks or source runs.
func baselinePath(name string) (string, bool, error) {
	clean := filepath.FromSlash(name)
	if name == "" || filepath.IsAbs(clean) || filepath.VolumeName(clean) != "" || strings.Contains(name, "\\") || clean == ".." || strings.HasPrefix(filepath.Clean(clean), ".."+string(filepath.Separator)) {
		return "", false, fmt.Errorf("unsafe baseline path")
	}
	for _, part := range strings.Split(filepath.ToSlash(clean), "/") {
		if part == "node_modules" || part == ".git" {
			return clean, false, nil
		}
	}
	switch filepath.Ext(clean) {
	case ".ts", ".tsx", ".mts", ".cts", ".js", ".jsx", ".mjs", ".cjs", ".json", ".jsonc":
		return clean, true, nil
	}
	return clean, false, nil
}

type baselineBlob struct{ name, sha string }

func baselineRoot(ctx context.Context, root, sha string) (string, error) {
	prefixCommand := exec.CommandContext(ctx, "git", "rev-parse", "--show-prefix")
	prefixCommand.Dir = root
	prefixBytes, err := prefixCommand.Output()
	if err != nil {
		return "", err
	}
	prefix := strings.TrimSpace(string(prefixBytes))
	listing := exec.CommandContext(ctx, "git", "ls-tree", "-r", "--full-tree", "-z", sha)
	listing.Dir = root
	stdout, err := listing.StdoutPipe()
	if err != nil {
		return "", err
	}
	if err = listing.Start(); err != nil {
		return "", err
	}
	data, readErr := io.ReadAll(io.LimitReader(stdout, 4*1024*1024+1))
	if readErr != nil || len(data) > 4*1024*1024 {
		listing.Process.Kill()
	}
	waitErr := listing.Wait()
	if readErr != nil {
		return "", readErr
	}
	if len(data) > 4*1024*1024 {
		return "", fmt.Errorf("baseline tree exceeds analysis budget")
	}
	if waitErr != nil {
		return "", waitErr
	}
	blobs := []baselineBlob{}
	var requests strings.Builder
	for _, entry := range bytes.Split(data, []byte{0}) {
		if len(entry) == 0 {
			continue
		}
		fields := bytes.SplitN(entry, []byte{'\t'}, 2)
		if len(fields) != 2 {
			return "", fmt.Errorf("invalid Git tree record")
		}
		meta := strings.Fields(string(fields[0]))
		if len(meta) != 3 {
			return "", fmt.Errorf("invalid Git tree metadata")
		}
		if (meta[0] != "100644" && meta[0] != "100755") || meta[1] != "blob" {
			continue
		} // no symlinks/submodules
		name := string(fields[1])
		if !strings.HasPrefix(name, prefix) {
			continue
		}
		name = strings.TrimPrefix(name, prefix)
		clean, selected, err := baselinePath(name)
		if err != nil {
			return "", err
		}
		if !selected {
			continue
		}
		if len(blobs) >= 20000 {
			return "", fmt.Errorf("baseline source exceeds 20000 files")
		}
		blobs = append(blobs, baselineBlob{clean, meta[2]})
		requests.WriteString(meta[2] + "\n")
	}
	temporary, err := os.MkdirTemp("", "tsguard-baseline-")
	if err != nil {
		return "", err
	}
	ok := false
	defer func() {
		if !ok {
			os.RemoveAll(temporary)
		}
	}()
	cmd := exec.CommandContext(ctx, "git", "cat-file", "--batch")
	cmd.Dir = root
	cmd.Stdin = strings.NewReader(requests.String())
	stream, err := cmd.StdoutPipe()
	if err != nil {
		return "", err
	}
	if err = cmd.Start(); err != nil {
		return "", err
	}
	copyErr := copyBaselineBlobs(bufio.NewReader(stream), temporary, blobs)
	if copyErr != nil {
		cmd.Process.Kill()
	}
	waitErr = cmd.Wait()
	if copyErr != nil {
		return "", copyErr
	}
	if waitErr != nil {
		return "", fmt.Errorf("git cat-file: %w", waitErr)
	}
	if _, err = os.Stat(filepath.Join(temporary, "package.json")); os.IsNotExist(err) {
		if err = os.WriteFile(filepath.Join(temporary, "package.json"), []byte("{}"), 0600); err != nil {
			return "", err
		}
	}
	ok = true
	return temporary, nil
}

func copyBaselineBlobs(reader *bufio.Reader, root string, blobs []baselineBlob) error {
	var total int64
	for _, blob := range blobs {
		header, err := reader.ReadString('\n')
		if err != nil {
			return err
		}
		fields := strings.Fields(header)
		if len(fields) != 3 || fields[0] != blob.sha || fields[1] != "blob" {
			return fmt.Errorf("invalid Git blob response")
		}
		size, err := strconv.ParseInt(fields[2], 10, 64)
		if err != nil || size < 0 || size > 8*1024*1024 {
			return fmt.Errorf("baseline file exceeds analysis budget")
		}
		total += size
		if total > 128*1024*1024 {
			return fmt.Errorf("baseline source exceeds 128 MiB")
		}
		destination := filepath.Join(root, blob.name)
		if err = os.MkdirAll(filepath.Dir(destination), 0755); err != nil {
			return err
		}
		file, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			return err
		}
		_, copyErr := io.CopyN(file, reader, size)
		closeErr := file.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
		separator, err := reader.ReadByte()
		if err != nil || separator != '\n' {
			return fmt.Errorf("invalid Git blob separator")
		}
	}
	return nil
}

func (r *Runner) comparisonUnavailable(rule, message string) {
	r.result.Plan("change")
	r.result.AddFinding(Finding{Gate: "change", Level: "change", Rule: "tsguard.change." + rule, Status: "advisory", Severity: "info", Category: "quality", Evidence: message, Remediation: "Provide an available Git commit with --baseline and rerun change; do not infer an improvement from per-file scores alone."})
	r.printf("  change: not evaluated — %s\n", message)
}

func runChange(r *Runner) int {
	if r.result == nil {
		r.result = newRunResult("change")
	}
	r.result.Plan("change")
	if r.baseline == "" {
		r.comparisonUnavailable("BASELINE_UNAVAILABLE", "No explicit Git baseline; change quality was not evaluated.")
		return 0
	}
	if _, err := exec.LookPath("git"); err != nil {
		r.executionFailure("change", "tool_missing", "Git is required for --baseline.")
		return 0
	}
	timeout := r.timeout
	if timeout <= 0 {
		timeout = 300
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--verify", "--end-of-options", r.baseline+"^{commit}")
	cmd.Dir = r.root
	out, err := cmd.Output()
	if err != nil {
		r.comparisonUnavailable("BASELINE_UNAVAILABLE", "Requested Git baseline could not be resolved to a commit.")
		return 0
	}
	sha := strings.TrimSpace(string(out))
	// Analyze candidate and baseline through the same trusted compiler runtime.
	// Only source facts are compared; dependency code is never executed.
	if r.snapshot == nil {
		runMaintainability(r, "maintainability")
	}
	if r.snapshot == nil {
		r.comparisonUnavailable("INCOMPLETE_INPUT", "Candidate structural analysis is unavailable.")
		return 0
	}
	root, err := baselineRoot(ctx, r.root, sha)
	if err != nil {
		r.executionFailure("change", "analyzer_failure", err.Error())
		return 0
	}
	defer os.RemoveAll(root)
	baseline := *r
	baseline.outputMode = "json"
	baseline.root = root
	baseline.snapshot = nil
	baseline.result = newRunResult("baseline")
	baseline.runOwnedAnalysis("baseline", maintainabilityAnalyzer, true, map[string]any{"sourceOnly": true, "compilerRoot": r.root, "excludeTests": r.ftaExcludeTests})
	if baseline.snapshot == nil || baseline.snapshot.UnresolvedImports > 0 || r.snapshot.UnresolvedImports > 0 {
		for _, f := range baseline.result.Findings {
			if f.Status == "execution_error" {
				r.executionFailure("change", f.Category, "Baseline analysis: "+f.Evidence)
			}
		}
		r.comparisonUnavailable("INCOMPLETE_INPUT", "Baseline or candidate structural inputs are incomplete; comparison was not evaluated.")
		return 0
	}
	if err := r.compareSnapshots(baseline.snapshot, r.snapshot, sha); err != nil {
		r.executionFailure("change", "adapter_failure", err.Error())
		return 0
	}
	r.result.RecordGate("change", true, true)
	return 0
}

type changeMetrics struct {
	Modules, Edges, Cycles, Depth, Wrappers, Branches, MaxFileBranches, MaxFunctionBranches int
}

func snapshotMetrics(s *maintainabilitySnapshot) (changeMetrics, error) {
	g, err := analyzeModules(s.Modules, s.Imports)
	if err != nil {
		return changeMetrics{}, err
	}
	m := changeMetrics{Modules: len(s.Modules), Edges: len(s.Imports), Cycles: g.Cycles}
	for _, n := range g.Depth {
		m.Depth = max(m.Depth, n)
	}
	perFile := map[string]int{}
	for _, f := range s.Functions {
		if f.ForwardTarget != "" {
			m.Wrappers++
		}
		m.Branches += f.Branches
		perFile[f.Location.File] += f.Branches
		m.MaxFunctionBranches = max(m.MaxFunctionBranches, f.Branches)
	}
	for _, n := range perFile {
		m.MaxFileBranches = max(m.MaxFileBranches, n)
	}
	return m, nil
}

func (r *Runner) compareSnapshots(before, after *maintainabilitySnapshot, sha string) error {
	base, err := snapshotMetrics(before)
	if err != nil {
		return err
	}
	candidate, err := snapshotMetrics(after)
	if err != nil {
		return err
	}
	metrics := []struct {
		name string
		a, b int
	}{
		{"module_count", base.Modules, candidate.Modules}, {"module_edges", base.Edges, candidate.Edges}, {"module_cycles", base.Cycles, candidate.Cycles}, {"dependency_depth", base.Depth, candidate.Depth}, {"wrappers", base.Wrappers, candidate.Wrappers}, {"total_branches", base.Branches, candidate.Branches}, {"max_file_branches", base.MaxFileBranches, candidate.MaxFileBranches}, {"max_function_branches", base.MaxFunctionBranches, candidate.MaxFunctionBranches},
	}
	for _, metric := range metrics {
		r.measure("change.baseline."+metric.name, "change", Location{File: "."}, float64(metric.a), "count")
		r.measure("change.candidate."+metric.name, "change", Location{File: "."}, float64(metric.b), "count")
	}
	// Multiple independent facts, never a raw file/dependency count judgment.
	displaced := base.MaxFileBranches > 0 && candidate.MaxFileBranches < base.MaxFileBranches && candidate.Branches >= base.Branches && candidate.Edges > base.Edges && (candidate.Depth > base.Depth || candidate.Wrappers > base.Wrappers || candidate.Cycles > base.Cycles)
	if displaced {
		r.result.AddFinding(Finding{Gate: "change", Level: "change", Rule: "tsguard.change.POSSIBLE_COMPLEXITY_DISPLACEMENT", Status: "advisory", Severity: "warning", Category: "quality", Location: &Location{File: "."},
			Evidence: fmt.Sprintf("Baseline %s: maximum per-file branches %d→%d, total branches %d→%d, runtime edges %d→%d, dependency depth %d→%d, unchanged forwarding functions %d→%d, cycles %d→%d.", sha, base.MaxFileBranches, candidate.MaxFileBranches, base.Branches, candidate.Branches, base.Edges, candidate.Edges, base.Depth, candidate.Depth, base.Wrappers, candidate.Wrappers, base.Cycles, candidate.Cycles), Remediation: "Review the end-to-end workflow and behavior before claiming simplification; smaller file scores alone do not show reduced maintainability cost."})
	}
	if candidate.Cycles > base.Cycles {
		r.result.AddFinding(Finding{Gate: "change", Level: "change", Rule: "tsguard.change.NEW_DEPENDENCY_CYCLES", Status: "advisory", Severity: "warning", Category: "quality", Location: &Location{File: "."}, Evidence: fmt.Sprintf("Runtime dependency cycles increased from %d to %d against %s.", base.Cycles, candidate.Cycles, sha)})
	}
	return nil
}
