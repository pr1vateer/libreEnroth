package dialog

import (
	"slices"
	"testing"

	"libre-enroth/internal/assets/tables"
	"libre-enroth/internal/game/clock"
	"libre-enroth/internal/game/party"
)

// serviceHost is a host with one house of each service type the tests use: 20 temple
// (Val 10), 21 tavern (Val 6), 22 bank, 23 stable (2DEvents type 0x1b, Val 2) with the
// routes below, 24 boat, 25 town hall, 26 a shop.
func serviceHost(t *testing.T) *fakeHost {
	h := newHost()
	hs, an := h.t.Houses, h.t.HouseAnims
	for id, typ := range map[int]int{20: TypeTemple, 21: TypeTavern, 22: TypeBank, 23: TypeStables,
		24: TypeBoats, 25: TypeTownHall, 26: TypeWeapons} {
		hs[id] = tables.House{Name: "H", Type: int16(typ), Video: int16(100 + id), Open: 0, Closed: 0, Owner: "O", Title: "T"}
		an[100+id] = tables.HouseAnim{Video: "v", Portrait: 1, Type: uint8(typ)}
	}
	hs[20].Val, hs[21].Val, hs[23].Val, hs[24].Val = 10, 6, 2, 1.5
	tr := &tables.Travel{HouseRoutes: make([]byte, 30*4)}
	for i := range tr.HouseRoutes {
		tr.HouseRoutes[i] = tables.NoRoute
	}
	tr.Routes = []tables.Route{
		{Map: 3, Days: [7]bool{true, false, false, false, false, true}, TravelDays: 2, X: 10, Y: 20, Z: 30, Dir: 512},
		{Map: 2, Days: [7]bool{true, true, true, true, true, true, true}, TravelDays: 0, X: 1, Y: 2, Z: 3, Dir: 4},
		{Map: 4, Days: [7]bool{true, true, true, true, true, true, true}, TravelDays: 3, QBit: 40},
	}
	copy(tr.HouseRoutes[23*4:], []byte{0, 1, 1, 2})
	copy(tr.HouseRoutes[24*4:], []byte{1, 1, 1, 1})
	h.t.Travel = tr
	h.m.Players[0].Name = "Zed"
	h.m.Selected = 1
	h.m.Gold = 1000
	return h
}

// openProprietor enters house id and picks the proprietor.
func openProprietor(t *testing.T, h *fakeHost, id int) *Dialog {
	t.Helper()
	d := OpenHouse(h, id)
	if d == nil || !d.OnProprietor() || d.Menu != 1 {
		t.Fatalf("house %d: %+v", id, d)
	}
	return d
}

func clickService(d *Dialog, code int) { d.Click(Button{MsgService, code}) }

// TestServiceMenus: the proprietors' buttons by type; the town hall offers Pay Fine
// only while there is a fine.
//
// mm8: 0x4b496e (House_ServiceMenu)
func TestServiceMenus(t *testing.T) {
	m := &party.Members{}
	for _, c := range []struct {
		typ  int
		want []int
	}{
		{TypeWeapons, []int{2, 0x5f, 0x5e, 0x60}},
		{TypeTavern, []int{0xf, 0x10, 0x60, 0x65}},
		{TypeBank, []int{7, 8}},
		{TypeTemple, []int{10, 0xb, 0x60}},
		{TypeBoats, []int{0x69, 0x6a, 0x6b, 0x6c}},
		{TypeTraining, []int{0x11, 0x60}},
		{0xe, []int{0x6e, 0x6f, 0x71, 0x70, 0x60}},
		{TypeTownHall, []int{99}},
		{8, nil},
	} {
		if got := params(ServiceMenu(c.typ, m)); !slices.Equal(got, c.want) {
			t.Errorf("type %#x: %#x, want %#x", c.typ, got, c.want)
		}
	}
	m.Fine = 5
	if got := params(ServiceMenu(TypeTownHall, m)); !slices.Equal(got, []int{99, 100}) {
		t.Errorf("town hall with a fine: %#x", got)
	}
}

