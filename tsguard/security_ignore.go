package main

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"unicode/utf8"
)

// These are the 1.23.0 cli/src/semgrep/templates/.semgrepignore defaults.
const legacySecurityIgnores = `.git
.svn
.hg
_darcs
CVS
build/
vendor/
dist/
*.min.js
.env/
.tox/
node_modules/
.npm/
.yarn/
.venv/
_opam/
_build/
_cargo/
test/
tests/
testsuite/
*_test.go
`

// legacyIgnorePatterns follows 1.23.0 ignores.py: only the cwd file is read;
// negations/ranges are discarded, includes are relative to their containing file,
// and * matches separators too (Python fnmatch, not gitignore).
func legacyIgnorePatterns(file string, active map[string]bool) ([]*regexp.Regexp, error) {
	file, err := filepath.Abs(file)
	if err != nil {
		return nil, err
	}
	if active[file] {
		return nil, fmt.Errorf("cyclic security ignore include: %s", file)
	}
	active[file] = true
	defer delete(active, file)
	data, err := os.ReadFile(file)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return parseLegacyIgnores(string(data), filepath.Dir(file), active)
}

func parseLegacyIgnores(data, base string, active map[string]bool) ([]*regexp.Regexp, error) {
	if !utf8.ValidString(data) {
		return nil, fmt.Errorf("security ignore is not UTF-8")
	}
	var result []*regexp.Regexp
	for _, line := range strings.Split(data, "\n") {
		// A comment starts only at the beginning or following whitespace.
		for i, c := range line {
			if c == '#' && (i == 0 || strings.ContainsRune(" \t\r\n\v\f", rune(line[i-1]))) {
				line = line[:i]
				break
			}
		}
		line = strings.TrimRight(line, " \t\r\n\v\f")
		if line == "" || strings.HasPrefix(line, "!") || regexp.MustCompile(`\[.*\]`).MatchString(line) {
			continue
		}
		if strings.HasPrefix(line, ":include ") {
			include := line[9:]
			if !filepath.IsAbs(include) {
				include = filepath.Join(base, include)
			}
			sub, err := legacyIgnorePatterns(include, active)
			if err != nil {
				return nil, err
			}
			result = append(result, sub...)
			continue
		}
		if line == ":" {
			continue
		}
		if strings.HasPrefix(line, ":") {
			return nil, fmt.Errorf("unknown security ignore directive: %s", line)
		}
		var unescaped strings.Builder
		escape := false
		for _, c := range line {
			if escape {
				unescaped.WriteRune(c)
				escape = false
			} else if c == '\\' {
				escape = true
			} else {
				unescaped.WriteRune(c)
			}
		}
		pat := unescaped.String()
		anchored := strings.Contains(strings.TrimRight(pat, "/"), "/")
		folder := strings.HasSuffix(pat, "/")
		if anchored && (strings.HasPrefix(pat, "./") || strings.HasPrefix(pat, "/")) {
			pat = strings.TrimLeft(strings.TrimLeft(pat, "."), "/")
		}
		patterns := []string{legacyPatternJoin(pat, "**")}
		if !folder {
			patterns = append(patterns, pat)
		}
		if !anchored {
			unanchored := legacyPatternJoin("**", pat)
			patterns = append(patterns, legacyPatternJoin(unanchored, "**"))
			if !folder {
				patterns = append(patterns, unanchored)
			}
		}
		for _, p := range patterns {
			var expression strings.Builder
			if runtime.GOOS == "windows" {
				expression.WriteString("(?i)")
			}
			expression.WriteString("(?s)^")
			for _, c := range p {
				switch c {
				case '*':
					expression.WriteString(".*")
				case '?':
					expression.WriteString(".")
				default:
					expression.WriteString(regexp.QuoteMeta(string(c)))
				}
			}
			expression.WriteString("$")
			result = append(result, regexp.MustCompile(expression.String()))
		}
	}
	return result, nil
}

// Unlike filepath.Join, Python's os.path.join does not clean repeated slashes.
func legacyPatternJoin(base, child string) string {
	if base == "" || strings.HasPrefix(child, "/") {
		return child
	}
	if strings.HasSuffix(base, "/") {
		return base + child
	}
	return base + "/" + child
}

// Materialize literal exclusions rather than translating globs to a different
// language. A random filename replaces ALL nested .semgrepignore discovery.
// File-backed exclusions keep argv bounded even on Windows. No rules are changed.
func securityIgnoreSnapshot(root string) (args []string, cleanup func(), err error) {
	root, err = filepath.Abs(root)
	if err != nil {
		return nil, nil, err
	}
	var patterns []*regexp.Regexp
	file := filepath.Join(root, ".semgrepignore")
	if _, e := os.Stat(file); os.IsNotExist(e) {
		patterns, err = parseLegacyIgnores(legacySecurityIgnores, root, map[string]bool{})
	} else {
		patterns, err = legacyIgnorePatterns(file, map[string]bool{})
	}
	if err != nil {
		return nil, nil, err
	}
	anchor := root
	// Opengrep anchors ignore files at the Git root, including when cwd is below it.
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	cmd.Dir = root
	if out, e := cmd.Output(); e == nil {
		anchor = strings.TrimSpace(string(out))
	} else {
		args = append(args, "--project-root", root)
	}
	snapshot, err := os.CreateTemp(anchor, ".oxguard-security-ignore-*")
	if err != nil {
		return nil, nil, err
	}
	cleanup = func() { os.Remove(snapshot.Name()) }
	defer func() {
		snapshot.Close()
		if err != nil {
			os.Remove(snapshot.Name())
		}
	}()
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, e := filepath.Rel(root, path)
		if e != nil {
			return e
		}
		if rel == "." {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return nil
		}
		if entry.IsDir() && (entry.Name() == "node_modules" || entry.Name() == ".git") {
			return filepath.SkipDir
		}
		// The legacy parser filters files, not directory names; trailing / means
		// descendants only. Do not prune using the bare directory name.
		if entry.IsDir() {
			return nil
		}
		for _, pattern := range patterns {
			if pattern.MatchString(filepath.ToSlash(rel)) {
				anchored, e := filepath.Rel(anchor, path)
				if e != nil {
					return e
				}
				anchored = filepath.ToSlash(anchored)
				if strings.ContainsAny(anchored, "\r\n") {
					return fmt.Errorf("security ignore path contains newline")
				}
				escaped := strings.NewReplacer("\\", "\\\\", "*", "\\*", "?", "\\?", "[", "\\[", "]", "\\]", "#", "\\#", "!", "\\!", " ", "\\ ").Replace(anchored)
				_, e = fmt.Fprintln(snapshot, "/"+escaped)
				return e
			}
		}
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	args = append(args, "--semgrepignore-filename", filepath.Base(snapshot.Name()))
	return args, cleanup, nil
}
