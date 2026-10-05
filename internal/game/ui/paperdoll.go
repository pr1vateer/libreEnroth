package ui

import (
	"fmt"
	"image"

	"libre-enroth/internal/assets/tables"
	"libre-enroth/internal/game/items"
	"libre-enroth/internal/game/party"
	"libre-enroth/internal/gfx"
)

// The character screen's paper doll: the body and hands of the member's face, the worn
// items drawn over them, and the jewellery view (re/notes/inventory.md#paper-doll).

// Paper doll tables in MM8-Rel.exe. Positions are offsets from the doll's base
// (0x4f7858, {467, 23}); body types are 0 male, 1 female, 2 minotaur, 3 troll, 4 dragon.
const (
	vaDollHands      = 0x4f7860 // [body]: off hand, main hand, bow {x, y} (0x80 bytes each)
	vaDollArmor      = 0x4f7a60 // [body*19 + armour]{x, y}
	vaDollBoots      = 0x4f7cc0 // [body*6 + boots]{x, y}
	vaDollCloakBack  = 0x4f7d80 // [body*6 + cloak]{x, y}
	vaDollCloakFront = 0x4f7e40 // the collar and the hood
	vaDollBelt       = 0x4f7f00 // [body*6 + belt]{x, y}
	vaDollHelm       = 0x4f7fc0 // [body*11 + helm]{x, y}
	vaDollLHOpen     = 0x4f8300 // [body]{x, y}: the left hand's default place
	vaDollLH2H       = 0x4f8320 // holding a two-handed weapon
	vaDollLHHold     = 0x4f8340 // holding something in the off hand
	vaDollRHFingers  = 0x4f8360 // the fingers over the main hand's weapon
	vaDollOffBase    = 0x4f8380 // the off hand's weapon
	vaDollRH         = 0x4f83a0 // [body]{x, y empty, x, y holding}
	vaDollRingX      = 0x4f83e0 // [6] the jewellery view's rings
	vaDollRingY      = 0x4f83f8
	vaDollBodyNames  = 0x4fede0 // per face: pcNNbod
	vaDollLHONames   = 0x4fec00 // pcNNLHo (open)
	vaDollLHUNames   = 0x4fec78 // pcNNLHu (holding)
	vaDollLH2Names   = 0x4fecf0 // two-handed
	vaDollRHBNames   = 0x4fee58 // pcNNRHb (fingers)
	vaDollRHDNames   = 0x4feed0 // pcNNRHd (empty)
	vaDollRHUNames   = 0x4fef48 // pcNNRHu (holding)
	dollBodies       = 5
	dollX, dollY     = 0x1d3, 0x17 // where the back of the doll goes
)

// dollTables are the doll's offsets and picture names.
type dollTables struct {
	base                                  image.Point
	off, main, bow                        [dollBodies]image.Point
	armor                                 []image.Point
	boots, cloakBack, cloakFront, belt    []image.Point
	helm                                  []image.Point
	lhOpen, lh2H, lhHold, rhFingers, offB [dollBodies]image.Point
	rhEmpty, rhHold                       [dollBodies]image.Point
	ringX, ringY                          []int32
	bodyOffs                              []image.Point
	back, body, lho, lhu, lh2, rhb, rhd   []string
	rhu                                   []string
}

