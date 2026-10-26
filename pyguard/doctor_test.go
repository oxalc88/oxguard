package main

import "testing"

func TestSupportedPythonVersion(t *testing.T) {
	tests := []struct {
		version string
		want    bool
	}{
		{version: "Python 3.14.1", want: false},
		{version: "Python 3.14.1+", want: false},
		{version: "Python 3.14.2", want: true},
		{version: "Python 3.14.0", want: true},
		{version: "Python 3.13.5", want: true},
		{version: "Python 3.12.0", want: true},
		{version: "Python 3.12.14", want: true},
		{version: "Python 3.11.9", want: false},
		{version: "Python 3.10.12", want: false},
		{version: "Python 3.14.1 (main, Feb  2026)", want: false},
		{version: "Python 3.14.2 (main, Feb  2026)", want: true},
		{version: "Python 4.0.0", want: true},
		{version: "Python 2.7.18", want: false},
		{version: "  Python 3.12.0", want: true},
		{version: "3.12.0", want: false},
		{version: "python 3.12.0", want: false},
		{version: "Python 3.14", want: true},
		{version: "Python 3.12.15+", want: true},
		{version: "Python 3.13.0rc1", want: true},
		{version: "Python 3.14.0b2", want: true},
		{version: "Python 3.14.1+ (main, Feb  2026)", want: false},
		{version: "Python 3.11.0rc1", want: false},
		{version: "Python 3.12x.15", want: false},
		{version: "Python 3.12.", want: false},
		{version: "Python .12.15", want: false},
		{version: "Python 3.11", want: false},
		{version: "Python", want: false},
		{version: "Python x.y.z", want: false},
		{version: "", want: false},
		{version: "not a version", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.version, func(t *testing.T) {
			if got := isSupportedPythonVersion(tt.version); got != tt.want {
				t.Errorf("isSupportedPythonVersion(%q) = %v, want %v", tt.version, got, tt.want)
			}
		})
	}
}

// Version banners from real interpreters carry a numeric component plus an
// optional non-digit tail (+, rc1, a1, b2); garbage must not parse at all.
func TestParsePythonVersion(t *testing.T) {
	tests := []struct {
		version string
		major   int
		minor   int
		patch   int
		ok      bool
	}{
		{version: "Python 3.14.1", major: 3, minor: 14, patch: 1, ok: true},
		{version: "Python 3.14.1+", major: 3, minor: 14, patch: 1, ok: true},
		{version: "Python 3.12.15+", major: 3, minor: 12, patch: 15, ok: true},
		{version: "Python 3.13.0rc1", major: 3, minor: 13, patch: 0, ok: true},
		{version: "Python 3.14.0b2", major: 3, minor: 14, patch: 0, ok: true},
		{version: "Python 3.14.0a7+ (main)", major: 3, minor: 14, patch: 0, ok: true},
		{version: "Python 3.14rc1", major: 3, minor: 14, patch: 0, ok: true},
		{version: "Python 3.12", major: 3, minor: 12, patch: 0, ok: true},
		{version: "Python 3.12x.15", ok: false},
		{version: "Python 3.12.rc1", ok: false},
		{version: "Python 3.12.", ok: false},
		{version: "Python .12.15", ok: false},
		{version: "Python x.y.z", ok: false},
		{version: "Python 3", ok: false},
		{version: "3.12.15+", ok: false},
		{version: "", ok: false},
	}

	for _, tt := range tests {
		t.Run(tt.version, func(t *testing.T) {
			major, minor, patch, ok := parsePythonVersion(tt.version)
			if ok != tt.ok {
				t.Fatalf("parsePythonVersion(%q) ok = %v, want %v", tt.version, ok, tt.ok)
			}
			if !ok {
				return
			}
			if major != tt.major || minor != tt.minor || patch != tt.patch {
				t.Errorf("parsePythonVersion(%q) = %d.%d.%d, want %d.%d.%d", tt.version, major, minor, patch, tt.major, tt.minor, tt.patch)
			}
		})
	}
}
