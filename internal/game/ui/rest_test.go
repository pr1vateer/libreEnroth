package ui

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"libre-enroth/internal/game/clock"
	"libre-enroth/internal/game/party"
	"libre-enroth/internal/gfx"
	"libre-enroth/internal/render"
)

// restWorld is a World the party can rest in, with a fixed refusal.
type restWorld struct {
	refusal  int
	food     int
	ambushed bool
	done     int
}

func (w *restWorld) Update(*Input)                  {}
func (w *restWorld) Render(*render.Frame)           {}
func (w *restWorld) MapName() string                { return "test.blv" }
func (w *restWorld) Outdoor() bool                  { return false }
func (w *restWorld) Party() (float64, float64, int) { return 0, 0, 0 }
func (w *restWorld) MapMarkers(func(x, y float64))  {}
func (w *restWorld) RestRefusal() int               { return w.refusal }
func (w *restWorld) RestFood() int                  { return w.food }
func (w *restWorld) RestEncounter() bool            { return w.ambushed }
func (w *restWorld) RestDone()                      { w.done++ }

func newRestApp(t *testing.T, w *restWorld, faces []int) (*App, *inGame) {
	t.Helper()
	a := newTestAppParty(t, StateTitle, faces)
	a.r.LoadWorld = func(string) (World, error) { return w, nil }
	if err := a.enter(StateInGame); err != nil {
		t.Fatal(err)
	}
	return a, a.Screen().(*inGame)
}

func canvasHash(a *App) (string, *gfx.Canvas) {
	c := gfx.NewCanvas()
	a.Draw(c)
	sum := sha256.Sum256(c.Img.Pix)
	return hex.EncodeToString(sum[:]), c
}

// TestRestHeal: R opens the rest screen (the viewport is covered); R again rests: the
// food is eaten, everyone sleeps, 8 hours pass in 80 frames of 6 minutes (each adding
// the frame's ticks), then everyone wakes and the screen closes.
//
// mm8: 0x42f877 (msgs 0x68, 0x61), 0x41f297 (Rest_Step), 0x4d112c (Rest_Close)
func TestRestHeal(t *testing.T) {
	w := &restWorld{food: 2}
	a, g := newRestApp(t, w, []int{0, 5})
	m := a.r.Party
	run(t, a, Input{Keys: []Key{'R'}})
	if g.rest == nil || !a.Viewport().Empty() || !m.Resting {
		t.Fatal("rest screen not open")
	}
	got, c := canvasHash(a)
	if *update {
		writePNG(t, c, "rest_open")
		t.Logf("%q: %q,", "rest_open", got)
	} else if want := screenHashes["rest_open"]; got != want {
		t.Errorf("rest_open canvas %s, want %s", got, want)
	}
	start := m.Time
	run(t, a, Input{Keys: []Key{'R'}})
	if g.rest == nil || g.rest.mode != restResting || m.Food != 5 {
		t.Fatalf("not resting: food %d", m.Food)
	}
	if m.Players[0].Conditions[party.CondAsleep] == 0 {
		t.Error("not asleep")
	}
	frames := 1
	for g.rest != nil && frames < 200 {
		run(t, a, Input{})
		frames++
	}
	// R's own tick takes the first step; after the 80th a last one ends the rest and
	// the tick after that closes the screen.
	if frames != 82 {
		t.Errorf("rest took %d frames", frames)
	}
	if d := m.Time - start; d < 8*clock.Hour || d > 8*clock.Hour+3*81 {
		t.Errorf("rest moved the clock %d ticks", d)
	}
	if m.Players[0].Conditions[party.CondAsleep] != 0 || m.Resting || w.done != 1 {
		t.Errorf("after rest: asleep %d resting %v done %d", m.Players[0].Conditions[party.CondAsleep], m.Resting, w.done)
	}
}