func loadDollTables(l *loader) *dollTables {
	pts := func(va uint32, n int) []image.Point {
		v := l.exeInts(va, 2*n)
		out := make([]image.Point, n)
		for i := range out {
			out[i] = image.Pt(int(v[2*i]), int(v[2*i+1]))
		}
		return out
	}
	d := &dollTables{
		armor: pts(vaDollArmor, 4*19), boots: pts(vaDollBoots, 4*6),
		cloakBack: pts(vaDollCloakBack, 4*6), cloakFront: pts(vaDollCloakFront, 4*6),
		belt: pts(vaDollBelt, 4*6), helm: pts(vaDollHelm, 4*11),
		ringX: l.exeInts(vaDollRingX, 6), ringY: l.exeInts(vaDollRingY, 6),
		bodyOffs: pts(vaBodyOffsets, numFaces),
		back:     l.exeStrings(vaBackDollNames, numFaces), body: l.exeStrings(vaDollBodyNames, numFaces),
		lho: l.exeStrings(vaDollLHONames, numFaces), lhu: l.exeStrings(vaDollLHUNames, numFaces),
		lh2: l.exeStrings(vaDollLH2Names, numFaces), rhb: l.exeStrings(vaDollRHBNames, numFaces),
		rhd: l.exeStrings(vaDollRHDNames, numFaces), rhu: l.exeStrings(vaDollRHUNames, numFaces),
	}
	d.base = pts(vaDollBase, 1)[0]
	for b := 0; b < dollBodies; b++ {
		h := pts(vaDollHands+uint32(b)*0x80, 3)
		d.off[b], d.main[b], d.bow[b] = h[0], h[1], h[2]
		d.lhOpen[b] = pts(vaDollLHOpen+uint32(b)*8, 1)[0]
		d.lh2H[b] = pts(vaDollLH2H+uint32(b)*8, 1)[0]
		d.lhHold[b] = pts(vaDollLHHold+uint32(b)*8, 1)[0]
		d.rhFingers[b] = pts(vaDollRHFingers+uint32(b)*8, 1)[0]
		d.offB[b] = pts(vaDollOffBase+uint32(b)*8, 1)[0]
		rh := pts(vaDollRH+uint32(b)*16, 2)
		d.rhEmpty[b], d.rhHold[b] = rh[0], rh[1]
	}
	return d
}

// bodyType is the doll a face uses: trolls 3, minotaurs 2, dragons 4, else 1 for a
// woman and 0 for a man.
//
// mm8: 0x43aeee
func bodyType(face int) int {
	switch face {
	case 22, 23:
		return 3
	case 20, 21:
		return 2
	case 24, 25:
		return 4
	}
	if IsFemale(face) {
		return 1
	}
	return 0
}

// overlayName is the picture of worn item id on doll body (0..3) in part 0, 1 or 2
// ("" when there is none): armour "item%03dv%d" + "a" for part 0 (no part 2 on men
// and women), belts and boots "item%03dv%d", helms the item's picture + "v%d",
// cloaks "item%03dv%d" + a, b, c. Minotaurs and trolls (3, 4 in the original's 1-based
// numbering) share body 1's belts and cloaks; minotaurs body 1's tall helms. A few
// artifacts borrow numbers (0x201/0x215 0xfb, 0x202 0xfc, 0x203 0xfd, 0x206 0x106,
// 0x20a 0x104, 0x219 0x79).
//
// mm8: 0x43ac03 (PaperDoll_OverlayName; the loader 0x43a448 preloads only the items the
// member has, the others read "pending")
func overlayName(t *tables.Items, id int32, body, part int) string {
	v := body + 1
	tall := v == 3 || v == 4
	switch {
	case id >= 0x54 && id <= 0x62 || id == 0x201 || id == 0x202 || id == 0x203 || id == 0x215:
		switch id {
		case 0x201, 0x215:
			id = 0xfb
		case 0x202:
			id = 0xfc
		case 0x203:
			id = 0xfd
		}
		if tall {
			return fmt.Sprintf("item%03dv%d", id, v)
		}
		switch part {
		case 0:
			return fmt.Sprintf("item%03dv%da", id, v)
		case 1:
			return fmt.Sprintf("item%03dv%d", id, v)
		}
		return ""
	case id >= 0x75 && id <= 0x79 || id == 0x219:
		if id == 0x219 {
			id = 0x79
		}
		if tall {
			v = 1
		}
		return fmt.Sprintf("item%03dv%d", id, v)
	case id >= 0x84 && id <= 0x88 || id == 0x206:
		if id == 0x206 {
			id = 0x106
		}
		return fmt.Sprintf("item%03dv%d", id, v)
	case id >= 0x6d && id <= 0x74 || id == 0x208 || id == 0x209 || id == 0x218:
		if v == 2 && (id >= 0x72 && id <= 0x74 || id == 0x218) {
			v = 1
		}
		return fmt.Sprintf("%sv%d", t.Item(id).Picture, v)
	case id >= 0x7a && id <= 0x7e || id == 0x20a:
		if id == 0x20a {
			id = 0x104
		}
		if tall {
			v = 1
		}
		if part < 0 || part > 2 {
			return ""
		}
		return fmt.Sprintf("item%03dv%d%c", id, v, 'a'+part)
	}
	return ""
}

