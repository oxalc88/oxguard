package hooks

import (
	"os"
	"path/filepath"
)

func GenerateCodexHook(root string, agentsContent []byte) error {
	codexDir := filepath.Join(root, "..", ".codex")
	if err := os.MkdirAll(codexDir, 0o755); err != nil {
		return err
	}
	hookContent := `{
  "hooks": {
    "PostToolUse": [
      {
        "matcher": "Bash",
        "hooks": [
          {
            "type": "command",
            "command": "tsguard check --if-typescript",
            "timeout": 120,
            "statusMessage": "Running quality gates"
          }
        ]
      }
    ]
  }
}
`
	if err := writeHookFile(filepath.Join(codexDir, "hooks.json"), hookContent); err != nil {
		return err
	}

	return writeHookFile(filepath.Join(root, "..", "AGENTS.md"), string(agentsContent))
}
