package party

import (
	"testing"

	"libre-enroth/internal/assets/tables"
	"libre-enroth/internal/game/items"
	"libre-enroth/internal/game/party/partytest"
)

// notes records what the item code shows.
type notes struct{ status, stubs []string }

func (n *notes) Global(i int) string {
	return map[int]string{0x43: "%s lacks the skill", 0x24: "%s?", 0x1d3: "%lu gold"}[i]
}
func (n *notes) Status(text string, seconds int) { n.status = append(n.status, text) }
func (n *notes) Stub(key, what string)           { n.stubs = append(n.stubs, key) }
func (n *notes) AutonoteText(int) bool           { return true }

// A click on the pack: picks up, drops at the cell (else anywhere), swaps.
//
// mm8: 0x421764 (CharScreen_ClickInventory), 0x421733 (Player_ItemAtCell)
func TestClickPack(t *testing.T) {
	c := testCtx()
	tb := c.Items
	m := membersOf(knight())
	p := &m.Players[0]
	p.AddItem(tb, 0, items.Item{Number: partytest.Sword}) // 1x3 at cell 0
	// A click on its lower cell (no picture under the mouse) finds it by the grid.
	m.ClickPack(0, tb, 0, 28, c)
	if m.MouseItem.Number != partytest.Sword || p.Grid[0] != 0 {
		t.Fatalf("pick up: mouse %v grid %d", m.MouseItem, p.Grid[0])
	}
	// Dropped on cell 5.
	m.ClickPack(0, tb, 0, 5, c)
	if m.MouseItem.Number != 0 || p.Grid[5] != 1 || p.Grid[19] != -6 {
		t.Fatalf("drop: mouse %v cells %d %d", m.MouseItem, p.Grid[5], p.Grid[19])
	}
	// A gem on the cursor dropped on the sword's picture: they swap.
	m.MouseItem = items.Item{Number: partytest.Gem}
	m.ClickPack(0, tb, 1, -1, c)
	if m.MouseItem.Number != partytest.Sword || p.Grid[5] == 0 || p.Item(p.Grid[5]).Number != partytest.Gem {
		t.Fatalf("swap: mouse %v at 5 %d", m.MouseItem, p.Grid[5])
	}
	// Dropped where it does not fit (the last column): it goes anywhere.
	m.ClickPack(0, tb, 0, 13+14*8, c)
	if m.MouseItem.Number != 0 || p.Grid[0] == 0 {
		t.Fatalf("drop anywhere: mouse %v grid0 %d", m.MouseItem, p.Grid[0])
	}
	// A swap that cannot place the cursor's item puts the old one back.
	var q Player
	for q.AddItem(tb, -1, items.Item{Number: partytest.Ring}) != 0 {
	}
	m = membersOf(q)
	m.MouseItem = items.Item{Number: partytest.Chain}
	m.ClickPack(0, tb, 0, 0, c)
	if m.MouseItem.Number != partytest.Chain || m.Players[0].Grid[0] != 1 || m.Players[0].Items[0].Number != partytest.Ring {
		t.Fatalf("failed swap: mouse %v grid %d", m.MouseItem, m.Players[0].Grid[0])
	}
}

