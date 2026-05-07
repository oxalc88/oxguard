package hooks

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func GenerateCodexHook(root string) error {
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
`, strings.ReplaceAll(pyguardBin, `\`, `\\`))
	if err := writeHookFile(filepath.Join(codexDir, "hooks.json"), hookContent); err != nil {
		return err
	}

	agentsContent := fmt.Sprintf("# Python Quality Gates\n\nAfter editing Python files, run `%s check` to validate quality gates.\nIf checks fail, fix the issues before committing.\n\nQuality gate commands:\n- `%s check`    — full gate (lint, types, complexity, tests, security)\n- `%s fix`      — auto-format\n- `%s ruff`     — lint only\n- `%s mypy`     — type check only\n- `%s coverage` — tests + coverage\n", pyguardBin, pyguardBin, pyguardBin, pyguardBin, pyguardBin, pyguardBin)
	return writeHookFile(filepath.Join(root, "..", "AGENTS.md"), agentsContent)
}
