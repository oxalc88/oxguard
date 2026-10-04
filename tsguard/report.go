package main

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"unicode"
	"unicode/utf8"
)

const agentFindingLimit = 10
const agentLineBytes = 240

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

// Agent output has at most 26 lines and 6 KiB, regardless of analyzer log size.
// Every dynamic line is sanitized and bounded. JSON never applies this reduction.
func reportResult(w io.Writer, mode string, result *RunResult) error {
	if mode == "json" {
		enc := json.NewEncoder(w)
		enc.SetEscapeHTML(false)
		return enc.Encode(result)
	}
	var b strings.Builder
	fmt.Fprintln(&b, strings.ToUpper(result.Status))
	fmt.Fprintln(&b, "command: "+agentText(result.Command))
	gate := "none"
	if len(result.Findings) > 0 {
		gate = result.Findings[0].Gate
	}
	fmt.Fprintln(&b, "gate: "+agentText(gate))
	fmt.Fprintf(&b, "findings: %d\n", len(result.Findings))
	for i, f := range result.Findings {
		if i == agentFindingLimit {
			break
		}
		location := ""
		if f.Location != nil {
			location = " " + f.Location.File
			if f.Location.Line > 0 {
				location += fmt.Sprintf(":%d", f.Location.Line)
			}
		}
		fmt.Fprintln(&b, agentText(f.Rule+location+" ["+f.Category+"/"+f.Status+"]"))
		fmt.Fprintln(&b, agentText(f.Evidence))
	}
	if omitted := len(result.Findings) - agentFindingLimit; omitted > 0 {
		fmt.Fprintf(&b, "omitted: %d (use --output json)\n", omitted)
	}
	if len(result.Diagnostics) > 0 {
		fmt.Fprintln(&b, "diagnostics: "+agentText(result.Diagnostics[0].Path)+" (paths in --output json)")
	} else {
		fmt.Fprintln(&b, "diagnostics: none")
	}
	_, err := io.WriteString(w, b.String())
	return err
}

func agentText(s string) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.IsSpace(r) {
			return ' '
		}
		return r
	}, s)
	s = strings.Join(strings.Fields(s), " ")
	if len(s) <= agentLineBytes {
		return s
	}
	n := agentLineBytes - 3
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n] + "..."
}
