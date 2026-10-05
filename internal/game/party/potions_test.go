package party

import (
	"testing"

	"libre-enroth/internal/assets/tables"
	"libre-enroth/internal/game/items"
)

// potionCtx is testCtx with the reagents, the bottle and the potions 0xdc..0x10f, and a
// mixing table: Cure Wounds + Magic Potion = Haste (0xe4, expert), Cure Wounds + Cure
// Weakness = Cure Disease (0xe1, normal), Magic Potion + Cure Weakness = E3.
func potionCtx() *Ctx {
	c := testCtx()
	t := c.Items
	for len(t.Items) <= lastPotion {
		t.Items = append(t.Items, tables.ItemDef{W: 1, H: 1})
	}
	for id := firstReagent; id <= lastReagent; id++ {
		t.Items[id] = tables.ItemDef{Name: "Reagent", EquipType: tables.EquipReagent, DiceCount: uint8(id - firstReagent + 1), W: 1, H: 1}
	}
	for id := ItemBottle; id <= lastPotion; id++ {
		t.Items[id] = tables.ItemDef{Name: "Potion", EquipType: tables.EquipPotion, W: 1, H: 1}
	}
	t.Potion, t.PotNotes = &tables.MixTable{}, &tables.MixTable{}
	set := func(a, b int32, v, note int16) {
		t.Potion[a-tables.FirstMixPotion][b-tables.FirstMixPotion] = v
		t.PotNotes[a-tables.FirstMixPotion][b-tables.FirstMixPotion] = note
	}
	set(ItemCureWounds, ItemMagicPotion, 0xe4, 30)
	set(ItemCureWounds, ItemCureWeak, 0xe1, 31)
	set(ItemMagicPotion, ItemCureWeak, 3, 0)
	return c
}

// A reagent in a bottle: the basic potion of its group, power = dice + alchemy.
//
// mm8: 0x415c6d
func TestMixReagent(t *testing.T) {
	c := potionCtx()
	m := membersOf(knight())
	p := &m.Players[0]
	p.Skills[SkillAlchemy] = 3
	s := p.AddItem(c.Items, -1, items.Item{Number: ItemBottle})
	m.MouseItem = items.Item{Number: 207} // the third reagent of the second group
	if r := m.MixPotion(0, s, true, c, &notes{}); r != MixDone {
		t.Fatalf("mix %v", r)
	}
	if it := p.Item(s); it.Number != ItemMagicPotion || it.Bonus != 8+3 || m.MouseItem.Number != 0 {
		t.Fatalf("got %v, mouse %v", *it, m.MouseItem)
	}
	// The bottle on the cursor shows the pop-up instead.
	m.MouseItem = items.Item{Number: ItemBottle}
	if r := m.MixPotion(0, s, true, c, &notes{}); r != MixNone {
		t.Errorf("bottle: %v", r)
	}
}

// Two potions: a result above the alchemy rank explodes, else the target becomes it at
// the mean power, a bottle comes back and the recipe's autonote is set.
func TestMixPotions(t *testing.T) {
	c := potionCtx()
	m := membersOf(knight())
	m.Autonotes = make(Bits, AutonoteBytes)
	p := &m.Players[0]
	p.HP = 200
	target := p.AddItem(c.Items, -1, items.Item{Number: ItemMagicPotion, Bonus: 10})
	// Without alchemy, Haste (expert) blows up: explosion 2, 30..100 fire damage and one
	// broken item; the target is gone, the cursor too.
	p.Items[50] = items.Item{Number: 1}
	m.MouseItem = items.Item{Number: ItemCureWounds, Bonus: 20}
	n := &notes{}
	if r := m.MixPotion(0, target, true, c, n); r != MixExplode {
		t.Fatalf("no alchemy: %v", r)
	}
	if p.Item(target).Number != 0 || m.MouseItem.Number != 0 || p.HP >= 200 || p.HP < 100 || !p.Items[50].Broken() {
		t.Fatalf("explosion: target %v HP %d item %v", *p.Item(target), p.HP, p.Items[50])
	}
	// An expert makes it.
	p.Skills[SkillAlchemy] = SkillExpert | 2
	target = p.AddItem(c.Items, -1, items.Item{Number: ItemMagicPotion, Bonus: 10})
	m.MouseItem = items.Item{Number: ItemCureWounds, Bonus: 20}
	if r := m.MixPotion(0, target, true, c, n); r != MixDone {
		t.Fatalf("expert: %v", r)
	}
	if it := p.Item(target); it.Number != 0xe4 || it.Bonus != 15 || !m.Autonotes.Get(30) {
		t.Fatalf("haste: %v autonote %v", *it, m.Autonotes.Get(30))
	}
	bottles := 0
	for _, it := range p.Items {
		if it.Number == ItemBottle {
			bottles++
		}
	}
	if bottles != 1 {
		t.Errorf("%d bottles back", bottles)
	}
	// An "E3" cell explodes whatever the rank: 50..250 damage, five breaks.
	target = p.AddItem(c.Items, -1, items.Item{Number: ItemCureWeak})
	m.MouseItem = items.Item{Number: ItemMagicPotion}
	if r := m.MixPotion(0, target, false, c, n); r != MixExplode {
		t.Fatalf("E3: %v", r)
	}
	// No recipe: the pop-up.
	target = p.AddItem(c.Items, -1, items.Item{Number: 0xe5})
	m.MouseItem = items.Item{Number: 0xe6}
	if r := m.MixPotion(0, target, false, c, n); r != MixNone {
		t.Fatalf("no recipe: %v", r)
	}
	// The catalyst gives its power (with any alchemy).
	m.MouseItem = items.Item{Number: ItemCatalyst, Bonus: 33}
	if r := m.MixPotion(0, target, false, c, n); r != MixDone || p.Item(target).Bonus != 33 || p.Item(target).Number != 0xe5 {
		t.Fatalf("catalyst: %v %v", r, *p.Item(target))
	}
}

