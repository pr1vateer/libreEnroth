package world

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"libre-enroth/internal/evt"
	"libre-enroth/internal/game/clock"
	"libre-enroth/internal/game/ui"
	"libre-enroth/internal/maps/delta"
)

// quietSession is a new session whose notes about later milestones are dropped.
func quietSession() *Session {
	s := NewSession()
	s.Log = func(string, ...any) {}
	return s
}

// global gives the session global.txt.
func (e *env) global(t *testing.T, s *Session) {
	t.Helper()
	_, raw, err := e.d.LangFile("global.txt")
	if err != nil {
		t.Fatal(err)
	}
	g := ui.ParseGlobal(raw)
	s.Global = func(i int) string {
		if i < 0 || i >= len(g) {
			return ""
		}
		return g[i]
	}
}

func (e *env) load(t *testing.T, name string, s *Session) *World {
	t.Helper()
	w, err := Load(e.d, e.tables, e.tex, name, s)
	if err != nil {
		t.Fatal(err)
	}
	return w
}

// runGuarded runs fn and fails the test if it does not return within a few seconds
// (an event that loops forever).
func runGuarded(t *testing.T, what string, fn func()) {
	t.Helper()
	done := make(chan any, 1)
	go func() {
		defer func() { done <- recover() }()
		fn()
	}()
	select {
	case p := <-done:
		if p != nil {
			t.Fatalf("%s: panic: %v", what, p)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("%s: still running after 5 s", what)
	}
}

// TestEventSweep loads every map (running its OnMapReload events and timers) and runs
// every one of its events from step 0, then every global.evt event as an NPC topic and
// as an interactive decoration: none panics or loops.
func TestEventSweep(t *testing.T) {
	e := newEnv(t)
	var maps []string
	for _, en := range e.d.Games.Entries {
		n := strings.ToLower(en.Name)
		if strings.HasSuffix(n, ".odm") || strings.HasSuffix(n, ".blv") {
			maps = append(maps, n)
		}
	}
	if len(maps) != 61 {
		t.Errorf("%d maps, want 61", len(maps))
	}
	events := 0
	for _, name := range maps {
		w := e.load(t, name, quietSession())
		if w.vm.Map == nil {
			continue
		}
		seen := map[int]bool{}
		for _, r := range w.vm.Map.Records {
			id := r.ID()
			if seen[id] {
				continue
			}
			seen[id] = true
			events++
			runGuarded(t, fmt.Sprintf("%s event %d", name, id), func() {
				w.RunEvent(id, true)
				w.Update(&ui.Input{})
			})
			w.travel = nil
		}
	}
	w := e.load(t, "d05.blv", quietSession())
	seen := map[int]bool{}
	for _, r := range e.tables.Global.Records {
		if id := r.ID(); !seen[id] {
			seen[id] = true
			runGuarded(t, fmt.Sprintf("global event %d", id), func() {
				w.vm.Run(evt.Source{Kind: evt.SourceGlobal}, id, 0, true)
				w.vm.Run(evt.Source{Kind: evt.SourceDecoration, Dec: 0}, id, 0, true)
			})
			w.travel = nil
		}
	}
	t.Logf("%d map events in %d maps, %d global events", events, len(maps), len(seen))
}

// TestD05Reload: d05's OnMapReload (event 2) closes doors 108..113 and clears the
// lever variables while map variable 10 is below 15.
//
// d05.evt event 2: Compare var 0x87 >= 15 -> 14; doors 0x6c, 0x6d close; vars 0x7d..0x84,
// 0x87 = 0; jump 16; doors 0x6e..0x71 close.
func TestD05Reload(t *testing.T) {
	e := newEnv(t)
	w := e.load(t, "d05.blv", quietSession())
	for id := uint32(108); id <= 113; id++ {
		d := w.indoor.door(id)
		if d == nil {
			t.Fatalf("no door %d", id)
		}
		if d.State != delta.DoorClosing && d.State != delta.DoorClosed {
			t.Errorf("door %d state %d, want closing or closed", id, d.State)
		}
	}
	// The doors finish closing.
	for range 600 {
		w.Update(&ui.Input{})
	}
	for id := uint32(108); id <= 113; id++ {
		if d := w.indoor.door(id); d.State != delta.DoorClosed {
			t.Errorf("door %d state %d after 10 s, want closed", id, d.State)
		}
	}
}

// TestOut01Timers: out01 registers six countdown timers and event 500, which at 10:00
// every day sets quest bit 232 (after its SpeakNPC, M6's).
//
// mm8: 0x441caf (Evt_InitTimers), 0x446fb5 (Evt_TimerScan)
func TestOut01Timers(t *testing.T) {
	e := newEnv(t)
	s := quietSession()
	s.SetTimeOfDay(9, 0)
	w := e.load(t, "out01.odm", s)
	var ids, reloads []int
	for _, tm := range w.timers.List {
		ids = append(ids, tm.ID)
		reloads = append(reloads, tm.Reload)
	}
	if want := []int{456, 460, 463, 468, 469, 479, 500}; !slices.Equal(ids, want) {
		t.Fatalf("timers %v, want %v", ids, want)
	}
	if want := []int{30, 30, 20, 40, 40, 20, 0}; !slices.Equal(reloads, want) {
		t.Errorf("intervals %v, want %v", reloads, want)
	}
	if tm := w.timers.List[6]; tm.Hour != 10 || tm.Minute != 0 || tm.Op != evt.OpOnTimer {
		t.Errorf("event 500: %+v", tm)
	}
	m := s.Party
	step := func(to clock.Time) {
		for m.Time < to {
			m.Time = min(m.Time+128, to)
			w.Update(&ui.Input{})
		}
	}
	step(clock.Time(9)*clock.Hour + 59*clock.Minute)
	if m.QBits.Get(232) {
		t.Fatal("quest bit 232 set before 10:00")
	}
	step(clock.Time(10)*clock.Hour + clock.Minute)
	if !m.QBits.Get(232) {
		t.Error("quest bit 232 not set at 10:00")
	}
	if c := w.timers.List[0]; c.Countdown <= 0 || c.Countdown > 30 {
		t.Errorf("event 456 countdown %d", c.Countdown)
	}
}

// TestD05ToOut01: event 501 (the exit of the Abandoned Temple, exit picture 1) asks "Do
// you wish to leave Abandoned Temple?" first; Close stays, OK moves the party to out01
// at (-12789, 18734) facing 1536; the temple keeps its state for the next visit.
//
// mm8: 0x4446bd (case 6), 0x4425f5 (Evt_TransitionDialog), 0x42f877 (msgs 0x19b,
// 0x19c), 0x447f80 (Evt_Travel), 0x44808d (Level_PlaceParty)
func TestD05ToOut01(t *testing.T) {
	e := newEnv(t)
	s := quietSession()
	e.global(t, s)
	d05 := e.load(t, "d05.blv", s)
	d05.MapVars()[50] = 7
	if res := d05.RunEvent(501, true); res.Moved {
		t.Fatal("event 501 moved without asking")
	}
	tr := d05.TransitionDialog()
	if tr == nil || tr.Icon != "ticon01" || tr.Video != "" || tr.Title != "Abandoned Temple" ||
		tr.Text != "Do you wish to leave Abandoned Temple?" || tr.Topbar != "topbar" {
		t.Fatalf("transition %+v", tr)
	}
	d05.AnswerTransition(false)
	if _, ok := d05.Travel(); ok || d05.TransitionDialog() != nil {
		t.Fatal("Close travelled or stayed open")
	}
	d05.RunEvent(501, true)
	d05.AnswerTransition(true)
	name, ok := d05.Travel()
	if !ok || name != "out01.odm" {
		t.Fatalf("Travel %q %v", name, ok)
	}
	if _, again := d05.Travel(); again {
		t.Error("Travel reported twice")
	}
	out := e.load(t, name, s)
	p := out.PartyState()
	if p.X != -12789 || p.Y != 18734 || p.Dir != 1536 {
		t.Errorf("party at %d,%d facing %d; want -12789,18734 facing 1536", p.X, p.Y, p.Dir)
	}
	// z 1857 is just above the floor of the temple's model, not the terrain.
	if z, _, _, _ := out.outdoor.geo.FloorZ(p.X, p.Y, 1857, false, false); p.Z < z || p.Z > z+2 || z < 1800 {
		t.Errorf("party z %d, floor %d", p.Z, z)
	}
	back := e.load(t, "d05.blv", s)
	if back != d05 || back.MapVars()[50] != 7 {
		t.Error("d05 did not come back as it was left")
	}
}

// TestClickDoor: a click on d05's door (face 3307, event 11) opens door 1 and shows
// "Door" on hover; Space in front of it does the same; a click on a plain wall says
// "Nothing here" (global.txt 0x209) and one beyond 512 units nothing.
//
// mm8: 0x421a65 (Evt_Click), 0x469b64 (Evt_Interact), 0x420aab (Mouse_UpdateHover)
func TestClickDoor(t *testing.T) {
	e := newEnv(t)
	for _, how := range []string{"click", "space"} {
		t.Run(how, func(t *testing.T) {
			a := e.app(t, "d05.blv")
			w := a.World().(*World)
			w.Cam = FreeCam{X: 8512, Y: 2150, Z: -640, Yaw: 512}
			w.SetPartyFromCam()
			Compose(a, 640, 480)
			if got := w.Hover(320, 200); got != "Door" {
				t.Errorf("hover %q, want Door", got)
			}
			if how == "click" {
				w.Click(320, 200)
			} else {
				w.Interact()
			}
			if d := w.indoor.door(1); d.State != delta.DoorOpening {
				t.Errorf("door 1 state %d, want opening", d.State)
			}
		})
	}
	a := e.app(t, "d05.blv")
	w := a.World().(*World)
	st := &recStatus{}
	w.S.Status, w.S.Global = st, func(i int) string { return fmt.Sprintf("global %#x", i) }
	w.Cam = FreeCam{X: 8512, Y: 2150, Z: -640, Yaw: 0} // facing the side wall (+x)
	w.SetPartyFromCam()
	Compose(a, 640, 480)
	w.Click(320, 200)
	if !slices.Equal(st.shown, []string{"global 0x209"}) {
		t.Errorf("wall click: status %q, want Nothing here", st.shown)
	}
	w.Cam = FreeCam{X: 8512, Y: 1800, Z: -640, Yaw: 512} // the door 700 units away
	w.SetPartyFromCam()
	Compose(a, 640, 480)
	w.Click(320, 200)
	if d := w.indoor.door(1); d.State != delta.DoorClosed || len(st.shown) != 1 {
		t.Errorf("far click: door state %d, status %q", d.State, st.shown)
	}
}

type recStatus struct{ shown []string }

func (s *recStatus) Show(text string, _ int) { s.shown = append(s.shown, text) }
func (s *recStatus) Timed() string           { return "" }
