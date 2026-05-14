package hooks

import (
	"os"
	"path/filepath"
)

func GenerateCodexHook(root string) error {
	codexDir := filepath.Join(root, "..", ".codex")
	if err := os.MkdirAll(codexDir, 0o755); err != nil {
		return err
	}
	hookContent := `{
  "hooks": {
    "PostToolUse": [
      {
        "matcher": "Bash",
        "hooks": [
          {
            "type": "command",
            "command": "tsguard check --if-typescript",
            "timeout": 120,
            "statusMessage": "Running quality gates"
          }
        ]
      }
    ]
  }
}
`
	if err := writeHookFile(filepath.Join(codexDir, "hooks.json"), hookContent); err != nil {
		return err
	}

	agentsContent := `# TypeScript Quality Gates

After editing TypeScript files, run ` + "`tsguard check`" + ` to validate quality gates.
If checks fail, fix the issues before committing.

Quality gate commands:
- ` + "`tsguard check`" + `      — full gate (lint, types, coverage, security)
- ` + "`tsguard fix`" + `        — auto-format
- ` + "`tsguard lint`" + `       — lint only (ultracite check)
- ` + "`tsguard types`" + `      — type check only
- ` + "`tsguard coverage`" + `   — tests + coverage
- ` + "`tsguard complexity`" + ` — compatibility alias for ultracite's complexity checks
`
	return writeHookFile(filepath.Join(root, "..", "AGENTS.md"), agentsContent)
}
