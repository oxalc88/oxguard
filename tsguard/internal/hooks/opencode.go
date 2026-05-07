package hooks

import (
	"os"
	"path/filepath"
)

func GenerateOpenCodePlugin(root string) error {
	pluginDir := filepath.Join(root, "..", ".opencode", "plugins", "tsguard")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		return err
	}

	tsContent := `import type { Plugin } from "@opencode-ai/plugin"

export const KpsPlugin: Plugin = async ({ $ }) => {
  return {
    "file.edited": async (input: { filePath: string }) => {
      if (input.filePath.endsWith(".ts") || input.filePath.endsWith(".tsx")) {
        await $` + "`tsguard check`" + `
      }
    },
  }
}
`
	if err := writeHookFile(filepath.Join(pluginDir, "index.ts"), tsContent); err != nil {
		return err
	}

	opencodeContent := `{
  "$schema": "https://opencode.ai/config.json",
  "plugin": [".opencode/plugins/tsguard"]
}
`
	return writeHookFile(filepath.Join(root, "..", "opencode.json"), opencodeContent)
}
