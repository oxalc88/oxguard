package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Runner executes subprocesses from the project root.
type Runner struct {
	root      string
	timeout   int    // seconds per tool
	logFile   string // path to append full output (empty = no log file)
	tailLines int    // print only the last N lines to stdout (0 = all)
}

// Result holds the outcome of a single tool run.
type Result struct {
	name   string
	ok     bool
	output string // captured output (may be truncated to tailLines for display)
}

// Run executes a command, buffers output, and returns the result.
// On success: prints "[OK] name". On failure: prints "[FAIL] name" + output.
// Output is bounded to 2 MB in memory; --tail N caps the displayed lines.
func (r *Runner) Run(name string, args ...string) Result {
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
		}
	}

	cmd.Stdout = io.MultiWriter(writers...)
	cmd.Stderr = cmd.Stdout

	stopSignals := forwardSignals(cmd)

	err := cmd.Start()
	if err != nil {
		stopSignals()
		return Result{name: name, ok: false, output: fmt.Sprintf("failed to start: %v", err)}
	}

	timer := time.AfterFunc(time.Duration(r.timeout)*time.Second, func() {
		killProcessGroup(cmd)
	})

	err = cmd.Wait()
	timer.Stop()
	stopSignals()

	output := cbuf.tail(r.tailLines)
	ok := err == nil

	if ok {
		fmt.Printf("  [OK]   %s\n", name)
	} else if r.logFile != "" {
		fmt.Printf("  [FAIL] %s (see %s)\n", name, r.logFile)
	} else {
		fmt.Printf("  [FAIL] %s\n", name)
		if output != "" {
			for _, line := range strings.Split(strings.TrimRight(output, "\n"), "\n") {
				fmt.Printf("         %s\n", line)
			}
		}
	}

	return Result{name: name, ok: ok, output: output}
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
// Prevents runaway subprocess output from growing pkn's own RSS without bound.
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
