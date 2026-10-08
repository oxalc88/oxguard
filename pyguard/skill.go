package main

import (
	"bytes"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
)

//go:embed skill/SKILL.md
var claudeSkillContent []byte

//go:embed skill/cursor.mdc
var cursorRuleContent []byte

//go:embed skill/opencode.ts
var opencodePluginContent []byte

// deployClaudeSkill writes the embedded SKILL.md to .claude/skills/pyguard/SKILL.md
// inside the project root (never the parent workspace).
// No-op if the file is already up to date.
func deployClaudeSkill(root string) {
	dest := filepath.Join(root, ".claude", "skills", "pyguard", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		fmt.Printf("  [WARN] Claude Code skill: %v\n", err)
		return
	}
	existing, _ := os.ReadFile(dest)
	if bytes.Equal(existing, claudeSkillContent) {
		fmt.Println("  [OK]   .claude/skills/pyguard/SKILL.md (no change)")
		return
	}
	if err := os.WriteFile(dest, claudeSkillContent, 0o644); err != nil {
		fmt.Printf("  [WARN] Claude Code skill: %v\n", err)
		return
	}
	fmt.Println("  [OK]   .claude/skills/pyguard/SKILL.md")
}
