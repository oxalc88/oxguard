package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// runDoctor checks the full toolchain and reports status without making changes.
func runDoctor(root string) int {
	fmt.Println("pkn doctor")
	fmt.Println("──────────")

	failures := 0

	// Python >= 3.13
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
		fmt.Println("  [FAIL] .venv — not found. Run: pkn setup")
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
		fmt.Println("  [FAIL] .secrets.baseline — not found. Run: pkn secrets --init")
		failures++
	} else {
		fmt.Println("  [OK]   .secrets.baseline")
	}

	// AWS CLI (optional, for pkn invoke)
	out, _, err := RunSilent("", "aws", "--version")
	if err != nil {
		fmt.Println("  [SKIP] aws cli — not found (optional, needed for pkn invoke)")
	} else {
		version := strings.TrimSpace(strings.Split(out, "\n")[0])
		fmt.Printf("  [OK]   aws cli (%s)\n", version)
	}

	fmt.Println()
	if failures == 0 {
		fmt.Println("  All checks passed.")
	} else {
		fmt.Printf("  %d issue(s) found. Run pkn setup to fix.\n", failures)
		return 1
	}
	return 0
}

// checkPython verifies Python >= 3.13 is available.
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
		if isPython313OrNewer(version) {
			fmt.Printf("  [OK]   %s\n", version)
			return true
		}
		fmt.Printf("  [FAIL] Python — found %s but need >= 3.13\n", version)
		printPythonInstallHint()
		return false
	}

	fmt.Println("  [FAIL] Python 3.13 — not found")
	printPythonInstallHint()
	return false
}

func checkUV() bool {
	out, _, err := RunSilent("", "uv", "--version")
	if err != nil {
		fmt.Println("  [FAIL] uv — not found. Run: pkn setup")
		return false
	}
	fmt.Printf("  [OK]   uv (%s)\n", strings.TrimSpace(out))
	return true
}

// isPython313OrNewer parses "Python X.Y.Z" and checks >= 3.13.
func isPython313OrNewer(version string) bool {
	var major, minor int
	_, err := fmt.Sscanf(version, "Python %d.%d", &major, &minor)
	if err != nil {
		return false
	}
	return major > 3 || (major == 3 && minor >= 13)
}

func printPythonInstallHint() {
	switch runtime.GOOS {
	case "windows":
		fmt.Println("         Install: winget install Python.Python.3.13")
		fmt.Println("         Or download from: https://python.org/downloads/")
	case "darwin":
		fmt.Println("         Install: brew install python@3.13")
	default:
		fmt.Println("         Install: sudo apt install python3.13")
		fmt.Println("         Or see: https://python.org/downloads/")
	}
}
