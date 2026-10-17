package main

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestInstalledSkillBundle(t *testing.T) {
	for _, relative := range []string{".claude/skills/tsguard", ".agents/skills/tsguard"} {
		t.Run(relative, func(t *testing.T) {
			root := t.TempDir()
			if err := deploySkillBundle(root, relative); err != nil {
				t.Fatal(err)
			}
			if err := deploySkillBundle(root, relative); err != nil {
				t.Fatal(err)
			}
			links := regexp.MustCompile(`\]\(([^)]+\.md)\)`)
			err := fs.WalkDir(skillFiles, "skill", func(source string, entry fs.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if entry.IsDir() {
					return nil
				}
				expected, err := skillFiles.ReadFile(source)
				if err != nil {
					return err
				}
				dest := filepath.Join(root, relative, source[len("skill/"):])
				actual, err := os.ReadFile(dest)
				if err != nil {
					return err
				}
				if !bytes.Equal(expected, actual) {
					t.Errorf("installed %s differs", source)
				}
				for _, link := range links.FindAllSubmatch(actual, -1) {
					if strings.HasPrefix(string(link[1]), "https://") {
						continue
					}
					if _, err := os.ReadFile(filepath.Join(filepath.Dir(dest), string(link[1]))); err != nil {
						t.Errorf("broken installed link from %s: %v", source, err)
					}
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			owned := filepath.Join(root, relative, "workflows/check.md")
			if err := os.WriteFile(owned, []byte("old version"), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := deploySkillBundle(root, relative); err != nil {
				t.Fatal(err)
			}
			actual, _ := os.ReadFile(owned)
			expected, _ := skillFiles.ReadFile("skill/workflows/check.md")
			if !bytes.Equal(actual, expected) {
				t.Fatal("workflow was not refreshed")
			}
		})
	}
}

func TestSkillDeploymentRejectsSymlink(t *testing.T) {
	root := t.TempDir()
	external := filepath.Join(t.TempDir(), "external.md")
	if err := os.WriteFile(external, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(root, ".agents/skills/tsguard")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(external, filepath.Join(dest, "SKILL.md")); err != nil {
		t.Skip(err)
	}
	if err := deployAgentSkill(root); err == nil {
		t.Fatal("accepted symlink")
	}
	content, _ := os.ReadFile(external)
	if string(content) != "keep" {
		t.Fatal("changed external file")
	}
}
