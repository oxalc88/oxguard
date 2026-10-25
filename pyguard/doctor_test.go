package main

import "testing"

func TestSupportedPythonVersion(t *testing.T) {
	tests := []struct {
		version string
		want    bool
	}{
		{version: "Python 3.14.1", want: false},
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
