package hooks

import (
	"os"
	"path/filepath"
)

func GenerateOpenCodePlugin(root string, pluginContent []byte) error {
	pluginDir := filepath.Join(root, "..", ".opencode", "plugins", "tsguard")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		return err
	}

	if err := writeHookFile(filepath.Join(pluginDir, "index.ts"), string(pluginContent)); err != nil {
		return err
	}

	opencodeContent := `{
  "$schema": "https://opencode.ai/config.json",
  "plugin": [".opencode/plugins/tsguard"]
}
`
	return writeHookFile(filepath.Join(root, "..", "opencode.json"), opencodeContent)
}
