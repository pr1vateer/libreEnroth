package ui

import (
	"fmt"
	"image"
	"strings"

	"libre-enroth/internal/assets/tables"
	"libre-enroth/internal/game/dialog"
	"libre-enroth/internal/game/party"
	"libre-enroth/internal/gfx"
	"libre-enroth/internal/gfx/text"
)

// The proprietors' screens: what each house type draws in the right panel for the
// menu it is in, and where its buttons are. The original lays its buttons out again in
// every frame's draw (re/notes/houses.md#services); so does this.

// inkYellow is the highlight of the temple, tavern, bank, town hall, guilds, stables and
// boats (the shops and the training halls use inkHover).
var inkYellow = gfx.RGB16(0xff, 0xff, 0x9b)

// servicePanel is a proprietor's screen for the current menu: the laid-out buttons
// and the draw (hover: the button under the mouse, -1 none).
type servicePanel struct {
	spots []topicSpot
	draw  func(c *gfx.Canvas, hover int)
}

// panelRect is the text area of the right panel (x 0x1e3, 0x94 wide).
var panelRect = text.Rect{X: 0x1e3, W: 0x94, H: gfx.ScreenH}

// spread lays labels out down the panel: spacing (area - total)/n (at most limit when
// limit > 0), the first at ((area - n*spacing - total)/2 - spacing/2) + top.
//
// mm8: the menu draws 0x4b9b5b, 0x4b7cf2, 0x4b6b98, 0x4b8364, 0x4b8cde (menu 0x65)
func spread(f *text.Font, bs []dialog.Button, labels []string, area, top, limit int) []topicSpot {
	return spreadIn(f, bs, labels, area, area, top, limit)
}

// spreadIn is spread with the spacing taken from one area and the centring done in
// another (the town hall: 100 and 0x50).
func spreadIn(f *text.Font, bs []dialog.Button, labels []string, area, centre, top, limit int) []topicSpot {
	n := len(labels)
	if n == 0 {
		return nil
	}
	total := 0
	for _, l := range labels {
		total += textHeight(f, l, 0x94, 0)
	}
	sp := (area - total) / n
	if limit > 0 {
		sp = min(sp, limit)
	}
	y := ((centre-n*sp-total)/2 - sp/2) + top
	out := make([]topicSpot, n)
	for i, l := range labels {
		by := y + sp
		h := textHeight(f, l, 0x94, 0)
		out[i] = topicSpot{b: bs[i], label: l, r: image.Rect(0x1e0, by, 0x1e0+0x8c-1, by+h-1)}
		y = by - 1 + h
	}
	return out
}

// drawSpots draws laid-out labels, the hovered one in hi.
func drawSpots(c *gfx.Canvas, f *text.Font, spots []topicSpot, hover int, hi gfx.Color16) {
	for i, s := range spots {
		ink := inkWhite
		if i == hover {
			ink = hi
		}
		f.DrawCentered(c, panelRect, 0, s.r.Min.Y, ink, s.label, 3)
	}
}

// centredText draws s in the middle of the panel: at (area - h)/2 + top.
func centredText(c *gfx.Canvas, f *text.Font, s string, area, top int, ink gfx.Color16) {
	h := textHeight(f, s, 0x94, 0)
	f.DrawCentered(c, panelRect, 0, (area-h)/2+top, ink, s, 3)
}

// colour is the "\fNNNNN" colour code the menus put before each entry.
func colour(ink gfx.Color16) string { return fmt.Sprintf("\f%05d", ink) }

