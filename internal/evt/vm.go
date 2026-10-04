package evt

import (
	"libre-enroth/internal/game/npc"
	"libre-enroth/internal/game/party"
)

// SourceKind says which script an event comes from and who started it.
type SourceKind int

const (
	SourceMap        SourceKind = iota // the map's .evt (faces, decorations, timers)
	SourceGlobal                       // global.evt from an NPC or house dialogue
	SourceDecoration                   // global.evt from an interactive decoration
)

// Source is the event source (0x5db690: 0 map, 1 global, else the decoration).
type Source struct {
	Kind SourceKind
	Dec  int // SourceDecoration: the level decoration's index
}

// Host is what the VM acts on: the party, the map and the screen. internal/game/world
// implements it.
type Host interface {
	// Members is the party; the VM changes its members, clock-stamped variables, money
	// and bits directly. Ctx has its random generator and tables.
	Members() *party.Members
	Ctx() *party.Ctx
	// MapVars are the map's 200 event variables (vars 0x7d..0xe0), nil without a map.
	MapVars() *[200]byte
	// Location is the map's 0x28-byte location header (+8 reputation, +0xc var 0x133),
	// nil without a map.
	Location() []byte
	// Str is string i of the map's .str, NPCText of npctext.txt (M6: ""), Global of
	// global.txt.
	Str(i int) string
	NPCText(i int) string
	Global(i int) string
	// NothingHere shows "Nothing here" unless a message is up (event 0).
	NothingHere()
	// Status shows a timed message; Message is the map's message buffer (0x5c678c, the
	// text box M6 draws); Reply is the NPC dialogue's reply text (M6).
	Status(text string, seconds int)
	Message(text string)
	Reply(text string)

	// Teleport moves the party on this map: dir -1 keeps the heading.
	Teleport(x, y, z, dir, look, vz int32)
	// MoveToMap travels to another map; the party arrives at its Party Start, with the
	// non-zero coordinates overriding it.
	MoveToMap(name string, x, y, z, dir, look, vz int32)
	// SetDoor starts door id moving: action 0 opens, 1 closes, 2 toggles.
	SetDoor(id, action int)
	StopDoor(id int)
	SetTexture(cog int32, name string)
	SetSprite(cog int32, visible bool, name string)
	SetFacesBit(cog int32, bits uint32, on bool)
	SetLight(id int32, on bool)
	// ChangeEvent gives interactive decoration dec a new event (0 hides it).
	ChangeEvent(dec int, event int32)
	ToggleChestFlag(chest int32, bit uint16, on bool)
	// PartyBuff reports party buff i active (vars 0xf2 fly 7, 0x13c invisibility 11;
	// M9). Flying is Party +0x714.
	PartyBuff(i int) bool
	Flying() bool
	// QuestText and AutonoteText report a quest bit / autonote with text (M6): newly set
	// ones make a member speak.
	QuestText(n int) bool
	AutonoteText(n int) bool
	// NPCs is the game's NPC table, which SetNPCTopic, MoveNPC, SetNPCGreeting and
	// SetNPCGroupNews change (nil: none loaded, they do nothing).
	NPCs() *npc.State
	// NPCChanged tells the open dialogue that op (SetNPCTopic, MoveNPC) changed NPC id.
	NPCChanged(op Op, id int)
	// SpeakInHouse opens house's dialogue; the event goes on.
	SpeakInHouse(house int)
	// SpeakNPC opens NPC id's dialogue; with canShow false (OnMapReload) the game keeps
	// it for when the map is up (g_evtDeferredNpc 0x5db82c).
	SpeakNPC(id int, canShow bool)
	// Suspend parks the event until the player answers (PressAnyKey, InputString); the
	// UI resumes it with VM.Resume.
	Suspend(s Suspension)
	// Stub is an opcode of a later milestone (op.Milestone()), or a player variable the
	// stats do not have yet (var >= 0).
	Stub(op Op, r Record, v Var)
}

// Suspension is an event waiting for the player.
//
// mm8: 0x44328b (Evt_Suspend: g_evtResumeId/Step/Source 0x5db688/0x5db68c/0x5a52c8)
type Suspension struct {
	Src      Source
	ID, Step int // where it goes on
	Op       Op  // OpPressAnyKey or OpInputString
	// Question is InputString's question (NPCText), which the game puts in the status
	// line buffer g_statusTimed 0x5db694.
	Question string
}

