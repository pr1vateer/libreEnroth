package dialog

import (
	"libre-enroth/internal/assets/tables"
	"libre-enroth/internal/game/party"
)

// House types the house screen goes by (g_houseAnims[video].type, 0xffd2fc).
const (
	TypeWeapons  = 1
	TypeArmour   = 2
	TypeMagic    = 3
	TypeAlchemy  = 4
	TypeTownHall = 0x11
	TypeTavern   = 0x15
	TypeBank     = 0x16
	TypeTemple   = 0x17
	TypeStables  = 0x1b
	TypeBoats    = 0x1c
	TypeTraining = 0x1e
	TypePrison   = 0x1f
	TypeInn      = 0x23
)

// Service codes: the msgParam of the proprietors' buttons (msg 0x195), which become the
// sub-menu (g_houseMenu 0xffd300) they open.
const (
	SvcBuyStandard     = 2
	SvcSell            = 3
	SvcIdentify        = 4
	SvcRepair          = 5
	SvcDeposit         = 7
	SvcWithdraw        = 8
	SvcHeal            = 10
	SvcDonate          = 0xb
	SvcRentRoom        = 0xf
	SvcBuyFood         = 0x10
	SvcTrain           = 0x11
	SvcBounty          = 99
	SvcPayFine         = 100
	SvcDisplay         = 0x5e
	SvcBuySpecial      = 0x5f
	SvcLearn           = 0x60
	SvcArcomage        = 0x65 // the tavern's Arcomage menu
	SvcArcomageRules   = 0x66
	SvcArcomageVictory = 0x67
	SvcArcomagePlay    = 0x68
	SvcRoute0          = 0x69 // ..0x6c: a stable's or boat's routes
	SvcGuild0          = 0x6e // ..0x76: a guild's spell shelves
)

// ServiceMenu is a proprietor's menu for house type typ: the service codes of its
// buttons (msg 0x195), at x 0x1e0, y 0x92 + 0x1e*i. Shops: buy standard 2, buy special
// 0x5f, display/sell 0x5e, learn skills 0x60; guilds 0xc/0xd/0xe/0xf; tavern 0xf 0x10
// 0x60 0x65; bank 7 8; temple 10 0xb 0x60; stables and boats 0x69..0x6c; training 0x11
// 0x60; the town hall 99, and 100 while there is a fine to pay.
//
// mm8: 0x4b496e (with 0x4b4397 adding each button)
func ServiceMenu(typ int, m *party.Members) []Button {
	var codes []int
	switch {
	case typ == TypeTownHall:
		codes = []int{SvcBounty}
		if m.Fine != 0 {
			codes = append(codes, SvcPayFine)
		}
	case typ >= TypeWeapons && typ <= TypeAlchemy:
		codes = []int{SvcBuyStandard, SvcBuySpecial, SvcDisplay, SvcLearn}
	case typ == 0xc:
		codes = []int{0x75, SvcLearn}
	case typ == 0xd:
		codes = []int{0x76, SvcLearn}
	case typ == 0xe:
		codes = []int{0x6e, 0x6f, 0x71, 0x70, SvcLearn}
	case typ == 0xf:
		codes = []int{0x74, 0x73, 0x72, SvcLearn}
	case typ == TypeTavern:
		codes = []int{SvcRentRoom, SvcBuyFood, SvcLearn, SvcArcomage}
	case typ == TypeBank:
		codes = []int{SvcDeposit, SvcWithdraw}
	case typ == TypeTemple:
		codes = []int{SvcHeal, SvcDonate, SvcLearn}
	case typ == TypeStables, typ == TypeBoats:
		codes = []int{SvcRoute0, SvcRoute0 + 1, SvcRoute0 + 2, SvcRoute0 + 3}
	case typ == TypeTraining:
		codes = []int{SvcTrain, SvcLearn}
	}
	return serviceButtons(codes...)
}

func serviceButtons(codes ...int) []Button {
	out := make([]Button, len(codes))
	for i, c := range codes {
		out[i] = Button{MsgService, c}
	}
	return out
}

