package world

import (
	"strings"
	"testing"

	"libre-enroth/internal/evt"
	"libre-enroth/internal/game/dialog"
	"libre-enroth/internal/game/items"
	"libre-enroth/internal/game/monsters"
)

// Every SummonMonsters record of the shipped maps names a MapStats slot whose monsters
// exist: summoning them logs no "can't create" error, and the new monsters face the
// party with the record's name.
//
// mm8: 0x447193 (Evt_SummonMonsters), 0x44dfbe
func TestSummonRecords(t *testing.T) {
	e := newEnv(t)
	n := 0
	for _, en := range e.d.Games.Entries {
		name := strings.ToLower(en.Name)
		if !strings.HasSuffix(name, ".odm") && !strings.HasSuffix(name, ".blv") {
			continue
		}
		s := quietSession()
		var logged []string
		s.Log = func(f string, args ...any) {
			if strings.Contains(f, "SummonMonsters") {
				logged = append(logged, f)
			}
		}
		w := e.load(t, name, s)
		if w.vm.Map == nil {
			continue
		}
		for _, r := range w.vm.Map.Records {
			if r.Op() != evt.OpSummonMonsters {
				continue
			}
			n++
			before := len(w.Actors())
			w.SummonMonsters(evt.Summon{Type: r.U8(5), Level: r.U8(6), Count: r.U8(7), X: r.I32(8), Y: r.I32(0xc),
				Z: r.I32(0x10), Group: r.I32(0x14), UniqueName: r.I32(0x18)})
			for i := before; i < len(w.Actors()); i++ {
				if a := &w.Actors()[i]; a.Sprites[0] == 0 || a.Group != r.I32(0x14) {
					t.Errorf("%s event %d: actor %d sprites %v group %d", name, r.ID(), i, a.Sprites, a.Group)
				}
			}
		}
		if len(logged) != 0 {
			t.Errorf("%s: %q", name, logged)
		}
	}
	if n != 51 {
		t.Errorf("%d SummonMonsters records", n)
	}
}

// F6 kills: the dying animation runs out into a corpse; searching it adds the gold its
// dice rolled and removes it (or, one time in ten after a find, leaves it); a corpse
// without loot leaves at once.
//
// mm8: 0x402ec9, 0x4256b6
func TestCorpse(t *testing.T) {
	e := newEnv(t)
	s := e.statSession(0)
	e.global(t, s)
	w := e.load(t, "out01.odm", s)
	i := -1
	for k, a := range w.Actors() {
		if a.AIState != monsters.Disabled && a.Info.GoldDice != 0 && a.CarriedItem == 0 {
			i = k
			break
		}
	}
	if i < 0 {
		t.Fatal("no monster with gold")
	}
	w.brain().Die(i)
	if w.Actors()[i].AIState != monsters.Dying {
		t.Fatalf("state %d", w.Actors()[i].AIState)
	}
	simulate(w, 120)
	a := &w.Actors()[i]
	if a.AIState != monsters.Dead {
		t.Fatalf("after the dying: state %d", a.AIState)
	}
	gold := s.Party.Gold
	w.interactActor(i)
	if s.Party.Gold <= gold {
		t.Errorf("gold %d -> %d", gold, s.Party.Gold)
	}
	if a.AIState != monsters.Removed && a.AIState != monsters.Dead {
		t.Errorf("searched: state %d", a.AIState)
	}
}

