package ui

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"libre-enroth/internal/game/items"
	"libre-enroth/internal/game/party"
	"libre-enroth/internal/gfx"
)

// Frozen SHA-256s of the character screen scenes. Regenerate with:
// MM8_DATA=../games_mm8 go test ./internal/game/ui -run CharScreens -update
var charHashes = map[string]string{
	// The ready gem (M9a) sits under each portrait that can act.
	"char_stats":        "22bea5b80e57f0d3cf87a48b9512c977088102dd77dd8b5f2c3dc9a15a8c46f7",
	"char_skills":       "e78171ffe29328fcaa3cc33c189788bf86439e6ece852475f3f4eba3b5430a99",
	"char_awards":       "90d95eb6883127a658aab7007e7f942a13cb9fbeb0dcc0b996afbee92d7de7b0",
	"char_inventory":    "048157ce3d1ea74cf8034ae743178845a19ceff0ee7c45cdea0bcae0c6da1a69",
	"char_jewelry":      "ef790334bd22f86049d3b7d0e676b7a40d6e0a186712745c3c5e6a557db1f98c",
	"char_cleric":       "97aa17e547c8990a166c4fb2f187dfba6262943fb52c8177d963dd76e8be7115",
	"char_minotaur":     "f3cd67b69a03046207e85a9717691cf072dd22b95389b265367001a2d3a322e6",
	"char_troll":        "3ed09cf8f6c772757279814b9e538c0ff24d9f8aa7545dceb616e95ef5e51bae",
	"item_popup":        "3fc6addb3b886e608b6b0d469c884868a0ee1f6cc7c05077bc8eb064fb528394",
	"item_popup_broken": "0a77e84633c7bf28371f6c9cedbd2b2f195d1d8ec6df76f420d1530927d3b353",
	"skill_popup":       "ed1e5c668c109b4c55b4b2996836ce0939f4b99bdeb3f8d83465584593687d5a",
	"stat_popup":        "113e5ff6abc89467f5575e63ed9278a99c2d0e4ae0fc5cfb9e37d0d13d58e347",
}

// wear puts item id on member p in equipment slot (identified unless flags say).
func wear(t *testing.T, p *party.Player, id int32, slot int, flags uint32) {
	t.Helper()
	n := p.FreeSlot()
	if n < 0 {
		t.Fatal("no item slot")
	}
	p.Items[n] = items.Item{Number: id, Flags: items.FlagIdentified | flags, Slot: uint8(slot + 1)}
	p.Equip[slot] = int32(n + 1)
}

// newOutfitted is a five-member party in the game view: a knight in plate with sword,
// shield, bow, helm, cloak with hood, belt, gauntlets, boots and jewellery; a cleric
// with a mace and a dagger in the off hand; a necromancer; a minotaur with a
// two-handed axe; a troll with a spear. The knight carries a few items and has awards
// and skill points.
func newOutfitted(t *testing.T) (*App, *inGame) {
	t.Helper()
	a := newTestAppParty(t, StateInGame, []int{0, 5, 12, 20, 22})
	g := inGameOf(t, a)
	m := g.r.Party
	ctx, err := g.r.Ctx()
	if err != nil {
		t.Fatal(err)
	}
	tbl := ctx.Items
	for i, n := range []string{"Zoltan", "Lisa", "Mordred", "Grax", "Burk"} {
		m.Players[i].Name = n
	}
	k := &m.Players[0]
	wear(t, k, 1, party.SlotMainHand, 0)
	wear(t, k, 99, party.SlotOffhand, 0)
	wear(t, k, 95, party.SlotArmor, items.FlagBroken)
	wear(t, k, 111, party.SlotHelm, 0)
	wear(t, k, 126, party.SlotCloak, 0)
	wear(t, k, 118, party.SlotBelt, 0)
	wear(t, k, 129, party.SlotGauntlet, 0)
	wear(t, k, 133, party.SlotBoots, 0)
	wear(t, k, 57, party.SlotBow, 0)
	wear(t, k, 148, party.SlotAmulet, 0)
	wear(t, k, 138, party.SlotRing, 0)
	wear(t, k, 141, party.SlotRing+3, 0)
	k.AddItem(tbl, 0, items.Item{Number: 67, Flags: items.FlagIdentified})
	k.AddItem(tbl, 3, items.Item{Number: 90})                           // unidentified
	k.AddItem(tbl, 9, items.Item{Number: 112, Flags: items.FlagBroken}) // broken
	k.AddItem(tbl, -1, items.Item{Number: party.ItemCureWounds, Bonus: 8, Flags: items.FlagIdentified})
	k.AddItem(tbl, -1, items.Item{Number: 205, Flags: items.FlagIdentified})
	k.AddItem(tbl, -1, items.Item{Number: party.ItemBottle, Flags: items.FlagIdentified})
	k.SkillPoints = 3
	k.Skills[party.SkillPerception] = 2
	k.Skills[party.SkillFire] = party.SkillExpert | 4
	k.Skills[party.SkillAlchemy] = 1
	for _, aw := range []int{1, 2, 5, 9, 12, 20, 33, 40} {
		k.Awards.Set(aw, true)
	}
	c := &m.Players[1]
	wear(t, c, 66, party.SlotMainHand, 0)
	wear(t, c, 22, party.SlotOffhand, 0)
	wear(t, c, 84, party.SlotArmor, 0)
	wear(t, c, 122, party.SlotCloak, 0)
	wear(t, c, 115, party.SlotHelm, 0)
	wear(t, c, 132, party.SlotBoots, 0)
	wear(t, c, 117, party.SlotBelt, 0)
	wear(t, &m.Players[3], 36, party.SlotMainHand, 0)
	wear(t, &m.Players[3], 91, party.SlotArmor, 0)
	wear(t, &m.Players[4], 42, party.SlotMainHand, 0)
	wear(t, &m.Players[4], 89, party.SlotArmor, 0)
	return a, g
}

