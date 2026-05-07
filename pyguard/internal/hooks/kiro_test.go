package hooks

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestGenerateKiroHook(t *testing.T) {
	tmp := t.TempDir()
	proj := filepath.Join(tmp, "py-app")
	if err := os.MkdirAll(proj, 0o755); err != nil {
		t.Fatalf("setup: create proj dir: %v", err)
	}
	if err := GenerateKiroHook(proj); err != nil {
		t.Fatalf("GenerateKiroHook: %v", err)
	}

	t.Run("hook file exists at correct path", func(t *testing.T) {
		hookPath := filepath.Join(tmp, ".kiro", "hooks", "pyguard.kiro.hook")
		if _, err := os.Stat(hookPath); os.IsNotExist(err) {
			t.Fatalf("hook file not found at %s", hookPath)
		}
	})

	t.Run("agent file exists at correct path", func(t *testing.T) {
		agentPath := filepath.Join(tmp, ".kiro", "agents", "pyguard.json")
		if _, err := os.Stat(agentPath); os.IsNotExist(err) {
			t.Fatalf("agent file not found at %s", agentPath)
		}
	})

	t.Run("hook file contains valid JSON", func(t *testing.T) {
		hookPath := filepath.Join(tmp, ".kiro", "hooks", "pyguard.kiro.hook")
		data, err := os.ReadFile(hookPath)
		if err != nil {
			t.Fatalf("read hook file: %v", err)
		}
		var v any
		if err := json.Unmarshal(data, &v); err != nil {
			t.Fatalf("hook file is not valid JSON: %v", err)
		}
	})

	t.Run("agent file contains valid JSON", func(t *testing.T) {
		agentPath := filepath.Join(tmp, ".kiro", "agents", "pyguard.json")
		data, err := os.ReadFile(agentPath)
		if err != nil {
			t.Fatalf("read agent file: %v", err)
		}
		var v any
		if err := json.Unmarshal(data, &v); err != nil {
			t.Fatalf("agent file is not valid JSON: %v", err)
		}
	})
}