// The paper doll: taking off, the bow on an empty click, wearing by type, the off hand
// for experts, spears, two-handed weapons, rings, and the items that are used.
//
// mm8: 0x468a4b (PaperDoll_Click), 0x467840, 0x492cfc
func TestClickDoll(t *testing.T) {
	c := testCtx()
	k := knight()
	k.Skills[SkillSword] = 1
	k.Skills[SkillBow] = 1
	k.Skills[SkillSpear] = 1
	k.Skills[SkillShield] = 1
	k.Skills[SkillDagger] = SkillExpert | 1
	m := membersOf(k)
	m.Selected = 1
	p := &m.Players[0]
	n := &notes{}
	mouse := func(id int32) { m.MouseItem = items.Item{Number: id, Flags: items.FlagIdentified} }
	mouse(partytest.Sword)
	if r := m.ClickDoll(0, 0, 500, false, c, n); r != DollDone || p.Equip[SlotMainHand] == 0 || m.MouseItem.Number != 0 {
		t.Fatalf("sword: %v main %d", r, p.Equip[SlotMainHand])
	}
	main := p.Equip[SlotMainHand]
	if p.Item(main).Slot != SlotMainHand+1 {
		t.Errorf("body slot %d", p.Item(main).Slot)
	}
	mouse(partytest.Bow)
	m.ClickDoll(0, 0, 500, false, c, n)
	// An empty click takes the bow; not in a shop.
	if m.ClickDoll(0, 0, 500, true, c, n); m.MouseItem.Number != 0 {
		t.Fatal("shop: took the bow")
	}
	m.ClickDoll(0, 0, 500, false, c, n)
	if m.MouseItem.Number != partytest.Bow || p.Equip[SlotBow] != 0 {
		t.Fatalf("bow: mouse %v", m.MouseItem)
	}
	m.MouseItem = items.Item{}
	// The shield in the off hand, then a spear needs a master with something there.
	mouse(partytest.Shield)
	m.ClickDoll(0, 0, 500, false, c, n)
	if p.Equip[SlotOffhand] == 0 {
		t.Fatal("shield not worn")
	}
	mouse(partytest.Spear)
	if m.ClickDoll(0, 0, 500, false, c, n); m.MouseItem.Number != partytest.Spear {
		t.Fatal("spear beside a shield without mastery")
	}
	p.Skills[SkillSpear] = SkillMaster | 1
	m.ClickDoll(0, 0, 500, false, c, n)
	if m.MouseItem.Number != partytest.Sword || p.Item(p.Equip[SlotMainHand]).Number != partytest.Spear {
		t.Fatalf("spear master: mouse %v", m.MouseItem)
	}
	// Taking off the shield by its picture.
	m.MouseItem = items.Item{}
	off := p.Equip[SlotOffhand]
	m.ClickDoll(0, off, 500, false, c, n)
	if m.MouseItem.Number != partytest.Shield || p.Equip[SlotOffhand] != 0 || p.Item(off).Number != 0 {
		t.Fatalf("take off: mouse %v", m.MouseItem)
	}
	// Without the skill: the status line and 0x27.
	p.Skills[SkillShield] = 0
	n.status = nil
	if m.ClickDoll(0, 0, 500, false, c, n); m.MouseItem.Number != partytest.Shield || len(n.status) != 1 || n.status[0] != " lacks the skill" {
		t.Fatalf("no skill: %v %q", m.MouseItem, n.status)
	}
	// Rings: six fingers, the seventh replaces the last.
	for i := 0; i < 7; i++ {
		m.MouseItem = items.Item{Number: partytest.Ring, Strength: int32(i)}
		m.ClickDoll(0, 0, 500, false, c, n)
	}
	if m.MouseItem.Strength != 5 || p.Item(p.Equip[NumSlots-1]).Strength != 6 {
		t.Fatalf("rings: mouse %v", m.MouseItem)
	}
	// A potion is used, not worn.
	mouse(partytest.Potion)
	if r := m.ClickDoll(0, 0, 500, false, c, n); r != DollUse {
		t.Fatalf("potion: %v", r)
	}
}

// Two-handed weapons: with the off hand full and the main hand empty, the off hand's
// item comes onto the cursor (its slot keeps a stray copy, as in the original).
func TestClickDollTwoHanded(t *testing.T) {
	c := testCtx()
	tb := c.Items
	tb.Items[partytest.Sword].EquipType = tables.EquipWeapon2 // a two-handed sword
	k := knight()
	k.Skills[SkillSword] = 1
	m := membersOf(k)
	p := &m.Players[0]
	p.wear(1, SlotOffhand, items.Item{Number: partytest.Shield, Slot: SlotOffhand + 1})
	m.MouseItem = items.Item{Number: partytest.Sword}
	m.ClickDoll(0, 0, 500, false, c, &notes{})
	if m.MouseItem.Number != partytest.Shield || p.Equip[SlotOffhand] != 0 || p.Item(p.Equip[SlotMainHand]).Number != partytest.Sword {
		t.Fatalf("mouse %v main %d", m.MouseItem, p.Equip[SlotMainHand])
	}
	if p.Items[0].Number != partytest.Shield {
		t.Errorf("the leaked slot holds %v", p.Items[0])
	}
	// With both hands full: the error sound.
	p.wear(5, SlotOffhand, items.Item{Number: partytest.Shield})
	m.MouseItem = items.Item{Number: partytest.Sword}
	if r := m.ClickDoll(0, 0, 500, false, c, &notes{}); r != DollError {
		t.Fatalf("both hands: %v", r)
	}
}

