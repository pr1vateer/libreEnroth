package party

import (
	"strings"

	"libre-enroth/internal/assets/tables"
	"libre-enroth/internal/assets/txt"
	"libre-enroth/internal/game/items"
)

// NewRoster makes the roster characters from roster.txt: name, class, face, voice,
// birth year, experience and level, the base stats, resistances, skill points, skills
// (a B/E/M/G column and a level column each), the number of spells known per school
// (at most 11), the spell book page and the equipment. An equipment cell is id + 1000 *
// level: the generator makes an item of the id's equip type at that level (at least
// 1, a bonus forced) for its enchantment, which then takes the id; it is identified
// and worn when the character has its skill (or it needs none) and the slot is free,
// else packed. HP and SP start full. Without c's tables only the names, classes,
// faces and voices are filled.
//
// mm8: 0x49680a (Txt_LoadRoster)
func NewRoster(entries []tables.RosterEntry, c *Ctx, found *[items.NumArtifacts]bool) []Player {
	out := make([]Player, len(entries))
	for i, en := range entries {
		p := &out[i]
		*p = Player{Name: en.Name, Class: en.Class, Face: int(en.Face), Voice: int(en.Voice), RosterID: i, Expr: ExprNormal}
		if rosterBlurb < len(en.Cells) {
			p.Biography = txt.StripQuotes(en.Cells[rosterBlurb])
			if len(p.Biography) > maxBiography {
				p.Biography = p.Biography[:maxBiography]
			}
		}
		if c == nil || c.Items == nil || c.Classes == nil {
			continue
		}
		p.rosterStats(en, c, found)
	}
	return out
}

// roster.txt's Blurb column and the biography's size (strncpy 0x289).
const (
	rosterBlurb  = 123
	maxBiography = 0x289
)

// rosterStats fills a roster character's stats from its row's cells.
//
// mm8: 0x49680a (Txt_LoadRoster)
func (p *Player) rosterStats(en tables.RosterEntry, c *Ctx, found *[items.NumArtifacts]bool) {
	cell := func(k int) string {
		if k < len(en.Cells) {
			return en.Cells[k]
		}
		return ""
	}
	atoi := func(k int) int32 { return txt.Atoi(cell(k)) }
	p.BirthYear, p.Exp, p.LevelBase = en.Birth, en.Experience, int16(en.Level)
	for k := range p.Stats { // cells 8..14 in struct order: Speed (12) before Accuracy (13)
		p.Stats[k].Base = int16(atoi(8 + k))
	}
	for k, r := range [...]int{DamageFire, DamageAir, DamageWater, DamageEarth, DamageMind, DamageBody} {
		p.Resists[r] = uint16(atoi(15 + k))
	}
	p.SkillPoints = atoi(21)
	for s := 0; s < NumSkills; s++ {
		var m uint16
		switch k := 22 + 2*s; {
		case strings.EqualFold(cell(k), "G"):
			m = SkillGM
		case strings.EqualFold(cell(k), "M"):
			m = SkillMaster
		case strings.EqualFold(cell(k), "E"):
			m = SkillExpert
		}
		p.Skills[s] = m | uint16(atoi(23+2*s))
	}
	for school := 0; school < 12; school++ {
		n := min(int(atoi(100+school)), 11)
		for k := 0; k < n; k++ {
			p.Spells[school*spellsPerSchool+k] = true
		}
	}
	for s := 0; s < 12; s++ {
		if p.Skills[SkillFire+s] != 0 {
			p.SpellPage = uint8(s)
			break
		}
	}
	for k := 113; k < 123; k++ {
		v := atoi(k)
		id := v % 1000
		if id <= 0 {
			continue
		}
		lvl := max(int(v/1000), 1)
		d := c.Items.Item(id)
		it := items.Generate(c.Items, lvl, int(d.EquipType)+1, true, c.Rand, found)
		it.Flags |= items.FlagIdentified
		it.Number = id
		if d.Skill != tables.SkillMisc && p.SkillAt(int(d.Skill)) == 0 || !p.EquipItem(c.Items, c.Classes, it) {
			p.AddItem(c.Items, -1, it)
		}
	}
	e := &Env{Items: c.Items, Classes: c.Classes}
	p.HP, p.SP = p.MaxHP(e), p.MaxSP(e)
}

// RosterSlot is the party slot of roster character id, -1 when not in the party.
//
// mm8: 0x48dd0e
func (m *Members) RosterSlot(id int) int {
	for i := range m.Players {
		if m.Players[i].RosterID == id {
			return i
		}
	}
	return -1
}

// RosterPlayer is roster character id wherever it is: the member when in the party,
// else its roster entry; nil outside 0..49 (g_players + id).
//
// mm8: 0x4446bd (case 0x44: g_players + id when 0 <= id <= 0x31)
func (m *Members) RosterPlayer(id int) *Player {
	if i := m.RosterSlot(id); i >= 0 && id >= 0 {
		return &m.Players[i]
	}
	if id < 0 || id >= len(m.Roster) {
		return nil
	}
	return &m.Roster[id]
}

// AddRoster has roster character id join the party: 1 when it joins, -1 when it is
// already a member (or the id is bad), -2 when it cannot (the party is full or in
// turn-based mode). Roster character 4 joining stamps history entry 4.
//
// mm8: 0x48dc48 (Party_AddRosterMember; the join sound 0x36ec is M11's)
func (m *Members) AddRoster(id int) int {
	if m.TurnBased {
		return -2
	}
	if id == 4 {
		m.History[4] = m.Time
	}
	if len(m.Players) >= MaxMembers {
		return -2
	}
	if m.RosterSlot(id) >= 0 || id < 0 || id >= len(m.Roster) {
		return -1
	}
	p := m.Roster[id]
	p.RosterID = id
	m.Players = append(m.Players, p)
	return 1
}

// RemoveMember sends the member in slot (0-based) away: its roster quest bit
// (400 + roster id) is set, it goes back to the roster and the members after it move
// up. Nothing happens in turn-based mode. The buffs it cast end (M9).
//
// mm8: 0x48dbc2 (Party_RemoveMember), 0x48db1a (compacting g_partyRoster)
func (m *Members) RemoveMember(slot int) {
	if slot < 0 || slot >= len(m.Players) || m.TurnBased {
		return
	}
	p := m.Players[slot]
	if p.RosterID >= 0 {
		m.QBits.Set(400+p.RosterID, true)
		if p.RosterID < len(m.Roster) {
			m.Roster[p.RosterID] = p
		}
	}
	m.Players = append(m.Players[:slot], m.Players[slot+1:]...)
}
