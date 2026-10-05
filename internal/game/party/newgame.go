package party

import (
	"libre-enroth/internal/assets/tables"
	"libre-enroth/internal/game/items"
)

// NewBirthYear is a new character's birth year: 20 years before the game starts.
//
// mm8: 0x492094 (+0x1c00 = 0x480)
const NewBirthYear = 0x480

// SkillNone is the "no skill" answer of CreationSkill.
const SkillNone = NumSkills

// ResetCreation puts a character on the creation screen back to its class: the class's
// base stats, no resistances, no skills but the class's two starting ones at level 1,
// born in 1152. (The original also picks a default name here when +0x1c88 is 0; the
// name edit of libre-enroth starts empty instead.)
//
// mm8: 0x492094 (both flags 0)
func (p *Player) ResetCreation(cls *tables.Classes) {
	c := p.classIndex()
	for s := StatMight; s <= StatLuck; s++ {
		*p.BasePtr(s) = int16(cls.Stats[c][s].Base)
	}
	p.Skills = [NumSkills]uint16{}
	p.Resists = [NumResists]uint16{}
	for s := 0; s < NumSkills; s++ {
		if cls.ClassSkill(p.Class, s) == 2 {
			p.Skills[s] = 1
		}
	}
	p.BirthYear = NewBirthYear
}

// CreationSkill is the skill shown in creation slot n: 0-1 the class's starting
// skills, 2-3 the two chosen ones (the choosable skills the player has, in skill
// order), 4-12 the choosable ones; SkillNone when there is none.
//
// mm8: 0x4912b0 (Player_CreationSkill)
func (p *Player) CreationSkill(cls *tables.Classes, n int) int {
	pick := func(kind uint8, idx int, need bool) int {
		for s := 0; s < NumSkills; s++ {
			if cls.ClassSkill(p.Class, s) == kind && (!need || p.Skills[s] != 0) {
				if idx == 0 {
					return s
				}
				idx--
			}
		}
		return SkillNone
	}
	switch {
	case n < 0:
	case n < 2:
		return pick(2, n, false)
	case n < 4:
		return pick(1, n-2, true)
	case n < 13:
		return pick(1, n-4, false)
	}
	return SkillNone
}

// HasTwoExtraSkills reports more than three skills: the two starting ones and two more.
//
// mm8: 0x4916e0 (PartyCreate_HasTwoExtraSkills)
func (p *Player) HasTwoExtraSkills() bool {
	n := 0
	for _, s := range p.Skills {
		if s != 0 {
			n++
		}
	}
	return n > 3
}

// PointsLeft is the creation bonus pool: 15 plus every stat's distance from its class
// base, weighted by the cost of the steps (UpCost / UpStep above the base, the
// inverse below).
//
// mm8: 0x49170c (PartyCreate_PointsLeft)
func (p *Player) PointsLeft(cls *tables.Classes) int {
	pts := 15
	for s := StatMight; s <= StatLuck; s++ {
		e := cls.Stats[p.classIndex()][s]
		v, base := p.Base(s), int(e.Base)
		a, b := int(e.UpCost), int(e.UpStep)
		if v < base {
			a, b = b, a
		}
		pts += (base - v) * a / b
	}
	return pts
}

// StatUp raises stat s on the creation screen: by UpStep for UpCost points at or above
// the base (UpCost for UpStep below), when the points are there and the class maximum
// allows. Reports success (else the original plays the error sound 0x1b).
//
// mm8: 0x491516
func (p *Player) StatUp(cls *tables.Classes, s Stat) bool {
	e := cls.Stats[p.classIndex()][s]
	step, cost := int(e.UpStep), int(e.UpCost)
	if p.Base(s) < int(e.Base) {
		step, cost = cost, step
	}
	if cost > p.PointsLeft(cls) || p.Base(s)+step > int(e.Max) {
		return false
	}
	*p.BasePtr(s) += int16(step)
	return true
}

// StatDown lowers stat s on the creation screen: by UpStep above the base, by UpCost at
// or below it, down to two below the base.
//
// mm8: 0x491385
func (p *Player) StatDown(cls *tables.Classes, s Stat) bool {
	e := cls.Stats[p.classIndex()][s]
	step := int(e.UpStep)
	if p.Base(s) <= int(e.Base) {
		step = int(e.UpCost)
	}
	if int(e.Base)-2 > p.Base(s)-step {
		return false
	}
	*p.BasePtr(s) -= int16(step)
	return true
}

// ChooseSkill takes choosable skill k (creation slot k + 4) while the second extra slot
// is free.
//
// mm8: 0x433bbd (Menu_ProcessMessages, msg 0x40)
func (p *Player) ChooseSkill(cls *tables.Classes, k int) {
	s := p.CreationSkill(cls, k+4)
	if p.CreationSkill(cls, 3) == SkillNone && s != SkillNone {
		p.Skills[s] = 1
	}
}