// Explosions break items: 0 all but the hardened; n random ones after checking the
// k-th for hardening.
//
// mm8: 0x415b98 (Player_BreakItems)
func TestBreakItems(t *testing.T) {
	var p Player
	p.Items[0] = items.Item{Number: partytest.Sword}
	p.Items[1] = items.Item{Number: partytest.Chain, Flags: items.FlagHardened}
	p.Items[2] = items.Item{Number: 0xdc} // a bottle is not breakable
	p.BreakItems(0, NewRand(1))
	if !p.Items[0].Broken() || p.Items[1].Broken() || p.Items[2].Broken() {
		t.Fatalf("break all: %v", p.Items[:3])
	}
	p.Items[0].Flags = 0
	r := NewRand(1) // rand 41: 41 % 2 = 1, the chain
	p.BreakItems(1, r)
	if p.Items[0].Broken() || !p.Items[1].Broken() {
		t.Fatalf("break one: %v", p.Items[:2])
	}
}

// A skill point costs the new level; too few points or level 60 refuse.
//
// mm8: 0x42f877 (msg 0x79)
func TestSpendSkillPoint(t *testing.T) {
	c := testCtx()
	m := membersOf(knight())
	p := &m.Players[0]
	p.Skills[SkillSword] = SkillExpert | 3
	p.SkillPoints = 5
	if r := m.SpendSkillPoint(0, SkillSword, c); r != 0 || p.Skills[SkillSword] != SkillExpert|4 || p.SkillPoints != 1 {
		t.Fatalf("raise: %d %#x %d", r, p.Skills[SkillSword], p.SkillPoints)
	}
	if r := m.SpendSkillPoint(0, SkillSword, c); r != 0x1e8 {
		t.Errorf("too few: %#x", r)
	}
	p.Skills[SkillSword], p.SkillPoints = 60, 100
	if r := m.SpendSkillPoint(0, SkillSword, c); r != 0x1e7 {
		t.Errorf("at 60: %#x", r)
	}
}

// An item dropped on a portrait goes to that pack.
func TestDropOnPortrait(t *testing.T) {
	c := testCtx()
	m := membersOf(knight(), knight())
	m.MouseItem = items.Item{Number: partytest.Sword}
	if !m.DropOnPortrait(2, c) || m.MouseItem.Number != 0 || m.Players[1].Items[0].Number != partytest.Sword {
		t.Fatalf("drop: %v", m.Players[1].Items[0])
	}
	if m.DropOnPortrait(1, c) {
		t.Error("an empty cursor stored something")
	}
}

// identify and repair by skill: multiplier × level against the difficulty, or GM.
//
// mm8: 0x491ea1, 0x491f09
func TestIdentifyRepair(t *testing.T) {
	c := testCtx()
	m := membersOf(knight())
	p := &m.Players[0]
	e := m.Env(c)
	c.Items.Items[partytest.Sword].IDRepair = 6
	it := items.Item{Number: partytest.Sword}
	p.Skills[SkillIDItem] = SkillExpert | 2 // 2 × 2
	if p.CanIdentify(e, &it) {
		t.Error("4 identified 6")
	}
	p.Skills[SkillIDItem] = SkillExpert | 3
	if !p.CanIdentify(e, &it) {
		t.Error("6 did not identify 6")
	}
	p.Skills[SkillRepair] = SkillGM | 1
	if !p.CanRepair(e, &it) {
		t.Error("a grandmaster could not repair")
	}
}
