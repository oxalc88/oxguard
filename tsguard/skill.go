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

// deployClaudeSkill writes the embedded SKILL.md to .claude/skills/tsguard/SKILL.md
// one level above root (workspace/monorepo root, matching the hook convention).
// No-op if the file is already up to date.
func deployClaudeSkill(root string) {
	dir := filepath.Join(root, "..", ".claude", "skills", "tsguard")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		fmt.Printf("  [WARN] Claude Code skill: %v\n", err)
		return
	}
	dest := filepath.Join(dir, "SKILL.md")
	existing, _ := os.ReadFile(dest)
	if bytes.Equal(existing, claudeSkillContent) {
		fmt.Println("  [OK]   .claude/skills/tsguard/SKILL.md (no change)")
		return
	}
	if err := os.WriteFile(dest, claudeSkillContent, 0o644); err != nil {
		fmt.Printf("  [WARN] Claude Code skill: %v\n", err)
		return
	}
	fmt.Println("  [OK]   .claude/skills/tsguard/SKILL.md")
}
