package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

// Runner executes subprocesses from the project root.
type Runner struct {
	dirs         []string
	dirsExplicit bool
	outputMode   string
	result       *RunResult
	root         string
	timeout      int      // seconds per tool
	logFile      string   // path to append full output (empty = no log file)
	tailLines    int      // print only the last N lines to stdout (0 = all)
	excludeTests bool     // exclude conventional test files from radon/complexity gates
	exclude      []string // additional exclude globs from [tool.pyguard] exclude
}

// exportExcludeEnv sets PYGUARD_EXCLUDE_TESTS and PYGUARD_EXCLUDE_GLOBS for Python
// analysis scripts that use _paths.collect_paths as their single file-walk point.
func (r *Runner) exportExcludeEnv() {
	val := "0"
	if r.excludeTests {
		val = "1"
	}
	os.Setenv("PYGUARD_EXCLUDE_TESTS", val)
	os.Setenv("PYGUARD_EXCLUDE_GLOBS", strings.Join(r.exclude, ","))
}

// Result holds the outcome of a single tool run.
type Result struct {
	name     string
	ok       bool
	category string
	message  string
	exitCode int
	stderr   string
	output   string // captured output (may be truncated to tailLines for display)
}

// RunTool executes a command, normalizes its outcome, and renders by mode.
// On success: prints "[OK] name". On failure: prints "[FAIL] name" + output.
// Output is bounded to 2 MB in memory; --tail N caps the displayed lines.
func (r *Runner) RunTool(spec toolSpec, name string, args ...string) Result {
	if r.machine() {
		for _, arg := range args {
			if strings.HasPrefix(arg, "tools/analysis/") && strings.HasSuffix(arg, ".py") {
				if _, err := os.Stat(filepath.Join(r.root, arg)); err != nil {
					r.executionFailure(spec.gate, "tool_missing", "Project analysis helper is missing or unreadable; run pyguard setup to deploy updated helpers.")
					return Result{name: name}
				}
			}
		}
	}
	cmd := exec.Command(args[0], args[1:]...) //nolint:gosec
	cmd.Dir = r.root
	setSysProcAttr(cmd)

	cbuf := newCappedBuf()
	writers := []io.Writer{cbuf}

	if r.logFile != "" {
		logF, err := os.OpenFile(r.logFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err == nil {
			defer logF.Close()
			writers = append(writers, logF)
			if r.machine() {
				path := r.logFile
				if absolute, err := filepath.Abs(path); err == nil {
					path = absolute
				}
				r.result.Artifacts = append(r.result.Artifacts, Artifact{Kind: "log", Path: relativePath(r.root, path)})
			}
		} else if r.machine() {
			r.executionFailure(spec.gate, "diagnostics_failure", "Cannot open --log-file.")
			return Result{name: name}
		}
	}

	cmd.Stdout = io.MultiWriter(writers...)
	stderrBuf := newCappedBuf()
	cmd.Stderr = io.MultiWriter(append(writers, stderrBuf)...)
	var stdoutFile *os.File
	var refs []string
	if r.machine() {
		// Spool complete structured stdout separately from stderr. Normalization
		// never uses --tail or the 2 MiB diagnostic memory window.
		dir := filepath.Join(r.root, ".pyguard-cache", "diagnostics")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			r.executionFailure(spec.gate, "diagnostics_failure", "Cannot create diagnostics directory.")
			return Result{name: name}
		}
		for _, channel := range []string{"stdout", "stderr"} {
			id := fmt.Sprintf("diagnostic-%03d", len(r.result.Diagnostics)+1)
			path := filepath.Join(dir, id+"-"+spec.gate+"-"+channel+".log")
			file, err := os.Create(path)
			if err != nil {
				r.executionFailure(spec.gate, "diagnostics_failure", "Cannot write analyzer diagnostics.")
				return Result{name: name}
			}
			defer file.Close()
			r.result.Diagnostics = append(r.result.Diagnostics, Diagnostic{ID: id, Gate: spec.gate, Channel: channel, Path: relativePath(r.root, path)})
			refs = append(refs, id)
			if channel == "stdout" {
				stdoutFile = file
				cmd.Stdout = io.MultiWriter(append(writers, file)...)
			} else {
				cmd.Stderr = io.MultiWriter(append(append([]io.Writer{}, writers...), stderrBuf, file)...)
			}
		}
	}

	stopSignals := forwardSignals(cmd)

	err := cmd.Start()
	if err != nil {
		stopSignals()
		res := Result{name: name, output: fmt.Sprintf("failed to start: %v", err), category: "startup_failure", message: "Analyzer could not start."}
		if errors.Is(err, exec.ErrNotFound) || os.IsNotExist(err) {
			res.category = "tool_missing"
			res.message = "Analyzer executable is missing."
		}
		if r.machine() {
			r.normalize(spec, res, strings.NewReader(""), refs)
		} else if r.result != nil {
			r.result.Execution(spec.gate, res.category, res.message)
		}
		return res
	}

	var timedOut atomic.Bool
	timer := time.AfterFunc(time.Duration(r.timeout)*time.Second, func() {
		timedOut.Store(true)
		killProcessGroup(cmd)
	})

	err = cmd.Wait()
	timer.Stop()
	stopSignals()

	output := cbuf.tail(0)
	ok := err == nil
	res := Result{name: name, ok: ok, output: output, exitCode: cmd.ProcessState.ExitCode(), stderr: stderrBuf.tail(0)}
	if timedOut.Load() {
		res.category = "timeout"
		res.message = "Analyzer exceeded the per-tool timeout."
	} else if exitErr, yes := err.(*exec.ExitError); yes && exitErr.ExitCode() == -1 {
		res.category = "interrupted"
		res.message = "Analyzer was terminated by a signal."
	}
	if r.machine() {
		before := len(r.result.Findings)
		if _, err := stdoutFile.Seek(0, 0); err != nil {
			r.executionFailure(spec.gate, "diagnostics_failure", "Cannot read analyzer diagnostics.")
			res.ok = false
		} else {
			r.normalize(spec, res, stdoutFile, refs)
		}
		for _, f := range r.result.Findings[before:] {
			// A broken adapter cannot claim a successful run.
			if f.Status == "execution_error" {
				res.ok = false
			}
		}
		return res
	}
	// Human rendering consumes a normalized per-tool projection too. Retain
	// legacy analyzer invocations and display their text only as diagnostics.
	normalized := newRunResult(spec.gate)
	adapterRunner := *r
	adapterRunner.result = normalized
	spec.adapter = "" // native structured formats are opt-in; human tools stay unchanged
	adapterRunner.normalize(spec, res, strings.NewReader(output), nil)
	code := 0
	if !res.ok {
		code = 1
	}
	normalized.Finish(code)
	if r.result != nil {
		r.result.Findings = append(r.result.Findings, normalized.Findings...)
		r.result.Measurements = append(r.result.Measurements, normalized.Measurements...)
		r.result.Artifacts = append(r.result.Artifacts, normalized.Artifacts...)
	}
	output = cbuf.tail(r.tailLines)
	reportHumanTool(os.Stdout, name, normalized, r.logFile, output)

	res.output = output
	return res
}

