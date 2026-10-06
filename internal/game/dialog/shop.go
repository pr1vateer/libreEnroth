package dialog

import (
	"fmt"
	"strconv"
	"strings"

	"libre-enroth/internal/assets/tables"
	"libre-enroth/internal/game/clock"
	"libre-enroth/internal/game/items"
	"libre-enroth/internal/game/party"
)

// The shops (weapons, armour, magic, alchemists) and the guilds' spell shelves: their
// stock and restocking, the prices and the merchant's replies, and buying, selling,
// identifying and repairing (re/notes/houses.md#shops).

// Shop actions: what Shop_CanTrade and Text_Format price (the merchant.txt columns).
const (
	ActionBuy      = 2
	ActionSell     = 3
	ActionIdentify = 4
	ActionRepair   = 5
)

// Merchant reply rows (merchant.txt; Shop_CanTrade's result).
const (
	ReplyNoGold    = 0
	ReplyNoSkill   = 1 // the price without the Merchant skill
	ReplyRegular   = 2
	ReplyGood      = 3 // the price is what the item is worth
	ReplyWrongType = 4
	ReplyNotHere   = 5 // beyond this merchant
	ReplyStolen    = 6
)

// Items the shops treat apart.
const (
	maxTradedID      = 0x219 // 538.. are not traded (but the alchemists' 700..731)
	alchemyScrollLo  = 700
	alchemyScrollHi  = 0x2db
	sellScrollLo     = 0x2e4 // Shop_CanSellHere's message scrolls (never reached)
	sellScrollHi     = 0x303
	magicSwapLo      = 0x4c // a magic shop's standard shelf turns these into bottles
	magicSwapHi      = 0x4d
	alchemyRecipes   = 700 // an alchemist's special shelf: 700 + rand() % 32
	numAlchemyRecipe = 32
	alchemyBottles   = 6 // an alchemist's first six slots
)

// Shops are the shop tables (the shelf pictures and places the screens draw by).
func (d *Dialog) Shops() *tables.Shops { return d.h.Tables().Shops }

// IsShop reports a shop (house types 1..4); IsGuild a guild (0xc..0xf).
func (d *Dialog) IsShop() bool  { return d.Type >= TypeWeapons && d.Type <= TypeAlchemy }
func (d *Dialog) IsGuild() bool { return d.Type >= 0xc && d.Type <= 0xf }

// ShelfMenu reports a menu showing a shelf: a shop's Buy Standard or Buy Special, or a
// guild's school.
func (d *Dialog) ShelfMenu() bool {
	switch {
	case d.IsShop():
		return d.Menu == SvcBuyStandard || d.Menu == SvcBuySpecial
	case d.IsGuild():
		return d.Menu >= SvcGuild0 && d.Menu <= SvcGuild0+8
	}
	return false
}

// PackMenu reports a shop menu showing the selected member's pack: Display Inventory
// and its Sell, Identify and Repair.
func (d *Dialog) PackMenu() bool {
	if !d.IsShop() {
		return false
	}
	switch d.Menu {
	case SvcDisplay, SvcSell, SvcIdentify, SvcRepair:
		return true
	}
	return false
}

// Shelf is the shelf the menu shows, nil for none.
func (d *Dialog) Shelf() *[tables.ShopSlots]items.Item {
	st := d.h.Members().ShopState()
	switch {
	case d.IsShop() && d.House >= 0 && d.House < tables.NumShopHouses:
		switch d.Menu {
		case SvcBuyStandard:
			return &st.Standard[d.House]
		case SvcBuySpecial:
			return &st.Special[d.House]
		}
	case d.IsGuild() && d.ShelfMenu():
		if g := d.House - tables.FirstGuild; g >= 0 && g < tables.NumGuilds {
			return &st.Spells[g][d.Menu-SvcGuild0]
		}
	}
	return nil
}

