package npc

import (
	"slices"
	"testing"

	"libre-enroth/internal/assets/tables"
)

func state() *State {
	t := &tables.NPCTables{
		NPCs:      make([]tables.NPC, 8),
		Greetings: [][2]string{{}, {"first", "again"}},
		GroupNews: []int16{0, 2},
		News:      []string{"", "", "news two"},
	}
	t.NPCs[2] = tables.NPC{House: 5, Greeting: 1}
	t.NPCs[3] = tables.NPC{House: 5, Flags: FlagGone}
	t.NPCs[6] = tables.NPC{House: 5}
	t.NPCs[7] = tables.NPC{House: 9}
	return New(t)
}

// TestGreeting: the counter goes up on each visit (to 2) and the second greeting shows
// from the second visit on; a new greeting starts the count over.
//
// mm8: 0x443b6f, 0x443da1, 0x443441, 0x4446bd (case 0x32)
func TestGreeting(t *testing.T) {
	s := state()
	n := s.Get(2)
	var got []string
	for range 3 {
		Visit(n)
		got = append(got, s.Greeting(n))
	}
	if want := []string{"first", "again", "again"}; !slices.Equal(got, want) || n.Flags != 2 {
		t.Errorf("greetings %q (flags %d), want %q", got, n.Flags, want)
	}
	s.SetGreeting(2, 1)
	Visit(n)
	if g := s.Greeting(n); g != "first" {
		t.Errorf("after SetGreeting: %q", g)
	}
	if s.Greeting(s.Get(6)) != "" {
		t.Error("no greeting should be empty")
	}
}

// TestResidents: house 5's NPCs, in id order, without the gone ones; each visit counts.
//
// mm8: 0x443da1 (House_LoadResidents)
func TestResidents(t *testing.T) {
	s := state()
	if got := s.Residents(5); !slices.Equal(got, []int{2, 6}) {
		t.Errorf("residents %v", got)
	}
	if s.Get(2).Flags != 1 || s.Get(3).Flags != FlagGone {
		t.Errorf("flags %d %d", s.Get(2).Flags, s.Get(3).Flags)
	}
	s.Move(7, 5)
	if got := s.Residents(5); !slices.Equal(got, []int{2, 6, 7}) {
		t.Errorf("after Move: %v", got)
	}
	if s.Get(-1) != nil || s.Get(8) != nil || Kind(5000) != KindHireling || Kind(-1) != KindNegative || Kind(1) != KindNPC {
		t.Error("ids out of range")
	}
	s.SetTopic(2, 5, 77)
	s.SetTopic(2, 6, 78)
	if s.Get(2).Topics != [6]int32{5: 77} {
		t.Errorf("topics %v", s.Get(2).Topics)
	}
	if s.News(1) != "news two" || s.News(9) != "" {
		t.Error("news")
	}
	s.SetGroupNews(1, 0)
	if s.News(1) != "" {
		t.Error("SetGroupNews")
	}
}
