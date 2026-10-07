package main

import (
	"bytes"
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

//go:embed analysis/*.py
var analysisScripts embed.FS

// ensureAnalysisScripts deploys reference copies into <root>/tools/analysis/.
// RunTool never executes these project-local files; it uses private embedded copies.
// Behavior per file:
//   - missing  → write
//   - identical SHA256 → no-op
//   - differs  → overwrite when assumeYes/CI, else prompt [O]verwrite / [s]kip
func ensureAnalysisScripts(root string, cfg config) error {
	targetDir := filepath.Join(root, "tools", "analysis")
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return fmt.Errorf("creating tools/analysis: %w", err)
	}

	entries, err := fs.ReadDir(analysisScripts, "analysis")
	if err != nil {
		return fmt.Errorf("reading embedded analysis scripts: %w", err)
	}

	updated, skipped, unchanged := 0, 0, 0

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".py") {
			continue
		}

		embeddedData, err := analysisScripts.ReadFile("analysis/" + entry.Name())
		if err != nil {
			return fmt.Errorf("reading embedded %s: %w", entry.Name(), err)
		}

		target := filepath.Join(targetDir, entry.Name())
		diskData, readErr := os.ReadFile(target)

		switch {
		case readErr != nil && os.IsNotExist(readErr):
			// File missing — write without prompting.
			if err := os.WriteFile(target, embeddedData, 0o644); err != nil {
				return fmt.Errorf("writing %s: %w", entry.Name(), err)
			}
			fmt.Printf("  [NEW]  tools/analysis/%s\n", entry.Name())
			updated++

		case readErr != nil:
			return fmt.Errorf("reading %s: %w", target, readErr)

		case bytes.Equal(embeddedData, diskData):
			unchanged++

		default:
			// File exists but differs from embedded version.
			fmt.Printf("  tools/analysis/%s differs from embedded version.\n", entry.Name())
			if confirmYesNo("    Overwrite?", true, cfg.assumeYes) {
				if err := os.WriteFile(target, embeddedData, 0o644); err != nil {
					return fmt.Errorf("overwriting %s: %w", entry.Name(), err)
				}
				fmt.Printf("  [UPD]  tools/analysis/%s\n", entry.Name())
				updated++
			} else {
				fmt.Printf("  [SKIP] tools/analysis/%s\n", entry.Name())
				skipped++
			}
		}
	}

	if unchanged > 0 && updated == 0 && skipped == 0 {
		fmt.Println("  [OK]   tools/analysis/ scripts up to date")
	} else {
		fmt.Printf("  [OK]   tools/analysis/: %d written, %d skipped, %d unchanged\n",
			updated, skipped, unchanged)
	}
	return nil
}


// materializeAnalysisHelper creates an execution-only copy of all bundled Python
// helpers outside the untrusted project. Python can then import _paths from the
// same private directory. All files are sourced from the Go binary, never from
// <root>/tools/analysis, and the caller removes the directory after execution.
func materializeAnalysisHelper(projectPath string) (string, func(), error) {
	const prefix = "tools/analysis/"
	if !strings.HasPrefix(projectPath, prefix) {
		return "", nil, fmt.Errorf("not a PyGuard analysis helper: %q", projectPath)
	}
	name := strings.TrimPrefix(projectPath, prefix)
	if name == "" || filepath.Base(name) != name || !strings.HasSuffix(name, ".py") {
		return "", nil, fmt.Errorf("invalid PyGuard helper name: %q", name)
	}
	if _, err := analysisScripts.ReadFile("analysis/" + name); err != nil {
		return "", nil, fmt.Errorf("unknown bundled PyGuard helper %q: %w", name, err)
	}
	entries, err := fs.ReadDir(analysisScripts, "analysis")
	if err != nil {
		return "", nil, fmt.Errorf("read bundled helpers: %w", err)
	}
	dir, err := os.MkdirTemp("", "pyguard-analysis-")
	if err != nil {
		return "", nil, fmt.Errorf("create private helper directory: %w", err)
	}
	cleanup := func() { _ = os.RemoveAll(dir) }
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".py") {
			continue
		}
		data, err := analysisScripts.ReadFile("analysis/" + entry.Name())
		if err != nil {
			cleanup()
			return "", nil, fmt.Errorf("read bundled helper %q: %w", entry.Name(), err)
		}
		if err := os.WriteFile(filepath.Join(dir, entry.Name()), data, 0o600); err != nil {
			cleanup()
			return "", nil, fmt.Errorf("write private helper %q: %w", entry.Name(), err)
		}
	}
	return filepath.Join(dir, name), cleanup, nil
}
