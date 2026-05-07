package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// runSetup bootstraps the full dev environment (idempotent).
func runSetup(root string, cfg config) int {
	fmt.Println("pyguard setup")
	fmt.Println("─────────")
	repoPkn := pyguardBinary(root)

	fmt.Println("  [1/8] Python 3.13...")
	if !checkPython() {
		return 2
	}

	fmt.Print("  [2/8] uv... ")
	out, _, err := RunSilent("", "uv", "--version")
	if err != nil {
		fmt.Println("not found — downloading...")
		if err := downloadUV(); err != nil {
			fmt.Printf("  [FAIL] uv install failed: %v\n", err)
			fmt.Println("         Manual install:")
			printUVInstallHint()
			return 2
		}
		fmt.Println("  [OK]   uv installed")
	} else {
		fmt.Printf("found (%s)\n", strings.TrimSpace(out))
	}

	// Reconcile before uv sync so a single sync covers newly added deps.
	fmt.Println("  [3/8] dev-group manifest...")
	if err := ensureUvDevDeps(root, cfg); err != nil {
		fmt.Printf("  [FAIL] dev-group check failed: %v\n", err)
		return 1
	}

	fmt.Println("  [4/8] uv sync...")
	if err := RunStreaming(root, "uv", "sync"); err != nil {
		fmt.Printf("  [FAIL] uv sync failed: %v\n", err)
		return 1
	}
	fmt.Println("  [OK]   .venv ready")

	fmt.Println("  [5/8] analysis scripts...")
	if err := ensureAnalysisScripts(root, cfg); err != nil {
		fmt.Printf("  [FAIL] analysis scripts: %v\n", err)
		return 1
	}

	baseline := filepath.Join(root, ".secrets.baseline")
	fmt.Print("  [6/8] .secrets.baseline... ")
	if _, err := os.Stat(baseline); os.IsNotExist(err) {
		fmt.Println("creating...")
		out, scanErr := RunCapture(root, "uv", "run", "detect-secrets", "scan")
		if scanErr != nil {
			fmt.Println("  [FAIL] detect-secrets scan failed (run: uv sync first)")
		} else {
			if writeErr := os.WriteFile(baseline, []byte(out), 0o644); writeErr != nil {
				fmt.Printf("  [FAIL] could not write baseline: %v\n", writeErr)
			} else {
				fmt.Println("  [OK]   .secrets.baseline created")
			}
		}
	} else {
		fmt.Println("exists")
	}

	fmt.Print("  [7/8] repo-local pyguard... ")
	repoStatus, err := ensureRepoPknBinary(root)
	if err != nil {
		fmt.Printf("failed\n  [FAIL] %v\n", err)
		return 2
	}
	fmt.Println(repoStatus)

	fmt.Print("  [8/8] pyguard on PATH... ")
	pathReady, pathStatus := installPkn(root)
	fmt.Println(pathStatus)

	// AI tool hooks
	fmt.Println()
	runHooks(root)

	fmt.Println()
	fmt.Println("  Setup complete!")
	fmt.Printf("  Repo-local binary: %s\n", repoPkn)
	if pathReady || repoPknOnPath(repoPkn) {
		fmt.Println("  Run: pyguard doctor    (verify environment)")
		fmt.Println("  Run: pyguard check     (run quality gates)")
	} else {
		fmt.Printf("  Run: %s doctor\n", repoPkn)
		fmt.Printf("  Run: %s check\n", repoPkn)
	}
	return 0
}

// ensureRepoPknBinary builds the repo-local pyguard binary when it is missing.
func ensureRepoPknBinary(root string) (string, error) {
	dst := pyguardBinary(root)
	if _, err := os.Stat(dst); err == nil {
		return "ready", nil
	}

	if _, err := exec.LookPath("go"); err != nil {
		return "", fmt.Errorf("repo-local binary missing at %s and Go is not on PATH", dst)
	}

	buildDir := filepath.Join(root, "tools", "pyguard")
	if err := RunStreaming(buildDir, "go", "build", "-o", filepath.Base(dst), "."); err != nil {
		return "", fmt.Errorf("could not build repo-local pyguard at %s: %w", dst, err)
	}

	if _, err := os.Stat(dst); err != nil {
		return "", fmt.Errorf("repo-local binary still missing after build: %w", err)
	}
	return "built", nil
}

// installPkn creates a PATH helper only when it is safe to do so.
// It never replaces an existing non-matching install.
func installPkn(root string) (bool, string) {
	if runtime.GOOS == "windows" {
		return false, "skipped (hooks use repo-local pyguard.exe)"
	}

	src := pyguardBinary(root)
	if _, err := os.Stat(src); os.IsNotExist(err) {
		return false, fmt.Sprintf("skipped (repo-local binary missing at %s)", src)
	}

	home, _ := os.UserHomeDir()
	localBin := filepath.Join(home, ".local", "bin")
	dst := filepath.Join(localBin, "pyguard")

	if err := os.MkdirAll(localBin, 0o755); err != nil {
		return false, fmt.Sprintf("skipped (cannot create dir: %v)", err)
	}

	if info, err := os.Lstat(dst); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			resolved, resolveErr := filepath.EvalSymlinks(dst)
			if resolveErr == nil && resolved == src {
				return true, "already linked"
			}
			if resolveErr != nil && errors.Is(resolveErr, os.ErrNotExist) {
				return false, "skipped (existing pyguard symlink is broken; not replacing automatically)"
			}
			return false, "skipped (existing pyguard symlink points elsewhere; not replacing)"
		}
		return false, "skipped (existing ~/.local/bin/pyguard is not managed by setup)"
	}

	if err := os.Symlink(src, dst); err != nil {
		return false, fmt.Sprintf("skipped (symlink failed: %v)", err)
	}
	return true, "installed"
}