// ShelfSize is the number of places the shelf shows: a shop type's count (weapons 6,
// armour 8, magic and alchemists 12), a guild's 12.
func (d *Dialog) ShelfSize() int {
	if d.IsGuild() {
		return tables.ShopSlots
	}
	return d.h.Tables().Shops.Count(d.Type)
}

// ShelfEmpty reports a shelf with nothing to buy, which shows when it restocks. The
// weapon and armour shops look at their first six places only (the armour shop's
// last two can still hold items).
//
// mm8: 0x4b9b5b, 0x4bb328 (6), 0x4b5e2c, 0x4ba6f6, 0x4b6b98 (12)
func (d *Dialog) ShelfEmpty() bool {
	sh := d.Shelf()
	if sh == nil {
		return true
	}
	n := tables.ShopSlots
	if d.Type == TypeWeapons || d.Type == TypeArmour {
		n = 6
	}
	for _, it := range sh[:n] {
		if it.Number != 0 {
			return false
		}
	}
	return true
}

// restockDelay is a shop's or guild's time between restocks: the 2DEvents C column in
// days, ftol(C · 0xa8c000 · (1/30)f) ticks.
//
// mm8: 0x4bd028 (0x4bd5d9, 0x4bd8ca)
func restockDelay(c int16) clock.Time {
	return clock.Time(int64(float64(int32(c)*0xa8c000) * float64(float32(1.0/30))))
}

// rng is the party's rand().
func (d *Dialog) rng() items.Rand { return d.h.Ctx().Rand }

// generate is ItemGen_Generate(level, kind). Only the placeholder houses have level 0
// (which the original reads before its tables); it counts as 1 here.
func (d *Dialog) generate(level, kind int) items.Item {
	m := d.h.Members()
	return items.Generate(d.h.Ctx().Items, max(level, 1), kind, false, d.rng(), &m.ArtifactsFound)
}

// restockShop fills a shop's two shelves when their time is past (the standard one
// first), and sets the next restock.
//
// mm8: 0x4bd028 (codes 2, 0x5f)
func (d *Dialog) restockShop() {
	h := d.House
	m := d.h.Members()
	st := m.ShopState()
	if h < 0 || h >= tables.NumShopHouses || st.Restock[h] >= m.Time {
		return
	}
	d.stockStandard(&st.Standard[h])
	d.stockSpecial(&st.Special[h])
	st.Restock[h] = m.Time + restockDelay(d.Def.C)
}

// stockStandard fills a shop's standard shelf: weapons and armour from their records
// (one of four kinds at random; the armour shop's slots 4.. from its second record),
// magic shops misc items (items 0x4c and 0x4d become empty bottles), alchemists six
// empty bottles and reagents; all identified.
//
// mm8: 0x4b9820 (Shop_GenerateStandard)
func (d *Dialog) stockStandard(sh *[tables.ShopSlots]items.Item) {
	h, t := d.House, d.h.Tables().Shops
	for i := range t.Count(int(d.Def.Type)) {
		switch {
		case h >= 1 && h <= 0xe:
			s := t.StdWeapons(h)
			sh[i] = d.generate(s.Level, s.Kinds[d.rng().Int()%4])
		case h >= 0xf && h <= 0x1c:
			s := t.StdArmour(h, row(i))
			sh[i] = d.generate(s.Level, s.Kinds[d.rng().Int()%4])
		case h >= 0x1d && h <= 0x29:
			sh[i] = d.generate(t.MagicLevel(h, false), 0x16)
			if n := sh[i].Number; n == magicSwapLo || n == magicSwapHi {
				sh[i] = items.Item{Number: items.EmptyBottle}
			}
		case h >= 0x2a && h <= 0x35:
			if i < alchemyBottles {
				sh[i] = items.Item{Number: items.EmptyBottle}
				continue
			}
			sh[i] = d.generate(t.AlchemyLevel(h, false), 0x2d)
		default:
			continue
		}
		sh[i].Flags = items.FlagIdentified
	}
}