// VM runs events. Map is the current map's script (nil: none), Global global.evt.
type VM struct {
	Map, Global *Script
	Host        Host
	// OnMapLeave runs the map's OnMapLeave events; Run calls it after an event that
	// left for another map. Nil: VM.MapLeave.
	OnMapLeave func()

	start  int    // the running event's start step (g_evtStartStep 0x5ac150)
	answer string // InputString's answer, typed into g_statusTimed
}

// Result is how an event ended.
type Result struct {
	// Moved reports a MoveToMap to another map.
	Moved bool
	// Aborted reports a Subtract that could not pay (gold or bank).
	Aborted bool
	// Suspended is set when the event waits for the player (it went to Host.Suspend).
	Suspended *Suspension
}

// Selector values (ForPartyMember and the player byte of some records).
const (
	selAll      = 5
	selRandom   = 6
	selSelected = 7
)

// run is one event's state.
type run struct {
	vm      *VM
	h       Host
	m       *party.Members
	src     Source
	canShow bool
	sel     int
	prev    int // player of the last Compare/Add/Subtract/Set selector, -1 none
	abort   bool
	moved   bool
	susp    *Suspension
}

// Run runs event id from step: it scans the source's records in file order for the
// current step of the event, runs it and moves to the next step; a jump restarts the
// scan from the top at its target. Hint and LocationName do not use up a step. An
// opcode without a case does nothing but takes its step. The event ends with the
// records, at Exit, or when a Subtract could not pay. canShow is false for
// OnMapReload (messages are dropped, SpeakNPC deferred). Event 0 says "Nothing here".
//
// mm8: 0x4446bd (Evt_Process: __fastcall(id, targetPid), canShow on the stack, the start
// step in 0x5ac150, the source in 0x5db690)
func (vm *VM) Run(src Source, id, step int, canShow bool) Result {
	h := vm.Host
	if id == 0 {
		h.NothingHere() // Status_NothingHere 0x44a8a2
		return Result{}
	}
	s := vm.Map
	if src.Kind != SourceMap {
		s = vm.Global
	}
	if s == nil {
		return Result{}
	}
	m := h.Members()
	r := &run{vm: vm, h: h, m: m, src: src, canShow: canShow, prev: -1, sel: selRandom}
	defer func(s int) { vm.start = s }(vm.start)
	vm.start = step
	if m.Selected != 0 {
		r.sel = selSelected
	}
	cur := step
	recs := s.Records
	for i := 0; i < len(recs) && !r.abort; i++ {
		rec := recs[i]
		if rec.ID() != id || rec.Step() != cur {
			continue
		}
		next, exit := r.exec(rec)
		if exit {
			break
		}
		switch {
		case next >= 0: // a jump: rescan from the top
			i, cur = -1, next
		case next == noJump:
			cur++
		}
	}
	if r.moved {
		if vm.OnMapLeave != nil {
			vm.OnMapLeave()
		} else {
			vm.MapLeave()
		}
	}
	return Result{Moved: r.moved, Aborted: r.abort, Suspended: r.susp}
}

// Resume goes on with a suspended event; answer is what the player typed for an
// InputString (compared with the map's .str answers, case-insensitively).
//
// mm8: 0x4433fd (Evt_Resume), 0x4433b0 (Evt_ResumeUnpause)
func (vm *VM) Resume(s Suspension, answer string) Result {
	vm.answer = answer
	return vm.Run(s.Src, s.ID, s.Step, true)
}

// suspend parks the event (Evt_Suspend) at step.
func (r *run) suspend(op Op, id, step int, question string) {
	r.susp = &Suspension{Src: r.src, ID: id, Step: step, Op: op, Question: question}
	r.h.Suspend(*r.susp)
}

