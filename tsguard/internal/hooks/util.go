package hooks

import (
	"os"
	"path/filepath"
	"runtime"
)

func tsguardBinary(root string) string {
	name := "tsguard"
	if runtime.GOOS == "windows" {
		name = "tsguard.exe"
	}
	return filepath.Join(root, "tools", "tsguard", name)
}

func writeHookFile(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if existing, err := os.ReadFile(path); err == nil {
		if string(existing) == content {
			return nil
		}
	}
	return os.WriteFile(path, []byte(content), 0o644)
}
