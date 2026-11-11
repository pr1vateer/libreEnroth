package world

import (
	"libre-enroth/internal/assets/tables"
	"libre-enroth/internal/game/clock"
	"libre-enroth/internal/game/monsters"
	"libre-enroth/internal/game/party"
)

// The party's attacks (re/notes/combat.md). A presses the attack key (binding 8);
// a click on a hostile actor posts the same msg 0x17.

// attack is Party_Attack for the selected member. A dragon's breath and a wand are
// projectiles, and a bow is too once the target is out of a blow's reach. Anything
// else is a blow, or a swing at the air that only recovers.
//
// mm8: 0x42e27f
func (w *World) attack(mx, my int) {
	m := w.S.Party
	if m == nil || m.Selected < 1 || m.Selected > len(m.Players) || w.S.Ctx == nil {
		return
	}
	member := m.Selected - 1
	p := &m.Players[member]
	if !p.CanAct() {
		return
	}
	m.Buffs[party.PartyBuffInvisibility] = party.Buff{}
	w.casts.CancelPending()
	e := m.Env(w.S.Ctx)
	if p.Class == 14 || p.Class == 15 {
		w.enqueueCast(0x89, member)
		m.Speak(member, 0x31, w.S.Ctx) // sound 0x46a0 (M11)
		return
	}
	if it := p.Equipped(party.SlotMainHand); it != nil && w.tables != nil && w.tables.Game != nil &&
		it.Def(w.tables.Game.Items).EquipType == tables.EquipWand {
		if it.Charges < 1 {
			it.Number = 0
			p.Equip[party.SlotMainHand] = 0
			return
		}
		spell := int32(0)
		if n := int(it.Number); n >= 0 && n < len(w.tables.Game.Wands) {
			spell = w.tables.Game.Wands[n]
		}
		w.enqueueCast(int(spell), member)
		it.Charges--
		if it.Charges < 1 {
			it.Number = 0
			p.Equip[party.SlotMainHand] = 0
		}
		return
	}
	ti := w.attackTarget(mx, my)
	inRange := false
	if ti >= 0 {
		a := &w.Actors()[ti]
		inRange = float64(monsters.MeleeDistance(w.group.X, w.group.Y, w.group.Z, a)) <= monsters.MeleeReach
	}
	if !inRange && p.Equipped(party.SlotBow) != nil {
		w.enqueueCast(0x85, member)
		m.Speak(member, 0x32, w.S.Ctx) // sound 0x47, the bow's skill (M11)
		return
	}
	if inRange {
		if b := w.brain(); b != nil {
			a := &w.Actors()[ti]
			b.HitFromParty(member, ti, monsters.BlowDirection(w.group.X, w.group.Y, w.group.Z, a))
		}
	}
	units := p.RecoveryUnits(e, false)
	if delay := party.AttackDelay(units, m.TurnBased); delay > 0 {
		m.SetRecovery(member, delay)
	}
	// sound by the weapon's skill: 0x51, 0x54, 0x55, 0x4e, 0x50, 0x47, 0x53, 0x43 (M11)
}

// attackTarget is the actor the blow aims at: the one under the mouse when it can
// act, else the nearest drawn one within 0x1400 that is not dying, dead, removed,
// disabled or summoned.
//
// mm8: 0x42e27f (the pick), 0x42e6a4 (the buffer)
func (w *World) attackTarget(mx, my int) int {
	acts := w.Actors()
	if pid, _ := w.pick(mx, my); pid&7 == pidActor {
		i := pidIndex(pid)
		if i >= 0 && i < len(acts) && acts[i].CanAct() {
			return i
		}
	}
	f := w.frame
	if f == nil || f.ID == nil {
		return -1
	}
	best, bestD := -1, float64(0x1400)
	seen := map[int]bool{}
	for i, id := range f.ID {
		if id&7 != pidActor || i >= len(f.Z) || f.Z[i] <= 0 {
			continue
		}
		ai := pidIndex(id)
		if ai < 0 || ai >= len(acts) || seen[ai] {
			continue
		}
		d := 1 / float64(f.Z[i])
		if d > 0x1400 {
			continue
		}
		switch acts[ai].AIState {
		case monsters.Dying, monsters.Dead, monsters.Removed, monsters.Disabled, monsters.Summoned:
			continue
		}
		seen[ai] = true
		if d <= bestD {
			best, bestD = ai, d
		}
	}
	return best
}

