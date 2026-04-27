package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/term"
)

// toolOption represents a selectable AI tool in the setup menu.
type toolOption struct {
	key   string
	label string
}

var toolOptions = []toolOption{
	{"1", "Claude Code"},
	{"2", "GitHub Copilot (VS Code)"},
	{"3", "Cursor"},
	{"4", "OpenAI Codex CLI"},
	{"5", "OpenCode"},
}

// runHooks prompts for AI tool selection and generates hook configs.
func runHooks(root string) int {
	selected := promptToolSelection()

	if len(selected) == 0 {
		fmt.Println("  Skipping hook configuration.")
		return 0
	}

	generated := 0

	if selected["1"] {
		if err := generateClaudeHook(root); err != nil {
			fmt.Printf("  [FAIL] Claude Code hook: %v\n", err)
		} else {
			fmt.Println("  [OK]   .claude/settings.local.json (Claude Code PostToolUse)")
			generated++
		}
	}

	if selected["2"] {
		if err := generateCopilotHook(root); err != nil {
			fmt.Printf("  [FAIL] Copilot hook: %v\n", err)
		} else {
			fmt.Println("  [OK]   .github/hooks/tsguard-check.json (Copilot PostToolUse)")
			fmt.Println("  [OK]   .vscode/tasks.json (tsguard check + tsguard fix tasks)")
			generated++
		}
	}

	if selected["3"] {
		if err := generateCursorHook(root); err != nil {
			fmt.Printf("  [FAIL] Cursor hook: %v\n", err)
		} else {
			fmt.Println("  [OK]   .cursor/hooks.json (afterFileEdit)")
			generated++
		}
	}

	if selected["4"] {
		if err := generateCodexHook(root); err != nil {
			fmt.Printf("  [FAIL] Codex hook: %v\n", err)
		} else {
			fmt.Println("  [OK]   .codex/hooks.json (PostToolUse)")
			fmt.Println("  [OK]   AGENTS.md (project instructions)")
			generated++
		}
	}

	if selected["5"] {
		if err := generateOpenCodePlugin(root); err != nil {
			fmt.Printf("  [FAIL] OpenCode plugin: %v\n", err)
		} else {
			fmt.Println("  [OK]   .opencode/plugins/tsguard/index.ts")
			fmt.Println("  [OK]   opencode.json")
			generated++
		}
	}

	// Always update pre-commit
	if err := generatePreCommit(root); err != nil {
		fmt.Printf("  [FAIL] pre-commit hook: %v\n", err)
	} else {
		fmt.Println("  [OK]   .pre-commit-config.yaml (universal — all tools)")
		generated++
	}

	if generated > 0 {
		fmt.Printf("  Generated %d hook config(s).\n", generated)
	}
	return 0
}

// promptToolSelection shows an interactive checkbox picker when stdin is a TTY,
// or falls back to numbered text input for CI / piped environments.
func promptToolSelection() map[string]bool {
	if term.IsTerminal(int(os.Stdin.Fd())) {
		return promptTUI()
	}
	return promptText()
}