// paperDoll draws a member's doll.
type paperDoll struct {
	r    *Resources
	t    *dollTables
	it   *tables.Items
	magn *gfx.Sprite // MAGNIF-B
	hand *gfx.Sprite // BACKHAND
	// sparkle is the enchantment sparkle's time left (0x51e048); items with flags 0xf0
	// lose them when it runs out. The sparkle itself is M9's.
	sparkle int
}

func newPaperDoll(l *loader, it *tables.Items) *paperDoll {
	return &paperDoll{r: l.r, t: loadDollTables(l), it: it, magn: l.icon("MAGNIF-B", false), hand: l.icon("BACKHAND", false)}
}

func (d *paperDoll) icon(name string) *gfx.Sprite { return d.r.dialogIcon(name) }

// itemMask is how a worn or packed item is drawn: red when broken, green when not
// identified (if green), else plainly.
func itemMask(it *items.Item, green bool) gfx.Mask {
	switch {
	case it.Broken():
		return gfx.MaskRed
	case green && !it.Identified():
		return gfx.MaskGreen
	}
	return gfx.MaskNone
}

// drawItem draws an item picture as the doll and the pack do: an item with an
// enchantment sparkle (flags 0xf0) plainly while the sparkle lasts (the sparkle's blend
// is M9's), else by itemMask. dt is the timer ticks of this frame.
//
// mm8: 0x43aeee, 0x41a914 (the flags & 0xf0 / 2 / 1 chain; Screen_DrawTextureSparkle 0x4a64f5)
func (d *paperDoll) drawItem(c *gfx.Canvas, s *gfx.Sprite, x, y int, it *items.Item, green, rotated bool, dt int) {
	if it.Flags&0xf0 != 0 {
		d.sparkle -= dt
		if d.sparkle < 1 {
			d.sparkle = 0
			it.Flags &^= 0xf0
		}
		c.BlitEx(s, x, y, gfx.MaskNone, rotated)
		return
	}
	c.BlitEx(s, x, y, itemMask(it, green), rotated)
}

