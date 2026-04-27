// pkn — cross-platform quality gate runner for pakatnamu.
// Replaces GNU Make as the task runner, works natively on Windows, Linux, macOS.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const usage = `pkn — pakatnamu quality gate runner

Quality gates (replaces make):
  pkn check          full gate: ruff + mypy + radon + types + coverage + security
  pkn fix            auto-format: ruff --fix + ruff format
  pkn audit          informational: criticality + dead-code + deps
  pkn security       security only: bandit + pip-audit + secrets
  pkn ruff           lint + format check
  pkn mypy           type checking
  pkn radon          complexity analysis (fails if CC > 10)
  pkn types          type-annotation complexity (fails if depth>2 or length>40)
  pkn coverage       run tests with coverage
  pkn bandit         security scan
  pkn pip-audit      dependency vulnerability scan
  pkn secrets        credential scan
  pkn criticality    call-graph criticality analysis
  pkn dead-code      detect dead code
  pkn deps           dependency hygiene

Environment setup (replaces mise):
  pkn setup          install uv, run uv sync, configure AI tool hooks
  pkn doctor         verify toolchain (read-only)

Testing (replaces jq):
  pkn invoke <fn> <payload>   aws lambda invoke + parse response
  pkn test <fn> <client>      invoke test harness + parse summary

Flags:
  --dirs <d1,d2>    override target directories (default: functions,cdk)
  --timeout <s>     per-tool timeout in seconds (default: 300)
  --tail <n>        print only the last N lines of each tool's output to stdout
  --log-file <path> append full output to file (in addition to stdout)
  --if-python       only run check if stdin context indicates a .py file was edited
  --allow-pipe      suppress pipe refusal/warning (for CI wrappers that use tee)

Note: never pipe pkn through an external tail (pkn check 2>&1 | tail -50).
      Use pkn check --tail 50 or pkn check --log-file /tmp/pkn.log --tail 50.
      For heavy gates (check, security, coverage) pkn refuses to run when
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

	// Help commands need no root and no lock.
	switch cmd {
	case "help", "-h", "--help":
		fmt.Print(usage)
		os.Exit(0)
	}

	// Parse global flags
	cfg := parseFlags(args)

	// Find project root
	root, err := findProjectRoot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		fmt.Fprintf(os.Stderr, "Run pkn from inside the pakatnamu project directory.\n")
		os.Exit(exitUnknown)
	}

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

	// Instance lock — prevents concurrent pkn runs from stacking up.
	// Retries from Claude Code or other tools see exit 4 and stop spawning new wrappers.
	release, lockErr := acquireLock(root)
	if lockErr != nil {
		fmt.Fprintf(os.Stderr, "pkn: %v\n", lockErr)
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
					"pkn: stdout is a pipe for a long-running gate. Piping through\n"+
						"     tail/head can wedge the PTY (see pakatnamu/CLAUDE.md).\n"+
						"     Use --tail N or --log-file, or pass --allow-pipe to override.")
				return exitPipeRefused
			}
			fmt.Fprintln(os.Stderr,
				"pkn: warning — stdout is a pipe. For long commands prefer\n"+
					"     --tail N or --log-file to avoid PTY wedge risk.")
		}
	}

	r := &Runner{root: root, timeout: cfg.timeout, logFile: cfg.logFile, tailLines: cfg.tailLines}

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
		return runSetup(root)
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
	exitLocked      = 4 // another pkn instance is running
	exitPipeRefused = 5 // heavy gate invoked with piped stdout
)

// heavyGates are long-running commands that refuse to run on a pipe unless --allow-pipe.
var heavyGates = map[string]bool{
	"check": true, "security": true, "coverage": true,
}

// config holds parsed flags.
type config struct {
	dirs      []string
	timeout   int
	ifPython  bool
	initFlag  bool   // --init for pkn secrets
	logFile   string // --log-file path
	tailLines int    // --tail N
	allowPipe bool   // --allow-pipe
}

func parseFlags(args []string) config {
	cfg := config{
		dirs:    []string{"functions", "cdk"},
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
		}
	}
	return cfg
}

// findProjectRoot walks up from cwd looking for pyproject.toml containing name = "pakatnamu".
func findProjectRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		candidate := filepath.Join(dir, "pyproject.toml")
		if data, err := os.ReadFile(candidate); err == nil {
			if strings.Contains(string(data), `name = "pakatnamu"`) {
				return dir, nil
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", fmt.Errorf("pakatnamu project root not found (no pyproject.toml with name = \"pakatnamu\")")
}
