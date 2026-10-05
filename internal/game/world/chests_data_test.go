package world

import (
	"testing"

	"libre-enroth/internal/game/items"
	"libre-enroth/internal/game/ui"
	"libre-enroth/internal/gfx"
)

// d05's chests: no random item survives the entry, the first opening places the items in
// the 9 × 9 grid, a trap goes off for a member who cannot disarm it (and is gone after).
//
// mm8: 0x44ec9d, 0x420093, 0x41fef0
func TestChestsD05(t *testing.T) {
	e := newEnv(t)
	s := NewSession()
	s.Ctx.Items, s.Ctx.Classes = e.tables.Game.Items, e.tables.Game.Classes
	w, err := Load(e.d, e.tables, e.tex, "d05.blv", s)
	if err != nil {
		t.Fatal(err)
	}
	cs := w.chests()
	if len(cs) != 20 {
		t.Fatalf("%d chests", len(cs))
	}
	for i := range cs {
		for k, it := range cs[i].Items {
			if it.Number < 0 {
				t.Fatalf("chest %d item %d still random (%d)", i, k, it.Number)
			}
		}
	}
	if lvl := w.tables.MapStat("d05.blv", mapStatTrap); lvl < 1 {
		t.Fatalf("d05 trap level %d", lvl)
	}
	// Chest 1 is trapped; the knight (no Disarm Trap) sets it off: the event would end.
	w.S.Party.Selected = 1
	if cs[1].Flags&items.ChestTrapped == 0 {
		t.Fatal("chest 1 not trapped")
	}
	if w.OpenChest(1) || w.OpenedChest() != nil || cs[1].Flags&items.ChestTrapped != 0 {
		t.Fatal("the trap did not go off")
	}
	if !w.OpenChest(1) || w.OpenedChest() == nil {
		t.Fatal("chest 1 did not open once disarmed")
	}
	v := w.OpenedChest()
	if v.Chest.Flags&items.ChestPlaced == 0 || v.Grid.W != 9 {
		t.Fatalf("flags %#x grid %+v", v.Chest.Flags, v.Grid)
	}
	n, placed := 0, 0
	for k, it := range v.Chest.Items {
		if it.Number != 0 {
			n++
		}
		if k < len(v.Chest.Grid) && v.Chest.Grid[k] > 0 {
			placed++
		}
	}
	// An item that fits at none of the shuffled cells stays in the record unplaced
	// (and unseen), as in the original.
	if n == 0 || placed == 0 || placed > n {
		t.Errorf("%d items, %d placed", n, placed)
	}
	for k, it := range v.Chest.Items {
		if it.Number != 0 {
			d := e.tables.Game.Items.Item(it.Number)
			t.Logf("item %d: %d %s %dx%d", k, it.Number, d.Name, d.W, d.H)
		}
	}
	w.CloseChest()
	if w.OpenedChest() != nil {
		t.Error("still open")
	}
}

// The chest screen: a gold pile goes to the party, an item onto the cursor and back into
// the chest; the selected member's portrait shows the pack and Close returns.
//
// mm8: 0x4209ac (Chest_Click), 0x420921 (Party_AddGold), 0x4213c0 (mode 10 -> 0xf)
func TestChestScreen(t *testing.T) {
	e := newEnv(t)
	a := e.app(t, "d05.blv")
	w := a.World().(*World)
	if w.Dialog() != nil {
		a.Update(&ui.Input{Keys: []ui.Key{ui.KeyEscape}})
	}
	w.chests()[1].Flags &^= items.ChestTrapped
	w.RunEvent(82, true)
	a.Update(&ui.Input{X: 600, Y: 300})
	v := w.OpenedChest()
	if v == nil {
		t.Fatal("chest 1 not open")
	}
	a.Draw(gfx.NewCanvas())
	click := func(x, y int) {
		a.Update(&ui.Input{X: x, Y: y})
		a.Update(&ui.Input{X: x, Y: y, Left: true, LeftPressed: true})
		a.Update(&ui.Input{X: x, Y: y, LeftReleased: true})
		a.Draw(gfx.NewCanvas())
	}
	// cellPoint is the screen point of a cell's item centre.
	cellPoint := func(cell int) (int, int) {
		d := e.tables.Game.Items.Item(v.Chest.Items[v.Chest.Grid[cell]-1].Number)
		return cell%v.Grid.W*32 + v.Grid.X + d.W*16, cell/v.Grid.H*32 + v.Grid.Y + d.H*16
	}
	m := w.S.Party
	gold, item := -1, -1
	for c := 0; c < 81; c++ {
		if n := v.Chest.Grid[c]; n > 0 {
			switch id := v.Chest.Items[n-1].Number; {
			case id >= items.GoldSmall && id <= items.GoldLarge && gold < 0:
				gold = c
			case id < items.GoldSmall && item < 0:
				item = c
			}
		}
	}
	if gold < 0 || item < 0 {
		t.Fatalf("no gold (%d) or item (%d) in the chest", gold, item)
	}
	before, amount := m.Gold, v.Chest.Items[v.Chest.Grid[gold]-1].Special
	x, y := cellPoint(gold)
	click(x, y)
	if m.Gold != before+amount || v.Chest.Grid[gold] != 0 {
		t.Fatalf("gold: %d -> %d (+%d)", before, m.Gold, amount)
	}
	id := v.Chest.Items[v.Chest.Grid[item]-1].Number
	x, y = cellPoint(item)
	click(x, y)
	if m.MouseItem.Number != id {
		t.Fatalf("took %v, want %d", m.MouseItem, id)
	}
	click(200, 200)
	if m.MouseItem.Number != 0 {
		t.Fatalf("not put back: %v", m.MouseItem)
	}
	// The selected member's portrait: the pack (mode 0xf); Close: back to the chest.
	a.Update(&ui.Input{X: 600, Y: 300, Keys: []ui.Key{'1'}})
	a.Update(&ui.Input{X: 600, Y: 300, Keys: []ui.Key{ui.KeyEscape}})
	if w.OpenedChest() == nil {
		t.Fatal("Esc in the pack view closed the chest")
	}
	a.Update(&ui.Input{X: 600, Y: 300, Keys: []ui.Key{ui.KeyEscape}})
	if w.OpenedChest() != nil {
		t.Fatal("Esc did not close the chest")
	}
}
