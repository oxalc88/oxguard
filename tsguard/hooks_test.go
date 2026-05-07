package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestGenerateKiroHook(t *testing.T) {
	tmp := t.TempDir()
	proj := filepath.Join(tmp, "ts-app")
	if err := os.MkdirAll(proj, 0o755); err != nil {
		t.Fatalf("setup: MkdirAll proj: %v", err)
	}

	if err := generateKiroHook(proj); err != nil {
		t.Fatalf("generateKiroHook returned error: %v", err)
	}

	hookPath := filepath.Join(tmp, ".kiro", "hooks", "tsguard.kiro.hook")
	agentPath := filepath.Join(tmp, ".kiro", "agents", "tsguard.json")

	hookData, err := os.ReadFile(hookPath)
	if err != nil {
		t.Fatalf(".kiro/hooks/tsguard.kiro.hook not found: %v", err)
	}

	agentData, err := os.ReadFile(agentPath)
	if err != nil {
		t.Fatalf(".kiro/agents/tsguard.json not found: %v", err)
	}

	var hookJSON any
	if err := json.Unmarshal(hookData, &hookJSON); err != nil {
		t.Fatalf(".kiro/hooks/tsguard.kiro.hook is not valid JSON: %v", err)
	}

	var agentJSON any
	if err := json.Unmarshal(agentData, &agentJSON); err != nil {
		t.Fatalf(".kiro/agents/tsguard.json is not valid JSON: %v", err)
	}
}
