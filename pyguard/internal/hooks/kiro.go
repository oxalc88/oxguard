package hooks

import (
	"fmt"
	"path/filepath"
	"strings"
)

func GenerateKiroHook(root string, agentTemplate []byte) error {
	binJSON := jsonEscapePath(pyguardBinary(root))

	hooksDir := filepath.Join(root, "..", ".kiro", "hooks")
	agentsDir := filepath.Join(root, "..", ".kiro", "agents")

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

	agentContent := strings.ReplaceAll(string(agentTemplate), "__BINARY__", binJSON)
	return writeHookFile(filepath.Join(agentsDir, "pyguard.json"), agentContent)
}
