package hooks

import (
	"os"
	"path/filepath"
	"strings"
)

// GenerateCopilotInstructions appends tsguard usage instructions to
// .github/copilot-instructions.md using content from the caller's embedded
// skill/copilot.md. Creates the file if absent; idempotent via marker check.
func GenerateCopilotInstructions(root string, content []byte) error {
	ghDir := filepath.Join(root, "..", ".github")
	if err := os.MkdirAll(ghDir, 0o755); err != nil {
		return err
	}
	dest := filepath.Join(ghDir, "copilot-instructions.md")
	existing, _ := os.ReadFile(dest)
	marker := "## TypeScript quality gates (tsguard)"
	if strings.Contains(string(existing), marker) {
		return nil
	}
	f, err := os.OpenFile(dest, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	f.WriteString("\n")
	_, err = f.Write(content)
	return err
}

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
