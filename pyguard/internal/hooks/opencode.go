package hooks

import (
	"os"
	"path/filepath"
	"strings"
)

func GenerateOpenCodePlugin(root string, pluginTemplate []byte) error {
	pyguardBin := pyguardBinary(root)
	pluginDir := filepath.Join(root, "..", ".opencode", "plugins", "pyguard")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		return err
	}

	tsContent := strings.ReplaceAll(string(pluginTemplate), "__BINARY__", pyguardBin)
	if err := writeHookFile(filepath.Join(pluginDir, "index.ts"), tsContent); err != nil {
		return err
	}

	opencodeContent := `{
  "$schema": "https://opencode.ai/config.json",
  "plugin": [".opencode/plugins/pyguard"]
}
`
	return writeHookFile(filepath.Join(root, "..", "opencode.json"), opencodeContent)
}
