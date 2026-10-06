package ui

import (
	"image"

	"libre-enroth/internal/assets/tables"
	"libre-enroth/internal/game/dialog"
	"libre-enroth/internal/game/items"
	"libre-enroth/internal/game/party"
	"libre-enroth/internal/gfx"
)

// The shops' and guilds' screens: the shelves in the left area, the selected member's
// pack for Display Inventory, Sell, Identify and Repair, and the merchant's replies in
// the right panel (re/notes/houses.md#shops).

// The left area's button (msg 0x51): the main menu's rebuild puts it over the clip
// (0,0x17 0x1d5×0x165); House_RebuildSubMenu (Display Inventory) at 8,8 0x1c2×0x140.
//
// mm8: 0x4bd028, 0x4bcf8f
var (
	shelfArea = image.Rect(0, 0x17, 0x1d5, 0x17+0x165)
	packArea  = image.Rect(8, 8, 8+0x1c2, 8+0x140)
)

// shopArt is what the shop screens add to the dialogue's: fr_inven, the pop-up frame
// and the item drawing of the pack.
type shopArt struct {
	frInven *gfx.Sprite
	popup   *popupArt
	doll    *paperDoll
}

func loadShopArt(l *loader, t *tables.Items) *shopArt {
	return &shopArt{frInven: l.icon("fr_inven", false), popup: loadPopupArt(l), doll: &paperDoll{r: l.r, it: t}}
}

// dialogNotes is the dialogue screen as party.Notes.
type dialogNotes struct{ s *dialogScreen }

func (n dialogNotes) Global(i int) string             { return n.s.r.GlobalText(i) }
func (n dialogNotes) Status(text string, seconds int) { n.s.r.Status.Show(text, seconds) }
func (n dialogNotes) Stub(key, what string)           { n.s.r.note(key, what) }
func (n dialogNotes) AutonoteText(int) bool           { return false }

// shopItems is the item table.
func (s *dialogScreen) shopItems() *tables.Items {
	t, _ := s.r.Items()
	return t
}

// shopPanel is a shop's screen: the main menu, a shelf, Display Inventory with its
// buttons, or the pack for Sell, Identify and Repair (whose buttons stay where Display
// Inventory put them, undrawn and still working).
//
// mm8: 0x4b9b5b, 0x4bb328, 0x4b5e2c, 0x4ba6f6
func (s *dialogScreen) shopPanel() servicePanel {
	d, f := s.d, s.art.arrus
	switch {
	case d.Menu == 1:
		if len(d.Buttons) != 4 {
			break
		}
		labels := s.globals(0x86, 0x98, 0x9f, 0xa0)
		spots := spread(f, d.Buttons, labels, 0xae, 0x8a, 0)
		return servicePanel{spots: spots, draw: func(c *gfx.Canvas, hover int) { drawSpots(c, f, spots, hover, inkHover) }}
	case d.ShelfMenu():
		return servicePanel{draw: func(c *gfx.Canvas, _ int) { s.drawShelf(c) }}
	case d.PackMenu():
		spots := s.displaySpots()
		return servicePanel{spots: spots, draw: func(c *gfx.Canvas, hover int) {
			s.drawPackView(c)
			if d.Menu == dialog.SvcDisplay {
				drawSpots(c, f, spots, hover, inkHover)
				return
			}
			s.drawPackReply(c)
		}}
	}
	return servicePanel{draw: func(*gfx.Canvas, int) {}}
}

// displayLabels are the global.txt names of the Display Inventory buttons.
var displayLabels = map[int]int{dialog.SvcSell: 200, dialog.SvcIdentify: 0x71, dialog.SvcRepair: 0xb3}

// displaySpots lays out Display Inventory's buttons as its draw does: spread over 0xae
// from 0x8a.
//
// mm8: 0x4b9b5b (menu 0x5e)
func (s *dialogScreen) displaySpots() []topicSpot {
	var labels []string
	for _, b := range s.d.Buttons {
		labels = append(labels, s.r.GlobalText(displayLabels[b.Param]))
	}
	return spread(s.art.arrus, s.d.Buttons, labels, 0xae, 0x8a, 0)
}

// drawPackView is the selected member's pack over the left area: LEATHER, then the
// inventory as it draws in houses (ib-8pxbar at 0x1cd,0x15, food and gold, fr_inven, the
// items with the unidentified ones green), each item's slot into the pick buffer.
//
// mm8: 0x41ac11 (CharScreen_DrawLeather), 0x41a914 (CharScreen_DrawInventory, mode 0xd)
func (s *dialogScreen) drawPackView(c *gfx.Canvas) {
	a := s.art
	c.Blit(a.leather, 0, 0x17)
	c.Blit(a.bar8, 0x1cd, 0x15)
	drawGoldFood(c, a.scoreBG, a.smallnum, s.r.Party)
	c.Blit(s.shop.frInven, 0, 0x17)
	if p := s.selectedPlayer(); p != nil {
		drawPack(c, s.r, s.shopItems(), p, s.shop.doll, &s.pick, true, 2)
	}
}