// arcomageMenu is the tavern's Arcomage sub-menu: Rules, Victory Conditions, Play.
//
// mm8: 0x4b48b4
func arcomageMenu() []Button {
	return serviceButtons(SvcArcomageRules, SvcArcomageVictory, SvcArcomagePlay)
}

// deferred names the milestone of a service libre-enroth does not have yet ("" for
// one it has). They do nothing: the menu stays.
func deferred(typ, code int) string {
	switch {
	case typ == TypeTraining && code == SvcTrain:
		return "training (0x4b5618, M7)"
	case typ == TypeTownHall && code == SvcBounty:
		return "the bounty hunt (0x4bd028 code 99, the monsters: M8)"
	case code == SvcLearn || code >= 0x24 && code <= 0x4a:
		return "learning skills (0x4b43ce, M7)"
	case code >= SvcBuyStandard && code <= SvcRepair, code == SvcDisplay, code == SvcBuySpecial:
		return "shops (0x4b9820, 0x4b99e4, 0x4b4900, M7)"
	case code >= SvcGuild0 && code <= SvcGuild0+8:
		return "the guilds' spell shelves (M7)"
	case code == SvcArcomagePlay:
		return "Arcomage (msg 0x1d, M12)"
	}
	return ""
}

// GoldInput is the gold entry of the bank and the town hall: the text input
// (0x6c8ac8) opened for digits, 10 characters at most.
//
// mm8: 0x458c04 (TextInput_Open(&input, 1, 10, window))
type GoldInput struct {
	Text string
}

// GoldInputMax is the gold entry's length limit.
const GoldInputMax = 10

// selected is the selected member, nil for none.
func (d *Dialog) selected() *party.Player {
	m := d.h.Members()
	if m.Selected < 1 || m.Selected > len(m.Players) {
		return nil
	}
	return &m.Players[m.Selected-1]
}

// Blocked reports a selected member who cannot act: the menus of the shops, guilds,
// tavern, stables, boats and training hall then say "%s is in no condition to do
// anything" and their buttons do nothing.
//
// mm8: 0x4b22c1
func (d *Dialog) Blocked() bool {
	switch {
	case d.Type == TypeTemple, d.Type == TypeBank, d.Type == TypeTownHall:
		return false
	}
	p := d.selected()
	return p != nil && !p.CanAct()
}

// BlockedText is the text of a blocked menu.
//
// mm8: 0x4b22c1 (globalTxt 0x1ab with the name and globalTxt 0x232)
func (d *Dialog) BlockedText() string {
	p := d.selected()
	if p == nil {
		return ""
	}
	return cfmt(d.h.Global(0x1ab), p.Name, d.h.Global(0x232))
}

// service is a proprietor's button (msg 0x195): in the main menu it opens the
// sub-menu (the topic buttons go), a training the member is not ready for and a heal
// the member does not need excepted; the bank's and the town hall's sub-menus open the
// gold entry, the tavern's Arcomage menu its buttons; the services that act at once (a
// heal, a room, a journey, ...) do it here, where the original does it in the next
// frame's draw.
//
// mm8: 0x4bd028 (House_ClickService), the per-type draws 0x4b7cf2 (temple), 0x4b8cde
// (tavern), 0x4b87ec (bank), 0x4b78c9 (stables and boats), 0x4b8364 (town hall)
func (d *Dialog) service(code int) {
	if len(d.Buttons) == 0 || d.Menu == 1 && d.Blocked() {
		return
	}
	if d.Menu == 1 {
		if what := deferred(d.Type, code); what != "" {
			d.h.Note(what)
			return
		}
		if d.Type == TypeTemple && code == SvcHeal && d.selected() != nil && !d.CanHeal(d.selected()) {
			return
		}
		d.Buttons = nil
		d.Menu = code
	}
	switch d.Type {
	case TypeTownHall:
		if code == SvcPayFine {
			d.Input = &GoldInput{}
		}
		return
	case TypeBank:
		if d.Menu == SvcDeposit || d.Menu == SvcWithdraw {
			d.Input = &GoldInput{}
		}
		return
	case TypeStables, TypeBoats:
		if d.Menu >= SvcRoute0 && d.Menu <= SvcRoute0+3 {
			code := d.Menu
			d.Menu = -1
			d.travel(code)
		}
		return
	case TypeTemple:
		switch d.Menu {
		case SvcHeal:
			d.heal()
		case SvcDonate:
			d.donate()
		}
	case TypeTavern:
		switch d.Menu {
		case SvcRentRoom:
			d.rentRoom()
		case SvcBuyFood:
			d.buyFood()
		}
	}
	switch code {
	case SvcArcomage:
		d.Buttons = arcomageMenu()
	case SvcArcomageRules, SvcArcomageVictory:
		if d.Menu != SvcArcomagePlay {
			d.Menu = code
		}
	case SvcArcomagePlay:
		if d.Menu != SvcArcomagePlay {
			d.h.Note(deferred(d.Type, code))
		}
	}
}

