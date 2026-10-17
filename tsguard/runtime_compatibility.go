package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
)

//go:embed runtime-compatibility.json
var runtimeCompatibilityJSON []byte

// Future admits later majors (npm >=); otherwise only one major is accepted (npm ^).
type nodeVersionRange struct {
	Minimum [3]int `json:"minimum"`
	Future  bool   `json:"future"`
}

var supportedNodeRanges = func() []nodeVersionRange {
	var policy struct {
		Node []nodeVersionRange `json:"node"`
	}
	if err := json.Unmarshal(runtimeCompatibilityJSON, &policy); err != nil {
		panic(err) // The policy is built into the binary, never user input.
	}
	return policy.Node
}()

func nodeEngineRange() string {
	parts := make([]string, 0, len(supportedNodeRanges))
	for _, r := range supportedNodeRanges {
		prefix := "^"
		if r.Future {
			prefix = ">="
		}
		parts = append(parts, fmt.Sprintf("%s%d.%d.%d", prefix, r.Minimum[0], r.Minimum[1], r.Minimum[2]))
	}
	return strings.Join(parts, " || ")
}

func isSupportedNodeVersion(version string) bool {
	var major, minor, patch int
	n, err := fmt.Sscanf(version, "v%d.%d.%d", &major, &minor, &patch)
	if err != nil || n != 3 {
		return false
	}
	for _, r := range supportedNodeRanges {
		min := r.Minimum
		atLeast := major > min[0] || major == min[0] && (minor > min[1] || minor == min[1] && patch >= min[2])
		if atLeast && (r.Future || major == min[0]) {
			return true
		}
	}
	return false
}
