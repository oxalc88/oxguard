package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
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

// scopeToolError marks a failure of Opengrep or Git while listing nested test paths.
type scopeToolError struct{ err error }

func (e *scopeToolError) Error() string { return e.err.Error() }
func (e *scopeToolError) Unwrap() error { return e.err }

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
	// restoreNestedTests is set when a .semgrepignore exists at the scan root of
	// a Git subproject but not at the Git root. Opengrep then still applies its
	// built-in test paths below each root entry, which 1.23.0 did not.
	restoreNestedTests bool
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
//   - When only the scan root of a Git subproject has the file, Opengrep still
//     skips nested test directories; restoreNestedTests asks the caller to add
//     them as explicit targets (see nestedTestTargets).
func securityScopeArgs(root string, excludes []string) (securityScope, error) {
	var scope securityScope
	projectRoot := root
	project, inGit := detectGitProject(root)
	if inGit {
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
	atRoot, atProject := hasSecurityIgnoreFile(root), hasSecurityIgnoreFile(projectRoot)
	if !atRoot && !atProject {
		for _, pattern := range securityReplaceableDefaults {
			add(pattern)
		}
	}
	scope.restoreNestedTests = inGit && atRoot && !atProject && !sameDir(root, projectRoot)
	return scope, nil
}

func sameDir(a, b string) bool {
	x, errX := filepath.EvalSymlinks(a)
	y, errY := filepath.EvalSymlinks(b)
	if errX != nil || errY != nil {
		return filepath.Clean(a) == filepath.Clean(b)
	}
	return x == y
}

// securityExtraTargetBytes bounds the argv added by nestedTestTargets. Windows
// limits a command line to 32,767 characters, which also covers the engine
// path, the exclusion patterns and the root targets; 16,000 bytes of extra
// targets (an upper bound on UTF-16 units) leaves room for them on every
// platform, so behavior does not differ by OS.
const securityExtraTargetBytes = 16000

// securityListPattern is a generic-language pattern no file is expected to
// contain. The listing scan reads only paths.scanned; matches are ignored.
const securityListPattern = "OXGUARD_TARGET_LISTING_NEVER_MATCHES_2f9c1d"

// listSelectedFiles asks Opengrep which files it selects for args. The answer
// is the JSON "paths.scanned" array of a scan with a language-agnostic pattern,
// not the newline-separated --x-ls output: JSON strings are an unambiguous
// serialization, so file names holding newlines, carriage returns, U+0085,
// U+2028, quotes or spaces round-trip exactly and cannot forge another path. It
// is a variable so tests can replace the engine.
var listSelectedFiles = func(engine, root string, args []string) ([]string, error) {
	query := append([]string{"scan", "--json", "--quiet", "--lang", "generic", "--pattern", securityListPattern}, args...)
	cmd := exec.Command(engine, query...) //nolint:gosec
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "OPENGREP_ENABLE_VERSION_CHECK=false")
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("Opengrep could not list scan targets: %v: %s", err, strings.TrimSpace(stderr.String()))
	}
	var report struct {
		Paths struct {
			Scanned []string `json:"scanned"`
		} `json:"paths"`
		Errors []json.RawMessage `json:"errors"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		return nil, fmt.Errorf("Opengrep target listing was not valid JSON: %v", err)
	}
	if len(report.Errors) > 0 {
		return nil, fmt.Errorf("Opengrep reported %d error(s) while listing scan targets; the listing may be incomplete: %s", len(report.Errors), report.Errors[0])
	}
	files := make([]string, 0, len(report.Paths.Scanned))
	for _, file := range report.Paths.Scanned {
		files = append(files, filepath.ToSlash(file))
	}
	return files, nil
}

// gitIgnoredPaths returns the subset of paths that .gitignore rules exclude.
// check-ignore reports no tracked file, matching Opengrep's own Git handling.
// Paths travel NUL-separated both ways, so any file name is safe.
var gitIgnoredPaths = func(root string, paths []string) (map[string]bool, error) {
	ignored := map[string]bool{}
	if len(paths) == 0 {
		return ignored, nil
	}
	cmd := exec.Command("git", "-C", root, "check-ignore", "-z", "--stdin")
	cmd.Env = append(os.Environ(), "LC_ALL=C", "GIT_OPTIONAL_LOCKS=0")
	cmd.Stdin = strings.NewReader(strings.Join(paths, "\x00") + "\x00")
	out, err := cmd.Output()
	var exit *exec.ExitError
	if err != nil && !(errors.As(err, &exit) && exit.ExitCode() == 1) {
		return nil, fmt.Errorf("git check-ignore failed: %v", err)
	}
	for _, p := range strings.Split(string(out), "\x00") {
		if p != "" {
			ignored[p] = true
		}
	}
	return ignored, nil
}

// builtinTestPath is one entry of Opengrep 2.1.0's built-in "Common test paths"
// (src/targeting/Semgrepignore.ml, default_semgrepignore_for_semgrep_scan):
// test/, tests/, testsuite/ and *_test.go. A trailing slash makes an entry
// directory-only; without it the entry matches files and directories alike, so
// "*_test.go" also hides a directory named foo_test.go. Names are compared
// case-sensitively, as Opengrep does (a directory "Tests" or "TEST" is not
// hidden natively and is not restored). The list is securityReplaceableDefaults
// itself, which the harness compares with the pinned engine's embedded copy.
type builtinTestPath struct {
	glob    string
	dirOnly bool
}

func builtinTestPaths() []builtinTestPath {
	var patterns []builtinTestPath
	for _, entry := range securityReplaceableDefaults {
		patterns = append(patterns, builtinTestPath{glob: strings.TrimSuffix(entry, "/"), dirOnly: strings.HasSuffix(entry, "/")})
	}
	return patterns
}

// hiddenByBuiltins reports whether a path segment matches a built-in test path.
// path.Match is a single-segment glob with the same "*" meaning as gitignore.
func hiddenByBuiltins(name string, isDir bool) bool {
	for _, pattern := range builtinTestPaths() {
		if pattern.dirOnly && !isDir {
			continue
		}
		if ok, err := path.Match(pattern.glob, name); err == nil && ok {
			return true
		}
	}
	return false
}

// nestedTarget validates one derived target against the scan root before it can
// be passed to Opengrep: a clean relative path with no "." or ".." segment, no
// NUL, no absolute or volume form, that exists below the root with the
// expected type, is not a symlink and does not reach outside the root through
// a symlinked ancestor. A failure is fatal; nothing is guessed.
func nestedTarget(root, rel string, wantDir bool) (string, error) {
	fail := func(reason string) (string, error) {
		return "", &scopeToolError{fmt.Errorf("Refusing nested test target %q: %s", rel, reason)}
	}
	if rel == "" || strings.ContainsRune(rel, 0) {
		return fail("empty or NUL-containing path")
	}
	if strings.HasPrefix(rel, "/") || filepath.IsAbs(filepath.FromSlash(rel)) || filepath.VolumeName(filepath.FromSlash(rel)) != "" {
		return fail("absolute path")
	}
	for _, segment := range strings.Split(rel, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return fail("path is not a clean relative path")
		}
	}
	local := filepath.FromSlash(rel)
	info, err := os.Lstat(filepath.Join(root, local))
	if err != nil {
		return fail("the listed path no longer exists")
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fail("it is a symbolic link")
	}
	if wantDir != info.IsDir() || (!wantDir && !info.Mode().IsRegular()) {
		return fail("unexpected file type")
	}
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return fail("the scan root cannot be resolved")
	}
	realParent, err := filepath.EvalSymlinks(filepath.Dir(filepath.Join(root, local)))
	if err != nil || (realParent != realRoot && !strings.HasPrefix(realParent, realRoot+string(os.PathSeparator))) {
		return fail("it resolves outside the scan root")
	}
	return "." + string(os.PathSeparator) + local, nil
}

// nestedTestTargets lists explicit targets that restore 1.23.0 behavior for
// nested test paths when a Git subproject's own .semgrepignore replaced the
// defaults. Opengrep decides what is selected: it lists the files it would scan
// with built-ins disabled (--project-root makes the scan root the project, so
// the scan-root file replaces them) honoring every .semgrepignore and every
// exclusion; OxGuard only reads the paths.
//
//   - A directory below a root entry whose name matches a built-in test path
//     (test, tests, testsuite, and any name matching *_test.go) becomes an
//     explicit directory target. Opengrep does not apply its built-ins to the
//     target itself, still applies .gitignore, project ignore files and every
//     exclusion inside it, and skips matching directories nested in it
//     natively, so targets never overlap and nothing is reported twice.
//   - A nested file matching *_test.go becomes an explicit file target, which
//     bypasses all filters, so Git first drops files .gitignore excludes (the
//     listing already applied .semgrepignore and the exclusions).
//
// Every target is validated by nestedTarget and is a listed path or a
// directory prefix of one. node_modules is never listed: the listing covers
// only the root targets, and excluded paths are pruned by Opengrep. Symlinked
// directories are not followed. The result is bounded by
// securityExtraTargetBytes; a larger set fails closed with a pointer to the
// workaround.
func nestedTestTargets(engine, root string, scopeArgs, rootTargets []string) ([]string, error) {
	query := append([]string{"--project-root", "."}, scopeArgs...)
	listed, err := listSelectedFiles(engine, root, append(query, rootTargets...))
	if err != nil {
		return nil, &scopeToolError{err}
	}
	dirs, files := map[string]bool{}, map[string]bool{}
	for _, listedPath := range listed {
		if strings.ContainsRune(listedPath, 0) {
			return nil, &scopeToolError{fmt.Errorf("Opengrep listed a path with a NUL byte, which no file name can hold: %q", listedPath)}
		}
		segments := strings.Split(listedPath, "/")
		for i := 1; i < len(segments)-1; i++ {
			if hiddenByBuiltins(segments[i], true) {
				dirs[strings.Join(segments[:i+1], "/")] = true
			}
		}
		if len(segments) >= 2 && hiddenByBuiltins(segments[len(segments)-1], false) {
			files[listedPath] = true
		}
	}
	candidates := sortedKeys(files)
	ignored, err := gitIgnoredPaths(root, candidates)
	if err != nil {
		return nil, &scopeToolError{err}
	}
	var targets []string
	for _, dir := range sortedKeys(dirs) {
		target, err := nestedTarget(root, dir, true)
		if err != nil {
			return nil, err
		}
		targets = append(targets, target)
	}
	for _, file := range candidates {
		if ignored[file] {
			continue
		}
		target, err := nestedTarget(root, file, false)
		if err != nil {
			return nil, err
		}
		targets = append(targets, target)
	}
	size := 0
	for _, target := range targets {
		size += len(target) + 3
	}
	if size > securityExtraTargetBytes {
		return nil, &scopeConfigError{fmt.Sprintf("This Git subproject has too many nested test paths (%d targets) to restore explicitly; add an empty .semgrepignore at the Git root to replace Opengrep's built-in test paths", len(targets))}
	}
	return targets, nil
}

func sortedKeys(set map[string]bool) []string {
	keys := make([]string, 0, len(set))
	for key := range set {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
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
func securityScanArgs(root string, excludes []string, engine string) (securityScan, error) {
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
	args := append([]string{}, scope.args...)
	if scope.restoreNestedTests {
		extra, err := nestedTestTargets(engine, root, scope.args, targets)
		if err != nil {
			return securityScan{}, err
		}
		targets = append(targets, extra...)
	}
	return securityScan{args: append(args, targets...)}, nil
}
