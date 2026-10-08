package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

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
	// Security: secrets + CVE gates (no Python required).
	"secretlint",
	"@secretlint/secretlint-rule-preset-recommend",
	"audit-ci",
}

// Opengrep is pinned by upstream asset hash and stored in a user-owned cache
// outside the project being scanned; project files cannot replace its bytes.
const (
	opengrepVersion  = "v1.23.0"
	opengrepCacheDir = "node_modules/.cache/oxguard"
	opengrepBinary   = "opengrep"
)

// opengrepBinaryPath keeps standalone executable assets outside the scanned root.
// The npm-delivered binary remains controlled by the installed package.
func opengrepBinaryPath(root string) string {
	if packagedRuntime() != "" && os.Getenv("TSGUARD_OPENGREP") != "" {
		return os.Getenv("TSGUARD_OPENGREP")
	}
	name := opengrepBinary
	if runtime.GOOS == "windows" { name += ".exe" }
	cache, err := os.UserCacheDir()
	if err != nil { return "" }
	return filepath.Join(cache, "oxguard", "opengrep", opengrepVersion, runtime.GOOS+"-"+runtime.GOARCH, name)
}

// Digests are the same pinned values used by npm/native-tools.json for v1.23.0.
var opengrepDigests = map[string]string{
	"linux/amd64":   "1f06548af379ab6080698a609612890ffad2d92dc2172f1e97d38d48096d5ef8",
	"linux/arm64":   "ddf4935b138a2e825e6860529df1fb031524f7a2da8933ab7b2a16e5939c5178",
	"darwin/amd64":  "2fa99169e34e9f233fd7412d5cdc167114ba752a2510da3ce322b272c15ab755",
	"darwin/arm64":  "945739e56fec4aab28da296811f3473a0ff733af3dc2371d150b071430265c5f",
	"windows/amd64": "7cbe02d9cc3fb812adce8720fc46515bb6a954a4ceeb3856d3a099d783749aae",
}

func verifyOpengrepBinary(path string) error {
	if path == "" { return errors.New("user cache directory unavailable") }
	info, err := os.Lstat(path)
	if err != nil { return err }
	if !info.Mode().IsRegular() { return errors.New("Opengrep cache entry is not a regular file") }
	want, ok := opengrepDigests[runtime.GOOS+"/"+runtime.GOARCH]
	if !ok { return errors.New("platform has no pinned Opengrep digest") }
	f, err := os.Open(path)
	if err != nil { return err }
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil { return err }
	if hex.EncodeToString(h.Sum(nil)) != want {
		return errors.New("Opengrep SHA-256 mismatch")
	}
	return nil
}

// opengrepAssetName returns the Opengrep release asset filename for this platform.
// Asset naming: opengrep_<os>_<arch>[.exe]
// Linux: manylinux (glibc, standard distros) for amd64; musllinux for containers.
// Returns ("", false) for unsupported platforms.
func opengrepAssetName() (string, bool) {
	switch runtime.GOOS {
	case "linux":
		switch runtime.GOARCH {
		case "amd64":
			return "opengrep_manylinux_x86", true
		case "arm64":
			return "opengrep_manylinux_aarch64", true
		}
	case "darwin":
		switch runtime.GOARCH {
		case "amd64":
			return "opengrep_osx_x86", true
		case "arm64":
			return "opengrep_osx_arm64", true
		}
	case "windows":
		if runtime.GOARCH == "amd64" {
			return "opengrep_windows_x86.exe", true
		}
	}
	return "", false
}

