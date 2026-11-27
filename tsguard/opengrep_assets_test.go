package main

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestNativeManifestIsCurrent(t *testing.T) {
	data, err := os.ReadFile("../npm/native-tools.json")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(nativeToolsJSON, data) {
		t.Fatal("run go generate in tsguard after editing npm/native-tools.json")
	}
}

func TestOpengrepZipExtraction(t *testing.T) {
	for _, scenario := range []string{"valid", "traversal", "symlink", "duplicate", "missing", "oversized", "bad_archive", "bad_executable"} {
		t.Run(scenario, func(t *testing.T) {
			root := t.TempDir()
			archive := filepath.Join(root, "input.zip")
			f, err := os.Create(archive)
			if err != nil {
				t.Fatal(err)
			}
			z := zip.NewWriter(f)
			content := []byte("trusted executable")
			sum := sha256.Sum256(content)
			pin := nativeFile{hex.EncodeToString(sum[:]), int64(len(content))}
			name := "opengrep.exe"
			if scenario == "traversal" {
				name = "../opengrep.exe"
			}
			header := &zip.FileHeader{Name: name, Method: zip.Deflate}
			header.SetMode(0o700)
			if scenario == "symlink" {
				header.SetMode(os.ModeSymlink | 0o777)
			}
			output, err := z.CreateHeader(header)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := output.Write(content); err != nil {
				t.Fatal(err)
			}
			if scenario == "duplicate" {
				duplicateHeader := *header
				output, err = z.CreateHeader(&duplicateHeader)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := output.Write(content); err != nil {
					t.Fatal(err)
				}
			}
			if err := z.Close(); err != nil {
				t.Fatal(err)
			}
			f.Close()
			data, _ := os.ReadFile(archive)
			digest := sha256.Sum256(data)
			asset := nativeAsset{SHA256: hex.EncodeToString(digest[:]), Size: int64(len(data)), Files: map[string]nativeFile{"opengrep.exe": pin}}
			// Also whitelist traversal to ensure paths are rejected independently of allowlisting.
			if scenario == "traversal" {
				asset.Files = map[string]nativeFile{name: pin}
			}
			if scenario == "duplicate" {
				asset.Files["runtime.dll"] = pin
			}
			if scenario == "missing" {
				asset.Files["runtime.dll"] = pin
			}
			if scenario == "oversized" {
				pin.Size = maxOpengrepBytes + 1
				asset.Files[name] = pin
			}
			if scenario == "bad_archive" {
				asset.SHA256 = "invalid"
			}
			if scenario == "bad_executable" {
				pin.SHA256 = "invalid"
				asset.Files[name] = pin
			}
			target := filepath.Join(root, "out")
			os.Mkdir(target, 0o700)
			err = extractOpengrepZip(archive, target, asset)
			if scenario == "valid" {
				if err != nil {
					t.Fatal(err)
				}
				if err := verifyNativeFile(filepath.Join(target, name), pin); err != nil {
					t.Fatal(err)
				}
			} else if err == nil {
				t.Fatalf("accepted %s", scenario)
			}
		})
	}
}

func TestPackagedEngineRejectsEnvironmentSubstitution(t *testing.T) {
	t.Setenv("TSGUARD_RUNTIME", t.TempDir())
	file := filepath.Join(t.TempDir(), "opengrep")
	if err := os.WriteFile(file, []byte("project supplied executable"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TSGUARD_OPENGREP", file)
	if err := verifyOpengrepBinary(opengrepBinaryPath(t.TempDir())); err == nil {
		t.Fatal("environment supplied executable accepted")
	}
}

// Opt-in network smoke for standalone setup, independent of npm packaging.
func TestRealStandaloneOpengrepSetup(t *testing.T) {
	if os.Getenv("OXGUARD_TEST_REAL_OPENGREP") != "1" {
		t.Skip("set OXGUARD_TEST_REAL_OPENGREP=1 for verified upstream download smoke")
	}
	cache := t.TempDir()
	t.Setenv("TSGUARD_RUNTIME", "")
	t.Setenv("TSGUARD_OPENGREP", "")
	t.Setenv("XDG_CACHE_HOME", cache)
	t.Setenv("HOME", cache)
	t.Setenv("LocalAppData", cache)
	if !ensureOpengrep(t.TempDir(), config{assumeYes: true}) {
		t.Fatal("standalone setup failed")
	}
	if err := verifyOpengrepBinary(opengrepBinaryPath("")); err != nil {
		t.Fatal(err)
	}
	if !ensureOpengrep(t.TempDir(), config{assumeYes: true}) {
		t.Fatal("standalone cache reuse failed")
	}
	command := exec.Command(opengrepBinaryPath(""), "--version")
	command.Env = append(os.Environ(), "OPENGREP_ENABLE_VERSION_CHECK=false")
	output, err := command.CombinedOutput()
	if err != nil || strings.TrimSpace(string(output)) != strings.TrimPrefix(opengrepVersion, "v") {
		t.Fatalf("standalone executable failed: %v: %s", err, output)
	}

}
