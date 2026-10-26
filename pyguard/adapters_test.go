package main

import (
	"strings"
	"testing"
)

// mypy 2.4.0 emits zero-based columns for every diagnostic, including syntax
// diagnostics. Verified empirically: for parser errors on exit 2 and for
// semantic/version syntax errors on exit 1, mypy's own renderers treat the
// JSON column as zero-based (human output prints 1 + column; --pretty places
// the caret at that zero-based offset).
func TestDecodeMypyJSONNormalizesSyntaxColumns(t *testing.T) {
	cases := []struct {
		name, line string
		wantColumn int
	}{
		{
			// mypy -c "type X = int" --python-version 3.11 (exit 1):
			// the offending "type" keyword starts at zero-based column 0.
			name:       "version syntax error at column 0",
			line:       `{"file":"<string>","line":1,"column":0,"end_line":1,"end_column":1,"message":"\"type\" statements: requires Python 3.12 or newer","hint":null,"code":"syntax","severity":"error"}`,
			wantColumn: 1,
		},
		{
			// Parser error on exit 2 (fixture syntax-error): the JSON column
			// is zero-based like every other diagnostic.
			name:       "parser syntax error on exit 2",
			line:       `{"file":"src/main.py","line":2,"column":23,"end_line":2,"end_column":24,"message":"Expected a statement","hint":null,"code":"syntax","severity":"error"}`,
			wantColumn: 24,
		},
		{
			name:       "type diagnostic",
			line:       `{"file":"src/main.py","line":1,"column":9,"end_line":1,"end_column":12,"message":"Incompatible types in assignment","hint":null,"code":"assignment","severity":"error"}`,
			wantColumn: 10,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rows, ok := decodeMypyJSON(strings.NewReader(c.line + "\n"))
			if !ok || len(rows) != 1 {
				t.Fatalf("ok=%v rows=%v", ok, rows)
			}
			if rows[0].Column != c.wantColumn {
				t.Fatalf("column = %d, want %d", rows[0].Column, c.wantColumn)
			}
		})
	}
}
