package dialog

import (
	"slices"
	"strings"
	"testing"

	"libre-enroth/internal/assets/tables"
	"libre-enroth/internal/game/clock"
	"libre-enroth/internal/game/items"
	"libre-enroth/internal/game/party"
	"libre-enroth/internal/game/party/partytest"
)

// shopHost is serviceHost with a shop of each kind and a guild, made-up stock records
// and partytest's items: 1 weapons (Val 1.5, restock every 7 days, swords), 15 armour
// (leather on the top shelf, chain below), 29 magic (misc items, item 0x4c among
// them), 42 an alchemist, 139 an elemental guild (4 books a school).
func shopHost(t *testing.T) *fakeHost {
	h := serviceHost(t)
	hs, an := h.t.Houses, h.t.HouseAnims
	for i, id := range []int{1, 15, 29, 42, 139} {
		typ := []int{TypeWeapons, TypeArmour, TypeMagic, TypeAlchemy, 0xe}[i]
		hs[id] = tables.House{Name: "S", Type: int16(typ), Video: int16(150 + i), Owner: "O", Title: "Smith", Val: 1.5, C: 7}
		an[150+i] = tables.HouseAnim{Video: "v", Portrait: 1, Type: uint8(typ)}
	}
	s := tables.NewShops(nil)
	s.Counts = [tables.NumShopTypes]int{0, 6, 8, 12, 12}
	s.GuildSpells = make([]int16, tables.FirstGuild+tables.NumGuilds)
	s.GuildSpells[139] = 4
	shelf := func(va uint32, level, kind int) {
		s.SetShort(va, level)
		for k := range 4 {
			s.SetShort(va+2+uint32(2*k), kind)
		}
	}
	shelf(0x50279a+10, 1, 0x17)          // weapons std: swords
	shelf(0x5029ce+10, 1, 0x17)          // weapons spc
	shelf(0x502704+10*(2*15), 1, 0x1f)   // armour std, top: leather
	shelf(0x502704+10*(2*15+1), 1, 0x20) // armour std, bottom: chain
	shelf(0x502938+10*(2*15), 1, 0x1f)
	shelf(0x502938+10*(2*15+1), 1, 0x20)
	s.SetShort(0x50290e+2*29, 1)
	s.SetShort(0x502b42+2*29, 1)
	s.SetShort(0x5028fa+2*42, 1)
	s.SetShort(0x502b44+2*42, 1)
	h.t.Shops = s
	it := partytest.Items()
	it.Items = append(it.Items, make([]tables.ItemDef, 0x4e-len(it.Items))...)
	it.Items[0x4c] = tables.ItemDef{Name: "Swapped", EquipType: tables.EquipRing, Skill: tables.SkillMisc, Chance: [6]uint8{200}, W: 1, H: 1}
	h.ctx.Items, h.ctx.Classes = it, partytest.Classes()
	h.t.Merchant = &tables.Merchant{
		Buy:      [7]string{"no gold", "buy %24 for %27", "buy %25 / %27", "buy good %27"},
		Sell:     [7]string{"", "sell %27", "sell %25 / %27", "sell good %27", "wrong %28", "beyond", "stolen"},
		Identify: [7]string{"", "id %29", "id %29", "id %29", "wrong %24", "beyond", "stolen"},
		Repair:   [7]string{"", "fix %27", "fix %25 / %27", "fix good", "wrong", "beyond", "stolen"},
	}
	h.m.Time = 1000
	return h
}