// CanShowTopic decides whether a dialogue shows the topic of global event id, from its
// visibility records: OnCanShowDialogItemCmp (0x2c) jumps when a member passes the
// compare, SetCanShowDialogItem (0x2e) sets the answer, CanShowTopicIsActorKilled
// (0x34) jumps when the actors are dead (M8: never), Exit or EndCanShowDialogItem end
// it. 0: hidden, 1: shown, 2: the event has none of these records (shown).
//
// mm8: 0x4444fe (Evt_CanShowTopic; it starts at the running event's start step)
func (vm *VM) CanShowTopic(id int) int {
	if id == 0 {
		return 0
	}
	if vm.Global == nil {
		return 2
	}
	h := vm.Host
	r := &run{vm: vm, h: h, m: h.Members(), src: Source{Kind: SourceGlobal}, prev: -1}
	result, seen := 1, false
	cur := vm.start
	recs := vm.Global.Records
scan:
	for i := 0; i < len(recs); i++ {
		rec := recs[i]
		if rec.ID() != id || rec.Step() != cur {
			continue
		}
		switch rec.Op() {
		case OpExit, OpEndCanShowDialog:
			break scan
		case OpOnCanShowDialog:
			seen = true
			for p := range r.m.Players {
				if r.compare(p, Var(rec.U16(5)), rec.U32(7)) {
					i, cur = -1, rec.U8(0xb)-1
					break
				}
			}
		case OpSetCanShowDialog:
			result = rec.U8(5)
			seen = true
		case OpCanShowTopicKill:
			h.Stub(OpIsActorKilled, rec, -1) // actors are M8's: nobody counts as killed
		}
		cur++
	}
	if !seen {
		return 2
	}
	if result != 0 {
		return 1
	}
	return 0
}

// noJump and stay are exec's step results besides a jump target.
const (
	noJump = -1
	stay   = -2 // Hint, LocationName: the step is not used up
)

// player picks the member a record's selector means: a slot 0..4, random (6), the
// selected one (7, random when none) or, for anything else, the last one picked. -1:
// nobody (an empty slot).
func (r *run) player(sel int) int {
	n := len(r.m.Players)
	switch {
	case sel < 5:
		if sel >= n {
			return -1
		}
		return sel
	case sel == selRandom:
		return r.random()
	case sel == selSelected:
		if r.m.Selected == 0 {
			return r.random()
		}
		return r.m.Selected - 1
	}
	return r.prev
}

func (r *run) random() int {
	n := len(r.m.Players)
	if n == 0 {
		return -1
	}
	return r.h.Ctx().Rand.Int() % n
}

// facePlayers runs fn for the members of ShowFace's and CharacterAnimation's own
// selector byte: a slot, everyone (5), the selected one (7; with nobody selected the
// original reads the roster entry before the first, here nobody) or else a random one.
//
// mm8: 0x4446bd (cases 8, 0x3b)
func (r *run) facePlayers(sel int, fn func(p int)) {
	switch {
	case sel < 5:
		if sel < len(r.m.Players) {
			fn(sel)
		}
	case sel == selAll:
		for i := range r.m.Players {
			fn(i)
		}
	case sel == selSelected:
		if s := r.m.Selected; s > 0 && s <= len(r.m.Players) {
			fn(s - 1)
		}
	default:
		if p := r.random(); p >= 0 {
			fn(p)
		}
	}
}