// TestHealPrice: the house's Val times the weekday (1..7) a condition began on, the
// latest of the 14 lesser ones; death and stone 5 times theirs, eradication 10 times.
//
// mm8: 0x4b8b1d, 0x4b8ae1
func TestHealPrice(t *testing.T) {
	var p party.Player
	if got := HealPrice(&p, 10); got != 10 {
		t.Errorf("healthy: %d, want 10 (weekday 1)", got)
	}
	p.Conditions[party.CondWeak] = int64(3*clock.Day + clock.Hour) // weekday 4
	p.Conditions[party.CondPoison1] = int64(clock.Day)             // weekday 2
	if got := HealPrice(&p, 10); got != 40 {
		t.Errorf("weak from day 3: %d, want 40", got)
	}
	p.Conditions[party.CondDead] = int64(8 * clock.Day) // weekday 2
	if got := HealPrice(&p, 10); got != 100 {
		t.Errorf("dead from day 8: %d, want 100 (5 x 2 x 10)", got)
	}
	p.Conditions[party.CondEradicated] = int64(6 * clock.Day) // weekday 7
	if got := HealPrice(&p, 1.5); got != 105 {
		t.Errorf("eradicated: %d, want 105 (10 x 7 x 1.5)", got)
	}
	if got := HealPrice(&party.Player{}, 0.1); got != 1 {
		t.Errorf("at least 1: %d", got)
	}
}

// TestTemple: Heal shows only for a member who needs it, takes the price and clears
// every condition; Donate takes Val, improves the map's reputation (down to -5) and says
// "Thank You!"; both step back to the menu.
//
// mm8: 0x4bd028 (code 10: 0x4b7cbe), 0x4b7cf2 (menus 10, 0xb)
func TestTemple(t *testing.T) {
	h := serviceHost(t)
	d := openProprietor(t, h, 20)
	clickService(d, SvcHeal)
	if d.Menu != 1 || len(d.Buttons) != 3 {
		t.Fatalf("heal of a healthy member opened menu %#x", d.Menu)
	}
	p := &h.m.Players[0]
	p.Conditions[party.CondDisease1] = int64(2 * clock.Day) // weekday 3
	clickService(d, SvcHeal)
	if p.Conditions[party.CondDisease1] != 0 || h.m.Gold != 1000-30 || !d.TakeBack() {
		t.Errorf("heal: condition %d, gold %d", p.Conditions[party.CondDisease1], h.m.Gold)
	}
	if d.Back(); d.Menu != 1 || len(d.Buttons) != 3 {
		t.Errorf("back to menu %#x %v", d.Menu, d.Buttons)
	}
	put32(h.loc[8:], -4)
	clickService(d, SvcDonate)
	if r := le32(h.loc[8:]); r != -5 || h.m.Gold != 960 || h.status[len(h.status)-1] != "g0x20f" {
		t.Errorf("donate: reputation %d, gold %d, %q", r, h.m.Gold, h.status)
	}
	d.TakeBack()
	d.Back()
	clickService(d, SvcDonate)
	if r := le32(h.loc[8:]); r != -5 {
		t.Errorf("reputation went past -5: %d", r)
	}
}

