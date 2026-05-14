package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunDoctorInPnpmProjectDoesNotInvokeBiomeDirectly(t *testing.T) {
	r, logPath := newTestRunner(t)
	if err := os.Mkdir(filepath.Join(r.root, "node_modules"), 0o755); err != nil {
		t.Fatalf("mkdir node_modules: %v", err)
	}
	writeNodeCommand(t, filepath.Join(r.root, "bin", "node"))

	if code := runDoctor(r.root, "pnpm"); code != 0 {
		t.Fatalf("runDoctor returned %d", code)
	}

	for _, entry := range readCommandLog(t, logPath) {
		if strings.Contains(entry, "@biomejs/biome") {
			t.Fatalf("unexpected direct biome package invocation: %q", entry)
		}
		if entry == "pnpm exec biome --version" {
			t.Fatalf("unexpected standalone biome probe: %q", entry)
		}
	}
}

func writeNodeCommand(t *testing.T, path string) {
	t.Helper()

	script := "#!/bin/sh\n" +
		"printf '%s %s\\n' \"$(basename \"$0\")\" \"$*\" >> \"$TSGUARD_LOG\"\n" +
		"if [ \"$1\" = \"--version\" ]; then\n" +
		"  printf 'v22.0.0\\n'\n" +
		"fi\n" +
		"exit 0\n"

	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake node: %v", err)
	}
}
