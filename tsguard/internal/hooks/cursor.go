package hooks

import (
	"os"
	"path/filepath"
)

const cursorRule = `---
description: TypeScript quality gates — run tsguard after editing .ts/.tsx files
globs: "**/*.ts,**/*.tsx"
alwaysApply: false
---

# tsguard — TypeScript quality gate

After editing TypeScript files, run ` + "`tsguard check`" + ` to validate quality gates.
Fix any failures before committing.

**Commands:**
- ` + "`tsguard check`" + `    — full gate: lint → fta → types → coverage → security
- ` + "`tsguard fix`" + `      — auto-format (ultracite fix)
- ` + "`tsguard security`" + ` — secrets (secretlint) + CVE (audit-ci) + SAST (opengrep)
- ` + "`tsguard setup`" + `    — install deps + download opengrep SAST binary

**Config:** ` + "`oxguard.toml`" + ` at project root sets default dirs and excludes.
`

// GenerateCursorRule writes a Cursor agent rule to .cursor/rules/tsguard.mdc.
func GenerateCursorRule(root string) error {
	rulesDir := filepath.Join(root, "..", ".cursor", "rules")
	if err := os.MkdirAll(rulesDir, 0o755); err != nil {
		return err
	}
	return writeHookFile(filepath.Join(rulesDir, "tsguard.mdc"), cursorRule)
}

func GenerateCursorHook(root string) error {
	cursorDir := filepath.Join(root, "..", ".cursor")
	if err := os.MkdirAll(cursorDir, 0o755); err != nil {
		return err
	}
	content := `{
  "version": 1,
  "hooks": {
    "afterFileEdit": [
      {
        "type": "command",
        "command": "tsguard check --if-typescript",
        "timeout": 120
      }
    ]
  }
}
`
	return writeHookFile(filepath.Join(cursorDir, "hooks.json"), content)
}
