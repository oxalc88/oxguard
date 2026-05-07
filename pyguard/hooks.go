package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"golang.org/x/term"
)

// pyguardBinary returns the absolute path to the pyguard binary for the given project root.
func pyguardBinary(root string) string {
	name := "pyguard"
	if runtime.GOOS == "windows" {
		name = "pyguard.exe"
	}
	return filepath.Join(root, "tools", "pyguard", name)
}

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
	{"6", "Kiro"},
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
			fmt.Println("  [OK]   .github/hooks/pyguard-check.json (Copilot PostToolUse)")
			fmt.Println("  [OK]   .vscode/tasks.json (pyguard check + pyguard fix tasks)")
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
			fmt.Println("         Note: Codex hooks are currently disabled on Windows.")
			generated++
		}
	}

	if selected["5"] {
		if err := generateOpenCodePlugin(root); err != nil {
			fmt.Printf("  [FAIL] OpenCode plugin: %v\n", err)
		} else {
			fmt.Println("  [OK]   .opencode/plugins/pyguard/index.ts")
			fmt.Println("  [OK]   opencode.json")
			generated++
		}
	}

	if selected["6"] {
		if err := generateKiroHook(root); err != nil {
			fmt.Printf("  [FAIL] Kiro hook: %v\n", err)
		} else {
			fmt.Println("  [OK]   .kiro/hooks/pyguard.kiro.hook (IDE hook — auto-active)")
			fmt.Println("  [OK]   .kiro/agents/pyguard.json (CLI agent — activate with `/agent swap pyguard`)")
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
	pyguardBin := pyguardBinary(root)
	settingsDir := filepath.Join(root, "..", ".claude")
	path := filepath.Join(settingsDir, "settings.local.json")

	var hookCmd string
	if runtime.GOOS == "windows" {
		escaped := strings.ReplaceAll(pyguardBin, `\`, `\\`)
		hookCmd = fmt.Sprintf(`if ($env:CLAUDE_TOOL_OUTPUT_PATH -match '\\.py$') { & '%s' check }`, escaped)
	} else {
		hookCmd = fmt.Sprintf(`case "$CLAUDE_TOOL_OUTPUT_PATH" in *.py) '%s' check ;; esac`, pyguardBin)
	}

	// Escape for JSON string embedding
	pyguardBinJSON := strings.ReplaceAll(pyguardBin, `\`, `\\`)
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

func generateCopilotHook(root string) error {
	pyguardBin := pyguardBinary(root)
	// .github/hooks/pyguard-check.json
	hooksDir := filepath.Join(root, "..", ".github", "hooks")
	if err := os.MkdirAll(hooksDir, 0o755); err != nil {
		return err
	}
	hookContent := fmt.Sprintf(`{
  "hooks": {
    "PostToolUse": [
      {
        "type": "command",
        "command": "%s check --if-python",
        "timeout": 120
      }
    ]
  }
}
`, strings.ReplaceAll(pyguardBin, `\`, `\\`))
	if err := writeHookFile(filepath.Join(hooksDir, "pyguard-check.json"), hookContent); err != nil {
		return err
	}

	// .vscode/tasks.json
	vscodeDir := filepath.Join(root, "..", ".vscode")
	if err := os.MkdirAll(vscodeDir, 0o755); err != nil {
		return err
	}
	pyguardBinJSON := strings.ReplaceAll(pyguardBin, `\`, `\\`)
	tasksContent := fmt.Sprintf(`{
  "version": "2.0.0",
  "tasks": [
    {
      "label": "pyguard check",
      "type": "shell",
      "command": "%s check",
      "group": { "kind": "test", "isDefault": true },
      "presentation": { "reveal": "always", "panel": "shared" },
      "problemMatcher": []
    },
    {
      "label": "pyguard fix",
      "type": "shell",
      "command": "%s fix",
      "presentation": { "reveal": "always", "panel": "shared" },
      "problemMatcher": []
    }
  ]
}
`, pyguardBinJSON, pyguardBinJSON)
	return writeHookFile(filepath.Join(vscodeDir, "tasks.json"), tasksContent)
}

func generateCursorHook(root string) error {
	pyguardBin := pyguardBinary(root)
	cursorDir := filepath.Join(root, "..", ".cursor")
	if err := os.MkdirAll(cursorDir, 0o755); err != nil {
		return err
	}
	content := fmt.Sprintf(`{
  "version": 1,
  "hooks": {
    "afterFileEdit": [
      {
        "type": "command",
        "command": "%s check --if-python",
        "timeout": 120
      }
    ]
  }
}
`, strings.ReplaceAll(pyguardBin, `\`, `\\`))
	return writeHookFile(filepath.Join(cursorDir, "hooks.json"), content)
}

func generateCodexHook(root string) error {
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

func generateOpenCodePlugin(root string) error {
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

func generateKiroHook(root string) error {
	pyguardBin := pyguardBinary(root)
	binJSON := strings.ReplaceAll(pyguardBin, `\`, `\\`)

	hooksDir := filepath.Join(root, "..", ".kiro", "hooks")
	agentsDir := filepath.Join(root, "..", ".kiro", "agents")
	if err := os.MkdirAll(hooksDir, 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(agentsDir, 0o755); err != nil {
		return err
	}

	hookContent := fmt.Sprintf(`{
  "name": "pyguard",
  "description": "Run pyguard quality gate after editing Python files",
  "version": "1",
  "when": {
    "type": "fileEdited",
    "patterns": ["**/*.py"]
  },
  "then": {
    "type": "command",
    "command": "%s check --if-python"
  }
}
`, binJSON)
	if err := writeHookFile(filepath.Join(hooksDir, "pyguard.kiro.hook"), hookContent); err != nil {
		return err
	}

	agentContent := fmt.Sprintf(`{
  "name": "pyguard",
  "description": "Python quality gate runner. Quality checks run automatically after every file write.",
  "prompt": "You are working in a Python project guarded by pyguard. After every write, the postToolUse hook runs '%s check --if-python' automatically. If the gate fails, fix the issue before continuing. Manual commands: pyguard check, pyguard fix, pyguard ruff, pyguard mypy, pyguard coverage.",
  "tools": ["read", "write", "shell"],
  "hooks": {
    "postToolUse": [
      {
        "matcher": "fs_write",
        "command": "%s check --if-python"
      }
    ]
  }
}
`, binJSON, binJSON)
	return writeHookFile(filepath.Join(agentsDir, "pyguard.json"), agentContent)
}

func generatePreCommit(root string) error {
	repoRoot := filepath.Join(root, "..")
	path := filepath.Join(repoRoot, ".pre-commit-config.yaml")

	// Relative path keeps the committed config portable (pre-commit runs from repo root).
	binName := "pyguard"
	if runtime.GOOS == "windows" {
		binName = "pyguard.exe"
	}
	entry := filepath.ToSlash(filepath.Join("tools", "pyguard", binName))

	content := fmt.Sprintf(`repos:
  - repo: local
    hooks:
      - id: pyguard-check
        name: pyguard check
        entry: %s check
        language: system
        types: [python]
        pass_filenames: false
`, entry)
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
