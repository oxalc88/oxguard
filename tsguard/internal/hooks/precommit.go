package hooks

import "path/filepath"

func GeneratePreCommit(root string) error {
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
