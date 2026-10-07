package main

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestMaterializeAnalysisHelperUsesOnlyEmbeddedFiles(t *testing.T) {
	path, cleanup, err := materializeAnalysisHelper("tools/analysis/check_type_complexity.py")
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()

	if info, err := os.Stat(filepath.Dir(path)); err != nil || !info.IsDir() {
		t.Fatalf("missing private directory: %v", err)
	} else if runtime.GOOS != "windows" && info.Mode().Perm() != 0o700 {
		t.Fatalf("helper directory permissions: %v", info.Mode().Perm())
	}
	for _, name := range []string{
		"_paths.py", "analyze_criticality.py", "check_halstead.py",
		"check_secrets.py", "check_type_complexity.py", "vulture_report.py",
	} {
		want, err := analysisScripts.ReadFile("analysis/" + name)
		if err != nil {
			t.Fatal(err)
		}
		private := filepath.Join(filepath.Dir(path), name)
		got, err := os.ReadFile(private)
		if err != nil || !bytes.Equal(got, want) {
			t.Fatalf("%s differs from embedded source: %v", name, err)
		}
		info, err := os.Lstat(private)
		if err != nil || !info.Mode().IsRegular() {
			t.Fatalf("%s is not a regular file: %v", name, err)
		}
		if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
			t.Fatalf("%s permissions: %v", name, info.Mode().Perm())
		}
	}
	cleanup()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("private helper survives cleanup: %v", err)
	}
}

func TestMaterializeAnalysisHelperRejectsUnknownPaths(t *testing.T) {
	for _, path := range []string{
		"check_secrets.py",
		"tools/analysis/../malicious.py",
		"tools/analysis/nested/malicious.py",
		"tools/analysis/malicious.py",
		"tools/analysis/_paths.py/../check_secrets.py",
	} {
		_, _, err := materializeAnalysisHelper(path)
		if err == nil {
			t.Fatalf("accepted unbundled helper path: %s", path)
		}
	}
}

func TestRunnerIgnoresProjectHelperChangesAndSymlinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake uv shim is a POSIX shell script")
	}
	bin := t.TempDir()
	shim := filepath.Join(bin, "uv")
	// A fake interpreter records the actual executed helper and its contents.
	// This tests the execution boundary without installing Python dependencies.
	script := "#!/bin/sh\n" +
		"printf '%s' \"$3\" > \"$PYGUARD_HELPER_PATH\"\n" +
		"cp \"$3\" \"$PYGUARD_HELPER_COPY\"\n" +
		"printf '{\"schema_version\":\"1\",\"findings\":[],\"measurements\":[],\"artifacts\":[]}\\n'\n"
	if err := os.WriteFile(shim, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	for _, name := range []string{
		"check_type_complexity.py", "check_halstead.py",
		"check_secrets.py", "analyze_criticality.py", "vulture_report.py",
	} {
		for _, form := range []string{"modified", "symlink"} {
			t.Run(name+"/"+form, func(t *testing.T) {
				root := t.TempDir()
				helperDir := filepath.Join(root, "tools", "analysis")
				if err := os.MkdirAll(helperDir, 0o755); err != nil {
					t.Fatal(err)
				}
				payload := filepath.Join(root, "attacker.py")
				if err := os.WriteFile(payload, []byte("MALICIOUS_MARKER\n"), 0o644); err != nil {
					t.Fatal(err)
				}
				localHelper := filepath.Join(helperDir, name)
				if form == "symlink" {
					if err := os.Symlink(payload, localHelper); err != nil {
						t.Skipf("symlinks unavailable: %v", err)
					}
				} else if err := os.WriteFile(localHelper, []byte("MALICIOUS_MARKER\n"), 0o644); err != nil {
					t.Fatal(err)
				}

				recordedPath := filepath.Join(t.TempDir(), "selected.txt")
				recordedCopy := filepath.Join(t.TempDir(), "executed.py")
				t.Setenv("PYGUARD_HELPER_PATH", recordedPath)
				t.Setenv("PYGUARD_HELPER_COPY", recordedCopy)
				r := &Runner{root: root, timeout: 5, outputMode: "json", result: newRunResult("types")}
				res := r.RunTool(toolSpec{gate: "types", adapter: "owned"}, "owned helper",
					"uv", "run", "python", "tools/analysis/"+name, "--json")
				if !res.ok {
					t.Fatalf("trusted helper did not run: %s", res.message)
				}
				selected, err := os.ReadFile(recordedPath)
				if err != nil {
					t.Fatal(err)
				}
				if strings.HasPrefix(string(selected), root+string(os.PathSeparator)) {
					t.Fatalf("executed project-local helper: %s", selected)
				}
				if _, err := os.Stat(string(selected)); !os.IsNotExist(err) {
					t.Fatalf("execution copy not cleaned up: %v", err)
				}
				got, err := os.ReadFile(recordedCopy)
				if err != nil {
					t.Fatal(err)
				}
				want, err := analysisScripts.ReadFile("analysis/" + name)
				if err != nil || !bytes.Equal(want, got) {
					t.Fatalf("did not run trusted bytes for %s: %v", name, err)
				}
				if bytes.Contains(got, []byte("MALICIOUS_MARKER")) {
					t.Fatalf("ran tampered project source: %s", name)
				}
			})
		}
	}
}
