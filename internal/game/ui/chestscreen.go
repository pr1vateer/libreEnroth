package ui

import (
	"fmt"
	"image"

	"libre-enroth/internal/assets/tables"
	"libre-enroth/internal/game/items"
	"libre-enroth/internal/game/party"
	"libre-enroth/internal/gfx"
)

// The chest screen (GuiChest, g_screenMode 10): the chest's items on its picture, and
// in mode 0xf the selected member's pack instead (re/notes/inventory.md#chests).

// Chest screen messages (GuiChest_Build 0x4d33eb).
const (
	msgChestClose = 0xb
	msgChestClick = 0xc
)

// chestScreen shows an opened chest.
type chestScreen struct {
	r       *Resources
	g       *inGame
	v       *ChestView
	ct      Container
	da      *dialogArt
	art     *popupArt
	ctx     *party.Ctx
	it      *tables.Items
	pick    pickBuffer
	pic     *gfx.Sprite // chest%02d
	ticon   *gfx.Sprite
	doll    *paperDoll  // the item drawing of the pack view (mode 0xf)
	fr      *gfx.Sprite // fr_inven
	px, py  []int32
	members bool // mode 0xf: the selected member's pack
	// The right button, as on the character screen.
	rightHeld, speakOnce, used bool
	closed                     bool
}

// newChestScreen opens chest v.
//
// mm8: 0x420093 (Chest_Open: GuiChest_Ctor 0x4d3339, topbar2 / ib-8pxbar / ib-mb-A),
// 0x4d33eb (GuiChest_Build; the sound 0x8d is M11's)
func newChestScreen(g *inGame, v *ChestView) (*chestScreen, error) {
	r := g.r
	ctx, err := r.Ctx()
	if err != nil {
		return nil, err
	}
	da, err := loadDialogArt(r)
	if err != nil {
		return nil, err
	}
	l := &loader{r: r}
	s := &chestScreen{r: r, g: g, v: v, da: da, art: loadPopupArt(l), ctx: ctx, it: ctx.Items}
	s.pic = r.dialogIcon(fmt.Sprintf("chest%02d", v.Picture))
	s.ticon = l.icon("ticonCH", false)
	s.doll = &paperDoll{r: r, it: ctx.Items}
	s.fr = l.icon("fr_inven", false)
	s.px, s.py = l.exeInts(vaPortraitX, 36), l.exeInts(vaPortraitY, 36)
	closeBtn := l.button(0x202, 0x136, Msg{ID: msgChestClose}, "c_close_up", "c_close_dn", "c_close_ht", true)
	closeBtn.Hotkey = KeyEscape
	s.ct.Add(closeBtn)
	grid := NewHotspot(7, 0x17, 0x1cc, 0x157, Msg{ID: msgChestClick})
	grid.activateOnPress = true
	s.ct.Add(grid)
	for _, h := range g.portraits.slots {
		s.ct.Add(h)
	}
	if l.err != nil {
		return nil, l.err
	}
	return s, nil
}

// cs is the chest screen as party.Notes.
type chestNotes struct{ s *chestScreen }

func (n chestNotes) Global(i int) string             { return n.s.r.GlobalText(i) }
func (n chestNotes) Status(text string, seconds int) { n.s.r.Status.Show(text, seconds) }
func (n chestNotes) Stub(key, what string)           { n.s.r.note(key, what) }
func (n chestNotes) AutonoteText(i int) bool {
	if w, ok := n.s.g.world.(interface{ AutonoteText(int) bool }); ok {
		return w.AutonoteText(i)
	}
	return false
}

// update runs one tick; it reports the screen closing.
//
// mm8: 0x42f877 (msgs 0xb, 0xc, 0x6e in modes 10 and 0xf)
func (s *chestScreen) update(in *Input) bool {
	m := s.r.Party
	s.ct.Update(in)
	for _, msg := range s.ct.Queue.Drain() {
		switch msg.ID {
		case msgChestClose:
			if s.members {
				s.members = false
			} else {
				s.closed = true
			}
		case msgChestClick:
			if s.members {
				s.clickPack(in)
			} else {
				s.click(in)
			}
		case msgSelectPlayer:
			s.clickPortrait(msg.Param)
		}
	}
	if in.Right {
		if !s.rightHeld {
			s.rightHeld, s.speakOnce, s.used = true, true, false
		}
	} else if s.rightHeld {
		s.rightHeld, s.used = false, false
	}
	if s.rightHeld && s.members && !s.used && m.MouseItem.Number != 0 {
		if sel := m.Selected - 1; sel >= 0 && sel < len(m.Players) && m.Players[sel].CanAct() {
			if slot := s.packItemUnder(in); slot != 0 {
				switch m.MixPotion(sel, slot, s.speakOnce, s.ctx, chestNotes{s}) {
				case party.MixDone, party.MixFailed, party.MixExplode:
					s.used, s.speakOnce = true, false
				}
			}
		}
	}
	return s.closed
}

