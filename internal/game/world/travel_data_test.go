package world

import (
	"strings"
	"testing"

	"libre-enroth/internal/assets/tables"
	"libre-enroth/internal/game/clock"
	"libre-enroth/internal/game/dialog"
	"libre-enroth/internal/game/ui"
)

// TestTravelTables: the routes, house exits, edges and dungeon list of MM8-Rel.exe:
// 25 routes; stables 54..62 and boats 63..69 have routes; out02's north leads to out03
// in 5 days, arriving at South Start; out01 (an island) has no neighbours.
//
// mm8: 0x502d98, 0x502f77, 0x4f73dc, 0x4fe95b, 0x4f90dc
func TestTravelTables(t *testing.T) {
	e := newEnv(t)
	tr := e.tables.Game.Travel
	if len(tr.Routes) != tables.NumRoutes || tr.Routes[0].Map != 3 || tr.Routes[0].TravelDays != 2 ||
		tr.Routes[0].X != 3894 || tr.Routes[22].QBit != 223 {
		t.Errorf("routes %+v", tr.Routes[:1])
	}
	for h := 54; h <= 69; h++ {
		typ := e.tables.Game.Houses[h].Type
		if typ != dialog.TypeStables && typ != dialog.TypeBoats {
			t.Errorf("house %d type %#x", h, typ)
		}
	}
	if r := tr.HouseRoute(54, 0x6c); r != 15 {
		t.Errorf("house 54's fourth route %d", r)
	}
	if r := tr.HouseRoute(59, 0x69); r != tables.NoRoute {
		t.Errorf("house 59 route %d", r)
	}
	if a := tr.ExitArrivals[0]; a != (tables.Arrival{X: -6575, Y: 13740, Z: 177, Dir: 1536}) {
		t.Errorf("exit arrival 1 %+v", a)
	}
	if n, d, a, ok := dialog.EdgeNeighbour(tr, "out02.odm", 0, 0x5801); !ok || n != "out03.odm" || d != 5 || a != 2 {
		t.Errorf("out02 north: %q %d %d %v", n, d, a, ok)
	}
	for _, p := range [][2]int32{{0x5801, 0}, {-0x5801, 0}, {0, 0x5801}, {0, -0x5801}} {
		if _, _, _, ok := dialog.EdgeNeighbour(tr, "out01.odm", p[0], p[1]); ok {
			t.Errorf("out01 has a neighbour at %v", p)
		}
	}
	if len(tr.Dungeons) != tables.NumDungeons || tr.Dungeon("d21.BLV") != 0 || tr.Dungeon("NONE") != 6 {
		t.Errorf("dungeons %q", tr.Dungeons)
	}
}

// TestEdgeOut02North: walking north off Ravenshore stops past the edge with the
// map-edge dialogue ("It will take 5 days to travel to Alvar."); Close holds the party
// back at 0x5800, OK spends 5 days and 5 food and lands on Alvar's South Start.
//
// mm8: 0x46bb13 (World_TickOutdoor), 0x442bee, 0x42f877 (msgs 0x5a, 0x5b)
func TestEdgeOut02North(t *testing.T) {
	e := newEnv(t)
	s := quietSession()
	e.global(t, s)
	w := e.load(t, "out02.odm", s)
	walk := func() {
		w.group.Teleport(0, 0x5700, 2000, 512)
		w.dropParty()
		for i := 0; i < 300 && w.TransitionDialog() == nil; i++ {
			w.Update(&ui.Input{Held: []ui.Key{ui.KeyUp}})
		}
	}
	walk()
	tr := w.TransitionDialog()
	if tr == nil || tr.Title != "Alvar" || tr.Icon != "outside" ||
		tr.Text != "It will take 5 days to travel to Alvar.\n \nDo you wish to leave Ravenshore?" {
		t.Fatalf("dialogue %+v", tr)
	}
	if w.group.Y <= 0x5800 {
		t.Errorf("party at y %#x, not past the edge", w.group.Y)
	}
	w.AnswerTransition(false)
	if w.group.Y != 0x5800 || w.TransitionDialog() != nil {
		t.Errorf("Close: y %#x", w.group.Y)
	}
	walk()
	m := s.Party
	start, food := m.Time, m.Food
	w.AnswerTransition(true)
	name, ok := w.Travel()
	if !ok || name != "out03.odm" {
		t.Fatalf("travel %q %v", name, ok)
	}
	if m.Time-start != 5*clock.Day || m.Food != food-5 {
		t.Errorf("time +%d food %d", m.Time-start, m.Food)
	}
	out := e.load(t, name, s)
	var sx, sy int32
	for i := range out.numDecorations() {
		if d := out.decoration(i); strings.EqualFold(d.name, "South Start") {
			sx, sy = d.pos[0], d.pos[1]
		}
	}
	if p := out.PartyState(); sx == 0 && sy == 0 || p.X != sx || p.Y != sy {
		t.Errorf("party at %d,%d; South Start at %d,%d", p.X, p.Y, sx, sy)
	}
}

