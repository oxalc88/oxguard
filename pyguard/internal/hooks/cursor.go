package hooks

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func GenerateCursorHook(root string) error {
	pyguardBin := pyguardBinary(root)
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
`, strings.ReplaceAll(pyguardBin, `\`, `\\`))
	return writeHookFile(filepath.Join(cursorDir, "hooks.json"), content)
}