// clickPortrait is Party_ClickPortrait in mode 10/0xf: the selected member again shows
// that member's pack (mode 0xf), another member is selected when able.
//
// mm8: 0x4213c0
func (s *chestScreen) clickPortrait(slot int) {
	m := s.r.Party
	if slot < 1 || slot > len(m.Players) || m.DropOnPortrait(slot, s.ctx) {
		return
	}
	if m.Selected == slot {
		s.members = true
		m.CharPage = CharPageInventory
		return
	}
	if m.Players[slot-1].Recovery == 0 {
		m.Selected = slot
	}
}

// click takes the item under the mouse (gold goes to the party) or puts the cursor's
// item in the first place it fits.
//
// mm8: 0x4209ac (Chest_Click)
func (s *chestScreen) click(in *Input) {
	m := s.r.Party
	ch, g := s.v.Chest, s.v.Grid
	if m.MouseItem.Number == 0 {
		id := s.pick.at(in.X, in.Y)
		if id == 0 {
			return
		}
		it := ch.Take(s.it, g, int(id)-1)
		if it.Number == 0 {
			return
		}
		if s.it.Item(it.Number).EquipType == tables.EquipGold {
			m.AddGold(it.Special, chestNotes{s})
			return
		}
		m.SetMouseItem(s.it, it)
		return
	}
	ok, full := ch.Put(s.it, g, m.MouseItem)
	switch {
	case ok:
		m.MouseItem = items.Item{}
	case full && m.Selected >= 1:
		m.Speak(m.Selected-1, 0xf, s.ctx)
	}
}

// clickPack is a click on the selected member's pack in mode 0xf.
func (s *chestScreen) clickPack(in *Input) {
	m := s.r.Party
	sel := m.Selected - 1
	if sel < 0 || sel >= len(m.Players) {
		return
	}
	hit := s.pick.at(in.X, in.Y)
	cell := -1
	if hit == 0 {
		cell = ((in.Y-0x20)>>5)*14 + (in.X-6)>>5
		if in.X > 0x1c6 || in.X < 5 || cell < 0 {
			return
		}
	}
	m.ClickPack(sel, s.it, hit, cell, s.ctx)
}

// packItemUnder is the pack item under the mouse in mode 0xf.
func (s *chestScreen) packItemUnder(in *Input) int32 {
	if n := s.pick.at(in.X, in.Y); n != 0 {
		return n
	}
	m := s.r.Party
	sel := m.Selected - 1
	if sel < 0 || sel >= len(m.Players) || in.X <= 5 || in.X >= 0x1c6 {
		return 0
	}
	cell := ((in.Y-0x20)>>5)*14 + (in.X-6)>>5
	if cell < 0 || cell > 0x7e {
		return 0
	}
	n, _ := m.Players[sel].ItemAtCell(cell)
	return n
}

// draw draws the screen: black over the view, then the chest (or the pack), the
// buttons; the portraits and the status line follow in inGame.Draw.
//
// mm8: 0x4d3378 (GuiChest_Draw: Screen_FillRect 0,0..640,367), 0x4205af (Chest_Draw)
func (s *chestScreen) draw(c *gfx.Canvas) {
	c.Fill(image.Rect(0, 0, gfx.ScreenW, 0x16f), gfx.Color16(0).RGBA())
	s.pick.clear()
	if s.members {
		s.drawPack(c)
	} else {
		s.drawChest(c)
	}
	s.ct.Draw(c)
}

