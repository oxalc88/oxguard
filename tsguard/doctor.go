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
		{"biome", "@biomejs/biome"},
		{"ultracite", "ultracite"},
		{"fta-cli", "fta-cli"},
	} {
		out, _, err := RunSilent(root, pkgExec(pm, tool.cmd, "--version")...)
		if err != nil {
			fmt.Printf("  [FAIL] %s — not found. Run: tsguard setup\n", tool.name)
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
			fmt.Printf("  [SKIP] %s — not installed (optional, needed for tsguard audit)\n", tool.name)
		} else {
			version := strings.TrimSpace(strings.Split(out, "\n")[0])
			fmt.Printf("  [OK]   %s (%s)\n", tool.name, version)
		}
	}

	for _, tool := range []struct{ name, cmd, install string }{
		{"detect-secrets", "detect-secrets", "pip install detect-secrets"},
		{"semgrep", "semgrep", "pip install semgrep"},
	} {
		if out, _, err := RunSilent("", tool.cmd, "--version"); err != nil {
			fmt.Printf("  [SKIP] %s — not installed (optional, %s)\n", tool.name, tool.install)
		} else {
			fmt.Printf("  [OK]   %s (%s)\n", tool.name, strings.TrimSpace(out))
		}
	}

	// .secrets.baseline
	baseline := filepath.Join(root, ".secrets.baseline")
	if _, err := os.Stat(baseline); os.IsNotExist(err) {
		fmt.Println("  [FAIL] .secrets.baseline — not found. Run: tsguard secrets --init")
		failures++
	} else {
		fmt.Println("  [OK]   .secrets.baseline")
	}

	fmt.Println()
	if failures == 0 {
		fmt.Println("  All checks passed.")
	} else {
		fmt.Printf("  %d issue(s) found. Run tsguard setup to fix.\n", failures)
		return 1
	}
	return 0
}