// servicePanel lays out the selected proprietor's screen.
//
// mm8: 0x4b3dec (House_Draw: the per-type draws)
func (s *dialogScreen) servicePanel(hover int) servicePanel {
	d, f := s.d, s.art.arrus
	if (d.Menu == 1 || d.Menu == dialog.SvcLearn) && d.Blocked() {
		// mm8: 0x4b22c1 (the buttons stop working)
		t := d.BlockedText()
		return servicePanel{draw: func(c *gfx.Canvas, _ int) { centredText(c, f, t, 0xd4, 0x65, inkYellow) }}
	}
	if d.Menu == dialog.SvcLearn {
		return s.learnPanel()
	}
	switch t := d.Type; {
	case t >= dialog.TypeWeapons && t <= dialog.TypeAlchemy:
		return s.shopPanel()
	case t == dialog.TypeTraining:
		return s.trainingPanel()
	case t >= 0xc && t <= 0xf:
		return s.guildPanel()
	case t == dialog.TypeTownHall:
		return s.townHallPanel()
	case t == dialog.TypeTavern:
		return s.tavernPanel(hover)
	case t == dialog.TypeBank:
		return s.bankPanel()
	case t == dialog.TypeTemple:
		return s.templePanel()
	case t == dialog.TypeStables, t == dialog.TypeBoats:
		return s.travelPanel(hover)
	case t == dialog.TypePrison:
		return servicePanel{draw: func(c *gfx.Canvas, _ int) { s.drawPrison(c) }}
	}
	return servicePanel{draw: func(*gfx.Canvas, int) {}}
}

// guildLabels are the global.txt names of the guilds' buttons.
var guildLabels = map[int]int{
	0x6e: 0x11b, 0x6f: 0x11c, 0x70: 0x11d, 0x71: 0x11e, 0x72: 0x121, 0x73: 0x122, 0x74: 0x123,
	0x75: 0x11f, 0x76: 0x120, dialog.SvcLearn: 0xa0,
}

// guildPanel is a guild's main menu: its spell shelves and Learn Skills, spread
// over 0x95 pixels from 0xa2; or a school's shelf.
//
// mm8: 0x4b6b98 (menu 1, 0x6e..0x76)
func (s *dialogScreen) guildPanel() servicePanel {
	d, f := s.d, s.art.arrus
	if d.ShelfMenu() {
		return servicePanel{draw: func(c *gfx.Canvas, _ int) { s.drawShelf(c) }}
	}
	if d.Menu != 1 {
		return servicePanel{draw: func(*gfx.Canvas, int) {}}
	}
	var labels []string
	for _, b := range d.Buttons {
		labels = append(labels, s.r.GlobalText(guildLabels[b.Param]))
	}
	spots := spread(f, d.Buttons, labels, 0x95, 0xa2, 0x20)
	return servicePanel{spots: spots, draw: func(c *gfx.Canvas, hover int) { drawSpots(c, f, spots, hover, inkYellow) }}
}

// townHallPanel is the town hall: the current fine under the menu (Bounty Hunt, and
// Pay Fine while there is one), or the gold entry for paying it.
//
// mm8: 0x4b8364
func (s *dialogScreen) townHallPanel() servicePanel {
	d, f, g := s.d, s.art.arrus, s.r.GlobalText
	fine := fmt.Sprintf("%s: %d", g(0x25d), s.r.Party.Fine)
	drawFine := func(c *gfx.Canvas) { f.DrawCentered(c, panelRect, 0, 0x104, inkYellow, fine, 3) }
	switch d.Menu {
	case 1:
		labels := s.globals(0x25c, 0x25b)[:min(len(d.Buttons), 2)]
		spots := spreadIn(f, d.Buttons, labels, 100, 0x50, 0x9e, 0)
		return servicePanel{spots: spots, draw: func(c *gfx.Canvas, hover int) {
			drawFine(c)
			drawSpots(c, f, spots, hover, inkYellow)
		}}
	case dialog.SvcPayFine:
		return servicePanel{draw: func(c *gfx.Canvas, _ int) {
			drawFine(c)
			s.drawGoldInput(c, g(0x25e))
		}}
	}
	return servicePanel{draw: func(c *gfx.Canvas, _ int) { drawFine(c) }}
}