// Merchant is the price a member pays: less their merchant discount, at least a third
// of it. The discount (the merchant skill and items) is M7's; it is 0 now.
//
// mm8: 0x4b28ab (with 0x491f5e the discount)
func Merchant(p *party.Player, price int) int {
	discount := 0
	v := (100 - discount) * price / 100
	return max(v, price/3)
}

// pay takes price gold, or says "You don't have enough gold".
//
// mm8: 0x4be3e6
func (d *Dialog) pay(price int) bool {
	m := d.h.Members()
	if uint32(price) <= uint32(m.Gold) {
		m.TakeGold(uint32(price))
		return true
	}
	d.h.Status(d.h.Global(0x9b), 2)
	return false
}

// val is the house's price factor (the 2DEvents Val column).
func (d *Dialog) val() float32 { return d.Def.Val }

// ---- temple --------------------------------------------------------------------------

// CanHeal reports a member the temple can heal: one with a condition, or short of
// hit or spell points.
//
// mm8: 0x4b7cbe
func (d *Dialog) CanHeal(p *party.Player) bool {
	if p.MainCondition() != party.CondGood {
		return true
	}
	h := d.h.Ctx().TimeHooks()
	return p.HP < h.MaxHP(p) || p.SP < h.MaxSP(p)
}

// HealPrice is the temple's price for healing p: the house's Val times the weekday
// (1..7) a condition began on, the latest weekday of the 14 lesser conditions, 5 times
// that of death or stone, 10 times that of eradication. Not merchant-adjusted.
//
// mm8: 0x4b8b1d (with 0x4b8ae1 the weekday)
func HealPrice(p *party.Player, val float32) int {
	weekday := func(c party.Condition) int {
		s := p.Conditions[c] * 15 / 64 // ftol(time * 0.234375): seconds
		return int(uint32(s/60/60)/24%7) + 1
	}
	kind, w := 1, 0
	switch c := p.MainCondition(); {
	case c == party.CondDead || c == party.CondStoned:
		kind, w = 5, weekday(c)
	case c == party.CondEradicated:
		kind, w = 10, weekday(c)
	default:
		for c := range party.Condition(14) {
			w = max(w, weekday(c))
		}
	}
	return max(int(float64(w*kind)*float64(val)), 1)
}

// heal is the temple's Heal: every condition cleared, full hit and spell points.
//
// mm8: 0x4b7cf2 (menu 10)
func (d *Dialog) heal() {
	p := d.selected()
	if p == nil || !d.CanHeal(p) {
		return
	}
	if d.pay(HealPrice(p, d.val())) {
		p.Conditions = [len(p.Conditions)]int64{}
		h := d.h.Ctx().TimeHooks()
		p.HP, p.SP = h.MaxHP(p), h.MaxSP(p)
		d.h.Note("the heal sound 0x87 (M11)")
		d.speak(0x52)
	}
	d.post()
}

// donate is the temple's Donate: Val gold; the map's reputation improves by one (down
// to -5) and every few donations, the more the later in the week, the temple casts a
// spell on the donor (M9).
//
// mm8: 0x4b7cf2 (menu 0xb)
func (d *Dialog) donate() {
	if d.pay(int(d.val())) {
		if loc := d.h.Location(); loc != nil {
			if r := le32(loc[8:]); r > -5 {
				r--
				put32(loc[8:], max(r, -5))
			}
		}
		m := d.h.Members()
		if s := m.Selected; s >= 0 && s < len(d.donations) {
			if int(d.donations[s]) < int(m.Calendar.Day%7) {
				d.donations[s]++
			} else {
				d.h.Note("the temple's donation spells (0x4263ca, M9)")
				d.donations[s] = 0
			}
		}
		d.speak(0x53)
		d.h.Status(d.h.Global(0x20f), 2)
	}
	d.post()
}