func repoPknOnPath(repoPkn string) bool {
	pathPkn, err := exec.LookPath("pyguard")
	if err != nil {
		return false
	}
	if pathPkn == repoPkn {
		return true
	}
	resolved, err := filepath.EvalSymlinks(pathPkn)
	return err == nil && resolved == repoPkn
}

// downloadUV downloads the uv binary from GitHub releases and installs it.
// This bypasses PowerShell execution policies entirely — raw HTTP download.
func downloadUV() error {
	goos := runtime.GOOS
	goarch := runtime.GOARCH

	archMap := map[string]string{
		"amd64": "x86_64",
		"arm64": "aarch64",
		"386":   "i686",
	}
	arch, ok := archMap[goarch]
	if !ok {
		arch = goarch
	}

	var archiveName, binaryName string
	switch goos {
	case "windows":
		archiveName = fmt.Sprintf("uv-%s-pc-windows-msvc.zip", arch)
		binaryName = "uv.exe"
	case "darwin":
		archiveName = fmt.Sprintf("uv-%s-apple-darwin.tar.gz", arch)
		binaryName = "uv"
	default:
		archiveName = fmt.Sprintf("uv-%s-unknown-linux-gnu.tar.gz", arch)
		binaryName = "uv"
	}

	url := fmt.Sprintf("https://github.com/astral-sh/uv/releases/latest/download/%s", archiveName)
	fmt.Printf("         Downloading %s...\n", archiveName)

	resp, err := http.Get(url) //nolint:gosec,noctx
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed: HTTP %d", resp.StatusCode)
	}

	installDir := uvInstallDir()
	if err := os.MkdirAll(installDir, 0o755); err != nil {
		return fmt.Errorf("cannot create install dir %s: %w", installDir, err)
	}

	destPath := filepath.Join(installDir, binaryName)

	// Buffer the response body for zip (needs ReaderAt) or stream for tar
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read body failed: %w", err)
	}

	if goos == "windows" {
		if err := extractFromZip(body, binaryName, destPath); err != nil {
			return err
		}
	} else {
		if err := extractFromTarGz(body, binaryName, destPath); err != nil {
			return err
		}
	}

	if goos != "windows" {
		if err := os.Chmod(destPath, 0o755); err != nil {
			return fmt.Errorf("chmod failed: %w", err)
		}
	}

	// Add install dir to PATH for this process session
	current := os.Getenv("PATH")
	os.Setenv("PATH", installDir+string(os.PathListSeparator)+current) //nolint:errcheck

	fmt.Printf("         Installed to %s\n", destPath)
	fmt.Printf("         Note: add %s to your PATH permanently.\n", installDir)
	return nil
}

func uvInstallDir() string {
	if runtime.GOOS == "windows" {
		localApp := os.Getenv("LOCALAPPDATA")
		if localApp == "" {
			localApp = os.Getenv("USERPROFILE")
		}
		return filepath.Join(localApp, "uv", "bin")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "bin")
}

func printUVInstallHint() {
	switch runtime.GOOS {
	case "windows":
		fmt.Println("         PowerShell: powershell -ExecutionPolicy Bypass -c \"irm https://astral.sh/uv/install.ps1 | iex\"")
	default:
		fmt.Println("         Shell: curl -LsSf https://astral.sh/uv/install.sh | sh")
	}
}

func extractFromZip(data []byte, targetFile, destPath string) error {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return fmt.Errorf("zip open: %w", err)
	}
	for _, f := range zr.File {
		if filepath.Base(f.Name) == targetFile {
			rc, err := f.Open()
			if err != nil {
				return err
			}
			defer rc.Close()
			return writeFileTo(rc, destPath)
		}
	}
	return fmt.Errorf("%s not found in zip", targetFile)
}

func extractFromTarGz(data []byte, targetFile, destPath string) error {
	gr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("gzip open: %w", err)
	}
	defer gr.Close()

	tr := tar.NewReader(gr)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("tar read: %w", err)
		}
		if filepath.Base(header.Name) == targetFile {
			return writeFileTo(tr, destPath)
		}
	}
	return fmt.Errorf("%s not found in tar.gz", targetFile)
}

func writeFileTo(r io.Reader, destPath string) error {
	f, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("create %s: %w", destPath, err)
	}
	defer f.Close()
	if _, err := io.Copy(f, r); err != nil {
		return fmt.Errorf("write: %w", err)
	}
	return nil
}