// ensureOpengrep downloads only pinned assets to a private user cache.
func ensureOpengrep(root string, cfg config) bool {
	binaryPath := opengrepBinaryPath(root)
	if err := verifyOpengrepBinary(binaryPath); err == nil {
		fmt.Printf("  [OK]   opengrep %s (verified user cache)\n", opengrepVersion)
		return true
	} else if binaryPath == "" {
		fmt.Printf("  [FAIL] opengrep — cache unavailable: %v\n", err)
		return false
	}

	assetName, ok := opengrepAssetName()
	if !ok {
		fmt.Printf("  [FAIL] opengrep — unsupported platform %s/%s\n", runtime.GOOS, runtime.GOARCH)
		return false
	}
	if !confirmYesNo("  Download verified Opengrep SAST engine (~50 MB)?", true, cfg.assumeYes) {
		fmt.Println("  [SKIP] opengrep — required SAST engine is not installed")
		return false
	}
	cacheDir := filepath.Dir(binaryPath)
	if err := os.MkdirAll(cacheDir, 0o700); err != nil {
		fmt.Printf("  [FAIL] opengrep — cache directory: %v\n", err)
		return false
	}
	// Reject linked cache directories; do not write through attacker redirects.
	if info, err := os.Lstat(cacheDir); err != nil || !info.IsDir() {
		fmt.Println("  [FAIL] opengrep — cache directory is not a regular directory")
		return false
	}
	tmp, err := os.CreateTemp(cacheDir, "opengrep-download-*")
	if err != nil { fmt.Printf("  [FAIL] opengrep — cache creation: %v\n", err); return false }
	tmpPath := tmp.Name()
	_ = tmp.Close()
	defer os.Remove(tmpPath)
	baseURL := fmt.Sprintf("https://github.com/opengrep/opengrep/releases/download/%s", opengrepVersion)
	if err := downloadFile(tmpPath, baseURL+"/"+assetName); err != nil {
		fmt.Printf("  [FAIL] opengrep — download: %v\n", err)
		return false
	}
	if err := verifyOpengrepBinary(tmpPath); err != nil {
		fmt.Printf("  [FAIL] opengrep — checksum: %v\n", err)
		return false
	}
	if err := os.Chmod(tmpPath, 0o700); err != nil {
		fmt.Printf("  [FAIL] opengrep — chmod: %v\n", err)
		return false
	}
	if err := os.Rename(tmpPath, binaryPath); err != nil {
		fmt.Printf("  [FAIL] opengrep — install: %v\n", err)
		return false
	}
	fmt.Printf("  [OK]   opengrep %s installed (SHA-256 verified)\n", opengrepVersion)
	return true
}

// downloadFile downloads url to destPath via HTTP GET with a 3-minute timeout.
func downloadFile(destPath, url string) error {
	client := &http.Client{Timeout: 3 * time.Minute}
	resp, err := client.Get(url) //nolint:gosec,noctx
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d from %s", resp.StatusCode, url)
	}
	f, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, resp.Body)
	return err
}

// addToGitignore appends line to .gitignore if it is not already present.
func addToGitignore(root, line string) {
	gitignorePath := filepath.Join(root, ".gitignore")
	data, _ := os.ReadFile(gitignorePath)
	for _, existing := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(existing) == strings.TrimSpace(line) {
			return
		}
	}
	f, err := os.OpenFile(gitignorePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	prefix := ""
	if len(data) > 0 && data[len(data)-1] != '\n' {
		prefix = "\n"
	}
	_, _ = fmt.Fprintf(f, "%s%s\n", prefix, line)
}

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

// testRunnerPriority defines known JS/TS test runners checked against package.json deps.
var testRunnerPriority = []struct {
	name string
	deps []string
}{
	{"vitest", []string{"vitest"}},
	{"jest", []string{"jest", "ts-jest", "@jest/core", "babel-jest"}},
	{"mocha", []string{"mocha"}},
	{"ava", []string{"ava"}},
	{"jasmine", []string{"jasmine"}},
}

// detectTestRunner inspects package.json dependencies then config files to identify
// the project's test runner. Returns the runner name or "" if none is recognised.
func detectTestRunner(root string) string {
	data, err := os.ReadFile(filepath.Join(root, "package.json"))
	if err == nil {
		var pkg struct {
			Dependencies    map[string]json.RawMessage `json:"dependencies"`
			DevDependencies map[string]json.RawMessage `json:"devDependencies"`
		}
		if json.Unmarshal(data, &pkg) == nil {
			allDeps := make(map[string]bool, len(pkg.Dependencies)+len(pkg.DevDependencies))
			for k := range pkg.Dependencies {
				allDeps[k] = true
			}
			for k := range pkg.DevDependencies {
				allDeps[k] = true
			}
			for _, runner := range testRunnerPriority {
				for _, dep := range runner.deps {
					if allDeps[dep] {
						return runner.name
					}
				}
			}
		}
	}

	// Fallback: config file presence when runner isn't listed as a dep.
	for _, pair := range [][2]string{
		{"vitest.config.ts", "vitest"},
		{"vitest.config.js", "vitest"},
		{"vitest.config.mjs", "vitest"},
		{"jest.config.ts", "jest"},
		{"jest.config.js", "jest"},
		{"jest.config.json", "jest"},
	} {
		if _, err := os.Stat(filepath.Join(root, pair[0])); err == nil {
			return pair[1]
		}
	}

	return ""
}

