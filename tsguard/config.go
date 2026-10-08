package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// defaultDirs are scanned when neither CLI nor oxguard.toml specifies dirs.
// Scan from project root; excludeDirs filters the noise.
var defaultDirs = []string{"."}

// defaultExcludes are always applied to every gate.
var defaultExcludes = []string{
	"node_modules", "dist", ".next", "build", "coverage",
	".agents", ".claude", ".opencode", ".kiro", "skills",
}

// fileConfig holds values read from oxguard.toml at the project root.
// CLI flags always take precedence over file config; file config takes precedence
// over built-in defaults. The builder chooses human fallback or structured failure.
type fileConfig struct {
	Dirs            []string `toml:"dirs"`
	Exclude         []string `toml:"exclude"`
	FTAScoreCap     int      `toml:"fta-score-cap"`
	Timeout         int      `toml:"timeout"`
	FtaExcludeTests *bool    `toml:"fta-exclude-tests"` // nil = use default (true)
	FtaExclude      []string `toml:"fta-exclude"`       // extra globs appended to defaults
}

// loadFileConfig reads oxguard.toml; buildConfig treats a missing file as defaults.
func loadFileConfig(root string) (fileConfig, error) {
	data, err := os.ReadFile(filepath.Join(root, "oxguard.toml"))
	if err != nil {
		return fileConfig{}, err
	}
	var fc fileConfig
	if _, err := toml.NewDecoder(bytes.NewReader(data)).Decode(&fc); err != nil {
		return fileConfig{}, err
	}
	return fc, nil
}

// buildConfig merges built-in defaults, oxguard.toml, and CLI flags in priority order.
//
//	dirs:    CLI > file > default (replacement, not additive)
//	exclude: default + file + CLI (always additive; base set always applies)
//	scalars: CLI > file > default
//	booleans: always from CLI (zero value = not passed)
func buildConfig(cli config, root string) (config, error) {
	file, err := loadFileConfig(root)
	if err != nil && !os.IsNotExist(err) {
		if cli.output == "agent" || cli.output == "json" {
			return config{}, fmt.Errorf("oxguard.toml: %w", err)
		}
		fmt.Fprintf(os.Stderr, "tsguard: warning — oxguard.toml parse error: %v\n", err)
	}
	if (cli.output == "agent" || cli.output == "json") && (file.Timeout < 0 || file.FTAScoreCap < 0) {
		return config{}, fmt.Errorf("oxguard.toml: timeout and fta-score-cap must be positive")
	}

	cfg := config{
		output:          cli.output,
		root:            cli.root,
		timeout:         300,
		ftaScoreCap:     60,
		excludeDirs:     append([]string{}, defaultExcludes...),
		ftaExcludeTests: true, // default: skip conventional test files from FTA scoring
		ifTypeScript:    cli.ifTypeScript,
		initFlag:        cli.initFlag,
		logFile:         cli.logFile,
		tailLines:       cli.tailLines,
		allowPipe:       cli.allowPipe,
		assumeYes:       cli.assumeYes,
	}

	// File config: dirs replace default; scalars override default.
	if len(file.Dirs) > 0 {
		cfg.dirs = file.Dirs
	} else {
		cfg.dirs = append([]string{}, defaultDirs...)
	}
	cfg.excludeDirs = append(cfg.excludeDirs, file.Exclude...)
	if file.FTAScoreCap > 0 {
		cfg.ftaScoreCap = file.FTAScoreCap
	}
	if file.Timeout > 0 {
		cfg.timeout = file.Timeout
	}
	if file.FtaExcludeTests != nil {
		cfg.ftaExcludeTests = *file.FtaExcludeTests
	}
	cfg.ftaExclude = append(cfg.ftaExclude, file.FtaExclude...)

	// CLI always wins. nil slice = flag was not passed.
	if cli.dirs != nil {
		cfg.dirs = cli.dirs
	}
	if cli.excludeDirs != nil {
		cfg.excludeDirs = append(cfg.excludeDirs, cli.excludeDirs...)
	}
	if cli.timeout > 0 {
		cfg.timeout = cli.timeout
	}
	if cli.ftaScoreCap > 0 {
		cfg.ftaScoreCap = cli.ftaScoreCap
	}

	// Repository-supplied and CLI scopes must never escape the project, even
	// through symlinks. An analyzer must not forward untrusted option strings.
	if err := validateScanDirs(root, cfg.dirs); err != nil {
		return config{}, err
	}

	return cfg, nil
}