// draw draws member p's doll: the back (opaque), then within it the bow, the cloak's
// back, the body (a dragon's opaque, and nothing more), the left and right hands,
// armour, boots, belt, the cloak's collar, helm, the hood of cloak 0x7e, the main hand's
// weapon and its fingers, and the off hand's item (swords and daggers turned, with the
// left hand over them). Each worn item writes its slot into pick unless magnified; the
// magnifier icon follows.
//
// mm8: 0x43aeee (PaperDoll_Draw)
func (d *paperDoll) draw(c *gfx.Canvas, p *party.Player, pick *pickBuffer, magnified bool, dt int) {
	t, it := d.t, d.it
	body := bodyType(p.Face)
	back := d.icon(t.back[p.Face])
	c.ResetClip()
	c.Blit(back, dollX, dollY)
	clipIncl(c, dollX, dollY, min(back.W+dollX, 0x280), min(back.H+dollY, 0x1e0))
	clip := c.Clip()
	base := t.base
	reg := func(s *gfx.Sprite, x, y int, n int32, rotated bool) {
		if !magnified {
			pick.keyed(s, x, y, n, rotated, clip)
		}
	}
	worn := func(slot int) (*items.Item, int32) {
		n := p.Equip[slot]
		if n == 0 {
			return nil, 0
		}
		return p.Item(n), n
	}
	defOf := func(slot int) *tables.ItemDef {
		if w, _ := worn(slot); w != nil {
			return w.Def(it)
		}
		return nil
	}
	b := min(body, 4)
	// The bow on the back.
	if w, n := worn(party.SlotBow); w != nil {
		dd := w.Def(it)
		x := t.bow[b].X - int(dd.EquipX) + base.X
		y := t.bow[b].Y - int(dd.EquipY) + base.Y
		s := d.icon(dd.Picture)
		d.drawItem(c, s, x, y, w, true, false, dt)
		reg(s, x, y, n, false)
	}
	cloakIndex := func(id int32) int {
		k := int(id) - 0x7a
		if id == 0x20a {
			k = 5
		}
		return k
	}
	// The cloak's back (part a).
	if w, n := worn(party.SlotCloak); w != nil && body < 4 {
		if k := cloakIndex(w.Number); k >= 0 && k < 6 {
			pos := t.cloakBack[k+body*6]
			s := d.icon(overlayName(it, w.Number, body, 0))
			x, y := pos.X+base.X, pos.Y+base.Y
			d.drawItem(c, s, x, y, w, false, false, dt)
			reg(s, x, y, n, false)
		}
	}
	bo := t.bodyOffs[p.Face]
	bodyPic := d.icon(t.body[p.Face])
	if body == 4 {
		c.Blit(bodyPic, bo.X+base.X, bo.Y+base.Y)
		c.ResetClip()
		d.drawMagnifier(c, magnified)
		return
	}
	c.BlitKeyed(bodyPic, bo.X+base.X, bo.Y+base.Y)
	main, off := defOf(party.SlotMainHand), defOf(party.SlotOffhand)
	// The left hand: open, holding the off hand's item, or on a two-handed weapon.
	lh, lhPos := d.icon(t.lho[p.Face]), t.lhOpen[b]
	if main == nil || main.EquipType != tables.EquipWeapon2 || main.Skill != party.SkillSpear && off != nil {
		if off != nil && off.Skill != party.SkillShield {
			lh, lhPos = d.icon(t.lhu[p.Face]), t.lhHold[b]
		}
	} else {
		lh, lhPos = d.icon(t.lh2[p.Face]), t.lh2H[b]
	}
	c.BlitKeyed(lh, lhPos.X+base.X, lhPos.Y+base.Y)
	if main == nil {
		c.BlitKeyed(d.icon(t.rhd[p.Face]), t.rhEmpty[b].X+base.X, t.rhEmpty[b].Y+base.Y)
	} else {
		c.BlitKeyed(d.icon(t.rhu[p.Face]), t.rhHold[b].X+base.X, t.rhHold[b].Y+base.Y)
	}
	// Armour: part a with the main hand empty, else part 1.
	if w, n := worn(party.SlotArmor); w != nil {
		k := int(w.Number) - 0x54
		switch w.Number {
		case 0x201:
			k = 0xf
		case 0x202:
			k = 0x10
		case 0x203:
			k = 0x11
		case 0x215:
			k = 0x12
		}
		if k >= 0 && k <= 0x12 {
			pos := t.armor[k+body*0x13]
			part := 1
			if main == nil {
				part = 0
			}
			s := d.icon(overlayName(it, w.Number, body, part))
			x, y := pos.X+base.X, pos.Y+base.Y
			d.drawItem(c, s, x, y, w, true, false, dt)
			reg(s, x, y, n, false)
		}
	}
	simple := func(slot int, first, artifact int32, count int, tbl []image.Point) {
		w, n := worn(slot)
		if w == nil {
			return
		}
		k := int(w.Number - first)
		if w.Number == artifact {
			k = count - 1
		}
		if k < 0 || k >= count {
			return
		}
		pos := tbl[k+body*count]
		s := d.icon(overlayName(it, w.Number, body, 0))
		x, y := pos.X+base.X, pos.Y+base.Y
		d.drawItem(c, s, x, y, w, true, false, dt)
		reg(s, x, y, n, false)
	}
	simple(party.SlotBoots, 0x84, 0x206, 6, t.boots)
	simple(party.SlotBelt, 0x75, 0x219, 6, t.belt)
	// The cloak's collar (part b), unless its picture is missing.
	if w, n := worn(party.SlotCloak); w != nil {
		if k := cloakIndex(w.Number); k >= 0 && k < 6 {
			d.drawCloakPart(c, w, n, overlayName(it, w.Number, body, 1), t.cloakFront[k+body*6], base, reg, dt)
		}
	}
	// The helm.
	if w, n := worn(party.SlotHelm); w != nil {
		k := int(w.Number) - 0x6d
		switch w.Number {
		case 0x208:
			k = 8
		case 0x209:
			k = 9
		case 0x218:
			k = 10
		}
		if k >= 0 && k <= 10 {
			pos := t.helm[k+body*0xb]
			s := d.icon(overlayName(it, w.Number, body, 0))
			x, y := pos.X+base.X, pos.Y+base.Y
			d.drawItem(c, s, x, y, w, true, false, dt)
			reg(s, x, y, n, false)
		}
	}
	// Cloak 0x7e's hood (part c).
	if w, n := worn(party.SlotCloak); w != nil && w.Number == 0x7e {
		d.drawCloakPart(c, w, n, overlayName(it, w.Number, body, 2), t.cloakFront[body*6+4], base, reg, dt)
	}
	// The main hand's weapon and the fingers over it.
	if w, n := worn(party.SlotMainHand); w != nil {
		dd := w.Def(it)
		x := t.rhHold[b].X - int(dd.EquipX) + t.main[b].X + 5 + base.X
		y := t.rhHold[b].Y - int(dd.EquipY) + t.main[b].Y - 2 + base.Y
		s := d.icon(dd.Picture)
		d.drawItem(c, s, x, y, w, true, false, dt)
		reg(s, x, y, n, false)
		c.BlitKeyed(d.icon(t.rhb[p.Face]), t.rhFingers[b].X+base.X, t.rhFingers[b].Y+base.Y)
	}
	// The off hand's item; a sword or dagger turned 90°, the left hand drawn over it.
	if w, n := worn(party.SlotOffhand); w != nil {
		dd := w.Def(it)
		rot := dd.Skill == party.SkillSword || dd.Skill == party.SkillDagger
		s := d.icon(dd.Picture)
		x0 := t.offB[b].X + base.X + t.lhOpen[b].X
		y0 := t.offB[b].Y + base.Y + t.lhOpen[b].Y
		var dx, dy int
		if !rot {
			dx, dy = t.off[b].X-int(dd.EquipX), t.off[b].Y-int(dd.EquipY)
		} else {
			dx = t.off[b].X - int(dd.EquipY)
			dy = int(dd.EquipX) - s.W + t.off[b].Y
		}
		d.drawItem(c, s, x0+dx, y0+dy, w, true, rot, dt)
		reg(s, x0+dx, y0+dy, n, rot)
		if rot {
			pic, pos := d.icon(t.lhu[p.Face]), t.lhHold[b]
			if main != nil && main.EquipType == tables.EquipWeapon2 && main.Skill == party.SkillSpear {
				pic, pos = d.icon(t.lh2[p.Face]), t.lh2H[b]
			}
			c.BlitKeyed(pic, pos.X+base.X, pos.Y+base.Y)
		}
	}
	c.ResetClip()
	d.drawMagnifier(c, magnified)
}