// drawHint is the line under the left area: lucida at y 0x172, centred in 0x1c2 from 0xb.
//
// mm8: 0x4b5527 (House_DrawHint)
func (s *dialogScreen) drawHint(c *gfx.Canvas, id int) {
	t := s.r.GlobalText(id)
	f := s.art.lucida
	f.Draw(c, fullRect, f.CenterOffset(0x1c2, t)+0xb, 0x172, 0, 0, t, 0)
}

// drawBlocked draws "%s is in no condition to do anything" and reports a member who
// cannot act.
//
// mm8: 0x4b22c1 (House_CheckCanAct)
func (s *dialogScreen) drawBlocked(c *gfx.Canvas) bool {
	if !s.d.Blocked() {
		return false
	}
	centredText(c, s.art.arrus, s.d.BlockedText(), 0xd4, 0x65, inkYellow)
	return true
}

// packCell is the pack cell under x, y; ok is false outside the pack's columns.
//
// mm8: 0x4b9b5b.. ((y - 0x20 >> 5) · 14 + (x - 6 >> 5)), 0x46782d (Inventory_InPackX)
func packCell(x, y int) (cell int, ok bool) {
	return ((y-0x20)>>5)*items.GridW + (x-6)>>5, x > 5 && x < 0x1c6
}

// packItemAt is the selected member's pack item (1-based slot) under x, y, 0 for none.
func (s *dialogScreen) packItemAt(x, y int) int32 {
	p := s.selectedPlayer()
	cell, ok := packCell(x, y)
	if p == nil || !ok {
		return 0
	}
	n, _ := p.ItemAtCell(cell)
	return n
}

// drawPackReply is Sell's, Identify's or Repair's line under the pack and, unless the
// member cannot act, the merchant's reply to the item under the mouse.
//
// mm8: 0x4b9b5b.. (menus 3, 4, 5)
func (s *dialogScreen) drawPackReply(c *gfx.Canvas) {
	d := s.d
	s.drawHint(c, map[int]int{dialog.SvcSell: 199, dialog.SvcIdentify: 0xc5, dialog.SvcRepair: 0xc6}[d.Menu])
	if s.drawBlocked(c) {
		return
	}
	if n := s.packItemAt(s.mx, s.my); n != 0 {
		if t := d.PackReply(n); t != "" {
			centredText(c, s.art.arrus, t, 0xae, 0x8a, inkWhite)
		}
	}
}

// shelfPlace is where the picture of shelf place i goes and whether its pick is keyed:
// the weapon shop's table (x 70i + 0x3c, its own height down), the armour shop's two
// shelves (bottoms at 0x79, tops at 0xa6), the magic shops' and alchemists' two rows of
// six (x 75j + 0x28 kept within 0x12..0x1c9, bottoms at 0xce and 0x160), the guilds'
// (x 70j + 0x20, the same bottoms).
//
// mm8: 0x4b9b5b, 0x4bb328, 0x4b5e2c, 0x4ba6f6, 0x4b6b98
func (s *dialogScreen) shelfPlace(i int, pic *gfx.Sprite) (x, y int, keyed bool) {
	d := s.d
	w, h := pic.W, pic.H
	j := i % 6
	bottom := 0xce
	if i >= 6 {
		bottom = 0x160
	}
	switch d.Type {
	case dialog.TypeWeapons:
		return 0x46*i + 0x3c - w>>1, d.ShelfY[i] + 0x1e, false
	case dialog.TypeArmour:
		x = int(s.d.Shops().ArmourShelfX[i]) - w>>1
		if i < 4 {
			return x, 0x79 - h, true
		}
		return x, 0xa6, true
	case dialog.TypeMagic, dialog.TypeAlchemy:
		x = j*0x4b - w/2 + 0x28
		switch j {
		case 0:
			x = max(x, 0x12)
		case 5:
			x = min(x, 0x1c9-w)
		}
		return x, max(bottom-h, 0), false
	}
	return 0x20 + 0x46*j, max(bottom-h, 0), true
}

