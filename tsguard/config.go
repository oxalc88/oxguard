package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

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
// over built-in defaults. Missing or unparseable file silently returns zero values.
type fileConfig struct {
	Dirs            []string `toml:"dirs"`
	Exclude         []string `toml:"exclude"`
	FTAScoreCap     int      `toml:"fta-score-cap"`
	Timeout         int      `toml:"timeout"`
	FtaExcludeTests *bool    `toml:"fta-exclude-tests"` // nil = use default (true)
	FtaExclude      []string `toml:"fta-exclude"`        // extra globs appended to defaults
}

// loadFileConfig reads oxguard.toml from root. Missing file is silently ignored.
func loadFileConfig(root string) fileConfig {
	data, err := os.ReadFile(filepath.Join(root, "oxguard.toml"))
	if err != nil {
		return fileConfig{}
	}
	var fc fileConfig
	if _, err := toml.NewDecoder(bytes.NewReader(data)).Decode(&fc); err != nil {
		fmt.Fprintf(os.Stderr, "tsguard: warning — oxguard.toml parse error: %v\n", err)
		return fileConfig{}
	}
	return fc
}

// buildConfig merges built-in defaults, oxguard.toml, and CLI flags in priority order.
//
//   dirs:    CLI > file > default (replacement, not additive)
//   exclude: default + file + CLI (always additive; base set always applies)
//   scalars: CLI > file > default
//   booleans: always from CLI (zero value = not passed)
func buildConfig(cli config, root string) config {
	file := loadFileConfig(root)

	cfg := config{
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

	return cfg
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
