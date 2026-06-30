package hooks

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

// GenerateCopilotInstructions appends pyguard usage instructions to
// .github/copilot-instructions.md using content from the caller's embedded
// skill/copilot.md. Creates the file if absent; idempotent via marker check.
func GenerateCopilotInstructions(root string, content []byte) error {
	ghDir := filepath.Join(root, "..", ".github")
	if err := os.MkdirAll(ghDir, 0o755); err != nil {
		return err
	}
	dest := filepath.Join(ghDir, "copilot-instructions.md")
	existing, _ := os.ReadFile(dest)
	marker := "## Python quality gates (pyguard)"
	if strings.Contains(string(existing), marker) {
		return nil
	}
	f, err := os.OpenFile(dest, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(append([]byte("\n"), content...))
	return err
}