// row is the armour shelf (record) of slot i.
func row(i int) int {
	if i > 3 {
		return 1
	}
	return 0
}

// stockSpecial fills a shop's special shelf: weapons and armour from their records,
// magic shops misc items, alchemists six recipes (700 + rand() % 32, not identified)
// and potions.
//
// mm8: 0x4b99e4 (Shop_GenerateSpecial)
func (d *Dialog) stockSpecial(sh *[tables.ShopSlots]items.Item) {
	h, t := d.House, d.h.Tables().Shops
	for i := range t.Count(int(d.Def.Type)) {
		switch {
		case h < 0xf:
			s := t.SpcWeapons(h)
			sh[i] = d.generate(s.Level, s.Kinds[d.rng().Int()%4])
		case h < 0x1d:
			s := t.SpcArmour(h, row(i))
			sh[i] = d.generate(s.Level, s.Kinds[d.rng().Int()%4])
		case h < 0x2a:
			sh[i] = d.generate(t.MagicLevel(h, true), 0x16)
		case h < 0x36:
			if i < alchemyBottles {
				sh[i] = items.Item{Number: int32(alchemyRecipes + d.rng().Int()%numAlchemyRecipe)}
				continue
			}
			sh[i] = d.generate(t.AlchemyLevel(h, true), 0x2c)
		default:
			continue
		}
		sh[i].Flags = items.FlagIdentified
	}
}

// rollShelfY is the weapon shop's table: each weapon lies rand() % (300 - its height)
// down, drawn again at every Buy click.
//
// mm8: 0x4bd028 (g_weaponShelfY 0xffd2bc)
func (d *Dialog) rollShelfY() {
	if d.Type != TypeWeapons {
		return
	}
	sh, t := d.Shelf(), d.h.Ctx().Items
	if sh == nil {
		return
	}
	for i := range d.h.Tables().Shops.Count(int(d.Def.Type)) {
		if sh[i].Number != 0 {
			d.ShelfY[i] = d.rng().Int() % (300 - sh[i].Def(t).PicH)
		}
	}
}

// restockGuild fills all twelve schools of a guild's shelves when their time is past:
// twelve books each of the school's first n (the house's count), identified.
//
// mm8: 0x4bd028 (codes 0x6e..0x76), 0x4bcea6 (Guild_GenerateSpells)
func (d *Dialog) restockGuild() {
	h := d.House
	m := d.h.Members()
	st := m.ShopState()
	if h < 0 || h >= len(st.SpellRestock) || st.SpellRestock[h] >= m.Time {
		return
	}
	if typ := d.Def.Type; typ >= 0xc && typ <= 0xf {
		g := h - tables.FirstGuild
		n := d.h.Tables().Shops.GuildSpellCount(h)
		for school := range tables.NumSchools {
			for slot := range tables.ShopSlots {
				r := d.rng().Int()
				if g < 0 || g >= tables.NumGuilds || n == 0 {
					continue // the original divides by zero (no guild has 0)
				}
				st.Spells[g][school][slot] = items.Item{
					Number: int32(tables.FirstSpellBook + tables.SpellsPerSchool*school + r%n),
					Flags:  items.FlagIdentified,
				}
			}
		}
	}
	st.SpellRestock[h] = m.Time + restockDelay(d.Def.C)
}

// RestockLeft is the time until the shelf shown restocks.
func (d *Dialog) RestockLeft() clock.Time {
	m := d.h.Members()
	st := m.ShopState()
	switch {
	case d.IsGuild():
		if d.House >= 0 && d.House < len(st.SpellRestock) {
			return st.SpellRestock[d.House] - m.Time
		}
	case d.House >= 0 && d.House < tables.NumShopHouses:
		return st.Restock[d.House] - m.Time
	}
	return 0
}

