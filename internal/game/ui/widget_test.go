package ui

import (
	"testing"

	"libre-enroth/internal/gfx"
)

func sprite(w, h int) *gfx.Sprite {
	return &gfx.Sprite{W: w, H: h, Pix: make([]byte, 4*w*h)}
}

// tick feeds one input to a container and returns the messages it produced.
func tick(ct *Container, in Input) []Msg {
	ct.Update(&in)
	return ct.Queue.Drain()
}

func TestHitIsInclusive(t *testing.T) {
	b := NewButton(5, 5, Msg{ID: 1}, sprite(10, 10), nil, nil)
	for _, c := range []struct {
		x, y int
		in   bool
	}{{5, 5, true}, {15, 15, true}, {16, 15, false}, {4, 10, false}, {10, 16, false}} {
		if got := hit(b.Rect(), c.x, c.y); got != c.in {
			t.Errorf("hit(%d,%d) = %v", c.x, c.y, got)
		}
	}
}

func TestButtonStates(t *testing.T) {
	b := NewButton(10, 10, Msg{ID: 7, Param: 3}, sprite(20, 10), sprite(20, 10), sprite(20, 10))
	var ct Container
	ct.Add(b)

	tick(&ct, Input{X: 0, Y: 0})
	if b.State != StateUp {
		t.Fatalf("idle: state %d", b.State)
	}
	tick(&ct, Input{X: 15, Y: 15})
	if b.State != StateHover {
		t.Fatalf("hover: state %d", b.State)
	}
	if m := tick(&ct, Input{X: 15, Y: 15, Left: true, LeftPressed: true}); len(m) != 0 || b.State != StateDown {
		t.Fatalf("press: msgs %v state %d", m, b.State)
	}
	// Drag out: leave -> up; back in while held -> down.
	tick(&ct, Input{X: 50, Y: 50, Left: true})
	if b.State != StateUp {
		t.Fatalf("dragged out: state %d", b.State)
	}
	tick(&ct, Input{X: 12, Y: 12, Left: true})
	if b.State != StateDown {
		t.Fatalf("dragged back: state %d", b.State)
	}
	// Release inside: activates; the icon goes to "up" even though still hovered.
	m := tick(&ct, Input{X: 12, Y: 12, LeftReleased: true})
	if len(m) != 1 || m[0] != (Msg{ID: 7, Param: 3}) || b.State != StateUp {
		t.Fatalf("release inside: msgs %v state %d", m, b.State)
	}
	// Press inside, release outside: no message.
	tick(&ct, Input{X: 12, Y: 12, Left: true, LeftPressed: true})
	if m := tick(&ct, Input{X: 40, Y: 40, LeftReleased: true}); len(m) != 0 {
		t.Fatalf("release outside: msgs %v", m)
	}
	// Press outside, release inside: no message either.
	tick(&ct, Input{X: 40, Y: 40, Left: true, LeftPressed: true})
	if m := tick(&ct, Input{X: 12, Y: 12, LeftReleased: true}); len(m) != 0 {
		t.Fatalf("press outside: msgs %v", m)
	}
}

// The last child added gets events first (the original walks its circular child list
// from the tail: 0x4c3fde, 0x4c4338).
func TestHotkeyAndOrder(t *testing.T) {
	a := NewButton(0, 0, Msg{ID: 1}, sprite(10, 10), nil, nil)
	b := NewButton(0, 0, Msg{ID: 2}, sprite(10, 10), nil, nil) // overlaps a
	a.Hotkey, b.Hotkey = 'N', 'N'
	var ct Container
	ct.Add(a)
	ct.Add(b)
	if m := tick(&ct, Input{X: 100, Y: 100, Keys: []Key{'N'}}); len(m) != 1 || m[0].ID != 2 || b.State != StateDown {
		t.Errorf("hotkey: %v, state %d (the last child wins, shows pressed)", m, b.State)
	}
	tick(&ct, Input{X: 100, Y: 100, LeftReleased: true}) // any release resets the pressed icon
	if b.State != StateUp {
		t.Errorf("after release: state %d", b.State)
	}
	tick(&ct, Input{X: 5, Y: 5, Left: true, LeftPressed: true})
	if m := tick(&ct, Input{X: 5, Y: 5, LeftReleased: true}); len(m) != 1 || m[0].ID != 2 {
		t.Errorf("overlap click: %v (the last child consumes)", m)
	}
}

func TestDisabledButton(t *testing.T) {
	b := NewButton(0, 0, Msg{ID: 1}, sprite(10, 10), nil, nil)
	b.Icons[StateDisabled] = sprite(10, 10)
	b.SetDisabled(true)
	var ct Container
	ct.Add(b)
	tick(&ct, Input{X: 5, Y: 5, Left: true, LeftPressed: true})
	if m := tick(&ct, Input{X: 5, Y: 5, LeftReleased: true}); len(m) != 0 || b.State != StateDisabled {
		t.Errorf("disabled: %v state %d", m, b.State)
	}
}

func TestQueueCap(t *testing.T) {
	var q MsgQueue
	for i := 0; i < 50; i++ {
		q.Post(Msg{ID: i})
	}
	if m := q.Drain(); len(m) != msgQueueCap || m[39].ID != 39 {
		t.Errorf("queue kept %d", len(m))
	}
}

func TestParseGlobal(t *testing.T) {
	raw := []byte("Global Text\t\r\n\r\n0\tAC\r\n1\t\"Quoted, text\"\t\r\n2\tLast")
	got := ParseGlobal(raw)
	want := []string{"AC", "Quoted, text", "Last"}
	if len(got) != len(want) {
		t.Fatalf("ParseGlobal = %q", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestClassForFace(t *testing.T) {
	want := map[int]int{0: 4, 3: 4, 4: 2, 8: 0, 12: 12, 16: 10, 19: 10, 20: 8, 22: 6, 23: 6, 24: 14}
	for face, class := range want {
		if got := ClassForFace(face); got != class {
			t.Errorf("ClassForFace(%d) = %d, want %d", face, got, class)
		}
	}
	if !IsFemale(1) || IsFemale(2) || IsFemale(21) || !IsFemale(27) {
		t.Error("IsFemale")
	}
}