// ---- tavern --------------------------------------------------------------------------

// RoomPrice is the tavern's room: Val² / 10, merchant-adjusted, at least 1.
//
// mm8: 0x4b8cde
func (d *Dialog) RoomPrice() int {
	v := d.val()
	sq := v * v // stored as a float; the multiply by 0.1f is x87
	return max(Merchant(d.selected(), int(float64(sq)*float64(float32(0.1)))), 1)
}

// FoodPrice is filling the packs: Val³ / 100, merchant-adjusted, at least 1.
//
// mm8: 0x4b8cde
func (d *Dialog) FoodPrice() int {
	v := d.val()
	f := float64(v * v) // the square is stored as a float, the rest is x87
	return max(Merchant(d.selected(), int(f*float64(v)*float64(float32(0.01)))), 1)
}

// FoodDays is the food the packs are filled to (Val).
func (d *Dialog) FoodDays() int { return int(d.val()) }

// rentRoom pays for the room, leaves the house and rests until morning (msg 0x199).
//
// mm8: 0x4b8cde (menu 0xf)
func (d *Dialog) rentRoom() {
	if !d.pay(d.RoomPrice()) {
		d.post()
		return
	}
	d.Closed, d.RestInn = true, true
}

// buyFood fills the packs to Val days of food, unless they hold that much already.
//
// mm8: 0x4b8cde (menu 0x10)
func (d *Dialog) buyFood() {
	m := d.h.Members()
	if float32(uint32(m.Food)) < d.val() {
		if d.pay(d.FoodPrice()) {
			m.Food = int32(d.val())
		}
	} else {
		d.h.Status(d.h.Global(0x8c), 2)
		d.speak(0x43)
	}
	d.post()
}

// ---- stables and boats ---------------------------------------------------------------

// TravelPrice is a journey's price: 25 times Val in a stable, 50 times on a boat,
// merchant-adjusted (by the house's 2DEvents type).
//
// mm8: 0x4b78c9, 0x4b761b
func (d *Dialog) TravelPrice() int {
	base := 50
	if d.Def.Type == TypeStables {
		base = 25
	}
	return Merchant(d.selected(), int(float64(base)*float64(d.val())))
}

// RouteOffer is a journey a stable or boat offers today.
type RouteOffer struct {
	Code  int // the button's code
	Route *tables.Route
	Days  int // TravelDays, at least 1
}

// Routes are the journeys on offer today, in button order: a route goes today (day %
// 7) and its quest bit (if any) is set; a route repeating the one before is skipped.
//
// mm8: 0x4b78c9 (menu 1)
func (d *Dialog) Routes() []RouteOffer {
	t, m := d.h.Tables().Travel, d.h.Members()
	var out []RouteOffer
	prev := -1
	for i := range 4 {
		ri := t.HouseRoute(d.House, SvcRoute0+i)
		if ri == prev {
			continue
		}
		r := t.Route(ri)
		if r == nil || !r.Days[m.Calendar.Day%7] || r.QBit != 0 && !m.QBits.Get(int(int16(r.QBit))) {
			continue
		}
		prev = ri
		out = append(out, RouteOffer{Code: SvcRoute0 + i, Route: r, Days: max(r.TravelDays, 1)})
	}
	return out
}

