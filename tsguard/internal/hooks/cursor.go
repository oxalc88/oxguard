package hooks

import (
	"os"
	"path/filepath"
)

// GenerateCursorRule writes a Cursor agent rule to .cursor/rules/tsguard.mdc
// using content from the caller's embedded skill/cursor.mdc.
func GenerateCursorRule(root string, content []byte) error {
	rulesDir := filepath.Join(root, "..", ".cursor", "rules")
	return writeHookFile(filepath.Join(rulesDir, "tsguard.mdc"), string(content))
}

func GenerateCursorHook(root string) error {
	cursorDir := filepath.Join(root, "..", ".cursor")
	if err := os.MkdirAll(cursorDir, 0o755); err != nil {
		return err
	}
	content := `{
  "version": 1,
  "hooks": {
    "afterFileEdit": [
      {
        "type": "command",
        "command": "tsguard check --if-typescript",
        "timeout": 120
      }
    ]
  }
}
`
	return writeHookFile(filepath.Join(cursorDir, "hooks.json"), content)
}
