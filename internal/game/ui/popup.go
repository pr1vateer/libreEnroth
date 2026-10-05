package ui

import (
	"fmt"
	"image"

	"libre-enroth/internal/assets/tables"
	"libre-enroth/internal/game/clock"
	"libre-enroth/internal/game/items"
	"libre-enroth/internal/game/party"
	"libre-enroth/internal/gfx"
	"libre-enroth/internal/gfx/text"
)

// The right-click pop-ups: a parchment box with corners and edges, placed near the
// mouse (re/notes/inventory.md#pop-ups).

// popupArt are the box's pictures and the pop-up fonts.
type popupArt struct {
	parchment, ul, ur, ll, lr      *gfx.Sprite
	top, bottom, left, right       *gfx.Sprite
	arrus, comic, lucida, smallnum *text.Font
	create                         *text.Font
}

// loadPopupArt loads what Game_Init loads for the pop-ups.
//
// mm8: 0x4106f2.. (parchment, cornr_*, edge_* into 0x518d20..0x518d40)
func loadPopupArt(l *loader) *popupArt {
	return &popupArt{
		parchment: l.icon("parchment", false),
		ul:        l.icon("cornr_ul", false), ur: l.icon("cornr_ur", false),
		ll: l.icon("cornr_ll", false), lr: l.icon("cornr_lr", false),
		top: l.icon("edge_top", false), bottom: l.icon("edge_btm", false),
		left: l.icon("edge_lf", false), right: l.icon("edge_rt", false),
		arrus: l.font("arrus.fnt"), comic: l.font("comic.fnt"), lucida: l.font("lucida.fnt"),
		smallnum: l.font("smallnum.fnt"), create: l.font("create.fnt"),
	}
}

// clipIncl sets the clip from inclusive corners, as Screen_SetClip takes them.
func clipIncl(c *gfx.Canvas, x1, y1, x2, y2 int) { c.SetClip(image.Rect(x1, y1, x2+1, y2+1)) }

// drawFrame draws the box: w and h rounded up to even, parchment tiled from the corner,
// the four corners, and the edges when the box is wider or higher than 0x40.
//
// mm8: 0x414c9a (Popup_DrawFrame)
func (a *popupArt) drawFrame(c *gfx.Canvas, x, y, w, h int) {
	w += w & 1
	h += h & 1
	x2, y2 := x+w, y+h
	clipIncl(c, x, y, x2, y2)
	tw, th := a.parchment.W, a.parchment.H
	cols := w/tw + 1
	if w%tw != 0 {
		cols++
	}
	for row, ty := 0, y; row <= h/th; row, ty = row+1, ty+th {
		for k := 0; k < cols; k++ {
			c.Blit(a.parchment, x+k*tw, ty)
		}
	}
	clipIncl(c, x, y, x2+5, y2+5)
	c.BlitKeyed(a.ul, x, y)
	c.BlitKeyed(a.ll, x, y2-0x20)
	c.BlitKeyed(a.ur, x2-0x20, y)
	c.BlitKeyed(a.lr, x2-0x20, y2-0x20)
	if w > 0x40 {
		clipIncl(c, x+0x20, y, x2-0x1b, y2+5)
		c.BlitKeyed(a.top, x+0x20, y)
		c.BlitKeyed(a.bottom, x+0x20, y2-10)
		if w > 0x200 {
			c.BlitKeyed(a.top, x+0x220, y)
			c.BlitKeyed(a.bottom, x+0x220, y2-10)
		}
	}
	if h > 0x40 {
		clipIncl(c, x, y+0x20, x2+5, y2-0x1b)
		c.BlitKeyed(a.left, x, y+0x20)
		c.BlitKeyed(a.right, x2-10, y+0x20)
	}
	c.ResetClip()
}

// popupBox is the rect a pop-up draws in (the original's 0x54-byte rect: x, y, w, h,
// and +0x48 an optional text centred in lucida).
type popupBox struct {
	X, Y, W, H int
	Text       string
}

