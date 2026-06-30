package hooks

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func GenerateCodexHook(root string, agentsTemplate []byte) error {
	pyguardBin := pyguardBinary(root)
	codexDir := filepath.Join(root, "..", ".codex")
	if err := os.MkdirAll(codexDir, 0o755); err != nil {
		return err
	}
	hookContent := fmt.Sprintf(`{
  "hooks": {
    "PostToolUse": [
      {
        "matcher": "Bash",
        "hooks": [
          {
            "type": "command",
            "command": "%s check --if-python",
            "timeout": 120,
            "statusMessage": "Running quality gates"
          }
        ]
      }
    ]
  }
}
`, jsonEscapePath(pyguardBin))
	if err := writeHookFile(filepath.Join(codexDir, "hooks.json"), hookContent); err != nil {
		return err
	}

	agentsContent := strings.ReplaceAll(string(agentsTemplate), "__BINARY__", pyguardBin)
	return writeHookFile(filepath.Join(root, "..", "AGENTS.md"), agentsContent)
}
