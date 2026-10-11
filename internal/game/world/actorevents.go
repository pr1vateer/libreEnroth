package world

import (
	"libre-enroth/internal/evt"
	"libre-enroth/internal/game/monsters"
)

// The actor opcodes of the event scripts (evt.ActorHost).

// SummonMonsters implements evt.Host: the map's MapStats slot spawns at the point (no
// MapStats row: nothing); the new monsters face the party and carry the record's
// placemon.txt name.
//
// mm8: 0x447193 (Evt_SummonMonsters)
func (w *World) SummonMonsters(s evt.Summon) {
	dl, row := w.delta(), w.mapStats()
	if dl == nil || row == nil {
		return
	}
	sp := monsters.SpawnPoint{X: s.X, Y: s.Y, Z: s.Z, Radius: 0x20, Kind: monsters.SpawnMonsters,
		Index: uint16(s.Type + s.Level*3), Group: uint32(s.Group)}
	first := len(dl.Actors)
	var err error
	if dl.Actors, err = monsters.Spawn(w.monsterEnv(), dl.Actors, row, &sp, 0, s.Count, false); err != nil {
		w.S.logf("%s: SummonMonsters: %v", w.name, err)
	}
	if len(dl.Actors) == first {
		return
	}
	b := w.brain()
	dir := b.Direction(monsters.PID(first), monsters.KindParty, 1)
	for i := first; i < len(dl.Actors); i++ {
		a := &dl.Actors[i]
		a.LoadSprites(w.monsterTables(), w.tables.SFT, false)
		a.Yaw = uint16(dir.Yaw)
		a.UniqueName = s.UniqueName
	}
}

// SetActorFlag implements evt.Host.
func (w *World) SetActorFlag(actor int, bits uint32, on bool) {
	monsters.SetActorFlag(w.Actors(), actor, bits, on)
}

// SetGroupFlag implements evt.Host.
func (w *World) SetGroupFlag(group int32, bits uint32, on bool) {
	monsters.SetGroupFlag(w.Actors(), group, bits, on, spriteSet{w})
}

// SetActorGroup implements evt.Host.
func (w *World) SetActorGroup(actor int, group int32) {
	monsters.SetActorGroup(w.Actors(), actor, group)
}

// ChangeGroup implements evt.Host.
func (w *World) ChangeGroup(from, to int32) { monsters.ChangeGroup(w.Actors(), from, to) }

// ChangeGroupAlly implements evt.Host.
func (w *World) ChangeGroupAlly(group, ally int32) { monsters.ChangeGroupAlly(w.Actors(), group, ally) }

// SetActorItem implements evt.Host.
func (w *World) SetActorItem(actor int, item int32, on bool) {
	monsters.SetActorItem(w.Actors(), actor, item, on)
}

// NPCSetItem implements evt.Host.
func (w *World) NPCSetItem(npc, item int32, on bool) { monsters.NPCSetItem(w.Actors(), npc, item, on) }

// ActorsKilled implements evt.Host.
//
// mm8: 0x444473 (Evt_IsActorKilled)
func (w *World) ActorsKilled(mode int, param int32, n int, disabledToo bool) bool {
	return monsters.IsKilled(w.Actors(), w.alert(), mode, param, n, disabledToo)
}