// exec runs one record. It returns a jump target (>= 0), noJump or stay, and exit when
// the event ends.
func (r *run) exec(rec Record) (next int, exit bool) {
	h := r.h
	next = noJump
	switch op := rec.Op(); op {
	case OpExit:
		return next, true
	case OpHint, OpLocationName:
		return stay, false
	case OpSetSnow: // an empty function in MM8 (0x450f22)
	case OpMoveToMap:
		r.moveToMap(rec)
	case OpShowFace:
		pft := h.Ctx().PFT
		r.facePlayers(rec.U8(5), func(p int) { r.m.Players[p].SetExpression(pft, rec.U8(6), 0) })
	case OpCharacterAnim:
		r.facePlayers(rec.U8(5), func(p int) { r.m.Speak(p, rec.U8(6), h.Ctx()) })
	case OpSetTexture:
		h.SetTexture(rec.I32(5), rec.Str(9))
	case OpSetSprite:
		h.SetSprite(rec.I32(5), rec.U8(9) != 0, rec.Str(10))
	case OpCompare:
		v, val := Var(rec.U16(5)), rec.U32(7)
		ok := false
		if r.sel == selAll {
			for i := range r.m.Players {
				if r.compare(i, v, val) {
					ok = true
					break
				}
			}
		} else {
			r.prev = r.player(r.sel)
			ok = r.compare(r.prev, v, val)
		}
		if ok {
			next = rec.U8(0xb)
		}
	case OpAdd, OpSubtract, OpSet:
		v, val := Var(rec.U16(5)), rec.U32(7)
		f := r.add
		switch op {
		case OpSubtract:
			f = r.sub
		case OpSet:
			f = r.set
		}
		if r.sel == selAll {
			for i := range r.m.Players {
				f(i, v, val)
			}
		} else {
			r.prev = r.player(r.sel)
			f(r.prev, v, val)
		}
	case OpChangeDoorState:
		h.SetDoor(rec.U8(5), rec.U8(6))
	case OpStopDoor:
		h.StopDoor(rec.U8(5))
	case OpSetFacesBit:
		h.SetFacesBit(rec.I32(5), rec.U32(9), rec.U8(0xd) != 0)
	case OpToggleIndoorLight:
		h.SetLight(rec.I32(5), rec.U8(9) != 0)
	case OpRandomGoTo:
		n := 0
		for k := 5; k <= 10; k++ {
			if rec.U8(k) != 0 {
				n++
			}
		}
		if n > 0 {
			next = rec.U8(5 + h.Ctx().Rand.Int()%n)
		}
	case OpForPartyMember:
		r.sel = rec.U8(5)
	case OpJmp:
		next = rec.U8(5)
	case OpStatusText:
		i := int(rec.I32(5))
		switch r.src.Kind {
		case SourceMap:
			if r.canShow {
				h.Status(h.Str(i), 2)
			}
		case SourceGlobal:
			h.Reply(h.NPCText(i))
		default:
			if r.canShow {
				h.Status(h.NPCText(i), 2)
			}
		}
	case OpShowMessage:
		i := int(rec.I32(5))
		if r.src.Kind == SourceMap {
			h.Message(h.Str(i))
		} else {
			h.Reply(h.NPCText(i))
			h.Message("")
		}
	case OpChangeEvent:
		if r.src.Kind == SourceDecoration {
			h.ChangeEvent(r.src.Dec, rec.I32(5))
		}
	case OpCheckSeason:
		if r.m.Calendar.InSeason(rec.U8(5)) {
			next = rec.U8(6)
		}
	case OpToggleChestFlag:
		h.ToggleChestFlag(rec.I32(5), uint16(rec.U16(9)), rec.U8(0xd) != 0)
	case OpIsTotalBounty:
		// mm8: 0x44a981 (unsigned min <= bounty <= max)
		b := uint32(r.m.Bounty)
		if rec.U32(5) <= b && b <= rec.U32(9) {
			next = rec.U8(0xd)
		}
	case OpPressAnyKey:
		// mm8: 0x4446bd (case 0x21: Evt_Suspend(id, step + 1, 0x21), exit)
		r.suspend(op, rec.ID(), rec.Step()+1, "")
		return next, true
	case OpInputString:
		// mm8: 0x4446bd (case 0x1a): the first run (start step 0) puts the question
		// in the status buffer and waits at this very step; the resumed run compares
		// the answer with two strings of the map's .str and jumps on a match.
		if r.vm.start == 0 {
			r.suspend(op, rec.ID(), rec.Step(), h.NPCText(int(rec.I32(5))))
			return next, true
		}
		a := r.vm.answer
		if strEqualASCII(a, h.Str(int(rec.I32(9)))) || strEqualASCII(a, h.Str(int(rec.I32(0xd)))) {
			next = rec.U8(0x11)
		}
	case OpSpeakInHouse:
		// mm8: 0x4446bd (case 2): the house dialogue opens, the event goes on
		if house := rec.U32(5); house != 0 {
			h.SpeakInHouse(int(house))
		}
	case OpSpeakNPC:
		// mm8: 0x4446bd (case 0x16), 0x443b6f (Evt_SpeakNPC)
		h.SpeakNPC(int(rec.I32(5)), r.canShow)
	case OpSetNPCTopic:
		// mm8: 0x4446bd (case 0x27)
		id := int(rec.I32(5))
		if n := h.NPCs(); n != nil {
			n.SetTopic(id, rec.U8(9), rec.I32(0xa))
		}
		h.NPCChanged(op, id)
	case OpMoveNPC:
		// mm8: 0x4446bd (case 0x28)
		id := int(rec.I32(5))
		if n := h.NPCs(); n != nil {
			n.Move(id, rec.I32(9))
		}
		h.NPCChanged(op, id)
	case OpSetNPCGreeting:
		// mm8: 0x4446bd (case 0x32)
		if n := h.NPCs(); n != nil {
			n.SetGreeting(int(rec.I32(5)), rec.I32(9))
		}
	case OpSetNPCGroupNews:
		// mm8: 0x4446bd (case 0x2f)
		if n := h.NPCs(); n != nil {
			n.SetGroupNews(int(rec.I32(5)), int16(rec.U16(9)))
		}
	case OpIsPlayerInParty:
		// mm8: 0x4446bd (case 0x44): jumps when roster character id can act, in the
		// party or not
		if p := r.m.RosterPlayer(int(rec.I32(5))); p != nil && p.CanAct() {
			next = rec.U8(9)
		}
	case OpOpenChest:
		// OpenChest (M7) ends the event when it returns 0 (a trap went off).
		h.Stub(op, rec, -1)
	default:
		if op.Known() && op.Milestone() != "" {
			h.Stub(op, rec, -1)
		}
		// Triggers (OnTimer, OnMapReload, ...) and opcodes without a case: nothing.
	}
	return next, false
}