// TestShopPrices: buying costs value · Val less the discount but at least the value;
// selling pays value / (Val + 2) plus the discount's share, at most the value;
// identifying Val · 50 and repairing value / (6 - Val), both merchant-adjusted (at least
// a third); all at least 1.
//
// mm8: 0x4b8be3, 0x4b8ba3, 0x4b8c1a, 0x4b8c40, 0x4b8c6c.. (the base prices), 0x4b28ab
func TestShopPrices(t *testing.T) {
	for _, c := range []struct {
		name string
		got  int
		want int
	}{
		{"buy", BuyPrice(100, 1.5, 0), 150},
		{"buy, expert merchant 4", BuyPrice(100, 1.5, 15), 127},
		{"buy, bad reputation", BuyPrice(100, 1.5, -5), 157},
		{"buy, grandmaster", BuyPrice(100, 1.5, 10000), 100},
		{"buy, worthless", BuyPrice(0, 1.5, 0), 1},
		{"sell", SellPrice(100, 1.5, 0), 28},
		{"sell, expert merchant 4", SellPrice(100, 1.5, 15), 43},
		{"sell, grandmaster", SellPrice(100, 1.5, 10000), 100},
		{"sell, at least 1", SellPrice(1, 4, -10), 1},
		{"identify", IdentifyPrice(1.5, 0), 75},
		{"identify, expert merchant 4", IdentifyPrice(1.5, 15), 63},
		{"identify, grandmaster: a third", IdentifyPrice(1.5, 10000), 25},
		{"repair", RepairPrice(300, 1.5, 0), 66},
		{"repair, expert merchant 4", RepairPrice(300, 1.5, 15), 56},
		{"repair, Val 4", RepairPrice(300, 4, 0), 150},
		{"base buy", BasePrice(ActionBuy, 100, 2.5), 250},
		{"base sell", BasePrice(ActionSell, 100, 2.5), 22},
		{"base identify", BasePrice(ActionIdentify, 100, 2.5), 125},
		{"base repair", BasePrice(ActionRepair, 100, 2.5), 28},
	} {
		if c.got != c.want {
			t.Errorf("%s: %d, want %d", c.name, c.got, c.want)
		}
	}
	if got := restockDelay(7); got != 7*368640 {
		t.Errorf("restock after 7 days: %d ticks, want %d", got, 7*368640)
	}
}

// stock replays a shelf's draws: rand() % 4 picks the kind, then ItemGen_Generate.
func stock(it *tables.Items, r *party.Rand, n int, kind func(i int) [4]int) []items.Item {
	var found [items.NumArtifacts]bool
	var out []items.Item
	for i := range n {
		g := items.Generate(it, 1, kind(i)[r.Int()%4], false, r, &found)
		g.Flags = items.FlagIdentified
		out = append(out, g)
	}
	return out
}

// TestShopRestock: the first Buy fills both shelves (standard first, then special, each
// item's kind drawn before it is made, all identified) and sets the next restock C
// days on; later Buys keep the stock until that time has passed. The weapon shop
// draws each weapon's place on its table at every Buy.
//
// mm8: 0x4bd028 (codes 2, 0x5f), 0x4b9820, 0x4b99e4
func TestShopRestock(t *testing.T) {
	h := shopHost(t)
	d := openProprietor(t, h, 1)
	swords := func(int) [4]int { return [4]int{0x17, 0x17, 0x17, 0x17} }
	r := party.NewRand(1)
	want := append(stock(h.ctx.Items, r, 6, swords), stock(h.ctx.Items, r, 6, swords)...)
	var y [6]int
	for i := range y {
		y[i] = r.Int() % 300
	}
	clickService(d, SvcBuyStandard)
	st := h.m.Shops
	if d.Menu != SvcBuyStandard || st == nil {
		t.Fatalf("menu %#x", d.Menu)
	}
	if got := append(st.Standard[1][:6:6], st.Special[1][:6]...); !slices.Equal(got, want) {
		t.Errorf("stock:\n%v\nwant\n%v", got, want)
	}
	if st.Standard[1][0].Number != partytest.Sword || st.Standard[1][6].Number != 0 {
		t.Errorf("weapon shop: %v", st.Standard[1])
	}
	if !slices.Equal(d.ShelfY[:6], y[:]) {
		t.Errorf("table places %v, want %v", d.ShelfY[:6], y)
	}
	if st.Restock[1] != 1000+7*368640 {
		t.Errorf("next restock %d", st.Restock[1])
	}
	st.Standard[1][0] = items.Item{}
	d.Back()
	h.m.Time = st.Restock[1] // not past yet
	clickService(d, SvcBuySpecial)
	if st.Standard[1][0].Number != 0 {
		t.Error("restocked before its time")
	}
	d.Back()
	h.m.Time++
	clickService(d, SvcBuyStandard)
	if st.Standard[1][0].Number == 0 || st.Restock[1] != h.m.Time+7*368640 {
		t.Errorf("no restock after its time: %v %d", st.Standard[1][0], st.Restock[1])
	}
}

