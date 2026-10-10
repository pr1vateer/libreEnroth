package world

import (
	"libre-enroth/internal/game/monsters"
	"libre-enroth/internal/game/party"
	"libre-enroth/internal/game/physics"
)

// The monsters' turn of a frame (re/notes/monsters.md, "AI"): Game_Loop runs
// Actors_UpdateAI before World_Tick, which moves the party, then the actors; the melee
// blows land after it. Turn-based mode (M9) and the debug freeze (F7) stop them.

// brain is the map's AI, made on first use and refreshed with the frame's actors,
// objects and party; nil without actors or tables.
func (w *World) brain() *monsters.Brain {
	dl, mt := w.delta(), w.monsterTables()
	if dl == nil || mt == nil {
		return nil
	}
	if w.ai == nil {
		w.ai = &monsters.Brain{Tables: mt, Spells: w.tables.Game.Spells, SFT: w.tables.SFT, Sprites: spriteSet{w}, Host: aiHost{w}}
	}
	b := w.ai
	if w.indoor != nil {
		b.Indoor = w.indoor.geo
	} else {
		b.Outdoor = w.outdoor.geo
	}
	b.Rand = w.S.Ctx.Rand
	b.Actors, b.Objects = dl.Actors, dl.Objects
	p, m := w.group, w.S.Party
	b.Party = monsters.Party{X: p.X, Y: p.Y, Z: p.Z, Height: p.Height, EyeLevel: p.EyeLevel, Radius: p.Radius,
		Dir: p.Dir, Flying: p.Flying, TurnBased: m.TurnBased, Members: m, Ctx: w.S.Ctx}
	return b
}

// aiHost notes what the AI leaves to later milestones.
type aiHost struct{ w *World }

func (h aiHost) Stub(key, what string) { h.w.S.note(key, what) }

// monstersActive reports whether the monsters take their turn this frame.
func (w *World) monstersActive() bool { return !w.AIFrozen && !w.S.Party.TurnBased }

// thinkMonsters runs Actors_UpdateAI with g_miscTimer's step (1..32 ticks).
//
// mm8: 0x46261d (Game_Loop: Actors_UpdateAI 0x401ad8 before World_Tick)
func (w *World) thinkMonsters(ticks int32) {
	if !w.monstersActive() {
		return
	}
	if b := w.brain(); b != nil {
		b.Tick(max(1, min(ticks, 0x20)))
	}
}

// moveMonsters runs the actor movers after the party's move.
//
// mm8: 0x46bafa (World_TickIndoor: 0x46fb6d), 0x46bb13 (World_TickOutdoor: 0x470945)
func (w *World) moveMonsters(ticks int32) {
	if !w.monstersActive() {
		return
	}
	b := w.brain()
	if b == nil {
		return
	}
	if w.indoor != nil {
		b.MoveIndoor(ticks)
	} else {
		b.MoveOutdoor(ticks)
	}
}

// strikeMonsters lands the frame's melee blows.
//
// mm8: 0x46bac9 (World_Tick: AttackList_Process 0x4373e5 last)
func (w *World) strikeMonsters() {
	if !w.monstersActive() {
		return
	}
	if b := w.brain(); b != nil {
		b.Strike()
	}
}

// actorObstacles are the map's actors as the party's sweeps meet them.
type actorObstacles struct{ w *World }

func (o actorObstacles) Collide(s *physics.State) { monsters.CollideActors(s, o.w.Actors()) }

// Bumped ends the party's invisibility.
//
// mm8: 0x472e86, 0x473fee (pid & 7 == 3: SpellBuff_Clear(party buff 11))
func (o actorObstacles) Bumped() {
	o.w.S.Party.Buffs[party.PartyBuffInvisibility] = party.Buff{}
}

// MonstersNear reports a monster that keeps the party from resting: one alive within
// 0x1400 (0xa00 indoors) that is hostile to the party.
//
// mm8: 0x42e9d7 (Actors_HostileNear)
func (w *World) MonstersNear() bool {
	mt := w.monsterTables()
	if mt == nil {
		return false
	}
	p := w.group
	return monsters.HostileNear(mt, w.Actors(), p.X, p.Y, p.Z, w.indoor != nil)
}
