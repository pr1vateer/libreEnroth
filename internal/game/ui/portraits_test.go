package ui

import (
	"testing"

	"libre-enroth/internal/game/party"
)

func inGameOf(t *testing.T, a *App) *inGame {
	t.Helper()
	g, ok := a.Screen().(*inGame)
	if !ok {
		t.Fatalf("screen %T", a.Screen())
	}
	return g
}

// Hotkeys 1-5 and clicks select members (GuiPortraits_Build 0x4ca6a8, msg 0x6e ->
// Party_ClickPortrait 0x4213c0).
func TestPortraitSelect(t *testing.T) {
	a := newTestAppParty(t, StateInGame, []int{0, 5, 12})
	m := inGameOf(t, a).portraits.m
	if m.Selected != 1 {
		t.Fatalf("new game selects %d", m.Selected)
	}
	step := func(want int, ins ...Input) {
		t.Helper()
		run(t, a, ins...)
		if m.Selected != want {
			t.Fatalf("selected %d, want %d", m.Selected, want)
		}
	}
	click := func(x, y int) []Input {
		return []Input{{X: x, Y: y}, {X: x, Y: y, Left: true, LeftPressed: true}, {X: x, Y: y, LeftReleased: true}}
	}
	step(3, Input{Keys: []Key{'3'}})
	step(2, click(118+61, 389+80)...) // the hit test is inclusive
	step(2, Input{Keys: []Key{'5'}})  // empty slots are disabled
	step(2, click(405, 400)...)
	step(2, Input{Keys: []Key{'2'}}) // the selected one again: character screen (M7)
	m.Players[0].Recovery = 10
	step(2, Input{Keys: []Key{'1'}}) // recovering
	m.Players[0].Recovery = 0
	step(1, Input{Keys: []Key{'1'}})
}

// Every face with portrait icons loads its 56 frames, all the same size (59x79, inside
// the 62x80 slot hotspot).
func TestPortraitFaces(t *testing.T) {
	for first := 0; first < PortraitFaces; first += party.MaxMembers {
		var faces []int
		for f := first; f < min(first+party.MaxMembers, PortraitFaces); f++ {
			faces = append(faces, f)
		}
		p := inGameOf(t, newTestAppParty(t, StateInGame, faces)).portraits
		for i, fr := range p.faces {
			for t2, s := range fr {
				if s == nil || s.W != fr[0].W || s.H != fr[0].H || s.W > portraitW || s.H > portraitH {
					t.Fatalf("face %d frame %d: missing or %dx%d", faces[i], t2+1, s.W, s.H)
				}
			}
		}
	}
	if p := inGameOf(t, newTestApp(t, StateInGame)).portraits; p.dead == nil || p.erad == nil || p.shields[3] == nil {
		t.Error("DEAD/ERADCATE/IBshield")
	}
}

// Party creation hands the hero to the in-game screen as member 1.
func TestCreateHero(t *testing.T) {
	a := newTestAppParty(t, StateTitle, []int{3, 9})
	click := func(x, y int) []Input {
		return []Input{{X: x, Y: y}, {X: x, Y: y, Left: true, LeftPressed: true}, {X: x, Y: y, LeftReleased: true}}
	}
	run(t, a, Input{Keys: []Key{'N'}})
	run(t, a, Input{X: 300, Y: 20, Runes: []rune("Zoltan")})
	for range 7 {
		run(t, a, click(170, 170)...)
	}
	run(t, a, click(580, 455)...)
	g := inGameOf(t, a)
	got := g.portraits.m.Players
	if len(got) != 2 || got[0].Face != 7 || got[0].Voice != 7 || got[0].Name != "Zoltan" || got[1].Face != 9 {
		t.Fatalf("party %+v", got)
	}
	want, err := a.r.Cache.Icon("pc08-01", false)
	if err != nil || g.portraits.faces[0][0] != want {
		t.Errorf("slot 1 shows %v, want pc08-01 %v", g.portraits.faces[0][0], err)
	}
}
