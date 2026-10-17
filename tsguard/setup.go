package main

import (
	"fmt"
	"runtime"
	"strings"
)

// runSetup bootstraps the full dev environment (idempotent).
func runSetup(root string, cfg config) int {
	if packagedRuntime() != "" {
		fmt.Println("tsguard setup — npm-owned toolchain")
		if code := runDoctor(root, cfg.pkgManager); code != 0 {
			return code
		}
		return runHooks(root)
	}
	fmt.Println("tsguard setup")
	fmt.Println("─────────")

	fmt.Printf("  [1/4] Node.js (%s)...\n", nodeEngineRange())
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

	fmt.Println("  [4/4] Opengrep SAST engine (verified cache)...")
	if !ensureOpengrep(root, cfg) {
		return 1
	}

	fmt.Println()
	runHooks(root)

	fmt.Println()
	fmt.Println("  Setup complete!")
	fmt.Println("  Run: tsguard doctor    (verify environment)")
	fmt.Println("  Run: tsguard check     (run quality gates)")
	return 0
}

// checkNode validates the Node versions supported by Ultracite 8 and the npm package.
func checkNode() bool {
	out, _, err := RunSilent("", "node", "--version")
	if err != nil {
		fmt.Println("  [FAIL] Node.js — not found")
		printNodeInstallHint()
		return false
	}
	version := strings.TrimSpace(out)
	if isSupportedNodeVersion(version) {
		fmt.Printf("  [OK]   Node.js %s\n", version)
		return true
	}
	fmt.Printf("  [FAIL] Node.js — found %s but need %s\n", version, nodeEngineRange())
	printNodeInstallHint()
	return false
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
