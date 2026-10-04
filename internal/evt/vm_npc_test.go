package evt

import (
	"slices"
	"testing"

	"libre-enroth/internal/assets/tables"
	"libre-enroth/internal/game/npc"
	"libre-enroth/internal/game/party"
)

// TestCanShowTopic: the visibility records of a topic's global event.
//
// mm8: 0x4444fe (Evt_CanShowTopic)
func TestCanShowTopic(t *testing.T) {
	h := newHost(2)
	h.m.QBits = make(party.Bits, party.QBitBytes)
	h.m.QBits.Set(10, true)
	vm := &VM{Host: h, Global: script(t,
		// 1: no visibility records at all.
		rec(1, 0, OpSetNPCTopic, 0, 0, 0, 0, 0, 0, 0, 0, 0),
		// 2: qbit 10 set -> jump to 2 -> shown.
		rec(2, 0, OpOnCanShowDialog, varArgs(VarQBits, 10, 2)...),
		rec(2, 1, OpSetCanShowDialog, 0),
		rec(2, 2, OpSetCanShowDialog, 1),
		rec(2, 3, OpEndCanShowDialog),
		// 3: qbit 11 clear -> falls to the hidden branch.
		rec(3, 0, OpOnCanShowDialog, varArgs(VarQBits, 11, 3)...),
		rec(3, 1, OpSetCanShowDialog, 0),
		rec(3, 2, OpEndCanShowDialog),
		rec(3, 3, OpSetCanShowDialog, 1),
		// 4: Exit before anything visibility-related.
		rec(4, 0, OpExit),
		rec(4, 1, OpSetCanShowDialog, 0),
		// 5: a set value other than 0/1 counts as shown.
		rec(5, 0, OpSetCanShowDialog, 7),
	)}
	for id, want := range map[int]int{0: 0, 1: 2, 2: 1, 3: 0, 4: 2, 5: 1, 99: 2} {
		if got := vm.CanShowTopic(id); got != want {
			t.Errorf("CanShowTopic(%d) = %d, want %d", id, got, want)
		}
	}
	// Topic 3's compare passes when any member passes it.
	h.m.QBits.Set(11, true)
	if got := vm.CanShowTopic(3); got != 1 {
		t.Errorf("CanShowTopic(3) with qbit 11 = %d, want 1", got)
	}
}

// TestPressAnyKeyResume: PressAnyKey parks the event at the next step; Resume runs it
// from there.
//
// mm8: 0x4446bd (case 0x21), 0x44328b (Evt_Suspend), 0x4433b0 (Evt_ResumeUnpause)
func TestPressAnyKeyResume(t *testing.T) {
	h := newHost(1)
	vm := &VM{Host: h, Map: script(t,
		rec(7, 0, OpSet, varArgs(mapVar(0), 1)...),
		rec(7, 1, OpPressAnyKey),
		rec(7, 2, OpSet, varArgs(mapVar(1), 1)...),
	)}
	res := vm.Run(Source{}, 7, 0, true)
	want := Suspension{ID: 7, Step: 2, Op: OpPressAnyKey}
	if res.Suspended == nil || *res.Suspended != want || len(h.susp) != 1 || h.susp[0] != want {
		t.Fatalf("suspended %+v / %+v, want %+v", res.Suspended, h.susp, want)
	}
	if h.vars[0] != 1 || h.vars[1] != 0 {
		t.Fatalf("before resume: vars %v", h.vars[:2])
	}
	if res := vm.Resume(*res.Suspended, ""); res.Suspended != nil || h.vars[1] != 1 {
		t.Errorf("after resume: %+v, vars %v", res, h.vars[:2])
	}
	if vm.start != 0 {
		t.Errorf("start step left at %d", vm.start)
	}
}

