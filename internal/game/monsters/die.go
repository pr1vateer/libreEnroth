package monsters

import (
	"libre-enroth/internal/assets/tables"
	"libre-enroth/internal/game/items"
)

// Death and loot (re/notes/monsters.md, "Death"): Actor_Die starts the dying
// animation, marks the bounty hunts of the monster's kind as done and may drop
// reagents; the corpse's gold and item are rolled when the party searches it.

// Monster specials (MonsterInfo.Special).
const specialExplode = 3

// Reagent drops.
const (
	reagentChance = 20 // percent, per reagent
	reagentRise   = 0x10
)

// reagents are the items a monster of ids lo..hi may drop when it dies.
//
// mm8: 0x402ec9 (Actor_Die: the switch on the monster id)
var reagents = []struct {
	lo, hi int
	items  []int32
}{
	{0x1f, 0x21, []int32{0x28d}},
	{0x55, 0x57, []int32{0x279}},
	{0x61, 0x63, []int32{0xcf}},
	{0x6d, 0x6f, []int32{0xd6}},
	{0x76, 0x78, []int32{0xd9, 0x28e}},
	{0x79, 0x7b, []int32{0x280}},
	{0x82, 0x84, []int32{0xcc}},
	{0x88, 0x8a, []int32{0xd1}},
	{0xa0, 0xa2, []int32{0xca}},
	{0xa9, 0xab, []int32{0x278}},
}

// Reagents are the items a monster of id may drop as it dies.
func Reagents(id int) []int32 {
	for _, r := range reagents {
		if id >= r.lo && id <= r.hi {
			return r.items
		}
	}
	return nil
}

// Die starts an actor's death: the dying animation, no HP, no buffs. The bounty hunts
// for its kind are marked done, and each reagent it carries drops (20%) at its feet,
// thrown in a random direction. An exploding monster's blast is M9's.
//
// mm8: 0x402ec9 (Actor_Die)
func (b *Brain) Die(i int) {
	a := &b.Actors[i]
	a.ActionTime, a.AIState, a.Animation = 0, Dying, tables.AnimDying
	a.HP = 0
	a.ActionLength = b.frameTicks(a.Sprites[tables.AnimDying])
	a.Buffs[BuffParalysed].Clear()
	a.Buffs[BuffStoned].Clear()
	b.sound(i, 1)
	a.UpdateAnimation(b.Sprites)
	if m := b.Party.Members; m != nil {
		m.MarkKilled(int(int16(a.Info.ID)))
	}
	for k := range buffsExpired {
		a.Buffs[k].Clear()
	}
	for _, n := range Reagents(int(int16(a.Info.ID))) {
		it := items.Item{Number: n}
		if b.rand()%100 >= reagentChance {
			continue
		}
		speed := int32(b.rand()%200 + 200)
		e := b.env()
		if b.Objects == nil || e.Items == nil {
			continue
		}
		*b.Objects = e.DropItem(*b.Objects, &it, e.Items.Item(n).Sprite, int32(a.Pos[0]), int32(a.Pos[1]),
			int32(a.Pos[2])+reagentRise, speed, 1, true, 0)
	}
	if a.Info.Special == specialExplode {
		b.stub("explode", "Actor_Explode 0x404a87: a dying monster's blast (object 600, M9)")
	}
}

// GenerateLoot makes a corpse's loot ahead of its search: gold by the monster's dice
// into the gold slot (unless it holds something), and by the treasure chance an item
// of its level and kind into the item slot; the corpse is marked looted.
//
// mm8: 0x40920b (Actor_GenerateLoot; only a spell calls it, M9)
func (a *Actor) GenerateLoot(t *tables.Items, rng items.Rand, found *[items.NumArtifacts]bool) {
	if a.Items[ItemGold].Number == 0 && a.Info.GoldDice != 0 {
		gold := int32(0)
		for range a.Info.GoldDice {
			gold += int32(1 + rng.Int()%max(1, int(a.Info.GoldSides))) // the original divides by zero
		}
		if gold != 0 {
			a.Items[ItemGold] = items.Item{Number: items.GoldSmall, Special: gold}
		}
	}
	if rng.Int()%100 < int(a.Info.TreasureChance) && a.Info.ItemLevel != 0 {
		a.Items[ItemLoot] = items.Generate(t, int(a.Info.ItemLevel), int(a.Info.ItemKind), false, rng, found)
	}
	a.Flags |= FlagLooted
}
