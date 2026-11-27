package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// The two layers below are a fixed, partitioned copy of the default ignore set
// that Opengrep 1.23.0 applied when a project had no .semgrepignore
// (cli/src/semgrep/templates/.semgrepignore at
// https://github.com/opengrep/opengrep/blob/v1.23.0/cli/src/semgrep/templates/.semgrepignore).
// Opengrep 2.1.0 ships the same list but anchors it at each explicit target, so
// it never skips a root entry such as ./tests or ./vendor, and a project
// .semgrepignore replaces it entirely. OxGuard scans root entries as targets (it
// must not enumerate node_modules), so it supplies the patterns itself as
// --exclude arguments. They are gitignore syntax evaluated by Opengrep, anchored
// at the project root. Do not edit an entry without re-running
// scripts/compare-opengrep.py against both engines.
//
// securityBaselineExcludes always apply and a project .semgrepignore cannot
// remove them: version-control metadata, build output, vendored code, minified
// bundles and package-manager/virtualenv directories are not project source.
// Without this layer a short project file such as "docs/" would silently start
// scanning dependencies and generated bundles.
var securityBaselineExcludes = []string{
	// Version-control administrative folders and files.
	".git", ".svn", ".hg", "_darcs", "CVS",
	// Paths that are typically large and not project source.
	"build/", "vendor/", "dist/", "*.min.js", ".env/", ".tox/",
	// Package managers and virtual environments.
	"node_modules/", ".npm/", ".yarn/", ".venv/", "_opam/", "_build/", "_cargo/",
}

// securityReplaceableDefaults are the 1.23.0 conventional test paths. A project
// .semgrepignore takes over these choices (some teams intentionally scan their
// tests), so OxGuard supplies them only when the project root has no such file.
var securityReplaceableDefaults = []string{
	"test/", "tests/", "testsuite/", "*_test.go",
}

// securityIgnoreFile is the ignore file name Opengrep 2.1.0 reads natively.
// OxGuard never parses it; it only checks whether a project-root copy exists,
// because that decides whether the default set above applies.
const securityIgnoreFile = ".semgrepignore"

// gitProject describes the Git work tree that contains the scan root.
type gitProject struct {
	top    string // absolute Git work tree root
	prefix string // scan root relative to top, slash separated, trailing slash, "" at top
}

// detectGitProject reports the enclosing Git work tree. A missing git binary,
// no repository or an unsafe repository all mean "not a Git project", which is
// also how Opengrep classifies the directory.
func detectGitProject(root string) (gitProject, bool) {
	cmd := exec.Command("git", "-C", root, "rev-parse", "--show-toplevel", "--show-prefix")
	cmd.Env = append(os.Environ(), "LC_ALL=C", "GIT_OPTIONAL_LOCKS=0")
	out, err := cmd.Output()
	if err != nil {
		return gitProject{}, false
	}
	lines := strings.Split(strings.TrimRight(string(out), "\r\n"), "\n")
	if len(lines) < 1 || strings.TrimSpace(lines[0]) == "" {
		return gitProject{}, false
	}
	project := gitProject{top: filepath.FromSlash(strings.TrimRight(lines[0], "\r"))}
	if len(lines) > 1 {
		project.prefix = strings.TrimRight(lines[1], "\r")
	}
	return project, true
}

// opengrepExcludePattern converts one OxGuard exclusion (built-in, oxguard.toml
// or --exclude) into an Opengrep 2.1.0 --exclude pattern.
//
//   - "/path" is anchored at the scan root and matches that path and everything
//     below it. Inside a Git work tree whose root is above the scan root the
//     Git-relative prefix is added, because Opengrep anchors at the Git root.
//   - Any other entry matches at any depth, the 1.23.0 behavior: "dist" and
//     "cache/private" both match below every directory. Glob syntax is the
//     gitignore syntax Opengrep documents.
//
// Negation ("!") is rejected: an exclusion list must never re-include paths.
func opengrepExcludePattern(entry, gitPrefix string) (string, bool, error) {
	if entry == "" {
		return "", false, nil
	}
	if runtime.GOOS == "windows" {
		entry = strings.ReplaceAll(entry, `\`, "/")
	}
	for strings.HasPrefix(entry, "./") {
		entry = entry[2:]
	}
	if strings.HasPrefix(entry, "!") || strings.HasPrefix(entry, "#") {
		return "", false, fmt.Errorf("Unsupported exclusion %q: negation and comments are not allowed", entry)
	}
	if strings.HasPrefix(entry, "/") {
		rest := strings.TrimLeft(entry, "/")
		if rest == "" {
			return "", false, fmt.Errorf("Unsupported exclusion %q: name a path below the scan root", entry)
		}
		return "/" + gitPrefix + rest, true, nil
	}
	if entry == "." || entry == "./" {
		return "", false, fmt.Errorf("Unsupported exclusion %q: name a path below the scan root", entry)
	}
	if strings.HasPrefix(entry, "**/") {
		return entry, true, nil
	}
	return "**/" + entry, true, nil
}

// securityScopeArgs returns the Opengrep arguments that define the security
// scan scope for root. It reads the file system but writes nothing.
//
//   - Outside Git, --project-root pins the project to the scan root so that the
//     root .semgrepignore and every anchored pattern share one project.
//     Opengrep otherwise treats each root target as its own project.
//   - Inside Git, Opengrep keeps its native Git discovery, .gitignore handling
//     and project root (the Git root).
//   - --force-exclude makes every exclusion apply to root-level files too.
//     Root entries are explicit targets, which Opengrep otherwise exempts from
//     --exclude; without the flag a root minified bundle or a root file named
//     by an OxGuard exclusion would still be scanned.
//   - OxGuard exclusions and securityBaselineExcludes are always passed.
//     Command-line exclusions outrank every ignore file, so a project
//     .semgrepignore can add exclusions but never bring these paths back.
//   - securityReplaceableDefaults are passed only when the project root has no
//     .semgrepignore, the same condition under which Opengrep applies its own
//     built-in set. A project file, even an empty one, replaces them.
func securityScopeArgs(root string, excludes []string) ([]string, error) {
	var args []string
	projectRoot, prefix := root, ""
	if project, ok := detectGitProject(root); ok {
		projectRoot, prefix = project.top, project.prefix
	} else {
		args = append(args, "--project-root", ".")
	}
	args = append(args, "--force-exclude")
	seen := map[string]bool{}
	add := func(pattern string) {
		if !seen[pattern] {
			seen[pattern] = true
			args = append(args, "--exclude", pattern)
		}
	}
	for _, entry := range excludes {
		pattern, ok, err := opengrepExcludePattern(entry, prefix)
		if err != nil {
			return nil, err
		}
		if ok {
			add(pattern)
		}
	}
	for _, pattern := range securityBaselineExcludes {
		add(pattern)
	}
	if _, err := os.ReadFile(filepath.Join(projectRoot, securityIgnoreFile)); err != nil {
		for _, pattern := range securityReplaceableDefaults {
			add(pattern)
		}
	}
	return args, nil
}

// securityScanArgs combines scope arguments and root targets. The helper test
// used by scripts/compare-opengrep.py calls this production function.
func securityScanArgs(root string, excludes []string) ([]string, error) {
	scope, err := securityScopeArgs(root, excludes)
	if err != nil {
		return nil, err
	}
	// Keep security scans root-wide without asking the scanner to enumerate
	// node_modules first. Windows pnpm junctions can stall that enumeration
	// before Opengrep applies its --exclude filters.
	targets, err := securityRootTargets(root)
	if err != nil {
		return nil, err
	}
	return append(scope, targets...), nil
}
