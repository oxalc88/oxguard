package hooks

import (
	"os"
	"path/filepath"
	"strings"
)

const copilotInstructions = `
## TypeScript quality gates (tsguard)

After editing TypeScript files, run ` + "`tsguard check`" + ` to validate all quality gates.
Fix any failures before committing. Key commands:

- ` + "`tsguard check`" + `    — full gate: lint → fta → types → coverage → security (secretlint + audit-ci + opengrep)
- ` + "`tsguard fix`" + `      — auto-format (ultracite fix)
- ` + "`tsguard security`" + ` — secrets + CVE + SAST only
- ` + "`tsguard setup`" + `    — install deps + download opengrep binary

Project config: ` + "`oxguard.toml`" + ` at repo root overrides default dirs and excludes.
`

// GenerateCopilotInstructions appends tsguard usage instructions to
// .github/copilot-instructions.md, creating the file if absent.
func GenerateCopilotInstructions(root string) error {
	ghDir := filepath.Join(root, "..", ".github")
	if err := os.MkdirAll(ghDir, 0o755); err != nil {
		return err
	}
	dest := filepath.Join(ghDir, "copilot-instructions.md")
	existing, _ := os.ReadFile(dest)
	// Avoid duplicating the block if it's already present.
	marker := "## TypeScript quality gates (tsguard)"
	if strings.Contains(string(existing), marker) {
		return nil
	}
	f, err := os.OpenFile(dest, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(copilotInstructions)
	return err
}

func GenerateCopilotHook(root string) error {
	hooksDir := filepath.Join(root, "..", ".github", "hooks")
	if err := os.MkdirAll(hooksDir, 0o755); err != nil {
		return err
	}
	hookContent := `{
  "hooks": {
    "PostToolUse": [
      {
        "type": "command",
        "command": "tsguard check --if-typescript",
        "timeout": 120
      }
    ]
  }
}
`
	if err := writeHookFile(filepath.Join(hooksDir, "tsguard-check.json"), hookContent); err != nil {
		return err
	}

	vscodeDir := filepath.Join(root, "..", ".vscode")
	if err := os.MkdirAll(vscodeDir, 0o755); err != nil {
		return err
	}
	tasksContent := `{
  "version": "2.0.0",
  "tasks": [
    {
      "label": "tsguard check",
      "type": "shell",
      "command": "tsguard check",
      "group": { "kind": "test", "isDefault": true },
      "presentation": { "reveal": "always", "panel": "shared" },
      "problemMatcher": []
    },
    {
      "label": "tsguard fix",
      "type": "shell",
      "command": "tsguard fix",
      "presentation": { "reveal": "always", "panel": "shared" },
      "problemMatcher": []
    }
  ]
}
`
	return writeHookFile(filepath.Join(vscodeDir, "tasks.json"), tasksContent)
}