// strEqualASCII is _stricmp == 0.
func strEqualASCII(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		x, y := a[i], b[i]
		if 'A' <= x && x <= 'Z' {
			x += 'a' - 'A'
		}
		if 'A' <= y && y <= 'Z' {
			y += 'a' - 'A'
		}
		if x != y {
			return false
		}
	}
	return true
}

// moveToMap teleports the party on this map (name "0...") or travels to another. A
// house picture or exit picture asks first in a dialogue (M6); libre-enroth goes
// straight on.
//
// mm8: 0x4446bd (case 6), 0x4425f5 (transition dialogue), 0x447f80 (Evt_Travel)
func (r *run) moveToMap(rec Record) {
	x, y, z := rec.I32(5), rec.I32(9), rec.I32(0xd)
	dir, look, vz := rec.I32(0x11), rec.I32(0x15), rec.I32(0x19)
	if dir != -1 {
		dir &= 0x7ff
	}
	name := rec.Str(0x20)
	if len(name) > 0 && name[0] == '0' {
		if dir|vz|look|z|y|x != 0 {
			r.h.Teleport(x, y, z, dir, look, vz)
		}
		return
	}
	r.h.MoveToMap(name, x, y, z, dir, look, vz)
	r.moved = true
}

// MapLeave runs every OnMapLeave event of the map from the step of its trigger.
//
// mm8: 0x441c68 (Evt_MapLeave)
func (vm *VM) MapLeave() {
	if vm.Map == nil {
		return
	}
	for _, rec := range vm.Map.Records {
		if rec.Op() == OpOnMapLeave {
			vm.Run(Source{Kind: SourceMap}, rec.ID(), rec.Step(), true)
		}
	}
}

// MapReload runs every OnMapReload event from the step of its trigger, with messages
// off (part of Evt_InitTimers).
//
// mm8: 0x441caf (opcode 0x25: Evt_Process(id, 0, canShow 0))
func (vm *VM) MapReload() {
	if vm.Map == nil {
		return
	}
	for _, rec := range vm.Map.Records {
		if rec.Op() == OpOnMapReload {
			vm.Run(Source{Kind: SourceMap}, rec.ID(), rec.Step(), false)
		}
	}
}

// Set is Set of variable v for member p outside an event (the dialogues call Evt_Set
// directly).
//
// mm8: 0x448d4b (Evt_Set)
func (vm *VM) Set(p int, v Var, value uint32) {
	r := &run{vm: vm, h: vm.Host, m: vm.Host.Members(), prev: -1}
	r.set(p, v, value)
}
