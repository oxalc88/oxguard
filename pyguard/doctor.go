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
// Every component takes its numeric prefix, so real banners such as
// "3.12.15+", "3.13.0rc1" and "3.14.0b2" parse; only the last component may
// carry such a suffix, and a component without any digit is garbage.
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
		value, suffix, valid := splitVersionComponent(part)
		if !valid || (suffix != "" && i != len(parts)-1) {
			return 0, 0, 0, false
		}
		values[i] = value
	}
	return values[0], values[1], values[2], true
}

// splitVersionComponent splits one dot-separated component into its leading
// digits and the non-digit tail that follows them ("15+" → 15, "+";
// "0rc1" → 0, "rc1"). It reports false when the component has no leading
// digit, so garbage such as "x", "rc1" or "." never parses.
func splitVersionComponent(component string) (value int, suffix string, ok bool) {
	end := 0
	for end < len(component) && component[end] >= '0' && component[end] <= '9' {
		end++
	}
	if end == 0 {
		return 0, "", false
	}
	value, err := strconv.Atoi(component[:end])
	if err != nil {
		return 0, "", false
	}
	return value, component[end:], true
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
