package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
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
// tests), so OxGuard supplies them only when no such file exists (see
// securityScopeArgs for which directories count).
var securityReplaceableDefaults = []string{
	"test/", "tests/", "testsuite/", "*_test.go",
}

// securityIgnoreFile is the ignore file name Opengrep 2.1.0 reads natively.
// OxGuard never parses it; it only checks whether a scan-root or project-root copy exists,
// because that decides whether the test defaults apply.
const securityIgnoreFile = ".semgrepignore"

// hasSecurityIgnoreFile reports whether dir holds a readable .semgrepignore.
// A directory or an unreadable entry holds no patterns and does not count.
func hasSecurityIgnoreFile(dir string) bool {
	_, err := os.ReadFile(filepath.Join(dir, securityIgnoreFile))
	return err == nil
}

// scopeConfigError marks a scope problem the user can fix in configuration.
type scopeConfigError struct{ message string }

func (e *scopeConfigError) Error() string { return e.message }

// gitProject describes the Git work tree that contains the scan root.
type gitProject struct {
	top string // absolute Git work tree root
}

// detectGitProject reports the enclosing Git work tree. A missing git binary,
// no repository or an unsafe repository all mean "not a Git project", which is
// also how Opengrep classifies the directory.
func detectGitProject(root string) (gitProject, bool) {
	cmd := exec.Command("git", "-C", root, "rev-parse", "--show-toplevel")
	cmd.Env = append(os.Environ(), "LC_ALL=C", "GIT_OPTIONAL_LOCKS=0")
	out, err := cmd.Output()
	if err != nil {
		return gitProject{}, false
	}
	top := strings.TrimRight(string(out), "\r\n")
	if strings.TrimSpace(top) == "" {
		return gitProject{}, false
	}
	return gitProject{top: filepath.FromSlash(top)}, true
}

var repeatedSlashes = regexp.MustCompile(`/{2,}`)

// opengrepExcludePattern converts one OxGuard exclusion (built-in, oxguard.toml
// or --exclude) into an Opengrep 2.1.0 --exclude pattern with exactly the
// effective meaning the shipped version had under 1.23.0.
//
// 1.23.0 turned every entry E into the wcmatch patterns "**/E" and "**/E/**",
// so slashes never anchored anything: "/src" excluded "src" and "lib/src",
// "/a.ts" excluded "src/a.ts", "a/b", "/a/b" and "a/b/" were the same, and
// repeated separators collapsed ("a//b", "a///b" and "/a//b/" meant "a/b").
// The mapping strips leading and trailing slashes, collapses repeated ones and
// matches at any depth ("**/E"); scripts/compare-opengrep.py verifies each form
// against both engines. Other characters keep their meaning, including a
// leading "!" or "#" and "." segments ("./x" and "a/./b" were literals that
// matched nothing). Root-level files are explicit targets and were never
// excludable in 1.23.0 either.
//
// An entry that is empty after stripping slashes ("", "/", "//") made 1.23.0
// exclude every file below the root entries (explicit root-level files were
// still scanned), which silently disabled the gate. The --exclude flag already
// rejects an empty element ("foo,"); oxguard.toml entries were not validated,
// so empty and slash-only entries are now rejected as invalid configuration.
func opengrepExcludePattern(entry string) (string, error) {
	if runtime.GOOS == "windows" {
		entry = strings.ReplaceAll(entry, `\`, "/")
	}
	trimmed := strings.Trim(repeatedSlashes.ReplaceAllString(entry, "/"), "/")
	if trimmed == "" {
		return "", &scopeConfigError{fmt.Sprintf("Unsupported exclusion %q in oxguard.toml or --exclude: it would exclude every file below the project root entries; name a path, for example \"dist\" or \"packages/generated\"", entry)}
	}
	if strings.HasPrefix(trimmed, "**/") {
		return trimmed, nil
	}
	return "**/" + trimmed, nil
}

// securityScope is the read-only result of securityScopeArgs.
type securityScope struct {
	args []string
}

// securityScopeArgs returns the Opengrep arguments that define the security
// scan scope for root. It reads the file system but writes nothing.
//
//   - Outside Git, --project-root pins the project to the scan root so that the
//     root .semgrepignore and every anchored pattern share one project.
//     Opengrep otherwise treats each root target as its own project.
//   - Inside Git, Opengrep keeps its native Git discovery, .gitignore handling
//     and project root (the Git root).
//   - Root-level files are explicit targets, which Opengrep exempts from
//     --exclude and from ignore files, exactly as 1.23.0 did. OxGuard does not
//     pass --force-exclude: it would also drop root files whose names merely
//     equal a directory entry ("build", "skills", ".claude", "CVS"). The one
//     approved exception, minified root files, is handled in
//     securityRootTargets.
//   - OxGuard exclusions and securityBaselineExcludes are always passed.
//     Command-line exclusions outrank every ignore file, so a project
//     .semgrepignore can add exclusions but never bring these paths back.
//   - securityReplaceableDefaults are passed only when no .semgrepignore
//     exists at the scan root or at the project root (the Git root inside a Git
//     work tree). A project file, even an empty one, replaces them. 1.23.0
//     looked only at the scan root and Opengrep 2.1.0 only at the project
//     root; the union keeps both setups working.
//
// Known limit (deferred to a follow-up, branch feat/opengrep-nested-test-restoration):
// in a Git subproject whose only .semgrepignore is at the scan root, Opengrep
// 2.1.0 still applies its own built-in test paths below each root entry, so
// nested test directories such as src/tests and foo_test.go/ stay skipped (1.23.0
// scanned them). Workaround: an empty .semgrepignore at the Git root.
func securityScopeArgs(root string, excludes []string) (securityScope, error) {
	var scope securityScope
	projectRoot := root
	if project, inGit := detectGitProject(root); inGit {
		projectRoot = project.top
	} else {
		scope.args = append(scope.args, "--project-root", ".")
	}
	seen := map[string]bool{}
	add := func(pattern string) {
		if !seen[pattern] {
			seen[pattern] = true
			scope.args = append(scope.args, "--exclude", pattern)
		}
	}
	for _, entry := range excludes {
		pattern, err := opengrepExcludePattern(entry)
		if err != nil {
			return securityScope{}, err
		}
		add(pattern)
	}
	for _, pattern := range securityBaselineExcludes {
		add(pattern)
	}
	if !hasSecurityIgnoreFile(root) && !hasSecurityIgnoreFile(projectRoot) {
		for _, pattern := range securityReplaceableDefaults {
			add(pattern)
		}
	}
	return scope, nil
}

// securityScan is the complete Opengrep scope: flags followed by targets.
type securityScan struct {
	args []string
	// empty is set when every root entry was deliberately filtered (only minified
	// root files): there is nothing to scan and Opengrep must not be started,
	// because without targets it would scan the working directory.
	empty bool
}

// securityScanArgs combines scope flags and targets. The comparison helper
// calls this production function.
func securityScanArgs(root string, excludes []string) (securityScan, error) {
	scope, err := securityScopeArgs(root, excludes)
	if err != nil {
		return securityScan{}, err
	}
	// Keep security scans root-wide without asking the scanner to enumerate
	// node_modules first. Windows pnpm junctions can stall that enumeration
	// before Opengrep applies its --exclude filters.
	targets, err := securityRootTargets(root)
	if err != nil {
		return securityScan{}, err
	}
	if len(targets) == 0 {
		return securityScan{empty: true}, nil
	}
	return securityScan{args: append(append([]string{}, scope.args...), targets...)}, nil
}
