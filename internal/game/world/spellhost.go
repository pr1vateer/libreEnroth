package world

import (
	"libre-enroth/internal/assets/tables"
	"libre-enroth/internal/game/items"
	"libre-enroth/internal/game/monsters"
	"libre-enroth/internal/game/party"
	"libre-enroth/internal/game/spells"
)

// spellHost is the cast queue's view of the world.
type spellHost struct{ w *World }

func (h spellHost) Rand() int {
	if h.w.S == nil || h.w.S.Ctx == nil || h.w.S.Ctx.Rand == nil {
		return 0
	}
	return h.w.S.Ctx.Rand.Int()
}

func (h spellHost) Note(key, what string) {
	if h.w.S != nil {
		h.w.S.note(key, what)
	}
}

func (h spellHost) TurnBased() bool {
	return h.w.S != nil && h.w.S.Party != nil && h.w.S.Party.TurnBased
}

func (h spellHost) Hour() (int, int) {
	if h.w.S == nil || h.w.S.Party == nil {
		return 0, 0
	}
	c := h.w.S.Party.Calendar
	return c.Hour, c.Minute
}

func (h spellHost) Player(roster int) (spells.Player, bool) {
	p := h.player(roster)
	if p == nil || h.w.S == nil || h.w.S.Party == nil {
		return spells.Player{}, false
	}
	m := h.w.S.Party
	out := spells.Player{CanAct: p.CanAct(), Cursed: p.Conditions[party.CondCursed] != 0, SP: int(p.SP), Raw: p.Skills}
	out.Level = func(skill int) uint16 {
		if h.w.S.Ctx == nil {
			if skill < 0 || skill >= len(p.Skills) {
				return 0
			}
			return p.Skills[skill]
		}
		return p.Skill(m.Env(h.w.S.Ctx), skill)
	}
	return out, true
}

func (h spellHost) Pay(roster, cost int) bool {
	p := h.player(roster)
	if p == nil || int(p.SP) < cost {
		return false
	}
	p.SP -= int32(cost)
	return true
}

func (h spellHost) Spend(roster, cost int) {
	if p := h.player(roster); p != nil {
		p.SP -= int32(cost)
	}
}

func (h spellHost) Recover(roster, ticks int) {
	m := h.member()
	if m == nil {
		return
	}
	if slot := m.RosterSlot(roster); slot >= 0 {
		m.SetRecovery(slot, ticks)
	}
}

func (h spellHost) Speak(roster, id int) {
	m := h.member()
	if m == nil || h.w.S.Ctx == nil {
		return
	}
	if slot := m.RosterSlot(roster); slot >= 0 {
		m.Speak(slot, id, h.w.S.Ctx)
	}
}

func (h spellHost) NoMana() { h.w.Status(h.w.Global(0x24a), 2) }

func (h spellHost) Failed() { h.w.Status(h.w.Global(0x1ac), 2) }

func (h spellHost) ClearInvisibility() {
	if m := h.member(); m != nil {
		m.Buffs[party.PartyBuffInvisibility] = party.Buff{}
	}
}

func (h spellHost) Spell(id int) tables.Spell {
	if h.w.tables == nil || h.w.tables.Game == nil {
		return tables.Spell{School: 4}
	}
	return h.w.tables.Game.Spells.At(id)
}

func (h spellHost) Recovery(roster int, ranged bool) int {
	p := h.player(roster)
	if p == nil || h.w.S == nil || h.w.S.Ctx == nil || h.w.S.Party == nil {
		return 0
	}
	return p.RecoveryUnits(h.w.S.Party.Env(h.w.S.Ctx), ranged)
}

func (h spellHost) Target(roster, spell int) int32 {
	if i := h.w.attackTarget(h.w.mx, h.w.my); i >= 0 {
		return int32(uint32(i)<<3 | monsters.KindActor)
	}
	return 0
}

func (h spellHost) Aim(roster int, target int32) (int32, int32) {
	g := h.w.group
	if g == nil {
		return 0, 0
	}
	if target&7 == monsters.KindActor {
		if b := h.w.brain(); b != nil {
			d := b.Direction(uint32(roster)<<3|monsters.KindParty, uint32(target), 0)
			return d.Yaw, d.Pitch
		}
	}
	return g.Dir, g.Look
}

func (h spellHost) Place() (int32, int32, int32, int32) {
	g := h.w.group
	if g == nil {
		return 0, 0, 0, 0
	}
	return g.X, g.Y, g.Z, g.Height
}

func (h spellHost) Sector(x, y, z int32) int16 {
	if h.w.indoor == nil || h.w.indoor.geo == nil {
		return 0
	}
	return int16(h.w.indoor.geo.SectorAt(x, y, z))
}

func (h spellHost) Obj(typeID int16) (int16, int16) {
	mt := h.w.monsterTables()
	if mt == nil || typeID == 0 {
		return 0, 0
	}
	i := mt.ObjListOf(typeID)
	if i <= 0 || i >= len(mt.ObjList) {
		return 0, 0
	}
	return int16(i), mt.ObjList[i].Speed
}

func (h spellHost) Hand(roster, slot int) items.Item {
	p := h.player(roster)
	if p == nil || slot <= 0 {
		return items.Item{}
	}
	if it := p.Equipped(slot); it != nil {
		return *it
	}
	return items.Item{}
}

func (h spellHost) Fire(s spells.Shot) int {
	dl := h.w.delta()
	if dl == nil || s.Obj == 0 {
		return -1
	}
	o := monsters.Object{
		Type: s.Type, ObjList: s.Obj, Pos: [3]int32{s.X, s.Y, s.Z},
		Yaw: uint16(s.Yaw), Attrib: s.Attrib, Sector: s.Sector,
		Caster: s.Caster, Target: s.Target, Spell: s.Spell, SpellLevel: s.Level, SpellSkill: s.Skill,
		Sound: s.Sound, Item: s.Item, Glow: 1,
	}
	var roster []int
	if m := h.member(); m != nil {
		for i := range m.Players {
			roster = append(roster, m.Players[i].RosterID)
		}
	}
	var slot int
	dl.Objects, slot = monsters.LaunchFor(dl.Objects, o, s.Yaw, s.Pitch, s.Speed, s.Member, roster)
	return slot
}

func (h spellHost) player(roster int) *party.Player {
	m := h.member()
	if m == nil {
		return nil
	}
	return m.RosterPlayer(roster)
}

func (h spellHost) member() *party.Members {
	if h.w.S == nil {
		return nil
	}
	return h.w.S.Party
}

// enqueueCast writes one cast for party member (0-based).
func (w *World) enqueueCast(spell, member int) {
	m := w.S.Party
	roster := make([]int, len(m.Players))
	for i := range m.Players {
		roster[i] = m.Players[i].RosterID
	}
	w.casts.Enqueue(spell, member, 0, 0, 0, roster)
}
