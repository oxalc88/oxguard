package hooks

import (
	"fmt"
	"os"
	"path/filepath"
)

func GenerateOpenCodePlugin(root string) error {
	pyguardBin := pyguardBinary(root)
	pluginDir := filepath.Join(root, "..", ".opencode", "plugins", "pyguard")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		return err
	}

	tsContent := fmt.Sprintf("import type { Plugin } from \"@opencode-ai/plugin\"\n\nexport const PknPlugin: Plugin = async ({ $ }) => {\n  return {\n    \"file.edited\": async (input: { filePath: string }) => {\n      if (input.filePath.endsWith(\".py\")) {\n        await $`%s check`\n      }\n    },\n  }\n}\n", pyguardBin)
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
