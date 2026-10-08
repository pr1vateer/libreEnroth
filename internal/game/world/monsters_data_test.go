package world

import (
	"strings"
	"testing"

	"libre-enroth/internal/game/clock"
	"libre-enroth/internal/game/dialog"
	"libre-enroth/internal/game/monsters"
)

// Every monster's dmonlist record names sprite groups dsft.bin has (Actor_LoadSprites
// 0x45830d finds them with SFT_FindGroup).
func TestMonsterSprites(t *testing.T) {
	e := newEnv(t)
	mt := e.tables.Game.Monsters
	sft := e.tables.SFT
	for id := 1; id < len(mt.Infos); id++ {
		if mt.Infos[id].Name == "" {
			continue
		}
		ml := &mt.MonList[id-1]
		for k := range 8 {
			g := sft.FindGroup(ml.Sprites[k])
			if !strings.EqualFold(sft.Frames[g].Group, ml.Sprites[k]) {
				t.Errorf("monster %d (%s): sprite %d %q not in dsft.bin", id, ml.Name, k, ml.Sprites[k])
			}
		}
	}
}

// Loading a map: the template actors get their sprites and the spawn points their
// monsters; every actor stands in its sector (indoors) and stands (outdoors with its
// animation set: Level_Load does not call Actor_UpdateAnimation for the living).
func TestLoadActors(t *testing.T) {
	e := newEnv(t)
	for _, c := range []struct {
		name     string
		template int
	}{{"out01.odm", 62}, {"d05.blv", 0}, {"out02.odm", 143}, {"d16.blv", 6}} {
		w := e.load(t, c.name, quietSession())
		acts := w.Actors()
		if len(acts) <= c.template {
			t.Errorf("%s: %d actors, the template has %d", c.name, len(acts), c.template)
		}
		for i := range acts {
			a := &acts[i]
			if a.AIState == monsters.Disabled {
				continue
			}
			if a.Sprites[0] == 0 || a.AIState != monsters.Stand || w.outdoor != nil && a.Flags&monsters.FlagAnimationSet == 0 {
				t.Errorf("%s actor %d: sprites %v state %d flags %#x", c.name, i, a.Sprites, a.AIState, a.Flags)
			}
			if w.indoor != nil && int(a.Sector) != w.indoor.geo.SectorAt(int32(a.Pos[0]), int32(a.Pos[1]), int32(a.Pos[2])) {
				t.Errorf("%s actor %d at %v: sector %d", c.name, i, a.Pos, a.Sector)
			}
		}
	}
}

// Talking to out01's S'ton (actor 0, NPC 31) opens his dialogue; a peasant without an
// NPC and without group news says nothing.
func TestTalkToActor(t *testing.T) {
	e := newEnv(t)
	w := e.load(t, "out01.odm", quietSession())
	a := &w.Actors()[0]
	if a.NPC != 31 {
		t.Fatalf("actor 0 npc %d", a.NPC)
	}
	w.talkToActor(a)
	d := w.Dialog()
	if d == nil || d.Kind != dialog.KindNPC || d.NPC != 31 {
		t.Fatalf("dialogue %+v", d)
	}
	w.CloseDialog()
	if !w.interactActor(0) || w.Dialog() == nil {
		t.Error("Space on S'ton")
	}
}

// Picking up out01's first ground object (a pile of gold or an item) removes it; the
// item goes to a pack.
func TestPickUp(t *testing.T) {
	e := newEnv(t)
	s := quietSession()
	e.global(t, s)
	w := e.load(t, "out01.odm", s)
	objs := w.Objects()
	n := len(objs)
	o := objs[0]
	gold := s.Party.Gold
	if !w.interactItem(0) {
		t.Fatal("Space on a ground item")
	}
	if w.Objects()[0].ObjList != 0 && len(w.Objects()) == n {
		t.Error("the object stays")
	}
	if it := o.Item; e.tables.Game.Items.Item(it.Number).EquipType == 18 {
		if s.Party.Gold != gold+it.Special {
			t.Errorf("gold %d, want %d", s.Party.Gold, gold+it.Special)
		}
	} else if !s.Party.HasItem(0, it.Number) {
		t.Errorf("item %d not taken", it.Number)
	}
}

// A map comes back as left within its refill days; after them the template returns with
// new spawns.
//
// mm8: 0x47df28 (Odm_Load: day - lastVisit >= refill days)
func TestRespawn(t *testing.T) {
	e := newEnv(t)
	s := quietSession()
	w := e.load(t, "out01.odm", s)
	n := len(w.Actors())
	w.Actors()[0].HP = 1
	e.load(t, "d05.blv", s)
	w = e.load(t, "out01.odm", s)
	if len(w.Actors()) != n || w.Actors()[0].HP != 1 {
		t.Fatalf("out01 changed on a second visit: %d actors, hp %d", len(w.Actors()), w.Actors()[0].HP)
	}
	refill := w.mapStats().RefillDays
	s.Party.Time += clock.Time(refill) * clock.Day
	w = e.load(t, "out01.odm", s)
	if w.Actors()[0].HP == 1 || len(w.Actors()) < 62 {
		t.Errorf("out01 after %d days: %d actors, hp %d", refill, len(w.Actors()), w.Actors()[0].HP)
	}
}

// On the first visit out01's 16 interactive decorations (barrels, cauldrons...) get map
// variables 0x4b.. with an event each (one that rolls event 0x10c encodes as 0 and is
// hidden at once); a decoration whose variable is emptied hides on the next load.
//
// mm8: 0x44f66c, 0x48041d
func TestInteractiveDecorations(t *testing.T) {
	e := newEnv(t)
	s := quietSession()
	w := e.load(t, "out01.odm", s)
	mv := w.MapVars()
	n := 0
	first := -1
	for i := range w.numDecorations() {
		d := w.decoration(i)
		if d.event != 0 || !interactive(*d.idx) {
			continue
		}
		// Event 0x10c encodes as 0, which the load loop takes for used up: hidden.
		if int(d.evar) != decVarBase+n || mv[d.evar] == 0 && *d.flags&decHidden == 0 {
			t.Errorf("decoration %d: var %#x = %d, flags %#x", i, d.evar, mv[d.evar], *d.flags)
		}
		if first < 0 && mv[d.evar] != 0 {
			first = i
		}
		n++
	}
	if n != 16 {
		t.Fatalf("%d interactive decorations", n)
	}
	mv[w.decoration(first).evar] = 0
	e.load(t, "d05.blv", s)
	w = e.load(t, "out01.odm", s)
	if *w.decoration(first).flags&decHidden == 0 {
		t.Error("an emptied decoration still shows")
	}
}
