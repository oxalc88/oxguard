package main

import (
	"bytes"
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

//go:embed skill/SKILL.md skill/workflows/*.md
var skillFiles embed.FS

//go:embed skill/opencode.ts
var opencodePluginContent []byte

func versionedSkillContent(content []byte) []byte {
	text := strings.ReplaceAll(strings.ReplaceAll(string(content), "\r\n", "\n"), "https://github.com/oxalc88/oxguard/blob/main/", "https://github.com/oxalc88/oxguard/blob/"+documentationRef()+"/")
	label := "\nOxGuard version: " + version + "\n"
	if strings.HasPrefix(text, "---\n") {
		if end := strings.Index(text[4:], "\n---\n"); end >= 0 {
			offset := 4 + end + len("\n---\n")
			return []byte(text[:offset] + label + text[offset:])
		}
	}
	return []byte(label + "\n" + text)
}

func documentationRef() string {
	if version == "dev" {
		return "main"
	}
	return version
}

func writeSkillFile(dest string, content []byte) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	if info, err := os.Lstat(dest); err == nil && !info.Mode().IsRegular() {
		return fmt.Errorf("refusing non-regular skill file: %s", dest)
	} else if err != nil && !os.IsNotExist(err) {
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
}

func deploySkillBundle(root, relative string) error {
	err := fs.WalkDir(skillFiles, "skill", func(source string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		content, err := skillFiles.ReadFile(source)
		if err != nil {
			return err
		}
		dest := filepath.Join(root, relative, source[len("skill/"):])
		return writeSkillFile(dest, versionedSkillContent(content))
	})
	if err != nil {
		return err
	}
	return writeSkillFile(filepath.Join(root, relative, "VERSION"), []byte(version+"\n"))
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
