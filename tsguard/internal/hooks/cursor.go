package hooks

import (
	"os"
	"path/filepath"
)

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