// RestockText is the wait an empty shelf shows: "Come back in" and the days, hours,
// minutes (without days) and seconds (without hours) of ftol(left · 0.234375).
//
// mm8: 0x4b239d (House_DrawRestockWait)
func (d *Dialog) RestockText() string {
	g := d.h.Global
	secs := int64(float64(d.RestockLeft()) * 0.234375)
	mins := secs / 60
	hours := mins / 60
	days := uint32(hours) / 24
	s, mi, hr := secs%60, mins%60, hours%24
	var b strings.Builder
	b.WriteString(g(0x214))
	part := func(n int64, one, many int) {
		w := g(one)
		if n > 1 {
			w = g(many)
		}
		fmt.Fprintf(&b, "%d %s ", n, w)
	}
	if days != 0 {
		w := g(0x39)
		if days < 2 {
			w = g(0x38)
		}
		fmt.Fprintf(&b, "%d %s ", int32(days), w)
	}
	if hr != 0 {
		part(hr, 0x6d, 0x6e)
	}
	if mi != 0 && days == 0 {
		part(mi, 0x1b5, 0x1b4)
	}
	if s != 0 && hr == 0 {
		part(s, 0x1b7, 0x1b6)
	}
	return b.String()
}

// ---- prices ---------------------------------------------------------------------------

// merchantPrice is a price less the merchant discount, at least a third of it.
//
// mm8: 0x4b28ab (Merchant_Price)
func merchantPrice(price, discount int) int {
	return max((100-discount)*price/100, price/3)
}

// BuyPrice is a shop's price for an item worth value: ftol(value · Val) less the
// discount, at least the value, at least 1.
//
// mm8: 0x4b8be3 (Shop_BuyPrice)
func BuyPrice(value int32, val float32, discount int) int {
	p := int(int64(float64(value)*float64(val))) * (100 - discount) / 100
	return max(p, int(value), 1)
}

// SellPrice is what a shop pays: ftol(value / (Val + 2)) plus the discount's share of
// the value, at most the value, at least 1.
//
// mm8: 0x4b8ba3 (Shop_SellPrice)
func SellPrice(value int32, val float32, discount int) int {
	p := discount*int(value)/100 + int(int64(float64(value)/float64(val+2)))
	return max(min(p, int(value)), 1)
}

// IdentifyPrice is Val · 50 less the discount, at least 1.
//
// mm8: 0x4b8c1a (Shop_IdentifyPrice)
func IdentifyPrice(val float32, discount int) int {
	return max(merchantPrice(int(int64(float64(val)*50)), discount), 1)
}

// RepairPrice is value / (6 - Val) less the discount, at least 1.
//
// mm8: 0x4b8c40 (Shop_RepairPrice)
func RepairPrice(value int32, val float32, discount int) int {
	return max(merchantPrice(int(int64(float64(value)/(6-float64(val)))), discount), 1)
}

// BasePrice is the price before the discount the replies quote (%25): buy value · Val,
// sell value / (Val + 2), identify Val · 50, repair value / (6 - Val), at least 1.
//
// mm8: 0x4b8c8c, 0x4b8c6c, 0x4b8ca4, 0x4b8cbe (Shop_Base*Price)
func BasePrice(action int, value int32, val float32) int {
	var p int64
	switch action {
	case ActionSell:
		p = int64(float64(value) / float64(val+2))
	case ActionIdentify:
		p = int64(float64(val) * 50)
	case ActionRepair:
		p = int64(float64(value) / (6 - float64(val)))
	default:
		p = int64(float64(value) * float64(val))
	}
	return max(int(p), 1)
}

// reputation is the map's reputation (Map_Reputation 0x477ab0: the location's +8).
func (d *Dialog) reputation() int {
	if loc := d.h.Location(); len(loc) >= 12 {
		return int(le32(loc[8:]))
	}
	return 0
}

// discount is the selected member's merchant discount (0 with nobody selected).
//
// mm8: 0x491f5e
func (d *Dialog) discount() int {
	p := d.selected()
	if p == nil {
		return 0
	}
	return p.MerchantDiscount(d.h.Members().Env(d.h.Ctx()), d.reputation())
}

