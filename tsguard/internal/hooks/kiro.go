package hooks

import (
	"fmt"
	"path/filepath"
	"strings"
)

func GenerateKiroHook(root string, agentTemplate []byte) error {
	binJSON := jsonEscapePath(tsguardBinary(root))

	hooksDir := filepath.Join(root, "..", ".kiro", "hooks")
	agentsDir := filepath.Join(root, "..", ".kiro", "agents")

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

	agentContent := strings.ReplaceAll(string(agentTemplate), "__BINARY__", binJSON)
	return writeHookFile(filepath.Join(agentsDir, "tsguard.json"), agentContent)
}
