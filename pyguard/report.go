package main

import (
	"fmt"
	"github.com/oxalc88/oxguard/contract"
	"io"
	"strings"
)

const agentFindingLimit = contract.FindingLimit
const agentLineBytes = contract.LineBytes

// Preserve the human CLI. Raw text is optional diagnostic display, never the
// source of reporter decisions or the machine contract.
func reportHumanTool(w io.Writer, name string, result *RunResult, logFile, diagnostics string) {
	if result.ExitCode == 0 {
		fmt.Fprintf(w, "  [OK]   %s\n", name)
	} else if logFile != "" {
		fmt.Fprintf(w, "  [FAIL] %s (see %s)\n", name, logFile)
	} else {
		fmt.Fprintf(w, "  [FAIL] %s\n", name)
		if diagnostics != "" {
			for _, line := range strings.Split(strings.TrimRight(diagnostics, "\n"), "\n") {
				fmt.Fprintf(w, "         %s\n", line)
			}
		}
	}
}

func reportResult(w io.Writer, mode string, result *RunResult) error {
	return contract.Report(w, mode, result)
}
func agentText(s string) string { return contract.Text(s) }