// bankPanel is the bank: the balance, and Deposit / Withdraw at their House_AddService
// places, or the gold entry.
//
// mm8: 0x4b87ec
func (s *dialogScreen) bankPanel() servicePanel {
	d, f, g := s.d, s.art.arrus, s.r.GlobalText
	balance := fmt.Sprintf("%s: %d", g(0x19), s.r.Party.Bank)
	drawBalance := func(c *gfx.Canvas) { f.DrawCentered(c, panelRect, 0, 0xdc, inkYellow, balance, 3) }
	switch d.Menu {
	case 1:
		labels := s.globals(0x3c, 0xf4)
		spots := make([]topicSpot, 0, 2)
		for i, b := range d.Buttons {
			if i < len(labels) {
				y := 0x92 + 0x1e*i
				spots = append(spots, topicSpot{b: b, label: labels[i], r: image.Rect(0x1e0, y, 0x1e0+0x8c-1, y+0x1e-1)})
			}
		}
		return servicePanel{spots: spots, draw: func(c *gfx.Canvas, hover int) {
			drawBalance(c)
			for i, sp := range spots {
				ink := inkWhite
				if i == hover {
					ink = inkYellow
				}
				f.DrawCentered(c, panelRect, 0, []int{0x92, 0xb0}[i], ink, sp.label, 3)
			}
		}}
	case dialog.SvcDeposit, dialog.SvcWithdraw:
		what := g(0x3c)
		if d.Menu == dialog.SvcWithdraw {
			what = g(0xf4)
		}
		return servicePanel{draw: func(c *gfx.Canvas, _ int) {
			drawBalance(c)
			s.drawGoldInput(c, what)
		}}
	}
	return servicePanel{draw: func(c *gfx.Canvas, _ int) { drawBalance(c) }}
}

// drawGoldInput draws the gold entry: "<what>\nHow Much?" at 0x92, the digits typed at
// 0xba and the blinking caret after them.
//
// mm8: 0x4b87ec, 0x4b8364 (inputState 1), 0x4589b4 (TextInput_DrawCaret)
func (s *dialogScreen) drawGoldInput(c *gfx.Canvas, what string) {
	f, d := s.art.arrus, s.d
	if d.Input == nil {
		return
	}
	f.DrawCentered(c, panelRect, 0, 0x92, inkYellow, what+"\n"+s.r.GlobalText(0x70), 3)
	f.DrawCentered(c, panelRect, 0, 0xba, inkWhite, d.Input.Text, 3)
	if s.ticks%60 >= 30 {
		f.Draw(c, panelRect, f.TextWidth(d.Input.Text)/2+0x50, 0xb9, 0, 0, "_", 0)
	}
}

// templePanel is the temple's main menu: Heal (with its price, only when the member
// needs it), Donate, Learn Skills.
//
// mm8: 0x4b7cf2 (menu 1)
func (s *dialogScreen) templePanel() servicePanel {
	d, f, g := s.d, s.art.arrus, s.r.GlobalText
	if d.Menu != 1 || len(d.Buttons) != 3 {
		return servicePanel{draw: func(*gfx.Canvas, int) {}}
	}
	labels := []string{"", g(0x44), g(0xa0)}
	first := 1
	if p := s.selectedPlayer(); p != nil && d.CanHeal(p) {
		labels[0] = fmt.Sprintf("%s %d %s", g(0x68), dialog.HealPrice(p, d.Def.Val), g(0x61))
		first = 0
	}
	spots := spread(f, d.Buttons[first:], labels[first:], 0xae, 0x8a, 0x20)
	return servicePanel{spots: spots, draw: func(c *gfx.Canvas, hover int) { drawSpots(c, f, spots, hover, inkYellow) }}
}

