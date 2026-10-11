package dialog

import (
	"fmt"
	"math"

	"libre-enroth/internal/game/clock"
	"libre-enroth/internal/game/party"
)

// The bounty hunt (re/notes/houses.md, "Bounty hunt"): each month a monster carries a
// bounty of its level * 100 gold; killing one (Actor_Die) and asking again before the
// month ends pays it, with the award "Bounty hunter" for everyone.

// The bounty topic's event, state and texts (npctext.txt).
const (
	EventBounty     = 0x243
	EventArena      = 0x2c0
	StateBounty     = 0x53
	ParamBounty     = 0x53  // the topic column's one button (msg 0: it does nothing)
	textBounty      = 0x85  // "This month's bounty is on a %s. ... %lu gold reward."
	textBountyPaid  = 0x86  // "Congratulations on defeating the %s! Here is the %lu gold reward. ..."
	textBountyTaken = 0x87  // "Someone has already claimed the bounty this month. ..."
	textBountyKills = 0x26a // "... not yet reached the number of kills needed!"
	awardBounty     = 0x2c
	bountyMonsters  = 0xc3 // the topic draws monsters 4..195
	bountyFirstHall = 0x66 // town halls are houses 102..106, hunts 0..4
	// inkBounty is the monster's colour in the bounty text (Color16(0xff, 0xff, 0x9b)),
	// back to white after it.
	inkBounty = 0xfff3
	inkWhite  = 0xffff
)

// nextMonth is the time a bounty runs out: the start of the month after the calendar's,
// in the original's x87 arithmetic (months * 0x12750000 * (float)1/30).
//
// mm8: 0x4bc140, 0x4bd028
func nextMonth(c clock.Calendar) clock.Time {
	months := int64(c.Year)*12 + int64(c.Month) - 0x36ef
	return clock.Time(int64(math.Trunc(float64(months*0x12750000) * float64(float32(1.0/30)))))
}

// bountyText fills a bounty text with the monster's name (coloured) and reward.
//
// mm8: 0x4b363b (state 0x53), 0x4b8364 (menu 99): "\f%05d%s\f%05d" (0x503284)
func (d *Dialog) bountyText(text int, monster uint16) string {
	name, level := "", 0
	if mt := d.h.Tables().Monsters; mt != nil {
		if in := mt.Info(int(monster)); in != nil {
			name, level = in.Name, int(in.Level)
		}
	}
	args := []any{fmt.Sprintf("\f%05d%s\f%05d", inkBounty, name, inkWhite), level * 100}
	f := d.TopicText(text)
	return cfmt(f, args[:min(cVerbs(f), len(args))]...)
}

// cVerbs counts a C format's conversions (sprintf ignores the arguments past them).
func cVerbs(f string) int {
	n := 0
	for i := 0; i+1 < len(f); i++ {
		if f[i] == '%' {
			if f[i+1] == '%' {
				i++
				continue
			}
			n++
		}
	}
	return n
}

// payBounty pays hunt h's monster: level * 100 gold, the award for everyone, the
// party's bounty total; the hunt is over.
func (d *Dialog) payBounty(h *party.Hunt) {
	m := d.h.Members()
	gold := 0
	if mt := d.h.Tables().Monsters; mt != nil {
		if in := mt.Info(int(h.Monster)); in != nil {
			gold = int(in.Level) * 100
		}
	}
	d.h.AddGold(int32(gold))
	for i := range m.Players {
		d.h.SetAward(i, awardBounty)
	}
	m.Bounty += int32(gold)
	h.Monster, h.Killed = 0, 0
}

// bounty is the bounty topic: when the month's bounty has run out a new monster is
// drawn (4..195) and the kill forgotten. Then: no kill yet, the bounty (or, with a
// count of kills set, the kills reply; claimed already, "someone has claimed it"); a
// kill, the reward. The topic column shows one button that does nothing.
//
// mm8: 0x4bc140 (from House_ClickTopic 0x4b2b74 and NPC_ClickTopic 0x4bcbf2)
func (d *Dialog) bounty() {
	d.State = StateBounty
	d.Buttons = []Button{{0, ParamBounty}}
	d.Menu = -1
	m := d.h.Members()
	h := &m.Hunts[0]
	if h.Refresh < m.Time {
		h.Killed = 0
		h.Refresh = nextMonth(m.Calendar)
		rng := d.h.Ctx().Rand
		h.Monster = uint16(rng.Int()%bountyMonsters + 1)
		for h.Monster >= 1 && h.Monster <= 3 {
			h.Monster = uint16(rng.Int()%bountyMonsters + 1)
		}
	}
	mon := h.Monster
	text := textBountyTaken
	switch {
	case h.Killed != 0:
		text = textBountyPaid
		if mon != 0 {
			d.payBounty(h)
		}
	case mon != 0 && h.Kills != 0:
		text = textBountyKills
	case mon != 0:
		text = textBounty
	}
	d.Reply = d.bountyText(text, mon)
}

// townHallBounty is the town hall's Bounty Hunt (no MM8 house is a town hall): the
// hall's own hunt, renewed monthly with any monsters.txt monster and a kill count; a
// kill pays as the topic does.
//
// mm8: 0x4bd028 (House_ClickService, code 99), 0x4b8364 (House_DrawTownHall, menu 99)
func (d *Dialog) townHallBounty() {
	k := d.House - bountyFirstHall
	m := d.h.Members()
	if k < 0 || k >= len(m.Hunts) {
		return
	}
	h := &m.Hunts[k]
	if h.Refresh < m.Time {
		h.Killed = 0
		h.Refresh = nextMonth(m.Calendar)
		rng := d.h.Ctx().Rand
		h.Kills = uint16(rng.Int()%10 + 1)
		count := 1
		if mt := d.h.Tables().Monsters; mt != nil {
			count = len(mt.Infos)
		}
		h.Monster = uint16(rng.Int()%count + 1)
	}
	mon := h.Monster
	switch {
	case h.Killed != 0:
		if mon != 0 {
			d.payBounty(h)
		}
		d.Reply = d.bountyText(textBountyPaid, mon)
	case mon != 0:
		d.Reply = d.bountyText(textBounty, mon)
	default:
		d.Reply = d.bountyText(textBountyTaken, 0)
	}
}
