package contract

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"unicode"
	"unicode/utf8"
)

const FindingLimit = 10
const LineBytes = 240

// Agent output has at most 26 lines and 6 KiB, regardless of analyzer log size.
// Every dynamic line is sanitized and bounded. JSON never applies this reduction.
func Report(w io.Writer, mode string, result *RunResult) error {
	if mode == "json" {
		enc := json.NewEncoder(w)
		enc.SetEscapeHTML(false)
		return enc.Encode(result)
	}
	var b strings.Builder
	fmt.Fprintln(&b, strings.ToUpper(result.Status))
	fmt.Fprintln(&b, "command: "+Text(result.Command))
	gate := "none"
	if len(result.Findings) > 0 {
		gate = result.Findings[0].Gate
	}
	fmt.Fprintln(&b, "gate: "+Text(gate))
	fmt.Fprintf(&b, "findings: %d\n", len(result.Findings))
	for i, f := range result.Findings {
		if i == FindingLimit {
			break
		}
		location := ""
		if f.Location != nil {
			location = " " + f.Location.File
			if f.Location.Line > 0 {
				location += fmt.Sprintf(":%d", f.Location.Line)
			}
		}
		fmt.Fprintln(&b, Text(f.Rule+location+" ["+f.Category+"/"+f.Status+"]"))
		fmt.Fprintln(&b, Text(f.Evidence))
	}
	if omitted := len(result.Findings) - FindingLimit; omitted > 0 {
		fmt.Fprintf(&b, "omitted: %d (use --output json)\n", omitted)
	}
	if len(result.Diagnostics) > 0 {
		fmt.Fprintln(&b, "diagnostics: "+Text(result.Diagnostics[0].Path)+" (paths in --output json)")
	} else {
		fmt.Fprintln(&b, "diagnostics: none")
	}
	_, err := io.WriteString(w, b.String())
	return err
}

func Text(s string) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.IsSpace(r) {
			return ' '
		}
		return r
	}, s)
	s = strings.Join(strings.Fields(s), " ")
	if len(s) <= LineBytes {
		return s
	}
	n := LineBytes - 3
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n] + "..."
}
