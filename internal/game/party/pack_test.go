package party

import (
	"testing"

	"libre-enroth/internal/game/items"
	"libre-enroth/internal/game/party/partytest"
)

// The pack: items go down the columns from the left; removing frees the cells.
//
// mm8: 0x492e59, 0x492b25, 0x492c09, 0x49305c
func TestPack(t *testing.T) {
	tb := partytest.Items()
	var p Player
	if n := p.AddItem(tb, -1, items.Item{Number: partytest.Sword}); n != 1 {
		t.Fatalf("sword slot %d", n)
	}
	if p.Grid[0] != 1 || p.Grid[14] != -1 || p.Grid[28] != -1 || p.Grid[42] != 0 {
		t.Errorf("sword cells %v", p.Grid[:43])
	}
	if n := p.AddItem(tb, -1, items.Item{Number: partytest.Chain}); n != 2 || p.Grid[42] != 2 || p.Grid[71] != -43 {
		t.Errorf("chain slot %d at %d", n, p.Grid[42])
	}
	if n := p.AddItem(tb, 1, items.Item{Number: partytest.Gem}); n != 3 || p.Grid[1] != 3 || p.Grid[16] != -2 {
		t.Errorf("gem at cell 1: slot %d", n)
	}
	if n := p.AddItem(tb, 13, items.Item{Number: partytest.Gem}); n != 0 {
		t.Errorf("a 2x2 at the last column fit (%d)", n)
	}
	p.RemoveItemAt(tb, 0)
	if p.Grid[0] != 0 || p.Grid[28] != 0 || p.Items[0].Number != 0 || p.FreeSlot() != 0 {
		t.Errorf("removal: %v slot %d", p.Grid[:29], p.FreeSlot())
	}
	if n, full := p.AddItemNumber(tb, -1, partytest.Ring); n != 1 || full || p.Grid[0] != 1 {
		t.Errorf("ring number: %d %v", n, full)
	}
	// a full grid
	var q Player
	n := 0
	for q.AddItem(tb, -1, items.Item{Number: partytest.Ring}) != 0 {
		n++
	}
	if n != items.GridCells || q.FreeSlot() != -1 {
		t.Errorf("%d rings fill the pack, free slot %d", n, q.FreeSlot())
	}
}

// Equipping: the slot of the type, six ring slots, race rules.
//
// mm8: 0x492d41, 0x49326f
func TestEquip(t *testing.T) {
	tb, cls := partytest.Items(), partytest.Classes()
	p := knight()
	for k := 0; k < 6; k++ {
		if !p.EquipItem(tb, cls, items.Item{Number: partytest.Ring}) || p.Equip[SlotRing+k] != int32(k+1) {
			t.Fatalf("ring %d", k)
		}
	}
	if p.EquipItem(tb, cls, items.Item{Number: partytest.Ring}) {
		t.Error("a seventh ring")
	}
	if !p.EquipItem(tb, cls, items.Item{Number: partytest.Chain}) || p.Items[6].Slot != SlotArmor+1 {
		t.Errorf("chain: slot byte %d", p.Items[6].Slot)
	}
	if p.EquipItem(tb, cls, items.Item{Number: partytest.Plate}) {
		t.Error("two armours")
	}
	d := Player{Face: 24, Class: 14}
	if d.CanWear(tb, partytest.Chain) || !d.CanWear(tb, partytest.Ring) || d.CanWear(tb, partytest.Wand) {
		t.Error("dragon")
	}
	if (&Player{Face: 20}).CanWear(tb, partytest.Chain) != true {
		t.Error("minotaur armour")
	}
	if (&Player{Class: 4}).CanWear(tb, 0x211) || !(&Player{Class: 1}).CanWear(tb, 0x211) {
		t.Error("artifact 0x211 is for necromancers")
	}
}

// Party_AddItem: the selected member first, then the next ones; Party_SetMouseItem
// stows the item held before.
func TestPartyItems(t *testing.T) {
	tb := partytest.Items()
	c := testCtx()
	m := membersOf(knight(), knight(), knight())
	m.Selected = 2
	for range items.GridCells {
		m.Players[1].AddItem(tb, -1, items.Item{Number: partytest.Ring})
	}
	if !m.AddItem(tb, items.Item{Number: partytest.Ring}, c) || m.Players[2].Items[0].Number != partytest.Ring ||
		m.Players[2].Items[0].Flags != items.FlagIdentified {
		t.Errorf("the item went %+v", m.Players[2].Items[0])
	}
	m.SetMouseItem(tb, items.Item{Number: partytest.Sword})
	m.SetMouseItem(tb, items.Item{Number: partytest.Gem})
	if m.MouseItem.Number != partytest.Gem || m.Players[0].Items[0].Number != partytest.Sword {
		t.Errorf("mouse %d, member 1 has %d", m.MouseItem.Number, m.Players[0].Items[0].Number)
	}
}
