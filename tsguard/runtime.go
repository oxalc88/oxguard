package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// The npm launcher supplies package-owned tools. Standalone installs retain the
// existing project/package-manager resolution when no runtime is supplied.
func packagedRuntime() string { return os.Getenv("TSGUARD_RUNTIME") }

func packagedCommand(tool string, args ...string) []string {
	node := os.Getenv("TSGUARD_NODE")
	if node == "" {
		node = "node"
	}
	return append([]string{node, filepath.Join(packagedRuntime(), "bin", "tool.cjs"), tool}, args...)
}

func hasProjectConfig(root string, names ...string) bool {
	for _, name := range names {
		if _, err := os.Stat(filepath.Join(root, name)); err == nil {
			return true
		}
	}
	return false
}

func runtimeModule(root, name string) (string, error) {
	out, _, err := RunSilent(root, packagedCommand("--resolve", name)...)
	return strings.TrimSpace(out), err
}

func writeRuntimeConfig(root, name string, value any) (string, error) {
	directory := filepath.Join(root, opengrepCacheDir, "defaults")
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return "", err
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return "", err
	}
	file := filepath.Join(directory, name)
	return file, os.WriteFile(file, data, 0o644)
}

func (r *Runner) runPackagedBiome(fix bool) int {
	if !hasProjectConfig(r.root, "biome.json", "biome.jsonc", ".biome.json", ".biome.jsonc") && hasProjectConfig(r.root, "eslint.config.js", "eslint.config.mjs", "eslint.config.cjs", "eslint.config.ts", ".oxlintrc.json", "oxlint.config.ts") {
		command := "check"
		if fix {
			command = "fix"
		}
		if !r.Run("ultracite "+command, packagedCommand("ultracite", append([]string{command}, r.dirs...)...)...).ok {
			return 1
		}
		return 0
	}
	args := []string{"check"}
	if fix {
		args = append(args, "--write")
	}
	if !hasProjectConfig(r.root, "biome.json", "biome.jsonc", ".biome.json", ".biome.jsonc") {
		preset, err := runtimeModule(r.root, "ultracite/biome/core")
		if err != nil {
			fmt.Fprintf(os.Stderr, "tsguard: resolve bundled lint preset: %v\n", err)
			return 1
		}
		file, err := writeRuntimeConfig(r.root, "biome.json", map[string]any{
			"root": true, "extends": []string{preset},
			"vcs": map[string]any{"enabled": hasProjectConfig(r.root, ".gitignore"), "useIgnoreFile": hasProjectConfig(r.root, ".gitignore")},
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "tsguard: default lint config: %v\n", err)
			return 1
		}
		args = append(args, "--config-path", filepath.Dir(file))
	}
	args = append(args, r.dirs...)
	if !r.Run("Biome/Ultracite", packagedCommand("biome", args...)...).ok {
		return 1
	}
	return 0
}

func typescriptGlob(parts ...string) string {
	return filepath.Join(parts...)
}

func (r *Runner) runPackagedTypes() int {
	args := []string{"--noEmit"}
	if !hasProjectConfig(r.root, "tsconfig.json") {
		include := []string{}
		for _, dir := range r.dirs {
			include = append(include, typescriptGlob(r.root, dir, "**", "*.ts"), typescriptGlob(r.root, dir, "**", "*.tsx"))
		}
		exclude := []string{filepath.ToSlash(filepath.Join(r.root, "**", "*.test.*")), filepath.ToSlash(filepath.Join(r.root, "**", "*.spec.*"))}
		for _, dir := range r.excludeDirs {
			exclude = append(exclude, typescriptGlob(r.root, "**", dir, "**"))
		}
		file, err := writeRuntimeConfig(r.root, "tsconfig.json", map[string]any{
			"compilerOptions": map[string]any{"strict": true, "noEmit": true, "target": "ES2022", "module": "ESNext", "moduleResolution": "Bundler", "skipLibCheck": true, "jsx": "react-jsx"},
			"include":         include, "exclude": exclude,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "tsguard: default TypeScript config: %v\n", err)
			return 1
		}
		args = append(args, "--project", file)
	}
	if !r.Run("tsc --noEmit", packagedCommand("tsc", args...)...).ok {
		return 1
	}
	return 0
}

func (r *Runner) packagedSecretArgs() ([]string, error) {
	args := []string{}
	if hasProjectConfig(r.root, ".gitignore") {
		args = append(args, "--secretlintignore", ".gitignore")
	}
	if !hasProjectConfig(r.root, ".secretlintrc", ".secretlintrc.json", ".secretlintrc.yaml", ".secretlintrc.yml", ".secretlintrc.js", ".secretlintrc.cjs", ".secretlintrc.mjs") {
		preset, err := runtimeModule(r.root, "@secretlint/secretlint-rule-preset-recommend")
		if err != nil {
			return nil, err
		}
		file, err := writeRuntimeConfig(r.root, "secretlint.json", map[string]any{"rules": []any{map[string]any{"id": preset}}})
		if err != nil {
			return nil, err
		}
		args = append(args, "--secretlintrc", file)
	}
	// Secretlint accepts file globs, not directories. Let its own walker expand them.
	for _, dir := range r.dirs {
		args = append(args, filepath.ToSlash(filepath.Join(dir, "**", "*")))
	}
	return packagedCommand("secretlint", args...), nil
}
