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
