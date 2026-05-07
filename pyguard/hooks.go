package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/oxDevelop/oxguard/pyguard/internal/hooks"
	"golang.org/x/term"
)

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

func runHooks(root string) int {
	selected := promptToolSelection()

	if len(selected) == 0 {
		fmt.Println("  Skipping hook configuration.")
		return 0
	}

	generated := 0

	if selected["1"] {
		if err := hooks.GenerateClaudeHook(root); err != nil {
			fmt.Printf("  [FAIL] Claude Code hook: %v\n", err)
		} else {
			fmt.Println("  [OK]   .claude/settings.local.json (Claude Code PostToolUse)")
			generated++
		}
	}

	if selected["2"] {
		if err := hooks.GenerateCopilotHook(root); err != nil {
			fmt.Printf("  [FAIL] Copilot hook: %v\n", err)
		} else {
			fmt.Println("  [OK]   .github/hooks/pyguard-check.json (Copilot PostToolUse)")
			fmt.Println("  [OK]   .vscode/tasks.json (pyguard check + pyguard fix tasks)")
			generated++
		}
	}

	if selected["3"] {
		if err := hooks.GenerateCursorHook(root); err != nil {
			fmt.Printf("  [FAIL] Cursor hook: %v\n", err)
		} else {
			fmt.Println("  [OK]   .cursor/hooks.json (afterFileEdit)")
			generated++
		}
	}

	if selected["4"] {
		if err := hooks.GenerateCodexHook(root); err != nil {
			fmt.Printf("  [FAIL] Codex hook: %v\n", err)
		} else {
			fmt.Println("  [OK]   .codex/hooks.json (PostToolUse)")
			fmt.Println("  [OK]   AGENTS.md (project instructions)")
			fmt.Println("         Note: Codex hooks are currently disabled on Windows.")
			generated++
		}
	}

	if selected["5"] {
		if err := hooks.GenerateOpenCodePlugin(root); err != nil {
			fmt.Printf("  [FAIL] OpenCode plugin: %v\n", err)
		} else {
			fmt.Println("  [OK]   .opencode/plugins/pyguard/index.ts")
			fmt.Println("  [OK]   opencode.json")
			generated++
		}
	}

	if selected["6"] {
		if err := hooks.GenerateKiroHook(root); err != nil {
			fmt.Printf("  [FAIL] Kiro hook: %v\n", err)
		} else {
			fmt.Println("  [OK]   .kiro/hooks/pyguard.kiro.hook (IDE hook — auto-active)")
			fmt.Println("  [OK]   .kiro/agents/pyguard.json (CLI agent — activate with `/agent swap pyguard`)")
			generated++
		}
	}

	// Always update pre-commit
	if err := hooks.GeneratePreCommit(root); err != nil {
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
