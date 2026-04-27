package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// runSetup bootstraps the full dev environment (idempotent).
func runSetup(root string) int {
	fmt.Println("tsguard setup")
	fmt.Println("─────────")

	// Step 1: Require Node.js >= 22
	fmt.Println("  [1/3] Node.js 22...")
	if !checkNode() {
		return 2
	}

	// Step 2: npm install
	fmt.Println("  [2/3] npm install...")
	if err := RunStreaming(root, "npm", "install"); err != nil {
		fmt.Printf("  [FAIL] npm install failed: %v\n", err)
		return 1
	}
	fmt.Println("  [OK]   node_modules ready")

	// Step 3: Create .secrets.baseline if missing
	baseline := filepath.Join(root, ".secrets.baseline")
	fmt.Print("  [3/3] .secrets.baseline... ")
	if _, err := os.Stat(baseline); os.IsNotExist(err) {
		fmt.Println("creating...")
		if _, _, err := RunSilent("", "detect-secrets", "--version"); err != nil {
			fmt.Println("  [SKIP] detect-secrets not installed (pip install detect-secrets)")
		} else {
			out, scanErr := RunCapture(root, "detect-secrets", "scan")
			if scanErr != nil {
				fmt.Println("  [FAIL] detect-secrets scan failed")
			} else {
				if writeErr := os.WriteFile(baseline, []byte(out), 0o644); writeErr != nil {
					fmt.Printf("  [FAIL] could not write baseline: %v\n", writeErr)
				} else {
					fmt.Println("  [OK]   .secrets.baseline created")
				}
			}
		}
	} else {
		fmt.Println("exists")
	}

	// Step 4: AI tool hooks
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