func click(x, y int) []Input {
	return []Input{{X: x, Y: y}, {X: x, Y: y, Left: true, LeftPressed: true}, {X: x, Y: y, LeftReleased: true}}
}

// openChar opens the character screen on member slot (1-based) with two presses of its
// key, then lets a frame draw (clicks pick from the last frame).
func openChar(t *testing.T, a *App, slot int) *charScreen {
	t.Helper()
	k := Key('0' + slot)
	run(t, a, Input{X: 600, Y: 20, Keys: []Key{k}}, Input{X: 600, Y: 20}, Input{X: 600, Y: 20, Keys: []Key{k}})
	g := inGameOf(t, a)
	if g.char == nil {
		t.Fatal("character screen not open")
	}
	a.Draw(gfx.NewCanvas())
	return g.char
}

func TestCharScreens(t *testing.T) {
	away := Input{X: 600, Y: 20}
	for _, sc := range []struct {
		name string
		slot int
		in   []Input
	}{
		{"char_stats", 1, []Input{away}},
		{"char_skills", 1, []Input{{X: 600, Y: 20, Keys: []Key{'K'}}, away}},
		{"char_awards", 1, []Input{{X: 600, Y: 20, Keys: []Key{'A'}}, away}},
		{"char_inventory", 1, []Input{{X: 600, Y: 20, Keys: []Key{'I'}}, away}},
		{"char_jewelry", 1, append(click(0x25e+8, 300+8), away)},
		{"char_cleric", 2, []Input{away}},
		{"char_minotaur", 4, []Input{away}},
		{"char_troll", 5, []Input{away}},
		// The right button held over the mace in the pack.
		{"item_popup", 1, []Input{{X: 600, Y: 20, Keys: []Key{'I'}}, away, {X: 20, Y: 50}, {X: 20, Y: 50, Right: true, RightPressed: true}}},
		// Over the broken helm.
		{"item_popup_broken", 1, []Input{{X: 600, Y: 20, Keys: []Key{'I'}}, away, {X: 6 + 9*32 + 20, Y: 32 + 20}, {X: 6 + 9*32 + 20, Y: 32 + 20, Right: true, RightPressed: true}}},
		// A skill's description.
		{"skill_popup", 1, []Input{{X: 600, Y: 20, Keys: []Key{'K'}}, away, {X: 40, Y: 90}, {X: 40, Y: 90, Right: true, RightPressed: true}}},
		// The might description.
		{"stat_popup", 1, []Input{away, {X: 40, Y: 78}, {X: 40, Y: 78, Right: true, RightPressed: true}}},
	} {
		t.Run(sc.name, func(t *testing.T) {
			a, _ := newOutfitted(t)
			openChar(t, a, sc.slot)
			run(t, a, sc.in...)
			c := gfx.NewCanvas()
			a.Draw(c)
			sum := sha256.Sum256(c.Img.Pix)
			got := hex.EncodeToString(sum[:])
			if *update {
				writePNG(t, c, sc.name)
				t.Logf("%q: %q,", sc.name, got)
				return
			}
			if want, ok := charHashes[sc.name]; !ok || got != want {
				t.Errorf("canvas sha256 %s, want %s (inspect with -update)", got, want)
			}
		})
	}
}

// cellAt is the screen point inside pack cell c.
func cellAt(c int) (int, int) { return c%items.GridW*32 + 6 + 10, (c/items.GridW+1)*32 + 10 }