// TestTavern: a room costs Val²/10 and a night at the inn; the packs fill to Val days
// for Val³/100, unless they hold that much already ("Your packs are already full!").
// The float factors truncate: Val 10's food costs 9.
//
// mm8: 0x4b8cde
func TestTavern(t *testing.T) {
	h := serviceHost(t)
	d := openProprietor(t, h, 21)
	if r, f := d.RoomPrice(), d.FoodPrice(); r != 3 || f != 2 || d.FoodDays() != 6 {
		t.Errorf("Val 6: room %d, food %d, days %d; want 3, 2, 6", r, f, d.FoodDays())
	}
	h.m.Food = 2
	clickService(d, SvcBuyFood)
	if h.m.Food != 6 || h.m.Gold != 998 || !d.TakeBack() {
		t.Errorf("food %d gold %d", h.m.Food, h.m.Gold)
	}
	d.Back()
	clickService(d, SvcBuyFood)
	if h.m.Gold != 998 || h.status[len(h.status)-1] != "g0x8c" {
		t.Errorf("full packs: gold %d, %q", h.m.Gold, h.status)
	}
	d.TakeBack()
	d.Back()
	h.m.Gold = 2
	clickService(d, SvcRentRoom)
	if d.Closed || h.status[len(h.status)-1] != "g0x9b" || !d.TakeBack() {
		t.Errorf("a room without the gold: closed %v, %q", d.Closed, h.status)
	}
	d.Back()
	h.m.Gold = 10
	clickService(d, SvcRentRoom)
	if !d.Closed || !d.RestInn || h.m.Gold != 7 {
		t.Errorf("room: closed %v, rest %v, gold %d", d.Closed, d.RestInn, h.m.Gold)
	}
	for _, c := range []struct {
		val        float32
		room, food int
	}{{10, 10, 9}, {15, 22, 33}, {2, 1, 1}} { // 1000 x 0.01f truncates to 9
		h.t.Houses[21].Val = c.val
		if r, f := d.RoomPrice(), d.FoodPrice(); r != c.room || f != c.food {
			t.Errorf("Val %v: room %d, food %d; want %d, %d", c.val, r, f, c.room, c.food)
		}
	}
}

// TestTavernArcomage: the Arcomage menu's Rules and Victory Conditions step back to
// it, it steps back to the main menu; playing is M12's.
//
// mm8: 0x4bd028 (0x65..0x68), 0x4b48b4, 0x4bda0e (House_Back)
func TestTavernArcomage(t *testing.T) {
	h := serviceHost(t)
	d := openProprietor(t, h, 21)
	clickService(d, SvcArcomage)
	if d.Menu != SvcArcomage || !slices.Equal(params(d.Buttons), []int{0x66, 0x67, 0x68}) {
		t.Fatalf("menu %#x %#x", d.Menu, params(d.Buttons))
	}
	clickService(d, SvcArcomageVictory)
	if d.Menu != SvcArcomageVictory || d.TopicText(21+0x1e) != "text51" {
		t.Errorf("menu %#x", d.Menu)
	}
	d.Back()
	if d.Menu != SvcArcomage || len(d.Buttons) != 3 {
		t.Errorf("back: menu %#x", d.Menu)
	}
	d.Back()
	if d.Menu != 1 || len(d.Buttons) != 4 {
		t.Errorf("back again: menu %#x", d.Menu)
	}
}

// TestBank: Deposit and Withdraw open the gold entry; Enter moves as much as there is,
// Esc nothing; both step back. The balance follows.
//
// mm8: 0x4bd028 (bank), 0x4b87ec
func TestBank(t *testing.T) {
	h := serviceHost(t)
	d := openProprietor(t, h, 22)
	clickService(d, SvcDeposit)
	if d.Menu != SvcDeposit || d.Input == nil {
		t.Fatalf("deposit: menu %#x input %v", d.Menu, d.Input)
	}
	d.EnterGold("1500", true)
	if h.m.Gold != 0 || h.m.Bank != 1000 || d.Input != nil || !d.TakeBack() {
		t.Errorf("deposit 1500 of 1000: gold %d bank %d", h.m.Gold, h.m.Bank)
	}
	d.Back()
	clickService(d, SvcWithdraw)
	d.EnterGold("250x", true)
	if h.m.Gold != 250 || h.m.Bank != 750 {
		t.Errorf("withdraw 250: gold %d bank %d", h.m.Gold, h.m.Bank)
	}
	d.TakeBack()
	d.Back()
	clickService(d, SvcWithdraw)
	d.EnterGold("100", false)
	if h.m.Gold != 250 || !d.TakeBack() {
		t.Errorf("Esc: gold %d", h.m.Gold)
	}
	if atoi("4294967396") != 100 {
		t.Errorf("atoi wraps: %d", atoi("4294967396"))
	}
}

