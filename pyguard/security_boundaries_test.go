package main

import (
	"reflect"
	"testing"
)

func TestSecretsScansConfiguredProjectDirectories(t *testing.T) {
	for _, tc := range []struct {
		name    string
		dirs    []string
		machine bool
		want    []string
	}{
		{"human scope", []string{"src", "tests"}, false, []string{"uv", "run", "python", "tools/analysis/check_secrets.py", "src", "tests"}},
		{"structured scope", []string{"app", "packages"}, true, []string{"uv", "run", "python", "tools/analysis/check_secrets.py", "app", "packages", "--json"}},
		{"default project scope", nil, false, []string{"uv", "run", "python", "tools/analysis/check_secrets.py", "."}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := secretsScanArgs(tc.dirs, tc.machine)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("secrets argv = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestSetupNeverGlobalizesProjectBinary(t *testing.T) {
	root := t.TempDir()
	if _, err := ensureRepoPknBinary(root); err != nil {
		t.Fatal(err)
	}
	if enabled, _ := installPkn(root); enabled {
		t.Fatal("setup installed project-owned binary globally")
	}
}
