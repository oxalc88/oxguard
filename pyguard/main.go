// pyguard — cross-platform quality gate runner for Python projects.
// Replaces GNU Make as the task runner, works natively on Windows, Linux, macOS.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var version = "dev"

const usage = `pyguard — Python quality gate runner

Quality gates (replaces make):
  pyguard check          full gate: ruff + mypy + radon + types + coverage + security
  pyguard fix            auto-format: ruff --fix + ruff format
  pyguard audit          informational: criticality + dead-code + deps
  pyguard security       security only: bandit + pip-audit + secrets
  pyguard ruff           lint + format check
  pyguard mypy           type checking
  pyguard radon          complexity analysis (fails if CC > 10)
  pyguard types          type-annotation complexity (fails if depth>2 or length>40)
  pyguard coverage       run tests with coverage
  pyguard bandit         security scan
  pyguard pip-audit      dependency vulnerability scan
  pyguard secrets        credential scan
  pyguard criticality    call-graph criticality analysis
  pyguard dead-code      detect dead code
  pyguard deps           dependency hygiene

Environment setup (replaces mise):
  pyguard setup          install uv, run uv sync, configure AI tool hooks
  pyguard doctor         verify toolchain (read-only)

Testing (replaces jq):
  pyguard invoke <fn> <payload>   aws lambda invoke + parse response
  pyguard test <fn> <client>      invoke test harness + parse summary

Flags:
  --dirs <d1,d2>    override target directories (default: . — project root)
  --timeout <s>     per-tool timeout in seconds (default: 300)
  --tail <n>        print only the last N lines of each tool's output to stdout
  --log-file <path> append full output to file (in addition to stdout)
  --if-python       only run check if stdin context indicates a .py file was edited
  --allow-pipe      suppress pipe refusal/warning (for CI wrappers that use tee)

Note: never pipe pyguard through an external tail (pyguard check 2>&1 | tail -50).
      Use pyguard check --tail 50 or pyguard check --log-file /tmp/pyguard.log --tail 50.
      For heavy gates (check, security, coverage) pyguard refuses to run when
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

	// Help and version commands need no root and no lock.
	switch cmd {
	case "help", "-h", "--help":
		fmt.Print(usage)
		os.Exit(0)
	case "version", "--version", "-v":
		fmt.Println(version)
		os.Exit(0)
	}

	// Parse global flags
	cfg := parseFlags(args)

	// Find project root
	root, err := findProjectRoot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		fmt.Fprintf(os.Stderr, "Run pyguard from inside a Python project directory.\n")
		os.Exit(exitUnknown)
	}

	// Merge [tool.pyguard] from pyproject.toml — file config > built-in defaults.
	pgCfg := loadPyguardConfig(root)
	if pgCfg.ExcludeTests != nil {
		cfg.excludeTests = *pgCfg.ExcludeTests
	} else {
		cfg.excludeTests = true // default: skip conventional test files from complexity gates
	}
	cfg.exclude = append(cfg.exclude, pgCfg.Exclude...)

	// --if-python: exit 0 if the edited file (from stdin JSON) is not .py
	if cfg.ifPython {
		if !editedFileIsPython() {
			os.Exit(0)
		}
	}

	// doctor is read-only — exempt from the instance lock.
	if cmd == "doctor" {
		os.Exit(runDoctor(root))
	}

	// Instance lock — prevents concurrent pyguard runs from stacking up.
	// Retries from Claude Code or other tools see exit 4 and stop spawning new wrappers.
	release, lockErr := acquireLock(root)
	if lockErr != nil {
		fmt.Fprintf(os.Stderr, "pyguard: %v\n", lockErr)
		os.Exit(exitLocked)
	}

	code := dispatch(cmd, args, cfg, root)
	release()
	os.Exit(code)
}

func dispatch(cmd string, args []string, cfg config, root string) int {
	if !cfg.allowPipe {
		if info, err := os.Stdout.Stat(); err == nil && info.Mode()&os.ModeNamedPipe != 0 {
			if heavyGates[cmd] {
				fmt.Fprintln(os.Stderr,
					"pyguard: stdout is a pipe for a long-running gate. Piping through\n"+
						"     tail/head can wedge the PTY (see project CLAUDE.md).\n"+
						"     Use --tail N or --log-file, or pass --allow-pipe to override.")
				return exitPipeRefused
			}
			fmt.Fprintln(os.Stderr,
				"pyguard: warning — stdout is a pipe. For long commands prefer\n"+
					"     --tail N or --log-file to avoid PTY wedge risk.")
		}
	}

	r := &Runner{root: root, timeout: cfg.timeout, logFile: cfg.logFile, tailLines: cfg.tailLines, excludeTests: cfg.excludeTests, exclude: cfg.exclude}

	switch cmd {
	// Quality gates
	case "check":
		return runCheck(r, cfg.dirs)
	case "fix":
		return runFix(r, cfg.dirs)
	case "audit":
		return runAudit(r, cfg.dirs)
	case "security":
		return runSecurity(r, cfg.dirs)
	case "ruff":
		return runRuff(r, cfg.dirs)
	case "mypy":
		return runMypy(r, cfg.dirs)
	case "radon":
		return runRadon(r, cfg.dirs)
	case "types":
		return runTypes(r, cfg.dirs)
	case "coverage":
		return runCoverage(r)
	case "bandit":
		return runBandit(r, cfg.dirs)
	case "pip-audit":
		return runPipAudit(r)
	case "secrets":
		return runSecrets(r, cfg)
	case "criticality":
		return runCriticality(r)
	case "dead-code":
		return runDeadCode(r, cfg.dirs)
	case "deps":
		return runDeps(r)

	// Setup
	case "setup":
		return runSetup(root, cfg)
	case "hooks":
		return runHooks(root)

	// Lambda testing
	case "invoke":
		return runInvoke(args)
	case "test":
		return runTest(args)

	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", cmd)
		fmt.Print(usage)
		return exitUnknown
	}
}

const (
	exitUnknown     = 3 // unknown command
	exitLocked      = 4 // another pyguard instance is running
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
	ifPython     bool
	initFlag     bool     // --init for pyguard secrets
	logFile      string   // --log-file path
	tailLines    int      // --tail N
	allowPipe    bool     // --allow-pipe
	assumeYes    bool     // --yes / -y: skip interactive prompts
	excludeTests bool     // exclude conventional test files from radon/complexity gates
	exclude      []string // additional exclude globs from [tool.pyguard]
}

func parseFlags(args []string) config {
	cfg := config{
		dirs:    []string{"."},
		timeout: 300,
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
		case "--if-python":
			cfg.ifPython = true
		case "--init":
			cfg.initFlag = true
		case "--allow-pipe":
			cfg.allowPipe = true
		case "--yes", "-y":
			cfg.assumeYes = true
		}
	}
	return cfg
}

// findProjectRoot walks up from cwd looking for pyproject.toml.
func findProjectRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "pyproject.toml")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", fmt.Errorf("Python project root not found (no pyproject.toml in parent directories)")
}