// tavernPanel is the tavern: the main menu as one block of text (Rent Room, Fill Packs,
// Learn Skills, Play Arcomage), the Arcomage menu, or the Arcomage texts.
//
// mm8: 0x4b8cde
func (s *dialogScreen) tavernPanel(hover int) servicePanel {
	d, f, g := s.d, s.art.arrus, s.r.GlobalText
	switch d.Menu {
	case 1:
		if len(d.Buttons) != 4 {
			break
		}
		texts := []string{
			fmt.Sprintf(g(0xb2), d.RoomPrice()),
			fmt.Sprintf(g(0x56), d.FoodDays(), d.FoodPrice()),
			g(0xa0),
			g(0x263),
		}
		lh := f.Height - 3
		var hs [4]int
		var b strings.Builder
		for i, t := range texts {
			ink := inkWhite
			if i == hover {
				ink = inkYellow
			}
			entry := colour(ink) + t
			hs[i] = textHeight(f, entry, 0x94, 0)
			b.WriteString(entry)
			if i < 3 {
				b.WriteString("\n \n")
			}
		}
		// The Arcomage button's place adds its own height where Learn Skills' belongs,
		// as the original does.
		ys := []int{0x92, 0x92 + hs[0] + lh, 0x92 + hs[0] + hs[1] + 2*lh, 0x92 + hs[0] + hs[1] + hs[3] + 3*lh}
		heights := []int{hs[0], hs[1], hs[2], hs[3]}
		spots := make([]topicSpot, 4)
		for i := range spots {
			spots[i] = topicSpot{b: d.Buttons[i], label: texts[i], r: image.Rect(0x1e0, ys[i], 0x1e0+0x8c-1, ys[i]+heights[i]-1)}
		}
		block := b.String()
		return servicePanel{spots: spots, draw: func(c *gfx.Canvas, _ int) {
			f.DrawCentered(c, panelRect, 0, 0x92, 0, block, 3)
		}}
	case dialog.SvcArcomage, dialog.SvcArcomageRules, dialog.SvcArcomageVictory:
		if len(d.Buttons) != 3 {
			break
		}
		spots := spread(f, d.Buttons, s.globals(0x26c, 0x26e, 0x26d), 0xae, 0x8a, 0)
		if d.Menu == dialog.SvcArcomage {
			return servicePanel{spots: spots, draw: func(c *gfx.Canvas, hover int) { drawSpots(c, f, spots, hover, inkYellow) }}
		}
		// The rules and the victory conditions show in the reply box; the Arcomage
		// buttons stay where they were, undrawn.
		fallback := d.Menu == dialog.SvcArcomageRules
		t := d.TopicText(d.House + 0x1e)
		if fallback {
			t = d.TopicText(0x88)
		}
		return servicePanel{spots: spots, draw: func(c *gfx.Canvas, _ int) {
			s.art.drawReply(c, t, 0x1cc, 0xc, func(th int) bool { return fallback && 0x160-th < 8 })
		}}
	}
	return servicePanel{draw: func(*gfx.Canvas, int) {}}
}

// travelPanel is a stable's or boat's menu: the price, then a line per journey on offer
// today, or "Sorry, come back another day".
//
// mm8: 0x4b78c9 (menu 1)
func (s *dialogScreen) travelPanel(hover int) servicePanel {
	d, f, g := s.d, s.art.arrus, s.r.GlobalText
	if d.Menu != 1 {
		return servicePanel{draw: func(*gfx.Canvas, int) {}}
	}
	routes := d.Routes()
	if len(routes) == 0 {
		return servicePanel{draw: func(c *gfx.Canvas, _ int) { centredText(c, f, g(0x231), 0xae, 0x8a, inkWhite) }}
	}
	header := fmt.Sprintf(g(0x195), d.TravelPrice())
	lh := f.Height - 3
	y := 0x92 + textHeight(f, header, 0x94, 0) + lh
	var b strings.Builder
	b.WriteString(header + "\n \n")
	spots := make([]topicSpot, len(routes))
	for i, r := range routes {
		ink := inkWhite
		if i == hover {
			ink = inkYellow
		}
		line := fmt.Sprintf(g(0x194), r.Days, s.mapStatsName(r.Route.Map))
		b.WriteString(colour(ink) + line + "\n \n")
		h := textHeight(f, line, 0x94, 0)
		spots[i] = topicSpot{b: dialog.Button{Msg: dialog.MsgService, Param: r.Code}, label: line, r: image.Rect(0x1e0, y, 0x1e0+0x8c-1, y+h-1)}
		y += h + lh
	}
	block := b.String()
	return servicePanel{spots: spots, draw: func(c *gfx.Canvas, _ int) {
		f.DrawCentered(c, panelRect, 0, 0x92, 0, block, 3)
	}}
}

// drawPrison is the prison's sentence (house type 0x1f), in place of the portraits and
// of the proprietor's menu.
//
// mm8: 0x4b5dac
func (s *dialogScreen) drawPrison(c *gfx.Canvas) {
	centredText(c, s.art.arrus, s.r.GlobalText(0x2a0), 0x136, 0x12, inkYellow)
}