// travel takes route button code (msg 499): pay, then — when it goes today — leave
// the house for the destination (a teleport on the same map) and let the days pass.
// Without the gold, back to the menu; on a day it does not go, the empty main menu.
//
// mm8: 0x4b761b (the stables' and boats' speech 0x47 / 0x48 and their 1.5 / 2.5 s wait
// before the journey are left out)
func (d *Dialog) travel(code int) {
	if !d.pay(d.TravelPrice()) {
		d.post()
		return
	}
	t, m := d.h.Tables().Travel, d.h.Members()
	r := t.Route(t.HouseRoute(d.House, code))
	if r == nil || !r.Days[m.Calendar.Day%7] {
		d.Menu = 1
		return
	}
	d.Closed = true
	file := d.h.MapStatsFile(r.Map)
	if equalFold(d.h.MapName(), file) {
		d.h.Teleport(r.X, r.Y, r.Z, r.Dir, 0, 0)
	} else {
		d.h.MoveToMap(file, r.X, r.Y, r.Z, r.Dir, 0, 0)
	}
	m.AdvanceTime(max(r.TravelDays, 1)*24*60, 0, d.h.Ctx())
}

// ---- bank and town hall --------------------------------------------------------------

// EnterGold ends the gold entry: Enter (ok) deposits, withdraws or pays the typed
// amount (as much as there is), Esc does nothing; either way back to the menu.
//
// mm8: 0x4b87ec (menus 7, 8), 0x4b8364 (menu 100)
func (d *Dialog) EnterGold(text string, ok bool) {
	if d.Input == nil {
		return
	}
	d.Input = nil
	m := d.h.Members()
	n := atoi(text)
	switch {
	case !ok:
	case d.Menu == SvcDeposit:
		if a := min(uint32(n), uint32(m.Gold)); a != 0 {
			m.TakeGold(a)
			m.Bank += int32(a)
			d.speak(0x51)
			d.Traded = true
		}
	case d.Menu == SvcWithdraw:
		if n == 0 {
			break
		}
		if a := min(uint32(n), uint32(m.Bank)); a == 0 {
			d.h.Note("the house sound 2 (M11)")
		} else {
			m.Gold += int32(a)
			m.Bank -= int32(a)
			d.Traded = true
		}
	case d.Menu == SvcPayFine:
		if n < 1 {
			break
		}
		a := n
		if uint32(m.Gold) < uint32(a) {
			a = m.Gold
		}
		a = min(a, m.Fine)
		m.TakeGold(uint32(a))
		m.Fine = max(m.Fine-a, 0)
		d.speak(0x51)
	}
	d.post()
}

// atoi reads the leading digits as the CRT's does, wrapping around past 2^31.
//
// mm8: 0x4dd7c3 (_atoi)
func atoi(s string) int32 {
	var n int32
	for i := 0; i < len(s) && s[i] >= '0' && s[i] <= '9'; i++ {
		n = n*10 + int32(s[i]-'0')
	}
	return n
}

// ---- house exit ----------------------------------------------------------------------

// enterExit takes the "Other Exits" portrait (msg 0xbf): out to the exit's map, at
// the fixed arrival of a negative quest bit, else at its Party Start.
//
// mm8: 0x42f877 (msg 0xbf; the save of the map is M10's)
func (d *Dialog) enterExit() {
	var a tables.Arrival
	if q := int(d.Def.QBit); q < 0 {
		if k := -q - 1; k < len(d.h.Tables().Travel.ExitArrivals) {
			a = d.h.Tables().Travel.ExitArrivals[k]
		}
	}
	d.h.MoveToMap(d.h.MapStatsFile(d.ExitMap), a.X, a.Y, a.Z, a.Dir, 0, 0)
	d.Closed = true
}

// speak makes the selected member say id.
func (d *Dialog) speak(id int) {
	if s := d.h.Members().Selected; s != 0 {
		d.h.Speak(s-1, id)
	}
}

func le32(b []byte) int32 {
	return int32(uint32(b[0]) | uint32(b[1])<<8 | uint32(b[2])<<16 | uint32(b[3])<<24)
}

func put32(b []byte, v int32) {
	b[0], b[1], b[2], b[3] = byte(v), byte(v>>8), byte(v>>16), byte(v>>24)
}

// equalFold is _stricmp == 0 for ASCII.
func equalFold(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range len(a) {
		x, y := a[i], b[i]
		if 'A' <= x && x <= 'Z' {
			x += 'a' - 'A'
		}
		if 'A' <= y && y <= 'Z' {
			y += 'a' - 'A'
		}
		if x != y {
			return false
		}
	}
	return true
}
