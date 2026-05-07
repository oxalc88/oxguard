package hooks

import (
	"fmt"
	"os"
	"path/filepath"
)

func GenerateCopilotHook(root string) error {
	pyguardBinJSON := jsonEscapePath(pyguardBinary(root))
	hooksDir := filepath.Join(root, "..", ".github", "hooks")
	if err := os.MkdirAll(hooksDir, 0o755); err != nil {
		return err
	}
	hookContent := fmt.Sprintf(`{
  "hooks": {
    "PostToolUse": [
      {
        "type": "command",
        "command": "%s check --if-python",
        "timeout": 120
      }
    ]
  }
}
`, pyguardBinJSON)
	if err := writeHookFile(filepath.Join(hooksDir, "pyguard-check.json"), hookContent); err != nil {
		return err
	}

	vscodeDir := filepath.Join(root, "..", ".vscode")
	if err := os.MkdirAll(vscodeDir, 0o755); err != nil {
		return err
	}
	tasksContent := fmt.Sprintf(`{
  "version": "2.0.0",
  "tasks": [
    {
      "label": "pyguard check",
      "type": "shell",
      "command": "%s check",
      "group": { "kind": "test", "isDefault": true },
      "presentation": { "reveal": "always", "panel": "shared" },
      "problemMatcher": []
    },
    {
      "label": "pyguard fix",
      "type": "shell",
      "command": "%s fix",
      "presentation": { "reveal": "always", "panel": "shared" },
      "problemMatcher": []
    }
  ]
}
`, pyguardBinJSON, pyguardBinJSON)
	return writeHookFile(filepath.Join(vscodeDir, "tasks.json"), tasksContent)
}