// merchant is a price less the selected member's merchant discount.
//
// mm8: 0x4b28ab (Merchant_Price)
func (d *Dialog) merchant(price int) int { return merchantPrice(price, d.discount()) }

// Price is what the selected member pays (or gets) for action on it here; a broken item
// sells for 1.
//
// mm8: 0x491cbd / 0x4955a7 (the price by action), 0x4be37f (selling)
func (d *Dialog) Price(action int, it *items.Item) int {
	v, val, disc := it.Value(d.h.Ctx().Items), d.val(), d.discount()
	switch action {
	case ActionSell:
		if it.Broken() {
			return 1
		}
		return SellPrice(v, val, disc)
	case ActionIdentify:
		return IdentifyPrice(val, disc)
	case ActionRepair:
		return RepairPrice(v, val, disc)
	}
	return BuyPrice(v, val, disc)
}

// ---- what a shop deals in ------------------------------------------------------------

// tradedID reports an item id shops deal in: below 538, and the alchemists' 700..731.
func tradedID(id int32, alchemist bool) bool {
	if alchemist && id >= alchemyScrollLo && id <= alchemyScrollHi {
		return true
	}
	return id <= maxTradedID
}

// CanTrade is the merchant.txt row of the merchant's reply to action on it in shop
// type shop (1 weapons, 2 armour, 3 magic and the guilds, 4 alchemists): not this
// shop's item, the wrong kind, stolen, else the price without the Merchant skill, at
// the item's value, or below it.
//
// mm8: 0x491cbd (Shop_CanTrade)
func (d *Dialog) CanTrade(p *party.Player, it *items.Item, shop, action int) int {
	t := d.h.Ctx().Items
	def := it.Def(t)
	typ := int(def.EquipType)
	if !tradedID(it.Number, shop == TypeAlchemy) {
		return ReplyNotHere
	}
	checked := true
	switch shop {
	case TypeWeapons:
		if typ > tables.EquipMissile {
			return ReplyWrongType
		}
	case TypeArmour:
		if typ < tables.EquipArmor || typ > tables.EquipBoots {
			return ReplyWrongType
		}
	case TypeMagic:
		if def.Skill != tables.SkillMisc && typ != tables.EquipBook {
			return ReplyWrongType
		}
	case TypeAlchemy:
		if typ < tables.EquipReagent || typ > tables.EquipPotion && typ != tables.EquipMessageScroll {
			return ReplyWrongType
		}
	default:
		checked = false
	}
	if checked && it.Flags&items.FlagStolen != 0 {
		return ReplyStolen
	}
	price := d.Price(action, it)
	if p == nil || p.Skill(d.h.Members().Env(d.h.Ctx()), party.SkillMerchant) == 0 {
		return ReplyNoSkill
	}
	if price == int(it.Value(t)) {
		return ReplyGood
	}
	return ReplyRegular
}

// CanSellHere reports an item the shop takes (to buy, identify or repair): Shop_CanTrade's
// tests by the house's 2DEvents type, stolen items refused; the alchemist's message
// scrolls would have to be 740..771, which the id test has already turned away.
//
// mm8: 0x4bdc2f (Shop_CanSellHere)
func (d *Dialog) CanSellHere(it *items.Item) bool {
	def := it.Def(d.h.Ctx().Items)
	typ, id := int(def.EquipType), it.Number
	shop := int(d.Def.Type)
	if !tradedID(id, shop == TypeAlchemy) || it.Flags&items.FlagStolen != 0 {
		return false
	}
	switch shop {
	case TypeWeapons:
		return typ <= tables.EquipMissile
	case TypeArmour:
		return typ >= tables.EquipArmor && typ <= tables.EquipBoots
	case TypeMagic:
		return def.Skill == tables.SkillMisc || typ == tables.EquipBook
	case TypeAlchemy:
		switch {
		case typ < tables.EquipReagent:
			return false
		case typ <= tables.EquipPotion:
			return true
		case typ == tables.EquipMessageScroll:
			return id >= sellScrollLo && id <= sellScrollHi
		}
	}
	return false
}

