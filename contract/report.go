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
	if err := result.Validate(); err != nil {
		return err
	}
	if mode == "json" {
		enc := json.NewEncoder(w)
		enc.SetEscapeHTML(false)
		return enc.Encode(result)
	}
	if mode == "human" && result.Command == "diagnose" {
		fmt.Fprintf(w, "%s: assessment %s\n", strings.ToUpper(result.Status.String()), result.Assessment)
		for _, a := range result.Analyses {
			fmt.Fprintf(w, "  %s: %s (%s)", Text(a.Name), a.Status, a.Normalization)
			if a.Reason != "" {
				fmt.Fprint(w, " — "+Text(a.Reason))
			}
			fmt.Fprintln(w)
		}
		for _, f := range result.Findings {
			location := ""
			if f.Location != nil {
				location = " " + Text(f.Location.File)
			}
			fmt.Fprintf(w, "  %s%s [%s]: %s\n", Text(f.Rule), location, f.Status, Text(f.Evidence))
		}
		_, err := fmt.Fprintln(w, "Full findings, analysis coverage and diagnostic paths: use --output json.")
		return err
	}
	var b strings.Builder
	fmt.Fprintln(&b, strings.ToUpper(result.Status.String()))
	fmt.Fprintln(&b, "command: "+Text(result.Command))
	gate := "none"
	if len(result.Findings) > 0 {
		gate = result.Findings[0].Gate
	}
	notRun, partial := 0, 0
	for _, g := range result.Gates {
		if g.Status == StatusNotRun {
			notRun++
		}
		if g.Normalization == Partial {
			partial++
		}
	}
	coverage := fmt.Sprintf("gate: %s; assessment: %s; not_run: %d; partial: %d", gate, result.Assessment, notRun, partial)
	if len(result.Analyses) > 0 {
		incomplete := 0
		for _, a := range result.Analyses {
			if a.Status == StatusNotRun || a.Status == StatusError || a.Normalization != Complete {
				incomplete++
			}
		}
		coverage += fmt.Sprintf("; incomplete_analyses: %d (details: --output json)", incomplete)
	}
	fmt.Fprintln(&b, Text(coverage))
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
		fmt.Fprintln(&b, Text(f.Rule+location+" ["+f.Category+"/"+f.Status.String()+"]"))
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
