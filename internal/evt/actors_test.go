package evt

import (
	"encoding/binary"
	"slices"
	"testing"

	"libre-enroth/internal/game/party/partytest"
)

// arg32 is a little-endian int32 argument.
func arg32(v int32) []byte { return binary.LittleEndian.AppendUint32(nil, uint32(v)) }

func cat(parts ...[]byte) []byte {
	var b []byte
	for _, p := range parts {
		b = append(b, p...)
	}
	return b
}

// The actor opcodes hand their records' fields to the host: SummonMonsters (+5 type,
// +6 level, +7 count, +8 xyz, +0x14 group, +0x18 name), the flags (+5 actor or group,
// +9 bits, +0xd on), groups, allies, items; IsActorKilled jumps to +0xc when the kill
// check holds; SummonItem drops its item instead of putting it on the mouse.
//
// mm8: 0x4446bd (cases 0x13, 0x18, 0x22, 0x30, 0x31, 0x33, 0x36, 0x37, 0x39, 0x3c)
func TestActorOpcodes(t *testing.T) {
	h := newHost(1)
	h.ctx.Items = partytest.Items()
	vm := &VM{Host: h, Map: script(t,
		rec(1, 0, OpSummonMonsters, cat([]byte{2, 1, 3}, arg32(-100), arg32(200), arg32(30), arg32(7), arg32(9))...),
		rec(1, 1, OpToggleActorFlag, cat(arg32(4), arg32(0x10000), []byte{0})...),
		rec(1, 2, OpToggleGroupFlag, cat(arg32(5), arg32(0x80000), []byte{1})...),
		rec(1, 3, OpSetActorGroup, cat(arg32(2), arg32(11))...),
		rec(1, 4, OpChangeGroup, cat(arg32(11), arg32(12))...),
		rec(1, 5, OpChangeGroupAlly, cat(arg32(12), arg32(9999))...),
		rec(1, 6, OpSetActorItem, cat(arg32(3), arg32(0x28d), []byte{1})...),
		rec(1, 7, OpNPCSetItem, cat(arg32(40), arg32(0x28e), []byte{0})...),
		rec(1, 8, OpSummonItem, cat(arg32(partytest.Sword), arg32(1), arg32(2), arg32(3), arg32(150), []byte{2, 1})...),
		rec(1, 9, OpIsActorKilled, cat([]byte{1}, arg32(12), []byte{0, 1, 11})...),
		rec(1, 10, OpExit),
		rec(1, 11, OpStatusText, arg32(0)...),
		rec(1, 12, OpExit),
	)}
	h.strs = []string{"killed"}
	vm.Run(Source{Kind: SourceMap}, 1, 0, true)
	want := []string{
		"summon {Type:2 Level:1 Count:3 X:-100 Y:200 Z:30 Group:7 UniqueName:9}",
		"flag 4 0x10000 false",
		"groupflag 5 0x80000 true",
		"group 2 11",
		"changegroup 11 12",
		"ally 12 9999",
		"item 3 653 true",
		"npcitem 40 654 false",
		"drop 1 at 1,2,3 speed 150 x2 true",
		"killed 1 12 0 true",
	}
	if !slices.Equal(h.actors, want) {
		t.Errorf("calls:\n%q\nwant\n%q", h.actors, want)
	}
	if len(h.status) != 0 || len(h.stubs) != 0 || h.m.MouseItem.Number != 0 {
		t.Errorf("status %q stubs %v mouse %d", h.status, h.stubs, h.m.MouseItem.Number)
	}
	if d := h.drops; len(d) != 1 || d[0].Flags&1 == 0 {
		t.Errorf("dropped %+v", d)
	}
	h.killed, h.actors = true, nil
	vm.Run(Source{Kind: SourceMap}, 1, 9, true)
	if len(h.status) != 1 || h.status[0] != "killed" {
		t.Errorf("IsActorKilled did not jump: %q", h.status)
	}
}

// CanShowTopicIsActorKilled (0x34) jumps like a compare when the kill check holds, and
// is not itself a visibility record.
//
// mm8: 0x4444fe
func TestCanShowTopicKilled(t *testing.T) {
	h := newHost(1)
	vm := &VM{Host: h, Global: script(t,
		rec(7, 0, OpCanShowTopicKill, cat([]byte{0}, arg32(0), []byte{0, 0, 2})...),
		rec(7, 1, OpSetCanShowDialog, 0),
		rec(7, 2, OpEndCanShowDialog),
		rec(8, 0, OpCanShowTopicKill, cat([]byte{0}, arg32(0), []byte{0, 0, 2})...),
	)}
	if got := vm.CanShowTopic(7); got != 0 {
		t.Errorf("not killed: %d", got)
	}
	h.killed = true
	if got := vm.CanShowTopic(7); got != 2 {
		t.Errorf("killed: %d (the jump skips the only SetCanShowDialog)", got)
	}
	if got := vm.CanShowTopic(8); got != 2 {
		t.Errorf("alone: %d", got)
	}
}
