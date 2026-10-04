// tsguard — cross-platform quality gate runner for TypeScript projects.
// Replaces npm scripts as the task runner, works natively on Windows, Linux, macOS.
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

const usage = `tsguard — TypeScript quality gate runner

Quality gates (replaces npm scripts):
  tsguard check          full gate: lint + fta + types + coverage + security
  tsguard fix            auto-format: ultracite fix
  tsguard lint           lint + format check (ultracite check)
  tsguard types          type checking (tsc --noEmit)
  tsguard complexity     compatibility alias; complexity is enforced by ultracite check
  tsguard fta            Halstead + cyclomatic + LOC score per file (fta command, default cap 60)
  tsguard coverage       run tests with coverage (vitest --coverage)
  tsguard security       security: secretlint + npm/pnpm audit + audit-ci + opengrep SAST
  tsguard npm-audit      dependency vulnerability scan (npm/pnpm/yarn audit + audit-ci)
  tsguard secrets        credential scan (secretlint)
  tsguard dead-code      detect unused exports/deps (knip)
  tsguard duplicates     detect copy-paste code (jscpd)
  tsguard criticality    informational: function/method caller ranking
  tsguard audit          informational: criticality + dead-code + duplicates

Environment setup:
  tsguard setup          npm install, configure AI tool hooks
  tsguard doctor         verify toolchain (read-only)
  tsguard hooks          generate AI tool hook configs

Flags:
  --output <mode>  human (default), agent (bounded), or json (complete result)
  --root <path>    explicit project root containing package.json
  --dirs <d1,d2>    override target directories (default: . — project root)
  --exclude <d1,d2> additional directories to exclude from all scans (node_modules,dist,.next,build,coverage excluded by default)
  --timeout <s>     per-tool timeout in seconds (default: 300)
  --tail <n>        print only the last N lines of each tool's output to stdout
  --log-file <path> append full output to file (in addition to stdout)
  --if-typescript   only run check if stdin context indicates a .ts/.tsx file was edited
  --max-fta-score <n> FTA score cap per file (default: 60; >60 = Needs Improvement)
  --allow-pipe      suppress pipe refusal/warning (for CI wrappers that use tee)

Note: never pipe tsguard through an external tail (tsguard check 2>&1 | tail -50).
      Use tsguard check --tail 50 or tsguard check --log-file /tmp/tsguard.log --tail 50.
      In human mode, heavy gates (check, security, coverage) refuse to run when
      stdout is a pipe; use --allow-pipe to override.
      Lighter human commands warn on piped stdout but still run.
      Agent and JSON modes accept piped stdout.
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

	os.Exit(runCLI(cmd, args))
}

func runCLI(cmd string, args []string) int {
	cliCfg, err := parseFlags(args)
	result := newRunResult(cmd)
	finish := func(code int) int {
		result.Finish(code)
		if cliCfg.output == "agent" || cliCfg.output == "json" {
			if err := reportResult(os.Stdout, cliCfg.output, result); err != nil {
				return 1
			}
		}
		return code
	}
	fail := func(code int, category, message string) int {
		result.Execution("invocation", category, message)
		if cliCfg.output != "agent" && cliCfg.output != "json" {
			fmt.Fprintf(os.Stderr, "error: %s\n", message)
		}
		return finish(code)
	}
	if err != nil {
		return fail(exitUnknown, "invalid_configuration", err.Error())
	}
	if !analysisCommands[cmd] && cmd != "doctor" && cmd != "setup" && cmd != "hooks" {
		return fail(exitUnknown, "invalid_configuration", "unknown command: "+cmd)
	}
	if cliCfg.output != "human" && !analysisCommands[cmd] {
		return fail(exitUnknown, "invalid_configuration", "--output agent/json is supported only for analysis commands")
	}
	root := cliCfg.root
	if root == "" {
		root, err = findProjectRoot()
	} else {
		root, err = filepath.Abs(root)
		if err == nil {
			var info os.FileInfo
			info, err = os.Stat(filepath.Join(root, "package.json"))
			if err == nil && info.IsDir() {
				err = fmt.Errorf("package.json must be a file")
			}
		}
	}
	if err != nil {
		return fail(exitUnknown, "invalid_configuration", "TypeScript project root: "+err.Error())
	}
	cfg, err := buildConfig(cliCfg, root)
	if err != nil {
		return fail(exitUnknown, "invalid_configuration", err.Error())
	}
	cfg.pkgManager = detectPackageManager(root)
	if cfg.ifTypeScript && !editedFileIsTypeScript() {
		result.Status = "skipped"
		return finish(0)
	}
	if cmd == "doctor" {
		return runDoctor(root, cfg.pkgManager)
	}
	release, lockErr := acquireLock(root)
	if lockErr != nil {
		category := "lock_failure"
		var held *lockHeldError
		if errors.As(lockErr, &held) {
			category = "lock_contention"
		}
		return fail(exitLocked, category, lockErr.Error())
	}
	code := dispatchResult(cmd, cfg, root, result)
	release()
	return finish(code)
}

var analysisCommands = map[string]bool{
	"check": true, "fix": true, "lint": true, "types": true, "complexity": true,
	"fta": true, "coverage": true, "security": true, "npm-audit": true,
	"secrets": true, "dead-code": true, "duplicates": true, "audit": true, "criticality": true,
}

func dispatchResult(cmd string, cfg config, root string, result *RunResult) int {
	if !cfg.allowPipe && cfg.output == "human" {
		if info, err := os.Stdout.Stat(); err == nil && info.Mode()&os.ModeNamedPipe != 0 {
			if heavyGates[cmd] {
				fmt.Fprintln(os.Stderr,
					"tsguard: stdout is a pipe for a long-running gate. Piping through\n"+
						"     tail/head can wedge the PTY (see project CLAUDE.md).\n"+
						"     Use --tail N or --log-file, or pass --allow-pipe to override.")
				result.Execution("invocation", "pipe_refused", "Heavy gate refused piped stdout; use --allow-pipe.")
				return exitPipeRefused
			}
			fmt.Fprintln(os.Stderr,
				"tsguard: warning — stdout is a pipe. For long commands prefer\n"+
					"     --tail N or --log-file to avoid PTY wedge risk.")
		}
	}

	r := &Runner{outputMode: cfg.output, result: result, root: root, timeout: cfg.timeout, logFile: cfg.logFile, tailLines: cfg.tailLines, pkgManager: cfg.pkgManager, dirs: cfg.dirs, excludeDirs: cfg.excludeDirs, ftaExcludeTests: cfg.ftaExcludeTests, ftaExclude: cfg.ftaExclude}

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
		if cfg.initFlag {
			return runSecretsInit(r)
		}
		return runSecretlint(r)
	case "dead-code":
		return runDeadCode(r)
	case "duplicates":
		return runDuplicates(r, cfg.dirs)
	case "criticality":
		return runCriticality(r)
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
	output          string
	root            string
	dirs            []string
	excludeDirs     []string
	timeout         int
	ifTypeScript    bool
	initFlag        bool
	logFile         string
	tailLines       int
	allowPipe       bool
	ftaScoreCap     int
	assumeYes       bool
	pkgManager      string
	ftaExcludeTests bool     // exclude conventional test files from FTA (default true via buildConfig)
	ftaExclude      []string // additional fta-exclude globs from oxguard.toml
}

// parseFlags parses CLI arguments and returns only explicitly-set values.
// Zero values (nil slices, 0 ints, false bools) mean "not provided by caller".
// buildConfig applies defaults and merges with oxguard.toml before dispatch.
func parseFlags(args []string) (config, error) {
	cfg := config{output: "human"}
	// Determine the error reporter even when an earlier argument is invalid.
	for i := 0; i+1 < len(args); i++ {
		if args[i] == "--output" && (args[i+1] == "json" || args[i+1] == "agent") {
			cfg.output = args[i+1]
		}
	}
	for i := 0; i < len(args); i++ {
		flag := args[i]
		switch flag {
		case "--if-typescript":
			cfg.ifTypeScript = true
			continue
		case "--init":
			cfg.initFlag = true
			continue
		case "--allow-pipe":
			cfg.allowPipe = true
			continue
		case "--yes", "-y":
			cfg.assumeYes = true
			continue
		case "--dirs", "--exclude", "--timeout", "--tail", "--log-file", "--max-fta-score", "--root", "--output":
		default:
			return cfg, fmt.Errorf("unknown argument: %s", flag)
		}
		if i+1 >= len(args) || strings.HasPrefix(args[i+1], "--") || args[i+1] == "" {
			return cfg, fmt.Errorf("missing value for %s", flag)
		}
		i++
		value := args[i]
		switch flag {
		case "--dirs", "--exclude":
			dirs := strings.Split(value, ",")
			for _, dir := range dirs {
				if strings.TrimSpace(dir) == "" {
					return cfg, fmt.Errorf("empty directory in %s", flag)
				}
			}
			if flag == "--dirs" {
				cfg.dirs = dirs
			} else {
				cfg.excludeDirs = append(cfg.excludeDirs, dirs...)
			}
		case "--log-file":
			cfg.logFile = value
		case "--root":
			cfg.root = value
		case "--output":
			if value != "human" && value != "agent" && value != "json" {
				return cfg, fmt.Errorf("invalid output mode %q (use human, agent, or json)", value)
			}
			cfg.output = value
		default:
			n, err := strconv.Atoi(value)
			if err != nil || n < 0 || (n == 0 && flag != "--tail") {
				return cfg, fmt.Errorf("invalid value for %s: %s", flag, value)
			}
			switch flag {
			case "--timeout":
				cfg.timeout = n
			case "--tail":
				cfg.tailLines = n
			case "--max-fta-score":
				cfg.ftaScoreCap = n
			}
		}
	}
	return cfg, nil
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
