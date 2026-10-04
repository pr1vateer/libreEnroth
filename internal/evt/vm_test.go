package evt

import (
	"encoding/binary"
	"fmt"
	"slices"
	"testing"

	"libre-enroth/internal/game/clock"
	"libre-enroth/internal/game/npc"
	"libre-enroth/internal/game/party"
)

// fakeHost records what the VM asks of the world.
type fakeHost struct {
	m      *party.Members
	ctx    *party.Ctx
	vars   [200]byte
	loc    [0x28]byte
	strs   []string
	status []string
	doors  [][2]int
	moves  []string
	nothin int
	stubs  []Op
	npcs   *npc.State
	houses []int
	speak  [][2]int // npc, canShow
	susp   []Suspension
	trans  []Transition
	reply  []string
	notes  []string
}

func newHost(members int) *fakeHost {
	m := &party.Members{}
	for range members {
		m.Players = append(m.Players, party.Player{})
	}
	return &fakeHost{m: m, ctx: &party.Ctx{Rand: party.NewRand(1)}}
}

func (h *fakeHost) Members() *party.Members             { return h.m }
func (h *fakeHost) Ctx() *party.Ctx                     { return h.ctx }
func (h *fakeHost) MapVars() *[200]byte                 { return &h.vars }
func (h *fakeHost) Location() []byte                    { return h.loc[:] }
func (h *fakeHost) Str(i int) string                    { return h.strs[i] }
func (h *fakeHost) NPCText(int) string                  { return "" }
func (h *fakeHost) Global(int) string                   { return "%lu" }
func (h *fakeHost) NothingHere()                        { h.nothin++ }
func (h *fakeHost) Status(text string, _ int)           { h.status = append(h.status, text) }
func (h *fakeHost) Message(string)                      {}
func (h *fakeHost) Reply(s string)                      { h.reply = append(h.reply, s) }
func (h *fakeHost) Teleport(x, y, z, d, l, v int32)     {}
func (h *fakeHost) SetDoor(id, action int)              { h.doors = append(h.doors, [2]int{id, action}) }
func (h *fakeHost) StopDoor(int)                        {}
func (h *fakeHost) SetTexture(int32, string)            {}
func (h *fakeHost) SetSprite(int32, bool, string)       {}
func (h *fakeHost) SetFacesBit(int32, uint32, bool)     {}
func (h *fakeHost) SetLight(int32, bool)                {}
func (h *fakeHost) ChangeEvent(int, int32)              {}
func (h *fakeHost) ToggleChestFlag(int32, uint16, bool) {}
func (h *fakeHost) PartyBuff(int) bool                  { return false }
func (h *fakeHost) Flying() bool                        { return false }
func (h *fakeHost) QuestText(int) bool                  { return false }
func (h *fakeHost) AutonoteText(int) bool               { return false }
func (h *fakeHost) Stub(op Op, _ Record, _ Var)         { h.stubs = append(h.stubs, op) }
func (h *fakeHost) NPCs() *npc.State                    { return h.npcs }
func (h *fakeHost) NPCChanged(op Op, id int)            { h.notes = append(h.notes, fmt.Sprint(op, id)) }
func (h *fakeHost) SpeakInHouse(house int)              { h.houses = append(h.houses, house) }
func (h *fakeHost) Suspend(s Suspension)                { h.susp = append(h.susp, s) }
func (h *fakeHost) Transition(t Transition)             { h.trans = append(h.trans, t) }
func (h *fakeHost) SpeakNPC(id int, canShow bool) {
	c := 0
	if canShow {
		c = 1
	}
	h.speak = append(h.speak, [2]int{id, c})
}
func (h *fakeHost) MoveToMap(name string, x, y, z, d, l, v int32) {
	h.moves = append(h.moves, name)
}

// rec builds a record: len, u16 id, step, op, args.
func rec(id, step int, op Op, args ...byte) []byte {
	b := []byte{byte(4 + len(args)), 0, 0, byte(step), byte(op)}
	binary.LittleEndian.PutUint16(b[1:], uint16(id))
	return append(b, args...)
}

// varArgs are the arguments of Compare/Add/Subtract/Set: u16 var, u32 value (and the
// jump target of Compare).
func varArgs(v Var, value uint32, jmp ...byte) []byte {
	b := binary.LittleEndian.AppendUint16(nil, uint16(v))
	b = binary.LittleEndian.AppendUint32(b, value)
	return append(b, jmp...)
}

