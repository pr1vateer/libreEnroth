package world

import (
	"slices"
	"strings"
	"testing"

	"libre-enroth/internal/assets/tables"
	"libre-enroth/internal/game/dialog"
	"libre-enroth/internal/game/ui"
)

// TestShopTables: the shop tables of MM8-Rel.exe: 6, 8, 12 and 12 items by shop type,
// the shelf pictures, the armour shop's places, True Mettle's and the Tannery's records,
// the magic shops' and alchemists' levels and the guilds' book counts.
//
// mm8: 0x5029d0, 0x502758, 0x4ea9bc, 0x50279a.., 0x5030c2
func TestShopTables(t *testing.T) {
	e := newEnv(t)
	s := e.tables.Game.Shops
	if s.Counts != [tables.NumShopTypes]int{0, 6, 8, 12, 12} {
		t.Errorf("counts %v", s.Counts)
	}
	if got := s.Pictures[:5]; !slices.Equal(got, []string{"", "WEPNTABL", "ARMORY", "MAGSHELF", "MAGSHELF"}) {
		t.Errorf("pictures %q", got)
	}
	for typ := 0xc; typ <= 0xf; typ++ {
		if s.Picture(typ) != "MAGSHELF" {
			t.Errorf("guild type %#x: %q", typ, s.Picture(typ))
		}
	}
	if !slices.Equal(s.ArmourShelfX, []int32{114, 206, 296, 377, 114, 206, 296, 377}) {
		t.Errorf("armour shelf %v", s.ArmourShelfX)
	}
	for _, c := range []struct {
		name      string
		got, want tables.Shelf
	}{
		{"True Mettle standard", s.StdWeapons(1), tables.Shelf{Level: 1, Kinds: [4]int{23, 27, 25, 20}}},
		{"True Mettle special", s.SpcWeapons(1), tables.Shelf{Level: 2, Kinds: [4]int{28, 30, 26, 20}}},
		{"Tannery standard, top", s.StdArmour(15, 0), tables.Shelf{Level: 1, Kinds: [4]int{35, 35, 38, 38}}},
		{"Tannery standard, bottom", s.StdArmour(15, 1), tables.Shelf{Level: 1, Kinds: [4]int{31, 31, 31, 34}}},
	} {
		if c.got != c.want {
			t.Errorf("%s: %+v, want %+v", c.name, c.got, c.want)
		}
	}
	if a, b, c, d := s.MagicLevel(29, false), s.MagicLevel(29, true), s.AlchemyLevel(42, false), s.AlchemyLevel(42, true); a != 1 || b != 2 || c != 3 || d != 2 {
		t.Errorf("levels: magic %d / %d, alchemist %d / %d", a, b, c, d)
	}
	var books []int
	for h := 139; h <= 148; h++ {
		books = append(books, s.GuildSpellCount(h))
	}
	if !slices.Equal(books, []int{4, 7, 4, 11, 11, 0, 7, 10, 11, 11}) {
		t.Errorf("guild books %v", books)
	}
}

// TestShopSweep: every shop and guild opens at noon; every shelf fills with items whose
// pictures exist; the merchant's replies to every item fill all their codes; every
// shelf, Display Inventory and Sell draw; a purchase moves the item into the pack.
//
// mm8: 0x4bd028, 0x4b9820, 0x4b99e4, 0x4bcea6, 0x4955a7
func TestShopSweep(t *testing.T) {
	e := newEnv(t)
	a := e.app(t, "out01.odm")
	w := a.World().(*World)
	if w.Dialog() != nil {
		a.Update(&ui.Input{Keys: []ui.Key{ui.KeyEscape}})
	}
	it := e.tables.Game.Items
	icons := e.d.Icons
	shops := 0
	for id := 1; id < len(e.tables.Game.Houses); id++ {
		h := &e.tables.Game.Houses[id]
		typ := int(e.tables.Game.Anim(int(h.Video)).Type)
		if h.Video == 0 || !(typ >= 1 && typ <= 4 || typ >= 0xc && typ <= 0xf) {
			continue
		}
		w.S.SetTimeOfDay(12, 0)
		w.CloseDialog()
		w.SpeakInHouse(id)
		d := w.Dialog()
		if d == nil {
			t.Fatalf("house %d %q did not open at noon", id, h.Name)
		}
		var codes []int
		for _, b := range d.Buttons {
			if b.Param != dialog.SvcLearn && b.Param != dialog.SvcDisplay {
				codes = append(codes, b.Param)
			}
		}
		for _, code := range codes {
			d.Click(dialog.Button{Msg: dialog.MsgService, Param: code})
			sh := d.Shelf()
			if sh == nil {
				t.Fatalf("house %d code %#x: no shelf", id, code)
			}
			for i := range d.ShelfSize() {
				g := sh[i]
				if g.Number == 0 {
					t.Errorf("house %d code %#x: place %d empty", id, code, i)
					continue
				}
				if _, ok := icons.Find(g.Def(it).Picture); !ok {
					t.Errorf("house %d: item %d's picture %q", id, g.Number, g.Def(it).Picture)
				}
				if r := d.ShelfReply(i); r == "" || strings.Contains(r, "%") {
					t.Errorf("house %d item %d: reply %q", id, g.Number, r)
				}
			}
			a.Update(&ui.Input{X: 100, Y: 150})
			Compose(a, 640, 480)
			d.Back()
		}
		if typ <= 4 {
			d.Click(dialog.Button{Msg: dialog.MsgService, Param: dialog.SvcDisplay})
			a.Update(&ui.Input{X: 556, Y: 200})
			Compose(a, 640, 480)
			d.Click(dialog.Button{Msg: dialog.MsgService, Param: dialog.SvcSell})
			if d.Menu != dialog.SvcSell {
				t.Errorf("house %d: Sell opened %#x", id, d.Menu)
			}
			a.Update(&ui.Input{X: 20, Y: 50})
			Compose(a, 640, 480)
		}
		shops++
	}
	if shops < 30 {
		t.Errorf("%d shops and guilds", shops)
	}

	// A purchase at True Mettle.
	w.CloseDialog()
	w.SpeakInHouse(1)
	d := w.Dialog()
	d.Click(dialog.Button{Msg: dialog.MsgService, Param: dialog.SvcBuyStandard})
	m := w.S.Party
	m.Gold = 100000
	p := &m.Players[0]
	for c := range p.Grid {
		p.Grid[c] = 0
	}
	sh := d.Shelf()
	want := sh[0]
	d.ClickArea(1, -1)
	got := p.Item(p.Grid[0])
	if got == nil || got.Number != want.Number || !got.Identified() || sh[0].Number != 0 ||
		m.Gold != 100000-int32(dialog.BuyPrice(want.Value(it), h1Val(e), 0)) {
		t.Errorf("bought %v (wanted %v), gold %d", got, want, m.Gold)
	}
	w.CloseDialog()
}

// h1Val is True Mettle's price factor.
func h1Val(e *env) float32 { return e.tables.Game.Houses[1].Val }