// draw keeps the box on the screen near the mouse (my): pushed left of the right edge
// or right of the left one, it moves 0x1e below the mouse; past the bottom, 0x1e above
// it. The frame is as high as the text + 0x18 (else the box), at least 0x40, ending
// above y 0x1da; the text is centred in it.
//
// mm8: 0x415072 (Popup_DrawBox, clip 0: the whole screen)
func (a *popupArt) draw(c *gfx.Canvas, b *popupBox, my int) {
	const minX, minY, maxX, maxY = 0, 0, gfx.ScreenW, gfx.ScreenH
	if b.X < minX {
		b.X, b.Y = minX, my+0x1e
	} else if maxX < b.X+b.W {
		b.X, b.Y = maxX-b.W, my+0x1e
	}
	if b.Y < minY {
		b.Y = my + 0x1e
	} else if b.H+b.Y > maxY {
		b.Y = my - b.H - 0x1e
	}
	b.Y, b.X = max(b.Y, minY), max(b.X, minX)
	inner := text.Rect{X: b.X + 0xc, Y: b.Y + 0xc, W: b.W - 0x18, H: b.H - 0xc}
	h := b.H
	th := 0
	if b.Text != "" {
		th = textHeight(a.lucida, b.Text, inner.W, 0)
		h = th + 0x18
	}
	h = max(h, 0x40)
	if b.Y+h+5 > 0x1df {
		h = 0x1da - b.Y
	}
	a.drawFrame(c, b.X, b.Y, b.W, h)
	if b.Text != "" {
		a.lucida.DrawCentered(c, inner, 0, (h-th)/2-0xe, 0, b.Text, 3)
	}
}

// drawText is a titled pop-up 0x180 wide at x 0x80 under the mouse: the title in
// create.fnt (light yellow), the text in smallnum.fnt.
//
// mm8: 0x41767c (Popup_DrawText)
func (a *popupArt) drawText(c *gfx.Canvas, title, body string, my int) {
	b := popupBox{X: 0x80, Y: my + 0x1e, W: 0x180}
	b.H = textHeight(a.smallnum, body, b.W, 0x18) + 0x18 + 2*a.lucida.Height
	a.draw(c, &b, my)
	r := text.Rect{X: b.X + 0xc, Y: b.Y + 0xc, W: b.W - 0x18, H: b.H - 0xc}
	t := fmt.Sprintf("\f%05d%s\f00000", uint16(inkTitle), title)
	a.create.DrawCentered(c, r, 0, 1, 0, t, 3)
	a.smallnum.Draw(c, r, 1, a.lucida.Height, 0, 0, body, 0)
}

// inkTitle is the pop-ups' title colour.
var inkTitle = gfx.RGB16(0xff, 0xff, 0x9b)

// itemInfo is what the item pop-up shows and does besides drawing.
type itemInfo struct {
	t     *tables.Items
	m     *party.Members
	ctx   *party.Ctx
	notes party.Notes
	// speakOnce is the flag that lets the pop-up's identify and repair reactions be
	// spoken once per right-button press (0x4f73d8).
	speakOnce *bool
}