// TestStableToAlvar: the Ravenshore stable (house 54) on day 1 offers "2 days to
// Alvar" for 50 gold; taking it leaves the house for out03 at the route's arrival two
// days later.
//
// mm8: 0x4b78c9, 0x4b761b
func TestStableToAlvar(t *testing.T) {
	e := newEnv(t)
	s := quietSession()
	e.global(t, s)
	w := e.load(t, "out02.odm", s)
	w.SpeakInHouse(54)
	d := w.Dialog()
	if d == nil || !d.OnProprietor() {
		t.Fatalf("house 54: %+v", d)
	}
	r := d.Routes()
	if len(r) != 1 || r[0].Code != 0x69 || r[0].Days != 2 || w.MapStatsName(r[0].Route.Map) != "Alvar" || d.TravelPrice() != 50 {
		t.Fatalf("routes %+v, price %d", r, d.TravelPrice())
	}
	gold, start := s.Party.Gold, s.Party.Time
	d.Click(dialog.Button{Msg: dialog.MsgService, Param: 0x69})
	if !d.Closed || s.Party.Gold != gold-50 || s.Party.Time-start != 2*clock.Day {
		t.Fatalf("closed %v gold %d time +%d", d.Closed, s.Party.Gold, s.Party.Time-start)
	}
	name, ok := w.Travel()
	if !ok || !strings.EqualFold(name, "out03.odm") {
		t.Fatalf("travel %q", name)
	}
	out := e.load(t, name, s)
	if p := out.PartyState(); p.X != 3894 || p.Y != 13136 || p.Dir != 1536 {
		t.Errorf("party at %d,%d facing %d", p.X, p.Y, p.Dir)
	}
}

// TestServiceHouses: every house with a proprietor's menu opens at noon and its screen
// draws, for a healthy and for a weak member (the temple's Heal), on each weekday.
func TestServiceHouses(t *testing.T) {
	e := newEnv(t)
	a := e.app(t, "out02.odm")
	w := a.World().(*World)
	if w.Dialog() != nil {
		a.Update(&ui.Input{Keys: []ui.Key{ui.KeyEscape}})
	}
	n := 0
	for id := 1; id < len(e.tables.Game.Houses); id++ {
		h := &e.tables.Game.Houses[id]
		if len(dialog.ServiceMenu(int(e.tables.Game.Anim(int(h.Video)).Type), w.S.Party)) == 0 {
			continue
		}
		for day := range 7 {
			w.S.SetTimeOfDay(12, 0)
			w.S.Party.Time += clock.Time(day) * clock.Day
			w.S.Party.Calendar = w.S.Party.Time.Calendar()
			w.S.Party.Players[0].Conditions[0] = int64(day) // cursed on weekdays 1..6
			w.CloseDialog()
			w.SpeakInHouse(id)
			d := w.Dialog()
			if d == nil {
				t.Fatalf("house %d %q did not open at noon", id, h.Name)
			}
			if !d.OnProprietor() {
				d.SelectResident(0)
			}
			a.Update(&ui.Input{X: 556, Y: 200})
			Compose(a, 640, 480)
		}
		n++
	}
	w.CloseDialog()
	if n < 60 {
		t.Errorf("%d service houses", n)
	}
}
