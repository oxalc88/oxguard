package hooks

import (
	"fmt"
	"os"
	"path/filepath"
)

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
