// kps — cross-platform quality gate runner for kapso.
// Replaces npm scripts as the task runner, works natively on Windows, Linux, macOS.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const usage = `kps — kapso quality gate runner

Quality gates (replaces npm scripts):
  kps check          full gate: lint + complexity + fta + types + coverage + security
  kps fix            auto-format: ultracite fix
  kps lint           lint + format check (ultracite check)
  kps types          type checking (tsc --noEmit)
  kps complexity     complexity analysis (biome noExcessiveCognitiveComplexity)
  kps fta            Halstead + cyclomatic + LOC score per file (fta-cli, default cap 60)
  kps coverage       run tests with coverage (vitest --coverage)
  kps security       security: detect-secrets (hard) + npm audit (warning)
  kps npm-audit      dependency vulnerability scan (warning only)
  kps secrets        credential scan (detect-secrets)
  kps dead-code      detect unused exports/deps (knip)
  kps duplicates     detect copy-paste code (jscpd)
  kps audit          informational: dead-code + duplicates

Environment setup:
  kps setup          npm install, configure AI tool hooks
  kps doctor         verify toolchain (read-only)
  kps hooks          generate AI tool hook configs

Flags:
  --dirs <d1,d2>    override target directories (default: src,cdk)
  --timeout <s>     per-tool timeout in seconds (default: 300)
  --tail <n>        print only the last N lines of each tool's output to stdout
  --log-file <path> append full output to file (in addition to stdout)
  --if-typescript   only run check if stdin context indicates a .ts/.tsx file was edited
  --max-fta-score <n> FTA score cap per file (default: 60; >60 = Needs Improvement)
  --allow-pipe      suppress pipe refusal/warning (for CI wrappers that use tee)

Note: never pipe kps through an external tail (kps check 2>&1 | tail -50).
      Use kps check --tail 50 or kps check --log-file /tmp/kps.log --tail 50.
      For heavy gates (check, security, coverage) kps refuses to run when
      stdout is a pipe; use --allow-pipe to override.
      Lighter commands warn on piped stdout but still run.
`

func main() {
	if len(os.Args) < 2 {
		fmt.Print(usage)
		os.Exit(0)
	}

	cmd := os.Args[1]
	args := os.Args[2:]

	switch cmd {
	case "help", "-h", "--help":
		fmt.Print(usage)
		os.Exit(0)
	}

	cfg := parseFlags(args)

	root, err := findProjectRoot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		fmt.Fprintf(os.Stderr, "Run kps from inside the kapso project directory.\n")
		os.Exit(exitUnknown)
	}

	if cfg.ifTypeScript {
		if !editedFileIsTypeScript() {
			os.Exit(0)
		}
	}

	// doctor is read-only — exempt from the instance lock.
	if cmd == "doctor" {
		os.Exit(runDoctor(root))
	}

	// Instance lock — prevents concurrent kps runs from stacking up.
	release, lockErr := acquireLock(root)
	if lockErr != nil {
		fmt.Fprintf(os.Stderr, "kps: %v\n", lockErr)
		os.Exit(exitLocked)
	}

	code := dispatch(cmd, cfg, root)
	release()
	os.Exit(code)
}

func dispatch(cmd string, cfg config, root string) int {
	if !cfg.allowPipe {
		if info, err := os.Stdout.Stat(); err == nil && info.Mode()&os.ModeNamedPipe != 0 {
			if heavyGates[cmd] {
				fmt.Fprintln(os.Stderr,
					"kps: stdout is a pipe for a long-running gate. Piping through\n"+
						"     tail/head can wedge the PTY (see kapso/CLAUDE.md).\n"+
						"     Use --tail N or --log-file, or pass --allow-pipe to override.")
				return exitPipeRefused
			}
			fmt.Fprintln(os.Stderr,
				"kps: warning — stdout is a pipe. For long commands prefer\n"+
					"     --tail N or --log-file to avoid PTY wedge risk.")
		}
	}

	r := &Runner{root: root, timeout: cfg.timeout, logFile: cfg.logFile, tailLines: cfg.tailLines}

	switch cmd {
	case "check":
		return runCheck(r, cfg.dirs, cfg.ftaScoreCap)
	case "fix":
		return runFix(r)
	case "lint":
		return runLint(r)
	case "types":
		return runTypes(r)
	case "complexity":
		return runComplexity(r, cfg.dirs)
	case "fta":
		return runFTA(r, cfg.dirs, cfg.ftaScoreCap)
	case "coverage":
		return runCoverage(r)
	case "security":
		return runSecurity(r, cfg.initFlag)
	case "npm-audit":
		return runNpmAudit(r)
	case "secrets":
		return runSecrets(r, cfg.initFlag)
	case "dead-code":
		return runDeadCode(r)
	case "duplicates":
		return runDuplicates(r, cfg.dirs)
	case "audit":
		return runAudit(r, cfg.dirs)
	case "setup":
		return runSetup(root)
	case "hooks":
		return runHooks(root)
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", cmd)
		fmt.Print(usage)
		return exitUnknown
	}
}

const (
	exitUnknown     = 3 // unknown command
	exitLocked      = 4 // another kps instance is running
	exitPipeRefused = 5 // heavy gate invoked with piped stdout
)

// heavyGates are long-running commands that refuse to run on a pipe unless --allow-pipe.
var heavyGates = map[string]bool{
	"check": true, "security": true, "coverage": true,
}

// config holds parsed flags.
type config struct {
	dirs         []string
	timeout      int
	ifTypeScript bool
	initFlag     bool
	logFile      string
	tailLines    int
	allowPipe    bool
	ftaScoreCap  int
}

func parseFlags(args []string) config {
	cfg := config{
		dirs:        []string{"src", "cdk"},
		timeout:     300,
		ftaScoreCap: 60,
	}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--dirs":
			if i+1 < len(args) {
				i++
				cfg.dirs = strings.Split(args[i], ",")
			}
		case "--timeout":
			if i+1 < len(args) {
				i++
				fmt.Sscanf(args[i], "%d", &cfg.timeout)
			}
		case "--tail":
			if i+1 < len(args) {
				i++
				fmt.Sscanf(args[i], "%d", &cfg.tailLines)
			}
		case "--log-file":
			if i+1 < len(args) {
				i++
				cfg.logFile = args[i]
			}
		case "--if-typescript":
			cfg.ifTypeScript = true
		case "--init":
			cfg.initFlag = true
		case "--allow-pipe":
			cfg.allowPipe = true
		case "--max-fta-score":
			if i+1 < len(args) {
				i++
				fmt.Sscanf(args[i], "%d", &cfg.ftaScoreCap)
			}
		}
	}
	return cfg
}

func findProjectRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		candidate := filepath.Join(dir, "package.json")
		if data, err := os.ReadFile(candidate); err == nil {
			var pkg struct {
				Name string `json:"name"`
			}
			if err := json.Unmarshal(data, &pkg); err == nil && pkg.Name == "kapso" {
				return dir, nil
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", fmt.Errorf("kapso project root not found (no package.json with name \"kapso\")")
}