// TestShopStockKinds: the armour shop's top four places come from its first record, the
// lower four from the second; a magic shop's items 0x4c and 0x4d become empty bottles
// (identified); an alchemist's first six standard places are empty bottles and its
// first six special ones recipes 700..731, neither identified.
//
// mm8: 0x4b9820, 0x4b99e4
func TestShopStockKinds(t *testing.T) {
	h := shopHost(t)
	clickService(openProprietor(t, h, 15), SvcBuyStandard)
	st := h.m.Shops
	for i, it := range st.Standard[15][:8] {
		want := int32(partytest.Leather)
		if i > 3 {
			want = partytest.Chain
		}
		if it.Number != want || st.Special[15][i].Number != want {
			t.Errorf("armour place %d: %d / %d, want %d", i, it.Number, st.Special[15][i].Number, want)
		}
	}
	clickService(openProprietor(t, h, 29), SvcBuyStandard)
	bottles := 0
	for _, it := range st.Standard[29] {
		switch {
		case it.Number == 0x4c || it.Number == 0x4d:
			t.Errorf("magic shop keeps %d", it.Number)
		case it.Number == items.EmptyBottle:
			bottles++
			if it.Flags != items.FlagIdentified {
				t.Errorf("bottle flags %#x", it.Flags)
			}
		}
	}
	if bottles == 0 {
		t.Error("no item 0x4c turned into a bottle")
	}
	clickService(openProprietor(t, h, 42), SvcBuyStandard)
	for i := range 6 {
		if it := st.Standard[42][i]; it.Number != items.EmptyBottle || it.Flags != 0 {
			t.Errorf("alchemist place %d: %v", i, it)
		}
		if it := st.Special[42][i]; it.Number < 700 || it.Number > 731 || it.Flags != 0 {
			t.Errorf("alchemist special place %d: %v", i, it)
		}
	}
	if st.Standard[42][6].Flags != items.FlagIdentified {
		t.Errorf("alchemist place 6: %v", st.Standard[42][6])
	}
}

// TestGuildShelves: a shelf button fills all twelve schools of the guild with books of
// the school's first n spells (n by house), identified, until the restock.
//
// mm8: 0x4bd028 (codes 0x6e..0x76), 0x4bcea6
func TestGuildShelves(t *testing.T) {
	h := shopHost(t)
	d := openProprietor(t, h, 139)
	clickService(d, 0x70) // water
	st := h.m.Shops
	if d.Menu != 0x70 || d.Shelf() != &st.Spells[0][2] {
		t.Fatalf("menu %#x", d.Menu)
	}
	for school := range tables.NumSchools {
		for _, it := range st.Spells[0][school] {
			lo := int32(400 + 11*school)
			if it.Number < lo || it.Number >= lo+4 || it.Flags != items.FlagIdentified {
				t.Fatalf("school %d: %v", school, it)
			}
		}
	}
	if st.SpellRestock[139] != 1000+7*368640 {
		t.Errorf("next restock %d", st.SpellRestock[139])
	}
}