// learnPanel is the Learn Skills menu: "Skill Cost: %lu" at 0x92, then the skills the
// selected member's class has and the member does not know, spread over 0x95 from 0xa2
// (the others' buttons are moved out of reach); with none, "Seek knowledge elsewhere".
// The shops and the training halls highlight in inkHover, the guilds, temples and
// taverns in yellow; the refusal is inkHover in the weapon, armour and alchemy shops,
// yellow in taverns, white elsewhere.
//
// mm8: 0x4b5618 (menu 0x60), the same block in 0x4b9b5b, 0x4bb328, 0x4b5e2c, 0x4ba6f6,
// 0x4b6b98, 0x4b7cf2, 0x4b8cde
func (s *dialogScreen) learnPanel() servicePanel {
	d, f, g := s.d, s.art.arrus, s.r.GlobalText
	p := s.selectedPlayer()
	if p == nil {
		return servicePanel{draw: func(*gfx.Canvas, int) {}}
	}
	hi, none := inkYellow, inkWhite
	switch d.Type {
	case dialog.TypeWeapons, dialog.TypeArmour, dialog.TypeAlchemy:
		hi, none = inkHover, inkHover
	case dialog.TypeMagic, dialog.TypeTraining:
		hi = inkHover
	case dialog.TypeTavern:
		none = inkYellow
	}
	var bs []dialog.Button
	var labels []string
	for _, b := range d.Buttons {
		if sk := b.Param - dialog.LearnCode0; d.CanLearn(p, sk) {
			bs = append(bs, b)
			labels = append(labels, g(tables.SkillNameGlobal[sk]))
		}
	}
	if len(bs) == 0 {
		t := fmt.Sprintf(g(0x220), p.Name, g(tables.ClassNameGlobal+p.Class)) + "\n \n" + g(0x210)
		return servicePanel{draw: func(c *gfx.Canvas, _ int) { centredText(c, f, t, 0xae, 0x8a, none) }}
	}
	header := fmt.Sprintf(cDecimal(g(0x191)), d.LearnPrice())
	spots := spread(f, bs, labels, 0x95, 0xa2, 0x20)
	return servicePanel{spots: spots, draw: func(c *gfx.Canvas, hover int) {
		f.DrawCentered(c, panelRect, 0, 0x92, 0, header, 3)
		drawSpots(c, f, spots, hover, hi)
	}}
}

// trainingPanel is a training hall's main menu: the Train line (the offer, the
// experience still needed, or the cap's answer) and Learn Skills, spread over 0xae from
// 0x8a.
//
// mm8: 0x4b5618 (menu 1)
func (s *dialogScreen) trainingPanel() servicePanel {
	d, f, g := s.d, s.art.arrus, s.r.GlobalText
	p := s.selectedPlayer()
	if d.Menu != 1 || len(d.Buttons) != 2 || p == nil {
		return servicePanel{draw: func(*gfx.Canvas, int) {}}
	}
	labels := []string{d.TrainingLabel(p), g(0xa0)}
	spots := spread(f, d.Buttons, labels, 0xae, 0x8a, 0)
	return servicePanel{spots: spots, draw: func(c *gfx.Canvas, hover int) { drawSpots(c, f, spots, hover, inkHover) }}
}

// globals returns global.txt strings.
func (s *dialogScreen) globals(ids ...int) []string {
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = s.r.GlobalText(id)
	}
	return out
}

func (s *dialogScreen) selectedPlayer() *party.Player {
	m := s.r.Party
	if m.Selected < 1 || m.Selected > len(m.Players) {
		return nil
	}
	return &m.Players[m.Selected-1]
}

// mapStatsName is MapStats row i's name.
func (s *dialogScreen) mapStatsName(i int) string { return s.d.MapStatsName(i) }

// updateGoldInput types into the open gold entry: digits up to 10, Backspace, Enter
// (done) and Esc (cancel). It reports whether the entry had the keys.
//
// mm8: 0x458c04 (the text input in its digits mode)
func (s *dialogScreen) updateGoldInput(in *Input) bool {
	d := s.d
	if d.Input == nil {
		return false
	}
	switch {
	case in.Pressed(KeyEnter):
		d.EnterGold(d.Input.Text, true)
		return true
	case in.Pressed(KeyEscape):
		d.EnterGold("", false)
		return true
	}
	t := []byte(d.Input.Text)
	for _, r := range in.Runes {
		if r >= '0' && r <= '9' && len(t) < dialog.GoldInputMax {
			t = append(t, byte(r))
		}
	}
	if in.Pressed(KeyBackspace) && len(t) > 0 {
		t = t[:len(t)-1]
	}
	d.Input.Text = string(t)
	return false
}
