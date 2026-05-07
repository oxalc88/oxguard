package hooks

import (
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
)

func GenerateClaudeHook(root string) error {
	pyguardBin := pyguardBinary(root)
	settingsDir := filepath.Join(root, "..", ".claude")
	path := filepath.Join(settingsDir, "settings.local.json")

	var hookCmd string
	if runtime.GOOS == "windows" {
		hookCmd = fmt.Sprintf(`if ($env:CLAUDE_TOOL_OUTPUT_PATH -match '\\.py$') { & '%s' check }`, jsonEscapePath(pyguardBin))
	} else {
		hookCmd = fmt.Sprintf(`case "$CLAUDE_TOOL_OUTPUT_PATH" in *.py) '%s' check ;; esac`, pyguardBin)
	}

	pyguardBinJSON := jsonEscapePath(pyguardBin)
	hookCmdJSON := strings.ReplaceAll(hookCmd, `"`, `\"`)

	content := fmt.Sprintf(`{
  "permissions": {
    "allow": [
      "Bash(%s check:*)",
      "Bash(%s fix:*)"
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
`, pyguardBinJSON, pyguardBinJSON, hookCmdJSON)
	return writeHookFile(path, content)
}
