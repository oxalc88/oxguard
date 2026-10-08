package hooks

import (
	"os"
	"runtime"
	"strings"
)

// Hooks execute a separately installed command, never a project-owned binary.
func tsguardBinary(root string) string {
	if runtime.GOOS == "windows" { return "tsguard.exe" }
	return "tsguard"
}

func jsonEscapePath(p string) string {
	return strings.ReplaceAll(p, `\`, `\\`)
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
