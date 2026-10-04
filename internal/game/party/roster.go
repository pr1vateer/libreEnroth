package party

import "libre-enroth/internal/assets/tables"

// NewRoster makes the roster characters from roster.txt (Txt_LoadRoster 0x49680a): name,
// class, face and voice so far; the stats, skills and items are M7's.
func NewRoster(entries []tables.RosterEntry) []Player {
	out := make([]Player, len(entries))
	for i, e := range entries {
		out[i] = Player{Name: e.Name, Class: e.Class, Face: int(e.Face), Voice: int(e.Voice), RosterID: i, Expr: ExprNormal}
	}
	return out
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
