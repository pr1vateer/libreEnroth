package monsters

import "libre-enroth/internal/game/items"

// What the event scripts do to actors (re/notes/monsters.md, "Events"): flags, groups,
// allies, the items they carry, and counting the dead.

// More actor flags.
const (
	FlagSchedule = 0x400000 // follows its NPC schedule (Actor_UpdateSchedule)
)

// SetActorFlag sets or clears flag bits of actor i; clearing exactly the disabled bit
// stands a disabled actor up.
//
// mm8: 0x446e7d (Evt_ToggleActorFlag)
func SetActorFlag(actors []Actor, i int, bits uint32, on bool) {
	if i < 0 || i >= len(actors) {
		return
	}
	a := &actors[i]
	if on {
		a.Flags |= bits
		return
	}
	if bits == FlagDisabled && a.AIState == Disabled {
		a.AIState = Stand
	}
	a.Flags &^= bits
}

// SetGroupFlag sets or clears flag bits of every actor of group (0: none). Setting
// exactly the disabled bit disables them; clearing it stands up those still alive.
//
// mm8: 0x446edd (Evt_ToggleActorGroupFlag)
func SetGroupFlag(actors []Actor, group int32, bits uint32, on bool, s Sprites) {
	if group == 0 {
		return
	}
	for i := range actors {
		a := &actors[i]
		if a.Group != group {
			continue
		}
		if on {
			a.Flags |= bits
			if bits == FlagDisabled {
				a.AIState = Disabled
				a.UpdateAnimation(s)
			}
			continue
		}
		if bits == FlagDisabled && a.AIState != Dead && a.AIState != Dying && a.AIState != Removed && a.HP > 0 {
			a.AIState = Stand
		}
		a.Flags &^= bits
	}
}

// SetActorGroup moves actor i to group g.
//
// mm8: 0x4446bd (case 0x30; the original does not check the index)
func SetActorGroup(actors []Actor, i int, g int32) {
	if i >= 0 && i < len(actors) {
		actors[i].Group = g
	}
}

// ChangeGroup moves the actors of group from to group to.
//
// mm8: 0x446bd6 (Evt_ChangeGroup)
func ChangeGroup(actors []Actor, from, to int32) {
	for i := range actors {
		if actors[i].Group == from {
			actors[i].Group = to
		}
	}
}

// ChangeGroupAlly gives the actors of group the hostile.txt class ally.
//
// mm8: 0x446bf6 (Evt_ChangeGroupAlly)
func ChangeGroupAlly(actors []Actor, group, ally int32) {
	for i := range actors {
		if actors[i].Group == group {
			actors[i].Ally = ally
		}
	}
}

// setItem gives or takes item id: given, into the carried item, else the first free
// event slot (none: nothing); taken, from the carried item, else the first event slot
// holding it.
func (a *Actor) setItem(id int32, on bool) {
	ev := &a.Items
	if on {
		switch {
		case a.CarriedItem == 0:
			a.CarriedItem = uint16(id)
		case ev[ItemEvent0].Number == 0:
			ev[ItemEvent0].Number = id
		case ev[ItemEvent1].Number == 0:
			ev[ItemEvent1].Number = id
		}
		return
	}
	switch {
	case int32(int16(a.CarriedItem)) == id:
		a.CarriedItem = 0
	case ev[ItemEvent0].Number == id:
		ev[ItemEvent0] = items.Item{}
	case ev[ItemEvent1].Number == id:
		ev[ItemEvent1] = items.Item{}
	}
}

// SetActorItem gives actor i item id or takes it away.
//
// mm8: 0x44690e (Evt_SetActorItem)
func SetActorItem(actors []Actor, i int, id int32, on bool) {
	if i >= 0 && i < len(actors) {
		actors[i].setItem(id, on)
	}
}

// NPCSetItem gives the first actor that is NPC npc item id, or takes it away.
//
// mm8: 0x446854 (Evt_NPCSetItem)
func NPCSetItem(actors []Actor, npc int32, id int32, on bool) {
	for i := range actors {
		if int32(actors[i].NPC) == npc {
			actors[i].setItem(id, on)
			return
		}
	}
}

