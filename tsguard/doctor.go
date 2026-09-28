package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// runDoctor checks the full toolchain and reports status without making changes.
func runDoctor(root, pm string) int {
	fmt.Println("tsguard doctor")
	fmt.Println("──────────")

	failures := 0
	location, repair := "project-local", "Run: tsguard setup"
	if packagedRuntime() != "" {
		location, repair = "bundled", "Reinstall @oxguard/tsguard with optional dependencies enabled"
	}

	// Node.js >= 22
	if !checkNode() {
		failures++
	}

	// package manager
	if out, _, err := RunSilent("", pm, "--version"); err != nil {
		fmt.Printf("  [FAIL] %s — not found\n", pm)
		failures++
	} else {
		fmt.Printf("  [OK]   %s (%s)\n", pm, strings.TrimSpace(out))
	}

	// node_modules
	nodeModules := filepath.Join(root, "node_modules")
	if _, err := os.Stat(nodeModules); os.IsNotExist(err) {
		fmt.Println("  [FAIL] node_modules — not found. Run: tsguard setup")
		failures++
	} else {
		fmt.Println("  [OK]   node_modules")
	}

	// Individual tools via package manager exec
	for _, tool := range []struct{ name, cmd string }{
		{"tsc", "tsc"},
		{"vitest", "vitest"},
		{lintDoctorTool(), lintDoctorTool()},
		{"fta", "fta"},
	} {
		out, _, err := RunSilent(root, pkgExec(pm, tool.cmd, "--version")...)
		if err != nil {
			fmt.Printf("  [FAIL] %s — not found. %s\n", tool.name, repair)
			failures++
		} else {
			version := strings.TrimSpace(strings.Split(out, "\n")[0])
			fmt.Printf("  [OK]   %s (%s)\n", tool.name, version)
		}
	}

	// Optional tools (informational only)
	for _, tool := range []struct{ name, cmd string }{
		{"knip", "knip"},
		{"jscpd", "jscpd"},
	} {
		out, _, err := RunSilent(root, pkgExec(pm, tool.cmd, "--version")...)
		if err != nil {
			if packagedRuntime() != "" {
				fmt.Printf("  [FAIL] %s — missing from packaged toolchain; reinstall @oxguard/tsguard\n", tool.name)
				failures++
			} else {
				fmt.Printf("  [SKIP] %s — not installed (optional, needed for tsguard audit)\n", tool.name)
			}
		} else {
			version := strings.TrimSpace(strings.Split(out, "\n")[0])
			fmt.Printf("  [OK]   %s (%s)\n", tool.name, version)
		}
	}
	if packagedRuntime() != "" {
		if _, _, err := RunSilent(root, packagedCommand("audit-ci", "--version")...); err != nil {
			fmt.Println("  [FAIL] audit-ci — missing from packaged toolchain")
			failures++
		}
		for _, module := range []string{"@vitest/coverage-v8", "@secretlint/secretlint-rule-preset-recommend", "ultracite/biome/core"} {
			if _, err := runtimeModule(root, module); err != nil {
				fmt.Printf("  [FAIL] %s — missing from packaged toolchain\n", module)
				failures++
			}
		}
	}

	// Opengrep project-local SAST binary (required for security gate).
	opengrepBin := opengrepBinaryPath(root)
	if out, _, err := RunSilent("", opengrepBin, "--version"); err != nil {
		fmt.Printf("  [FAIL] opengrep — not found (%s). %s\n", location, repair)
		failures++
	} else {
		fmt.Printf("  [OK]   opengrep %s (%s)\n", strings.TrimSpace(strings.Split(out, "\n")[0]), location)
	}

	// secretlint (npm dev-dep, required for secrets gate).
	if out, _, err := RunSilent(root, pkgExec(pm, "secretlint", "--version")...); err != nil {
		fmt.Printf("  [FAIL] secretlint — not found. %s\n", repair)
		failures++
	} else {
		fmt.Printf("  [OK]   secretlint (%s)\n", strings.TrimSpace(strings.Split(out, "\n")[0]))
	}

	fmt.Println()
	if failures == 0 {
		fmt.Println("  All checks passed.")
	} else {
		fmt.Printf("  %d issue(s) found. %s\n", failures, repair)
		return 1
	}
	return 0
}

func lintDoctorTool() string {
	if packagedRuntime() != "" {
		return "biome"
	}
	return "ultracite"
}