// tradeShop is the shop type the draws pass Shop_CanTrade: the house type, 3 in the
// guilds.
func (d *Dialog) tradeShop() int {
	if d.IsGuild() {
		return TypeMagic
	}
	return d.Type
}

// ShelfReply is the merchant's reply to the shelf item in slot (0-based), "" for none.
//
// mm8: 0x4b9b5b.. (menus 2, 0x5f), 0x4b6b98 (the guild shelves)
func (d *Dialog) ShelfReply(slot int) string {
	sh := d.Shelf()
	if sh == nil || slot < 0 || slot >= len(sh) || sh[slot].Number == 0 {
		return ""
	}
	it := &sh[slot]
	row := d.CanTrade(d.selected(), it, d.tradeShop(), ActionBuy)
	return d.FormatText(d.h.Tables().Merchant.Buy[row], it, ActionBuy)
}

// PackReply is the reply to pack item n (1-based) of the selected member in the sell,
// identify and repair menus: an identified item's name in Identify, nothing for an
// unbroken one in Repair.
//
// mm8: 0x4b9b5b.. (menus 3, 4, 5)
func (d *Dialog) PackReply(n int32) string {
	p := d.selected()
	if p == nil {
		return ""
	}
	it := p.Item(n)
	if it == nil || it.Number == 0 {
		return ""
	}
	mt := d.h.Tables().Merchant
	switch d.Menu {
	case SvcSell:
		return d.FormatText(mt.Sell[d.CanTrade(p, it, d.Type, ActionSell)], it, ActionSell)
	case SvcIdentify:
		if it.Identified() {
			return d.FormatText("%24", it, ActionIdentify)
		}
		return d.FormatText(mt.Identify[d.CanTrade(p, it, d.Type, ActionIdentify)], it, ActionIdentify)
	case SvcRepair:
		if !it.Broken() {
			return ""
		}
		return d.FormatText(mt.Repair[d.CanTrade(p, it, d.Type, ActionRepair)], it, ActionRepair)
	}
	return ""
}

// inkItemName is the colour of an item's name in the replies (Color16(255, 255, 155)).
const inkItemName = 0xfff3

// FormatText fills the %NN codes of a text for the selected member, item it and
// action: 02 the member, 06 / 07 / 10 sir or lady and the like, 23 the map, 24 the
// item's name in yellow, 25 the price before the discount, 27 the price, 28 the house's
// title, 29 identifying's price, 31..34 the first member's name and gender words; the
// NPC, award and date codes are not used by the shops (they read the number).
//
// mm8: 0x4955a7 (Text_Format)
func (d *Dialog) FormatText(f string, it *items.Item, action int) string {
	g := d.h.Global
	m := d.h.Members()
	p := d.selected()
	var first *party.Player
	if len(m.Players) > 0 {
		first = &m.Players[0]
	}
	female := func(p *party.Player) bool { return p != nil && party.IsFemale(p.Face) }
	pick := func(fem bool, man, woman int) string {
		if fem {
			return g(woman)
		}
		return g(man)
	}
	var b strings.Builder
	for i := 0; i < len(f); i++ {
		if f[i] != '%' || i+2 >= len(f) {
			b.WriteByte(f[i])
			continue
		}
		code := int(f[i+1]-'0')*10 + int(f[i+2]-'0')
		i += 2
		switch {
		case code == 2 && p != nil:
			b.WriteString(p.Name)
		case code == 6:
			b.WriteString(pick(female(p), 0x181, 0x183))
		case code == 7:
			b.WriteString(pick(female(p), 0x182, 0x185))
		case code == 10:
			b.WriteString(pick(female(p), 0x184, 0x185))
		case code == 23:
			name := g(0x18a)
			if k := d.h.MapStatsIndex(d.h.MapName()); k != 0 {
				name = d.h.MapStatsName(k)
			}
			b.WriteString(name)
		case code == 24 && it != nil:
			fmt.Fprintf(&b, "\f%05d%s\f00000", inkItemName, it.Name(d.h.Ctx().Items, m.Namer(noteStatus{d.h})))
		case code == 25 && it != nil:
			b.WriteString(strconv.Itoa(BasePrice(action, it.Value(d.h.Ctx().Items), d.val())))
		case code == 27 && it != nil:
			b.WriteString(strconv.Itoa(d.Price(action, it)))
		case code == 28:
			b.WriteString(d.Def.Title)
		case code == 29:
			b.WriteString(strconv.Itoa(IdentifyPrice(d.val(), d.discount())))
		case code == 31 && first != nil:
			b.WriteString(first.Name)
		case code == 32:
			b.WriteString(pick(female(first), 0x17f, 0x180))
		case code == 33:
			b.WriteString(pick(female(first), 0x2de, 0x2df))
		case code == 34:
			b.WriteString(pick(female(first), 0x2e0, 0x180))
		default:
			n, _ := strconv.Atoi(f[i-1 : i+1])
			b.WriteString(strconv.Itoa(n))
		}
	}
	return b.String()
}