// TestInputString: the first run asks (the question is NPCText) and waits at its own
// step; the answer is compared with two .str strings case-insensitively.
//
// mm8: 0x4446bd (case 0x1a)
func TestInputString(t *testing.T) {
	h := newHost(1)
	h.strs = []string{"", "Wind", "Air"}
	args := func(q, a1, a2 uint32, jmp byte) []byte {
		b := varArgs(0, 0)[:0]
		for _, v := range []uint32{q, a1, a2} {
			b = append(b, byte(v), byte(v>>8), byte(v>>16), byte(v>>24))
		}
		return append(b, jmp)
	}
	vm := &VM{Host: h, Global: script(t,
		rec(164, 1, OpInputString, args(595, 1, 2, 4)...),
		rec(164, 2, OpSet, varArgs(mapVar(0), 1)...), // wrong
		rec(164, 3, OpExit),
		rec(164, 4, OpSet, varArgs(mapVar(1), 1)...), // right
	)}
	src := Source{Kind: SourceGlobal}
	vm.Run(src, 164, 1, true)
	// Started at step 1, not 0: the original treats that as the resumed run already.
	if len(h.susp) != 0 || h.vars[0] != 1 {
		t.Fatalf("start step 1: suspended %v, vars %v", h.susp, h.vars[:2])
	}
	h.vars = [200]byte{}
	vm = &VM{Host: h, Global: script(t,
		rec(164, 0, OpOnCanShowDialog, varArgs(VarQBits, 1, 1)...),
		rec(164, 1, OpInputString, args(595, 1, 2, 4)...),
		rec(164, 2, OpSet, varArgs(mapVar(0), 1)...),
		rec(164, 3, OpExit),
		rec(164, 4, OpSet, varArgs(mapVar(1), 1)...),
	)}
	res := vm.Run(src, 164, 0, true)
	want := Suspension{Src: src, ID: 164, Step: 1, Op: OpInputString}
	if res.Suspended == nil || *res.Suspended != want {
		t.Fatalf("suspended %+v, want %+v", res.Suspended, want)
	}
	vm.Resume(want, "aIR")
	if h.vars[0] != 0 || h.vars[1] != 1 {
		t.Errorf("right answer: vars %v", h.vars[:2])
	}
	h.vars = [200]byte{}
	vm.Resume(want, "fire")
	if h.vars[0] != 1 || h.vars[1] != 0 {
		t.Errorf("wrong answer: vars %v", h.vars[:2])
	}
}

func npcState() *npc.State {
	t := &tables.NPCTables{
		NPCs:      make([]tables.NPC, tables.NumNPCs+1),
		Greetings: make([][2]string, 4),
		GroupNews: make([]int16, tables.NumGroups),
		News:      make([]string, tables.NumGroups),
	}
	t.NPCs[31] = tables.NPC{Name: "Thirty-one", House: 5, Greeting: 1, Flags: 2, Topics: [6]int32{1, 2}}
	return npc.New(t)
}

// TestNPCOpcodes: SetNPCTopic, MoveNPC, SetNPCGreeting, SetNPCGroupNews, SpeakNPC and
// SpeakInHouse.
//
// mm8: 0x4446bd (cases 0x27, 0x28, 0x32, 0x2f, 0x16, 2)
func TestNPCOpcodes(t *testing.T) {
	h := newHost(1)
	h.npcs = npcState()
	i32 := func(v int32) []byte { return []byte{byte(v), byte(v >> 8), byte(v >> 16), byte(v >> 24)} }
	vm := &VM{Host: h, Map: script(t,
		rec(1, 0, OpSetNPCTopic, slices.Concat(i32(31), []byte{2}, i32(905))...),
		rec(1, 1, OpSetNPCTopic, slices.Concat(i32(31), []byte{6}, i32(1))...), // no slot 6
		rec(1, 2, OpMoveNPC, slices.Concat(i32(31), i32(120))...),
		rec(1, 3, OpSetNPCGreeting, slices.Concat(i32(31), i32(3))...),
		rec(1, 4, OpSetNPCGroupNews, slices.Concat(i32(4), []byte{0x34, 0x12})...),
		rec(1, 5, OpSpeakNPC, i32(31)...),
		rec(1, 6, OpSpeakInHouse, i32(0)...),
		rec(1, 7, OpSpeakInHouse, i32(120)...),
		rec(2, 0, OpOnMapReload),
		rec(2, 1, OpSpeakNPC, i32(32)...),
	)}
	vm.Run(Source{}, 1, 0, true)
	n := h.npcs.Get(31)
	if n.Topics != [6]int32{1, 2, 905} || n.House != 120 || n.Greeting != 3 || n.Flags != 0 || h.npcs.GroupNews[4] != 0x1234 {
		t.Errorf("npc 31 = %+v, news %#x", n, h.npcs.GroupNews[4])
	}
	if want := []string{"SetNPCTopic 31", "SetNPCTopic 31", "MoveNPC 31"}; !slices.Equal(h.notes, want) {
		t.Errorf("NPCChanged %v, want %v", h.notes, want)
	}
	if !slices.Equal(h.houses, []int{120}) {
		t.Errorf("houses %v", h.houses)
	}
	vm.MapReload()
	if want := [][2]int{{31, 1}, {32, 0}}; !slices.Equal(h.speak, want) {
		t.Errorf("SpeakNPC %v, want %v", h.speak, want)
	}
	if len(h.stubs) != 0 {
		t.Errorf("stubs %v", h.stubs)
	}
}

