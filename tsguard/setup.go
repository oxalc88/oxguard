package main

import (
	"fmt"
	"runtime"
	"strings"
)

// runSetup bootstraps the full dev environment (idempotent).
func runSetup(root string, cfg config) int {
	fmt.Println("tsguard setup")
	fmt.Println("─────────")

	fmt.Println("  [1/4] Node.js (22+)...")
	if !checkNode() {
		return 2
	}

	// Reconcile before npm install so a single sync covers newly added deps.
	fmt.Println("  [2/4] devDependency manifest...")
	if err := ensureNpmDevDeps(root, cfg); err != nil {
		fmt.Printf("  [FAIL] devDependency check failed: %v\n", err)
		return 1
	}

	fmt.Printf("  [3/4] %s install...\n", cfg.pkgManager)
	if err := RunStreaming(root, cfg.pkgManager, "install"); err != nil {
		fmt.Printf("  [FAIL] %s install failed: %v\n", cfg.pkgManager, err)
		return 1
	}
	fmt.Println("  [OK]   node_modules ready")

	fmt.Println("  [4/4] Opengrep SAST engine (project-local)...")
	ensureOpengrep(root, cfg)

	fmt.Println()
	runHooks(root)

	fmt.Println()
	fmt.Println("  Setup complete!")
	fmt.Println("  Run: tsguard doctor    (verify environment)")
	fmt.Println("  Run: tsguard check     (run quality gates)")
	return 0
}

// checkNode verifies Node.js >= 22 is available.
func checkNode() bool {
	out, _, err := RunSilent("", "node", "--version")
	if err != nil {
		fmt.Println("  [FAIL] Node.js — not found")
		printNodeInstallHint()
		return false
	}
	version := strings.TrimSpace(out)
	if isNode22OrNewer(version) {
		fmt.Printf("  [OK]   Node.js %s\n", version)
		return true
	}
	fmt.Printf("  [FAIL] Node.js — found %s but need >= 22\n", version)
	printNodeInstallHint()
	return false
}

// isNode22OrNewer parses "vX.Y.Z" and checks major >= 22.
func isNode22OrNewer(version string) bool {
	var major int
	_, err := fmt.Sscanf(version, "v%d", &major)
	if err != nil {
		return false
	}
	return major >= 22
}

func printNodeInstallHint() {
	switch runtime.GOOS {
	case "windows":
		fmt.Println("         Install: winget install OpenJS.NodeJS.LTS")
	case "darwin":
		fmt.Println("         Install: brew install node@22")
	default:
		fmt.Println("         Install: nvm install 22  or  sudo apt install nodejs")
	}
}
