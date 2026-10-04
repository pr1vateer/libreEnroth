package txt

import (
	"reflect"
	"testing"
)

func TestParse(t *testing.T) {
	in := "Id\tName\tNote\r\n1\t\"Quoted, \"\"x\"\"\"\tplain\r\n2\t\t\n3\n\r\n"
	got := Parse([]byte(in)).Rows
	want := [][]string{
		{"Id", "Name", "Note"},
		{"1", `Quoted, "x"`, "plain"},
		{"2", "", ""},
		{"3"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %q\nwant %q", got, want)
	}
}

func TestCell(t *testing.T) {
	tb := Parse([]byte("a\tb\nc"))
	for _, c := range []struct {
		r, c int
		want string
	}{{0, 1, "b"}, {1, 0, "c"}, {1, 1, ""}, {5, 0, ""}, {-1, 0, ""}, {0, -1, ""}} {
		if got := tb.Cell(c.r, c.c); got != c.want {
			t.Errorf("Cell(%d,%d) = %q, want %q", c.r, c.c, got, c.want)
		}
	}
	if n := len(Parse(nil).Rows); n != 0 {
		t.Errorf("empty input: %d rows", n)
	}
}

func TestLines(t *testing.T) {
	in := "head\r\n1\tA \"x\"\nmore\r\n\r\n2\tB\r\n"
	got := Lines([]byte(in))
	// The final "\n" is a token of its own: an empty last line, as strtok sees it.
	want := []string{"head", "1\tA \"x\"\nmore", "", "2\tB", ""}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Lines = %q, want %q", got, want)
	}
}

func TestStripQuotes(t *testing.T) {
	for in, want := range map[string]string{`"ab"`: "ab", `"ab`: "a", `ab"`: `ab"`, `"`: "", "": "", `"a "" b"`: `a "" b`} {
		if got := StripQuotes(in); got != want {
			t.Errorf("StripQuotes(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestAtoi(t *testing.T) {
	for in, want := range map[string]int32{"12": 12, " -7x": -7, "+3": 3, "x": 0, "": 0, "4294967297": 1, "1.5": 1} {
		if got := Atoi(in); got != want {
			t.Errorf("Atoi(%q) = %d, want %d", in, got, want)
		}
	}
	for in, want := range map[string]float64{"1.5": 1.5, "2": 2, "": 0, "x": 0, " 0.75 ": 0.75, "3.": 3} {
		if got := Atof(in); got != want {
			t.Errorf("Atof(%q) = %v, want %v", in, got, want)
		}
	}
}