func script(t *testing.T, recs ...[]byte) *Script {
	t.Helper()
	s, err := Parse(slices.Concat(recs...), MapMaxBytes)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func mapVar(i int) Var { return VarMapFirst + Var(i) }

// TestJumpRescans: a jump restarts the scan from the first record at its target, so
// steps stored before the jump run again, and a step the jump skips never runs.
//
// mm8: 0x4446bd (LAB_00444978: idx = -1, step = target - 1)
func TestJumpRescans(t *testing.T) {
	h := newHost(1)
	vm := &VM{Host: h, Map: script(t,
		rec(1, 0, OpJmp, 2),
		rec(1, 1, OpSet, varArgs(mapVar(0), 1)...), // skipped
		rec(1, 2, OpAdd, varArgs(mapVar(1), 1)...),
		rec(1, 3, OpCompare, varArgs(mapVar(1), 3, 5)...),
		rec(1, 4, OpJmp, 2),
		rec(1, 5, OpAdd, varArgs(mapVar(2), 1)...),
		rec(1, 6, OpExit),
		rec(1, 7, OpSet, varArgs(mapVar(3), 1)...), // after Exit
	)}
	vm.Run(Source{}, 1, 0, true)
	if got := h.vars[:4]; !slices.Equal(got, []byte{0, 3, 1, 0}) {
		t.Errorf("map vars %v, want [0 3 1 0]", got)
	}
	// The records of a step may come after later steps in the file: the scan only goes
	// forward, so a step stored too early is missed.
	h = newHost(1)
	vm = &VM{Host: h, Map: script(t,
		rec(2, 1, OpSet, varArgs(mapVar(0), 1)...),
		rec(2, 0, OpSet, varArgs(mapVar(1), 1)...),
		rec(2, 2, OpSet, varArgs(mapVar(2), 1)...),
	)}
	vm.Run(Source{}, 2, 0, true)
	if got := h.vars[:3]; !slices.Equal(got, []byte{0, 1, 0}) {
		t.Errorf("out of order: map vars %v, want [0 1 0]", got)
	}
	// Starting at a later step (a resumed event, a trigger's step).
	h = newHost(1)
	vm = &VM{Host: h, Map: script(t,
		rec(3, 0, OpSet, varArgs(mapVar(0), 1)...),
		rec(3, 1, OpSet, varArgs(mapVar(1), 1)...),
	)}
	vm.Run(Source{}, 3, 1, true)
	if got := h.vars[:2]; !slices.Equal(got, []byte{0, 1}) {
		t.Errorf("from step 1: map vars %v, want [0 1]", got)
	}
}

// TestHintKeepsStep: Hint and LocationName share their step with the next record, as
// the shipped scripts have them ("1 0 Hint", "1 0 OnMapReload", "1 1 ...").
func TestHintKeepsStep(t *testing.T) {
	h := newHost(1)
	vm := &VM{Host: h, Map: script(t,
		rec(1, 0, OpHint, 2),
		rec(1, 0, OpLocationName, 2),
		rec(1, 0, OpSet, varArgs(mapVar(0), 7)...),
		rec(1, 1, OpSet, varArgs(mapVar(1), 8)...),
	)}
	vm.Run(Source{}, 1, 0, true)
	if got := h.vars[:2]; !slices.Equal(got, []byte{7, 8}) {
		t.Errorf("map vars %v, want [7 8]", got)
	}
	// An opcode with no case (a trigger in a normal run) takes its step.
	h = newHost(1)
	vm = &VM{Host: h, Map: script(t,
		rec(1, 0, OpOnMapReload, 0),
		rec(1, 0, OpSet, varArgs(mapVar(0), 7)...),
		rec(1, 1, OpSet, varArgs(mapVar(1), 8)...),
	)}
	vm.Run(Source{}, 1, 0, true)
	if got := h.vars[:2]; !slices.Equal(got, []byte{0, 8}) {
		t.Errorf("trigger: map vars %v, want [0 8]", got)
	}
}

// TestSelectors: Subtract of a condition clears it for the members the selector picks:
// a slot, everyone (5), the selected member (7, the default when one is selected), or
// for any other value the member the last selector record picked.
//
// mm8: 0x4446bd (local_530: 7 if g_selectedPlayer else 6; cases 0xe, 0x10..0x12, 0x23)
func TestSelectors(t *testing.T) {
	const cursed = VarCondFirst
	clearCurse := rec(1, 1, OpSubtract, varArgs(cursed, 0)...)
	for _, c := range []struct {
		name     string
		selected int
		recs     [][]byte
		want     []bool // cursed after
	}{
		{"slot 2", 0, [][]byte{rec(1, 0, OpForPartyMember, 2), clearCurse}, []bool{true, true, false, true}},
		{"all", 0, [][]byte{rec(1, 0, OpForPartyMember, 5), clearCurse}, []bool{false, false, false, false}},
		{"selected", 4, [][]byte{rec(1, 0, OpForPartyMember, 7), clearCurse}, []bool{true, true, true, false}},
		{"default selected", 2, [][]byte{rec(1, 0, OpSetSnow), clearCurse}, []bool{true, false, true, true}},
		{"empty slot", 0, [][]byte{rec(1, 0, OpForPartyMember, 4), clearCurse}, []bool{true, true, true, true}},
		{"previous", 0, [][]byte{
			rec(1, 0, OpForPartyMember, 1), rec(1, 1, OpSubtract, varArgs(mapVar(0), 0)...),
			rec(1, 2, OpForPartyMember, 9), rec(1, 3, OpSubtract, varArgs(cursed, 0)...),
		}, []bool{true, false, true, true}},
	} {
		t.Run(c.name, func(t *testing.T) {
			h := newHost(4)
			h.m.Selected = c.selected
			for i := range h.m.Players {
				h.m.Players[i].Conditions[0] = 1
			}
			vm := &VM{Host: h, Map: script(t, c.recs...)}
			vm.Run(Source{}, 1, 0, true)
			for i, want := range c.want {
				if got := h.m.Players[i].Conditions[0] != 0; got != want {
					t.Errorf("member %d cursed %v, want %v", i, got, want)
				}
			}
		})
	}
	// Compare over everyone succeeds when any member passes.
	h := newHost(3)
	h.m.Players[1].Conditions[0] = 1
	vm := &VM{Host: h, Map: script(t,
		rec(1, 0, OpForPartyMember, 5),
		rec(1, 1, OpCompare, varArgs(cursed, 0, 3)...),
		rec(1, 2, OpExit),
		rec(1, 3, OpSet, varArgs(mapVar(0), 1)...),
	)}
	vm.Run(Source{}, 1, 0, true)
	if h.vars[0] != 1 {
		t.Error("Compare over the party: no member passed")
	}
}

// TestSubtractAborts: Subtract of gold or bank gold the party lacks ends the event
// (the next step does not run) and takes nothing; enough gold is taken and goes on.
//
// mm8: 0x44a0fe (Evt_Sub: abort flag 0x5ccc14)
func TestSubtractAborts(t *testing.T) {
	for _, c := range []struct {
		v          Var
		have, cost uint32
		aborted    bool
	}{
		{VarGold, 10, 20, true},
		{VarGold, 20, 20, false},
		{VarBank, 5, 6, true},
		{VarBank, 6, 6, false},
	} {
		h := newHost(1)
		h.m.Gold, h.m.Bank = int32(c.have), int32(c.have)
		vm := &VM{Host: h, Map: script(t,
			rec(1, 0, OpSubtract, varArgs(c.v, c.cost)...),
			rec(1, 1, OpSet, varArgs(mapVar(0), 1)...),
		)}
		res := vm.Run(Source{}, 1, 0, true)
		have := h.m.Gold
		if c.v == VarBank {
			have = h.m.Bank
		}
		wantHave, wantVar := int32(c.have-c.cost), byte(1)
		if c.aborted {
			wantHave, wantVar = int32(c.have), 0
		}
		if res.Aborted != c.aborted || have != wantHave || h.vars[0] != wantVar {
			t.Errorf("%#x %d-%d: aborted %v left %d var %d; want %v %d %d", int(c.v), c.have, c.cost,
				res.Aborted, have, h.vars[0], c.aborted, wantHave, wantVar)
		}
	}
}

// TestRandomGoTo: the target is rand() % n of the first n non-zero steps.
//
// mm8: 0x4446bd (case 0x19)
func TestRandomGoTo(t *testing.T) {
	seen := map[byte]int{}
	ref := party.NewRand(7)
	h := newHost(1)
	h.m.Selected = 1 // else each Set picks a random member with rand() too
	h.ctx.Rand = party.NewRand(7)
	vm := &VM{Host: h, Map: script(t,
		rec(1, 0, OpRandomGoTo, 2, 3, 4, 0, 0, 0),
		rec(1, 1, OpExit),
		rec(1, 2, OpSet, varArgs(mapVar(0), 2)...),
		rec(1, 3, OpExit),
		rec(1, 3, OpSet, varArgs(mapVar(0), 3)...),
		rec(1, 4, OpSet, varArgs(mapVar(0), 4)...),
		rec(1, 5, OpExit),
	)}
	for range 60 {
		h.vars[0] = 0
		vm.Run(Source{}, 1, 0, true)
		want := []byte{2, 0, 4}[ref.Int()%3] // step 3 exits before its Set
		if h.vars[0] != want {
			t.Fatalf("var %d, want %d", h.vars[0], want)
		}
		seen[h.vars[0]]++
	}
	if len(seen) != 3 {
		t.Errorf("outcomes %v, want all three", seen)
	}
	// No non-zero target: the step is used up.
	h = newHost(1)
	vm = &VM{Host: h, Map: script(t,
		rec(1, 0, OpRandomGoTo, 0, 0, 0, 0, 0, 0),
		rec(1, 1, OpSet, varArgs(mapVar(0), 1)...),
	)}
	vm.Run(Source{}, 1, 0, true)
	if h.vars[0] != 1 {
		t.Error("RandomGoTo without targets did not go on")
	}
}

// TestEventZero: event 0 says "Nothing here".
func TestEventZero(t *testing.T) {
	h := newHost(1)
	vm := &VM{Host: h, Map: script(t, rec(1, 0, OpExit))}
	vm.Run(Source{}, 0, 0, true)
	if h.nothin != 1 {
		t.Errorf("NothingHere %d times", h.nothin)
	}
}

// TestMoveToMap: a teleport within the map is not a map change; another map is, and
// the map's OnMapLeave events run after the event.
func TestMoveToMap(t *testing.T) {
	move := func(name string) []byte {
		b := make([]byte, 0x1b)
		binary.LittleEndian.PutUint32(b[0:], 100) // x
		return append(append(b, name...), 0)
	}
	h := newHost(1)
	vm := &VM{Host: h, Map: script(t,
		rec(1, 0, OpMoveToMap, move("0")...),
		rec(2, 0, OpMoveToMap, move("out01.odm")...),
		rec(2, 1, OpSet, varArgs(mapVar(0), 1)...),
		rec(3, 0, OpOnMapLeave),
		rec(3, 1, OpAdd, varArgs(mapVar(1), 1)...),
	)}
	if res := vm.Run(Source{}, 1, 0, true); res.Moved || len(h.moves) != 0 {
		t.Errorf("teleport: %+v %v", res, h.moves)
	}
	res := vm.Run(Source{}, 2, 0, true)
	if !res.Moved || !slices.Equal(h.moves, []string{"out01.odm"}) {
		t.Errorf("travel: %+v %v", res, h.moves)
	}
	if h.vars[0] != 1 || h.vars[1] != 1 {
		t.Errorf("map vars %v: the event goes on and OnMapLeave runs once", h.vars[:2])
	}
}

// TestMoveToMapAsks: a MoveToMap with a house or an exit picture opens the transition
// dialogue instead and ends the event; it would resume at the next step.
//
// mm8: 0x4446bd (case 6: Evt_TransitionDialog, g_evtResumeStep = step + 1)
func TestMoveToMapAsks(t *testing.T) {
	move := func(house uint16, pic byte) []byte {
		b := make([]byte, 0x1b)
		binary.LittleEndian.PutUint32(b[0:], 100) // x
		binary.LittleEndian.PutUint16(b[0x18:], house)
		b[0x1a] = pic
		return append(append(b, "d05.blv"...), 0)
	}
	h := newHost(1)
	vm := &VM{Host: h, Map: script(t,
		rec(1, 0, OpMoveToMap, move(191, 1)...),
		rec(1, 1, OpSet, varArgs(mapVar(0), 1)...),
		rec(2, 3, OpMoveToMap, move(0, 2)...),
	)}
	res := vm.Run(Source{}, 1, 0, true)
	if res.Moved || len(h.moves) != 0 || h.vars[0] != 0 || len(h.trans) != 1 {
		t.Fatalf("asked: %+v moves %v var %d", res, h.moves, h.vars[0])
	}
	tr := h.trans[0]
	if tr.House != 191 || tr.Pic != 1 || tr.Map != "d05.blv" || tr.X != 100 || !tr.Moves() ||
		tr.Resume != (Suspension{ID: 1, Step: 1, Op: OpMoveToMap}) {
		t.Errorf("transition %+v", tr)
	}
	vm.Run(Source{}, 2, 3, true)
	if tr := h.trans[1]; tr.House != 0 || tr.Pic != 2 || tr.Resume.Step != 4 {
		t.Errorf("exit picture: %+v", tr)
	}
	vm.Resume(h.trans[0].Resume, "")
	if h.vars[0] != 1 {
		t.Error("resumed event did not go on")
	}
}

// timerRec is an OnTimer/OnLongTimer record: year/month/week/hour/min/sec flags and a
// u16 interval in 30-second steps.
func timerRec(id int, op Op, flags [6]byte, interval uint16) []byte {
	return rec(id, 0, op, append(flags[:], byte(interval), byte(interval>>8))...)
}

// TestTimers: a countdown timer fires every interval * 30 game seconds (and on the
// game's first scan, which counts from time 0); a daily one at its time of day. A long
// timer with no interval fires at once on the first visit, or when a period has passed
// since the last one; the original tests whole days without the flags, so a weekly
// one also fires after a day.
//
// mm8: 0x441caf (Evt_InitTimers), 0x446fb5 (Evt_TimerScan)
func TestTimers(t *testing.T) {
	const step = 128 // ticks per 30 game seconds
	s := script(t,
		timerRec(1, OpOnTimer, [6]byte{}, 4), // every 2 minutes
		timerRec(2, OpOnTimer, [6]byte{0, 0, 0, 12, 30, 0}, 0),
		timerRec(3, OpOnLongTimer, [6]byte{0, 0, 1, 0, 0, 0}, 0), // weekly
	)
	start := clock.Time(9 * 3600 * 128 / 30) // 9:00 on day 1
	fired := map[int][]clock.Time{}
	tm := InitTimers(s, start, 0)
	if len(tm.List) != 3 {
		t.Fatalf("%d timers", len(tm.List))
	}
	var last clock.Time
	var first []int
	tm.Scan(start, &last, func(id, _ int) { first = append(first, id) })
	if !slices.Equal(first, []int{1, 3}) || last != start {
		t.Errorf("first scan fired %v, last %d; want [1 3] %d", first, last, start)
	}
	tm.Scan(start+step-1, &last, func(int, int) { t.Error("fired within 30 seconds") })
	for now := start + step; now <= start+clock.Time(4*3600*128/30); now += step {
		tm.Scan(now, &last, func(id, _ int) { fired[id] = append(fired[id], now) })
	}
	if n := len(fired[1]); n != 4*30 {
		t.Errorf("countdown fired %d times in 4 hours, want 120", n)
	}
	if f := fired[1]; len(f) > 1 && f[1]-f[0] != 4*step {
		t.Errorf("countdown period %d ticks", f[1]-f[0])
	}
	noon := clock.Time(12*3600+30*60) * 128 / 30
	if f := fired[2]; len(f) != 1 || f[0] < noon || f[0] >= noon+step {
		t.Errorf("daily timer fired at %v, want once at 12:30 (%d)", f, noon)
	}
	if len(fired[3]) != 0 {
		t.Errorf("weekly long timer fired again at %v", fired[3])
	}
	// Coming back eight days later: the long timer fires at once.
	week := clock.Time(8*24*3600) * 128 / 30
	tm = InitTimers(s, start+week, start)
	var ids []int
	last = start + week - step
	tm.Scan(start+week, &last, func(id, _ int) { ids = append(ids, id) })
	if !slices.Contains(ids, 3) {
		t.Errorf("weekly long timer after 8 days: fired %v", ids)
	}
	// ... and after a day (the quirk), but not within one.
	for _, c := range []struct {
		after clock.Time
		fires bool
	}{{week / 8, true}, {week / 16, false}} {
		tm = InitTimers(s, start+c.after, start)
		ids = nil
		last = start + c.after - step
		tm.Scan(start+c.after, &last, func(id, _ int) { ids = append(ids, id) })
		if slices.Contains(ids, 3) != c.fires {
			t.Errorf("weekly long timer %d ticks after the visit: fired %v", c.after, ids)
		}
	}
}
