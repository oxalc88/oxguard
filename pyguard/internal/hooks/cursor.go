package hooks

import (
	"fmt"
	"os"
	"path/filepath"
)

// GenerateCursorRule writes a Cursor agent rule to .cursor/rules/pyguard.mdc
// using content from the caller's embedded skill/cursor.mdc.
func GenerateCursorRule(root string, content []byte) error {
	rulesDir := filepath.Join(root, "..", ".cursor", "rules")
	return writeHookFile(filepath.Join(rulesDir, "pyguard.mdc"), string(content))
}

func GenerateCursorHook(root string) error {
	cursorDir := filepath.Join(root, "..", ".cursor")
	if err := os.MkdirAll(cursorDir, 0o755); err != nil {
		return err
	}
	content := fmt.Sprintf(`{
  "version": 1,
  "hooks": {
    "afterFileEdit": [
      {
        "type": "command",
        "command": "%s check --if-python",
        "timeout": 120
      }
    ]
  }
}
`, jsonEscapePath(pyguardBinary(root)))
	return writeHookFile(filepath.Join(cursorDir, "hooks.json"), content)
}