// ftaConfig is the fta.json schema subset tsguard writes for exclusion control.
// fta appends user-provided values to its built-in defaults (dist/bin/build, .d.ts/.min.js/.bundle.js).
type ftaConfig struct {
	ExcludeFilenames   []string `json:"exclude_filenames,omitempty"`
	ExcludeDirectories []string `json:"exclude_directories,omitempty"`
}

// conventionalTestGlobs are universally-conventional test file names in the JS/TS ecosystem.
// Project-specific patterns (*.pbt.ts, *.bench.ts, etc.) belong in oxguard.toml fta-exclude.
var conventionalTestGlobs = []string{
	"*.test.ts", "*.test.tsx", "*.test.js", "*.test.jsx",
	"*.spec.ts", "*.spec.tsx", "*.spec.js", "*.spec.jsx",
}

// conventionalTestDirs are directory names that universally hold test infrastructure.
var conventionalTestDirs = []string{"__tests__", "__mocks__", "__fixtures__"}

// writeFTAConfig generates a project-local fta.json in node_modules/.cache/oxguard/ and
// returns its absolute path. Returns "" when nothing needs to be excluded (no config written).
// When --config-path is passed fta no longer auto-discovers the project root fta.json, so
// any existing project-root fta.json is read and merged in to preserve its exclusions.
func writeFTAConfig(root string, excludeTests bool, extraExclude []string) (string, error) {
	var excludeFilenames, excludeDirs []string

	if excludeTests {
		excludeFilenames = append(excludeFilenames, conventionalTestGlobs...)
		excludeDirs = append(excludeDirs, conventionalTestDirs...)
	}
	excludeFilenames = append(excludeFilenames, extraExclude...)

	// Fold in any project-root fta.json — fta reads only one config file.
	if data, err := os.ReadFile(filepath.Join(root, "fta.json")); err == nil {
		var proj ftaConfig
		if json.Unmarshal(data, &proj) == nil {
			excludeFilenames = append(excludeFilenames, proj.ExcludeFilenames...)
			excludeDirs = append(excludeDirs, proj.ExcludeDirectories...)
		}
	}

	if len(excludeFilenames) == 0 && len(excludeDirs) == 0 {
		return "", nil
	}

	cacheDir := filepath.Join(root, opengrepCacheDir)
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return "", err
	}
	data, err := json.MarshalIndent(ftaConfig{
		ExcludeFilenames:   excludeFilenames,
		ExcludeDirectories: excludeDirs,
	}, "", "  ")
	if err != nil {
		return "", err
	}
	configPath := filepath.Join(cacheDir, "fta.json")
	return configPath, os.WriteFile(configPath, data, 0o644)
}

// validateScanDirs rejects paths which could make mutating tools leave the
// selected project. Missing targets are left to the analyzer to report.
func validateScanDirs(root string, dirs []string) error {
	physicalRoot, err := filepath.EvalSymlinks(root)
	if err != nil { return fmt.Errorf("resolve project root: %w", err) }
	for _, dir := range dirs {
		if dir == "" || strings.HasPrefix(dir, "-") || filepath.IsAbs(dir) || filepath.VolumeName(dir) != "" {
			return fmt.Errorf("unsafe scan directory %q", dir)
		}
		clean := filepath.Clean(dir)
		if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			return fmt.Errorf("scan directory escapes project: %q", dir)
		}
		candidate := filepath.Join(physicalRoot, clean)
		if resolved, err := filepath.EvalSymlinks(candidate); err == nil {
			rel, err := filepath.Rel(physicalRoot, resolved)
			if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				return fmt.Errorf("scan directory leaves project through symlink: %q", dir)
			}
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("resolve scan directory %q: %w", dir, err)
		}
	}
	return nil
}
