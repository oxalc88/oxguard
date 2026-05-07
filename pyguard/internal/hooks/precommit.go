package hooks

import (
	"fmt"
	"path/filepath"
	"runtime"
)

func GeneratePreCommit(root string) error {
	repoRoot := filepath.Join(root, "..")
	path := filepath.Join(repoRoot, ".pre-commit-config.yaml")

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