// DropSkill gives back the chosen skill in creation slot 2 or 3. (The original writes
// past the skills, into the awards, when the slot is empty; this does not.)
//
// mm8: 0x433bbd (msgs 0x1cd, 0x1ce)
func (p *Player) DropSkill(cls *tables.Classes, slot int) {
	if s := p.CreationSkill(cls, slot); s != SkillNone {
		p.Skills[s] = 0
	}
}

// Starting items of the skills: a weapon or armour, a spell book (and its first
// spell), or an empty bottle and a potion.
//
// mm8: 0x495cb6 (PartyCreation_Run's switch)
var skillItems = [NumSkills]int32{
	SkillStaff: 0x4f, SkillSword: 1, SkillDagger: 0x15, SkillAxe: 0x1f, SkillSpear: 0x29,
	SkillBow: 0x38, SkillMace: 0x42, SkillShield: 99, SkillLeather: 0x54, SkillChain: 0x59,
	SkillPlate: 0x5e,
	SkillFire:  0x191, SkillFire + 1: 0x19c, SkillFire + 2: 0x1a7, SkillFire + 3: 0x1b2,
	SkillFire + 4: 0x1bd, SkillFire + 5: 0x1c8, SkillFire + 6: 0x1d3, SkillFire + 8: 0x1e9,
}

// potionSkills get an empty bottle and one of the potions 200, 205 and 210.
var potionSkills = map[int]bool{
	SkillIDItem: true, SkillRepair: true, SkillMeditation: true, SkillPerception: true,
	SkillDisarm: true, SkillLearning: true,
}

// spellsPerSchool is the number of spells of a school (Player +0x406: 11 per school).
const spellsPerSchool = 11

// FinishCreation is what happens to the hero (member 0) when party creation ends: the
// party's order of 0..31 is shuffled, the spell book opens at the first magic skill, a
// random level-2 ring goes in the pack, every skill brings its item (weapon, armour,
// spell book with the school's first spell, or a bottle and a random potion; the
// racial skills their first spell), everything is identified and HP and SP are full.
//
// mm8: 0x495cb6 (PartyCreation_Run, after the screen closes)
func (m *Members) FinishCreation(c *Ctx) {
	rng := c.Rand
	var used [32]bool
	for i := range m.Shuffle {
		k := 0
		tries := 0
		for ; tries < 10; tries++ {
			k = rng.Int() % 0x20
			if !used[k] {
				break
			}
		}
		if tries == 10 {
			for k = 0; used[k]; k++ {
			}
		}
		m.Shuffle[i] = uint8(k)
		used[k] = true
	}
	p := &m.Players[0]
	p.ExprTime = 0
	for s := 0; s < 12; s++ {
		if p.Skills[SkillFire+s] != 0 {
			p.SpellPage = uint8(s)
			break
		}
	}
	ring := items.Generate(c.Items, 2, tables.EquipRing+0x1e, false, rng, &m.ArtifactsFound)
	p.AddItem(c.Items, -1, ring)
	e := m.Env(c)
	for s := 0; s < NumSkills; s++ {
		if p.Skills[s] == 0 {
			continue
		}
		switch {
		case skillItems[s] != 0:
			p.AddItemNumber(c.Items, -1, skillItems[s])
			if s >= SkillFire {
				p.Spells[(s-SkillFire)*spellsPerSchool] = true
			}
		case s >= SkillDarkElf && s <= SkillDragon:
			p.Spells[(s-SkillFire)*spellsPerSchool] = true
		case potionSkills[s]:
			p.AddItemNumber(c.Items, -1, items.EmptyBottle)
			p.AddItemNumber(c.Items, -1, int32((rng.Int()%3+0x28)*5))
		}
		// (Every skill, even one without an item, goes on to the identification and
		// the HP and SP.)
		for k := range p.Items {
			if p.Items[k].Number != 0 {
				p.Items[k].Flags |= items.FlagIdentified
			}
		}
		p.HP, p.SP = p.MaxHP(e), p.MaxSP(e)
	}
}

// DefaultMember gives a character that skipped the creation screen (the debug party)
// its class's base stats and starting skills and full HP and SP.
func (m *Members) DefaultMember(i int, c *Ctx) {
	p := &m.Players[i]
	p.ResetCreation(c.Classes)
	if p.LevelBase == 0 {
		p.LevelBase = 1
	}
	e := m.Env(c)
	p.HP, p.SP = p.MaxHP(e), p.MaxSP(e)
}

// DebugEquip gives member i the items of its skills, as party creation gives the hero,
// and wears what it can (a debug aid for the -equip flag; not in the original).
func (m *Members) DebugEquip(i int, c *Ctx) {
	p := &m.Players[i]
	for s := 0; s < NumSkills; s++ {
		id := skillItems[s]
		if p.Skills[s] == 0 || id == 0 {
			continue
		}
		it := items.New(c.Items, id)
		it.Flags |= items.FlagIdentified
		if !p.EquipItem(c.Items, c.Classes, it) {
			p.AddItem(c.Items, -1, it)
		}
	}
	e := m.Env(c)
	p.HP, p.SP = p.MaxHP(e), p.MaxSP(e)
}
