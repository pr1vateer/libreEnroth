package world

import (
	"testing"

	"libre-enroth/internal/game/monsters"
	"libre-enroth/internal/game/physics"
	"libre-enroth/internal/game/ui"
)

// simulate runs n game-loop frames with no input.
func simulate(w *World, n int) {
	in := &ui.Input{}
	for range n {
		w.Update(in)
	}
}

// 600 frames (10 seconds) of out01 and d05 with seed 1 run without a panic, and every
// actor stays on the map: inside the outdoor bounds or in a sector with a floor, not
// sunk more than 0x10 below it (the movers settle an actor on the floor at the start
// of its next move).
//
// mm8: 0x401ad8, 0x46fb6d, 0x470945
func TestAISim(t *testing.T) {
	e := newEnv(t)
	for _, name := range []string{"out01.odm", "d05.blv"} {
		s := e.statSession(0, 5, 12)
		w := e.load(t, name, s)
		simulate(w, 600)
		moved := 0
		for i, a := range w.Actors() {
			if a.AIState == monsters.Disabled || a.AIState == monsters.Removed {
				continue
			}
			if a.Pos != a.Start {
				moved++
			}
			x, y, z := int32(a.Pos[0]), int32(a.Pos[1]), int32(a.Pos[2])
			var fz int32
			if w.outdoor != nil {
				fz, _, _, _ = w.outdoor.geo.FloorZ(x, y, z, false, false)
				if x > 0x5800 || x < -0x5800 || y > 0x5800 || y < -0x5800 {
					t.Errorf("%s actor %d off the map at %v", name, i, a.Pos)
				}
			} else {
				sec := int(a.Sector)
				fz, _ = w.indoor.geo.FloorZSector(x, y, z, &sec)
			}
			if fz == physics.NoFloor || z < fz-0x10 {
				t.Errorf("%s actor %d at %v: floor %d", name, i, a.Pos, fz)
			}
		}
		if moved == 0 {
			t.Errorf("%s: nobody moved", name)
		}
	}
}

// Dropped among d05's couatls the party is attacked: within 300 frames a monster is in
// melee and the members' HP drop; resting is refused for the hostile monsters near.
// Out01's start has pirates within 0x1400 (Actors_HostileNear counts any relation).
//
// mm8: 0x401ad8, 0x4373e5, 0x438625, 0x42e9d7
func TestAIMelee(t *testing.T) {
	e := newEnv(t)
	s := e.statSession(0, 5, 12)
	e.global(t, s)
	w := e.load(t, "d05.blv", s)
	w.group.Teleport(-2900, -1300, 945, 0)
	w.dropParty()
	var hp0 int32
	for _, p := range s.Party.Players {
		hp0 += p.HP
	}
	melee := false
	for range 300 {
		simulate(w, 1)
		for _, i := range w.ai.List() {
			if w.Actors()[i].AIState == monsters.Melee {
				melee = true
			}
		}
	}
	var hp1 int32
	for _, p := range s.Party.Players {
		hp1 += p.HP
	}
	if !melee || hp1 >= hp0 {
		t.Errorf("melee %v, HP %d -> %d", melee, hp0, hp1)
	}
	if !w.MonstersNear() || w.RestRefusal() != txtRestHostile {
		t.Errorf("rest among the couatls: %d", w.RestRefusal())
	}
	w = e.load(t, "out01.odm", e.statSession(0))
	if !w.MonstersNear() {
		t.Error("out01's start: the pirates")
	}
}

// F7 freezes the monsters: nothing moves, no action time passes.
func TestAIFrozen(t *testing.T) {
	e := newEnv(t)
	w := e.load(t, "out01.odm", e.statSession(0))
	before := append([]monsters.Actor(nil), w.Actors()...)
	w.AIFrozen = true
	simulate(w, 100)
	for i, a := range w.Actors() {
		if a.Pos != before[i].Pos || a.ActionTime != before[i].ActionTime {
			t.Fatalf("actor %d moved while frozen", i)
		}
	}
}
