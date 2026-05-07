package hooks

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func GenerateKiroHook(root string) error {
	tsguardBin := tsguardBinary(root)
	binJSON := strings.ReplaceAll(tsguardBin, `\`, `\\`)

	hooksDir := filepath.Join(root, "..", ".kiro", "hooks")
	agentsDir := filepath.Join(root, "..", ".kiro", "agents")
	if err := os.MkdirAll(hooksDir, 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(agentsDir, 0o755); err != nil {
		return err
	}

	hookContent := fmt.Sprintf(`{
  "name": "tsguard",
  "description": "Run tsguard quality gate after editing TypeScript files",
  "version": "1",
  "when": {
    "type": "fileEdited",
    "patterns": ["**/*.ts", "**/*.tsx"]
  },
  "then": {
    "type": "command",
    "command": "%s check --if-typescript"
  }
}
`, binJSON)
	if err := writeHookFile(filepath.Join(hooksDir, "tsguard.kiro.hook"), hookContent); err != nil {
		return err
	}

	agentContent := fmt.Sprintf(`{
  "name": "tsguard",
  "description": "TypeScript quality gate runner. Quality checks run automatically after every file write.",
  "prompt": "You are working in a TypeScript project guarded by tsguard. After every write, the postToolUse hook runs '%s check --if-typescript' automatically. If the gate fails, fix the issue before continuing. Manual commands: tsguard check, tsguard fix, tsguard lint, tsguard types, tsguard coverage.",
  "tools": ["read", "write", "shell"],
  "hooks": {
    "postToolUse": [
      {
        "matcher": "fs_write",
        "command": "%s check --if-typescript"
      }
    ]
  }
}
`, binJSON, binJSON)
	return writeHookFile(filepath.Join(agentsDir, "tsguard.json"), agentContent)
}