// TestRestWaitDawn: waiting until dawn from 9:00 passes 20 hours, 200 steps of 6
// minutes, each with the frame's 2 or 3 ticks on top (Party_UpdateTime adds g_timer.dt
// in Party_AdvanceMinutes), so it ends a minute and a half past 5.
func TestRestWaitDawn(t *testing.T) {
	a, g := newRestApp(t, &restWorld{food: 2}, nil)
	m := a.r.Party
	run(t, a, Input{Keys: []Key{'R'}}, Input{Keys: []Key{'D'}})
	for range 400 {
		run(t, a, Input{})
	}
	if g.rest == nil || m.Calendar.Hour != 5 || m.Calendar.Minute != 1 || m.Calendar.Day != 1 {
		t.Errorf("dawn: %+v", m.Calendar)
	}
	run(t, a, Input{Keys: []Key{KeyEscape}})
	if g.rest != nil || m.Resting {
		t.Error("Esc did not close")
	}
	// Esc during a wait finishes it at once.
	run(t, a, Input{Keys: []Key{'R'}}, Input{Keys: []Key{'H'}}, Input{Keys: []Key{KeyEscape}})
	if g.rest != nil || m.Calendar.Hour != 6 {
		t.Errorf("Esc mid-wait: %+v", m.Calendar)
	}
}

// TestRestRefusals: the world's refusal and missing food go to the status line.
func TestRestRefusals(t *testing.T) {
	w := &restWorld{refusal: 0x1de, food: 2}
	a, g := newRestApp(t, w, nil)
	run(t, a, Input{Keys: []Key{'R'}})
	if want := a.r.GlobalText(0x1de); g.rest != nil || a.r.Status.Timed() != want || want == "" {
		t.Errorf("refusal: open %v status %q", g.rest != nil, a.r.Status.Timed())
	}
	for range 2*60 + 1 {
		run(t, a, Input{})
	}
	if a.r.Status.Timed() != "" {
		t.Error("message did not expire after 2 s")
	}
	w.refusal, w.food = 0, 9
	run(t, a, Input{Keys: []Key{'R'}}, Input{Keys: []Key{'R'}})
	if want := a.r.GlobalText(0x1e2); g.rest == nil || g.rest.mode != restIdle || a.r.Status.Timed() != want {
		t.Errorf("no food: status %q", a.r.Status.Timed())
	}
}

// TestTurnBasedClock: Enter stops the party's clock for the world.
func TestTurnBasedClock(t *testing.T) {
	a, _ := newRestApp(t, &restWorld{}, nil)
	run(t, a, Input{Keys: []Key{KeyEnter}})
	if !a.r.Party.TurnBased {
		t.Fatal("Enter did not start turn-based mode")
	}
	run(t, a, Input{Keys: []Key{KeyEnter}})
	if a.r.Party.TurnBased {
		t.Fatal("Enter did not end turn-based mode")
	}
}

// TestHoverStatus: the minimap shows the date and time, a portrait the member.
//
// mm8: 0x42f877 (msgs 0x5c, 0x5e), 0x41bf09 (Hud_DrawStatusLine)
func TestHoverStatus(t *testing.T) {
	a, _ := newRestApp(t, &restWorld{}, []int{0, 5})
	a.r.Party.Players[0].Name = "Zoltan"
	run(t, a, Input{X: 540, Y: 420})
	if got := a.r.Status.Hover(); got != "9:00am Monday 1 January 1172" {
		t.Errorf("minimap hover %q", got)
	}
	got, c := canvasHash(a)
	if *update {
		writePNG(t, c, "hover_time")
		t.Logf("%q: %q,", "hover_time", got)
	} else if want := screenHashes["hover_time"]; got != want {
		t.Errorf("hover_time canvas %s, want %s", got, want)
	}
	run(t, a, Input{X: portraitX[0] + 10, Y: portraitY + 10})
	if got := a.r.Status.Hover(); got != "Zoltan the Knight: Good" {
		t.Errorf("portrait hover %q", got)
	}
	run(t, a, Input{X: 320, Y: 200})
	if got := a.r.Status.Hover(); got != "" {
		t.Errorf("hover over nothing %q", got)
	}
}
