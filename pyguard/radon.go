package main

import (
	"fmt"
	"strings"
)

// runRadon runs complexity checks and fails if any function has CC > 10 (grade C or
// worse), any file has MI below grade B, or any function exceeds Halstead thresholds.
func runRadon(r *Runner, dirs []string) int {
	fmt.Println("  radon:")

	// Informational display: CC averages, MI, Halstead raw numbers
	args := append([]string{"uv", "run", "radon", "cc", "--total-average"}, dirs...)
	r.Run("radon cc avg", args...)

	args = append([]string{"uv", "run", "radon", "mi"}, dirs...)
	r.Run("radon mi", args...)

	args = append([]string{"uv", "run", "radon", "hal"}, dirs...)
	r.Run("radon hal", args...)

	if code := runRadonEnforce(r, dirs, "cc", "C", "functions exceeding CC > 10", "all functions CC <= 10"); code != 0 {
		return code
	}
	if code := runRadonEnforce(r, dirs, "mi", "B", "files below maintainability grade B", "all files MI >= grade B"); code != 0 {
		return code
	}

	halArgs := append([]string{"uv", "run", "python", "tools/analysis/check_halstead.py"}, dirs...)
	if code := r.Run("halstead", halArgs...); !code.ok {
		return 1
	}
	return 0
}

// runRadonEnforce runs `radon <subCmd> -n <grade>` and fails if stdout has any output.
// Uses RunSilent so uv's VIRTUAL_ENV stderr warning doesn't cause false-fails.
func runRadonEnforce(r *Runner, dirs []string, subCmd, grade, failMsg, okMsg string) int {
	args := append([]string{"uv", "run", "radon", subCmd, "-n", grade}, dirs...)
	stdout, _, err := RunSilent(r.root, args...)
	trimmed := strings.TrimSpace(stdout)
	if err != nil || trimmed != "" {
		fmt.Printf("  [FAIL] radon — %s:\n", failMsg)
		for _, line := range strings.Split(trimmed, "\n") {
			if line != "" {
				fmt.Printf("         %s\n", line)
			}
		}
		return 1
	}
	fmt.Printf("  [OK]   radon — %s\n", okMsg)
	return 0
}
