package ui

import (
	"slices"
	"testing"
)

func TestScript(t *testing.T) {
	sc, err := ParseScript("Up:2, x+Shift:1,-:1,[:1")
	if err != nil {
		t.Fatal(err)
	}
	if sc.Len() != 5 {
		t.Fatalf("Len %d, want 5", sc.Len())
	}
	want := []struct{ held, keys []Key }{
		{[]Key{KeyUp}, []Key{KeyUp}},
		{[]Key{KeyUp}, nil},
		{[]Key{'X', KeyShift}, []Key{'X', KeyShift}},
		{nil, nil},
		{[]Key{KeyBracketLeft}, []Key{KeyBracketLeft}},
	}
	for i, w := range want {
		in := &Input{}
		sc.Next(in)
		if !slices.Equal(in.Held, w.held) || !slices.Equal(in.Keys, w.keys) {
			t.Errorf("tick %d: held %v keys %v, want %v %v", i, in.Held, in.Keys, w.held, w.keys)
		}
	}
	if !sc.Done() {
		t.Error("not done")
	}
	for _, bad := range []string{"Up", "Up:0", "Foo:3", "Up:x"} {
		if _, err := ParseScript(bad); err == nil {
			t.Errorf("%q parsed", bad)
		}
	}
}