// TestTownHall: Pay Fine takes what is typed, up to the fine and the gold; the bounty
// hunt is M8's.
//
// mm8: 0x4b8364
func TestTownHall(t *testing.T) {
	h := serviceHost(t)
	h.m.Fine = 300
	d := openProprietor(t, h, 25)
	clickService(d, SvcBounty)
	if d.Menu != 1 || len(h.notes) == 0 {
		t.Errorf("bounty: menu %#x, notes %q", d.Menu, h.notes)
	}
	clickService(d, SvcPayFine)
	d.EnterGold("500", true)
	if h.m.Fine != 0 || h.m.Gold != 700 {
		t.Errorf("fine %d gold %d", h.m.Fine, h.m.Gold)
	}
}

// TestRoutes: a stable offers the routes that go today and whose quest bit is set, a
// route repeating the one before once; a journey pays 25 x Val (50 x on a boat), leaves
// the house, goes to the destination (a teleport on the same map) and lets the days
// pass with a rest; without the gold it steps back.
//
// mm8: 0x4b78c9, 0x4b761b
func TestRoutes(t *testing.T) {
	h := serviceHost(t)
	d := openProprietor(t, h, 23)
	codes := func() []int {
		var out []int
		for _, r := range d.Routes() {
			out = append(out, r.Code)
		}
		return out
	}
	if got := codes(); !slices.Equal(got, []int{0x69, 0x6a}) || d.Routes()[1].Days != 1 {
		t.Errorf("day 0: %#x", got)
	}
	h.m.QBits.Set(40, true)
	h.m.Calendar.Day = 1
	if got := codes(); !slices.Equal(got, []int{0x6a, 0x6c}) {
		t.Errorf("day 1 with quest bit 40: %#x", got)
	}
	if p := d.TravelPrice(); p != 50 {
		t.Errorf("stable price %d, want 50", p)
	}
	h.m.Calendar.Day = 0
	start := h.m.Time
	p := &h.m.Players[0]
	p.Conditions[party.CondWeak] = 1
	clickService(d, 0x69)
	if !d.Closed || h.m.Gold != 950 || len(h.moves) != 1 || h.moves[0] != "out03.odm 10 20 30 512" {
		t.Fatalf("travel: closed %v gold %d moves %q", d.Closed, h.m.Gold, h.moves)
	}
	if h.m.Time-start != 2*clock.Day || p.Conditions[party.CondWeak] != 0 {
		t.Errorf("time +%d, weak %d", h.m.Time-start, p.Conditions[party.CondWeak])
	}
	b := openProprietor(t, h, 24)
	if p := b.TravelPrice(); p != 75 {
		t.Errorf("boat price %d, want 75", p)
	}
	clickService(b, 0x69)
	if len(h.moves) != 2 || h.moves[1] != "teleport 1 2 3 4" || h.m.Time-start != 3*clock.Day {
		t.Errorf("same map: %q, time +%d", h.moves, h.m.Time-start)
	}
	b = openProprietor(t, h, 24)
	h.m.Gold = 10
	clickService(b, 0x69)
	if b.Closed || len(h.moves) != 2 || !b.TakeBack() {
		t.Errorf("without gold: closed %v", b.Closed)
	}
}

// TestBlocked: a member who cannot act blocks the shops' and the tavern's menus but
// not the temple's.
//
// mm8: 0x4b22c1
func TestBlocked(t *testing.T) {
	h := serviceHost(t)
	h.m.Players[0].Conditions[party.CondAsleep] = 1
	d := openProprietor(t, h, 21)
	if !d.Blocked() || d.BlockedText() == "" {
		t.Fatal("tavern not blocked")
	}
	clickService(d, SvcBuyFood)
	if d.Menu != 1 {
		t.Errorf("blocked click opened %#x", d.Menu)
	}
	if openProprietor(t, h, 20).Blocked() {
		t.Error("temple blocked")
	}
}