// drawShelf is a shelf: the shop type's picture at 8,0x17, the items (place + 1 into the
// pick buffer), then, unless the member cannot act, the hint, and when nothing is left
// the wait for the restock, else the merchant's reply to the item under the mouse.
//
// mm8: 0x4b9b5b.. (menus 2, 0x5f), 0x4b6b98 (0x6e..0x76), 0x4b4382 (House_DrawShelf)
func (s *dialogScreen) drawShelf(c *gfx.Canvas) {
	d := s.d
	c.Blit(s.r.dialogIcon(s.d.Shops().Picture(d.Type)), 8, 0x17)
	sh := d.Shelf()
	if sh == nil {
		return
	}
	t := s.shopItems()
	full := image.Rect(0, 0, gfx.ScreenW, gfx.ScreenH)
	for i := range d.ShelfSize() {
		if sh[i].Number == 0 {
			continue
		}
		pic := s.r.dialogIcon(sh[i].Def(t).Picture)
		x, y, keyed := s.shelfPlace(i, pic)
		c.BlitKeyed(pic, x, y)
		if keyed {
			s.pick.keyed(pic, x, y, int32(i+1), false, full)
		} else {
			s.pick.rect(pic, x, y, int32(i+1), full)
		}
	}
	if s.drawBlocked(c) {
		return
	}
	hint := 0xc4
	if d.Menu == dialog.SvcBuyStandard && d.IsShop() {
		hint = 0xc3
	}
	s.drawHint(c, hint)
	if d.ShelfEmpty() {
		centredText(c, s.art.arrus, d.RestockText(), 0xd4, 0x65, inkYellow)
		return
	}
	if id := s.pick.at(s.mx, s.my); id != 0 {
		if r := d.ShelfReply(int(id) - 1); r != "" {
			centredText(c, s.art.arrus, r, 0xae, 0x8a, inkWhite)
		}
	}
}

// clickArea is a click on the left area of a shelf or pack menu (msg 0x51).
//
// mm8: 0x4bdcd8 (House_ClickVideoArea)
func (s *dialogScreen) clickArea(in *Input) bool {
	d := s.d
	area := packArea
	switch {
	case !d.OnProprietor():
		return false
	case d.ShelfMenu():
		area = shelfArea
	case !d.PackMenu():
		return false
	}
	if !hit(area, in.X, in.Y) {
		return false
	}
	cell, ok := packCell(in.X, in.Y)
	if !ok {
		cell = -1
	}
	d.ClickArea(s.pick.at(in.X, in.Y), cell)
	return true
}

// clickPortrait is Party_ClickPortrait in a house (g_screenMode 0xd): an item on the
// cursor goes into that member's pack; otherwise the member is selected (not while the
// gold entry is open).
//
// mm8: 0x4213c0
func (s *dialogScreen) clickPortrait(in *Input) bool {
	m := s.r.Party
	for i, x := range portraitX {
		if !hit(image.Rect(x, portraitY, x+portraitW, portraitY+portraitH), in.X, in.Y) {
			continue
		}
		slot := i + 1
		if ctx, err := s.r.Ctx(); err == nil && m.DropOnPortrait(slot, ctx) {
			return true
		}
		if s.d.Input == nil && slot <= len(m.Players) && m.MouseItem.Number == 0 {
			m.Selected = slot
		}
		return true
	}
	return false
}

// updateRight follows the right button: the pop-up shows while it is held, and the
// selected member's identify and repair reactions speak once per press.
//
// mm8: 0x41697c (Mouse_RightClick), 0x433b68 (Mouse_RightRelease)
func (s *dialogScreen) updateRight(in *Input) {
	if in.Right {
		if !s.rightHeld {
			s.rightHeld, s.speakOnce = true, true
		}
	} else {
		s.rightHeld = false
	}
}

// drawOver draws the right-click pop-up of a house (x < 0x1d5, y <= 0x158): a shelf
// item's, or in the pack menus the pack item's, information. A guild book's spell box
// is M9's.
//
// mm8: 0x41697c (mode 0xd), 0x4b2576 (House_RightClick)
func (s *dialogScreen) drawOver(c *gfx.Canvas, in *Input) {
	d := s.d
	if !s.rightHeld || in.Y > 0x158 || in.X >= 0x1d5 || d.Kind != dialog.KindHouse || !d.OnProprietor() {
		return
	}
	var it *items.Item
	switch {
	case d.IsGuild() && d.ShelfMenu():
		if s.pick.at(in.X, in.Y) != 0 {
			s.r.note("guild-spell-popup", "the spell pop-up of a guild book (0x4b2054, spells.txt: M9)")
		}
		return
	case d.ShelfMenu():
		if id := s.pick.at(in.X, in.Y); id != 0 {
			if sh := d.Shelf(); sh != nil && sh[id-1].Number != 0 {
				it = &sh[id-1]
			}
		}
	case d.PackMenu():
		if p := s.selectedPlayer(); p != nil {
			if n := s.packItemAt(in.X, in.Y); n != 0 {
				it = p.Item(n)
			}
		}
	}
	if it == nil {
		return
	}
	ctx, err := s.r.Ctx()
	if err != nil {
		return
	}
	info := &itemInfo{t: s.shopItems(), m: s.r.Party, ctx: ctx, notes: dialogNotes{s}, speakOnce: &s.speakOnce}
	s.shop.popup.showItem(c, it, in.X, in.Y, info, s.r)
}

var _ party.Notes = dialogNotes{}
