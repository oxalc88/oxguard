package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

// runDoctor checks the full toolchain and reports status without making changes.
func runDoctor(root string) int {
	fmt.Println("pyguard doctor")
	fmt.Println("──────────")

	failures := 0

	// Python >= 3.12 (3.14.1 excluded)
	if !checkPython() {
		failures++
	}

	// uv
	if !checkUV() {
		failures++
	}

	// .venv
	venv := filepath.Join(root, ".venv")
	if _, err := os.Stat(venv); os.IsNotExist(err) {
		fmt.Println("  [FAIL] .venv — not found. Run: pyguard setup")
		failures++
	} else {
		fmt.Println("  [OK]   .venv")
	}

	// Individual tools via uv run
	for _, tool := range []struct{ name, cmd string }{
		{"ruff", "ruff"},
		{"mypy", "mypy"},
		{"pytest", "pytest"},
		{"bandit", "bandit"},
		{"radon", "radon"},
	} {
		out, _, err := RunSilent(root, "uv", "run", tool.cmd, "--version")
		if err != nil {
			fmt.Printf("  [FAIL] %s — not found. Run: uv sync\n", tool.name)
			failures++
		} else {
			version := strings.TrimSpace(strings.Split(out, "\n")[0])
			fmt.Printf("  [OK]   %s (%s)\n", tool.name, version)
		}
	}

	// .secrets.baseline
	baseline := filepath.Join(root, ".secrets.baseline")
	if _, err := os.Stat(baseline); os.IsNotExist(err) {
		fmt.Println("  [FAIL] .secrets.baseline — not found. Run: pyguard secrets --init")
		failures++
	} else {
		fmt.Println("  [OK]   .secrets.baseline")
	}

	// AWS CLI (optional, for pyguard invoke)
	out, _, err := RunSilent("", "aws", "--version")
	if err != nil {
		fmt.Println("  [SKIP] aws cli — not found (optional, needed for pyguard invoke)")
	} else {
		version := strings.TrimSpace(strings.Split(out, "\n")[0])
		fmt.Printf("  [OK]   aws cli (%s)\n", version)
	}

	fmt.Println()
	if failures == 0 {
		fmt.Println("  All checks passed.")
	} else {
		fmt.Printf("  %d issue(s) found. Run pyguard setup to fix.\n", failures)
		return 1
	}
	return 0
}

// checkPython verifies Python >= 3.12 is available (3.14.1 excluded).
func checkPython() bool {
	candidates := []string{"python3", "python"}
	if runtime.GOOS == "windows" {
		candidates = []string{"python", "python3"}
	}

	for _, cmd := range candidates {
		out, _, err := RunSilent("", cmd, "--version")
		if err != nil {
			continue
		}
		version := strings.TrimSpace(out)
		if isSupportedPythonVersion(version) {
			fmt.Printf("  [OK]   %s\n", version)
			return true
		}
		fmt.Printf("  [FAIL] Python — found %s but need >= 3.12 (not 3.14.1)\n", version)
		printPythonInstallHint()
		return false
	}

	fmt.Println("  [FAIL] Python 3.12 — not found")
	printPythonInstallHint()
	return false
}

func checkUV() bool {
	out, _, err := RunSilent("", "uv", "--version")
	if err != nil {
		fmt.Println("  [FAIL] uv — not found. Run: pyguard setup")
		return false
	}
	fmt.Printf("  [OK]   uv (%s)\n", strings.TrimSpace(out))
	return true
}

// isSupportedPythonVersion parses "Python X.Y.Z" and accepts >= 3.12,
// rejecting exactly 3.14.1 (networkx 3.7 supports >= 3.12 but excludes 3.14.1).
// Unparseable output is rejected.
func isSupportedPythonVersion(version string) bool {
	major, minor, patch, ok := parsePythonVersion(version)
	if !ok {
		return false
	}
	if major != 3 {
		return major > 3
	}
	if minor < 12 {
		return false
	}
	return !(minor == 14 && patch == 1)
}

// parsePythonVersion reads the leading "Python M.m.p" of a version banner.
// The patch component is optional and defaults to 0.
func parsePythonVersion(version string) (major, minor, patch int, ok bool) {
	rest, found := strings.CutPrefix(strings.TrimSpace(version), "Python ")
	if !found {
		return 0, 0, 0, false
	}
	fields := strings.Fields(rest)
	if len(fields) == 0 {
		return 0, 0, 0, false
	}
	parts := strings.Split(fields[0], ".")
	if len(parts) < 2 || len(parts) > 3 {
		return 0, 0, 0, false
	}
	values := make([]int, 3)
	for i, part := range parts {
		value, err := strconv.Atoi(part)
		if err != nil || value < 0 {
			return 0, 0, 0, false
		}
		values[i] = value
	}
	return values[0], values[1], values[2], true
}

func printPythonInstallHint() {
	switch runtime.GOOS {
	case "windows":
		fmt.Println("         Install: winget install Python.Python.3.12")
		fmt.Println("         Or download from: https://python.org/downloads/")
	case "darwin":
		fmt.Println("         Install: brew install python@3.12")
	default:
		fmt.Println("         Install: sudo apt install python3.12")
		fmt.Println("         Or see: https://python.org/downloads/")
	}
}
