package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/term"
)

// requiredNpmDevDeps is the manifest of npm packages tsguard invokes.
var requiredNpmDevDeps = []string{
	"ultracite",
	"@biomejs/biome",
	"typescript",
	"vitest",
	"@vitest/coverage-v8",
	"fta-cli",
	"knip",
	"jscpd",
}

// pythonHelperTools are installed system-wide via uv tool / pipx, not npm.
var pythonHelperTools = []string{"semgrep", "detect-secrets"}

// missingNpmDevDeps reads package.json and returns deps from manifest that are
// absent from both devDependencies and dependencies. Returns nil if package.json
// is not found (not a TS project yet — let npm install fail naturally).
func missingNpmDevDeps(root string, manifest []string) ([]string, error) {
	data, err := os.ReadFile(filepath.Join(root, "package.json"))
	if err != nil {
		return nil, nil
	}

	var pkg struct {
		Dependencies    map[string]json.RawMessage `json:"dependencies"`
		DevDependencies map[string]json.RawMessage `json:"devDependencies"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
		return nil, fmt.Errorf("parsing package.json: %w", err)
	}

	var missing []string
	for _, dep := range manifest {
		_, inDev := pkg.DevDependencies[dep]
		_, inDeps := pkg.Dependencies[dep]
		if !inDev && !inDeps {
			missing = append(missing, dep)
		}
	}
	return missing, nil
}

// ensureNpmDevDeps checks for missing devDependencies, prompts if any are absent,
// and installs them using the project's package manager.
func ensureNpmDevDeps(root string, cfg config) error {
	missing, err := missingNpmDevDeps(root, requiredNpmDevDeps)
	if err != nil {
		return err
	}
	if len(missing) == 0 {
		fmt.Println("  [OK]   all required devDependencies present")
		return nil
	}

	fmt.Println("  Missing devDependencies:")
	for _, dep := range missing {
		fmt.Printf("    - %s\n", dep)
	}

	if !confirmYesNo("  Add them to package.json now?", true, cfg.assumeYes) {
		fmt.Println("  [SKIP] devDependencies unchanged — run tsguard setup again to add later")
		return nil
	}

	var installArgs []string
	switch cfg.pkgManager {
	case "pnpm", "yarn":
		installArgs = append([]string{cfg.pkgManager, "add", "-D"}, missing...)
	default:
		installArgs = append([]string{"npm", "install", "--save-dev"}, missing...)
	}
	if err := RunStreaming(root, installArgs...); err != nil {
		return fmt.Errorf("%s add devDependencies: %w", cfg.pkgManager, err)
	}
	fmt.Println("  [OK]   devDependencies added")
	return nil
}

// ensurePythonHelperTools installs semgrep and detect-secrets system-wide via
// uv tool install → pipx install → install hint. Never blocks setup.
func ensurePythonHelperTools(cfg config) {
	for _, tool := range pythonHelperTools {
		if toolAvailable(tool) {
			fmt.Printf("  [OK]   %s\n", tool)
			continue
		}
		if !confirmYesNo(fmt.Sprintf("  Install %s (uv tool install)?", tool), true, cfg.assumeYes) {
			fmt.Printf("  [SKIP] %s — install manually: uv tool install %s\n", tool, tool)
			continue
		}
		ensureSinglePythonTool(tool)
	}
}

// ensureSinglePythonTool ensures the tool is available, installing it if missing.
// Returns true if available after the attempt. Runtime gates call this directly
// so a drifted environment self-heals instead of silently passing the security gate.
func ensureSinglePythonTool(tool string) bool {
	if toolAvailable(tool) {
		return true
	}
	uvOk := toolAvailable("uv")
	pipxOk := toolAvailable("pipx")
	if !uvOk && !pipxOk {
		fmt.Printf("  [SKIP] %s — install manually: uv tool install %s\n", tool, tool)
		return false
	}
	fmt.Printf("  [..]   %s — installing...\n", tool)
	if uvOk {
		if err := RunStreaming("", "uv", "tool", "install", tool); err == nil {
			fmt.Printf("  [OK]   %s (via uv)\n", tool)
			return true
		}
	}
	if pipxOk {
		if err := RunStreaming("", "pipx", "install", tool); err == nil {
			fmt.Printf("  [OK]   %s (via pipx)\n", tool)
			return true
		}
	}
	fmt.Printf("  [SKIP] %s — install manually:\n         uv tool install %s   # or: pipx install %s\n", tool, tool, tool)
	return false
}

// confirmYesNo prints a y/n prompt and returns the user's choice.
// Returns defaultYes immediately when assumeYes is set, CI=true, or stdin is not a TTY.
func confirmYesNo(prompt string, defaultYes bool, assumeYes bool) bool {
	if assumeYes || isCI() || !term.IsTerminal(int(os.Stdin.Fd())) {
		hint := "y"
		if !defaultYes {
			hint = "n"
		}
		fmt.Printf("%s [%s] (auto)\n", prompt, hint)
		return defaultYes
	}

	suffix := "[Y/n]"
	if !defaultYes {
		suffix = "[y/N]"
	}
	fmt.Printf("%s %s ", prompt, suffix)

	scanner := bufio.NewScanner(os.Stdin)
	scanner.Scan()
	input := strings.ToLower(strings.TrimSpace(scanner.Text()))

	switch input {
	case "y", "yes":
		return true
	case "n", "no":
		return false
	default:
		return defaultYes
	}
}

// isCI returns true when running in a known CI environment.
func isCI() bool {
	return os.Getenv("CI") == "true" || os.Getenv("CI") == "1"
}

// detectPackageManager returns "pnpm", "yarn", or "npm" by inspecting the project root.
// Resolution order: package.json "packageManager" field → lockfile → default npm.
func detectPackageManager(root string) string {
	if data, err := os.ReadFile(filepath.Join(root, "package.json")); err == nil {
		var pkg struct {
			PackageManager string `json:"packageManager"`
		}
		if json.Unmarshal(data, &pkg) == nil && pkg.PackageManager != "" {
			name := strings.SplitN(pkg.PackageManager, "@", 2)[0]
			switch name {
			case "pnpm", "yarn":
				return name
			}
		}
	}
	for _, pair := range [][2]string{
		{"pnpm-lock.yaml", "pnpm"},
		{"yarn.lock", "yarn"},
		{"package-lock.json", "npm"},
	} {
		if _, err := os.Stat(filepath.Join(root, pair[0])); err == nil {
			return pair[1]
		}
	}
	return "npm"
}

// pkgExec returns a command slice for running a locally-installed tool with the
// project's package manager (npx for npm, pnpm exec for pnpm, yarn exec for yarn).
func pkgExec(pm string, args ...string) []string {
	switch pm {
	case "pnpm":
		return append([]string{"pnpm", "exec"}, args...)
	case "yarn":
		return append([]string{"yarn", "exec"}, args...)
	default:
		return append([]string{"npx"}, args...)
	}
}