// RunSilent executes a command and returns (stdout, stderr, error) without printing.
// Used by setup and doctor for internal checks.
func RunSilent(dir string, args ...string) (string, string, error) {
	cmd := exec.Command(args[0], args[1:]...) //nolint:gosec
	if dir != "" {
		cmd.Dir = dir
	}
	setSysProcAttr(cmd)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	stopSignals := forwardSignals(cmd)
	err := cmd.Run()
	stopSignals()
	return stdout.String(), stderr.String(), err
}

// RunCapture executes a command and returns combined output + error.
func RunCapture(dir string, args ...string) (string, error) {
	cmd := exec.Command(args[0], args[1:]...) //nolint:gosec
	if dir != "" {
		cmd.Dir = dir
	}
	setSysProcAttr(cmd)
	stopSignals := forwardSignals(cmd)
	out, err := cmd.CombinedOutput()
	stopSignals()
	return string(out), err
}

// RunStreaming executes a command with output streamed directly to stdout/stderr.
// Used for long-running interactive commands like uv sync.
func RunStreaming(dir string, args ...string) error {
	cmd := exec.Command(args[0], args[1:]...) //nolint:gosec
	if dir != "" {
		cmd.Dir = dir
	}
	setSysProcAttr(cmd)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	stopSignals := forwardSignals(cmd)
	err := cmd.Run()
	stopSignals()
	return err
}