// PartyThreat is g_partyFlags 0x10 and 0x20: a hostile actor within a blow's reach,
// and one within 0x1400. The ready gem is red, yellow or green from the two.
func (w *World) PartyThreat() (close, near bool) {
	if w.ai == nil {
		return false, false
	}
	return w.ai.Close, w.ai.Near
}

// partyDied is what follows nobody being able to act. The original reloads save 0
// first (M10); without a save, the party itself is revived and sent to the temple,
// and that map's actors are reset when it loads.
//
// mm8: 0x46261d (Game_Loop, g_partyCreateResult == 8)
func (w *World) partyDied() {
	m := w.S.Party
	c := w.S.Ctx
	if m == nil || c == nil {
		w.PartyDead = false
		return
	}
	for i := range w.Actors() {
		w.Actors()[i].Flags &^= monsters.FlagHostile
	}
	w.group.Flags &^= party.FlagBoundObject | party.FlagWater | party.FlagBurning
	w.group.Fly, w.group.WaterWalk, w.group.FeatherFall, w.group.Levitate = false, false, false, false
	m.Deaths++
	m.DaysWithoutRest = 0
	m.Time += clock.Week
	m.Calendar = m.Time.Calendar()
	m.Gold = 0
	m.TurnBased = false
	m.Buffs = [party.NumPartyBuffs]party.Buff{}
	e := m.Env(c)
	for i := range m.Players {
		p := &m.Players[i]
		if !p.Awards.Get(0x2b) && w.AwardText(0x2b) {
			m.Speak(i, 0x60, c)
		}
		p.Awards.Set(0x2b, true)
		for k := range p.Conditions {
			p.Conditions[k] = 0
		}
		p.Buffs = [party.NumPlayerBuffs]party.Buff{}
		p.HP = p.MaxHP(e)
		p.SP = p.MaxSP(e)
		p.Recovery = 0
		m.Selected = 1
	}
	var living []int
	for i := range m.Players {
		if m.Players[i].CanAct() {
			living = append(living, i)
		}
	}
	if len(living) > 0 {
		m.Speak(living[c.Rand.Int()%len(living)], 99, c)
	}
	w.Status(w.Global(0x20c), 2)
	name := "out01.odm"
	x, y, z, dir := int32(0xeb6), int32(0x1de1), int32(0x221), int32(0x4e0)
	if m.QBits.Get(0x5d) {
		name = "out02.odm"
		x, y, z, dir = 0x207b, -0x3cca, 0x115, 2048 // 2048 masks to the original's 0
	}
	w.S.reviveReset = true
	w.MoveToMap(name, x, y, z, dir, 0, 0)
	w.PartyDead = false
}

// reviveActors resets a map the party was just revived onto.
func (w *World) reviveActors() {
	if w.S == nil || !w.S.reviveReset {
		return
	}
	w.S.reviveReset = false
	for i := range w.Actors() {
		w.Actors()[i].Flags &^= monsters.FlagHostile
	}
	if b := w.brain(); b != nil {
		b.ResetAfterRest(w.S.Party.Calendar.Hour)
	}
	w.removeSpentObjects()
}

// removeSpentObjects drops projectiles (a lifetime object whose frame clock has started).
//
// mm8: 0x46261d (the object loop of a revival)
func (w *World) removeSpentObjects() {
	objs, mt := w.Objects(), w.monsterTables()
	if mt == nil {
		return
	}
	for i := range objs {
		o := &objs[i]
		if o.ObjList <= 0 || int(o.ObjList) >= len(mt.ObjList) {
			continue
		}
		if mt.ObjList[o.ObjList].Flags&tables.ObjLifetime != 0 && o.Time != 0 {
			monsters.RemoveObject(objs, i)
		}
	}
}