// Kill check modes (Evt_IsActorKilled).
const (
	KilledAll     = 0 // every actor
	KilledGroup   = 1 // the actors of a group
	KilledMonster = 2 // the actors of monsters.txt id param + 1
	KilledActor   = 3 // actor param
	KilledUnique  = 4 // the actors with placemon.txt name param
)

// Killed counts the actors a kill check looks at and how many of them are gone (dead,
// dying, removed, summoned or stoned, and with disabledToo disabled). Only actors whose
// alert-only flag bit equals the map's alert value count: the original compares the
// 0x100000 bit with Map_IsAlert's 0 or 1, so on an alert map none do.
//
// mm8: 0x4094d8, 0x4095a8, 0x40953a, 0x40968c, 0x40961a
func Killed(actors []Actor, alert uint32, mode int, param int32, disabledToo bool) (gone, total int) {
	count := func(a *Actor) {
		if a.Flags&FlagAlertOnly != alert {
			return
		}
		total++
		if a.Gone(disabledToo) {
			gone++
		}
	}
	if mode == KilledActor {
		if param >= 0 && int(param) < len(actors) {
			count(&actors[param])
		}
		return gone, total
	}
	for i := range actors {
		a := &actors[i]
		switch mode {
		case KilledGroup:
			if a.Group != param {
				continue
			}
		case KilledMonster:
			if int32(int16(a.Info.ID))-1 != param {
				continue
			}
		case KilledUnique:
			if a.UniqueName != param {
				continue
			}
		}
		count(a)
	}
	return gone, total
}

// IsKilled is the kill check: with n 0 whether every actor it counts is gone (also
// when it counts none), else whether at least n are. Unknown modes are false.
//
// mm8: 0x444473 (Evt_IsActorKilled)
func IsKilled(actors []Actor, alert uint32, mode int, param int32, n int, disabledToo bool) bool {
	if mode < KilledAll || mode > KilledUnique {
		return false
	}
	gone, total := Killed(actors, alert, mode, param, disabledToo)
	if n == 0 {
		return gone == total
	}
	return n <= gone
}

// ResetAfterRest puts the actors back for a new day: those able to act (and the
// disabled) return to their start with full HP, and those able to act stand facing
// their last target for their recovery time. Every actor calms down, a peasant forgets
// it was hostile, a corpse's loot is rolled again on its next search, and those on a
// schedule go to the job of the hour.
//
// mm8: 0x40928c (Actors_ResetAfterRest)
func (b *Brain) ResetAfterRest(hour int) {
	for i := range b.Actors {
		a := &b.Actors[i]
		if a.CanAct() || a.AIState == Disabled {
			a.Pos = a.Start
			a.HP = int16(a.Info.HP)
			if a.AIState != Disabled {
				b.Stand(i, b.targets[i], a.Info.Recovery, nil)
			}
		}
		a.Info.Hostility = 0
		if a.IsPeasant() {
			a.Flags &^= FlagHostile
		}
		a.Flags &^= FlagLooted
		if a.Flags&FlagSchedule != 0 {
			b.UpdateSchedule(i, hour, true)
		}
	}
}

// Job attributes (Job.Attrib).
const jobActive = 0x1

// UpdateSchedule moves actor i's start (where it wanders and returns to) and its Move
// to the last active job whose hour has come; with snap the actor is put there.
//
// Deviation (re/notes/monsters.md): the original indexes the jobs with the actor's
// index added twice, so every actor but the first reads past its 8 jobs (and, with no
// job due, 0xffff jobs on); libre-enroth reads the actor's own jobs and leaves an actor
// with none due alone. No shipped actor has a schedule.
//
// mm8: 0x403446 (Actor_UpdateSchedule)
func (b *Brain) UpdateSchedule(i, hour int, snap bool) {
	a := &b.Actors[i]
	if !a.CanAct() {
		return
	}
	k := len(a.Jobs) - 1
	for ; k >= 0; k-- {
		if j := &a.Jobs[k]; j.Attrib&jobActive != 0 && int(j.Hour) <= hour {
			break
		}
	}
	if k < 0 {
		return
	}
	j := &a.Jobs[k]
	at := [3]int16{j.X, j.Y, j.Z}
	if a.Start == at && a.Info.Move == j.Action {
		return
	}
	a.Start, a.Info.Move = at, j.Action
	if snap {
		a.Pos = a.Start
	}
}