// showItem is the item pop-up: before drawing, the selected member tries to identify
// an unidentified item and repair a broken one (by ID Item and Repair) and says how
// that went once per press; then the box shows the picture, the name and type, the
// attack/damage or armour, the bonus, power or charges, the notes and the value; an
// unidentified or broken item only its name and "Not identified"/"Broken" in red.
//
// mm8: 0x41d549 (Item_ShowInfo)
func (a *popupArt) showItem(c *gfx.Canvas, it *items.Item, x, my int, info *itemInfo, r *Resources) {
	if it.Number == 0 {
		return
	}
	t, m := info.t, info.m
	d := it.Def(t)
	pic := r.dialogIcon(d.Picture)
	b := popupBox{W: 0x180, H: 0xb4, Y: 0x28}
	if x < 0x141 {
		b.X = x + 0x1e
	} else {
		b.X = x - 0x1e - b.W
	}
	picX := 100 - pic.W
	if picX > 0 {
		picX >>= 1
	}
	picY := 0
	if v := 0x90 - pic.H; v >= 1 {
		picY = v >> 1
	}
	if d.IDRepair == 0 {
		it.Flags |= items.FlagIdentified
	}
	broken := it.Broken()
	var gold int32
	if d.EquipType == tables.EquipGold {
		gold = it.Special
	}
	if s := m.Selected; s >= 1 && s <= len(m.Players) {
		p := &m.Players[s-1]
		e := m.Env(info.ctx)
		say := func(id int) {
			if *info.speakOnce {
				m.Speak(s-1, id, info.ctx)
				*info.speakOnce = false
			}
		}
		if !it.Identified() {
			if p.CanIdentify(e, it) {
				it.Flags |= items.FlagIdentified
			}
			speech := 9
			if !it.Identified() {
				info.notes.Status(r.GlobalText(0x1be), 2)
			} else {
				speech = 8
				if it.Value(t) < int32(p.LevelBase+5)*100 {
					speech = 7
				}
			}
			say(speech)
		}
		it.ExpireBonus(int64(m.Time))
		if broken {
			if p.CanRepair(e, it) {
				it.Flags = it.Flags&^items.FlagBroken | items.FlagIdentified
			}
			speech := 0xb
			if !it.Broken() {
				speech = 0xa
			} else {
				info.notes.Status(r.GlobalText(0x1c0), 2)
			}
			say(speech)
		}
	}
	namer := m.Namer(info.notes)
	if it.Broken() || !it.Identified() {
		a.draw(c, &b, my)
		clipIncl(c, b.X+0xc, b.Y+0xc, b.X-0xc+b.W, b.Y-0xc+b.H)
		rr := text.Rect{X: b.X, Y: b.Y, W: b.W - 0x18, H: b.H - 0xc}
		name, msg := d.UnidentifiedName, r.GlobalText(0xe8)
		if it.Broken() {
			c.BlitEx(pic, picX+b.X, picY+0x1e+b.Y, gfx.MaskRed, false)
			if it.Identified() {
				name = it.FullName(t, namer)
			}
			msg = r.GlobalText(0x20)
		} else {
			c.BlitKeyed(pic, picX+b.X, picY+0x1e+b.Y)
		}
		a.arrus.DrawCentered(c, rr, 0, 0xc, inkTitle, name, 3)
		th := textHeight(a.arrus, msg, rr.W, 0)
		a.arrus.DrawCentered(c, rr, 100, rr.H>>1-th>>1, gfx.RGB16(0xff, 0x19, 0x19), msg, 3)
		c.ResetClip()
		return
	}
	var lines [3]string
	lines[0] = cfmt1(r.GlobalText(0x1cf), d.UnidentifiedName)
	switch typ := d.EquipType; {
	case typ < tables.EquipMissile, typ == tables.EquipMissile:
		label := r.GlobalText(0x12)
		if typ == tables.EquipMissile {
			label = r.GlobalText(0xcb)
		}
		lines[1] = fmt.Sprintf("%s: +%d   %s: %dd%d", label, d.Mod2, r.GlobalText(0x35), d.DiceCount, d.DiceSides)
		if d.Mod2 != 0 {
			lines[1] += fmt.Sprintf(" +%d", d.Mod2)
		}
	case typ > tables.EquipMissile && typ < tables.EquipWand && d.DiceCount != 0:
		lines[1] = fmt.Sprintf("%s: +%d", r.GlobalText(0xb), int(d.Mod2)+int(d.DiceCount))
	}
	if gold == 0 {
		switch {
		case d.EquipType == tables.EquipPotion:
			if it.Bonus != 0 {
				lines[2] = fmt.Sprintf("%s: %d", r.GlobalText(0x1c1), it.Bonus)
			}
		case d.EquipType == tables.EquipReagent:
			lines[2] = fmt.Sprintf("%s: %d", r.GlobalText(0x1c1), d.DiceCount)
		case it.Bonus != 0:
			lines[2] = fmt.Sprintf("%s: %s +%d", r.GlobalText(0xd2), stdStat(t, it.Bonus), it.Strength)
		case it.Special != 0:
			lines[2] = fmt.Sprintf("%s: %s", r.GlobalText(0xd2), spcDesc(t, it.Special))
		case it.Charges != 0:
			lines[2] = fmt.Sprintf("%s: %d", r.GlobalText(0x1d0), uint32(it.Charges))
		}
	}
	b.W -= 0xc
	rr := func() text.Rect { return text.Rect{X: b.X, Y: b.Y, W: b.W, H: b.H} }
	th := (a.arrus.Height + 8) * 3
	for _, s := range lines {
		if s != "" {
			th += 3 + textHeight(a.comic, s, b.W, 100)
		}
	}
	if d.Notes != "" {
		th += textHeight(a.smallnum, d.Notes, b.W, 100)
	}
	b.H = max(pic.H+0x36+picY, th)
	temp := it.Flags&items.FlagTempBonus != 0 && (it.Special != 0 || it.Bonus != 0)
	if temp {
		b.H += a.comic.Height
	}
	extra := 0
	full := it.FullName(t, namer)
	if a.arrus.Height != 0 && textHeight(a.arrus, full, b.W-0x18, 0)/a.arrus.Height != 0 {
		extra = a.arrus.Height
	}
	b.H += extra
	b.W += 0xc
	a.draw(c, &b, my)
	clipIncl(c, b.X+8, b.Y+8, b.X-8+b.W, b.Y-8+b.H)
	b.W -= 0xc
	hp := b.H - pic.H
	b.H -= 0xc
	c.BlitKeyed(pic, picX+b.X, hp/2+b.Y)
	ty := extra + 0x23
	for _, s := range lines {
		if s != "" {
			a.comic.Draw(c, rr(), 100, ty, 0, 0, s, 0)
			ty += 3 + textHeight(a.comic, s, b.W, 100)
		}
	}
	if d.Notes != "" {
		a.smallnum.Draw(c, rr(), 100, ty, 0, 0, d.Notes, 0)
	}
	a.arrus.DrawCentered(c, text.Rect{X: b.X + 0xc, Y: b.Y, W: b.W - 0x18, H: b.H}, 0, 0xc, inkTitle, full, 3)
	if gold != 0 {
		a.comic.Draw(c, rr(), 100, b.H-a.comic.Height, 0, 0, fmt.Sprintf("%s: %d", r.GlobalText(0x1d1), uint32(gold)), 0)
		c.ResetClip()
		return
	}
	if temp {
		a.comic.Draw(c, rr(), 100, b.H-2*a.comic.Height, 0, 0, durationText(it.Expires-int64(m.Time)), 0)
	}
	value := fmt.Sprintf("%s: %d", r.GlobalText(0x1d1), uint32(it.Value(t)))
	a.comic.Draw(c, rr(), 100, b.H-a.comic.Height, 0, 0, value, 0)
	tag := ""
	switch {
	case it.Flags&items.FlagStolen != 0:
		tag = r.GlobalText(0xbb)
	case it.Flags&items.FlagHardened != 0:
		tag = r.GlobalText(0x28b)
	}
	if tag != "" {
		// The colour is the surface's red channel mask (0xf800).
		a.comic.Draw(c, rr(), a.comic.TextWidth(value)+0x84, b.H-a.comic.Height, 0xf800, 0, tag, 0)
	}
	c.ResetClip()
}