// noteStatus makes a Host the party.Notes that item names need (global.txt only).
type noteStatus struct{ h Host }

func (n noteStatus) Global(i int) string             { return n.h.Global(i) }
func (n noteStatus) Status(text string, seconds int) { n.h.Status(text, seconds) }
func (n noteStatus) Stub(key, what string)           { n.h.Note(what) }
func (n noteStatus) AutonoteText(int) bool           { return false }

// ---- clicks ---------------------------------------------------------------------------

// displayMenu are the Display Inventory buttons: Sell, Identify, Repair (the alchemists
// have no Repair).
//
// mm8: 0x4b4900 (House_DisplayMenu)
func displayMenu(typ int) []Button {
	switch {
	case typ >= TypeWeapons && typ < TypeAlchemy:
		return serviceButtons(SvcSell, SvcIdentify, SvcRepair)
	case typ == TypeAlchemy:
		return serviceButtons(SvcSell, SvcIdentify)
	}
	return nil
}

// ClickArea is a click on the left area of a shop or guild menu (msg 0x51): pick is the
// pick buffer's id under the mouse (a shelf place + 1, or a pack slot), cell the pack
// cell under it (-1 outside the pack's columns). A member who cannot act gets the error
// sound; a shelf item is bought; Display Inventory moves the pack's items; Sell,
// Identify and Repair act on the pack item under the mouse.
//
// mm8: 0x4bdcd8 (House_ClickVideoArea)
func (d *Dialog) ClickArea(pick int32, cell int) {
	if !d.IsShop() && !d.IsGuild() {
		return
	}
	m := d.h.Members()
	p := d.selected()
	if p == nil || d.Blocked() {
		d.h.Note("the error sound 0x1b (M11)")
		return
	}
	t := d.h.Ctx().Items
	switch {
	case d.ShelfMenu():
		if pick > 0 {
			d.buy(int(pick) - 1)
		}
	case d.Menu == SvcDisplay:
		m.CharPage = 0x67
		if pick != 0 || cell >= 0 {
			m.ClickPack(m.Selected-1, t, pick, cell, d.h.Ctx())
		}
	case d.Menu == SvcSell, d.Menu == SvcIdentify, d.Menu == SvcRepair:
		if cell < 0 {
			return
		}
		n, top := p.ItemAtCell(cell)
		if n == 0 {
			return
		}
		switch d.Menu {
		case SvcSell:
			d.sell(p, n, top)
		case SvcIdentify:
			d.identify(p.Item(n))
		case SvcRepair:
			d.repair(p.Item(n))
		}
	case d.Menu >= 0x24 && d.Menu <= 0x4a:
		d.h.Note(deferred(d.Type, d.Menu))
	}
}