// topCell is the top-left cell of the first item id in member p's pack.
func topCell(t *testing.T, p *party.Player, id int32) int {
	t.Helper()
	for c, n := range p.Grid {
		if n > 0 && p.Item(n).Number == id {
			return c
		}
	}
	t.Fatalf("item %d not in the pack", id)
	return -1
}

// The pack through the pick buffer: an item picked up by its picture, dropped on an
// empty cell; a skill label raises the skill; a reagent right-clicked on the bottle
// makes a potion; a potion dropped on the doll is drunk and closes the screen.
func TestCharScreenClicks(t *testing.T) {
	a, g := newOutfitted(t)
	m := g.r.Party
	k := &m.Players[0]
	cs := openChar(t, a, 1)
	run(t, a, Input{X: 600, Y: 20, Keys: []Key{'I'}})
	a.Draw(gfx.NewCanvas())
	x, y := cellAt(topCell(t, k, 67))
	run(t, a, click(x, y)...)
	if m.MouseItem.Number != 67 {
		t.Fatalf("picked up %v", m.MouseItem)
	}
	x, y = cellAt(11) // the 2x5 flail fits from row 0
	run(t, a, click(x, y)...)
	if n := k.Grid[11]; m.MouseItem.Number != 0 || n <= 0 || k.Item(n).Number != 67 {
		t.Fatalf("drop: mouse %v cell %d grid %v", m.MouseItem, n, k.Grid[:])
	}
	// The reagent onto the bottle: Magic Potion of power 6 (dice 5 + alchemy 1).
	a.Draw(gfx.NewCanvas())
	x, y = cellAt(topCell(t, k, 205))
	run(t, a, click(x, y)...)
	a.Draw(gfx.NewCanvas())
	bottle := topCell(t, k, party.ItemBottle)
	x, y = cellAt(bottle)
	run(t, a, Input{X: x, Y: y}, Input{X: x, Y: y, Right: true, RightPressed: true}, Input{X: x, Y: y})
	if it := k.Item(k.Grid[bottle]); it.Number != party.ItemMagicPotion || it.Bonus != int32(cs.it.Item(205).DiceCount)+1 || m.MouseItem.Number != 0 {
		t.Fatalf("mix: %v mouse %v", *it, m.MouseItem)
	}
	// A skill point on Sword (level 1 -> 2 costs 2 of the 3).
	run(t, a, Input{X: 600, Y: 20, Keys: []Key{'K'}})
	a.Draw(gfx.NewCanvas())
	var sword *skillRow
	for i := range cs.rows {
		if cs.rows[i].skill == party.SkillSword {
			sword = &cs.rows[i]
		}
	}
	if sword == nil {
		t.Fatal("no sword label")
	}
	r := sword.name.Rect()
	run(t, a, click(r.Min.X+5, r.Min.Y+3)...)
	if k.Skills[party.SkillSword]&party.SkillLevel != 2 || k.SkillPoints != 1 || sword.level.Text != "2" {
		t.Fatalf("sword %#x points %d label %q", k.Skills[party.SkillSword], k.SkillPoints, sword.level.Text)
	}
	// The Cure Wounds on the doll: drunk, the screen closes.
	run(t, a, Input{X: 600, Y: 20, Keys: []Key{'I'}})
	a.Draw(gfx.NewCanvas())
	k.HP = 5
	x, y = cellAt(topCell(t, k, party.ItemCureWounds))
	run(t, a, click(x, y)...)
	run(t, a, click(520, 200)...)
	if k.HP != 5+8+10 || m.MouseItem.Number != 0 || g.char != nil {
		t.Fatalf("drink: HP %d mouse %v screen %v", k.HP, m.MouseItem, g.char != nil)
	}
}

// Taking the helm off the doll by clicking it, and putting it back.
func TestCharScreenDoll(t *testing.T) {
	a, g := newOutfitted(t)
	m := g.r.Party
	k := &m.Players[0]
	openChar(t, a, 1)
	helm := k.Equip[party.SlotHelm]
	// Find a pixel of the helm in the pick buffer.
	px, py := -1, -1
	for y := 0x17; y < 0x16f && px < 0; y++ {
		for x := 0x1dc; x < 0x280; x++ {
			if g.char.pick.at(x, y) == helm {
				px, py = x, y
				break
			}
		}
	}
	if px < 0 {
		t.Fatal("the helm wrote no pick id")
	}
	run(t, a, click(px, py)...)
	if m.MouseItem.Number != 111 || k.Equip[party.SlotHelm] != 0 {
		t.Fatalf("take off: mouse %v", m.MouseItem)
	}
	a.Draw(gfx.NewCanvas())
	run(t, a, click(520, 200)...)
	if m.MouseItem.Number != 0 || k.Equip[party.SlotHelm] == 0 {
		t.Fatalf("put on: mouse %v", m.MouseItem)
	}
}
