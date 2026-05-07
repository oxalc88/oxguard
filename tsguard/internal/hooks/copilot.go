package hooks

import (
	"os"
	"path/filepath"
)

func GenerateCopilotHook(root string) error {
	hooksDir := filepath.Join(root, "..", ".github", "hooks")
	if err := os.MkdirAll(hooksDir, 0o755); err != nil {
		return err
	}
	hookContent := `{
  "hooks": {
    "PostToolUse": [
      {
        "type": "command",
        "command": "tsguard check --if-typescript",
        "timeout": 120
      }
    ]
  }
}
`
	if err := writeHookFile(filepath.Join(hooksDir, "tsguard-check.json"), hookContent); err != nil {
		return err
	}

	vscodeDir := filepath.Join(root, "..", ".vscode")
	if err := os.MkdirAll(vscodeDir, 0o755); err != nil {
		return err
	}
	tasksContent := `{
  "version": "2.0.0",
  "tasks": [
    {
      "label": "tsguard check",
      "type": "shell",
      "command": "tsguard check",
      "group": { "kind": "test", "isDefault": true },
      "presentation": { "reveal": "always", "panel": "shared" },
      "problemMatcher": []
    },
    {
      "label": "tsguard fix",
      "type": "shell",
      "command": "tsguard fix",
      "presentation": { "reveal": "always", "panel": "shared" },
      "problemMatcher": []
    }
  ]
}
`
	return writeHookFile(filepath.Join(vscodeDir, "tasks.json"), tasksContent)
}
