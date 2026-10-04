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
	sc, err = ParseScript("Click@320/200:2,Mouse@5/6+Space:1")
	if err != nil {
		t.Fatal(err)
	}
	for i, w := range []Input{
		{X: 320, Y: 200, Left: true, LeftPressed: true},
		{X: 320, Y: 200, Left: true, LeftReleased: true},
		{X: 5, Y: 6, Keys: []Key{KeySpace}, Held: []Key{KeySpace}},
	} {
		in := &Input{}
		sc.Next(in)
		if in.X != w.X || in.Y != w.Y || in.Left != w.Left || in.LeftPressed != w.LeftPressed ||
			in.LeftReleased != w.LeftReleased || !slices.Equal(in.Keys, w.Keys) {
			t.Errorf("tick %d: %+v, want %+v", i, *in, w)
		}
	}
	for _, bad := range []string{"Up", "Up:0", "Foo:3", "Up:x", "Click@1:1", "Drag@1/2:1"} {
		if _, err := ParseScript(bad); err == nil {
			t.Errorf("%q parsed", bad)
		}
	}
}
