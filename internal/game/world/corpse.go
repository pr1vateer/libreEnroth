package world

import (
	"fmt"
	"strings"

	"libre-enroth/internal/assets/tables"
	"libre-enroth/internal/game/items"
	"libre-enroth/internal/game/monsters"
)

// Searching a corpse (re/notes/monsters.md, "Death").

// Global.txt strings.
const txtFoundGoldItem = 0x1ea // "You found %d gold and an item (%s)!"

// Item rolls of a dead NPC's carried item.
const itemSupremeProtection = 0xdc // a potion whose power is not rolled

// searchCorpse takes what corpse i has: its gold (the monster's dice, or the gold its
// loot was rolled with), then its carried item (a wand gets charges, a potion a power),
// else its treasure (the monster's chance, level and kind; with the loot rolled, the
// gold slot again - the original never looks at the loot's item slot), and the two
// items events gave it. The corpse goes - except, when something was found without a
// carried item, one time in ten, when it stays to be searched again (its loot rolled
// anew).
//
// mm8: 0x4256b6 (Actor_SearchCorpse)
func (w *World) searchCorpse(i int) {
	a := &w.Actors()[i]
	m, ctx := w.S.Party, w.S.Ctx
	t := w.tables.Game.Items
	rng := ctx.Rand
	notes := worldNotes{w}
	m.DropMouseItem(t)
	looted := a.Flags&monsters.FlagLooted != 0
	var gold int32
	switch {
	case !looted:
		for range a.Info.GoldDice {
			gold += int32(1 + rng.Int()%max(1, int(a.Info.GoldSides))) // the original divides by zero
		}
	case t.Item(a.Items[monsters.ItemGold].Number).EquipType == tables.EquipGold:
		gold = a.Items[monsters.ItemGold].Special
		a.Items[monsters.ItemGold] = items.Item{}
	}
	if gold != 0 {
		m.AddGold(gold, notes)
	}
	found := func(it items.Item) {
		name := t.Item(it.Number).UnidentifiedName
		if gold == 0 {
			w.Status(cfmtS(w.Global(txtFoundItem), name), 2)
		} else {
			f := strings.Replace(w.Global(txtFoundGoldItem), "%d", fmt.Sprint(gold), 1)
			w.Status(cfmtS(f, name), 2)
		}
		if !m.AddItem(t, it, ctx) {
			m.SetMouseItem(t, it)
		}
	}
	// eventItems hands over the items events gave; one that goes on the mouse counts as
	// found.
	eventItems := func() (got bool) {
		for _, k := range []int{monsters.ItemEvent0, monsters.ItemEvent1} {
			if it := a.Items[k]; it.Number != 0 {
				if !m.AddItem(t, it, ctx) {
					m.DropMouseItem(t)
					m.SetMouseItem(t, it)
					got = true
				}
				a.Items[k] = items.Item{}
			}
		}
		return got
	}
	if a.CarriedItem != 0 {
		it := items.Item{Number: int32(int16(a.CarriedItem))}
		d := t.Item(it.Number)
		name := d.UnidentifiedName
		if gold == 0 {
			w.Status(cfmtS(w.Global(txtFoundItem), name), 2)
		} else {
			f := strings.Replace(w.Global(txtFoundGoldItem), "%d", fmt.Sprint(gold), 1)
			w.Status(cfmtS(f, name), 2)
		}
		if d.EquipType == tables.EquipWand {
			it.Charges = max(int32(rng.Int()%6+1+int(d.Mod2)), 1)
			it.MaxCharges = uint8(it.Charges)
		}
		if d.EquipType == tables.EquipPotion && it.Number != itemSupremeProtection {
			it.Bonus = int32(rng.Int()%4*2 + 2)
		}
		it.ApplySpecial(t)
		if !m.AddItem(t, it, ctx) {
			m.SetMouseItem(t, it)
		}
		a.CarriedItem = 0
		eventItems()
		a.AIState = monsters.Removed
		return
	}
	got := false
	switch {
	case !looted:
		if rng.Int()%100 < int(a.Info.TreasureChance) && a.Info.ItemLevel != 0 {
			found(items.Generate(t, int(a.Info.ItemLevel), int(a.Info.ItemKind), false, rng, &m.ArtifactsFound))
			got = true
		}
	case a.Items[monsters.ItemGold].Number != 0:
		it := a.Items[monsters.ItemGold]
		a.Items[monsters.ItemGold] = items.Item{}
		found(it)
		got = true
	}
	if eventItems() {
		got = true
	}
	if got && rng.Int()%100 > 0x59 {
		return
	}
	a.AIState = monsters.Removed
}
