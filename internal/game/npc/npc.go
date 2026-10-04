// Package npc is the game's NPC table: the runtime copy of npcdata.txt that events
// change (topics, house, greeting) and the group news (re/notes/houses.md).
package npc

import "libre-enroth/internal/assets/tables"

// NPC flags (+0x08).
const (
	FlagVisits = 3    // visit counter, 0..2: the second greeting shows from the second visit
	FlagGone   = 0x80 // joined the party or sent away: no longer in houses
)

// Kinds of NPC id (0x443b4f).
const (
	KindNPC      = 1 // 0..4999: the NPC table
	KindHireling = 2 // 5000 and up: the street NPCs actors carry (M8)
	KindNegative = 3
)

// State is the NPC table of a game (0x76bc2c, 0x4c bytes each, saved with the game) and
// the group news (0x779e8e).
type State struct {
	NPCs      []tables.NPC
	GroupNews []int16
	t         *tables.NPCTables
}

// New starts a game's table from the templates.
//
// mm8: 0x49228d (Party_InitNewGame: memcpy(0x76bc2c, 0x761898, 0xa394) and the group news)
func New(t *tables.NPCTables) *State {
	return &State{
		NPCs:      append([]tables.NPC(nil), t.NPCs...),
		GroupNews: append([]int16(nil), t.GroupNews...),
		t:         t,
	}
}

// Kind classifies an NPC id.
//
// mm8: 0x443b4f
func Kind(id int) int {
	switch {
	case id < 0:
		return KindNegative
	case id < 5000:
		return KindNPC
	}
	return KindHireling
}

// Get returns NPC id, nil for ids outside the table (the original errors out above 550
// and keeps the 5000+ hirelings elsewhere, M8).
//
// mm8: 0x443ae2
func (s *State) Get(id int) *tables.NPC {
	if id < 0 || id >= len(s.NPCs) {
		return nil
	}
	return &s.NPCs[id]
}

// SetTopic gives NPC id event in topic slot 0..5 (other slots do nothing).
//
// mm8: 0x4446bd (case 0x27)
func (s *State) SetTopic(id, slot int, event int32) {
	if n := s.Get(id); n != nil && slot >= 0 && slot < len(n.Topics) {
		n.Topics[slot] = event
	}
}

// Move puts NPC id in house.
//
// mm8: 0x4446bd (case 0x28: npc +0x14)
func (s *State) Move(id int, house int32) {
	if n := s.Get(id); n != nil {
		n.House = house
	}
}

// SetGreeting gives NPC id a greeting and starts its visits over.
//
// mm8: 0x4446bd (case 0x32: +0x1c, flags +8 &= ~3)
func (s *State) SetGreeting(id int, greet int32) {
	if n := s.Get(id); n != nil {
		n.Greeting = greet
		n.Flags &^= FlagVisits
	}
}

// SetGroupNews sets the news group tells (index as the original: no range check
// beyond the table).
//
// mm8: 0x4446bd (case 0x2f: 0x779e8e + group*2)
func (s *State) SetGroupNews(group int, news int16) {
	if group >= 0 && group < len(s.GroupNews) {
		s.GroupNews[group] = news
	}
}

// News is the text group tells, "" when none.
func (s *State) News(group int) string {
	if group < 0 || group >= len(s.GroupNews) {
		return ""
	}
	if i := int(s.GroupNews[group]); i >= 0 && i < len(s.t.News) {
		return s.t.News[i]
	}
	return ""
}

// Visit counts a visit: the counter in flags bits 0-1 goes up to 2.
//
// mm8: 0x443b6f (Evt_SpeakNPC), 0x443da1 (House_LoadResidents)
func Visit(n *tables.NPC) {
	if n.Flags&FlagVisits != 2 {
		n.Flags++
	}
}

// Greeting is the greeting n says now: the second one from the second visit on (the
// counter is already up for this visit). "" for none.
//
// mm8: 0x443441, 0x4b363b (0x7797b8[(greet*2 + (flags&3 == 2))])
func (s *State) Greeting(n *tables.NPC) string {
	g := int(n.Greeting)
	if g <= 0 || g >= len(s.t.Greetings) {
		return ""
	}
	second := 0
	if n.Flags&FlagVisits == 2 {
		second = 1
	}
	return s.t.Greetings[g][second]
}

// Residents lists the NPCs of house in id order (not gone), counting a visit for each.
//
// mm8: 0x443da1 (House_LoadResidents: ids 1..count-1 with +0x14 == house and !(flags & 0x80))
func (s *State) Residents(house int) []int {
	var out []int
	for id := 1; id < len(s.NPCs); id++ {
		n := &s.NPCs[id]
		if int(n.House) == house && n.Flags&FlagGone == 0 {
			out = append(out, id)
			Visit(n)
		}
	}
	return out
}