// TestCanTrade: the merchant's reply: not this shop's (ids from 538), the wrong kind,
// stolen, else by the Merchant skill: none, a price above the value, the value.
//
// mm8: 0x491cbd (Shop_CanTrade)
func TestCanTrade(t *testing.T) {
	h := shopHost(t)
	d := openProprietor(t, h, 1)
	p := &h.m.Players[0]
	sword := items.Item{Number: partytest.Sword, Flags: items.FlagIdentified}
	for _, c := range []struct {
		name  string
		it    items.Item
		shop  int
		skill uint16
		want  int
	}{
		{"no skill", sword, TypeWeapons, 0, ReplyNoSkill},
		{"expert 4: 127 > 100", sword, TypeWeapons, party.SkillExpert | 4, ReplyRegular},
		{"grandmaster: the value", sword, TypeWeapons, party.SkillGM | 1, ReplyGood},
		{"chain in a weapon shop", items.Item{Number: partytest.Chain}, TypeWeapons, 0, ReplyWrongType},
		{"sword in an armour shop", sword, TypeArmour, 0, ReplyWrongType},
		{"ring in a magic shop", items.Item{Number: partytest.Ring}, TypeMagic, 0, ReplyNoSkill},
		{"potion at an alchemist", items.Item{Number: partytest.Potion}, TypeAlchemy, 0, ReplyNoSkill},
		{"id 538", items.Item{Number: 538}, TypeWeapons, 0, ReplyNotHere},
		{"id 700 elsewhere", items.Item{Number: 700}, TypeMagic, 0, ReplyNotHere},
		{"id 700 at an alchemist: its kind", items.Item{Number: 700}, TypeAlchemy, 0, ReplyWrongType},
		{"stolen", items.Item{Number: partytest.Sword, Flags: items.FlagStolen}, TypeWeapons, 0, ReplyStolen},
	} {
		p.Skills[party.SkillMerchant] = c.skill
		it := c.it
		if got := d.CanTrade(p, &it, c.shop, ActionBuy); got != c.want {
			t.Errorf("%s: row %d, want %d", c.name, got, c.want)
		}
	}
	p.Skills[party.SkillMerchant] = party.SkillGM | 1
	broken := items.Item{Number: partytest.Sword, Flags: items.FlagBroken}
	if got := d.CanTrade(p, &broken, TypeWeapons, ActionSell); got != ReplyRegular {
		t.Errorf("selling a broken sword (1 gold): row %d", got)
	}
}

// packed puts it at cell in member 0's pack.
func packed(t *testing.T, h *fakeHost, cell int, it items.Item) *items.Item {
	t.Helper()
	p := &h.m.Players[0]
	n := p.AddItem(h.ctx.Items, cell, it)
	if n == 0 {
		t.Fatalf("no room for %v at %d", it, cell)
	}
	return p.Item(n)
}