// buy buys the item in shelf place slot: for its price (else "You don't have enough
// gold"), into the first place it fits in the selected member's pack (else the member
// says so and "No room in pack", 5 s in a guild), identified.
//
// mm8: 0x4bdcd8 (menus 2, 0x5f, 0x6e..0x76)
func (d *Dialog) buy(slot int) {
	sh := d.Shelf()
	if sh == nil || slot < 0 || slot >= len(sh) || sh[slot].Number == 0 {
		return
	}
	m, p, t := d.h.Members(), d.selected(), d.h.Ctx().Items
	it := sh[slot]
	price := BuyPrice(it.Value(t), d.val(), d.discount())
	if uint32(m.Gold) < uint32(price) {
		d.h.Note("the house sound 2 (M11)")
		d.h.Status(d.h.Global(0x9b), 2)
		return
	}
	n, _ := p.AddItemNumber(t, -1, it.Number)
	if n == 0 {
		d.speak(0xf)
		secs := 2
		if d.IsGuild() {
			secs = 5
		}
		d.h.Status(d.h.Global(0x233), secs)
		return
	}
	it.Flags |= items.FlagIdentified
	*p.Item(n) = it
	d.Traded = true
	m.TakeGold(uint32(price))
	sh[slot] = items.Item{}
	d.speak(0x4b)
}

// sell sells pack item n (top-left cell top) when the shop takes it (else the member
// says 0x4f): its price (a broken item 1) goes to the gold.
//
// mm8: 0x4bdcd8 (menu 3), 0x4be37f (Shop_SellItem)
func (d *Dialog) sell(p *party.Player, n int32, top int) {
	it := p.Item(n)
	if !d.CanSellHere(it) {
		d.speak(0x4f)
		d.h.Note("the error sound 0x1b (M11)")
		return
	}
	d.Traded = true
	price := d.Price(ActionSell, it)
	p.RemoveItemAt(d.h.Ctx().Items, top)
	m := d.h.Members()
	m.Gold += int32(price)
	d.h.Note("the gold sound 0x85 (M11)")
	d.speak(0x4d)
}

// identify identifies a pack item for the price: an identified one the member says
// 0x4c about, one the shop does not take 0x4f; without the gold, the house sound.
//
// mm8: 0x4bdcd8 (menu 4)
func (d *Dialog) identify(it *items.Item) {
	price := IdentifyPrice(d.val(), d.discount())
	switch {
	case it.Identified():
		d.speak(0x4c)
	case !d.CanSellHere(it):
		d.h.Note("the error sound 0x1b (M11)")
		d.speak(0x4f)
	case uint32(price) <= uint32(d.h.Members().Gold):
		d.Traded = true
		d.h.Members().TakeGold(uint32(price))
		it.Flags |= items.FlagIdentified
		d.speak(0x49)
		d.h.Status(d.h.Global(0x239), 2)
	default:
		d.h.Note("the house sound 2 (M11)")
	}
}

// repair repairs a broken pack item for the price, which also identifies it: an unbroken
// one the member says 0x4c about, one the shop does not take 0x4f.
//
// mm8: 0x4bdcd8 (menu 5)
func (d *Dialog) repair(it *items.Item) {
	price := RepairPrice(it.Value(d.h.Ctx().Items), d.val(), d.discount())
	switch {
	case !it.Broken():
		d.speak(0x4c)
	case !d.CanSellHere(it):
		d.h.Note("the error sound 0x1b (M11)")
		d.speak(0x4f)
	case uint32(price) <= uint32(d.h.Members().Gold):
		d.Traded = true
		d.h.Members().TakeGold(uint32(price))
		it.Flags = it.Flags&^items.FlagBroken | items.FlagIdentified
		d.speak(0x4a)
		d.h.Status(d.h.Global(0x23a), 2)
	default:
		d.h.Note("the house sound 2 (M11)")
	}
}