// installedVersion reads the "version" field from node_modules/<pkg>/package.json.
// Returns "" if the package is not installed or the file cannot be parsed.
func installedVersion(root, pkg string) string {
	data, err := os.ReadFile(filepath.Join(root, "node_modules", pkg, "package.json"))
	if err != nil {
		return ""
	}
	var p struct {
		Version string `json:"version"`
	}
	if json.Unmarshal(data, &p) != nil {
		return ""
	}
	return p.Version
}

// majorVersion returns the major component of a semver string (e.g. "3.2.6" → "3").
func majorVersion(v string) string {
	if idx := strings.Index(v, "."); idx != -1 {
		return v[:idx]
	}
	return v
}

// checkVitestVersionMatch verifies that vitest and @vitest/coverage-v8 share the same
// major version. Returns an error with both versions when they differ.
func checkVitestVersionMatch(root string) error {
	vv := installedVersion(root, "vitest")
	cv := installedVersion(root, "@vitest/coverage-v8")
	if vv == "" || cv == "" {
		return nil // one or both not installed — let vitest surface its own error
	}
	if majorVersion(vv) != majorVersion(cv) {
		return fmt.Errorf("vitest@%s and @vitest/coverage-v8@%s major versions differ — align them in package.json", vv, cv)
	}
	return nil
}

// detectCoverageWrapper returns "c8", "nyc", or "" by checking package.json deps.
// c8 is preferred over nyc (more modern, native V8 coverage).
func detectCoverageWrapper(root string) string {
	data, err := os.ReadFile(filepath.Join(root, "package.json"))
	if err != nil {
		return ""
	}
	var pkg struct {
		Dependencies    map[string]json.RawMessage `json:"dependencies"`
		DevDependencies map[string]json.RawMessage `json:"devDependencies"`
	}
	if json.Unmarshal(data, &pkg) != nil {
		return ""
	}
	allDeps := make(map[string]bool, len(pkg.Dependencies)+len(pkg.DevDependencies))
	for k := range pkg.Dependencies {
		allDeps[k] = true
	}
	for k := range pkg.DevDependencies {
		allDeps[k] = true
	}
	if allDeps["c8"] {
		return "c8"
	}
	if allDeps["nyc"] {
		return "nyc"
	}
	return ""
}

// detectPackageManager returns "pnpm", "yarn", or "npm" by inspecting the project root.
// Resolution order: package.json "packageManager" field → lockfile (walks up to workspace root) → default npm.
// Lockfile walk is necessary for pnpm/yarn workspaces where the lockfile lives at the workspace
// root rather than in each subpackage directory.
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
	lockfiles := [][2]string{
		{"pnpm-lock.yaml", "pnpm"},
		{"yarn.lock", "yarn"},
		{"package-lock.json", "npm"},
	}
	dir := root
	for {
		for _, pair := range lockfiles {
			if _, err := os.Stat(filepath.Join(dir, pair[0])); err == nil {
				return pair[1]
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "npm"
}

// pkgExec returns a command slice for running a locally-installed tool with the
// project's package manager (npx for npm, pnpm exec for pnpm, yarn exec for yarn).
func pkgExec(pm string, args ...string) []string {
	if packagedRuntime() != "" && len(args) > 0 {
		switch args[0] {
		case "biome", "tsc", "vitest", "fta", "knip", "jscpd", "secretlint", "audit-ci":
			return packagedCommand(args[0], args[1:]...)
		}
	}
	switch pm {
	case "pnpm":
		return append([]string{"pnpm", "exec"}, args...)
	case "yarn":
		return append([]string{"yarn", "exec"}, args...)
	default:
		return append([]string{"npx"}, args...)
	}
}
