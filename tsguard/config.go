package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// defaultDirs are scanned when neither CLI nor oxguard.toml specifies dirs.
var defaultDirs = []string{"src", "cdk"}

// defaultExcludes are always applied to every gate.
var defaultExcludes = []string{
	"node_modules", "dist", ".next", "build", "coverage",
	".agents", ".claude", ".opencode", ".kiro", "skills",
}

// fileConfig holds values read from oxguard.toml at the project root.
// CLI flags always take precedence over file config; file config takes precedence
// over built-in defaults. Missing or unparseable file silently returns zero values.
type fileConfig struct {
	Dirs        []string `toml:"dirs"`
	Exclude     []string `toml:"exclude"`
	FTAScoreCap int      `toml:"fta-score-cap"`
	Timeout     int      `toml:"timeout"`
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
		timeout:      300,
		ftaScoreCap:  60,
		excludeDirs:  append([]string{}, defaultExcludes...),
		ifTypeScript: cli.ifTypeScript,
		initFlag:     cli.initFlag,
		logFile:      cli.logFile,
		tailLines:    cli.tailLines,
		allowPipe:    cli.allowPipe,
		assumeYes:    cli.assumeYes,
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