// Throwing: with every pack full the mouse item is thrown before the party (a temporary
// object at its eyes, pitch 0xb8, speed 200); it lands and comes to rest, can be
// picked up again, and is gone when the map loads again.
//
// mm8: 0x4211c8, 0x42eaea, 0x472047, 0x40933c
func TestThrowItem(t *testing.T) {
	e := newEnv(t)
	s := e.statSession(0)
	e.global(t, s)
	w := e.load(t, "out01.odm", s)
	m := s.Party
	n := len(w.Objects())
	m.MouseItem = items.Item{Number: 1, Flags: items.FlagIdentified}
	w.throwItem(m.MouseItem, 0)
	m.MouseItem = items.Item{}
	objs := w.Objects()
	if len(objs) != n+1 && w.Objects()[0].ObjList != 0 {
		t.Fatalf("%d objects, %d before", len(objs), n)
	}
	k := -1
	for j := range objs {
		if objs[j].Attrib&monsters.ObjTemporary != 0 && objs[j].Item.Number == 1 {
			k = j
		}
	}
	if k < 0 {
		t.Fatal("no thrown object")
	}
	o := &w.Objects()[k]
	if o.Vel[2] <= 0 || o.Pos[2] != w.group.Z+w.group.EyeLevel || o.Caster != monsters.KindParty {
		t.Errorf("thrown: %+v", o)
	}
	simulate(w, 300)
	o = &w.Objects()[k]
	fz, _, _, _ := w.outdoor.geo.FloorZ(o.Pos[0], o.Pos[1], o.Pos[2], false, false)
	if o.Vel != [3]int16{} || o.Pos[2] < fz || o.Pos[2] > fz+2 {
		t.Errorf("at rest: pos %v vel %v floor %d", o.Pos, o.Vel, fz)
	}
	if !w.pickUp(k, false) || !m.HasItem(0, 1) || w.Objects()[k].ObjList != 0 {
		t.Error("picked up")
	}
	w.throwItem(items.Item{Number: 1}, 0)
	e.load(t, "d05.blv", s)
	w = e.load(t, "out01.odm", s)
	for _, o := range w.Objects() {
		if o.ObjList != 0 && o.Attrib&monsters.ObjTemporary != 0 {
			t.Errorf("a thrown object survived the load: %+v", o)
		}
	}
}

// The bounty topic of Bryant Conlan (house 272) names a monster; once one of its kind
// dies the next visit pays level * 100 gold and the award.
//
// mm8: 0x4bc140, 0x402ec9
func TestBountyHunt(t *testing.T) {
	e := newEnv(t)
	s := e.statSession(0)
	e.global(t, s)
	w := e.load(t, "out01.odm", s)
	w.SpeakInHouse(272)
	d := w.Dialog()
	if d == nil {
		t.Fatal("no house")
	}
	d.SelectResident(0)
	topic := dialog.Button{Msg: dialog.MsgHouseTopic, Param: dialog.ParamTopic0}
	d.Click(topic)
	h := &s.Party.Hunts[0]
	if h.Monster < 4 || h.Monster > 0xc3 || !strings.Contains(d.Reply, "bounty") {
		t.Fatalf("hunt %+v reply %q", h, d.Reply)
	}
	in := w.monsterTables().Info(int(h.Monster))
	a := w.Actors()[0]
	a.Info = *in
	w.S.Party.MarkKilled(int(a.Info.ID))
	gold := s.Party.Gold
	d.Click(topic)
	want := int32(in.Level) * 100
	if s.Party.Gold != gold+want || s.Party.Bounty != want || !s.Party.Players[0].Awards.Get(0x2c) {
		t.Errorf("paid %d (bounty %d), want %d", s.Party.Gold-gold, s.Party.Bounty, want)
	}
	if !strings.Contains(d.Reply, in.Name) {
		t.Errorf("reply %q", d.Reply)
	}
}

// An inn's night longer than 4 hours resets out02's monsters: back home, full HP.
//
// mm8: 0x40928c (from msg 0x199)
func TestInnResetsActors(t *testing.T) {
	e := newEnv(t)
	w := e.load(t, "out02.odm", quietSession())
	i := -1
	for k, a := range w.Actors() {
		if a.CanAct() {
			i = k
			break
		}
	}
	a := &w.Actors()[i]
	a.Pos[0] += 300
	a.HP = 1
	w.ResetActors()
	if a.Pos != a.Start || int32(a.HP) != a.Info.HP || a.AIState != monsters.Stand {
		t.Errorf("actor %d: pos %v start %v hp %d state %d", i, a.Pos, a.Start, a.HP, a.AIState)
	}
}

// A rest encounter brings the map's monster type to a clear point near the party,
// hostile.
//
// mm8: 0x44f0da
func TestRestEncounter(t *testing.T) {
	e := newEnv(t)
	for _, name := range []string{"out01.odm", "d05.blv"} {
		w := e.load(t, name, quietSession())
		n := len(w.Actors())
		if !w.spawnEncounter(w.mapStats(), 1) {
			if name == "d05.blv" {
				continue // nothing able to act in d05's template: no encounter
			}
			t.Fatalf("%s: no encounter", name)
		}
		if len(w.Actors()) <= n {
			t.Fatalf("%s: nobody came", name)
		}
		p := w.group
		for _, a := range w.Actors()[n:] {
			d := approxDist(abs32(int32(a.Pos[0])-p.X), abs32(int32(a.Pos[1])-p.Y), 0)
			if a.Flags&monsters.FlagHostile == 0 || d > 0x700 {
				t.Errorf("%s: %+v at %d", name, a.Pos, d)
			}
		}
	}
}
