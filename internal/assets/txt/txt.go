// Package txt parses the tab-separated tables (2DEvents.txt, MapStats.txt, ...) of the
// language LODs. Only the generic cell structure is handled here; what each column means
// is decoded by the milestone that uses the table.
package txt

import "strings"

// Table is a parsed table. Rows keep the header lines; callers skip them as the original
// loaders do.
type Table struct {
	Rows [][]string
}

// Parse splits data (Windows-1252 bytes, kept as-is) into rows on LF (CRLF accepted) and
// cells on TAB. A cell wrapped in double quotes loses the quotes and "" becomes ".
// Trailing empty lines are dropped.
func Parse(data []byte) *Table {
	s := string(data)
	lines := strings.Split(s, "\n")
	for n := len(lines); n > 0 && strings.TrimRight(lines[n-1], "\r") == ""; n-- {
		lines = lines[:n-1]
	}
	t := &Table{Rows: make([][]string, 0, len(lines))}
	for _, ln := range lines {
		ln = strings.TrimSuffix(ln, "\r")
		cells := strings.Split(ln, "\t")
		for i, c := range cells {
			cells[i] = unquote(c)
		}
		t.Rows = append(t.Rows, cells)
	}
	return t
}

func unquote(c string) string {
	if len(c) >= 2 && c[0] == '"' && c[len(c)-1] == '"' {
		return strings.ReplaceAll(c[1:len(c)-1], `""`, `"`)
	}
	return c
}

// Cell returns row r, column c, or "" when out of range.
func (t *Table) Cell(r, c int) string {
	if r < 0 || r >= len(t.Rows) || c < 0 || c >= len(t.Rows[r]) {
		return ""
	}
	return t.Rows[r][c]
}