// TestEnterExit: the "Other Exits" portrait's Enter leaves for the exit's map, at the
// arrival table's point for a negative quest bit.
//
// mm8: 0x42f877 (msg 0xbf)
func TestEnterExit(t *testing.T) {
	h := serviceHost(t)
	h.t.Travel.ExitArrivals = []tables.Arrival{{X: 1, Y: 2, Z: 3, Dir: 4}, {X: 5, Y: 6, Z: 7, Dir: 8}}
	h.m.QBits.Set(30, true)
	d := OpenHouse(h, 5)
	d.SelectResident(len(d.Portraits) - 1)
	d.Click(Button{MsgEnterExit, 1})
	if !d.Closed || len(h.moves) != 1 || h.moves[0] != "out07.odm 0 0 0 0" {
		t.Errorf("exit: %v %q", d.Closed, h.moves)
	}
	h.t.Houses[5].QBit = -2
	d = OpenHouse(h, 5)
	d.SelectResident(len(d.Portraits) - 1)
	d.Click(Button{MsgEnterExit, 1})
	if h.moves[1] != "out07.odm 5 6 7 8" {
		t.Errorf("exit with arrival 2: %q", h.moves)
	}
}

// TestEdgeNeighbour: past an edge of outNN.odm the edge table names the neighbour,
// the days and the arrival marker; out of range or a zero entry, nothing.
//
// mm8: 0x48a53c
func TestEdgeNeighbour(t *testing.T) {
	tr := &tables.Travel{Edge: make([]byte, 0xa9)}
	k := 2*4 + 1 // out02, north
	tr.Edge[k], tr.Edge[0x34+k], tr.Edge[0x68+k] = 3, 5, 2
	tr.Edge[2*4+4] = 0xe // west: out of range
	if n, d, a, ok := EdgeNeighbour(tr, "Out02.odm", 0, 0x5801); !ok || n != "out03.odm" || d != 5 || a != 2 {
		t.Errorf("north: %q %d %d %v", n, d, a, ok)
	}
	for _, c := range []struct {
		name string
		x, y int32
	}{{"out02.odm", 0, 0x5800}, {"out02.odm", -0x5801, 0}, {"out02.odm", 0, -0x5801}, {"elema.odm", 0, 0x5801}, {"out2.odm", 0, 0x5801}} {
		if n, _, _, ok := EdgeNeighbour(tr, c.name, c.x, c.y); ok {
			t.Errorf("%s at %d,%d: %q", c.name, c.x, c.y, n)
		}
	}
}

// TestTransitionTexts: a house entrance plays the house's clip under topbar2 and reads
// its trans.txt row; a listed dungeon reads rows 1..14 without a clip; another exit
// asks to leave this map.
//
// mm8: 0x4425f5, 0x4428bc
func TestTransitionTexts(t *testing.T) {
	h := serviceHost(t)
	h.t.Travel.Dungeons = []string{"D21.blv", "D26.blv"}
	h.t.Trans = []string{"", "dungeon 1", "dungeon 2", "", "", "", "", "", "", "", "", "", "", "", "", "", "", "", "", "", "house 20"}
	tr := NewTransition(h, 20, 1, "out05.odm")
	if tr.Video != "v" || tr.Topbar != "topbar2" || tr.Icon != "ticon01" || tr.Title != "map5" || tr.Text != "house 20" {
		t.Errorf("house: %+v", tr)
	}
	tr = NewTransition(h, 0, 3, "d26.blv")
	if tr.Video != "" || tr.Topbar != "topbar" || tr.Title != "map0" || tr.Text != "dungeon 2" {
		t.Errorf("dungeon: %+v", tr)
	}
	tr = NewTransition(h, 0, 1, "out04.odm")
	if tr.Title != "map2" || tr.Text != "Do you wish to leave map2?" {
		t.Errorf("exit: %+v", tr)
	}
}
