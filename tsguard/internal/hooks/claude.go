package hooks

import (
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
)

func GenerateClaudeHook(root string) error {
	settingsDir := filepath.Join(root, "..", ".claude")
	path := filepath.Join(settingsDir, "settings.local.json")

	tsguardBin := tsguardBinary(root)

	var hookCmd string
	if runtime.GOOS == "windows" {
		escaped := strings.ReplaceAll(tsguardBin, `\`, `\\`)
		hookCmd = fmt.Sprintf(`if ($env:CLAUDE_TOOL_OUTPUT_PATH -match '\.(ts|tsx)$') { & '%s' check }`, escaped)
	} else {
		hookCmd = fmt.Sprintf(`case "$CLAUDE_TOOL_OUTPUT_PATH" in *.ts|*.tsx) '%s' check ;; esac`, tsguardBin)
	}

	tsguardBinJSON := strings.ReplaceAll(tsguardBin, `\`, `\\`)
	hookCmdJSON := strings.ReplaceAll(hookCmd, `"`, `\"`)

	content := fmt.Sprintf(`{
  "permissions": {
    "allow": [
      "Bash(%s check:*)",
      "Bash(%s fix:*)",
      "Bash(%s doctor:*)"
    ]
  },
  "hooks": {
    "PostToolUse": [
      {
        "matcher": "Edit|Write|MultiEdit",
        "hooks": [
          {
            "type": "command",
            "command": "%s"
          }
        ]
      }
    ]
  }
}
`, tsguardBinJSON, tsguardBinJSON, tsguardBinJSON, hookCmdJSON)
	return writeHookFile(path, content)
}