// drawChest draws the chest's picture, the dialogue frame with ticonCH in the portrait
// place, food and gold, and the items centred in their cells, each writing its cell + 1
// into the pick buffer.
//
// mm8: 0x4205af (Chest_Draw)
func (s *chestScreen) drawChest(c *gfx.Canvas) {
	c.Blit(s.pic, 8, 0x17)
	c.Blit(s.da.topbar2, 0, 0)
	c.Blit(s.da.bar8, 0, 0x17)
	c.Blit(s.da.panel, 0x1d5, 0)
	px, py := int(s.px[6]), int(s.py[6])
	c.Blit(s.da.frame, px-4, py-4)
	c.Blit(s.ticon, px, py)
	drawGoldFood(c, s.da.scoreBG, s.da.smallnum, s.r.Party)
	ch, g := s.v.Chest, s.v.Grid
	full := image.Rect(0, 0, gfx.ScreenW, gfx.ScreenH)
	for cell := 0; cell < g.W*g.H && cell < len(ch.Grid); cell++ {
		n := ch.Grid[cell]
		if n <= 0 {
			continue
		}
		pic := s.r.dialogIcon(ch.Items[n-1].Def(s.it).Picture)
		x := cell%g.W*32 + g.X + (tables.Cells(pic.W)*32-pic.W)>>1
		// The original divides by the height for the row (both are 9).
		y := cell/g.H*32 + g.Y + (tables.Cells(pic.H)*32-pic.H)>>1
		c.BlitKeyed(pic, x, y)
		s.pick.keyed(pic, x, y, int32(cell+1), false, full)
	}
}

// drawPack is mode 0xf: LEATHER, then the selected member's pack as the inventory page
// draws it in this mode (topbar2, the right panel, food and gold, fr_inven).
//
// mm8: 0x4d3378 (CharScreen_DrawLeather 0x41ac11, CharScreen_DrawInventory 0x41a914)
func (s *chestScreen) drawPack(c *gfx.Canvas) {
	m := s.r.Party
	sel := m.Selected - 1
	if sel < 0 || sel >= len(m.Players) {
		return
	}
	c.Blit(s.da.leather, 0, 0x17)
	c.Blit(s.da.topbar2, 0, 0)
	c.Blit(s.da.panel, 0x1d5, 0)
	drawGoldFood(c, s.da.scoreBG, s.da.smallnum, m)
	c.Blit(s.fr, 0, 0x17)
	drawPack(c, s.r, s.it, &m.Players[sel], s.doll, &s.pick, false, 2)
}

// drawOver draws the right-click pop-up: the chest item under the mouse, or a pack
// item in mode 0xf; "%s is %s" when the selected member cannot act.
//
// mm8: 0x41697c (Mouse_RightClick, modes 10 and 0xf)
func (s *chestScreen) drawOver(c *gfx.Canvas, in *Input) {
	if !s.rightHeld || s.used {
		return
	}
	m := s.r.Party
	sel := m.Selected - 1
	if sel < 0 || sel >= len(m.Players) {
		return
	}
	p := &m.Players[sel]
	info := &itemInfo{t: s.it, m: m, ctx: s.ctx, notes: chestNotes{s}, speakOnce: &s.speakOnce}
	if s.members {
		if in.X >= 0x1d4 || in.Y > 0x158 {
			return
		}
		slot := s.packItemUnder(in)
		if slot == 0 {
			return
		}
		if !p.CanAct() {
			s.cannot(c, in, p)
			return
		}
		s.art.showItem(c, p.Item(slot), in.X, in.Y, info, s.r)
		return
	}
	if !p.CanAct() {
		s.cannot(c, in, p)
		return
	}
	id := s.pick.at(in.X, in.Y)
	if id == 0 {
		return
	}
	ch := s.v.Chest
	if n := ch.Grid[id-1]; n > 0 {
		s.art.showItem(c, &ch.Items[n-1], in.X, in.Y, info, s.r)
	}
}

// cannot is the "%s is %s" box of a member who cannot act.
func (s *chestScreen) cannot(c *gfx.Canvas, in *Input, p *party.Player) {
	b := popupBox{W: 0x180, H: 0xb4, Y: 0x28, Text: cfmt2(s.r.GlobalText(0x1ab), p.Name, s.r.GlobalText(0x21d))}
	if in.X < 0x141 {
		b.X = in.X + 0x1e
	} else {
		b.X = in.X - 0x1e - b.W
	}
	s.art.draw(c, &b, in.Y)
}
