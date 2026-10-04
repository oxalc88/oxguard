// pyguard — cross-platform quality gate runner for Python projects.
// Replaces GNU Make as the task runner, works natively on Windows, Linux, macOS.
package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
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
  --output <mode>  human (default), agent (bounded), json (complete)
  --root <path>    explicit project root containing pyproject.toml
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

	os.Exit(runCLI(cmd, args))
}

var analysisCommands = map[string]bool{
	"check": true, "fix": true, "audit": true, "security": true, "ruff": true, "mypy": true, "radon": true, "types": true, "coverage": true, "bandit": true, "pip-audit": true, "secrets": true, "criticality": true, "dead-code": true, "deps": true,
}

func runCLI(cmd string, args []string) int {
	flagArgs := args
	if cmd == "invoke" || cmd == "test" {
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "Two positional arguments are required")
			return exitUnknown
		}
		flagArgs = args[2:]
	}
	cfg, err := parseFlags(flagArgs)
	result := newRunResult(cmd)
	finish := func(code int) int {
		result.Finish(code)
		if cfg.output != "human" {
			if err := reportResult(os.Stdout, cfg.output, result); err != nil {
				return 1
			}
		}
		return code
	}
	fail := func(code int, category, message string) int {
		result.Execution("invocation", category, message)
		if cfg.output == "human" {
			fmt.Fprintln(os.Stderr, "error: "+message)
		}
		return finish(code)
	}
	if err != nil {
		return fail(exitUnknown, "invalid_configuration", err.Error())
	}
	if !analysisCommands[cmd] && cmd != "setup" && cmd != "doctor" && cmd != "hooks" && cmd != "invoke" && cmd != "test" {
		return fail(exitUnknown, "invalid_configuration", "unknown command: "+cmd)
	}
	if cfg.output != "human" && !analysisCommands[cmd] {
		return fail(exitUnknown, "invalid_configuration", "--output agent/json is supported only for analysis commands")
	}
	if cfg.output != "human" && cfg.initFlag {
		return fail(exitUnknown, "invalid_configuration", "--init requires human output")
	}
	root := cfg.root
	if root == "" {
		root, err = findProjectRoot()
	} else {
		root, err = filepath.Abs(root)
		if err == nil {
			var info os.FileInfo
			info, err = os.Stat(filepath.Join(root, "pyproject.toml"))
			if err == nil && info.IsDir() {
				err = fmt.Errorf("pyproject.toml must be a file")
			}
		}
	}
	if err != nil {
		return fail(exitUnknown, "invalid_configuration", err.Error())
	}
	if physical, e := filepath.EvalSymlinks(root); e == nil {
		root = physical
	}
	if cfg.logFile != "" {
		cfg.logFile, err = filepath.Abs(cfg.logFile)
		if err != nil {
			return fail(exitUnknown, "invalid_configuration", err.Error())
		}
	}
	pgCfg, err := loadPyguardConfig(root)
	if err != nil {
		return fail(exitUnknown, "invalid_configuration", err.Error())
	}
	cfg.excludeTests = true
	if pgCfg.ExcludeTests != nil {
		cfg.excludeTests = *pgCfg.ExcludeTests
	}
	cfg.exclude = append(cfg.exclude, pgCfg.Exclude...)
	if cfg.ifPython && !editedFileIsPython() {
		result.Status = "skipped"
		return finish(0)
	}
	if cmd == "doctor" {
		return runDoctor(root)
	}
	release, err := acquireLock(root)
	if err != nil {
		category := "lock_failure"
		var held *lockHeldError
		if errors.As(err, &held) {
			category = "lock_contention"
		}
		return fail(exitLocked, category, err.Error())
	}
	defer release()
	return finish(dispatch(cmd, args, cfg, root, result))
}

func dispatch(cmd string, args []string, cfg config, root string, result *RunResult) int {
	if cfg.output == "human" && !cfg.allowPipe {
		if info, err := os.Stdout.Stat(); err == nil && info.Mode()&os.ModeNamedPipe != 0 {
			if heavyGates[cmd] {
				fmt.Fprintln(os.Stderr,
					"pyguard: stdout is a pipe for a long-running gate. Piping through\n"+
						"     tail/head can wedge the PTY (see project CLAUDE.md).\n"+
						"     Use --tail N or --log-file, or pass --allow-pipe to override.")
				result.Execution("invocation", "pipe_refused", "Heavy gate refused piped stdout; use --allow-pipe.")
				return exitPipeRefused
			}
			fmt.Fprintln(os.Stderr,
				"pyguard: warning — stdout is a pipe. For long commands prefer\n"+
					"     --tail N or --log-file to avoid PTY wedge risk.")
		}
	}

	r := &Runner{outputMode: cfg.output, result: result, dirs: cfg.dirs, dirsExplicit: cfg.dirsExplicit, root: root, timeout: cfg.timeout, logFile: cfg.logFile, tailLines: cfg.tailLines, excludeTests: cfg.excludeTests, exclude: cfg.exclude}

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
	dirsExplicit bool
	timeout      int
	ifPython     bool
	initFlag     bool // --init for pyguard secrets
	output       string
	root         string
	logFile      string   // --log-file path
	tailLines    int      // --tail N
	allowPipe    bool     // --allow-pipe
	assumeYes    bool     // --yes / -y: skip interactive prompts
	excludeTests bool     // exclude conventional test files from radon/complexity gates
	exclude      []string // additional exclude globs from [tool.pyguard]
}

func parseFlags(args []string) (config, error) {
	cfg := config{dirs: []string{"."}, timeout: 300, output: "human"}
	// Select a recognized requested reporter even when another argument is invalid.
	for i := 0; i+1 < len(args); i++ {
		if args[i] == "--output" && (args[i+1] == "json" || args[i+1] == "agent") {
			cfg.output = args[i+1]
		}
	}
	for i := 0; i < len(args); i++ {
		flag := args[i]
		switch flag {
		case "--dirs", "--timeout", "--tail", "--log-file", "--output", "--root":
			if i+1 == len(args) || strings.HasPrefix(args[i+1], "--") || args[i+1] == "" {
				return cfg, fmt.Errorf("missing value for %s", flag)
			}
			i++
			value := args[i]
			switch flag {
			case "--dirs":
				cfg.dirsExplicit = true
				cfg.dirs = strings.Split(value, ",")
				for _, d := range cfg.dirs {
					if strings.TrimSpace(d) == "" {
						return cfg, fmt.Errorf("empty directory in --dirs")
					}
				}
			case "--root":
				cfg.root = value
			case "--log-file":
				cfg.logFile = value
			case "--output":
				if value != "human" && value != "json" && value != "agent" {
					return cfg, fmt.Errorf("invalid output mode: %s", value)
				}
				cfg.output = value
			case "--timeout", "--tail":
				n, e := strconv.Atoi(value)
				if e != nil || n < 0 || (flag == "--timeout" && n == 0) {
					return cfg, fmt.Errorf("invalid value for %s", flag)
				}
				if flag == "--timeout" {
					cfg.timeout = n
				} else {
					cfg.tailLines = n
				}
			}
		case "--if-python":
			cfg.ifPython = true
		case "--init":
			cfg.initFlag = true
		case "--allow-pipe":
			cfg.allowPipe = true
		case "--yes", "-y":
			cfg.assumeYes = true
		default:
			return cfg, fmt.Errorf("unknown argument: %s", flag)
		}
	}
	return cfg, nil
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
