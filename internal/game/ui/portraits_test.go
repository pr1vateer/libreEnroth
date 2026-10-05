package ui

import (
	"testing"

	"libre-enroth/internal/game/party"
	"libre-enroth/internal/gfx"
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
	finishCreate(t, a)
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

// The HP bar shows the bottom share of its picture: the rows above 0x1ae + ftol((1 -
// share) * h) keep the panel, the colour follows the share, no bar at 0 HP.
//
// mm8: 0x41b67a (Portraits_DrawBars)
func TestHPBar(t *testing.T) {
	a := newTestAppParty(t, StateInGame, []int{0})
	p := inGameOf(t, a).portraits
	m := p.m
	e := m.Env(p.ctx)
	max := m.Players[0].MaxHP(e)
	draw := func(hp int32) *gfx.Canvas {
		m.Players[0].HP = hp
		c := gfx.NewCanvas()
		p.draw(c)
		return c
	}
	none := draw(0)
	x := int(p.hpBarX[0]) + 1
	h := p.barG.H
	for _, c := range []struct {
		hp  int32
		bar *gfx.Sprite
	}{{max, p.barG}, {max * 3 / 4, p.barG}, {max / 2, p.barY}, {max / 4, p.barR}, {1, p.barR}} {
		img := draw(c.hp)
		top := barY + int((1-float64(c.hp)/float64(max))*float64(h))
		for y := barY; y < barY+h; y++ {
			same := true
			for dx := 0; dx < c.bar.W; dx++ {
				if img.Img.RGBAAt(x+dx, y) != none.Img.RGBAAt(x+dx, y) {
					same = false
				}
			}
			if y < top && !same || y >= top && same && y < barY+c.bar.H {
				t.Errorf("HP %d/%d: row %d (top %d) same %v", c.hp, max, y, top, same)
				break
			}
		}
		b := c.bar.Pix[(c.bar.H-1)*c.bar.W*4:]
		if got := img.Img.RGBAAt(x, barY+c.bar.H-1); got.R != b[0] || got.G != b[1] || got.B != b[2] {
			t.Errorf("HP %d/%d: bottom pixel %v, want the bar's %v", c.hp, max, got, b[:3])
		}
	}
}
