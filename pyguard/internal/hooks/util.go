package hooks

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Hooks execute a separately installed command, never a project-owned binary.
func pyguardBinary(root string) string {
	if runtime.GOOS == "windows" { return "pyguard.exe" }
	return "pyguard"
}

func jsonEscapePath(p string) string {
	return strings.ReplaceAll(p, `\`, `\\`)
}

func writeHookFile(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if info, err := os.Lstat(path); err == nil {
		if !info.Mode().IsRegular() {
			return fmt.Errorf("refusing non-regular hook configuration: %s", path)
		}
		existing, err := os.ReadFile(path)
		if err != nil { return err }
		if string(existing) == content { return nil }
		return fmt.Errorf("refusing to replace existing hook configuration: %s", path)
	} else if !os.IsNotExist(err) { return err }
	return os.WriteFile(path, []byte(content), 0o600)
}
