package hooks

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func GenerateKiroHook(root string) error {
	pyguardBin := pyguardBinary(root)
	binJSON := strings.ReplaceAll(pyguardBin, `\`, `\\`)

	hooksDir := filepath.Join(root, "..", ".kiro", "hooks")
	agentsDir := filepath.Join(root, "..", ".kiro", "agents")
	if err := os.MkdirAll(hooksDir, 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(agentsDir, 0o755); err != nil {
		return err
	}

	hookContent := fmt.Sprintf(`{
  "name": "pyguard",
  "description": "Run pyguard quality gate after editing Python files",
  "version": "1",
  "when": {
    "type": "fileEdited",
    "patterns": ["**/*.py"]
  },
  "then": {
    "type": "command",
    "command": "%s check --if-python"
  }
}
`, binJSON)
	if err := writeHookFile(filepath.Join(hooksDir, "pyguard.kiro.hook"), hookContent); err != nil {
		return err
	}

	agentContent := fmt.Sprintf(`{
  "name": "pyguard",
  "description": "Python quality gate runner. Quality checks run automatically after every file write.",
  "prompt": "You are working in a Python project guarded by pyguard. After every write, the postToolUse hook runs '%s check --if-python' automatically. If the gate fails, fix the issue before continuing. Manual commands: pyguard check, pyguard fix, pyguard ruff, pyguard mypy, pyguard coverage.",
  "tools": ["read", "write", "shell"],
  "hooks": {
    "postToolUse": [
      {
        "matcher": "fs_write",
        "command": "%s check --if-python"
      }
    ]
  }
}
`, binJSON, binJSON)
	return writeHookFile(filepath.Join(agentsDir, "pyguard.json"), agentContent)
}