// forwardSignals starts a goroutine that kills cmd on SIGINT/SIGTERM.
// The returned stop func must be called after cmd exits.
func forwardSignals(cmd *exec.Cmd) func() {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	done := make(chan struct{})
	go func() {
		select {
		case <-sigCh:
			killProcessGroup(cmd)
		case <-done:
		}
		signal.Stop(sigCh)
	}()
	return func() { close(done) }
}

// editedFileIsPython reads stdin JSON and checks if the edited file is a .py file.
// Returns true if the file is .py or if no file context is available (run manually).
func editedFileIsPython() bool {
	stdinCh := make(chan []byte, 1)
	go func() {
		data, _ := io.ReadAll(os.Stdin)
		stdinCh <- data
	}()

	var data []byte
	select {
	case data = <-stdinCh:
	case <-time.After(100 * time.Millisecond):
		return true
	}

	if len(data) == 0 {
		return true
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return true
	}

	path := findFilePath(raw)
	if path == "" {
		return true
	}

	return strings.HasSuffix(path, ".py")
}

// findFilePath searches common field names across different AI tool hook formats.
func findFilePath(m map[string]interface{}) string {
	for _, key := range []string{"file_path", "filePath", "CLAUDE_TOOL_OUTPUT_PATH"} {
		if v, ok := m[key]; ok {
			if s, ok := v.(string); ok && s != "" {
				return s
			}
		}
	}
	for _, key := range []string{"tool_input", "toolInput"} {
		if nested, ok := m[key].(map[string]interface{}); ok {
			for _, fkey := range []string{"file_path", "filePath", "path"} {
				if v, ok := nested[fkey]; ok {
					if s, ok := v.(string); ok && s != "" {
						return s
					}
				}
			}
		}
	}
	return ""
}

// cappedBuf is a thread-safe write buffer that keeps only the most recent maxBytes.
// Prevents runaway subprocess output from growing pyguard's own RSS without bound.
const defaultBufCap = 2 * 1024 * 1024 // 2 MB

type cappedBuf struct {
	mu  sync.Mutex
	b   []byte
	max int
}

func newCappedBuf() *cappedBuf { return &cappedBuf{max: defaultBufCap} }

func (c *cappedBuf) Write(p []byte) (int, error) {
	c.mu.Lock()
	c.b = append(c.b, p...)
	if len(c.b) > c.max {
		// Copy into a fresh allocation so the old backing array can be GC'd.
		c.b = append([]byte(nil), c.b[len(c.b)-c.max:]...)
	}
	c.mu.Unlock()
	return len(p), nil
}

// tail returns the last n lines. n <= 0 returns all captured output.
// Scans backward through the buffer to avoid splitting every line.
func (c *cappedBuf) tail(n int) string {
	c.mu.Lock()
	b := c.b
	c.mu.Unlock()
	if n <= 0 {
		return string(b)
	}
	nlFound := 0
	i := len(b) - 1
	if i >= 0 && b[i] == '\n' {
		i-- // trailing newline doesn't start a new line
	}
	for i >= 0 {
		if b[i] == '\n' {
			nlFound++
			if nlFound == n {
				return string(b[i+1:])
			}
		}
		i--
	}
	return string(b) // fewer than n lines in the buffer
}
