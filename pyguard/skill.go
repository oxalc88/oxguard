package main

import (
	"bytes"
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

//go:embed skill/SKILL.md skill/workflows/*.md
var skillFiles embed.FS

//go:embed skill/opencode.ts
var opencodePluginContent []byte

func deploySkillBundle(root, relative string) error {
	return fs.WalkDir(skillFiles, "skill", func(source string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		dest := filepath.Join(root, relative, source[len("skill/"):])
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return err
		}
		if info, err := os.Lstat(dest); err == nil && !info.Mode().IsRegular() {
			return fmt.Errorf("refusing non-regular skill file: %s", dest)
		} else if err != nil && !os.IsNotExist(err) {
			return err
		}
		content, err := skillFiles.ReadFile(source)
		if err != nil {
			return err
		}
		existing, err := os.ReadFile(dest)
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		if bytes.Equal(existing, content) {
			return nil
		}
		return os.WriteFile(dest, content, 0o644)
	})
}

func deployClaudeSkill(root string) {
	if err := deploySkillBundle(root, ".claude/skills/pyguard"); err != nil {
		fmt.Printf("  [WARN] Claude Code skill: %v\n", err)
		return
	}
	fmt.Println("  [OK]   .claude/skills/pyguard (entry and workflows)")
}

func deployAgentSkill(root string) error {
	return deploySkillBundle(root, ".agents/skills/pyguard")
}

var agentSkillInstructions = []byte("## Python quality gates (pyguard)\n\nRead `.agents/skills/pyguard/SKILL.md` before running or interpreting pyguard. Select the linked workflow for the task.\n")
