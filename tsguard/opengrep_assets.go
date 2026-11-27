package main

import (
	"archive/zip"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// npm/native-tools.json is authoritative. Regenerate and commit the embedded copy.
//
//go:generate node ../npm/sync-native.cjs
//go:embed native-tools.generated.json
var nativeToolsJSON []byte

type nativeFile struct {
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}
type nativeAsset struct {
	Asset  string                `json:"asset"`
	SHA256 string                `json:"sha256"`
	Size   int64                 `json:"size"`
	Files  map[string]nativeFile `json:"files"`
}

var nativeTools = func() struct {
	Version   string                 `json:"version"`
	Platforms map[string]nativeAsset `json:"platforms"`
} {
	var manifest struct {
		Version   string                 `json:"version"`
		Platforms map[string]nativeAsset `json:"platforms"`
	}
	if err := json.Unmarshal(nativeToolsJSON, &manifest); err != nil {
		panic(err)
	}
	return manifest
}()
var opengrepVersion = nativeTools.Version

func currentOpengrepAsset() (nativeAsset, bool) {
	osName, arch := runtime.GOOS, runtime.GOARCH
	if osName == "windows" {
		osName = "win32"
	}
	if arch == "amd64" {
		arch = "x64"
	}
	asset, ok := nativeTools.Platforms[osName+"-"+arch]
	return asset, ok
}
func opengrepAssetName() (string, bool) {
	asset, ok := currentOpengrepAsset()
	return asset.Asset, ok
}

func verifyNativeFile(path string, pin nativeFile) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Size() != pin.Size {
		return fmt.Errorf("Opengrep entry is not a regular file of pinned size: %s", filepath.Base(path))
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	if hex.EncodeToString(h.Sum(nil)) != pin.SHA256 {
		return fmt.Errorf("Opengrep SHA-256 mismatch: %s", filepath.Base(path))
	}
	return nil
}

// Verify the executable and every adjacent runtime DLL, including npm mode.
// Merely setting TSGUARD_RUNTIME/TSGUARD_OPENGREP cannot authorize arbitrary bytes.
func verifyOpengrepBinary(path string) error {
	if path == "" {
		return fmt.Errorf("user cache directory unavailable")
	}
	asset, ok := currentOpengrepAsset()
	if !ok {
		return fmt.Errorf("platform has no pinned Opengrep asset")
	}
	for name, pin := range asset.Files {
		file := filepath.Join(filepath.Dir(path), name)
		if name == "opengrep" || name == "opengrep.exe" {
			file = path
		}
		if err := verifyNativeFile(file, pin); err != nil {
			return err
		}
	}
	return nil
}

const maxOpengrepBytes int64 = 220 * 1024 * 1024

// Read verified flat entries into a private staging directory. Reject unexpected,
// duplicate, nonregular, oversized and missing entries before installation.
func extractOpengrepZip(archive, directory string, asset nativeAsset) error {
	if asset.Size < 0 || asset.Size > maxOpengrepBytes {
		return fmt.Errorf("oversized Opengrep ZIP")
	}
	if err := verifyNativeFile(archive, nativeFile{asset.SHA256, asset.Size}); err != nil {
		return err
	}
	z, err := zip.OpenReader(archive)
	if err != nil {
		return err
	}
	defer z.Close()
	if len(z.File) != len(asset.Files) || len(z.File) > 16 {
		return fmt.Errorf("unexpected ZIP entry count")
	}
	seen := map[string]bool{}
	var total int64
	for _, entry := range z.File {
		pin, ok := asset.Files[entry.Name]
		if !ok || seen[entry.Name] || strings.ContainsAny(entry.Name, "/\\:") || entry.Name == "." || entry.Name == ".." ||
			!entry.Mode().IsRegular() || entry.UncompressedSize64 != uint64(pin.Size) || pin.Size < 0 || pin.Size > maxOpengrepBytes {
			return fmt.Errorf("unsafe Opengrep ZIP entry: %s", entry.Name)
		}
		total += pin.Size
		if total > maxOpengrepBytes {
			return fmt.Errorf("oversized Opengrep ZIP")
		}
		seen[entry.Name] = true
		input, err := entry.Open()
		if err != nil {
			return err
		}
		file := filepath.Join(directory, entry.Name)
		output, err := os.OpenFile(file, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o700)
		if err != nil {
			input.Close()
			return err
		}
		_, copyErr := io.Copy(output, io.LimitReader(input, pin.Size+1))
		closeErr := output.Close()
		input.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
		if err := verifyNativeFile(file, pin); err != nil {
			return err
		}
	}
	return nil
}

func installOpengrepAsset(archive, directory string) error {
	asset, ok := currentOpengrepAsset()
	if !ok {
		return fmt.Errorf("unsupported platform")
	}
	if err := verifyNativeFile(archive, nativeFile{asset.SHA256, asset.Size}); err != nil {
		return err
	}
	if !strings.HasSuffix(asset.Asset, ".zip") {
		if err := verifyNativeFile(archive, asset.Files["opengrep"]); err != nil {
			return err
		}
		if err := os.Chmod(archive, 0o700); err != nil {
			return err
		}
		return os.Rename(archive, filepath.Join(directory, "opengrep"))
	}
	staging, err := os.MkdirTemp(directory, ".extract-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(staging)
	if err := extractOpengrepZip(archive, staging, asset); err != nil {
		return err
	}
	for name := range asset.Files {
		if name == "opengrep.exe" {
			continue
		}
		if err := os.Rename(filepath.Join(staging, name), filepath.Join(directory, name)); err != nil {
			return err
		}
	}
	return os.Rename(filepath.Join(staging, "opengrep.exe"), filepath.Join(directory, "opengrep.exe"))
}