// TestShopSell: Sell takes the pack item under the mouse (a covered cell finds it) when
// the shop deals in it: its price (1 for a broken item) to the gold, the member says
// 0x4d; another kind, or a stolen item, the member refuses (0x4f).
//
// mm8: 0x4bdcd8 (menu 3), 0x4bdc2f, 0x4be37f
func TestShopSell(t *testing.T) {
	h := shopHost(t)
	d := openProprietor(t, h, 1)
	clickService(d, SvcDisplay)
	if d.Menu != SvcDisplay || !slices.Equal(params(d.Buttons), []int{3, 4, 5}) {
		t.Fatalf("display: %#x %v", d.Menu, params(d.Buttons))
	}
	clickService(d, SvcSell)
	if d.Menu != SvcSell {
		t.Fatalf("menu %#x", d.Menu)
	}
	p := &h.m.Players[0]
	packed(t, h, 0, items.Item{Number: partytest.Sword, Flags: items.FlagIdentified})
	packed(t, h, 2, items.Item{Number: partytest.Chain})
	packed(t, h, 5, items.Item{Number: partytest.Sword, Flags: items.FlagBroken})
	packed(t, h, 7, items.Item{Number: partytest.Sword, Flags: items.FlagStolen})
	d.ClickArea(0, 14) // the sword's second cell
	if h.m.Gold != 1000+28 || p.Grid[0] != 0 || !slices.Equal(h.spoke, []int{0x4d}) || !d.Traded {
		t.Errorf("sold: gold %d, grid %d, spoke %#x", h.m.Gold, p.Grid[0], h.spoke)
	}
	d.ClickArea(0, 5)
	if h.m.Gold != 1000+28+1 {
		t.Errorf("broken sword: gold %d", h.m.Gold)
	}
	h.spoke = nil
	d.ClickArea(0, 2)
	d.ClickArea(0, 7)
	if p.Grid[2] == 0 || p.Grid[7] == 0 || !slices.Equal(h.spoke, []int{0x4f, 0x4f}) {
		t.Errorf("refusals: %#x", h.spoke)
	}
	d.ClickArea(0, 40) // an empty cell
	d.ClickArea(0, -1)
	if h.m.Gold != 1029 {
		t.Errorf("gold %d", h.m.Gold)
	}
	if !d.Back() || d.Menu != SvcDisplay || len(d.Buttons) != 3 {
		t.Errorf("back from Sell: %#x", d.Menu)
	}
	if !d.Back() || d.Menu != 1 {
		t.Errorf("back from Display: %#x", d.Menu)
	}
	d = openProprietor(t, h, 42)
	clickService(d, SvcDisplay)
	if !slices.Equal(params(d.Buttons), []int{3, 4}) {
		t.Errorf("alchemist's display: %v", params(d.Buttons))
	}
}

// TestShopIdentifyRepair: identifying costs Val · 50 and marks the item identified
// (0x49, "Identified"); repairing costs value / (6 - Val) and also identifies it
// (0x4a); an item that needs neither the member says 0x4c about; without the gold
// nothing happens.
//
// mm8: 0x4bdcd8 (menus 4, 5)
func TestShopIdentifyRepair(t *testing.T) {
	h := shopHost(t)
	d := openProprietor(t, h, 1)
	clickService(d, SvcDisplay)
	clickService(d, SvcIdentify)
	a := packed(t, h, 0, items.Item{Number: partytest.Sword})
	b := packed(t, h, 3, items.Item{Number: partytest.Sword, Flags: items.FlagBroken | items.FlagIdentified})
	d.ClickArea(0, 0)
	if !a.Identified() || h.m.Gold != 1000-75 || h.status[len(h.status)-1] != "g0x239" {
		t.Errorf("identify: %v, gold %d, %q", a, h.m.Gold, h.status)
	}
	d.ClickArea(0, 0)
	if h.m.Gold != 925 || h.spoke[len(h.spoke)-1] != 0x4c {
		t.Errorf("identified again: gold %d, spoke %#x", h.m.Gold, h.spoke)
	}
	clickService(d, SvcRepair)
	d.ClickArea(0, 3)
	if b.Broken() || !b.Identified() || h.m.Gold != 925-22 || h.spoke[len(h.spoke)-1] != 0x4a {
		t.Errorf("repair: %v, gold %d (100 / 4.5), spoke %#x", b, h.m.Gold, h.spoke)
	}
	d.ClickArea(0, 0)
	if h.spoke[len(h.spoke)-1] != 0x4c {
		t.Errorf("unbroken: %#x", h.spoke)
	}
	b.Flags = items.FlagBroken
	h.m.Gold = 5
	d.ClickArea(0, 3)
	if !b.Broken() || h.m.Gold != 5 {
		t.Errorf("no gold: %v %d", b, h.m.Gold)
	}
}