// Potions for items: Harden, Recharge.
func TestPotionOnItem(t *testing.T) {
	c := potionCtx()
	tb := c.Items
	m := membersOf(knight())
	p := &m.Players[0]
	w := p.AddItem(tb, -1, items.Item{Number: 10, Charges: 3, MaxCharges: 20}) // the wand
	m.MouseItem = items.Item{Number: ItemRecharge, Bonus: 50}
	if r := m.MixPotion(0, w, true, c, &notes{}); r != MixDone || p.Item(w).MaxCharges != 16 || p.Item(w).Charges != 16 {
		t.Fatalf("recharge: %v %v", r, *p.Item(w))
	}
	s := p.AddItem(tb, -1, items.Item{Number: 1})
	m.MouseItem = items.Item{Number: ItemRecharge, Bonus: 50}
	if r := m.MixPotion(0, s, true, c, &notes{}); r != MixError || m.MouseItem.Number != ItemRecharge {
		t.Fatalf("recharge a sword: %v", r)
	}
	m.MouseItem = items.Item{Number: ItemHarden}
	if r := m.MixPotion(0, s, true, c, &notes{}); r != MixDone || p.Item(s).Flags&items.FlagHardened == 0 {
		t.Fatalf("harden: %v %v", r, *p.Item(s))
	}
}

// Drinking: heal, mana, cures, the one-time stat potions, Haste while weak, recovery.
//
// mm8: 0x467aa9 (Player_UseItem)
func TestUseItem(t *testing.T) {
	c := potionCtx()
	m := membersOf(knight())
	p := &m.Players[0]
	e := m.Env(c)
	p.HP = 1
	m.MouseItem = items.Item{Number: ItemCureWounds, Bonus: 5}
	r := m.UseItem(0, c, &notes{})
	if !r.Close || p.HP != 16 || m.MouseItem.Number != 0 || p.Recovery != 213 {
		t.Fatalf("cure wounds: %+v HP %d recovery %d", r, p.HP, p.Recovery)
	}
	p.Conditions[CondPoison2], p.Conditions[CondDisease1] = 5, 5
	m.MouseItem = items.Item{Number: 0xe2}
	m.UseItem(0, c, &notes{})
	if p.Conditions[CondPoison2] != 0 || p.Conditions[CondDisease1] == 0 {
		t.Fatalf("cure poison: %v", p.Conditions)
	}
	before := p.Base(StatMight)
	for i := 0; i < 2; i++ {
		m.MouseItem = items.Item{Number: 0x10e}
		m.UseItem(0, c, &notes{})
	}
	if p.Base(StatMight) != before+50 || !p.PurePotions[6] {
		t.Fatalf("pure might twice: %d", p.Base(StatMight))
	}
	p.Conditions[CondWeak] = 1
	m.MouseItem = items.Item{Number: 0xe4}
	if r := m.UseItem(0, c, &notes{}); !r.Close || m.MouseItem.Number != 0 {
		t.Fatalf("haste while weak: %+v", r)
	}
	// The bottle and a potion for items cannot be drunk.
	n := &notes{}
	m.MouseItem = items.Item{Number: ItemHarden}
	if r := m.UseItem(0, c, n); !r.Error || m.MouseItem.Number != ItemHarden || len(n.status) != 1 {
		t.Fatalf("harden: %+v %q", r, n.status)
	}
	// The catalyst poisons.
	m.MouseItem = items.Item{Number: ItemCatalyst}
	m.UseItem(0, c, &notes{})
	if p.Conditions[CondPoison1] == 0 {
		t.Error("catalyst did not poison")
	}
	// Magic Potion: SP up to the maximum.
	p.Class = 2
	m.MouseItem = items.Item{Number: ItemMagicPotion, Bonus: 100}
	m.UseItem(0, c, &notes{})
	if p.SP != p.MaxSP(e) {
		t.Errorf("SP %d, max %d", p.SP, p.MaxSP(e))
	}
}

// Books: learnt when the school's rank reaches the spell.
func TestReadBook(t *testing.T) {
	c := potionCtx()
	tb := c.Items
	for len(tb.Items) <= 0x1a0 {
		tb.Items = append(tb.Items, tables.ItemDef{W: 1, H: 1})
	}
	for id := 400; id < 0x1a0; id++ {
		tb.Items[id] = tables.ItemDef{Name: "Book", EquipType: tables.EquipBook, W: 1, H: 1}
	}
	m := membersOf(knight())
	p := &m.Players[0]
	m.MouseItem = items.Item{Number: 400 + 4, Flags: items.FlagIdentified} // the fifth fire spell
	n := &notes{}
	p.Skills[SkillFire] = 1
	if m.UseItem(0, c, n); p.Spells[4] || m.MouseItem.Number == 0 {
		t.Fatal("a normal rank learnt spell 5")
	}
	p.Skills[SkillFire] = SkillExpert | 1
	if r := m.UseItem(0, c, n); r.Close || !p.Spells[4] || m.MouseItem.Number != 0 {
		t.Fatalf("expert: %+v learnt %v", r, p.Spells[4])
	}
}
