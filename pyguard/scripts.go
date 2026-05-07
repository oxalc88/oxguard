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

// ensureAnalysisScripts deploys pyguard/analysis/*.py into <root>/tools/analysis/.
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

