// tsguard — cross-platform quality gate runner for TypeScript projects.
// Replaces npm scripts as the task runner, works natively on Windows, Linux, macOS.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var version = "dev"

const usage = `tsguard — TypeScript quality gate runner

Quality gates (replaces npm scripts):
  tsguard check          full gate: lint + fta + types + coverage + security
  tsguard fix            auto-format: ultracite fix
  tsguard lint           lint + format check (ultracite check)
  tsguard types          type checking (tsc --noEmit)
  tsguard complexity     compatibility alias; complexity is enforced by ultracite check
  tsguard fta            Halstead + cyclomatic + LOC score per file (fta command, default cap 60)
  tsguard coverage       run tests with coverage (vitest --coverage)
  tsguard security       security: detect-secrets (hard) + npm audit (warning)
  tsguard npm-audit      dependency vulnerability scan (warning only)
  tsguard secrets        credential scan (detect-secrets)
  tsguard dead-code      detect unused exports/deps (knip)
  tsguard duplicates     detect copy-paste code (jscpd)
  tsguard audit          informational: dead-code + duplicates

Environment setup:
  tsguard setup          npm install, configure AI tool hooks
  tsguard doctor         verify toolchain (read-only)
  tsguard hooks          generate AI tool hook configs

Flags:
  --dirs <d1,d2>    override target directories (default: src,cdk)
  --timeout <s>     per-tool timeout in seconds (default: 300)
  --tail <n>        print only the last N lines of each tool's output to stdout
  --log-file <path> append full output to file (in addition to stdout)
  --if-typescript   only run check if stdin context indicates a .ts/.tsx file was edited
  --max-fta-score <n> FTA score cap per file (default: 60; >60 = Needs Improvement)
  --allow-pipe      suppress pipe refusal/warning (for CI wrappers that use tee)

Note: never pipe tsguard through an external tail (tsguard check 2>&1 | tail -50).
      Use tsguard check --tail 50 or tsguard check --log-file /tmp/tsguard.log --tail 50.
      For heavy gates (check, security, coverage) tsguard refuses to run when
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
	case "version", "--version", "-v":
		fmt.Println(version)
		os.Exit(0)
	}

	cfg := parseFlags(args)

	root, err := findProjectRoot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		fmt.Fprintf(os.Stderr, "Run tsguard from inside a TypeScript project directory.\n")
		os.Exit(exitUnknown)
	}

	cfg.pkgManager = detectPackageManager(root)

	if cfg.ifTypeScript {
		if !editedFileIsTypeScript() {
			os.Exit(0)
		}
	}

	// doctor is read-only — exempt from the instance lock.
	if cmd == "doctor" {
		os.Exit(runDoctor(root, cfg.pkgManager))
	}

	// Instance lock — prevents concurrent tsguard runs from stacking up.
	release, lockErr := acquireLock(root)
	if lockErr != nil {
		fmt.Fprintf(os.Stderr, "tsguard: %v\n", lockErr)
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
					"tsguard: stdout is a pipe for a long-running gate. Piping through\n"+
						"     tail/head can wedge the PTY (see project CLAUDE.md).\n"+
						"     Use --tail N or --log-file, or pass --allow-pipe to override.")
				return exitPipeRefused
			}
			fmt.Fprintln(os.Stderr,
				"tsguard: warning — stdout is a pipe. For long commands prefer\n"+
					"     --tail N or --log-file to avoid PTY wedge risk.")
		}
	}

	r := &Runner{root: root, timeout: cfg.timeout, logFile: cfg.logFile, tailLines: cfg.tailLines, pkgManager: cfg.pkgManager}

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
		return runSetup(root, cfg)
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
	exitLocked      = 4 // another tsguard instance is running
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
	assumeYes    bool
	pkgManager   string
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
		case "--yes", "-y":
			cfg.assumeYes = true
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
		if _, err := os.Stat(filepath.Join(dir, "package.json")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", fmt.Errorf("TypeScript project root not found (no package.json in parent directories)")
}