// TestRoster: var 0x13e and IsPlayerInParty against the roster.
//
// mm8: 0x449726 / 0x44a0fe / 0x4483d3 (var 0x13e), 0x48dc48, 0x48dbc2, 0x4446bd (case 0x44)
func TestRoster(t *testing.T) {
	h := newHost(1)
	m := h.m
	m.QBits = make(party.Bits, party.QBitBytes)
	m.Roster = make([]party.Player, 50)
	for i := range m.Roster {
		m.Roster[i] = party.Player{RosterID: i, Face: i % 28}
	}
	m.Players[0].RosterID = 0
	m.Roster[9].Conditions[party.CondDead] = 1
	i32 := func(v int32) []byte { return []byte{byte(v), byte(v >> 8), byte(v >> 16), byte(v >> 24)} }
	vm := &VM{Host: h, Map: script(t,
		rec(1, 0, OpAdd, varArgs(VarInParty, 3)...),
		rec(1, 1, OpCompare, varArgs(VarInParty, 3, 3)...),
		rec(1, 2, OpExit),
		rec(1, 3, OpSet, varArgs(mapVar(0), 1)...),
		rec(1, 4, OpIsPlayerInParty, append(i32(9), 6)...), // dead: no jump
		rec(1, 5, OpIsPlayerInParty, append(i32(8), 7)...), // can act, not in the party
		rec(1, 6, OpExit),
		rec(1, 7, OpSet, varArgs(mapVar(1), 1)...),
	)}
	vm.Run(Source{}, 1, 0, true)
	if len(m.Players) != 2 || m.Players[1].RosterID != 3 || m.Players[1].Face != 3 || !m.QBits.Get(403) {
		t.Fatalf("after Add: %+v", m.Players)
	}
	if h.vars[0] != 1 || h.vars[1] != 1 {
		t.Errorf("Compare/IsPlayerInParty: vars %v", h.vars[:2])
	}
	// Full party: "Party is full!" as the reply, the quest bit still set.
	for _, id := range []int{4, 5, 6} {
		m.AddRoster(id)
	}
	if m.History[4] != m.Time {
		t.Error("roster 4 joining stamps history 4")
	}
	h.reply = nil
	vm = &VM{Host: h, Map: script(t, rec(2, 0, OpAdd, varArgs(VarInParty, 7)...))}
	vm.Run(Source{}, 2, 0, true)
	if len(m.Players) != 5 || len(h.reply) != 1 || !m.QBits.Get(407) {
		t.Errorf("full party: %d members, replies %v", len(m.Players), h.reply)
	}
	// Subtract takes a party slot: slot 1 (roster 3) goes back to the roster.
	m.Players[1].Name = "changed"
	vm = &VM{Host: h, Map: script(t, rec(3, 0, OpSubtract, varArgs(VarInParty, 1)...))}
	vm.Run(Source{}, 3, 0, true)
	ids := []int{}
	for _, p := range m.Players {
		ids = append(ids, p.RosterID)
	}
	if !slices.Equal(ids, []int{0, 4, 5, 6}) || m.Roster[3].Name != "changed" {
		t.Errorf("after Subtract: roster ids %v, roster 3 %+v", ids, m.Roster[3])
	}
	if got := m.AddRoster(4); got != -1 {
		t.Errorf("AddRoster of a member = %d, want -1", got)
	}
	m.TurnBased = true
	if got := m.AddRoster(3); got != -2 {
		t.Errorf("AddRoster in turn-based mode = %d, want -2", got)
	}
}