// durationText is a temporary bonus's time left, "Duration:" and from the largest unit
// that is not 0: years, months, days, hours, minutes.
//
// mm8: 0x41d549, 0x493f45 (the time as a calendar)
func durationText(left int64) string {
	cal := clock.Time(left).Calendar()
	yr := cal.Year - clock.BaseYear
	s := "Duration:"
	on := false
	for _, u := range []struct {
		v    int
		unit string
	}{{yr, "yr"}, {cal.Month, "mo"}, {cal.Day, "dy"}, {cal.Hour, "hr"}, {cal.Minute, "mn"}} {
		if on = on || u.v != 0 || u.unit == "mo" && yr != 0; on {
			s += fmt.Sprintf(" %d:%s", u.v, u.unit)
		}
	}
	return s
}

func stdStat(t *tables.Items, n int32) string {
	if n < 1 || int(n) > len(t.Std) {
		return ""
	}
	return t.Std[n-1].Stat
}

func spcDesc(t *tables.Items, n int32) string {
	if n < 1 || int(n) > len(t.Spc) {
		return ""
	}
	return t.Spc[n-1].Description
}

// cfmt1 fills the first "%s" of a C format.
func cfmt1(f, s string) string {
	for i := 0; i+1 < len(f); i++ {
		if f[i] == '%' && f[i+1] == 's' {
			return f[:i] + s + f[i+2:]
		}
	}
	return f
}