// promptTUI is an arrow-key + space-to-toggle checkbox selector.
func promptTUI() map[string]bool {
	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		return promptText()
	}
	defer term.Restore(int(os.Stdin.Fd()), oldState)

	n := len(toolOptions)
	selected := make([]bool, n)
	cursor := 0

	renderMenu := func() {
		var sb strings.Builder
		sb.WriteString("  \033[1mWhich AI tools do you use?\033[0m\r\n")
		sb.WriteString("  \033[2m↑↓ navigate · space select · enter confirm\033[0m\r\n\r\n")
		for i, opt := range toolOptions {
			check := "\033[2m○\033[0m"
			if selected[i] {
				check = "\033[32m◉\033[0m"
			}
			if i == cursor {
				sb.WriteString(fmt.Sprintf("  \033[36m❯\033[0m %s \033[36m%s\033[0m\r\n", check, opt.label))
			} else {
				sb.WriteString(fmt.Sprintf("    %s %s\r\n", check, opt.label))
			}
		}
		fmt.Print(sb.String())
	}

	eraseMenu := func() {
		var sb strings.Builder
		for i := 0; i < n+3; i++ {
			sb.WriteString("\033[A\033[2K")
		}
		sb.WriteString("\r")
		fmt.Print(sb.String())
	}

	fmt.Print("\033[?25l") // hide cursor during interaction
	renderMenu()

	b := make([]byte, 4)
	for {
		nr, _ := os.Stdin.Read(b)
		if nr == 0 {
			continue
		}

		switch {
		case b[0] == '\r' || b[0] == '\n': // Enter — confirm
			eraseMenu()
			fmt.Print("\033[?25h")
			result := map[string]bool{}
			for i, s := range selected {
				if s {
					result[toolOptions[i].key] = true
				}
			}
			return result

		case b[0] == ' ': // Space — toggle
			selected[cursor] = !selected[cursor]

		case b[0] == 3: // Ctrl+C
			eraseMenu()
			fmt.Print("\033[?25h")
			os.Exit(1)

		case nr >= 3 && b[0] == 27 && b[1] == '[': // Arrow keys
			switch b[2] {
			case 'A': // Up
				if cursor > 0 {
					cursor--
				}
			case 'B': // Down
				if cursor < n-1 {
					cursor++
				}
			}

		default:
			continue // unknown key — skip redraw
		}

		eraseMenu()
		renderMenu()
	}
}

// promptText is the non-TTY fallback: numbered list with comma-separated input.
func promptText() map[string]bool {
	fmt.Println("  Which AI tools do you use?")
	for i, opt := range toolOptions {
		fmt.Printf("  [%d] %s\n", i+1, opt.label)
	}
	fmt.Printf("  [%d] None / skip\n", len(toolOptions)+1)
	fmt.Print("  Select (comma-separated, e.g. 1,3): ")

	scanner := bufio.NewScanner(os.Stdin)
	scanner.Scan()
	input := strings.TrimSpace(scanner.Text())

	skip := fmt.Sprintf("%d", len(toolOptions)+1)
	if input == "" || input == skip {
		return map[string]bool{}
	}

	result := map[string]bool{}
	for _, part := range strings.Split(input, ",") {
		result[strings.TrimSpace(part)] = true
	}
	return result
}

func generateClaudeHook(root string) error {
	// .claude/ is at the repo root (parent of the TypeScript project directory)
	settingsDir := filepath.Join(root, "..", ".claude")
	path := filepath.Join(settingsDir, "settings.local.json")

	tsguardPath := filepath.Join(root, "tools", "tsguard", "tsguard")
	content := fmt.Sprintf(`{
  "permissions": {
    "allow": [
      "Bash(tsguard check:*)",
      "Bash(tsguard fix:*)",
      "Bash(tsguard doctor:*)"
    ]
  },
  "hooks": {
    "PostToolUse": [
      {
        "matcher": "Edit|Write|MultiEdit",
        "hooks": [
          {
            "type": "command",
            "command": "case \"$CLAUDE_TOOL_OUTPUT_PATH\" in *.ts|*.tsx) %s check ;; esac"
          }
        ]
      }
    ]
  }
}
`, tsguardPath)
	return writeHookFile(path, content)
}

func generateCopilotHook(root string) error {
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

func generateCursorHook(root string) error {
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

func generateCodexHook(root string) error {
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
- ` + "`tsguard complexity`" + ` — complexity analysis
`
	return writeHookFile(filepath.Join(root, "..", "AGENTS.md"), agentsContent)
}

func generateOpenCodePlugin(root string) error {
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

func generatePreCommit(root string) error {
	repoRoot := filepath.Join(root, "..")
	path := filepath.Join(repoRoot, ".pre-commit-config.yaml")

	content := `repos:
  - repo: local
    hooks:
      - id: tsguard-check
        name: tsguard check
        entry: tsguard check
        language: system
        types: [ts]
        pass_filenames: false
`
	return writeHookFile(path, content)
}

// writeHookFile writes content to path, creating parent dirs as needed.
// Does not overwrite if file already exists with identical content.
func writeHookFile(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	// Check if already identical
	if existing, err := os.ReadFile(path); err == nil {
		if string(existing) == content {
			return nil // already up to date
		}
	}
	return os.WriteFile(path, []byte(content), 0o644)
}