// TestShopBuy: a shelf item goes into the first place it fits in the selected member's
// pack, identified, for its price; without the gold "You don't have enough gold", with a
// full pack the member says 0xf and "No room".
//
// mm8: 0x4bdcd8 (menus 2, 0x5f)
func TestShopBuy(t *testing.T) {
	h := shopHost(t)
	d := openProprietor(t, h, 1)
	clickService(d, SvcBuyStandard)
	sh := d.Shelf()
	sh[2] = items.Item{Number: partytest.Sword, Bonus: 1, Strength: 2}
	price := BuyPrice(300, 1.5, 0) // 100 + 100 · 2
	d.ClickArea(3, -1)
	p := &h.m.Players[0]
	got := p.Item(p.Grid[0])
	if got == nil || got.Number != partytest.Sword || got.Strength != 2 || !got.Identified() ||
		sh[2].Number != 0 || h.m.Gold != int32(1000-price) || h.spoke[len(h.spoke)-1] != 0x4b {
		t.Fatalf("bought %v, shelf %v, gold %d (price %d)", got, sh[2], h.m.Gold, price)
	}
	h.m.Gold = 10
	d.ClickArea(1, -1)
	if sh[0].Number == 0 || h.status[len(h.status)-1] != "g0x9b" {
		t.Errorf("no gold: %v %q", sh[0], h.status)
	}
	h.m.Gold = 100000
	for c := range items.GridCells {
		p.Grid[c] = -1
	}
	d.ClickArea(1, -1)
	if sh[0].Number == 0 || h.status[len(h.status)-1] != "g0x233" || h.spoke[len(h.spoke)-1] != 0xf {
		t.Errorf("no room: %v %q %#x", sh[0], h.status, h.spoke)
	}
	d.ClickArea(5, -1) // place 4: no pick on an empty place
	d.ClickArea(0, -1)
	p.Conditions[party.CondAsleep] = 1
	gold := h.m.Gold
	d.ClickArea(1, -1)
	if h.m.Gold != gold || h.notes[len(h.notes)-1] != "the error sound 0x1b (M11)" {
		t.Errorf("asleep: %d %q", h.m.Gold, h.notes)
	}
}

// TestShopTexts: the replies fill %06 (sir or lady), %24 (the item's name in yellow),
// %25 (the price before the discount), %27 (the price), %28 (the house's title) and %29
// (identifying's price); the shelf's wait reads days, hours, and minutes or seconds.
//
// mm8: 0x4955a7 (Text_Format), 0x4b239d
func TestShopTexts(t *testing.T) {
	h := shopHost(t)
	d := openProprietor(t, h, 1)
	sword := items.Item{Number: partytest.Sword, Flags: items.FlagIdentified}
	got := d.FormatText("Sorry, %06. %24: %25 / %27 here at the %28; %29 to identify, %99%", &sword, ActionBuy)
	if want := "Sorry, g0x181. \f65523Sword\f00000: 150 / 150 here at the Smith; 75 to identify, 99%"; got != want {
		t.Errorf("%q\nwant %q", got, want)
	}
	if got := d.FormatText("%27", &sword, ActionSell); got != "28" {
		t.Errorf("sell price %q", got)
	}
	h.m.Players[0].Face = 1
	if got := d.FormatText("%06", nil, ActionBuy); got != "g0x183" {
		t.Errorf("lady: %q", got)
	}
	clickService(d, SvcBuyStandard)
	st := h.m.Shops
	st.Restock[1] = h.m.Time + clock.Time((24*3600+3600+120)*64/15)
	if got := d.RestockText(); got != "g0x2141 g0x38 1 g0x6d " {
		t.Errorf("a day, an hour, 2 minutes: %q", got)
	}
	st.Restock[1] = h.m.Time + clock.Time(210*64/15)
	if got := d.RestockText(); got != "g0x2143 g0x1b4 30 g0x1b6 " {
		t.Errorf("3 minutes 30 seconds: %q", got)
	}
	if got := d.ShelfReply(0); !strings.HasPrefix(got, "buy \f65523") {
		t.Errorf("shelf reply %q", got)
	}
}