// drawCloakPart draws a cloak's collar or hood: red when broken, never green, and not
// at all when its picture is missing.
func (d *paperDoll) drawCloakPart(c *gfx.Canvas, w *items.Item, n int32, name string, pos, base image.Point, reg func(*gfx.Sprite, int, int, int32, bool), dt int) {
	s, err := d.r.Cache.Icon(name, false)
	if name == "" || err != nil {
		return // "pending"
	}
	x, y := pos.X+base.X, pos.Y+base.Y
	d.drawItem(c, s, x, y, w, false, false, dt)
	reg(s, x, y, n, false)
}

func (d *paperDoll) drawMagnifier(c *gfx.Canvas, magnified bool) {
	if !magnified {
		c.BlitKeyed(d.magn, 0x25b, 299)
	}
}

// drawJewelry is the magnified view: the doll, BACKHAND over it, the six rings, the
// amulet and the gauntlets centred in their boxes (each writing its whole rectangle into
// pick), and the magnifier.
//
// mm8: 0x43c854 (PaperDoll_DrawJewelry)
func (d *paperDoll) drawJewelry(c *gfx.Canvas, p *party.Player, pick *pickBuffer, dt int) {
	d.draw(c, p, pick, true, dt)
	c.BlitKeyed(d.hand, dollX, dollY)
	full := image.Rect(0, 0, gfx.ScreenW, gfx.ScreenH)
	put := func(w *items.Item, n int32, s *gfx.Sprite, x, y int) {
		d.drawItem(c, s, x, y, w, true, false, dt)
		pick.rect(s, x, y, n, full)
	}
	for k := 0; k < 6; k++ {
		if n := p.Equip[party.SlotRing+k]; n != 0 {
			w := p.Item(n)
			put(w, n, d.icon(w.Def(d.it).Picture), int(d.t.ringX[k]), int(d.t.ringY[k]))
		}
	}
	if n := p.Equip[party.SlotAmulet]; n != 0 {
		w := p.Item(n)
		s := d.icon(w.Def(d.it).Picture)
		put(w, n, s, (0x2e-s.W)/2+0x1e6, (0x50-s.H)/2+0x5b)
	}
	if n := p.Equip[party.SlotGauntlet]; n != 0 {
		w := p.Item(n)
		s := d.icon(w.Def(d.it).Picture)
		put(w, n, s, (0x2e-s.W)/2+0x241, (0x50-s.H)/2+0x58)
	}
	c.BlitKeyed(d.magn, 0x25b, 299)
}
