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
	for _, relative := range []string{".claude/skills/pyguard", ".agents/skills/pyguard"} {
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
				if !bytes.Equal(versionedSkillContent(expected), actual) {
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
			if !bytes.Equal(actual, versionedSkillContent(expected)) {
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
	dest := filepath.Join(root, ".agents/skills/pyguard")
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

func TestSkillVersionUpgrade(t *testing.T) {
	original := version
	defer func() { version = original }()
	root := t.TempDir()
	for _, tag := range []string{"v0.9.0", "v0.9.1"} {
		version = tag
		if err := deployAgentSkill(root); err != nil {
			t.Fatal(err)
		}
		directory := filepath.Join(root, ".agents/skills/pyguard")
		marker, err := os.ReadFile(filepath.Join(directory, "VERSION"))
		if err != nil || string(marker) != tag+"\n" {
			t.Fatalf("version marker: %q, %v", marker, err)
		}
		total := 0
		err = filepath.WalkDir(directory, func(file string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() {
				return nil
			}
			content, err := os.ReadFile(file)
			if err != nil {
				return err
			}
			total += len(strings.Fields(string(content)))
			if strings.HasSuffix(file, ".md") && !bytes.Contains(content, []byte("OxGuard version: "+tag)) {
				t.Errorf("unversioned skill file %s", file)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		if total > 700 {
			t.Fatalf("installed bundle has %d words", total)
		}
		entry, _ := os.ReadFile(filepath.Join(directory, "SKILL.md"))
		if len(strings.Fields(string(entry))) > 150 {
			t.Fatal("installed router exceeds 150 words")
		}
		if !bytes.HasPrefix(entry, []byte("---\nname:")) {
			t.Fatal("frontmatter moved")
		}
		if !bytes.Contains(entry, []byte("/blob/"+tag+"/docs/")) || bytes.Contains(entry, []byte("/blob/main/")) {
			t.Fatal("documentation link does not match installed version")
		}
	}
}

func TestVersionLabelPreservesWindowsFrontmatter(t *testing.T) {
	rendered := versionedSkillContent([]byte("---\r\nname: example\r\ndescription: Example\r\n---\r\n\r\n# Example\r\n"))
	if !bytes.HasPrefix(rendered, []byte("---\nname:")) || !bytes.Contains(rendered, []byte("\n---\n\nOxGuard version: ")) {
		t.Fatal("version label broke frontmatter")
	}
}
